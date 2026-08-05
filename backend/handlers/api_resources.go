package handlers

import (
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// Endpoint REST API "resource" (grup, kontak, riwayat, media) — auth API key per-nomor.

type apiPage struct {
	Page    int
	PerPage int
	Offset  int
}

func parseAPIPage(c *gin.Context, defaultPerPage, maxPerPage int) apiPage {
	page := 1
	if n, err := strconv.Atoi(c.Query("page")); err == nil && n > 0 {
		page = n
	}
	perPage := defaultPerPage
	value := c.Query("per_page")
	if value == "" {
		value = c.Query("limit") // kompatibilitas endpoint versi awal
	}
	if n, err := strconv.Atoi(value); err == nil && n > 0 {
		perPage = n
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return apiPage{Page: page, PerPage: perPage, Offset: (page - 1) * perPage}
}

func apiPageMeta(p apiPage, total int64) gin.H {
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(p.PerPage) - 1) / int64(p.PerPage))
	}
	return gin.H{"page": p.Page, "per_page": p.PerPage, "total": total, "total_pages": totalPages}
}

// APIListGroups — GET /api/v1/groups
func APIListGroups(c *gin.Context) {
	agent := apiAgent(c)
	if !services.WA(agent.ID).IsConnected() {
		c.JSON(409, gin.H{"error": "Nomor WhatsApp sedang tidak tersambung."})
		return
	}
	groups, err := services.WA(agent.ID).GetGroups()
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal mengambil grup: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": groups})
}

// APIGroupSendMessage — POST /api/v1/groups/:jid/messages (body sama seperti /messages)
func APIGroupSendMessage(c *gin.Context) {
	agent := apiAgent(c)
	jid := c.Param("jid")
	if !services.IsGroupJID(jid) {
		c.JSON(400, gin.H{"error": "JID grup tidak valid (harus diakhiri @g.us)."})
		return
	}
	var req apiMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Body JSON tidak valid."})
		return
	}
	msgID, code, errMsg := deliverAPIMessage(agent.ID, jid, req)
	if errMsg != "" {
		c.JSON(code, gin.H{"error": errMsg})
		return
	}
	c.JSON(200, gin.H{"status": "sent", "to": jid, "type": respType(req.Type), "message_id": msgID})
}

// APIListContacts — GET /api/v1/contacts?q=&page=&per_page=
func APIListContacts(c *gin.Context) {
	agent := apiAgent(c)
	q := database.DB.Where("agent_id = ?", agent.ID)
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		like := "%" + s + "%"
		q = q.Where("name LIKE ? OR number LIKE ?", like, like)
	}
	if tags := strings.TrimSpace(c.Query("tags")); tags != "" {
		q = q.Where("tags LIKE ?", "%"+tags+"%")
	}
	p := parseAPIPage(c, 50, 500)
	var total int64
	q.Model(&models.Contact{}).Count(&total)
	var rows []models.Contact
	q.Order("updated_at desc").Offset(p.Offset).Limit(p.PerPage).Find(&rows)
	c.JSON(200, gin.H{"data": rows, "meta": apiPageMeta(p, total)})
}

// APISaveContact — POST /api/v1/contacts  Body: {"number","name","notes","tags"} (upsert per nomor)
func APISaveContact(c *gin.Context) {
	agent := apiAgent(c)
	var req struct {
		Number    string  `json:"number"`
		Name      string  `json:"name"`
		Notes     string  `json:"notes"`
		Tags      string  `json:"tags"`
		LeadStage *string `json:"lead_stage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	num := services.NormalizePhone(req.Number)
	if num == "" {
		c.JSON(400, gin.H{"error": "Field 'number' wajib diisi."})
		return
	}
	assign := map[string]any{"name": strings.TrimSpace(req.Name), "notes": req.Notes, "tags": strings.TrimSpace(req.Tags)}
	if req.LeadStage != nil {
		stage, valid := normalizeLeadStage(*req.LeadStage)
		if !valid {
			c.JSON(400, gin.H{"error": "lead_stage harus salah satu: new, cold, warm, hot, customer, unqualified."})
			return
		}
		assign["lead_stage"] = stage
		assign["lead_stage_source"] = "manual"
		assign["lead_stage_reason"] = "Status diperbarui melalui REST API"
		assign["lead_stage_confidence"] = 1
		assign["lead_stage_locked"] = true
		assign["lead_stage_updated_at"] = time.Now()
	}
	var ct models.Contact
	result := database.DB.Where(models.Contact{AgentID: agent.ID, Number: num}).
		Attrs(models.Contact{LeadStage: leadStageNew, LeadStageSource: "system", LeadStageReason: "Kontak dibuat melalui REST API"}).
		Assign(assign).FirstOrCreate(&ct)
	if result.Error != nil {
		c.JSON(500, gin.H{"error": "Kontak belum bisa disimpan."})
		return
	}
	c.JSON(200, gin.H{"data": ct})
}

func APIGetContact(c *gin.Context) {
	agent := apiAgent(c)
	num := services.NormalizePhone(c.Param("number"))
	var contact models.Contact
	if num == "" || database.DB.Where("agent_id = ? AND number = ?", agent.ID, num).First(&contact).Error != nil {
		c.JSON(404, gin.H{"error": "Kontak tidak ditemukan."})
		return
	}
	c.JSON(200, gin.H{"data": contact})
}

func APIUpdateContact(c *gin.Context) {
	agent := apiAgent(c)
	num := services.NormalizePhone(c.Param("number"))
	var contact models.Contact
	if num == "" || database.DB.Where("agent_id = ? AND number = ?", agent.ID, num).First(&contact).Error != nil {
		c.JSON(404, gin.H{"error": "Kontak tidak ditemukan."})
		return
	}
	var req struct {
		Name      *string `json:"name"`
		Notes     *string `json:"notes"`
		Tags      *string `json:"tags"`
		LeadStage *string `json:"lead_stage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid."})
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = strings.TrimSpace(*req.Name)
	}
	if req.Notes != nil {
		updates["notes"] = *req.Notes
	}
	if req.Tags != nil {
		updates["tags"] = strings.TrimSpace(*req.Tags)
	}
	if req.LeadStage != nil {
		stage, valid := normalizeLeadStage(*req.LeadStage)
		if !valid {
			c.JSON(400, gin.H{"error": "lead_stage harus salah satu: new, cold, warm, hot, customer, unqualified."})
			return
		}
		updates["lead_stage"] = stage
		updates["lead_stage_source"] = "manual"
		updates["lead_stage_reason"] = "Status diperbarui melalui REST API"
		updates["lead_stage_confidence"] = 1
		updates["lead_stage_locked"] = true
		updates["lead_stage_updated_at"] = time.Now()
	}
	if len(updates) == 0 {
		c.JSON(400, gin.H{"error": "Isi minimal satu field: name, notes, tags, atau lead_stage."})
		return
	}
	if err := database.DB.Model(&contact).Updates(updates).Error; err != nil {
		c.JSON(500, gin.H{"error": "Kontak belum bisa diperbarui."})
		return
	}
	database.DB.First(&contact, contact.ID)
	c.JSON(200, gin.H{"data": contact})
}

func APIDeleteContact(c *gin.Context) {
	agent := apiAgent(c)
	num := services.NormalizePhone(c.Param("number"))
	if num == "" {
		c.JSON(400, gin.H{"error": "Nomor tidak valid."})
		return
	}
	result := database.DB.Where("agent_id = ? AND number = ?", agent.ID, num).Delete(&models.Contact{})
	if result.Error != nil {
		c.JSON(500, gin.H{"error": "Kontak belum bisa dihapus."})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "Kontak tidak ditemukan."})
		return
	}
	c.JSON(200, gin.H{"status": "deleted", "number": num})
}

type apiChatRow struct {
	Sender      string    `json:"number"`
	Name        string    `json:"name"`
	LastAt      time.Time `json:"last_at"`
	LastMessage string    `json:"last_message"`
	NeedsHuman  bool      `json:"needs_human"`
}

// APIListChats — GET /api/v1/chats?q=&page=&per_page=
func APIListChats(c *gin.Context) {
	agent := apiAgent(c)
	p := parseAPIPage(c, 50, 200)
	search := strings.TrimSpace(c.Query("q"))
	base := database.DB.Model(&models.ChatHistory{}).Where("agent_id = ?", agent.ID)
	if search != "" {
		base = base.Where("sender LIKE ?", "%"+search+"%")
	}
	var total int64
	base.Distinct("sender").Count(&total)

	type latestRow struct {
		Sender      string
		LastAt      time.Time
		LastMessage string
	}
	var latest []latestRow
	query := `
		SELECT ch.sender, ch.created_at AS last_at,
			CASE WHEN ch.message <> '' THEN ch.message ELSE ch.reply END AS last_message
		FROM chat_histories ch
		INNER JOIN (
			SELECT sender, MAX(id) AS max_id FROM chat_histories
			WHERE agent_id = ? AND (? = '' OR sender LIKE ?)
			GROUP BY sender
		) x ON x.max_id = ch.id
		ORDER BY ch.id DESC LIMIT ? OFFSET ?`
	database.DB.Raw(query, agent.ID, search, "%"+search+"%", p.PerPage, p.Offset).Scan(&latest)

	numbers := make([]string, 0, len(latest))
	for _, row := range latest {
		numbers = append(numbers, row.Sender)
	}
	names := map[string]string{}
	needsHuman := map[string]bool{}
	if len(numbers) > 0 {
		var contacts []models.Contact
		database.DB.Where("agent_id = ? AND number IN ?", agent.ID, numbers).Find(&contacts)
		for _, contact := range contacts {
			names[contact.Number] = contact.Name
		}
		var handoffs []models.Handoff
		database.DB.Where("agent_id = ? AND sender IN ?", agent.ID, numbers).Find(&handoffs)
		for _, handoff := range handoffs {
			needsHuman[handoff.Sender] = true
		}
	}
	rows := make([]apiChatRow, 0, len(latest))
	for _, row := range latest {
		rows = append(rows, apiChatRow{Sender: row.Sender, Name: names[row.Sender], LastAt: row.LastAt, LastMessage: row.LastMessage, NeedsHuman: needsHuman[row.Sender]})
	}
	c.JSON(200, gin.H{"data": rows, "meta": apiPageMeta(p, total)})
}

// APIChatMessages — GET /api/v1/chats/:number/messages?limit=  (riwayat terbaru dulu)
func APIChatMessages(c *gin.Context) {
	agent := apiAgent(c)
	num := services.NormalizePhone(c.Param("number"))
	if num == "" {
		c.JSON(400, gin.H{"error": "Nomor tidak valid."})
		return
	}
	p := parseAPIPage(c, 50, 200)
	q := database.DB.Where("agent_id = ? AND sender = ?", agent.ID, num)
	var total int64
	q.Model(&models.ChatHistory{}).Count(&total)
	var rows []models.ChatHistory
	q.Order("id desc").Offset(p.Offset).Limit(p.PerPage).Find(&rows)
	c.JSON(200, gin.H{"data": rows, "meta": apiPageMeta(p, total)})
}

// APIServeMessageMedia memakai message_id dari event webhook, sehingga integrasi tidak perlu tahu ID internal chat.
func APIServeMessageMedia(c *gin.Context) {
	agent := apiAgent(c)
	messageID := strings.TrimSpace(c.Param("message_id"))
	var ch models.ChatHistory
	if messageID == "" || database.DB.Where("wa_msg_id = ? AND agent_id = ?", messageID, agent.ID).First(&ch).Error != nil || ch.MediaPath == "" {
		c.JSON(404, gin.H{"error": "Media pesan tidak ditemukan."})
		return
	}
	c.File(ch.MediaPath)
}

// APIMessageAnalysis — GET /api/v1/messages/:message_id/analysis
func APIMessageAnalysis(c *gin.Context) {
	agent := apiAgent(c)
	messageID := strings.TrimSpace(c.Param("message_id"))
	var row models.ChatHistory
	if messageID == "" || database.DB.Where("wa_msg_id = ? AND agent_id = ?", messageID, agent.ID).First(&row).Error != nil {
		c.JSON(404, gin.H{"error": "Pesan tidak ditemukan."})
		return
	}
	if row.MediaType != "image" && row.MediaType != "sticker" {
		c.JSON(400, gin.H{"error": "Pesan ini bukan gambar."})
		return
	}
	status := row.ImageAnalysisStatus
	if status == "" {
		status = "pending"
	}
	c.JSON(200, gin.H{"data": gin.H{
		"message_id": row.WAMsgID, "from": row.Sender, "status": status,
		"analysis": row.ImageAnalysis, "confidence": row.ImageAnalysisConfidence,
		"model": row.ImageAnalysisModel, "answer": row.ImageAnalysisAnswer,
		"product_id": row.ImageAnalysisProductID, "needs_human": row.ImageAnalysisNeedsHuman,
	}})
}

// APIServeMedia — GET /api/v1/media/:cid  (unduh file media dari sebuah pesan riwayat milik nomor ini)
func APIServeMedia(c *gin.Context) {
	agent := apiAgent(c)
	var ch models.ChatHistory
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("cid"), agent.ID).First(&ch).Error != nil || ch.MediaPath == "" {
		c.AbortWithStatus(404)
		return
	}
	c.File(ch.MediaPath)
}
