package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// ---- Fase 5: Meta Conversions API (CAPI) — settings + log ----

// GetMetaConfig = baca konfigurasi CAPI agent (token TIDAK pernah dikirim balik).
func GetMetaConfig(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var a models.Agent
	database.DB.First(&a, id)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"pixel_id":         a.MetaPixelID,
		"configured":       a.MetaAccessToken != "",
		"test_event_code":  a.MetaTestEventCode,
		"conv_labels":      a.MetaConvLabels,
		"event_name":       a.MetaEventName,
		"label_events":     services.MetaLabelEventsMap(a.MetaLabelEvents),
		"standard_events":  services.MetaStandardEvents(),
		"recent_events":    services.MetaConversions(id, 20),
		"available_labels": labelResponseData(id),
	}})
}

// SaveMetaConfig = simpan konfigurasi CAPI. Access token kosong = pertahankan lama.
func SaveMetaConfig(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		PixelID       string            `json:"pixel_id"`
		AccessToken   string            `json:"access_token"`
		TestEventCode string            `json:"test_event_code"`
		ConvLabels    string            `json:"conv_labels"`
		EventName     string            `json:"event_name"`
		LabelEvents   map[string]string `json:"label_events"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Format data tidak valid"})
		return
	}
	eventName := strings.TrimSpace(req.EventName)
	if eventName == "" {
		eventName = "Purchase"
	}
	labelEventsJSON := marshalMetaLabelEvents(req.LabelEvents)
	updates := map[string]any{
		"meta_pixel_id":        strings.TrimSpace(req.PixelID),
		"meta_test_event_code": strings.TrimSpace(req.TestEventCode),
		"meta_conv_labels":     strings.TrimSpace(req.ConvLabels),
		"meta_event_name":      eventName,
		"meta_label_events":    labelEventsJSON,
	}
	if req.AccessToken != "" {
		updates["meta_access_token"] = strings.TrimSpace(req.AccessToken)
	}
	if err := database.DB.Model(&models.Agent{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menyimpan konfigurasi"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"saved": true}})
}

// marshalMetaLabelEvents serialisasi map label_id -> event menjadi JSON array.
func marshalMetaLabelEvents(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	arr := make([]services.MetaLabelEvent, 0, len(m))
	for labelID, ev := range m {
		if strings.TrimSpace(labelID) == "" || strings.TrimSpace(ev) == "" {
			continue
		}
		arr = append(arr, services.MetaLabelEvent{LabelID: labelID, Event: strings.TrimSpace(ev)})
	}
	b, err := json.Marshal(arr)
	if err != nil {
		return ""
	}
	return string(b)
}

// TestMetaEvent = kirim 1 event percobaan ke Pixel (pakai test_event_code bila diisi).
func TestMetaEvent(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	// Cek konfigurasi minimum.
	var a models.Agent
	database.DB.Select("meta_pixel_id", "meta_access_token").Where("id = ?", id).Limit(1).Find(&a)
	if a.MetaPixelID == "" || a.MetaAccessToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pixel ID dan Access Token wajib diisi dulu."})
		return
	}
	// Kirim event test langsung (dedup dilewati untuk event test).
	services.SendMetaTestEvent(id, "TEST-EVENT", a.MetaPixelID, a.MetaAccessToken)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"sent": true}})
}

// MetaConversionLogs = log 20 event CAPI terakhir.
func MetaConversionLogs(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": services.MetaConversions(id, 20)})
}
