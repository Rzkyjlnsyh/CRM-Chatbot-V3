package handlers

import (
	"os"
	"strings"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
)

func ListTemplates(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var tpls []models.Template
	database.DB.Where("agent_id = ?", id).Order("sort_order asc, id asc").Find(&tpls)
	c.JSON(200, gin.H{"data": tpls})
}

// bindTemplateMedia membaca file "file" dari form (max 8 MB) → (path, mime, nama, mediaType).
func bindTemplateMedia(c *gin.Context) (mediaPath, mime, name, mediaType string) {
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		return "", "", "", ""
	}
	if file.Size > 8<<20 {
		return "", "", "", ""
	}
	fh, err := file.Open()
	if err != nil {
		return "", "", "", ""
	}
	defer fh.Close()
	data := make([]byte, file.Size)
	if _, err := fh.Read(data); err != nil {
		return "", "", "", ""
	}
	mime = file.Header.Get("Content-Type")
	mediaType = classifyMediaType(mime, file.Filename)
	if mediaType == "" {
		return "", "", "", ""
	}
	return storeMedia(currentAgentID(c), data, mime, file.Filename), mime, file.Filename, mediaType
}

func CreateTemplate(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	t := models.Template{AgentID: id}
	if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		// Multipart: teks + lampiran sekaligus (pola v4).
		t.Title = strings.TrimSpace(c.PostForm("title"))
		t.Body = c.PostForm("body")
		if t.Title == "" || strings.TrimSpace(t.Body) == "" {
			c.JSON(400, gin.H{"error": "Judul & isi template wajib diisi"})
			return
		}
		t.MediaPath, t.Mimetype, t.FileName, t.MediaType = bindTemplateMedia(c)
	} else {
		var req struct {
			Title     string `json:"title"`
			Body      string `json:"body"`
			SortOrder int    `json:"sort_order"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Format data tidak valid"})
			return
		}
		if strings.TrimSpace(req.Title) == "" || strings.TrimSpace(req.Body) == "" {
			c.JSON(400, gin.H{"error": "Judul & isi template wajib diisi"})
			return
		}
		t.Title, t.Body, t.SortOrder = req.Title, req.Body, req.SortOrder
	}
	if err := database.DB.Create(&t).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal"})
		return
	}
	c.JSON(201, gin.H{"data": t})
}

func UpdateTemplate(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var t models.Template
	if database.DB.Where("agent_id = ?", id).First(&t, c.Param("tid")).Error != nil {
		c.JSON(404, gin.H{"error": "Template tidak ditemukan"})
		return
	}
	if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		title := strings.TrimSpace(c.PostForm("title"))
		body := c.PostForm("body")
		if title != "" {
			t.Title = title
		}
		if strings.TrimSpace(body) != "" {
			t.Body = body
		}
		if path, mime, name, mtype := bindTemplateMedia(c); path != "" {
			if t.MediaPath != "" {
				_ = os.Remove(t.MediaPath)
			}
			t.MediaPath, t.Mimetype, t.FileName, t.MediaType = path, mime, name, mtype
		}
	} else {
		var req struct {
			Title     *string `json:"title"`
			Body      *string `json:"body"`
			SortOrder *int    `json:"sort_order"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "Format data tidak valid"})
			return
		}
		if req.Title != nil {
			t.Title = *req.Title
		}
		if req.Body != nil {
			t.Body = *req.Body
		}
		if req.SortOrder != nil {
			t.SortOrder = *req.SortOrder
		}
	}
	_ = database.DB.Save(&t).Error
	c.JSON(200, gin.H{"data": t})
}

func DeleteTemplate(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var t models.Template
	if database.DB.Where("agent_id = ?", id).First(&t, c.Param("tid")).Error == nil && t.MediaPath != "" {
		_ = os.Remove(t.MediaPath)
	}
	_ = database.DB.Where("agent_id = ?", id).Delete(&models.Template{}, c.Param("tid")).Error
	c.JSON(200, gin.H{"message": "Deleted"})
}

// ServeTemplateMedia — GET /agents/:id/templates/:tid/media (preview lampiran).
func ServeTemplateMedia(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var t models.Template
	if database.DB.Where("agent_id = ?", id).First(&t, c.Param("tid")).Error != nil || t.MediaPath == "" {
		c.JSON(404, gin.H{"error": "Lampiran tidak ada"})
		return
	}
	c.File(t.MediaPath)
}

// classifyMediaType memetakan mimetype/nama file ke jenis media yang didukung.
func classifyMediaType(mime, fileName string) string {
	lower := strings.ToLower(mime + " " + fileName)
	switch {
	case strings.Contains(lower, "image"):
		return "image"
	case strings.Contains(lower, "video"):
		return "video"
	case strings.Contains(lower, "audio"):
		return "audio"
	case strings.Contains(lower, "pdf") || strings.Contains(lower, "document") ||
		strings.Contains(lower, ".doc") || strings.Contains(lower, ".xls") ||
		strings.Contains(lower, ".ppt") || strings.Contains(lower, ".csv") || strings.Contains(lower, ".txt"):
		return "document"
	default:
		return "document"
	}
}
