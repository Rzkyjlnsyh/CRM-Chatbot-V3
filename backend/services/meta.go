package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// ---- Fase 5: Meta Conversions API (CAPI) ----
// Saat label konversi menempel ke kontak (OnLabelAssoc), event dikirim ke
// Pixel Meta (server-side) sehingga Meta Ads melihat konversi nyata.
// Dedup per (agent, kontak, label) — satu label = satu event.

const metaGraphVersion = "v19.0"

// hashUserData = sha256 lowercase (format yang diwajibkan Meta utk user_data).
func hashUserData(v string) string {
	if v == "" {
		return ""
	}
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(v))))
	return hex.EncodeToString(h[:])
}

// MetaLabelEvent = pemetaan label_id -> event CAPI (skema pelabelan).
type MetaLabelEvent struct {
	LabelID string `json:"label_id"`
	Event   string `json:"event"`
}

// parseMetaLabelEvents mengurai JSON mapping label->event menjadi map.
func parseMetaLabelEvents(raw string) map[string]string {
	out := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return out
	}
	var arr []MetaLabelEvent
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		return out
	}
	for _, m := range arr {
		if id := strings.TrimSpace(m.LabelID); id != "" {
			if ev := strings.TrimSpace(m.Event); ev != "" {
				out[id] = ev
			}
		}
	}
	return out
}

// MetaEventForLabel = event CAPI untuk label tertentu (dari mapping agent),
// fallback ke event default (MetaEventName) bila tidak dipetakan.
func MetaEventForLabel(agentID uint, labelID, fallback string) string {
	if labelID == "" {
		return fallback
	}
	var a models.Agent
	database.DB.Select("meta_label_events").Where("id = ?", agentID).Limit(1).Find(&a)
	if ev, ok := parseMetaLabelEvents(a.MetaLabelEvents)[labelID]; ok {
		return ev
	}
	return fallback
}

// MetaLabelEventsMap = map label_id -> event (dari JSON raw) untuk UI.
func MetaLabelEventsMap(raw string) map[string]string {
	return parseMetaLabelEvents(raw)
}

// MetaStandardEvents = daftar event standar Meta (untuk dropdown skema pelabelan).
func MetaStandardEvents() []string {
	return []string{
		"Purchase", "Lead", "Contact", "CompleteRegistration",
		"SubmitApplication", "Schedule", "Subscribe", "StartTrial",
		"InitiateCheckout", "AddToCart", "AddToWishlist", "ViewContent", "Search",
	}
}

// agentMetaConvLabels = daftar label_id yang dianggap konversi milik agent.
func agentMetaConvLabels(agentID uint) (pixelID, token, testCode, eventName string, labels []string) {
	var a models.Agent
	database.DB.Select("meta_pixel_id", "meta_access_token", "meta_test_event_code", "meta_event_name", "meta_conv_labels").
		Where("id = ?", agentID).Limit(1).Find(&a)
	eventName = strings.TrimSpace(a.MetaEventName)
	if eventName == "" {
		eventName = "Purchase"
	}
	for _, l := range strings.Split(a.MetaConvLabels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			labels = append(labels, l)
		}
	}
	return a.MetaPixelID, a.MetaAccessToken, a.MetaTestEventCode, eventName, labels
}

// IsMetaConvLabel = true bila label ini dikonfigurasi sebagai label konversi.
func IsMetaConvLabel(agentID uint, labelID string) bool {
	_, token, _, _, labels := agentMetaConvLabels(agentID)
	if token == "" || len(labels) == 0 {
		return false
	}
	for _, l := range labels {
		if l == labelID {
			return true
		}
	}
	return false
}

// FireMetaConversion mengirim event CAPI (async via RecoverGo) — dedup
// per (agent, kontak, label): event yang sama tidak dikirim dua kali.
func FireMetaConversion(agentID uint, sender, labelID string) {
	if sender == "" || labelID == "" {
		return
	}
	pixelID, token, testCode, eventName, labels := agentMetaConvLabels(agentID)
	if pixelID == "" || token == "" || len(labels) == 0 {
		return
	}
	match := false
	for _, l := range labels {
		if l == labelID {
			match = true
			break
		}
	}
	if !match {
		return
	}
	// Event per-label (skema pelabelan) — fallback ke event default agent.
	eventName = MetaEventForLabel(agentID, labelID, eventName)
	Go("MetaCAPI", func() {
		defer RecoverGo("MetaCAPI")
		// Dedup: baris unik (agent_id, sender, label_id) — FirstOrCreate.
		var existing models.MetaConversion
		database.DB.Where(models.MetaConversion{AgentID: agentID, Sender: sender, LabelID: labelID}).First(&existing)
		if existing.ID > 0 {
			return // sudah pernah dikirim
		}
		payload := map[string]any{
			"event_name": eventName,
			"event_time": time.Now().Unix(),
			"user_data": map[string]string{
				"ph": hashUserData(sender),
			},
			"custom_data": map[string]any{"currency": "IDR"},
		}
		if testCode != "" {
			payload["test_event_code"] = testCode
		}
		body, _ := json.Marshal(map[string]any{"data": []any{payload}})
		url := fmt.Sprintf("https://graph.facebook.com/%s/%s/events", metaGraphVersion, pixelID)
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
		if err != nil {
			log.Printf("MetaCAPI: build request gagal: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{Timeout: 12 * time.Second}
		resp, err := client.Do(req)
		status := "sent"
		respInfo := ""
		if err != nil {
			status = "failed"
			respInfo = truncateForLog(err.Error(), 255)
		} else {
			defer resp.Body.Close()
			buf := make([]byte, 1024)
			n, _ := resp.Body.Read(buf)
			respInfo = strings.TrimSpace(string(buf[:n]))
			if resp.StatusCode >= 300 {
				status = "failed"
			}
		}
		database.DB.Create(&models.MetaConversion{
			AgentID: agentID, Sender: sender, LabelID: labelID,
			EventName: eventName, Status: status, Response: truncateForLog(respInfo, 255),
			SentAt: time.Now(),
		})
		log.Printf("MetaCAPI: agent %d event %s utk %s (label %s) -> %s", agentID, eventName, sender, labelID, status)
	})
}

// SendMetaTestEvent = kirim event percobaan langsung (utk tombol "Tes Kirim").
func SendMetaTestEvent(agentID uint, sender, pixelID, token string) {
	Go("MetaTestEvent", func() {
		defer RecoverGo("MetaTestEvent")
		payload := map[string]any{
			"event_name": "TestEvent",
			"event_time": time.Now().Unix(),
			"user_data":  map[string]string{"ph": hashUserData(sender)},
		}
		body, _ := json.Marshal(map[string]any{"data": []any{payload}})
		url := fmt.Sprintf("https://graph.facebook.com/%s/%s/events", metaGraphVersion, pixelID)
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{Timeout: 12 * time.Second}
		resp, err := client.Do(req)
		status := "sent"
		respInfo := ""
		if err != nil {
			status = "failed"
			respInfo = truncateForLog(err.Error(), 255)
		} else {
			defer resp.Body.Close()
			buf := make([]byte, 1024)
			n, _ := resp.Body.Read(buf)
			respInfo = strings.TrimSpace(string(buf[:n]))
			if resp.StatusCode >= 300 {
				status = "failed"
			}
		}
		database.DB.Create(&models.MetaConversion{
			AgentID: agentID, Sender: sender, LabelID: "TEST-EVENT",
			EventName: "TestEvent", Status: status, Response: truncateForLog(respInfo, 255),
			SentAt: time.Now(),
		})
		log.Printf("MetaCAPI: test event agent %d -> %s (%s)", agentID, status, truncateForLog(respInfo, 120))
	})
}

// MetaConversions = log event CAPI (utk dashboard "Tes & Log").
func MetaConversions(agentID uint, limit int) []models.MetaConversion {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var out []models.MetaConversion
	database.DB.Where("agent_id = ?", agentID).Order("id desc").Limit(limit).Find(&out)
	return out
}
