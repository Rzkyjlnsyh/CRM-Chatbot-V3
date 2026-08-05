package handlers

import (
	"fmt"
	"log"
	"strings"

	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

type metaBrowserContext struct {
	EventID        string `json:"event_id"`
	EventSourceURL string `json:"event_source_url"`
	FBP            string `json:"fbp"`
	FBC            string `json:"fbc"`
}

func normalizeMetaBrowserContext(value metaBrowserContext) metaBrowserContext {
	value.EventID = strings.TrimSpace(value.EventID)
	value.EventSourceURL = strings.TrimSpace(value.EventSourceURL)
	value.FBP = strings.TrimSpace(value.FBP)
	value.FBC = strings.TrimSpace(value.FBC)
	if len(value.EventID) > 100 {
		value.EventID = ""
	}
	if len(value.EventSourceURL) > 2048 {
		value.EventSourceURL = value.EventSourceURL[:2048]
	}
	if len(value.FBP) > 255 {
		value.FBP = ""
	}
	if len(value.FBC) > 255 {
		value.FBC = ""
	}
	return value
}

func metaUserData(c *gin.Context, email, phone, externalID string, browser metaBrowserContext) services.MetaUserDataInput {
	return services.MetaUserDataInput{
		Email: email, Phone: phone, ExternalID: externalID,
		ClientIP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
		FBP: browser.FBP, FBC: browser.FBC,
	}
}

func enqueueMetaEvent(input services.MetaEventInput) bool {
	if err := services.EnqueueMetaEvent(input); err != nil {
		log.Printf("[meta-capi] gagal menyimpan %s/%s: %v", input.EventName, input.EventID, err)
		return false
	}
	return true
}

func PublicMetaPixelConfig(c *gin.Context) {
	enabled, pixelID := services.PublicMetaPixelConfig()
	c.JSON(200, gin.H{"enabled": enabled, "pixel_id": pixelID})
}

func AdminGetMetaTracking(c *gin.Context) {
	cfg, err := services.GetMetaTrackingConfig()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"enabled":          cfg.Enabled,
		"pixel_id":         cfg.PixelID,
		"graph_version":    cfg.GraphVersion,
		"test_event_code":  cfg.TestEventCode,
		"token_configured": cfg.AccessToken != "",
		"stats":            services.GetMetaTrackingStats(),
	})
}

func AdminSetMetaTracking(c *gin.Context) {
	var req struct {
		Enabled          bool   `json:"enabled"`
		PixelID          string `json:"pixel_id"`
		AccessToken      string `json:"access_token"`
		GraphVersion     string `json:"graph_version"`
		TestEventCode    string `json:"test_event_code"`
		ClearAccessToken bool   `json:"clear_access_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Data konfigurasi tidak valid"})
		return
	}
	cfg, err := services.SaveMetaTrackingConfig(services.MetaTrackingConfigInput{
		Enabled: req.Enabled, PixelID: req.PixelID, AccessToken: req.AccessToken,
		GraphVersion: req.GraphVersion, TestEventCode: req.TestEventCode,
		ClearAccessToken: req.ClearAccessToken,
	})
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"message":          "Pengaturan Meta Pixel dan CAPI disimpan",
		"enabled":          cfg.Enabled,
		"pixel_id":         cfg.PixelID,
		"graph_version":    cfg.GraphVersion,
		"test_event_code":  cfg.TestEventCode,
		"token_configured": cfg.AccessToken != "",
		"stats":            services.GetMetaTrackingStats(),
	})
}

func AdminTestMetaTracking(c *gin.Context) {
	_, err := services.TestMetaCAPI(c.Request.Context(), services.MetaUserDataInput{
		ExternalID: "admin-test-" + fmt.Sprint(c.GetUint("user_id")),
		ClientIP:   c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	})
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": "Event tes diterima Meta"})
}
