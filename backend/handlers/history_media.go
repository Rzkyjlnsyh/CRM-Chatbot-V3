package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// GetHistoryMedia — GET /agents/:id/history-media/:cid
// Media riwayat WhatsApp diunduh ON-DEMAND saat dibuka di Inbox:
//   - media_path sudah ada → serve cache disk.
//   - belum → unduh dari WhatsApp via metadata protobuf (pending) → simpan →
//     serve; gagal → tandai failed (502; frontend bisa ulang).
func GetHistoryMedia(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	cid, err := strconv.ParseUint(c.Param("cid"), 10, 64)
	if err != nil || cid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cid tidak valid"})
		return
	}
	var row models.ChatHistory
	if database.DB.Where("agent_id = ? AND id = ?", id, uint(cid)).First(&row).Error != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pesan tidak ditemukan"})
		return
	}
	if row.MediaType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pesan bukan media"})
		return
	}
	if strings.TrimSpace(row.MediaPath) != "" {
		c.File(row.MediaPath)
		return
	}
	if len(row.MediaMetadata) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Media tidak tersedia di riwayat (tanpa metadata)"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	data, mime, err := services.WA(id).DownloadHistoryMedia(ctx, row.MediaMetadata)
	if err != nil || len(data) == 0 {
		_ = database.DB.Model(&models.ChatHistory{}).Where("id = ?", row.ID).
			Update("media_fetch_status", "failed").Error
		log.Printf("[history-media] unduh gagal agent=%d cid=%d: %v", id, row.ID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Gagal mengunduh media riwayat: " + err.Error()})
		return
	}
	mediaPath := storeMedia(id, data, mime, row.FileName)
	if mediaPath == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan media"})
		return
	}
	updates := map[string]any{
		"media_path":         mediaPath,
		"media_fetch_status": "done",
	}
	if row.Mimetype == "" && mime != "" {
		updates["mimetype"] = mime
	}
	if err := database.DB.Model(&models.ChatHistory{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		log.Printf("[history-media] gagal update path agent=%d cid=%d: %v", id, row.ID, err)
	}
	c.File(mediaPath)
}
