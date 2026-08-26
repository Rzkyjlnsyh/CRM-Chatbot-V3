package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	openai "github.com/sashabaranov/go-openai"
)

// --- Learning Engine ---
// Menganalisa percakapan CS manusia dan mengekstrak pola gaya bahasa,
// teknik closing, penggunaan emoji, dan frasa efektif.
// Hasil pembelajaran bisa di-review dan diterapkan ke knowledge base + persona.

// StyleProfile = profil gaya bahasa yg diekstrak dari sekumpulan chat manusia.
type StyleProfile struct {
	GreetingPatterns  []string `json:"greeting_patterns"`  // variasi sapaan pembuka
	ClosingPatterns   []string `json:"closing_patterns"`   // variasi penutup/closing
	CommonPhrases     []string `json:"common_phrases"`     // frasa yg sering muncul
	EmojiUsage        []string `json:"emoji_usage"`        // emoji yg sering dipakai + konteks
	ToneDescription   string   `json:"tone_description"`   // deskripsi tone natural
	PacingStyle       string   `json:"pacing_style"`       // gaya pacing (cepat/santai/detail)
	ObjectionHandling []string `json:"objection_handling"` // cara menangani keberatan
	UpsellTechniques  []string `json:"upsell_techniques"`  // teknik upsell yg dipakai
	FollowUpStyle     []string `json:"follow_up_style"`    // gaya follow-up
}

// ExtractedPattern = satu pola yg diekstrak AI dari chat.
type ExtractedPattern struct {
	PatternType      string  `json:"pattern_type"`
	TriggerContext   string  `json:"trigger_context"`
	ResponseTemplate string  `json:"response_template"`
	EmojiSignature   string  `json:"emoji_signature"`
	Confidence       float64 `json:"confidence"`
	ClosingImpact    float64 `json:"closing_impact"`
}

// EnqueueLearningRun membuat learning run (pending) dan memprosesnya di background.
func EnqueueLearningRun(agentID uint, startDate, endDate *time.Time) (*models.LearningRun, error) {
	run := models.LearningRun{
		AgentID:         agentID,
		Status:          "pending",
		SourceStartDate: startDate,
		SourceEndDate:   endDate,
	}
	if err := database.DB.Create(&run).Error; err != nil {
		return nil, fmt.Errorf("gagal membuat learning run: %w", err)
	}
	Go("learning-run", func() { processLearningRun(run.ID, agentID, startDate, endDate) })
	return &run, nil
}

// processLearningRun menjalankan analisa learning di goroutine background:
// style profile global + pola per-label (SEMUA label, cara penanganan menuju closing).
func processLearningRun(runID, agentID uint, startDate, endDate *time.Time) {
	run := models.LearningRun{}
	if database.DB.First(&run, runID).Error != nil {
		return
	}
	run.Status = "running"
	database.DB.Save(&run)

	// 1. Chat CS manusia global (untuk style profile + pola umum).
	humanChats, err := loadHumanCSChats(agentID, startDate, endDate)
	if err != nil {
		failLearningRun(&run, "gagal memuat chat CS manusia: "+err.Error())
		return
	}
	if len(humanChats) == 0 {
		// Pesan lebih pintar: beri tahu berapa chat CS tersedia di 90 hari
		// terakhir supaya user tahu kenapa gagal & cara memperbaikinya.
		var last90 int64
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND reply <> '' AND from_human = ? AND created_at >= ?",
				agentID, true, time.Now().AddDate(0, 0, -90)).Count(&last90)
		msg := "tidak ditemukan chat CS manusia dalam rentang tersebut. Pastikan CS manusia sudah membalas pelanggan via WhatsApp terhubung"
		if last90 > 0 {
			msg = fmt.Sprintf("tidak ditemukan chat CS manusia dalam rentang yang dipilih, padahal ada %d chat CS dalam 90 hari terakhir. Perluas rentang tanggal (Dari lebih awal) lalu jalankan lagi.", last90)
		}
		failLearningRun(&run, msg)
		return
	}
	var totalChats int64
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND reply <> ''", agentID).Count(&totalChats)
	run.TotalChats = int(totalChats)
	run.HumanChats = len(humanChats)

	// 2. Style profile global.
	styleProfile, err := extractStyleProfile(agentID, humanChats)
	if err != nil {
		failLearningRun(&run, "gagal ekstrak style profile: "+err.Error())
		return
	}
	styleJSON, _ := json.Marshal(styleProfile)
	run.StyleProfile = string(styleJSON)

	// 3. Pola global (semua chat CS manusia).
	patterns, err := extractPatterns(agentID, humanChats, styleProfile)
	if err != nil {
		log.Printf("Learning: gagal ekstrak pola global: %v (lanjut)", err)
	}
	savePatterns(run.ID, agentID, "", "", "human", patterns)

	// 4. Pola per-label — pelajari SEMUA label & cara penanganannya menuju closing.
	for _, l := range agentLabels(agentID) {
		labeled := loadLabeledHumanChats(agentID, l.LabelID, startDate, endDate)
		if len(labeled) < 3 {
			continue // data label ini terlalu sedikit untuk diekstrak
		}
		lp, err := extractLabelPatterns(agentID, l.Name, labeled)
		if err != nil {
			log.Printf("Learning: label %q gagal: %v", l.Name, err)
			continue
		}
		savePatterns(run.ID, agentID, l.LabelID, l.Name, "human", lp)
	}

	// 4b. Belajar dari chat AI sendiri yang terbukti menuju closing (supervised
	// oleh HASIL nyata, bukan estimasi): pola dari percakapan yang berakhir
	// closing diberi confidence lebih konservatif (maks 0.75).
	if cfg := GetLearningConfig(agentID); cfg.IncludeAIClosed == nil || *cfg.IncludeAIClosed {
		aiChats := loadAISuccessChats(agentID, startDate, endDate)
		if len(aiChats) >= 3 {
			if aiPatterns, aerr := extractPatterns(agentID, aiChats, styleProfile); aerr != nil {
				log.Printf("Learning: ekstrak pola AI-success gagal: %v (lanjut)", aerr)
			} else {
				savePatterns(run.ID, agentID, "", "", "ai_success", aiPatterns)
			}
		}
	}

	// 5. Selesaikan + hitung total pola.
	now := time.Now()
	run.Status = "completed"
	run.CompletedAt = &now
	var pc int64
	database.DB.Model(&models.LearningPattern{}).Where("learning_run_id = ?", run.ID).Count(&pc)
	run.PatternCount = int(pc)

	// Berapa label berbeda yang dipelajari (pola ber-label).
	var labeledCount int64
	database.DB.Model(&models.LearningPattern{}).Where("learning_run_id = ? AND label_id <> ''", run.ID).Distinct("label_id").Count(&labeledCount)

	// 6. Auto-apply bila diaktifkan (learning otomatis, bukan cuma review manual).
	cfg := GetLearningConfig(agentID)
	appliedCount := 0
	if cfg.AutoApply {
		appliedCount = autoApplyPatterns(agentID, run.ID, cfg)
		log.Printf("Learning: agent %d auto-apply %d pola (run %d)", agentID, appliedCount, run.ID)
	}

	// 7. Rekap hasil — keterangan apa saja yg dipelajari/diterapkan AI.
	run.Summary = buildRunSummary(run, int(labeledCount), appliedCount, cfg.AutoApply)
	database.DB.Save(&run)
}

// buildRunSummary menyusun rekap teks hasil learning: berapa chat dianalisa,
// berapa pola diekstrak, berapa label dipelajari, dan berapa pola diterapkan.
func buildRunSummary(run models.LearningRun, labeledCount, appliedCount int, autoApplied bool) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Menganalisa %d chat CS manusia (dari total %d chat).", run.HumanChats, run.TotalChats))
	lines = append(lines, fmt.Sprintf("Mengekstrak %d pola komunikasi.", run.PatternCount))
	if labeledCount > 0 {
		lines = append(lines, fmt.Sprintf("Mempelajari penanganan %d label WhatsApp beserta jalur menuju closing.", labeledCount))
	}
	if autoApplied {
		if appliedCount > 0 {
			lines = append(lines, fmt.Sprintf("Auto-menerapkan %d pola confidence tinggi ke knowledge base.", appliedCount))
		} else {
			lines = append(lines, "Auto-apply aktif, tapi tidak ada pola yang memenuhi ambang confidence.")
		}
	} else {
		lines = append(lines, fmt.Sprintf("%d pola menunggu review kamu di tab Patterns.", run.PatternCount))
	}
	return strings.Join(lines, " ")
}

// failLearningRun menandai run gagal dengan pesan error.
func failLearningRun(run *models.LearningRun, msg string) {
	now := time.Now()
	run.Status = "failed"
	run.Error = msg
	run.CompletedAt = &now
	database.DB.Save(run)
	log.Printf("Learning: run %d gagal: %s", run.ID, msg)
}

// savePatterns menyimpan pola hasil ekstraksi ke DB dengan konteks label.
// source: "human" (dari CS manusia) atau "ai_success" (dari chat AI yg closing).
func savePatterns(runID, agentID uint, labelID, labelName, source string, patterns []ExtractedPattern) {
	for _, p := range patterns {
		conf := p.Confidence
		if source == "ai_success" && conf > 0.75 {
			conf = 0.75 // konservatif: pola dari AI sendiri belum setinggi pola CS manusia
		}
		lp := models.LearningPattern{
			LearningRunID:    runID,
			AgentID:          agentID,
			LabelID:          labelID,
			LabelName:        labelName,
			PatternType:      p.PatternType,
			Source:           source,
			TriggerContext:   p.TriggerContext,
			ResponseTemplate: p.ResponseTemplate,
			EmojiSignature:   p.EmojiSignature,
			Confidence:       conf,
			UsageCount:       1,
			ClosingImpact:    p.ClosingImpact,
			Status:           "suggested",
		}
		database.DB.Create(&lp)
	}
}

// agentLabels mengembalikan daftar label WhatsApp milik agent.
func agentLabels(agentID uint) []models.Label {
	var labels []models.Label
	database.DB.Where("agent_id = ?", agentID).Order("name asc").Find(&labels)
	return labels
}

// loadLabeledHumanChats mengambil chat CS manusia dari kontak yang diberi
// label tertentu (label_id), dalam rentang tanggal.
func loadLabeledHumanChats(agentID uint, labelID string, startDate, endDate *time.Time) []models.ChatHistory {
	var senders []string
	database.DB.Model(&models.ChatLabel{}).Where("agent_id = ? AND label_id = ?", agentID, labelID).Pluck("sender", &senders)
	if len(senders) == 0 {
		return nil
	}
	var chats []models.ChatHistory
	q := database.DB.Where("agent_id = ? AND from_human = ? AND sender IN ?", agentID, true, senders)
	if startDate != nil {
		q = q.Where("created_at >= ?", *startDate)
	}
	if endDate != nil {
		q = q.Where("created_at <= ?", *endDate)
	}
	q.Order("created_at asc").Find(&chats)
	return chats
}

// autoApplyPatterns menerapkan pola suggested dgn confidence >= MinConfidence.
// Buat snapshot keamanan dulu (rollback point), lalu apply; return jumlah yg diterapkan.
func autoApplyPatterns(agentID, runID uint, cfg models.LearningConfig) int {
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = 0.7
	}
	if _, err := CreateSnapshot(agentID, "Auto-apply learning run "+fmt.Sprint(runID), &runID); err != nil {
		log.Printf("Learning: snapshot auto-apply gagal: %v", err)
	}
	var patterns []models.LearningPattern
	database.DB.Where("agent_id = ? AND status = ? AND confidence >= ?", agentID, "suggested", cfg.MinConfidence).
		Order("confidence desc").Find(&patterns)
	applied := 0
	for _, p := range patterns {
		if _, err := ApplyPattern(agentID, p.ID); err == nil {
			applied++
		}
	}
	return applied
}

// loadHumanCSChats mengambil chat di mana CS manusia yg membalas (via device/WA Web).
func loadHumanCSChats(agentID uint, startDate, endDate *time.Time) ([]models.ChatHistory, error) {
	var chats []models.ChatHistory
	q := database.DB.Where("agent_id = ? AND reply <> '' AND from_human = ?", agentID, true)
	if startDate != nil {
		q = q.Where("created_at >= ?", *startDate)
	}
	if endDate != nil {
		q = q.Where("created_at <= ?", *endDate)
	}
	if err := q.Order("created_at asc").Find(&chats).Error; err != nil {
		return nil, err
	}
	return chats, nil
}

// noiseReplies = balasan fallback/hold yang BUKAN hasil pelayanan nyata —
// jangan pernah dijadikan materi belajar (anti-halu).
var noiseReplies = []string{
	safeUngroundedReply(),
	// Salinan literal handlers.humanFacingHoldReply (hindari import cycle handlers↔services).
	"Baik kak, saya cek dulu ya biar informasinya tepat. Mohon ditunggu sebentar 🙏",
	"Maaf, ada kendala teknis.",
}

// loadAISuccessChats mengambil chat yang DIBALAS AI dan pelanggannya kemudian
// closing (ada ClosingRecord atau tahap kontak = closing). Sumber belajar
// loadAISuccessChats — kontak closing = gabungan: ClosingRecord (transaksi
// terdeteksi), tahap "customer", DAN label WA closing yang dipasang CS manusia.
func loadAISuccessChats(agentID uint, startDate, endDate *time.Time) []models.ChatHistory {
	seen := make(map[string]bool)
	var recSenders []string
	database.DB.Model(&models.ClosingRecord{}).Where("agent_id = ?", agentID).Pluck("sender", &recSenders)
	for _, s := range recSenders {
		seen[s] = true
	}
	// Label WA closing (dipasang CS manusia via WhatsApp, tersinkron).
	for _, s := range labelClosingSenders(agentID, nil) {
		seen[s] = true
	}
	// Tahap closing chatloop = "customer" (tahap baku). Chat AI yang
	// pelanggannya closing dijadikan sumber belajar "terbukti berhasil".
	var contactSenders []string
	database.DB.Model(&models.Contact{}).Where("agent_id = ? AND lead_stage = ?", agentID, "customer").
		Pluck("number", &contactSenders)
	for _, s := range contactSenders {
		seen[s] = true
	}
	senders := make([]string, 0, len(seen))
	for s := range seen {
		if s != "" {
			senders = append(senders, s)
		}
	}
	if len(senders) == 0 {
		return nil
	}
	var chats []models.ChatHistory
	q := database.DB.Where("agent_id = ? AND reply <> '' AND from_human = ? AND sender IN ? AND reply NOT IN ?",
		agentID, false, senders, noiseReplies)
	if startDate != nil {
		q = q.Where("created_at >= ?", *startDate)
	}
	if endDate != nil {
		q = q.Where("created_at <= ?", *endDate)
	}
	q.Order("created_at asc").Find(&chats)
	return chats
}

// --- Incremental (real-time) learning ---

var (
	incMu      sync.Mutex
	incLastRun = map[uint]time.Time{}
)

const incrementalDebounce = 15 * time.Minute

// MaybeTriggerIncrementalLearning dipanggil setiap kali CS manusia membalas.
// Bila learning Enabled (toggle ON), antrikan analisa ringan — debounce 15
// menit per agent — atas chat 24 jam terakhir. Pola baru langsung masuk
// "suggested" tanpa menunggu jadwal harian. Non-blocking & murah (tanpa
// ekstraksi style profile; hanya pola + dedup).
func MaybeTriggerIncrementalLearning(agentID uint) {
	if agentID == 0 || database.DB == nil {
		return
	}
	// Hanya agent yang pernah membuka pengaturan learning (punya baris config)
	// yang berpartisipasi — hindari membuat config diam-diam untuk semua agent.
	var count int64
	database.DB.Model(&models.LearningConfig{}).Where("agent_id = ?", agentID).Count(&count)
	if count == 0 {
		return
	}
	cfg := GetLearningConfig(agentID)
	if !cfg.Enabled {
		return
	}
	incMu.Lock()
	last, ok := incLastRun[agentID]
	due := !ok || time.Since(last) >= incrementalDebounce
	if due {
		incLastRun[agentID] = time.Now()
	}
	incMu.Unlock()
	if !due {
		return
	}
	Go("incremental-learning", func() {
		defer RecoverGo("incremental-learning")
		runIncrementalLearning(agentID)
	})
}

// runIncrementalLearning menjalankan satu putaran analisa ringan real-time:
// chat CS manusia + chat AI-closing 24 jam terakhir → pola baru (dedup + cap)
// masuk suggested. Memakai SATU LearningRun bertanda "[realtime]" per hari
// agar tab Runs tidak dibanjiri baris.
func runIncrementalLearning(agentID uint) {
	start := time.Now().Add(-24 * time.Hour)
	end := time.Now()
	humanChats, err := loadHumanCSChats(agentID, &start, &end)
	if err != nil || len(humanChats) < 3 {
		return
	}
	cfg := GetLearningConfig(agentID)
	run := ensureIncrementalRun(agentID)
	if run.ID == 0 {
		return
	}
	// Tanpa style profile: pola cukup; profil gaya tetap dijalankan oleh run
	// penuh (manual/jadwal) supaya biaya AI incremental tetap minimal.
	patterns, perr := extractPatterns(agentID, humanChats, StyleProfile{})
	if perr == nil {
		savePatternsDedup(run.ID, agentID, "", "", "human", patterns, cfg.MaxPatternsPerRun)
	} else {
		log.Printf("Learning[realtime]: agent %d pola gagal: %v", agentID, perr)
	}
	if cfg.IncludeAIClosed == nil || *cfg.IncludeAIClosed {
		aiChats := loadAISuccessChats(agentID, &start, &end)
		if len(aiChats) >= 3 {
			if ap, aerr := extractPatterns(agentID, aiChats, StyleProfile{}); aerr == nil {
				savePatternsDedup(run.ID, agentID, "", "", "ai_success", ap, cfg.MaxPatternsPerRun)
			}
		}
	}
	now := time.Now()
	var pc int64
	database.DB.Model(&models.LearningPattern{}).Where("learning_run_id = ?", run.ID).Count(&pc)
	database.DB.Model(&run).Updates(map[string]any{
		"total_chats":  len(humanChats),
		"human_chats":  len(humanChats),
		"pattern_count": int(pc),
		"completed_at": &now,
	})
}

// ensureIncrementalRun mencari (atau membuat) LearningRun harian bertanda
// incremental — satu baris per hari per agent, status langsung completed.
func ensureIncrementalRun(agentID uint) models.LearningRun {
	var run models.LearningRun
	today := time.Now().Truncate(24 * time.Hour)
	if database.DB.Where("agent_id = ? AND summary LIKE ? AND created_at >= ?", agentID, "[realtime]%", today).
		Order("id desc").First(&run).Error == nil {
		return run
	}
	run = models.LearningRun{
		AgentID: agentID,
		Status:  "completed",
		Summary: "[realtime] Belajar kontinu dari percakapan terbaru (24 jam)",
	}
	if err := database.DB.Create(&run).Error; err != nil {
		log.Printf("Learning[realtime]: gagal buat run agent %d: %v", agentID, err)
		return models.LearningRun{}
	}
	return run
}

// savePatternsDedup menyimpan pola hasil ekstraksi dengan dedup (pola
// suggested yang sama tidak diduplikasi) dan batas jumlah per putaran.
func savePatternsDedup(runID, agentID uint, labelID, labelName, source string, patterns []ExtractedPattern, cap int) {
	if cap <= 0 {
		cap = 10
	}
	saved := 0
	for _, p := range patterns {
		if saved >= cap {
			break
		}
		var dup int64
		database.DB.Model(&models.LearningPattern{}).
			Where("agent_id = ? AND trigger_context = ? AND response_template = ? AND status = ?",
				agentID, p.TriggerContext, p.ResponseTemplate, "suggested").Count(&dup)
		if dup > 0 {
			continue
		}
		conf := p.Confidence
		if source == "ai_success" && conf > 0.75 {
			conf = 0.75
		}
		database.DB.Create(&models.LearningPattern{
			LearningRunID:    runID,
			AgentID:          agentID,
			LabelID:          labelID,
			LabelName:        labelName,
			PatternType:      p.PatternType,
			Source:           source,
			TriggerContext:   p.TriggerContext,
			ResponseTemplate: p.ResponseTemplate,
			EmojiSignature:   p.EmojiSignature,
			Confidence:       conf,
			UsageCount:       1,
			ClosingImpact:    p.ClosingImpact,
			Status:           "suggested",
		})
		saved++
	}
}

// extractStyleProfile menggunakan AI untuk menganalisa kumpulan chat dan
// menghasilkan profil gaya bahasa CS manusia. Memakai key AI yang dikonfigurasi.
// agent (BYO, Fase 0).
func extractStyleProfile(agentID uint, chats []models.ChatHistory) (StyleProfile, error) {
	if len(chats) == 0 {
		return StyleProfile{}, fmt.Errorf("chat kosong")
	}

	// Bangun transkrip untuk analisa (batasi agar tidak terlalu panjang)
	transcript := buildTranscriptSample(chats, 8000)

	prompt := `Analisa transkrip chat WhatsApp dari customer service MANUSIA berikut.
Ekstrak profil gaya komunikasi dalam JSON.

Fokus pada:
1. greeting_patterns: variasi sapaan pembuka yg dipakai (maks 5)
2. closing_patterns: cara menutup percakapan / mengarahkan ke transaksi (maks 5)
3. common_phrases: frasa khas yg sering muncul (maks 8)
4. emoji_usage: emoji yg sering dipakai beserta konteksnya (maks 5, format: "emoji - konteks")
5. tone_description: deskripsi tone/gaya bahasa (1-2 kalimat, Bahasa Indonesia)
6. pacing_style: gaya tempo percakapan — "cepat/to the point" atau "santai/berbasa-basi" atau "detail/edukatif"
7. objection_handling: cara CS menangani keberatan/keraguan pelanggan (maks 3)
8. upsell_techniques: teknik upsell/cross-sell yg dipakai (maks 3)
9. follow_up_style: gaya follow-up setelah percakapan (maks 2)

Output HANYA JSON valid, tanpa penjelasan tambahan.`

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := CreateAICompletion(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: prompt},
		{Role: openai.ChatMessageRoleUser, Content: transcript},
	}, 1500, 0.3)
	if err != nil {
		return StyleProfile{}, err
	}
	if len(resp.Choices) == 0 {
		return StyleProfile{}, fmt.Errorf("AI tidak mengembalikan hasil")
	}

	var profile StyleProfile
	raw := cleanJSON(resp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(raw), &profile); err != nil {
		return StyleProfile{}, fmt.Errorf("gagal parse style profile: %w (raw: %.200s)", err, raw)
	}
	return profile, nil
}

// extractPatterns mengekstrak pola-pola individual (greeting, closing, dll) dari chat.
// Memakai key AI yang dikonfigurasi di pengaturan.
func extractPatterns(agentID uint, chats []models.ChatHistory, profile StyleProfile) ([]ExtractedPattern, error) {
	if len(chats) < 3 {
		return nil, fmt.Errorf("chat terlalu sedikit untuk ekstrak pola (min 3)")
	}

	transcript := buildTranscriptSample(chats, 6000)

	prompt := `Analisa transkrip chat WhatsApp CS manusia berikut dan ekstrak POLA-POLA komunikasi yg efektif.
Output dalam JSON array. Setiap elemen punya field:
- pattern_type: "greeting" | "closing" | "objection_handling" | "upsell" | "follow_up" | "phrase" | "emoji_style"
- trigger_context: situasi/kalimat pelanggan yg memicu pola ini (singkat, 1 kalimat)
- response_template: contoh balasan CS yg bisa ditiru AI (natural, tidak kaku)
- emoji_signature: emoji yg cocok untuk pola ini (bisa kosong "")
- confidence: 0.0-1.0, seberapa yakin pola ini efektif (lihat dari reaksi pelanggan setelahnya)
- closing_impact: 0.0-1.0, seberapa besar pola ini mendorong ke closing (0 kalau bukan closing)

Prioritaskan pola yg:
- Mendapat respons positif dari pelanggan (lanjut diskusi, makin antusias)
- Natural dan tidak terasa template
- Efektif mengarahkan ke closing

Max 10 pola. Output HANYA JSON array.`

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := CreateAICompletion(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: prompt},
		{Role: openai.ChatMessageRoleUser, Content: transcript},
	}, 2000, 0.4)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("AI tidak mengembalikan pola")
	}

	var patterns []ExtractedPattern
	raw := cleanJSON(resp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(raw), &patterns); err != nil {
		return nil, fmt.Errorf("gagal parse patterns: %w (raw: %.200s)", err, raw)
	}

	// Filter: hanya pola dengan confidence cukup
	var filtered []ExtractedPattern
	for _, p := range patterns {
		if p.Confidence >= 0.4 && p.ResponseTemplate != "" {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

// extractLabelPatterns mengekstrak pola penanganan untuk satu label: bagaimana
// CS manusia menangani kontak berlabel ini dan mengarahkannya ke closing.
func extractLabelPatterns(agentID uint, labelName string, chats []models.ChatHistory) ([]ExtractedPattern, error) {
	if len(chats) < 3 {
		return nil, fmt.Errorf("chat label %q terlalu sedikit", labelName)
	}
	transcript := buildTranscriptSample(chats, 6000)

	prompt := fmt.Sprintf(`Analisa transkrip chat WhatsApp dari kontak yang berlabel "%s".
Ekstrak bagaimana CS manusia MENANGANI kontak dengan label ini, dan bagaimana ia MENGARAHKAN mereka ke closing.

Fokus:
1. label_handling: pola cara CS menyapa/menangani kontak berlabel ini (trigger_context = situasi khas label ini)
2. objection_handling: cara CS menangani keberatan/keraguan khas label ini
3. closing_path: langkah/tawaran yg dipakai CS untuk memindahkan kontak dari label ini ke closing (mis. tawaran diskon, follow-up, trial)
4. upsell: teknik upsell yg relevan untuk label ini
5. follow_up: gaya follow-up utk kontak berlabel ini

Output JSON array, tiap elemen:
- pattern_type: "label_handling" | "objection_handling" | "closing_path" | "upsell" | "follow_up"
- trigger_context: situasi/kalimat pelanggan (singkat, 1 kalimat)
- response_template: contoh balasan CS natural yg bisa ditiru AI
- emoji_signature: emoji yg cocok (bisa "")
- confidence: 0.0-1.0
- closing_impact: 0.0-1.0 (seberapa besar mendorong closing)

Max 8 pola. Output HANYA JSON array.`, labelName)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := CreateAICompletion(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: prompt},
		{Role: openai.ChatMessageRoleUser, Content: transcript},
	}, 2000, 0.4)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("AI tidak mengembalikan pola")
	}
	var patterns []ExtractedPattern
	raw := cleanJSON(resp.Choices[0].Message.Content)
	if err := json.Unmarshal([]byte(raw), &patterns); err != nil {
		return nil, fmt.Errorf("gagal parse pola label: %w (raw: %.200s)", err, raw)
	}
	var filtered []ExtractedPattern
	for _, p := range patterns {
		if p.Confidence >= 0.4 && p.ResponseTemplate != "" {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

// buildTranscriptSample membangun sampel transkrip dari chat (dibatasi karakter).
// Mengambil chat TERBARU (paling relevan) bukan yang paling lama.
func buildTranscriptSample(chats []models.ChatHistory, maxChars int) string {
	var sb strings.Builder
	for i := len(chats) - 1; i >= 0; i-- {
		c := chats[i]
		line := ""
		if c.Message != "" {
			line = "Pelanggan: " + c.Message + "\n"
		}
		if c.Reply != "" {
			line += "CS: " + c.Reply + "\n"
		}
		if sb.Len()+len(line) > maxChars {
			break
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// ApplyPattern menerapkan satu pola ke knowledge base agent.
func ApplyPattern(agentID, patternID uint) (*models.Knowledge, error) {
	var pattern models.LearningPattern
	if err := database.DB.Where("agent_id = ? AND id = ?", agentID, patternID).First(&pattern).Error; err != nil {
		return nil, fmt.Errorf("pola tidak ditemukan: %w", err)
	}
	if pattern.Status == "applied" {
		return nil, fmt.Errorf("pola sudah diterapkan sebelumnya")
	}

	// Buat knowledge entry dari pola
	question := pattern.TriggerContext
	if question == "" {
		question = fmt.Sprintf("Situasi: %s", pattern.PatternType)
	}
	answer := pattern.ResponseTemplate

	k := models.Knowledge{
		AgentID:  agentID,
		Question: question,
		Answer:   answer,
		Tags:     fmt.Sprintf("learned, %s", pattern.PatternType),
		Source:   "learning",
	}
	if err := database.DB.Create(&k).Error; err != nil {
		return nil, fmt.Errorf("gagal simpan knowledge: %w", err)
	}

	// Index embedding untuk knowledge baru
	IndexKnowledge(&k)

	// Tandai pola sebagai applied
	now := time.Now()
	kid := k.ID
	pattern.Status = "applied"
	pattern.AppliedAt = &now
	pattern.KnowledgeID = &kid
	database.DB.Save(&pattern)

	return &k, nil
}

// RejectPattern menolak satu pola (tidak diterapkan).
func RejectPattern(agentID, patternID uint) error {
	return database.DB.Model(&models.LearningPattern{}).
		Where("agent_id = ? AND id = ? AND status = ?", agentID, patternID, "suggested").
		Update("status", "rejected").Error
}

// CreateSnapshot membuat versi backup persona + knowledge agent.
func CreateSnapshot(agentID uint, label string, runID *uint) (*models.LearningSnapshot, error) {
	var agent models.Agent
	if err := database.DB.First(&agent, agentID).Error; err != nil {
		return nil, fmt.Errorf("agent tidak ditemukan")
	}

	var knowledge []models.Knowledge
	database.DB.Where("agent_id = ?", agentID).Find(&knowledge)

	// Serialize knowledge (tanpa embedding untuk hemat space)
	type kMini struct {
		Question string `json:"q"`
		Answer   string `json:"a"`
		Tags     string `json:"t"`
		Source   string `json:"s"`
	}
	var kList []kMini
	for _, k := range knowledge {
		kList = append(kList, kMini{
			Question: k.Question,
			Answer:   k.Answer,
			Tags:     k.Tags,
			Source:   k.Source,
		})
	}
	data := map[string]any{
		"persona":   agent.SystemPrompt,
		"tone":      agent.Tone,
		"knowledge": kList,
	}
	dataJSON, _ := json.Marshal(data)

	snap := models.LearningSnapshot{
		AgentID:        agentID,
		LearningRunID:  runID,
		SnapshotType:   "full",
		Label:          label,
		DataJSON:       string(dataJSON),
		PersonaAt:      agent.SystemPrompt,
		KnowledgeCount: len(knowledge),
	}
	if err := database.DB.Create(&snap).Error; err != nil {
		return nil, fmt.Errorf("gagal buat snapshot: %w", err)
	}
	return &snap, nil
}

// RollbackToSnapshot mengembalikan persona & knowledge agent ke versi snapshot.
// Knowledge yang ada dihapus dulu, lalu direstore dari snapshot.
func RollbackToSnapshot(agentID, snapshotID uint) (*models.LearningSnapshot, error) {
	var snap models.LearningSnapshot
	if err := database.DB.Where("agent_id = ? AND id = ?", agentID, snapshotID).First(&snap).Error; err != nil {
		return nil, fmt.Errorf("snapshot tidak ditemukan")
	}

	// Parse data
	var data struct {
		Persona   string `json:"persona"`
		Tone      string `json:"tone"`
		Knowledge []struct {
			Question string `json:"q"`
			Answer   string `json:"a"`
			Tags     string `json:"t"`
			Source   string `json:"s"`
		} `json:"knowledge"`
	}
	if err := json.Unmarshal([]byte(snap.DataJSON), &data); err != nil {
		return nil, fmt.Errorf("gagal parse snapshot: %w", err)
	}

	// 1. Hapus knowledge yg ada (kecuali source=manual jika config preserve)
	// Untuk rollback, kita hapus semua dan restore dari snapshot
	database.DB.Where("agent_id = ? AND source <> ?", agentID, "manual").Delete(&models.Knowledge{})

	// 2. Restore knowledge
	for _, k := range data.Knowledge {
		newK := models.Knowledge{
			AgentID:  agentID,
			Question: k.Question,
			Answer:   k.Answer,
			Tags:     k.Tags,
			Source:   k.Source,
		}
		database.DB.Create(&newK)
		IndexKnowledge(&newK)
	}

	// 3. Restore persona
	database.DB.Model(&models.Agent{}).Where("id = ?", agentID).Updates(map[string]any{
		"system_prompt": data.Persona,
		"tone":          data.Tone,
	})

	// Invalidate cache
	InvalidateKB(agentID)

	log.Printf("Learning: rollback agent %d ke snapshot %d (%s) — %d knowledge direstore", agentID, snapshotID, snap.Label, len(data.Knowledge))
	return &snap, nil
}

// DefaultClosingLabels = nama label WhatsApp penanda closing bawaan. Bisa
// diubah per agent dari panel Konfigurasi Learning (bukan hardcode kaku).
const DefaultClosingLabels = "Transfer,Closing,Deal,Lunas,DP,Selesai"

// ClosingLabelNames memecah string label closing jadi set nama (trim + lower).
func ClosingLabelNames(raw string) map[string]bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = DefaultClosingLabels
	}
	out := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		n := strings.ToLower(strings.TrimSpace(p))
		if n != "" {
			out[n] = true
		}
	}
	return out
}

// labelClosingSenders = kontak unik yang menyandang label WA closing (dipasang
// CS manusia via WhatsApp, tersinkron ke sistem). since != nil membatasi ke
// kontak yang aktif chat sejak tanggal itu (label tak ber-tanggal).
func labelClosingSenders(agentID uint, since *time.Time) []string {
	cfg := GetLearningConfig(agentID)
	names := ClosingLabelNames(cfg.ClosingLabels)
	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil
	}
	var labelIDs []string
	database.DB.Model(&models.Label{}).Where("LOWER(name) IN ?", keys).Pluck("label_id", &labelIDs)
	if len(labelIDs) == 0 {
		return nil
	}
	q := database.DB.Model(&models.ChatLabel{}).
		Where("agent_id = ? AND label_id IN ?", agentID, labelIDs)
	if since != nil {
		// Hanya kontak yang aktif chat dalam rentang — label bisa menempel
		// sejak lama, tapi close rate mengukur kontak aktif.
		var active []string
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND created_at >= ?", agentID, *since).
			Distinct("sender").Pluck("sender", &active)
		q = q.Where("sender IN ?", active)
	}
	var senders []string
	q.Distinct("sender").Pluck("sender", &senders)
	return senders
}

// ClosingSenderSet = kontak unik yang dianggap closing (gabungan): punya
// ClosingRecord (transaksi terdeteksi) ATAU menyandang label WA closing.
func ClosingSenderSet(agentID uint, since *time.Time) map[string]bool {
	out := map[string]bool{}
	var recSenders []string
	rq := database.DB.Model(&models.ClosingRecord{}).Where("agent_id = ?", agentID)
	if since != nil {
		rq = rq.Where("created_at >= ?", *since)
	}
	rq.Pluck("sender", &recSenders)
	for _, s := range recSenders {
		if s != "" {
			out[s] = true
		}
	}
	for _, s := range labelClosingSenders(agentID, since) {
		if s != "" {
			out[s] = true
		}
	}
	return out
}

// GetLearningConfig mengambil/membuat config learning untuk agent.
func GetLearningConfig(agentID uint) models.LearningConfig {
	var cfg models.LearningConfig
	if database.DB.Where("agent_id = ?", agentID).First(&cfg).Error != nil {
		// Default config
		cfg = models.LearningConfig{
			AgentID:                 agentID,
			Enabled:                 false,
			AutoApply:               false,
			MinConfidence:           0.7,
			MinUsageCount:           3,
			MaxPatternsPerRun:       10,
			PreserveManualKnowledge: true,
			ScheduleEnabled:         false,
			ScheduleCron:            "0 2 * * *",
			LookbackDays:            30,
			ClosingLabels:           DefaultClosingLabels,
		}
		database.DB.Create(&cfg)
	}
	return cfg
}

// SaveLearningConfig menyimpan perubahan config.
func SaveLearningConfig(cfg models.LearningConfig) error {
	cfg.UpdatedAt = time.Now()
	return database.DB.Save(&cfg).Error
}

// cleanJSON membersihkan output AI dari markdown fences.
func cleanJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	// Hapus ```json ... ``` fences
	if idx := strings.Index(raw, "```"); idx >= 0 {
		start := idx + 3
		if semi := strings.Index(raw[start:], "\n"); semi >= 0 {
			start += semi + 1
		}
		if end := strings.LastIndex(raw, "```"); end > start {
			raw = raw[start:end]
		}
	}
	// Cari JSON array/object boundary
	raw = strings.TrimSpace(raw)
	if len(raw) > 0 && (raw[0] == '[' || raw[0] == '{') {
		return raw
	}
	// Fallback: coba temukan JSON dalam teks
	if idx := strings.Index(raw, "["); idx >= 0 {
		if end := strings.LastIndex(raw, "]"); end > idx {
			return raw[idx : end+1]
		}
	}
	if idx := strings.Index(raw, "{"); idx >= 0 {
		if end := strings.LastIndex(raw, "}"); end > idx {
			return raw[idx : end+1]
		}
	}
	return raw
}
