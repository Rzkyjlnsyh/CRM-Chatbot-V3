package handlers

import (
	"strings"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
)

// matchAutoReply mencari aturan auto-reply yang cocok dengan teks masuk.
// Mengembalikan balasan & true bila ada yang cocok (aturan urutan teratas menang).
func matchAutoReply(agentID uint, text string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(text))
	if t == "" {
		return "", false
	}
	var rules []models.AutoReply
	database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).Order("sort_order asc, id asc").Find(&rules)
	for _, r := range rules {
		for _, kw := range strings.Split(r.Keywords, ",") {
			kw = strings.ToLower(strings.TrimSpace(kw))
			if kw == "" {
				continue
			}
			var matched bool
			switch r.MatchType {
			case "exact":
				matched = t == kw
			case "prefix":
				matched = strings.HasPrefix(t, kw)
			default: // contains
				matched = strings.Contains(t, kw)
			}
			if matched {
				return r.Reply, true
			}
		}
	}
	return "", false
}

func ListAutoReplies(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var rules []models.AutoReply
	database.DB.Where("agent_id = ?", id).Order("sort_order asc, id asc").Find(&rules)
	c.JSON(200, gin.H{"data": rules})
}

func CreateAutoReply(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Keywords  string `json:"keywords"`
		MatchType string `json:"match_type"`
		Reply     string `json:"reply"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if strings.TrimSpace(req.Keywords) == "" || strings.TrimSpace(req.Reply) == "" {
		c.JSON(400, gin.H{"error": "Kata kunci & balasan wajib diisi"})
		return
	}
	mt := req.MatchType
	if mt == "" {
		mt = "contains"
	}
	r := models.AutoReply{AgentID: id, Keywords: req.Keywords, MatchType: mt, Reply: req.Reply, Enabled: true, SortOrder: req.SortOrder}
	if err := database.DB.Create(&r).Error; err != nil { c.JSON(500, gin.H{"error": "Gagal"}); return }
	c.JSON(201, gin.H{"data": r})
}

func UpdateAutoReply(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var r models.AutoReply
	if database.DB.Where("agent_id = ?", id).First(&r, c.Param("rid")).Error != nil {
		c.JSON(404, gin.H{"error": "Aturan tidak ditemukan"})
		return
	}
	var req struct {
		Keywords  *string `json:"keywords"`
		MatchType *string `json:"match_type"`
		Reply     *string `json:"reply"`
		Enabled   *bool   `json:"enabled"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if req.Keywords != nil {
		r.Keywords = *req.Keywords
	}
	if req.MatchType != nil {
		r.MatchType = *req.MatchType
	}
	if req.Reply != nil {
		r.Reply = *req.Reply
	}
	if req.Enabled != nil {
		r.Enabled = *req.Enabled
	}
	if req.SortOrder != nil {
		r.SortOrder = *req.SortOrder
	}
	_ = database.DB.Save(&r).Error
	c.JSON(200, gin.H{"data": r})
}

func DeleteAutoReply(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	_ = database.DB.Where("agent_id = ?", id).Delete(&models.AutoReply{}, c.Param("rid")).Error
	c.JSON(200, gin.H{"message": "Deleted"})
}
