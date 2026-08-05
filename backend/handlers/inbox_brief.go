package handlers

import (
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// GetConversationBrief — GET /agents/:id/conversation/brief?sender=
// Mengembalikan ringkasan operasional untuk CS (cache bila masih segar).
func GetConversationBrief(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := strings.TrimSpace(c.Query("sender"))
	if sender == "" {
		c.JSON(400, gin.H{"error": "Parameter sender wajib"})
		return
	}
	force := c.Query("refresh") == "1" || c.Query("force") == "1"
	brief, err := loadOrBuildBrief(id, sender, force)
	if err != nil {
		c.JSON(502, gin.H{"error": "Ringkasan belum bisa dibuat: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": brief})
}

// RefreshConversationBrief — POST /agents/:id/conversation/brief  body: {"sender":"..."}
func RefreshConversationBrief(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Sender string `json:"sender"`
	}
	_ = c.ShouldBindJSON(&req)
	sender := strings.TrimSpace(req.Sender)
	if sender == "" {
		sender = strings.TrimSpace(c.Query("sender"))
	}
	if sender == "" {
		c.JSON(400, gin.H{"error": "Parameter sender wajib"})
		return
	}
	brief, err := loadOrBuildBrief(id, sender, true)
	if err != nil {
		c.JSON(502, gin.H{"error": "Ringkasan belum bisa dibuat: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": brief})
}

func loadOrBuildBrief(agentID uint, sender string, force bool) (services.ConversationBrief, error) {
	var mem models.ConversationMemory
	_ = database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).First(&mem).Error

	var last models.ChatHistory
	_ = database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).Order("id desc").First(&last).Error

	// Cache hit
	if !force {
		if cached, ok := services.DecodeBrief(mem.BriefJSON); ok && mem.BriefChatID > 0 {
			cached.Stale = last.ID > 0 && last.ID > mem.BriefChatID
			// Segar bila last chat id cocok
			if !cached.Stale {
				var ho int64
				database.DB.Model(&models.Handoff{}).Where("agent_id = ? AND sender = ?", agentID, sender).Count(&ho)
				cached.NeedsHuman = ho > 0
				return cached, nil
			}
			// Stale: tetap kembalikan cache cepat + stale=true; client bisa refresh
			// Untuk UX: auto-rebuild jika stale lebih dari 3 pesan baru
			var newCount int64
			database.DB.Model(&models.ChatHistory{}).
				Where("agent_id = ? AND sender = ? AND id > ?", agentID, sender, mem.BriefChatID).
				Count(&newCount)
			if newCount < 3 {
				var ho int64
				database.DB.Model(&models.Handoff{}).Where("agent_id = ? AND sender = ?", agentID, sender).Count(&ho)
				cached.NeedsHuman = ho > 0
				return cached, nil
			}
			// else fall through rebuild
		}
	}

	// Ambil 80 turn terakhir (id desc → reverse ke kronologis).
	var recent []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).
		Order("id desc").Limit(80).Find(&recent)
	msgs := make([]models.ChatHistory, 0, len(recent))
	for i := len(recent) - 1; i >= 0; i-- {
		msgs = append(msgs, recent[i])
	}

	var ho int64
	database.DB.Model(&models.Handoff{}).Where("agent_id = ? AND sender = ?", agentID, sender).Count(&ho)
	needsHuman := ho > 0

	brief, err := services.BuildConversationBrief(agentID, sender, msgs, mem.Summary, needsHuman, force)
	if err != nil {
		return brief, err
	}

	// Persist cache
	now := time.Now()
	mem.AgentID = agentID
	mem.Sender = sender
	mem.BriefJSON = services.EncodeBrief(brief)
	mem.BriefChatID = brief.LastChatID
	mem.BriefAt = &now
	if mem.ID == 0 {
		// FirstOrCreate path
		var existing models.ConversationMemory
		if database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).First(&existing).Error == nil {
			mem.ID = existing.ID
			mem.Summary = existing.Summary
			mem.LastChatID = existing.LastChatID
		}
	}
	_ = database.DB.Save(&mem).Error
	brief.Stale = false
	return brief, nil
}
