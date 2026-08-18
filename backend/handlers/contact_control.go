package handlers

import (
	"net/http"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
)

// ---- Kontrol AI per kontak (dipakai CS dari inbox) ----

// permanentPauseUntil = cap waktu jeda permanen (sampai tombol resume ditekan).
// Memakai kolom manual_pause_until yang sama dengan jeda otomatis 10 menit,
// sehingga TIDAK perlu perubahan skema — pipeline yang sudah ada
// (manualPaused -> silent) langsung menghormatinya.
var permanentPauseUntil = time.Date(2099, 12, 31, 23, 59, 59, 0, time.Local)

// PauseAIContact mematikan seluruh balasan otomatis untuk satu nomor customer:
// AI, auto-reply, alur otomatis, tombol produk, dan form AI. Berlaku sampai
// tombol "Lanjutkan AI" ditekan.
func PauseAIContact(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := strings.TrimPrefix(c.Param("sender"), "@")
	if sender == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nomor kontak kosong"})
		return
	}
	var contact models.Contact
	if err := database.DB.
		Where("agent_id = ? AND number = ?", agentID, sender).
		FirstOrCreate(&contact, models.Contact{AgentID: agentID, Number: sender}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menjeda AI untuk kontak ini"})
		return
	}
	if err := database.DB.Model(&contact).Update("manual_pause_until", &permanentPauseUntil).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menjeda AI untuk kontak ini"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"paused": true, "manual_pause_until": permanentPauseUntil}})
}

// ResumeAIContact menghidupkan kembali balasan otomatis untuk kontak.
func ResumeAIContact(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := strings.TrimPrefix(c.Param("sender"), "@")
	if sender == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nomor kontak kosong"})
		return
	}
	var contact models.Contact
	if err := database.DB.
		Where("agent_id = ? AND number = ?", agentID, sender).
		FirstOrCreate(&contact, models.Contact{AgentID: agentID, Number: sender}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal melanjutkan AI"})
		return
	}
	if err := database.DB.Model(&contact).Update("manual_pause_until", nil).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal melanjutkan AI"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"paused": false}})
}

// ContactAIStatus = status jeda AI untuk satu kontak (untuk state tombol UI).
func ContactAIStatus(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := strings.TrimPrefix(c.Param("sender"), "@")
	if sender == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nomor kontak kosong"})
		return
	}
	var contact models.Contact
	data := gin.H{"paused": false}
	if database.DB.Select("manual_pause_until").
		Where("agent_id = ? AND number = ?", agentID, sender).First(&contact).Error == nil &&
		contact.ManualPauseUntil != nil && contact.ManualPauseUntil.After(time.Now()) {
		data["paused"] = true
		data["manual_pause_until"] = contact.ManualPauseUntil
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// ManualHandoffContact memindahkan percakapan ke antrean "Butuh CS" secara
// manual — jalur yang sama dengan handoff otomatis (soft handoff aktif:
// AI tetap melayani info aman sampai CS membalas, lalu diam).
func ManualHandoffContact(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	sender := strings.TrimPrefix(c.Param("sender"), "@")
	if sender == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nomor kontak kosong"})
		return
	}
	var last models.ChatHistory
	lastMsg := ""
	if database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).
		Order("created_at desc").First(&last).Error == nil {
		lastMsg = last.Message
	}
	ensureHandoff(agentID, sender, lastMsg)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"handoff": true,
		"message": "Percakapan dipindahkan ke antrean Butuh CS",
	}})
}
