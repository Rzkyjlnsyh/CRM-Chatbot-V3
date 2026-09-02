package handlers

import (
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
)

// InboxUnreadSummary — GET /agents/:id/inbox/unread-summary
// Ringkasan belum-dibaca per agent memakai ConversationRead (mark-read fork
// yang sudah ada — TIDAK dibuat tabel read-state baru agar tidak dobel).
func InboxUnreadSummary(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	type unreadRow struct {
		Sender         string `json:"sender"`
		LastReadChatID uint   `json:"last_read_chat_id"`
	}
	var reads []unreadRow
	database.DB.Model(&models.ConversationRead{}).
		Where("agent_id = ?", id).
		Order("sender").
		Scan(&reads)

	// Unread per sender = jumlah pesan MASUK (from_human=false) dengan
	// id > last_read_chat_id. Sender tanpa baris ConversationRead dianggap
	// semua belum dibaca.
	readMap := map[string]uint{}
	for _, r := range reads {
		readMap[r.Sender] = r.LastReadChatID
	}
	var recent []struct {
		Sender string
		Total  int64
	}
	database.DB.Model(&models.ChatHistory{}).
		Select("sender, COUNT(*) AS total").
		Where("agent_id = ? AND from_human = ?", id, false).
		Group("sender").Scan(&recent)

	total := 0
	senders := []string{}
	for _, row := range recent {
		lastRead, ok := readMap[row.Sender]
		if !ok {
			total += int(row.Total)
			senders = append(senders, row.Sender)
			continue
		}
		var unread int64
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ? AND from_human = ? AND id > ?", id, row.Sender, false, lastRead).
			Count(&unread)
		if unread > 0 {
			total += int(unread)
			senders = append(senders, row.Sender)
		}
	}
	c.JSON(200, gin.H{"total": total, "senders": senders})
}
