package handlers

import (
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
)

// InboxUnreadSummary — GET /agents/:id/inbox/unread-summary
// Ringkasan belum-dibaca memakai InboxReadState sebagai SATU-SATUNYA sumber
// kebenaran (konsolidasi A6). ConversationRead tidak lagi dibaca di sini;
// tabelnya dipertahankan sebagai legacy (data lama tidak dihapus).
func InboxUnreadSummary(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var rows []struct {
		Sender string `gorm:"column:sender"`
		Unread int    `gorm:"column:unread"`
	}
	database.DB.Model(&models.InboxReadState{}).
		Select("sender, whats_app_unread_count AS unread").
		Where("agent_id = ? AND whats_app_unread_count > 0", id).
		Order("sender").
		Scan(&rows)

	total := 0
	senders := make([]string, 0, len(rows))
	for _, row := range rows {
		total += row.Unread
		senders = append(senders, row.Sender)
	}
	c.JSON(200, gin.H{"total": total, "senders": senders})
}
