package handlers

import (
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

func CreateTemplate(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
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
	t := models.Template{AgentID: id, Title: req.Title, Body: req.Body, SortOrder: req.SortOrder}
	if err := database.DB.Create(&t).Error; err != nil { c.JSON(500, gin.H{"error": "Gagal"}); return }
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
	_ = database.DB.Save(&t).Error
	c.JSON(200, gin.H{"data": t})
}

func DeleteTemplate(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	_ = database.DB.Where("agent_id = ?", id).Delete(&models.Template{}, c.Param("tid")).Error
	c.JSON(200, gin.H{"message": "Deleted"})
}
