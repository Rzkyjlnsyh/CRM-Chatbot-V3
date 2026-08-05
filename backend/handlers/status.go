package handlers

import (
	"io"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// CreateStatus memposting WhatsApp Status (Story) sekarang, atau menjadwalkannya. Multipart:
// text + gambar opsional. run_at kosong = posting langsung; run_at di masa depan = dijadwalkan.
func CreateStatus(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	tid := currentTenantID(c)

	text := strings.TrimSpace(c.PostForm("text"))

	// Gambar opsional (status foto).
	var mediaType, mimetype, mediaPath string
	var mediaBytes []byte
	if fh, ferr := c.FormFile("file"); ferr == nil {
		f, e := fh.Open()
		if e != nil {
			c.JSON(400, gin.H{"error": "Gagal membaca gambar"})
			return
		}
		defer f.Close()
		data, readErr := io.ReadAll(f)
		if readErr != nil {
			c.JSON(400, gin.H{"error": "Gagal membaca gambar"})
			return
		}
		mimetype = fh.Header.Get("Content-Type")
		if !strings.HasPrefix(mimetype, "image/") {
			c.JSON(400, gin.H{"error": "Status hanya mendukung gambar untuk saat ini"})
			return
		}
		mediaType = "image"
		mediaBytes = data
		mediaPath = storeMedia(id, data, mimetype, fh.Filename)
	}

	if text == "" && mediaType == "" {
		c.JSON(400, gin.H{"error": "Status tidak boleh kosong (isi teks atau gambar)"})
		return
	}

	runAtStr := strings.TrimSpace(c.PostForm("run_at"))

	// Posting sekarang bila tanpa waktu jadwal.
	if runAtStr == "" {
		if !services.WA(id).IsConnected() {
			c.JSON(400, gin.H{"error": "WhatsApp belum tersambung"})
			return
		}
		rec := models.ScheduledStatus{
			TenantID: tid, AgentID: id, RunAt: time.Now(), Text: text,
			MediaType: mediaType, Mimetype: mimetype, MediaPath: mediaPath, Status: "running",
		}
		if err := database.DB.Create(&rec).Error; err != nil {
			c.JSON(500, gin.H{"error": "Gagal menyimpan status"})
			return
		}
		if err := services.WA(id).PostStatus(text, mimetype, mediaBytes); err != nil {
			database.DB.Model(&rec).Updates(map[string]any{"status": "failed", "error": err.Error()})
			c.JSON(500, gin.H{"error": "Gagal memposting status: " + err.Error()})
			return
		}
		database.DB.Model(&rec).Updates(map[string]any{"status": "done", "error": ""})
		c.JSON(200, gin.H{"data": rec})
		return
	}

	// Dijadwalkan untuk nanti.
	runAt, err := time.Parse(time.RFC3339, runAtStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "Waktu jadwal tidak valid"})
		return
	}
	if runAt.Before(time.Now().Add(-time.Minute)) {
		c.JSON(400, gin.H{"error": "Waktu jadwal sudah lewat"})
		return
	}
	if !tenantPlanAllows(tid, "schedule") {
		c.JSON(403, gin.H{"error": planFeatureMessage})
		return
	}
	rec := models.ScheduledStatus{
		TenantID: tid, AgentID: id, RunAt: runAt, Text: text,
		MediaType: mediaType, Mimetype: mimetype, MediaPath: mediaPath, Status: "scheduled",
	}
	if err := database.DB.Create(&rec).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal membuat jadwal status"})
		return
	}
	c.JSON(200, gin.H{"data": rec})
}

// ListStatuses = daftar status terjadwal & riwayat postingan agent (terbaru dulu).
func ListStatuses(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var ss []models.ScheduledStatus
	database.DB.Where("agent_id = ?", id).Order("run_at desc").Limit(100).Find(&ss)
	c.JSON(200, gin.H{"data": ss})
}

// CancelStatus membatalkan status yang masih terjadwal (belum diposting).
func CancelStatus(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	database.DB.Model(&models.ScheduledStatus{}).
		Where("id = ? AND agent_id = ? AND status = ?", c.Param("sid"), id, "scheduled").
		Update("status", "cancelled")
	c.JSON(200, gin.H{"ok": true})
}
