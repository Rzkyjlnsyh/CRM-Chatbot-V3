package handlers

import (
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"
)

// currentAgentID mengembalikan id agent dari path (:id), divalidasi milik tenant pemanggil.
// Endpoint lama tanpa :id memakai agent pertama milik tenant. 0 = tidak ada / bukan milik tenant.
func currentAgentID(c *gin.Context) uint {
	tid := currentTenantID(c)
	if tid == 0 {
		return 0
	}
	if p := c.Param("id"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		var a models.Agent
		if database.DB.Select("id").Where("id = ? AND tenant_id = ?", n, tid).First(&a).Error != nil {
			return 0
		}
		return a.ID
	}
	var a models.Agent
	database.DB.Select("id").Where("tenant_id = ?", tid).Order("id asc").First(&a)
	return a.ID
}

// resolveAgent memastikan agent valid & milik tenant; bila tidak, tulis 404 dan return false.
func resolveAgent(c *gin.Context) (uint, bool) {
	id := currentAgentID(c)
	if id == 0 {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return 0, false
	}
	return id, true
}

// Tidak ada batas jumlah nomor — internal company, tanpa paket langganan.

// quotaMessage = balasan saat kuota AI bulan ini habis (kontak dialihkan ke CS manusia).
const quotaMessage = "Halo kak 🙏 pesan kakak sudah kami terima, CS kami akan segera membalas ya."

// ---- Debounce: gabungkan pesan teks beruntun jadi satu sebelum diproses ----
// Lebih manusiawi (bot tidak membalas tiap baris) & hemat panggilan AI.

const debounceWindow = 5 * time.Second
const manualAIPauseDuration = 10 * time.Minute
const recentContextRuneBudget = 24000

type pendingText struct {
	timer *time.Timer
	texts []string
	ids   []string
	tmpl  services.IncomingMessage // simpan PushName dll. dari pesan pertama
}

var (
	debounceMu   sync.Mutex
	pending      = map[string]*pendingText{}
	summaryMu    sync.Map // key agent|kontak -> *sync.Mutex, cegah ringkasan tumpang tindih
	processMuMap sync.Map // key agent|kontak -> *sync.Mutex, serialisasi balasan AI per kontak
)

// withContactProcessLock menahan processMessage lain untuk kontak yang sama.
// Mencegah double-reply saat AI masih generate dan pesan baru sudah di-flush.
func withContactProcessLock(agentID uint, senderUser string, fn func()) {
	key := fmt.Sprintf("%d|%s", agentID, senderUser)
	val, _ := processMuMap.LoadOrStore(key, &sync.Mutex{})
	mu := val.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	fn()
}

func debounceKey(agentID uint, sender types.JID) string {
	return fmt.Sprintf("%d|%s", agentID, sender.User)
}

// enqueueText menampung pesan teks; bila ada pesan lain dalam jeda singkat, digabung & timer di-reset.
func enqueueText(agentID uint, sender types.JID, in services.IncomingMessage) {
	key := debounceKey(agentID, sender)
	debounceMu.Lock()
	defer debounceMu.Unlock()
	if p := pending[key]; p != nil {
		p.texts = append(p.texts, in.Text)
		p.ids = append(p.ids, in.WAMsgIDs...)
		p.timer.Reset(debounceWindow)
		return
	}
	p := &pendingText{tmpl: in, texts: []string{in.Text}, ids: append([]string(nil), in.WAMsgIDs...)}
	p.timer = time.AfterFunc(debounceWindow, func() { flushText(agentID, sender, false) })
	pending[key] = p
}

// flushText memproses pesan teks tertunda. stopTimer=true saat dipanggil manual (mis. ada media menyusul).
func flushText(agentID uint, sender types.JID, stopTimer bool) {
	key := debounceKey(agentID, sender)
	debounceMu.Lock()
	p := pending[key]
	delete(pending, key)
	debounceMu.Unlock()
	if p == nil {
		return
	}
	if stopTimer {
		p.timer.Stop()
	}
	combined := p.tmpl
	combined.Text = strings.TrimSpace(strings.Join(p.texts, "\n"))
	combined.WAMsgIDs = p.ids
	processMessage(agentID, sender, combined)
}

// OnWAMessage dipanggil saat ada pesan masuk untuk agent tertentu.
func OnWAMessage(agentID uint, sender types.JID, in services.IncomingMessage) {
	num := sender.User

	// Setiap nomor yang mengirim pesan wajib masuk CRM meski WhatsApp tidak
	// menyediakan nama profil. Nama hanya diperbarui bila benar-benar tersedia.
	contactQuery := database.DB.Where(models.Contact{AgentID: agentID, Number: num}).
		Attrs(models.Contact{LeadStage: leadStageNew, LeadStageSource: "system", LeadStageReason: "Kontak masuk dari percakapan WhatsApp"})
	if name := strings.TrimSpace(in.PushName); name != "" {
		contactQuery = contactQuery.Assign(models.Contact{Name: name})
	}
	if err := contactQuery.FirstOrCreate(&models.Contact{}).Error; err != nil {
		log.Printf("Gagal memastikan kontak CRM (agent %d, %s): %v", agentID, num, err)
	}

	// Notifikasi webhook tenant (bila diset) untuk setiap pesan masuk nyata — asinkron,
	// tidak memblokir alur balasan. Lewati pesan protokol/kosong (bukan teks & bukan media).
	if in.MediaType != "" || strings.TrimSpace(in.Text) != "" {
		dispatchIncomingWebhook(agentID, sender, in)
	}

	// Media diproses langsung; flush dulu teks tertunda kontak ini agar urutannya benar.
	if in.MediaType != "" {
		flushText(agentID, sender, true)
		processMessage(agentID, sender, in)
		return
	}
	if strings.TrimSpace(in.Text) == "" {
		return // tipe pesan lain (mis. teks kosong) diabaikan
	}
	// Input menu/alur harus utuh & terpisah — jangan lewat debounce (yang menggabung pesan
	// beruntun jadi "1\n2" sehingga tak cocok ke opsi mana pun). Proses langsung, tapi flush
	// teks tertunda kontak ini dulu supaya urutannya tetap benar.
	if in.ActionID != "" || inCheckoutContext(agentID, sender.User, in.ActionID) || inAIFormContext(agentID, sender.User, in.ActionID) || inFlowContext(agentID, sender.User, in.Text) {
		flushText(agentID, sender, true)
		processMessage(agentID, sender, in)
		return
	}
	if shouldProcessContextualReplyImmediately(agentID, sender.User, in.Text) {
		flushText(agentID, sender, true)
		processMessage(agentID, sender, in)
		return
	}
	enqueueText(agentID, sender, in)
}

// OnWAOwnMessage menangkap balasan manual dari HP atau WhatsApp Web lain.
// Kontak langsung dijeda dari AI dan balasan admin ikut menjadi konteks percakapan.
func OnWAOwnMessage(agentID uint, recipient types.JID, in services.IncomingMessage) {
	num := strings.TrimSpace(recipient.User)
	if num == "" {
		return
	}
	text := strings.TrimSpace(in.Text)
	if text == "" && in.MediaType != "" {
		text = mediaPlaceholder(in.MediaType, in.FileName)
	}
	if text == "" {
		return
	}
	// Pasang jeda sementara sebelum menguras pesan customer yang masih di debounce,
	// sehingga pesan itu tetap tercatat tetapi tidak sempat memicu jawaban AI baru.
	pauseAIForManualReply(agentID, num)
	flushText(agentID, recipient, true)
	now := time.Now()
	if err := database.DB.Create(&models.ChatHistory{
		AgentID: agentID, Sender: num, Reply: text, FromHuman: true,
		MediaType: in.MediaType, FileName: in.FileName, Mimetype: in.Mimetype,
		WAMsgID: in.WAMsgID, DeliveryStatus: "sent", CreatedAt: now,
	}).Error; err != nil {
		log.Printf("Gagal mencatat balasan manual perangkat (agent %d, %s): %v", agentID, num, err)
	}
}

func pauseAIForManualReply(agentID uint, sender string) time.Time {
	until := time.Now().Add(manualAIPauseDuration)
	_ = database.DB.Model(&models.Contact{}).
		Where("agent_id = ? AND number = ?", agentID, sender).
		Update("manual_pause_until", &until).Error
	return until
}

func shouldProcessContextualReplyImmediately(agentID uint, sender, text string) bool {
	if len([]rune(strings.TrimSpace(text))) > 80 || len(strings.Fields(text)) > 6 {
		return false
	}
	var last models.ChatHistory
	if database.DB.Where("agent_id = ? AND sender = ? AND reply <> ''", agentID, sender).Order("id desc").First(&last).Error != nil {
		return false
	}
	return time.Since(last.CreatedAt) <= 2*time.Hour && isShortReplyToAssistantQuestion(text, []models.ChatHistory{last})
}

func isShortReplyToAssistantQuestion(text string, history []models.ChatHistory) bool {
	if strings.TrimSpace(text) == "" || len([]rune(strings.TrimSpace(text))) > 80 || len(strings.Fields(text)) > 6 {
		return false
	}
	for i := len(history) - 1; i >= 0; i-- {
		reply := strings.ToLower(strings.TrimSpace(history[i].Reply))
		if reply == "" {
			continue
		}
		for _, marker := range []string{"?", "apakah ", "mau ", "boleh ", "mana ", "kapan ", "berapa ", "bagaimana ", "gimana ", "apa "} {
			if strings.Contains(reply, marker) {
				return true
			}
		}
		return false
	}
	return false
}

// processMessage menjalankan pipeline balasan (opt-out, handoff, jam kerja, sapaan, keyword, AI).
func processMessage(agentID uint, sender types.JID, in services.IncomingMessage) {
	withContactProcessLock(agentID, sender.User, func() {
		processMessageLocked(agentID, sender, in)
	})
}

func processMessageLocked(agentID uint, sender types.JID, in services.IncomingMessage) {
	num := sender.User

	var agent models.Agent
	prompt := "Kamu adalah asisten AI yang ramah. Jawab dalam bahasa Indonesia."
	tone := "ramah"
	if database.DB.First(&agent, agentID).Error == nil {
		if agent.SystemPrompt != "" {
			prompt = agent.SystemPrompt
		}
		if agent.Tone != "" {
			tone = agent.Tone
		}
	}
	manualPaused := false
	var contact models.Contact
	if database.DB.Select("id", "manual_pause_until").Where("agent_id = ? AND number = ?", agentID, num).First(&contact).Error == nil && contact.ManualPauseUntil != nil {
		if contact.ManualPauseUntil.After(time.Now()) {
			manualPaused = true
		} else {
			// Ketika jeda manual berakhir, rangkum percakapan yang terjadi selama admin
			// menangani chat sebelum AI menjawab pesan baru berikutnya.
			_ = database.DB.Model(&contact).Update("manual_pause_until", nil).Error
			maybeSummarize(agent, num)
		}
	}
	// Langganan tidak aktif — tidak berlaku untuk instalasi internal.
	// Long-term memory: inject ringkasan percakapan lama KONTAK INI saja (per-sender,
	// bukan global agar konteks customer lain tidak bocor ke percakapan ini).
	var mem models.ConversationMemory
	if database.DB.Where("agent_id = ? AND sender = ?", agentID, num).First(&mem).Error == nil && mem.Summary != "" {
		prompt = "PERCAKAPAN SEBELUMNYA DENGAN KONTAK INI: " + mem.Summary + "\n\n" + prompt
	}

	// Simpan media ke disk dulu (kalau ada).
	mediaPath := ""
	if in.MediaType != "" && len(in.Data) > 0 {
		mediaPath = storeMedia(agentID, in.Data, in.Mimetype, in.FileName)
	}
	// Teks tampilan: caption, atau placeholder kalau media tanpa caption.
	displayText := in.Text
	if displayText == "" && in.MediaType != "" {
		displayText = mediaPlaceholder(in.MediaType, in.FileName)
	}
	imageAnalysis := ""
	imageAnalysisStatus := ""
	imageAnalysisModel := ""
	imageAnalysisConfidence := float64(0)
	imageAnalysisAnswer := ""
	imageAnalysisProductID := uint(0)
	imageAnalysisNeedsHuman := false
	// logRow mencatat satu baris percakapan beserta lampiran media (bila ada).
	logRow := func(message, reply string, sendErr error) {
		status, errMsg, nextRetryAt := deliveryFields(sendErr)
		if strings.TrimSpace(reply) == "" {
			status, errMsg, nextRetryAt = "sent", "", nil
		}
		row := models.ChatHistory{
			AgentID: agentID, Sender: num, Message: message, Reply: reply,
			MediaType: in.MediaType, MediaPath: mediaPath, FileName: in.FileName, Mimetype: in.Mimetype,
			ImageAnalysis: imageAnalysis, ImageAnalysisStatus: imageAnalysisStatus,
			ImageAnalysisModel: imageAnalysisModel, ImageAnalysisConfidence: imageAnalysisConfidence,
			ImageAnalysisAnswer: imageAnalysisAnswer, ImageAnalysisProductID: imageAnalysisProductID,
			ImageAnalysisNeedsHuman: imageAnalysisNeedsHuman,
			WAMsgID:                 in.WAMsgID, ReplyTo: in.ReplyTo,
			DeliveryStatus: status, SendError: errMsg, NextRetryAt: nextRetryAt,
		}
		if err := database.DB.Create(&row).Error; err != nil {
			log.Printf("Gagal mencatat ChatHistory (agent %d, %s): %v", agentID, num, err)
		} else if agent.AIEnabled && strings.TrimSpace(message) != "" {
			chatID := row.ID
			services.Go("crm-ai-stage", func() { maybeAssessCRMLeadStage(agentID, num, chatID) })
		}
	}
	readMarked := false
	markBeforeReply := func() {
		if readMarked {
			return
		}
		readMarked = true
		ids := in.WAMsgIDs
		if len(ids) == 0 && in.WAMsgID != "" {
			ids = []string{in.WAMsgID}
		}
		if err := services.WA(agentID).MarkIncomingRead(in.ChatJID, in.SenderJID, ids); err != nil {
			log.Printf("WA agent %d gagal menandai pesan dibaca sebelum membalas: %v", agentID, err)
		}
	}
	send := func(text string) error {
		markBeforeReply()
		err := services.WA(agentID).SendMessage(sender, text)
		if err != nil {
			log.Printf("WA send gagal (agent %d, %s): %v", agentID, num, err)
		}
		return err
	}

	// 0. Permintaan berhenti (opt-out) -> catat agar tidak ikut broadcast lagi, lalu konfirmasi.
	if in.Text != "" && isOptOutKeyword(in.Text) {
		_ = database.DB.Where(models.OptOut{AgentID: agentID, Sender: num}).FirstOrCreate(&models.OptOut{AgentID: agentID, Sender: num}).Error
		now := time.Now()
		_ = database.DB.Model(&models.ContactConsent{}).
			Where("agent_id = ? AND number = ? AND revoked_at IS NULL", agentID, num).
			Update("revoked_at", &now).Error
		ack := "Baik kak 🙏 nomor ini tidak akan kami kirimi pesan promosi lagi. Terima kasih."
		logRow(in.Text, ack, send(ack))
		return
	}

	// 1. Media diproses dengan konteks checkout/form agar foto dapat menjadi jawaban langkah.
	// Lokasi dibiarkan masuk ke pipeline teks/AI karena extractIncoming sudah mengubah
	// koordinat, alamat, dan link Maps menjadi konteks yang bisa langsung digunakan.
	// Jika kontak sudah diambil alih CS, analisis tetap disimpan tetapi bot tidak membalas.
	if in.MediaType != "" && in.MediaType != "location" {
		ack := "Terima kasih kak 🙏 file/medianya sudah saya terima, saya cek dulu ya."
		var existingHandoff models.Handoff
		alreadyHandled := database.DB.Where("agent_id = ? AND sender = ?", agentID, num).First(&existingHandoff).Error == nil
		visionModel := ""
		visionErrText := ""
		visionStart := time.Now()
		visionAttempted := false
		needsHandoff := true
		visionAnswer := ""
		visionProductID := uint(0)
		mediaButtons := []services.ReplyButton{}
		workflow := activeVisionWorkflow(agentID, num)
		// Media bukan input menu statis. Tutup sesi menu lama agar jawaban lanjutan
		// terhadap analisis gambar tidak tersangkut sebagai pilihan menu.
		clearFlowSession(agentID, num)
		if agent.AIEnabled && (in.MediaType == "image" || in.MediaType == "sticker") && len(in.Data) > 0 {
			visionAttempted = true
			if !alreadyHandled {
				markBeforeReply()
				_ = services.WA(agentID).Typing(num, true)
			}
			var visionHistory []models.ChatHistory
			database.DB.Where("agent_id = ? AND sender = ? AND created_at > ?", agentID, num, time.Now().AddDate(0, 0, -7)).
				Order("created_at desc").Limit(12).Find(&visionHistory)
			for i, j := 0, len(visionHistory)-1; i < j; i, j = i+1, j-1 {
				visionHistory[i], visionHistory[j] = visionHistory[j], visionHistory[i]
			}
			result, visionErr := services.AnalyzeCustomerImage(agentID, prompt, tone, in.Text, workflow.Instruction, in.Mimetype, in.Data, visionHistory)
			if visionErr == nil {
				ack = result.Reply
				imageAnalysis = result.Analysis
				imageAnalysisStatus = "completed"
				imageAnalysisModel = result.Model
				imageAnalysisConfidence = result.Confidence
				imageAnalysisAnswer = result.Answer
				imageAnalysisProductID = result.ProductID
				visionModel = result.Model
				visionAnswer = result.Answer
				visionProductID = result.ProductID
				needsHandoff = result.NeedsHuman || result.Confidence < 0.55
				imageAnalysisNeedsHuman = needsHandoff
				workflowHandled := false
				if !alreadyHandled && workflow.Kind == "checkout" && result.Answer != "" {
					if flowResult, active, accepted := handleCheckoutImageAnswer(agentID, num, result.Answer, in.WAMsgID, needsHandoff); active {
						workflowHandled = true
						needsHandoff = flowResult.handoff
						mediaButtons = flowResult.buttons
						ack = flowResult.reply
						if accepted {
							ack = "Foto berhasil dibaca dan dipakai sebagai jawaban.\n\n" + ack
						}
					}
				} else if !alreadyHandled && workflow.Kind == "form" && result.Answer != "" {
					if flowResult, active, accepted := handleAIFormImageAnswer(agentID, num, result.Answer, in.WAMsgID, needsHandoff); active {
						workflowHandled = true
						needsHandoff = flowResult.handoff
						mediaButtons = flowResult.buttons
						ack = flowResult.reply
						if accepted {
							ack = "Foto berhasil dibaca dan dipakai sebagai jawaban.\n\n" + ack
						}
					}
				}
				if !alreadyHandled && workflow.Kind != "" && !workflowHandled {
					needsHandoff = false
					ack = result.Reply + "\n\nSaya belum mendapat jawaban yang sesuai dari foto itu. Silakan kirim foto yang lebih jelas atau jawab dengan teks."
				}
				if !alreadyHandled && workflow.Kind == "" && result.ProductID != 0 {
					markProductLead(agentID, num, "warm")
					mediaButtons = visionProductButtons(agentID, result.ProductID)
				}
				if needsHandoff && !result.NeedsHuman && workflow.Kind == "" {
					ack += " Detail gambarnya belum cukup jelas, jadi saya cek dulu biar tidak salah menilai ya."
				}
			} else {
				imageAnalysis = "Gambar belum dapat dianalisis otomatis. Perlu dicek lebih detail."
				imageAnalysisStatus = "failed"
				imageAnalysisNeedsHuman = true
				visionErrText = "vision: " + visionErr.Error()
				log.Printf("Vision gagal (agent %d, %s): %v", agentID, num, visionErr)
				if workflow.Kind != "" {
					needsHandoff = false
					ack = "Maaf, foto belum dapat dibaca. Silakan kirim ulang foto yang lebih jelas atau jawab pertanyaan aktif dengan teks."
				}
			}
			if !alreadyHandled {
				_ = services.WA(agentID).Typing(num, false)
			}
		}
		if agent.AIEnabled && !alreadyHandled {
			if needsHandoff {
				ensureHandoff(agentID, num, displayText)
				// Persona CS: jangan sebut petugas di balasan vision ke pelanggan.
				ack = stripInternalStaffSpeak(ack)
				if strings.Contains(strings.ToLower(ack), "petugas") || strings.Contains(strings.ToLower(ack), "diteruskan") {
					ack = humanFacingHoldReply
				}
			}
			if len(mediaButtons) > 0 {
				markBeforeReply()
				sendErr := services.WA(agentID).SendButtons(num, ack, "Pilih tindak lanjut", mediaButtons)
				if sendErr != nil {
					sendErr = send(ack)
				}
				logRow(displayText, ack, sendErr)
			} else {
				logRow(displayText, ack, send(ack))
			}
		} else {
			logRow(displayText, "", nil)
		}
		if visionAttempted {
			logAITurn(agentID, num, displayText, ack, visionModel, 0, false, needsHandoff, visionErrText, time.Since(visionStart).Milliseconds(), services.RetrievalTrace{RetrievalMode: "vision"})
			dispatchImageAnalysisWebhook(agentID, sender, in, imageAnalysis, imageAnalysisStatus, imageAnalysisModel, imageAnalysisConfidence, imageAnalysisNeedsHuman, visionAnswer, visionProductID)
		}
		log.Printf("Media (%s) dari %s (agent %d) -> dianalisis jika didukung; handoff=%t", in.MediaType, num, agentID, needsHandoff)
		return
	}

	// 2. Butuh CS (handoff) — soft by default:
	// - CS manusia sudah balas → AI diam (hormati percakapan manusia).
	// - Belum ada balasan CS & < timeout → AI tetap layani sebagai CS yang sama (FAQ aman).
	// - Lewat timeout tanpa CS → hapus handoff, AI full lagi (anti orphan).
	// Ke pelanggan: jangan sebut petugas/diteruskan/bot.
	hoState := resolveHandoffState(agentID, num)
	if hoState.active && !hoState.softAI {
		logRow(displayText, "", nil)
		return
	}
	if manualPaused {
		logRow(displayText, "", nil)
		return
	}
	// Soft handoff + topik sensitif: tahan keputusan, tetap persona CS manusia.
	if hoState.softAI && isSoftHandoffSensitive(in.Text) {
		hold := humanFacingSoftSensitiveReply
		logRow(displayText, hold, send(hold))
		ensureHandoff(agentID, num, displayText)
		return
	}

	// 2a. Tombol produk dan sesi checkout diproses deterministik sebelum Alur
	// Otomatis, Auto-Reply, dan AI. AI hanya dipakai bila aksi tombol memang "ai".
	productResult := handleProductInteraction(agentID, num, in.Text, in.ActionID)
	productAIContext := productResult.aiContext
	if productResult.handled {
		if productResult.handoff {
			ensureHandoff(agentID, num, displayText)
			productResult.reply = stripInternalStaffSpeak(productResult.reply)
		}
		var productSendErr error
		if len(productResult.buttons) > 0 {
			markBeforeReply()
			productSendErr = services.WA(agentID).SendButtons(num, productResult.reply, "Pilih salah satu", productResult.buttons)
			if productSendErr != nil {
				// Pesan teks tetap memastikan checkout bisa dilanjutkan saat tombol
				// interaktif ditolak oleh versi WhatsApp penerima.
				productSendErr = send(productResult.reply)
			}
		} else if strings.TrimSpace(productResult.reply) != "" {
			productSendErr = send(productResult.reply)
		}
		logRow(displayText, productResult.reply, productSendErr)
		return
	}

	// 2b. Form AI adalah alur pengumpulan data non-produk. Diproses sebelum menu
	// otomatis agar booking/daftar/konsultasi bisa berjalan step-by-step.
	formResult := aiFormRuntimeResult{}
	if !strings.HasPrefix(in.ActionID, "flow:") && (inAIFormContext(agentID, num, in.ActionID) || agent.AIEnabled) {
		formResult = handleAIFormMessage(agentID, num, in.Text, in.ActionID)
	}
	if productAIContext == "" && formResult.handled {
		if formResult.handoff {
			ensureHandoff(agentID, num, displayText)
			formResult.reply = stripInternalStaffSpeak(formResult.reply)
		}
		var formSendErr error
		if len(formResult.buttons) > 0 {
			markBeforeReply()
			formSendErr = services.WA(agentID).SendButtons(num, formResult.reply, "Pilih salah satu", formResult.buttons)
			if formSendErr != nil {
				formSendErr = send(formResult.reply)
			}
		} else if strings.TrimSpace(formResult.reply) != "" {
			formSendErr = send(formResult.reply)
		}
		logRow(displayText, formResult.reply, formSendErr)
		return
	}

	// 2c. Alur/menu otomatis diproses sebelum jam kerja, auto-reply, dan AI. Menu
	// deterministik tetap dapat melayani pelanggan meskipun AI sedang nonaktif.
	if result := handleFlowMessage(agentID, num, in.Text, in.ActionID); productAIContext == "" && result.handled {
		if result.handoff {
			ensureHandoff(agentID, num, displayText)
			result.reply = stripInternalStaffSpeak(result.reply)
			if strings.TrimSpace(result.reply) == "" {
				result.reply = humanFacingHoldReply
			}
			log.Printf("Alur menandai Butuh CS internal (agent %d, %s)", agentID, num)
		}
		if strings.TrimSpace(result.reply) != "" {
			markBeforeReply()
			var flowSendErr error
			if len(result.buttons) > 0 {
				flowSendErr = services.WA(agentID).SendButtonsWithDelay(num, result.reply, "Pilih salah satu", result.buttons, result.delayMin, result.delayMax)
				if flowSendErr != nil {
					flowSendErr = services.WA(agentID).SendMessageWithDelay(sender, result.fallback, 0, 0)
					result.reply = result.fallback
				}
			} else {
				flowSendErr = services.WA(agentID).SendMessageWithDelay(sender, result.reply, result.delayMin, result.delayMax)
			}
			if flowSendErr != nil {
				log.Printf("WA alur gagal mengirim (agent %d, %s): %v", agentID, num, flowSendErr)
			}
			logRow(displayText, result.reply, flowSendErr)
		} else {
			logRow(displayText, "", nil)
		}
		return
	}

	// 3. Di luar jam kerja -> kirim pesan away (sekali), jangan panggil AI.
	if !withinBusinessHours(agent) {
		away := agent.AwayMessage
		if away == "" {
			away = "Mohon maaf, saat ini di luar jam operasional. Pesan kakak sudah kami terima dan akan kami balas pada jam kerja ya 🙏"
		}
		var last models.ChatHistory
		database.DB.Where("agent_id = ? AND sender = ?", agentID, num).Order("created_at desc").First(&last)
		if last.Reply != away {
			logRow(displayText, away, send(away))
		} else {
			logRow(displayText, "", nil)
		}
		return
	}

	// 4. Sapaan untuk kontak baru.
	// Pure greeting: kirim template saja (hindari double greeting + balasan AI).
	// Pesan pertama berisi intent + AI on: lewati template; AI menjawab langsung.
	// AI off: tetap kirim welcome template (tanpa log terpisah) lalu lanjut auto-reply/inbox.
	if agent.GreetingEnabled && agent.GreetingMessage != "" && isNewContact(agentID, num) {
		if isGenericGreetingMessage(in.Text) {
			logRow(displayText, agent.GreetingMessage, send(agent.GreetingMessage))
			return
		}
		if !agent.AIEnabled {
			if err := send(agent.GreetingMessage); err != nil {
				log.Printf("Gagal kirim greeting (agent %d, %s): %v", agentID, num, err)
			}
		}
	}

	// 4b. Auto-reply kata kunci (instan, tanpa AI) -> dicek sebelum AI agar cepat & hemat biaya.
	if reply, matched := matchAutoReply(agentID, in.Text); productAIContext == "" && matched {
		logRow(displayText, reply, send(reply))
		return
	}

	// 4c. Balasan AI dimatikan -> bot tidak menjawab, pesan dicatat ke inbox untuk dibalas manual.
	if !agent.AIEnabled {
		logRow(displayText, "", nil)
		return
	}

	// 6. Jawaban AI teks biasa.
	var historyNewestFirst []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ?", agentID, num).
		Order("created_at desc").Find(&historyNewestFirst)
	history := historyWithinContextBudget(historyNewestFirst, recentContextRuneBudget)

	// Inject konteks ongkir realtime kalau user tanya ongkir.
	// Perkaya link Maps / URL: resolve short link, parse koordinat/nama, judul halaman.
	// Teks asli tetap di log (displayText); model menerima versi ter-enrich.
	turnStart := time.Now()
	aiUserMsg := services.EnrichUserMessageForAI(in.Text)
	if aiUserMsg != in.Text {
		log.Printf("Link enrich (agent %d, %s): pesan diperkaya konteks link/lokasi", agentID, num)
	}
	enhancedPrompt := prompt
	if hoState.prompt != "" {
		enhancedPrompt += "\n\n" + hoState.prompt
	}
	if productAIContext != "" {
		enhancedPrompt += productAIContext
	}
	routingText := aiUserMsg
	for i := len(history) - 1; i >= 0 && i >= len(history)-4; i-- {
		routingText += "\n" + history[i].Message + "\n" + history[i].Reply
	}
	if !isGenericGreetingMessage(in.Text) {
		if productRouting := productCheckoutRoutingPrompt(agentID, num, routingText); productRouting != "" {
			enhancedPrompt += "\n\n" + productRouting
		}
		if formRouting := aiFormRoutingPrompt(agentID, num); formRouting != "" {
			enhancedPrompt += "\n\n" + formRouting
		}
	}
	// Shipping: pakai teks ter-enrich agar koordinat/nama lokasi dari Maps ikut terdeteksi bila relevan.
	shippingCtx := maybeBuildShippingContext(agent, aiUserMsg, history)
	usedShippingTool := strings.Contains(shippingCtx, "ONGKIR_")
	turnError := shippingTurnError(shippingCtx)
	if shippingCtx != "" {
		enhancedPrompt += "\n\n" + shippingCtx
	}

	chatResult, err := services.ChatWithKnowledge(agentID, enhancedPrompt, tone, aiUserMsg, history)
	reply := chatResult.Reply
	escalate := chatResult.Escalate
	modelName := chatResult.Model
	knowledgeCount := chatResult.Trace.KnowledgeUsedCount
	trace := chatResult.Trace
	if err != nil {
		log.Printf("AI error (agent %d) dari %s: %v", agentID, num, err)
		reply = "Maaf, ada kendala teknis."
		escalate = false
		turnError = "ai: " + err.Error()
	}
	reply, escalate, turnError = applyEscalationPolicy(agentID, enhancedPrompt, tone, in.Text, history, reply, escalate, turnError)
	if escalate {
		// Internal: antrian Butuh CS. Eksternal: tetap persona CS yang sama (cek dulu).
		reply = humanFacingHoldReply
		ensureHandoff(agentID, num, displayText)
		log.Printf("Eskalasi internal / Butuh CS (agent %d) dari %s: %q", agentID, num, in.Text)
	}
	// Soft handoff: jangan create handoff ganda tiap turn; sudah di soft mode.
	// Pagar copy: hilangkan sebutan petugas/CS lain di balasan ke pelanggan.
	reply = stripInternalStaffSpeak(reply)
	if productStart, ok := handleProductCheckoutDirective(agentID, num, reply); ok {
		var checkoutSendErr error
		if len(productStart.buttons) > 0 {
			markBeforeReply()
			checkoutSendErr = services.WA(agentID).SendButtons(num, productStart.reply, "Isi checkout", productStart.buttons)
			if checkoutSendErr != nil {
				checkoutSendErr = send(productStart.reply)
			}
		} else {
			checkoutSendErr = send(productStart.reply)
		}
		latencyMs := time.Since(turnStart).Milliseconds()
		logRow(displayText, productStart.reply, checkoutSendErr)
		logAITurn(agentID, num, displayText, productStart.reply, modelName, knowledgeCount, usedShippingTool, false, turnError, latencyMs, trace)
		return
	}
	if formStart, ok := handleAIFormDirective(agentID, num, reply); ok {
		var formSendErr error
		if len(formStart.buttons) > 0 {
			markBeforeReply()
			formSendErr = services.WA(agentID).SendButtons(num, formStart.reply, "Isi data", formStart.buttons)
			if formSendErr != nil {
				formSendErr = send(formStart.reply)
			}
		} else {
			formSendErr = send(formStart.reply)
		}
		latencyMs := time.Since(turnStart).Milliseconds()
		logRow(displayText, formStart.reply, formSendErr)
		logAITurn(agentID, num, displayText, formStart.reply, modelName, knowledgeCount, usedShippingTool, false, turnError, latencyMs, trace)
		return
	}
	if productStart, ok := startProductFromFreeCollection(agentID, num, in.Text, routingText, reply); ok {
		var checkoutSendErr error
		if len(productStart.buttons) > 0 {
			markBeforeReply()
			checkoutSendErr = services.WA(agentID).SendButtons(num, productStart.reply, "Isi checkout", productStart.buttons)
			if checkoutSendErr != nil {
				checkoutSendErr = send(productStart.reply)
			}
		} else {
			checkoutSendErr = send(productStart.reply)
		}
		latencyMs := time.Since(turnStart).Milliseconds()
		logRow(displayText, productStart.reply, checkoutSendErr)
		logAITurn(agentID, num, displayText, productStart.reply, modelName, knowledgeCount, usedShippingTool, false, turnError, latencyMs, trace)
		return
	}
	if formStart, ok := startAIFormFromFreeCollection(agentID, num, in.Text, routingText, reply); ok {
		var formSendErr error
		if len(formStart.buttons) > 0 {
			markBeforeReply()
			formSendErr = services.WA(agentID).SendButtons(num, formStart.reply, "Isi data", formStart.buttons)
			if formSendErr != nil {
				formSendErr = send(formStart.reply)
			}
		} else {
			formSendErr = send(formStart.reply)
		}
		latencyMs := time.Since(turnStart).Milliseconds()
		logRow(displayText, formStart.reply, formSendErr)
		logAITurn(agentID, num, displayText, formStart.reply, modelName, knowledgeCount, usedShippingTool, false, turnError, latencyMs, trace)
		return
	}
	latencyMs := time.Since(turnStart).Milliseconds()
	// Cek directive [[SEND_MEDIA:id]] — AI mau kirim media otomatis
	if mediaReply, handled := handleSendMediaDirective(agentID, num, reply); handled {
		markBeforeReply()
		sendErr := sendChunked(agentID, sender, mediaReply.text, agent.AIReplyDelayMin, agent.AIReplyDelayMax, nil)
		logRow(displayText, mediaReply.text, sendErr)
		logAITurn(agentID, num, displayText, mediaReply.text, modelName, knowledgeCount, usedShippingTool, false, turnError, latencyMs, trace)
		return
	}
	reply = services.LinkifyWhatsApp(reply, agent.Number) // nomor WA jadi tautan klik (kecuali nomor sendiri)
	markBeforeReply()
	sendErr := sendChunked(agentID, sender, reply, agent.AIReplyDelayMin, agent.AIReplyDelayMax, func() bool {
		var count int64
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ? AND from_human = ? AND created_at >= ?", agentID, num, true, turnStart).
			Count(&count)
		if count > 0 {
			return false
		}
		var activePause int64
		database.DB.Model(&models.Contact{}).
			Where("agent_id = ? AND number = ? AND manual_pause_until > ?", agentID, num, time.Now()).
			Count(&activePause)
		return activePause == 0
	}) // balasan panjang dipecah jadi beberapa bubble (lebih manusiawi)
	if sendErr != nil {
		log.Printf("WA send chunked gagal (agent %d, %s): %v", agentID, num, sendErr)
	}
	logRow(displayText, reply, sendErr)
	logAITurn(agentID, num, displayText, reply, modelName, knowledgeCount, usedShippingTool, escalate, turnError, latencyMs, trace)

	// Long-term memory: auto-summary setelah percakapan (jeda >30 menit).
	services.Go("maybeSummarize", func() { maybeSummarize(agent, num) })

}

// historyWithinContextBudget memilih sebanyak mungkin percakapan terbaru berdasarkan
// kapasitas teks, bukan jumlah pesan tetap. Input dari DB berurutan terbaru -> lama,
// output dikembalikan kronologis agar role user/asisten tetap tepat untuk model.
func historyWithinContextBudget(newestFirst []models.ChatHistory, maxRunes int) []models.ChatHistory {
	if maxRunes <= 0 || len(newestFirst) == 0 {
		return nil
	}
	selected := make([]models.ChatHistory, 0, len(newestFirst))
	used := 0
	for _, row := range newestFirst {
		rowRunes := len([]rune(row.Message)) + len([]rune(row.Reply))
		if len(selected) > 0 && used+rowRunes > maxRunes {
			break
		}
		selected = append(selected, row)
		used += rowRunes
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return selected
}

// applyEscalationPolicy menyelaraskan penanganan [[ESCALATE]] antara production dan simulator:
// follow-up kontekstual serta pertanyaan tanpa sinyal manusia/risiko tidak langsung ke CS.
func applyEscalationPolicy(agentID uint, enhancedPrompt, tone, userMsg string, history []models.ChatHistory, reply string, escalate bool, turnError string) (string, bool, string) {
	if !escalate {
		return reply, false, turnError
	}
	if isShortReplyToAssistantQuestion(userMsg, history) {
		ctxReply, ctxErr := services.ResolveContextualFollowUp(agentID, enhancedPrompt, tone, userMsg, history)
		if ctxErr == nil && strings.TrimSpace(ctxReply) != "" {
			reply = ctxReply
		} else {
			reply = "Baik kak, saya pahami jawabannya. Kita lanjut dari informasi itu ya."
		}
		if turnError != "" {
			turnError += "; "
		}
		turnError += "escalation suppressed: contextual follow-up"
		return reply, false, turnError
	}
	if !shouldAllowHumanHandoff(userMsg) {
		ctxReply, ctxErr := services.ContextualFallback(agentID, enhancedPrompt, tone, userMsg, history)
		if ctxErr == nil && strings.TrimSpace(ctxReply) != "" {
			reply = ctxReply
		} else {
			reply = "Untuk detail itu belum bisa saya pastikan ya kak, jadi saya tidak ingin memberikan informasi yang keliru."
		}
		if turnError != "" {
			turnError += "; "
		}
		turnError += "escalation suppressed: no human intent or high-risk signal"
		return reply, false, turnError
	}
	return reply, true, turnError
}

func shouldAllowHumanHandoff(message string) bool {
	lower := strings.ToLower(message)
	human := containsAnyText(lower, "cs", "admin", "petugas", "operator", "manusia", "customer service", "live agent", "orang")
	request := containsAnyText(lower, "hubungkan", "sambungkan", "teruskan", "bicara", "ngobrol", "ngomong", "hubungi", "panggil", "alihkan") ||
		strings.Contains(lower, "minta cs") || strings.Contains(lower, "minta admin") ||
		strings.Contains(lower, "minta petugas") || strings.Contains(lower, "minta customer service") ||
		strings.Contains(lower, "ke customer service") || strings.Contains(lower, "sama orang")
	if human && request {
		return true
	}
	return containsAnyText(lower,
		"refund", "pengembalian dana", "salah transfer", "bukti pembayaran",
		"penipuan", "komplain", "keluhan serius", "data pribadi bocor", "akun diblokir")
}

func containsAnyText(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

// sendChunked mengirim balasan AI dalam 1-3 bubble (per paragraf), masing-masing dengan
// jeda "mengetik" alami dari SendMessage — terasa seperti manusia, bukan satu dinding teks.
func sendChunked(agentID uint, to types.JID, text string, delayMin, delayMax int, guard func() bool) error {
	for index, part := range splitReply(text) {
		var err error
		if index == 0 {
			err = services.WA(agentID).SendMessageWithDelayGuarded(to, part, delayMin, delayMax, guard)
		} else {
			if guard != nil && !guard() {
				return nil
			}
			err = services.WA(agentID).SendMessage(to, part)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// splitReply memecah teks per paragraf (baris kosong), maksimal 3 bubble; sisanya digabung ke bubble terakhir.
func splitReply(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var parts []string
	for _, p := range strings.Split(text, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return []string{text}
	}
	if len(parts) > 3 {
		parts = append(parts[:2], strings.Join(parts[2:], "\n\n"))
	}
	return parts
}

// withinBusinessHours true bila jam kerja nonaktif, atau waktu sekarang berada dalam rentang jam kerja.
func withinBusinessHours(a models.Agent) bool {
	if !a.BusinessHoursEnabled || a.BusinessStart == "" || a.BusinessEnd == "" {
		return true
	}
	cur := time.Now().Format("15:04")
	if a.BusinessStart <= a.BusinessEnd {
		return cur >= a.BusinessStart && cur <= a.BusinessEnd
	}
	return cur >= a.BusinessStart || cur <= a.BusinessEnd // rentang melewati tengah malam
}

func isNewContact(agentID uint, num string) bool {
	var n int64
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND sender = ?", agentID, num).Count(&n)
	return n == 0
}

// storeMedia menyimpan byte media ke disk dan mengembalikan path-nya (kosong bila gagal).
func storeMedia(agentID uint, data []byte, mimetype, fileName string) string {
	dir := fmt.Sprintf("data/media/agent-%d", agentID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("gagal buat folder media: %v", err)
		return ""
	}
	full := filepath.Join(dir, fmt.Sprintf("%d%s", time.Now().UnixNano(), mediaExt(mimetype, fileName)))
	if err := os.WriteFile(full, data, 0o600); err != nil {
		log.Printf("gagal simpan media: %v", err)
		return ""
	}
	return full
}

func mediaExt(mimetype, fileName string) string {
	if fileName != "" {
		if e := filepath.Ext(fileName); e != "" {
			return e
		}
	}
	mt := mimetype
	if i := strings.IndexByte(mt, ';'); i >= 0 {
		mt = mt[:i]
	}
	if exts, _ := mime.ExtensionsByType(strings.TrimSpace(mt)); len(exts) > 0 {
		return exts[0]
	}
	return ".bin"
}

func mediaPlaceholder(mediaType, fileName string) string {
	switch mediaType {
	case "image":
		return "📷 Foto"
	case "video":
		return "🎥 Video"
	case "audio":
		return "🎤 Pesan suara"
	case "sticker":
		return "🌟 Stiker"
	case "document":
		if fileName != "" {
			return "📎 " + fileName
		}
		return "📎 Dokumen"
	case "location":
		return "📍 Lokasi"
	}
	return ""
}

func logTurn(agentID uint, num, msg, reply string, fromHuman bool, replyTo string, replyText string) {
	if err := database.DB.Create(&models.ChatHistory{
		AgentID: agentID, Sender: num, Message: msg, Reply: reply, FromHuman: fromHuman,
		ReplyTo: replyTo, ReplyText: replyText,
	}).Error; err != nil {
		log.Printf("Gagal logTurn (agent %d, %s): %v", agentID, num, err)
	}
}

// --- Cek Ongkir Realtime via RajaOngkir ---
// --- Cek Ongkir Realtime via Mengantar API ---

var shippingKeywords = []string{"ongkir", "ongkos kirim", "biaya kirim", "kirim ke", "pengiriman ke", "berapa kirim", "cek ongkir", "ongkos"}

func detectShippingIntent(msg string) bool {
	lower := strings.ToLower(msg)
	for _, kw := range shippingKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func extractDestinationCity(msg string) string {
	msg = strings.ToLower(msg)
	patterns := []string{"ke ", "tujuan ", "ongkir ", "kirim "}
	stopWords := map[string]bool{
		"berapa": true, "kak": true, "ya": true, "dong": true, "sih": true, "nih": true,
		"brp": true, "gan": true, "min": true, "bro": true, "bang": true, "mas": true,
		"mbak": true, "mba": true, "om": true, "bos": true, "koh": true, "deh": true,
		"yah": true, "weh": true, "lur": true, "boss": true, "kuy": true, "guy": true,
		"brapa": true, "berape": true, "kaka": true, "abang": true, "kanda": true,
		"yaa": true, "sihh": true, "dehh": true, "ap": true, "berap": true,
		"berapa?": true, "brp?": true, "dong?": true, "ya?": true, "kak?": true,
		"untuk": true, "produk": true, "barang": true, "paket": true, "aja": true,
	}
	for _, p := range patterns {
		if idx := strings.Index(msg, p); idx >= 0 {
			rest := msg[idx+len(p):]
			rawWords := strings.Fields(rest)
			var words []string
			for _, w := range rawWords {
				if w = cleanShippingWord(w); w != "" {
					words = append(words, w)
				}
			}
			if len(words) > 0 {
				start := 0
				if words[0] == "ke" || words[0] == "di" {
					start = 1
				}
				for start < len(words) && stopWords[words[start]] {
					start++
				}
				if start >= len(words) {
					return ""
				}
				candidate := words[start]
				if start+1 < len(words) && !stopWords[words[start+1]] {
					candidate = words[start] + " " + words[start+1]
				}
				return strings.TrimSpace(candidate)
			}
		}
	}
	return ""
}

func cleanShippingWord(w string) string {
	return strings.Trim(strings.ToLower(w), " 	\n\r.,?!:;\"'()[]{}")
}

// maybeBuildShippingContext membangun konteks ongkir realtime untuk system prompt AI.
// Menggunakan Mengantar API untuk search kota + cek ongkir (JNE & JT).
func maybeBuildShippingContext(agent models.Agent, msg string, history []models.ChatHistory) string {
	// Cek apakah agent punya origin address configured (via PICKUP_AUTOFILL)
	originAutofillID := strings.TrimSpace(agent.MengantarOriginAutofillID)
	if originAutofillID == "" {
		// Fallback: coba ambil dari alamat pertama di Mengantar
		addrs, err := services.GetMyAddresses()
		if err == nil && len(addrs) > 0 {
			originAutofillID = addrs[0].PickupAutofill
		}
	}
	if originAutofillID == "" {
		return ""
	}

	hasIntent := detectShippingIntent(msg)
	destText := ""
	if hasIntent {
		destText = extractDestinationCity(msg)
	} else {
		if !lastReplyAskedShippingFollowup(history) {
			return ""
		}
		lower := strings.ToLower(msg)
		for _, qw := range []string{"kenapa", "kok", "gimana", "bagaimana", "apa ", "apakah", "lama", "banget", "resp", "respon"} {
			if strings.Contains(lower, qw) {
				return ""
			}
		}
		cleaned := strings.TrimSpace(msg)
		for _, suffix := range []string{" kak", " gan", " min", " bro", " bang", " mas", " mbak", " mba", " ya", " dong"} {
			cleaned = strings.TrimSuffix(cleaned, suffix)
			cleaned = strings.TrimSpace(cleaned)
		}
		if len(cleaned) >= 3 {
			destText = cleaned
		}
	}
	if destText == "" {
		if hasIntent {
			return "\n\nONGKIR_NEED_DESTINATION: Customer tanya ongkir tapi belum menyebut kota/kabupaten tujuan. JANGAN eskalasi. Tanya singkat: \"Boleh info kota/kabupaten tujuannya, kak?\""
		}
		return ""
	}

	// Cari kota via Mengantar
	addresses, err := services.SearchAddress(destText)
	if err != nil || len(addresses) == 0 {
		if hasIntent {
			return "\n\nONGKIR_NOTFOUND: Kota \"" + destText + "\" tidak ditemukan. JANGAN eskalasi. Bilang ke customer: \"Maaf kak, kota \"" + destText + "\" belum tersedia di sistem kami. Boleh sebutkan kota/kabupaten yang lebih spesifik ya.\""
		}
		return ""
	}

	if len(addresses) > 1 {
		// Ambiguous
		var sb strings.Builder
		sb.WriteString("\n\nONGKIR_AMBIGUOUS:\nBeberapa kota ditemukan:\n")
		for i, a := range addresses {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("%d. %s, %s, %s (%s)\n", i+1, a.SubdistrictName, a.DistrictName, a.CityName, a.ProvinceName))
		}
		sb.WriteString("Tanyakan customer pilih yang mana (balas dengan nomor).\n")
		return sb.String()
	}

	addr := addresses[0]
	destID := addr.ID

	weight := agent.DefaultWeightGram
	if weight <= 0 {
		weight = 1000
	}
	weightKg := float64(weight) / 1000.0
	if weightKg < 0.1 {
		weightKg = 0.1
	}

	// Cek ongkir JNE & JT
	couriers := []string{"JNE", "JT"}
	var sb strings.Builder
	sb.WriteString("\n\nONGKIR_REALTIME:\n")
	sb.WriteString(fmt.Sprintf("Kota asal ID: %s\n", originAutofillID))
	sb.WriteString(fmt.Sprintf("Tujuan: %s, %s, %s, %s (%s)\n", addr.SubdistrictName, addr.DistrictName, addr.CityName, addr.ProvinceName, addr.ZipCode))
	sb.WriteString(fmt.Sprintf("Berat: %dg (%.1f kg)\n", weight, weightKg))

	hasResults := false
	for _, courier := range couriers {
		est, err := services.EstimateShipping(originAutofillID, destID, courier, weightKg, 0)
		if err != nil || est.Unsupported {
			continue
		}
		hasResults = true
		price := est.EstimatedSpecialPrice
		if price <= 0 {
			price = est.EstimatedPrice
		}
		eta := est.EstimatedDate
		if eta == "" {
			eta = est.EstimateDelivery
		}
		discountNote := ""
		if est.Discount > 0 {
			discountNote = fmt.Sprintf(" (diskon Rp%s, normal Rp%s)", formatRupiah(est.Discount), formatRupiah(est.EstimatedPrice))
		}
		codNote := ""
		if est.UnsupportedCod {
			codNote = " [COD TIDAK TERSEDIA]"
		}
		sb.WriteString(fmt.Sprintf("%s: Rp%s%s estimasi %s%s\n", courier, formatRupiah(price), discountNote, eta, codNote))
	}

	if !hasResults {
		// Coba public endpoint
		allEst, err := services.EstimateAllPublic(originAutofillID, destID, weightKg, 0)
		if err == nil {
			for courier, est := range allEst {
				if courier != "JNE" && courier != "JT" {
					continue
				}
				if est.Unsupported {
					continue
				}
				hasResults = true
				price := est.EstimatedSpecialPrice
				if price <= 0 {
					price = est.EstimatedPrice
				}
				sb.WriteString(fmt.Sprintf("%s: Rp%s estimasi %s\n", courier, formatRupiah(price), est.EstimatedDate))
			}
		}
	}

	if !hasResults {
		return "\n\nONGKIR_EMPTY: Tidak ada tarif tersedia untuk tujuan ini via JNE/JT. JANGAN eskalasi. Bilang ke customer: \"Maaf kak, ongkir ke " + addr.CityName + " belum tersedia. Boleh kirim detail pesanan + alamat lengkap, nanti kami bantu cek manual ya.\""
	}

	sb.WriteString("\nAturan: data ONGKIR_REALTIME ini adalah sumber resmi untuk menjawab pertanyaan ongkir. Jawab langsung dengan daftar tarif di atas, jangan mengarang ekspedisi atau harga lain, jangan eskalasi, dan sebutkan bahwa tarif adalah estimasi dan bisa berubah. Jika customer tanya ongkir COD, sebutkan juga status ketersediaan COD.")
	return sb.String()
}

func lastReplyAskedShippingFollowup(history []models.ChatHistory) bool {
	for i := len(history) - 1; i >= 0 && i >= len(history)-3; i-- {
		reply := strings.ToLower(history[i].Reply)
		if reply == "" {
			continue
		}
		if strings.Contains(reply, "ongkir") && (strings.Contains(reply, "kota") || strings.Contains(reply, "tujuan") || strings.Contains(reply, "alamat")) {
			return true
		}
		if strings.Contains(reply, "kota/kabupaten") || strings.Contains(reply, "pilih yang mana") || strings.Contains(reply, "sebutkan kota") {
			return true
		}
	}
	return false
}

func normalizeCouriers(raw string) []string {
	allowed := map[string]bool{"jne": true, "jnt": true, "sicepat": true, "pos": true, "tiki": true, "anteraja": true, "wahana": true}
	seen := map[string]bool{}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		code := strings.ToLower(strings.TrimSpace(p))
		code = strings.ReplaceAll(code, "&", "n")
		code = strings.ReplaceAll(code, " ", "")
		if code == "j&t" || code == "jntcargo" {
			code = "jnt"
		}
		if allowed[code] && !seen[code] {
			out = append(out, code)
			seen[code] = true
		}
	}
	return out
}

func formatRupiah(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return "Rp" + s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	if s != "" {
		parts = append([]string{s}, parts...)
	}
	return "Rp" + strings.Join(parts, ".")
}

func shippingTurnError(ctx string) string {
	switch {
	case strings.Contains(ctx, "ONGKIR_ERROR"):
		return "shipping: error"
	case strings.Contains(ctx, "ONGKIR_EMPTY"):
		return "shipping: empty"
	case strings.Contains(ctx, "ONGKIR_NOTFOUND"):
		return "shipping: not_found"
	default:
		return ""
	}
}

// ListHandoffs: daftar kontak yang sedang butuh ditangani manusia (bot pause).
func ListHandoffs(c *gin.Context) {
	var hs []models.Handoff
	database.DB.Where("agent_id = ?", currentAgentID(c)).Order("created_at desc").Find(&hs)
	c.JSON(200, gin.H{"data": hs})
}

// ResumeHandoff: hapus handoff -> bot lanjut auto-reply ke kontak itu lagi.
func ResumeHandoff(c *gin.Context) {
	agentID := currentAgentID(c)
	database.DB.Where("agent_id = ? AND sender = ?", agentID, c.Param("sender")).Delete(&models.Handoff{})
	database.DB.Model(&models.Contact{}).Where("agent_id = ? AND number = ?", agentID, c.Param("sender")).Update("manual_pause_until", nil)
	c.JSON(200, gin.H{"message": "resumed"})
}

// OnDeviceLinked menyimpan device JID & nomor saat agent berhasil login via QR.
func OnDeviceLinked(agentID uint, jid, number string) {
	var a models.Agent
	if database.DB.First(&a, agentID).Error != nil {
		return
	}
	a.DeviceJID = jid
	a.Number = number
	if err := database.DB.Save(&a).Error; err != nil {
		log.Printf("Gagal menyimpan device agent %d: %v", agentID, err)
		return
	}
	log.Printf("Agent %d ter-link ke nomor %s", agentID, number)
}

// StartAgents menyambungkan ulang semua agent yang sudah punya device saat startup.
func StartAgents() {
	var agents []models.Agent
	if err := database.DB.Find(&agents).Error; err != nil {
		log.Printf("Gagal mengambil agent saat startup: %v", err)
		return
	}
	for i := range agents {
		a := agents[i]
		// Sesi WA selalu diizinkan — instalasi internal tanpa batas langganan.
		// Migrasi single-number lama: agent default (id 1) adopsi device yang sudah ter-link.
		if a.ID == 1 && a.DeviceJID == "" {
			if jid := services.FirstDeviceJID(); jid != "" {
				a.DeviceJID = jid
				if idx := strings.IndexAny(jid, ":@"); idx >= 0 {
					a.Number = jid[:idx]
				}
				if err := database.DB.Save(&a).Error; err != nil {
					log.Printf("Gagal migrasi device agent %d: %v", a.ID, err)
				}
			}
		}
		if a.DeviceJID != "" {
			go func(ag models.Agent) {
				defer services.RecoverGo("agentReconnect")
				status, err := services.WA(ag.ID).Connect(ag.DeviceJID)
				if err != nil {
					log.Printf("Agent %d gagal connect: %v", ag.ID, err)
					return
				}
				// Lengkapi cache nomor kalau belum ada.
				if status == "connected" && ag.Number == "" {
					if num, _ := services.WA(ag.ID).GetInfo(); num != "" {
						ag.Number = num
						if err := database.DB.Save(&ag).Error; err != nil {
							log.Printf("Gagal menyimpan nomor agent %d: %v", ag.ID, err)
						}
					}
				}
			}(a)
		}
	}
}

// ---- Agent CRUD ----

func ListAgents(c *gin.Context) {
	var agents []models.Agent
	database.DB.Where("tenant_id = ?", currentTenantID(c)).Order("id asc").Find(&agents)
	c.JSON(200, gin.H{"data": agents})
}

// AgentStatuses mengembalikan status koneksi live tiap agent: { "1": "connected", ... }.
// Dipakai dashboard untuk titik indikator hijau/kuning/merah tanpa menimpa form.
func AgentStatuses(c *gin.Context) {
	var agents []models.Agent
	database.DB.Where("tenant_id = ?", currentTenantID(c)).Order("id asc").Find(&agents)
	out := map[uint]string{}
	for _, a := range agents {
		out[a.ID] = services.WA(a.ID).GetStatus()
	}
	c.JSON(200, gin.H{"data": out})
}

func CreateAgent(c *gin.Context) {
	tid := currentTenantID(c)
	// Tidak ada batas jumlah nomor — internal company.
	var req struct {
		Name         string `json:"name"`
		SystemPrompt string `json:"system_prompt"`
		Tone         string `json:"tone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(400, gin.H{"error": "Nama CS wajib diisi"})
		return
	}
	if req.Tone == "" {
		req.Tone = "ramah"
	}
	// Balasan AI sengaja default OFF untuk nomor baru — user wajib setup (knowledge/persona)
	// dulu lalu mengaktifkannya manual. (Tanpa tag default DB, false ikut ter-insert eksplisit.)
	a := models.Agent{TenantID: tid, Name: strings.TrimSpace(req.Name), SystemPrompt: req.SystemPrompt, Tone: req.Tone, AIEnabled: false}
	if err := database.DB.Create(&a).Error; err != nil {
		log.Printf("Gagal membuat agent tenant %d: %v", tid, err)
		c.JSON(500, gin.H{"error": "Gagal membuat agent"})
		return
	}
	c.JSON(201, gin.H{"data": a})
}

func UpdateAgent(c *gin.Context) {
	var a models.Agent
	if database.DB.Where("tenant_id = ?", currentTenantID(c)).First(&a, c.Param("id")).Error != nil {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return
	}
	var req struct {
		Name                 string  `json:"name"`
		SystemPrompt         *string `json:"system_prompt"`
		Tone                 string  `json:"tone"`
		AIEnabled            *bool   `json:"ai_enabled"`
		AutoRead             *bool   `json:"auto_read"`
		AIReplyDelayMin      *int    `json:"ai_reply_delay_min"`
		AIReplyDelayMax      *int    `json:"ai_reply_delay_max"`
		GreetingEnabled      *bool   `json:"greeting_enabled"`
		GreetingMessage      *string `json:"greeting_message"`
		BusinessHoursEnabled *bool   `json:"business_hours_enabled"`
		BusinessStart        *string `json:"business_start"`
		BusinessEnd          *string `json:"business_end"`
		AwayMessage          *string `json:"away_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if req.Name != "" {
		a.Name = req.Name
	}
	if req.SystemPrompt != nil {
		a.SystemPrompt = *req.SystemPrompt
	}
	if req.Tone != "" {
		a.Tone = req.Tone
	}
	if req.AIEnabled != nil {
		a.AIEnabled = *req.AIEnabled
	}
	if req.AutoRead != nil {
		a.AutoRead = *req.AutoRead
	}
	if req.AIReplyDelayMin != nil || req.AIReplyDelayMax != nil {
		minDelay, maxDelay := a.AIReplyDelayMin, a.AIReplyDelayMax
		if req.AIReplyDelayMin != nil {
			minDelay = *req.AIReplyDelayMin
		}
		if req.AIReplyDelayMax != nil {
			maxDelay = *req.AIReplyDelayMax
		}
		if minDelay < 0 || minDelay > 30 || maxDelay < minDelay || maxDelay > 30 {
			c.JSON(400, gin.H{"error": "Jeda balasan AI harus antara 0-30 detik dan jeda maksimal tidak boleh lebih kecil"})
			return
		}
		a.AIReplyDelayMin, a.AIReplyDelayMax = minDelay, maxDelay
	}
	if req.GreetingEnabled != nil {
		a.GreetingEnabled = *req.GreetingEnabled
	}
	if req.GreetingMessage != nil {
		a.GreetingMessage = *req.GreetingMessage
	}
	if req.BusinessHoursEnabled != nil {
		a.BusinessHoursEnabled = *req.BusinessHoursEnabled
	}
	if req.BusinessStart != nil {
		a.BusinessStart = *req.BusinessStart
	}
	if req.BusinessEnd != nil {
		a.BusinessEnd = *req.BusinessEnd
	}
	if req.AwayMessage != nil {
		a.AwayMessage = *req.AwayMessage
	}
	if err := database.DB.Save(&a).Error; err != nil {
		log.Printf("Gagal menyimpan agent %d: %v", a.ID, err)
		c.JSON(500, gin.H{"error": "Gagal menyimpan data"})
		return
	}
	c.JSON(200, gin.H{"data": a})
}

// maybeSummarize memperbarui memori kumulatif per kontak secara bertahap. Chat lama
// diringkas, sedangkan chat terbaru tetap dikirim utuh ke model saat menjawab.
// Dijalankan di background goroutine supaya tidak blocking reply ke user.
func maybeSummarize(agent models.Agent, senderNum string) {
	lockValue, _ := summaryMu.LoadOrStore(debounceKey(agent.ID, types.NewJID(senderNum, types.DefaultUserServer)), &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	var mem models.ConversationMemory
	database.DB.Where("agent_id = ? AND sender = ?", agent.ID, senderNum).First(&mem)
	// Migrasi lembut untuk memori versi lama yang belum memiliki checkpoint ID.
	if mem.LastChatID == 0 && mem.LastSummaryAt != nil {
		var checkpoint models.ChatHistory
		if database.DB.Where("agent_id = ? AND sender = ? AND created_at <= ?", agent.ID, senderNum, *mem.LastSummaryAt).
			Order("id desc").First(&checkpoint).Error == nil {
			mem.LastChatID = checkpoint.ID
		}
	}
	var newCount int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ? AND id > ?", agent.ID, senderNum, mem.LastChatID).Count(&newCount)
	if newCount < 4 {
		return
	}
	// Proses seluruh pesan baru secara kronologis per batch. Setiap batch memperbarui
	// ringkasan kumulatif, sehingga percakapan panjang tidak kehilangan bagian tengah.
	for {
		var msgs []models.ChatHistory
		database.DB.Where("agent_id = ? AND sender = ? AND id > ?", agent.ID, senderNum, mem.LastChatID).
			Order("id asc").Limit(40).Find(&msgs)
		if len(msgs) == 0 {
			break
		}
		summary, err := services.UpdateConversationMemory(agent.ID, mem.Summary, msgs)
		if err != nil {
			log.Printf("Summarize gagal (agent %d, %s): %v", agent.ID, senderNum, err)
			return
		}
		summary = truncateRunes(summary, 2400)
		now := time.Now()
		mem.AgentID, mem.Sender, mem.Summary, mem.LastSummaryAt = agent.ID, senderNum, summary, &now
		mem.LastChatID = msgs[len(msgs)-1].ID
		if err := database.DB.Save(&mem).Error; err != nil {
			log.Printf("Gagal menyimpan summary (agent %d, %s): %v", agent.ID, senderNum, err)
			return
		}
		if len(msgs) < 40 {
			break
		}
	}
	log.Printf("Summarized (agent %d, %s): %s", agent.ID, senderNum, mem.Summary)
}

// truncateRunes memotong string ke maksimal n rune (aman untuk UTF-8/emoji,
// tidak membelah karakter multibyte seperti slice byte biasa).
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func DeleteAgent(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	// Bebaskan sesi WA dari memori (client, goroutine, file sesi) agar tidak bocor.
	services.RemoveWA(id)
	// Bersihkan data milik agent agar tidak jadi baris yatim di DB.
	database.DB.Where("agent_id = ?", id).Delete(&models.Knowledge{})
	database.DB.Where("agent_id = ?", id).Delete(&models.ChatHistory{})
	database.DB.Where("agent_id = ?", id).Delete(&models.Contact{})
	database.DB.Where("agent_id = ?", id).Delete(&models.Handoff{})
	database.DB.Where("agent_id = ?", id).Delete(&models.AutoReply{})
	database.DB.Where("agent_id = ?", id).Delete(&models.ConversationMemory{})
	database.DB.Where("agent_id = ?", id).Delete(&models.CrawlJob{})
	database.DB.Where("agent_id = ?", id).Delete(&models.CrawlPage{})
	database.DB.Where("tenant_id = ?", currentTenantID(c)).Delete(&models.Agent{}, id)
	c.JSON(200, gin.H{"message": "Deleted"})
}

// ---------------------------------------------------------------------------
// Media Directive: [[SEND_MEDIA:id]]
// Memungkinkan AI mengirim media (gambar/video/dokumen) secara otomatis.
// Format di system prompt: "Untuk mengirim gambar produk, gunakan [[SEND_MEDIA:ID]]"
// ---------------------------------------------------------------------------

type mediaDirectiveResult struct {
	text string // teks balasan (caption + sisa teks)
}

// handleSendMediaDirective mendeteksi dan mengeksekusi directive [[SEND_MEDIA:id]] di balasan AI.
// Mengembalikan (hasil, true) bila directive ditemukan dan diproses.
func handleSendMediaDirective(agentID uint, toNumber, reply string) (mediaDirectiveResult, bool) {
	const prefix = "[[SEND_MEDIA:"
	const suffix = "]]"

	if !strings.Contains(reply, prefix) {
		return mediaDirectiveResult{}, false
	}

	// Parse directive: [[SEND_MEDIA:123]]
	start := strings.Index(reply, prefix)
	end := strings.Index(reply[start:], suffix)
	if end < 0 {
		return mediaDirectiveResult{}, false
	}
	end += start

	idStr := reply[start+len(prefix) : end]
	mediaID, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 64)
	if err != nil {
		log.Printf("[media-directive] Invalid ID: %s", idStr)
		return mediaDirectiveResult{}, false
	}

	// Ambil media dari DB
	var media models.MediaAsset
	if database.DB.Where("id = ? AND agent_id = ? AND is_active = ?", mediaID, agentID, true).First(&media).Error != nil {
		log.Printf("[media-directive] Media #%d tidak ditemukan untuk agent %d", mediaID, agentID)
		return mediaDirectiveResult{}, false
	}

	// Baca file
	data, err := os.ReadFile(media.FilePath)
	if err != nil {
		log.Printf("[media-directive] Gagal baca file %s: %v", media.FilePath, err)
		return mediaDirectiveResult{}, false
	}

	// Kirim media
	var sendErr error
	switch media.MediaType {
	case "image":
		sendErr = services.WA(agentID).SendImage(toNumber, media.Caption, media.MimeType, data)
	case "video":
		sendErr = services.WA(agentID).SendVideo(toNumber, media.Caption, media.MimeType, data)
	case "document":
		sendErr = services.WA(agentID).SendDocument(toNumber, media.FileName, media.MimeType, media.Caption, data)
	default:
		sendErr = services.WA(agentID).SendDocument(toNumber, media.FileName, media.MimeType, media.Caption, data)
	}

	if sendErr != nil {
		log.Printf("[media-directive] Gagal kirim media #%d: %v", mediaID, sendErr)
	}

	// Ambil teks di luar directive (sebelum dan sesudah token)
	textBefore := strings.TrimSpace(reply[:start])
	textAfter := strings.TrimSpace(reply[end+len(suffix):])
	finalText := textBefore
	if textAfter != "" {
		if finalText != "" {
			finalText += "\n\n"
		}
		finalText += textAfter
	}
	if finalText == "" && media.Caption != "" {
		finalText = media.Caption
	}

	return mediaDirectiveResult{text: finalText}, true
}
