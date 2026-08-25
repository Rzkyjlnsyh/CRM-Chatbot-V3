package services

// Glue Meta CAPI ↔ label WhatsApp (single-tenant, non-SaaS):
// saat label konversi menempel ke kontak, event dikirim ke Pixel Meta
// (server-side) lengkap dengan nilai transaksi (value-based ads).

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

const (
	metaConvLabelsKey = "meta_conv_labels" // label WA yang dianggap konversi (koma)
	metaEventNameKey  = "meta_event_name"  // event default (mis. Purchase)
)

// MetaEventForLabel memetakan label → event Meta (dari setting global).
func MetaEventForLabel(labelID, fallback string) string {
	raw := database.GetAppSetting("meta_label_events", "")
	if raw == "" {
		return fallback
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		if v, ok := m[labelID]; ok && v != "" {
			return v
		}
	}
	return fallback
}

// IsMetaConvLabel mengecek apakah label termasuk label konversi (global).
func IsMetaConvLabel(labelID string) bool {
	cfg, err := GetMetaTrackingConfig()
	if err != nil || !cfg.Enabled || cfg.PixelID == "" || cfg.AccessToken == "" {
		return false
	}
	for _, l := range splitComma(database.GetAppSetting(metaConvLabelsKey, "")) {
		if l == labelID {
			return true
		}
	}
	return false
}

// FireMetaConversion mengirim event CAPI saat label konversi menempel.
// Dedup per (agent, kontak, label) — satu label = satu event.
func FireMetaConversion(agentID uint, sender, labelID string) {
	if sender == "" || labelID == "" {
		return
	}
	cfg, err := GetMetaTrackingConfig()
	if err != nil || !cfg.Enabled || cfg.PixelID == "" || cfg.AccessToken == "" {
		return
	}
	eventName := MetaEventForLabel(labelID, database.GetAppSetting(metaEventNameKey, "Purchase"))

	Go("MetaCAPI", func() {
		defer RecoverGo("MetaCAPI")
		// Dedup: EventID deterministik per (agent, kontak, label) — satu label = satu event.
		eventID := fmt.Sprintf("%d-%s-%s", agentID, sender, labelID)
		var existing models.MetaConversionEvent
		database.DB.Where("event_id = ?", eventID).First(&existing)
		if existing.ID > 0 {
			return // sudah pernah dikirim
		}
		customData := map[string]any{"currency": "IDR"}
		if v := metaPurchaseValue(agentID, sender); v > 0 {
			customData["value"] = v
		}
		ev := MetaEventInput{
			EventID:    eventID,
			EventName:  eventName,
			EventTime:  time.Now(),
			UserData:   MetaUserDataInput{Phone: sender},
			CustomData: customData,
		}
		if err := EnqueueMetaEvent(ev); err != nil {
			log.Printf("MetaCAPI: gagal antri event %s utk %s (label %s): %v", eventName, sender, labelID, err)
			return
		}
		log.Printf("MetaCAPI: agent %d event %s utk %s (label %s) diantri", agentID, eventName, sender, labelID)
	})
}

// metaPurchaseValue mencari nilai transaksi terbaru pelanggan untuk event
// value-based. Prioritas: ClosingRecord (order terdeteksi AI), lalu
// ProductOrder (checkout).
func metaPurchaseValue(agentID uint, sender string) float64 {
	var rec models.ClosingRecord
	if database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).
		Order("created_at desc").First(&rec).Error == nil {
		if v := extractAmountFromJSON(rec.DataJSON); v > 0 {
			return v
		}
	}
	var order models.ProductOrder
	if database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).
		Order("created_at desc").First(&order).Error == nil {
		if v := extractAmountFromJSON(order.DataJSON); v > 0 {
			return v
		}
	}
	return 0
}

// extractAmountFromJSON memindai nilai uang dari JSON (kunci umum level atas).
func extractAmountFromJSON(raw string) float64 {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return 0
	}
	for _, key := range []string{"total", "grand_total", "harga", "amount", "nilai", "price", "subtotal"} {
		if v, ok := m[key]; ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprintf("%v", v)), 64); err == nil && f > 0 {
				return f
			}
		}
	}
	return 0
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
