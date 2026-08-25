package handlers

import (
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"
)

// TestChat menjalankan AI agent tanpa WhatsApp (simulator "coba chat" di dashboard).
// Pipeline diselaraskan dengan production: routing produk/form, shipping, dan kebijakan eskalasi.
func TestChat(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Message string `json:"message"`
		History []struct {
			Role string `json:"role"` // "user" | "bot"
			Text string `json:"text"`
		} `json:"history"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.JSON(400, gin.H{"error": "Pesan kosong"})
		return
	}
	// Simulator multi-turn: pakai riwayat dari frontend (tanpa menyentuh ChatHistory asli/analytics).
	var history []models.ChatHistory
	hist := req.History
	if len(hist) > 40 {
		hist = hist[len(hist)-40:]
	}
	for _, h := range hist {
		switch h.Role {
		case "user":
			history = append(history, models.ChatHistory{Message: h.Text})
		case "bot":
			history = append(history, models.ChatHistory{Reply: h.Text})
		}
	}
	// Samakan pemotongan konteks dengan production (berdasarkan budget rune, bukan angka pesan tetap).
	newestFirst := make([]models.ChatHistory, len(history))
	for i := range history {
		newestFirst[len(history)-1-i] = history[i]
	}
	history = historyWithinContextBudget(newestFirst, recentContextRuneBudget)

	var agent models.Agent
	database.DB.First(&agent, id)
	prompt := agent.SystemPrompt
	if prompt == "" {
		prompt = "Kamu adalah asisten AI yang ramah. Jawab dalam bahasa Indonesia."
	}
	tone := agent.Tone
	if tone == "" {
		tone = "ramah"
	}
	// Memory per-kontak tidak dipakai di simulator (tidak ada nomor pengirim nyata).
	start := time.Now()
	enhancedPrompt := prompt
	routingText := req.Message
	for i := len(history) - 1; i >= 0 && i >= len(history)-4; i-- {
		routingText += "\n" + history[i].Message + "\n" + history[i].Reply
	}
	if !isGenericGreetingMessage(req.Message) {
		if productRouting := productCheckoutRoutingPrompt(id, testAITurnSender, routingText); productRouting != "" {
			enhancedPrompt += "\n\n" + productRouting
		}
		if formRouting := aiFormRoutingPrompt(id, testAITurnSender); formRouting != "" {
			enhancedPrompt += "\n\n" + formRouting
		}
	}
	shippingCtx := maybeBuildShippingContext(agent, req.Message, history)
	usedShippingTool := strings.Contains(shippingCtx, "ONGKIR_")
	turnError := shippingTurnError(shippingCtx)
	if shippingCtx != "" {
		enhancedPrompt += "\n\n" + shippingCtx
	}
	chatResult, err := services.ChatWithKnowledge(id, enhancedPrompt, tone, req.Message, history)
	reply := chatResult.Reply
	escalate := chatResult.Escalate
	model := chatResult.Model
	knowledgeCount := chatResult.Trace.KnowledgeUsedCount
	trace := chatResult.Trace
	if err != nil {
		latencyMs := time.Since(start).Milliseconds()
		if turnError != "" {
			turnError += "; "
		}
		turnError += "ai: " + err.Error()
		logAITurn(id, testAITurnSender, req.Message, "", model, knowledgeCount, usedShippingTool, false, turnError, latencyMs, trace)
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	reply, escalate, turnError = applyEscalationPolicy(id, enhancedPrompt, tone, req.Message, history, reply, escalate, turnError)
	if escalate {
		reply = humanFacingHoldReply
	}
	// Samakan production: jalankan directive form/checkout + free-collection di simulator.
	if productStart, ok := handleProductCheckoutDirective(id, testAITurnSender, reply); ok {
		reply = productStart.reply
	} else if formStart, ok := handleAIFormDirective(id, testAITurnSender, reply); ok {
		reply = formStart.reply
	} else if productStart, ok := startProductFromFreeCollection(id, testAITurnSender, req.Message, routingText, reply); ok {
		reply = productStart.reply
	} else if formStart, ok := startAIFormFromFreeCollection(id, testAITurnSender, req.Message, routingText, reply); ok {
		reply = formStart.reply
	}
	// Proses directive yang tidak mengubah alur percakapan
	reply, _ = handleLabelDirective(id, testAITurnSender, reply)
	reply, _ = handleBuatResiDirective(id, testAITurnSender, reply)
	latencyMs := time.Since(start).Milliseconds()
	logAITurn(id, testAITurnSender, req.Message, reply, model, knowledgeCount, usedShippingTool, escalate, turnError, latencyMs, trace)

	reply = services.LinkifyWhatsApp(reply, agent.Number) // nomor WA jadi tautan klik (kecuali nomor sendiri)
	c.JSON(200, gin.H{
		"reply": reply, "escalate": escalate, "model": model,
		"knowledge_count":  knowledgeCount,
		"retrieval_mode":   trace.RetrievalMode,
		"retrieval_query":  trace.RetrievalQuery,
		"top_similarity":   trace.TopSimilarity,
		"answer_overlap":   trace.AnswerOverlap,
		"product_ids":      trace.ProductIDs,
		"knowledge_ids":    trace.KnowledgeIDs,
		"grounding_retried":  trace.GroundingRetried,
		"grounding_fallback": trace.GroundingFallback,
	})
}

// AgentAnalytics mengembalikan ringkasan + tren 7 hari untuk satu agent.
func AgentAnalytics(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var totalIn, aiReplies, humanReplies, contacts, openHandoffs int64
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND message <> ''", id).Count(&totalIn)
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND reply <> '' AND from_human = ?", id, false).Count(&aiReplies)
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND from_human = ?", id, true).Count(&humanReplies)
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ?", id).Distinct("sender").Count(&contacts)
	database.DB.Model(&models.Handoff{}).Where("agent_id = ?", id).Count(&openHandoffs)

	pct := 0
	if totalIn > 0 {
		pct = int(aiReplies * 100 / totalIn)
	}

	// Tren pesan masuk 7 hari terakhir.
	type dayRow struct {
		Day string
		Cnt int
	}
	var rows []dayRow
	since := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	database.DB.Model(&models.ChatHistory{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') as day, COUNT(*) as cnt").
		Where("agent_id = ? AND message <> '' AND created_at >= ?", id, since+" 00:00:00").
		Group("day").Scan(&rows)
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.Day] = r.Cnt
	}
	trend := make([]gin.H, 0, 7)
	for i := 6; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		trend = append(trend, gin.H{"day": d, "count": counts[d]})
	}

	c.JSON(200, gin.H{
		"total_incoming": totalIn,
		"ai_replies":     aiReplies,
		"human_replies":  humanReplies,
		"contacts":       contacts,
		"open_handoffs":  openHandoffs,
		"ai_handled_pct": pct,
		"trend":          trend,
	})
}

// InboxContacts = daftar kontak (diurutkan dari yang terbaru) + penanda butuh manusia.
// Query memakai MAX(id) (lebih murah & stabil daripada MAX(created_at)+join ganda)
// dan hanya memuat nama/handoff/pause untuk sender yang muncul di halaman ini.
func InboxContacts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	type contactRow struct {
		Sender  string    `json:"sender"`
		LastAt  time.Time `json:"last_at"`
		LastMsg string    `json:"last_msg"`
	}
	var rows []contactRow
	// MAX(id) = pesan terbaru per sender (id mono naik). Hindari join created_at yang bisa
	// dobel baris saat timestamp sama.
	database.DB.Raw(`
		SELECT ch.sender, ch.created_at AS last_at,
			CASE
				WHEN TRIM(COALESCE(ch.message, '')) != '' THEN ch.message
				WHEN TRIM(COALESCE(ch.reply, '')) != '' THEN ch.reply
				WHEN ch.media_type != '' THEN CONCAT('[', ch.media_type, ']')
				ELSE ''
			END AS last_msg
		FROM chat_histories ch
		INNER JOIN (
			SELECT MAX(id) AS max_id
			FROM chat_histories
			WHERE agent_id = ?
			GROUP BY sender
		) latest ON ch.id = latest.max_id
		WHERE ch.agent_id = ?
		ORDER BY ch.id DESC
		LIMIT 500`, id, id).Scan(&rows)

	senders := make([]string, 0, len(rows))
	for _, r := range rows {
		senders = append(senders, r.Sender)
	}

	needsHuman := map[string]bool{}
	if len(senders) > 0 {
		var hs []models.Handoff
		database.DB.Select("sender").Where("agent_id = ? AND sender IN ?", id, senders).Find(&hs)
		for _, h := range hs {
			needsHuman[h.Sender] = true
		}
	}

	names := map[string]string{}
	pauses := map[string]*time.Time{}
	if len(senders) > 0 {
		var cs []models.Contact
		database.DB.Select("number", "name", "manual_pause_until").
			Where("agent_id = ? AND number IN ?", id, senders).Find(&cs)
		now := time.Now()
		for i := range cs {
			if cs[i].Name != "" {
				names[cs[i].Number] = cs[i].Name
			}
			if cs[i].ManualPauseUntil != nil && cs[i].ManualPauseUntil.After(now) {
				pauses[cs[i].Number] = cs[i].ManualPauseUntil
			}
		}
	}

	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		msg := strings.TrimSpace(r.LastMsg)
		// Normalisasi whitespace preview agar list tidak "melebar".
		msg = strings.Join(strings.Fields(msg), " ")
		if len([]rune(msg)) > 64 {
			msg = string([]rune(msg)[:64]) + "…"
		}
		out = append(out, gin.H{
			"sender": r.Sender, "last_at": r.LastAt, "last_msg": msg,
			"needs_human": needsHuman[r.Sender], "manual_pause_until": pauses[r.Sender],
			"name": names[r.Sender],
		})
	}
	c.JSON(200, gin.H{"data": out})
}

// DeleteInboxConversation menghapus riwayat chat satu kontak dari inbox agent.
// Menghapus: chat_histories, handoff, conversation_memory, ai_turns untuk sender itu.
// Tidak menghapus data CRM (contacts) — hanya menghilangkan thread di Inbox.
func DeleteInboxConversation(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := strings.TrimSpace(c.Query("sender"))
	if sender == "" {
		sender = strings.TrimSpace(c.Param("sender"))
	}
	if sender == "" {
		c.JSON(400, gin.H{"error": "sender wajib"})
		return
	}

	// Ambil path media dulu (opsional hapus file di disk).
	var mediaPaths []string
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ? AND media_path != '' AND media_path IS NOT NULL", id, sender).
		Pluck("media_path", &mediaPaths)

	tx := database.DB.Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"error": "Gagal memulai penghapusan chat"})
		return
	}
	res := tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.ChatHistory{})
	if res.Error != nil {
		tx.Rollback()
		c.JSON(500, gin.H{"error": "Gagal menghapus riwayat chat"})
		return
	}
	_ = tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.Handoff{}).Error
	_ = tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.ConversationMemory{}).Error
	_ = tx.Where("agent_id = ? AND sender = ?", id, sender).Delete(&models.AITurn{}).Error
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyelesaikan penghapusan chat"})
		return
	}

	// Best-effort hapus file media di disk (abaikan error).
	seen := map[string]bool{}
	for _, p := range mediaPaths {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		_ = os.Remove(p)
	}

	c.JSON(200, gin.H{
		"message":        "Chat dihapus dari inbox",
		"sender":         sender,
		"deleted_chats":  res.RowsAffected,
		"deleted_media":  len(seen),
	})
}

// InboxConversation = seluruh percakapan dengan satu kontak.
// Mendukung cursor pagination via query param `before_id` (ID pesan tertua yang sudah ditampilkan).
// Response menyertakan `has_more: true` bila masih ada pesan lebih lama.
func InboxConversation(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := c.Query("sender")
	if sender == "" {
		c.JSON(400, gin.H{"error": "sender wajib"})
		return
	}

	limit := 300 // default: tampung banyak percakapan
	if l, err := strconv.Atoi(c.DefaultQuery("limit", "300")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}

	query := database.DB.Where("agent_id = ? AND sender = ?", id, sender).Order("created_at asc")

	// Cursor pagination: jika ada before_id, ambil pesan SEBELUM ID tersebut
	if beforeID := c.Query("before_id"); beforeID != "" {
		if bid, err := strconv.Atoi(beforeID); err == nil && bid > 0 {
			query = query.Where("id < ?", bid)
		}
	}

	// Ambil 1 extra untuk deteksi has_more
	var msgs []models.ChatHistory
	query.Limit(limit + 1).Find(&msgs)

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit] // kembalikan tepat `limit`, sisakan 1 sebagai sinyal
	}

	// Balik urutan: newest-first agar frontend bisa append older messages di atas
	// Tapi data asli dari DB sudah asc (lama→baru), biarkan saja — frontend yang atur tampilan

	var h int64
	database.DB.Model(&models.Handoff{}).Where("agent_id = ? AND sender = ?", id, sender).Count(&h)
	var contact models.Contact
	database.DB.Select("manual_pause_until").Where("agent_id = ? AND number = ?", id, sender).First(&contact)
	var pauseUntil *time.Time
	if contact.ManualPauseUntil != nil && contact.ManualPauseUntil.After(time.Now()) {
		pauseUntil = contact.ManualPauseUntil
	}

	// Hitung total chat untuk info
	var totalCount int64
	database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND sender = ?", id, sender).Count(&totalCount)

	c.JSON(200, gin.H{
		"data":               msgs,
		"needs_human":        h > 0,
		"manual_pause_until": pauseUntil,
		"media_token":        issueMediaToken(currentTenantID(c)),
		"has_more":           hasMore,
		"total":              totalCount,
	})
}

// ReanalyzeInboxImage menjalankan ulang vision pada media yang sudah tersimpan.
// Instruksi CS hanya memengaruhi analisis ini dan tidak dikirim sebagai pesan pelanggan.
func ReanalyzeInboxImage(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Instruction string `json:"instruction"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format instruksi tidak valid"})
		return
	}
	req.Instruction = strings.TrimSpace(req.Instruction)
	if len([]rune(req.Instruction)) > 800 {
		c.JSON(400, gin.H{"error": "Instruksi maksimal 800 karakter"})
		return
	}
	var row models.ChatHistory
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("cid"), id).First(&row).Error != nil {
		c.JSON(404, gin.H{"error": "Pesan gambar tidak ditemukan"})
		return
	}
	if row.MediaPath == "" || (row.MediaType != "image" && row.MediaType != "sticker") {
		c.JSON(400, gin.H{"error": "Pesan ini bukan gambar yang dapat dianalisis"})
		return
	}
	data, err := os.ReadFile(row.MediaPath)
	if err != nil {
		c.JSON(404, gin.H{"error": "File gambar sudah tidak tersedia"})
		return
	}
	var agent models.Agent
	if database.DB.First(&agent, id).Error != nil {
		c.JSON(404, gin.H{"error": "Nomor tidak ditemukan"})
		return
	}
	prompt := strings.TrimSpace(agent.SystemPrompt)
	if prompt == "" {
		prompt = "Kamu adalah asisten AI bisnis yang ramah dan akurat."
	}
	tone := agent.Tone
	if tone == "" {
		tone = "ramah"
	}
	var history []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ? AND id < ?", id, row.Sender, row.ID).Order("id desc").Limit(12).Find(&history)
	for left, right := 0, len(history)-1; left < right; left, right = left+1, right-1 {
		history[left], history[right] = history[right], history[left]
	}
	result, err := services.AnalyzeCustomerImage(id, prompt, tone, row.Message, req.Instruction, row.Mimetype, data, history)
	if err != nil {
		failureText := "Analisis ulang gagal: " + err.Error()
		database.DB.Model(&row).Updates(map[string]any{
			"image_analysis_status": "failed", "image_analysis": failureText,
			"image_analysis_model": "", "image_analysis_confidence": 0,
			"image_analysis_answer": "", "image_analysis_product_id": 0,
			"image_analysis_needs_human": true,
		})
		name := contactNames(id)[row.Sender]
		dispatchStoredImageAnalysisWebhook(id, row, name, "failed", services.VisionAnalysisResult{Analysis: failureText}, true)
		c.JSON(502, gin.H{"error": "Analisis ulang gagal: " + err.Error()})
		return
	}
	needsHuman := result.NeedsHuman || result.Confidence < 0.55
	database.DB.Model(&row).Updates(map[string]any{
		"image_analysis": result.Analysis, "image_analysis_status": "completed",
		"image_analysis_model": result.Model, "image_analysis_confidence": result.Confidence,
		"image_analysis_answer": result.Answer, "image_analysis_product_id": result.ProductID,
		"image_analysis_needs_human": needsHuman,
	})
	if needsHuman {
		_ = database.DB.Where(models.Handoff{AgentID: id, Sender: row.Sender}).
			Assign(models.Handoff{LastMsg: row.Message}).FirstOrCreate(&models.Handoff{}).Error
	}
	name := contactNames(id)[row.Sender]
	dispatchStoredImageAnalysisWebhook(id, row, name, "completed", result, needsHuman)
	c.JSON(200, gin.H{"data": gin.H{
		"image_analysis": result.Analysis, "image_analysis_status": "completed",
		"image_analysis_model": result.Model, "image_analysis_confidence": result.Confidence,
		"image_analysis_answer": result.Answer, "image_analysis_product_id": result.ProductID,
		"image_analysis_needs_human": needsHuman,
	}})
}

// InboxSend mengirim pesan manual dari dashboard ke kontak (ambil alih dari bot).
func InboxSend(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		To        string `json:"to"`
		Message   string `json:"message"`
		ReplyTo   string `json:"reply_to"`
		ReplyText string `json:"reply_text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.To == "" || strings.TrimSpace(req.Message) == "" {
		c.JSON(400, gin.H{"error": "Nomor & pesan wajib diisi"})
		return
	}
	// Jeda dipasang sebelum proses kirim (yang menampilkan indikator mengetik), agar
	// balasan AI yang sedang menunggu langsung dibatalkan dan tidak beradu dengan CS.
	pauseAIForManualReply(id, req.To)
	var err error
	var waMsgID string
	if req.ReplyTo != "" {
		err = services.WA(id).SendReply(req.To, req.Message, req.ReplyTo)
	} else {
		waMsgID, err = services.WA(id).SendTextAndGetID(req.To, req.Message)
	}
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	logTurn(id, req.To, "", req.Message, true, req.ReplyTo, req.ReplyText)
	// Simpan WA message ID untuk keperluan revoke nanti
	if waMsgID != "" {
		_ = database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ? AND reply = ?", id, req.To, req.Message).
			Order("id desc").Limit(1).Update("wa_msg_id", waMsgID).Error
	}

	c.JSON(200, gin.H{"ok": true})
}

// ChatPresence mengirim indikator "mengetik" ke kontak (dipanggil dari Inbox saat CS mengetik).
func ChatPresence(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		To     string `json:"to"`
		Active bool   `json:"active"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.To == "" {
		c.JSON(400, gin.H{"error": "to wajib"})
		return
	}
	_ = services.WA(id).Typing(req.To, req.Active)
	c.JSON(200, gin.H{"ok": true})
}

// RevokeMessage menghapus (unsend) pesan yang sudah dikirim.
func RevokeMessage(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	msgID := c.Param("msgId")
	if msgID == "" {
		c.JSON(400, gin.H{"error": "msgId wajib"})
		return
	}
	var req struct {
		To string `json:"to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.To == "" {
		c.JSON(400, gin.H{"error": "to wajib"})
		return
	}
	if err := services.WA(id).RevokeMessage(req.To, types.MessageID(msgID)); err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	// Tandai pesan sebagai revoked di DB (tampilkan "Pesan ini dihapus" di Inbox)
	_ = database.DB.Model(&models.ChatHistory{}).Where("wa_msg_id = ?", msgID).Update("revoked", true).Error
	c.JSON(200, gin.H{"ok": true})
}

// InboxSendMedia mengirim gambar/file dari dashboard ke kontak (ambil alih dari bot).
func InboxSendMedia(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	to := c.PostForm("to")
	caption := c.PostForm("caption")
	if to == "" {
		c.JSON(400, gin.H{"error": "Nomor tujuan wajib"})
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "File wajib diunggah"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": "Gagal membaca file"})
		return
	}
	defer f.Close()
	data, _ := io.ReadAll(f)

	mimetype := fh.Header.Get("Content-Type")
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	var sendErr error
	switch {
	case strings.HasPrefix(mimetype, "image/"):
		sendErr = services.WA(id).SendImage(to, caption, mimetype, data)
	case strings.HasPrefix(mimetype, "video/"):
		sendErr = services.WA(id).SendVideo(to, caption, mimetype, data)
	default:
		sendErr = services.WA(id).SendDocument(to, fh.Filename, mimetype, caption, data)
	}
	if sendErr != nil {
		c.JSON(502, gin.H{"error": sendErr.Error()})
		return
	}

	mediaType := "document"
	if strings.HasPrefix(mimetype, "image/") {
		mediaType = "image"
	} else if strings.HasPrefix(mimetype, "video/") {
		mediaType = "video"
	}
	reply := caption
	if reply == "" {
		reply = mediaPlaceholder(mediaType, fh.Filename)
	}
	_ = database.DB.Create(&models.ChatHistory{
		AgentID: id, Sender: to, Reply: reply, FromHuman: true,
		MediaType: mediaType, MediaPath: storeMedia(id, data, mimetype, fh.Filename),
		FileName: fh.Filename, Mimetype: mimetype,
	}).Error

	// Real-time learning: balasan CS manusia baru = materi belajar terbaru.
	services.MaybeTriggerIncrementalLearning(id)

	var cnt int64
	database.DB.Model(&models.Handoff{}).Where("agent_id = ? AND sender = ?", id, to).Count(&cnt)
	if cnt == 0 {
		_ = database.DB.Create(&models.Handoff{AgentID: id, Sender: to, LastMsg: reply}).Error
	}
	c.JSON(200, gin.H{"ok": true})
}

// ServeMedia menyajikan file media sebuah pesan. Auth lewat ?token= (header tak bisa di <img>/<a>).
func ServeMedia(c *gin.Context) {
	tid, ok := tenantFromToken(c.Query("token"))
	if !ok {
		c.AbortWithStatus(401)
		return
	}
	agentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	var agent models.Agent
	if database.DB.Select("id").Where("id = ? AND tenant_id = ?", agentID, tid).First(&agent).Error != nil {
		c.AbortWithStatus(404)
		return
	}
	var ch models.ChatHistory
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("cid"), agentID).First(&ch).Error != nil || ch.MediaPath == "" {
		c.AbortWithStatus(404)
		return
	}
	c.File(ch.MediaPath)
}
