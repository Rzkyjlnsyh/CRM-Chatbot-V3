package handlers

import (
	"strings"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

type broadcastGuardRecipient struct {
	Number string `json:"number"`
	Name   string `json:"name"`
}

// BroadcastConsentSummary mengembalikan ringkasan catatan lokal untuk tampilan Kontak.
// Angka ini berasal dari aktivitas SlaluDiskon, bukan quality rating atau verifikasi WhatsApp.
func BroadcastConsentSummary(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}

	var activeConsent, marketingConsent, optedOut, interacted int64
	database.DB.Model(&models.ContactConsent{}).
		Where("agent_id = ? AND revoked_at IS NULL", agentID).
		Distinct("number").Count(&activeConsent)
	database.DB.Model(&models.ContactConsent{}).
		Where("agent_id = ? AND category = ? AND revoked_at IS NULL", agentID, "marketing").
		Distinct("number").Count(&marketingConsent)
	database.DB.Model(&models.OptOut{}).
		Where("agent_id = ?", agentID).
		Count(&optedOut)
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND message <> ''", agentID).
		Distinct("sender").Count(&interacted)

	c.JSON(200, gin.H{"data": gin.H{
		"active_consent":    activeConsent,
		"marketing_consent": marketingConsent,
		"interacted":        interacted,
		"opted_out":         optedOut,
	}})
}

// normalizeGuardRecipients merapikan daftar penerima: normalisasi nomor + buang duplikat/kosong.
func normalizeGuardRecipients(in []broadcastGuardRecipient) []broadcastGuardRecipient {
	seen := map[string]bool{}
	out := make([]broadcastGuardRecipient, 0, len(in))
	for _, r := range in {
		number := services.NormalizePhone(r.Number)
		if number == "" || seen[number] {
			continue
		}
		seen[number] = true
		out = append(out, broadcastGuardRecipient{Number: number, Name: strings.TrimSpace(r.Name)})
	}
	return out
}

// activeConsentSet = himpunan nomor yang punya consent aktif untuk kategori tertentu.
// Dipakai worker broadcast untuk menghormati consent broadcast lama (kategori kosong = no-op).
func activeConsentSet(agentID uint, category string, numbers []string) map[string]bool {
	set := map[string]bool{}
	if category == "" || len(numbers) == 0 {
		return set
	}
	var rows []models.ContactConsent
	database.DB.Where("agent_id = ? AND category = ? AND number IN ? AND revoked_at IS NULL", agentID, category, numbers).Find(&rows)
	for _, row := range rows {
		set[row.Number] = true
	}
	return set
}
