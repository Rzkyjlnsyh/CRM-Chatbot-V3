package handlers

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
)

// ListMediaAssets mengambil daftar media assets untuk satu agent
// GET /api/agents/:id/media-assets
func ListMediaAssets(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var assets []models.MediaAsset
	database.DB.Where("agent_id = ?", id).Order("id DESC").Find(&assets)
	c.JSON(200, gin.H{"success": true, "data": assets})
}

// UploadMediaAsset upload file media untuk digunakan AI via directive [[SEND_MEDIA:label]] atau [[SEND_MEDIA:id]].
// POST /api/agents/:id/media-assets (multipart form)
func UploadMediaAsset(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	caption := strings.TrimSpace(c.PostForm("caption"))
	triggerKeys := strings.TrimSpace(c.PostForm("trigger_keys"))
	label := strings.TrimSpace(c.PostForm("label"))
	sortOrder, _ := strconv.Atoi(strings.TrimSpace(c.PostForm("sort_order")))

	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "File wajib diunggah"})
		return
	}
	f, err := fh.Open()
	if err != nil {
		c.JSON(400, gin.H{"error": "Gagal membaca file"})
		return
	}
	defer f.Close()
	data, _ := io.ReadAll(f)

	mimeType := fh.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Tentukan media type
	mediaType := "document"
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		mediaType = "image"
	case strings.HasPrefix(mimeType, "video/"):
		mediaType = "video"
	}

	// Simpan file ke disk
	uploadDir := filepath.Join("data", "media", strconv.Itoa(int(id)))
	os.MkdirAll(uploadDir, 0755)

	ext := filepath.Ext(fh.Filename)
	if ext == "" {
		switch mediaType {
		case "image":
			ext = ".jpg"
		case "video":
			ext = ".mp4"
		default:
			ext = ".bin"
		}
	}
	savedPath := filepath.Join(uploadDir, strconv.FormatInt(time.Now().UnixNano(), 36)+ext)
	if err := os.WriteFile(savedPath, data, 0644); err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan file: " + err.Error()})
		return
	}

	if name == "" {
		name = fh.Filename
	}

	asset := models.MediaAsset{
		AgentID:     id,
		TenantID:    1,
		Name:        name,
		FileName:    fh.Filename,
		MediaType:   mediaType,
		MimeType:    mimeType,
		FilePath:    savedPath,
		Caption:     caption,
		FileSize:    int64(len(data)),
		Label:       label,
		TriggerKeys: triggerKeys,
		SortOrder:   sortOrder,
		IsActive:    true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	database.DB.Create(&asset)

	log.Printf("[media] Agent %d upload media #%d: %s (%s, %d bytes)", id, asset.ID, name, mediaType, len(data))
	c.JSON(200, gin.H{"success": true, "data": asset})
}

// DeleteMediaAsset menghapus media asset
// DELETE /api/agents/:id/media-assets/:assetId
func DeleteMediaAsset(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	assetID, err := strconv.ParseUint(c.Param("assetId"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "ID tidak valid"})
		return
	}

	var asset models.MediaAsset
	if database.DB.Where("id = ? AND agent_id = ?", assetID, id).First(&asset).Error != nil {
		c.JSON(404, gin.H{"error": "Media tidak ditemukan"})
		return
	}

	// Hapus file dari disk
	if asset.FilePath != "" {
		os.Remove(asset.FilePath)
	}

	database.DB.Delete(&asset)
	c.JSON(200, gin.H{"success": true, "message": "Media dihapus"})
}


// ServeMediaAssetFile menyajikan file media asset (untuk preview di dashboard).
// Route publik dengan token query — pola sama dengan ServeProductImage.
// GET /api/agents/:id/media-assets/:assetId/file
func ServeMediaAssetFile(c *gin.Context) {
	tid, ok := tenantFromToken(c.Query("token"))
	if !ok {
		c.AbortWithStatus(401)
		return
	}
	agentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	var agent models.Agent
	if database.DB.Select("id").Where("id = ? AND tenant_id = ?", agentID, tid).First(&agent).Error != nil {
		c.AbortWithStatus(404)
		return
	}
	assetID, err := strconv.Atoi(c.Param("assetId"))
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	var asset models.MediaAsset
	if database.DB.Where("id = ? AND agent_id = ?", assetID, agentID).First(&asset).Error != nil || asset.FilePath == "" {
		c.AbortWithStatus(404)
		return
	}
	c.File(asset.FilePath)
}
