package handlers

import (
	"strings"

	"wa-assistant/backend/database"

	"github.com/gin-gonic/gin"
)

// Key AppSetting untuk link grup komunitas (diatur super-admin, dipakai semua tenant).
const (
	keyCommunityWhatsApp = "community_whatsapp"
	keyCommunityTelegram = "community_telegram"
)

func communityLinks() gin.H {
	return gin.H{
		"whatsapp": database.GetAppSetting(keyCommunityWhatsApp, ""),
		"telegram": database.GetAppSetting(keyCommunityTelegram, ""),
	}
}

// PublicCommunityLinks = link grup komunitas untuk ditampilkan & diklik user.
func PublicCommunityLinks(c *gin.Context) {
	c.JSON(200, communityLinks())
}

// AdminGetCommunityLinks = baca link komunitas (super-admin).
func AdminGetCommunityLinks(c *gin.Context) {
	c.JSON(200, communityLinks())
}

// AdminSetCommunityLinks = simpan link komunitas. Kosongkan salah satu untuk menyembunyikannya.
func AdminSetCommunityLinks(c *gin.Context) {
	var req struct {
		WhatsApp string `json:"whatsapp"`
		Telegram string `json:"telegram"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	database.SetAppSetting(keyCommunityWhatsApp, strings.TrimSpace(req.WhatsApp))
	database.SetAppSetting(keyCommunityTelegram, strings.TrimSpace(req.Telegram))
	c.JSON(200, communityLinks())
}
