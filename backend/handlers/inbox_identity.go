package handlers

import (
	"os"
	"strings"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// inbox_identity — normalisasi identitas nomor WA & reset state Inbox saat
// akun WhatsApp yang terhubung berganti (pola v4 chatloop-1.6-1.7).

const resetInboxConfirmation = "RESET INBOX"

type inboxResetResult struct {
	DeletedChats int64 `json:"deleted_chats"`
	DeletedMedia int   `json:"deleted_media"`
}

func normalizedWhatsAppAccount(number string) string {
	return services.NormalizePhone(strings.TrimSpace(number))
}

func whatsappAccountChanged(previous, current string) bool {
	previous = normalizedWhatsAppAccount(previous)
	current = normalizedWhatsAppAccount(current)
	return previous != "" && current != "" && previous != current
}

// resetAgentInboxData membersihkan state yang berasal dari akun WhatsApp lama.
// Data konfigurasi agent dan kontak CRM sengaja dipertahankan; chat lama
// (dari akun lama) dihapus agar tidak bercampur dengan nomor baru.
func resetAgentInboxData(agentID uint) (inboxResetResult, error) {
	var result inboxResetResult
	var mediaPaths []string
	if err := database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND media_path != '' AND media_path IS NOT NULL", agentID).
		Pluck("media_path", &mediaPaths).Error; err != nil {
		return result, err
	}

	tx := database.DB.Begin()
	if tx.Error != nil {
		return result, tx.Error
	}
	rollback := func(err error) (inboxResetResult, error) {
		tx.Rollback()
		return result, err
	}

	deleted := tx.Where("agent_id = ?", agentID).Delete(&models.ChatHistory{})
	if deleted.Error != nil {
		return rollback(deleted.Error)
	}
	result.DeletedChats = deleted.RowsAffected

	for _, model := range []any{
		&models.InboxReadState{},
		&models.ConversationRead{},
		&models.Handoff{},
		&models.ConversationMemory{},
		&models.AITurn{},
		&models.FlowSession{},
		&models.AIFormSession{},
		&models.ProductCheckoutSession{},
		&models.ChatLabel{},
		&models.Label{},
	} {
		if err := tx.Where("agent_id = ?", agentID).Delete(model).Error; err != nil {
			return rollback(err)
		}
	}
	if err := tx.Commit().Error; err != nil {
		return result, err
	}

	seen := make(map[string]struct{}, len(mediaPaths))
	for _, mediaPath := range mediaPaths {
		mediaPath = strings.TrimSpace(mediaPath)
		if mediaPath == "" {
			continue
		}
		if _, exists := seen[mediaPath]; exists {
			continue
		}
		seen[mediaPath] = struct{}{}
		if os.Remove(mediaPath) == nil {
			result.DeletedMedia++
		}
	}
	return result, nil
}

// ResetAgentInbox memperbaiki state Inbox yang sudah telanjur tercampur.
// Endpoint ini TIDAK menyentuh kontak CRM maupun chat pada perangkat WhatsApp.
func ResetAgentInbox(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Confirm) != resetInboxConfirmation {
		c.JSON(400, gin.H{"error": "Konfirmasi reset Inbox tidak valid"})
		return
	}

	result, err := resetAgentInboxData(agentID)
	if err != nil {
		c.JSON(500, gin.H{"error": "Gagal mereset data Inbox"})
		return
	}

	// Jadikan nomor yang sedang terhubung sebagai pemilik state baru.
	var agent models.Agent
	if database.DB.First(&agent, agentID).Error == nil {
		if current := normalizedWhatsAppAccount(agent.Number); current != "" {
			database.DB.Model(&agent).Update("inbox_owner_number", current)
		}
	}
	publishInboxEvent(agentID, "", "reset")

	c.JSON(200, gin.H{
		"message":       "Inbox berhasil direset",
		"deleted_chats": result.DeletedChats,
		"deleted_media": result.DeletedMedia,
	})
}
