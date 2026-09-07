// Lincah Webhook — sisi MASUK: Lincah memanggil endpoint kita saat status
// paket berubah. Format payload resmi Lincah tidak didokumentasikan di PDF,
// jadi parser dibuat TOLERAN (beberapa bentuk umum) + payload mentah selalu
// disimpan untuk diagnostik (jangan pernah kehilangan data).
//
// Alur:
//  1. POST /api/lincah/webhook (publik; secret opsional X-Lincah-Secret)
//  2. Parse no_order/resi/status dari berbagai bentuk JSON
//  3. Update LincahOrder lokal (WebhookStatus)
//  4. Bila AutoNotify aktif & status BARU → kirim WA ke pelanggan
//     (satu kali per status — anti-spam via NotifiedStatus)
package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// lincahSendFn = titik kirim WA (dapat di-stub di tes).
var lincahSendFn = func(agentID uint, to, text string) error {
	inst := WA(agentID)
	if inst == nil {
		return fmt.Errorf("agent WA tidak aktif")
	}
	return inst.SendReply(to, text, "")
}

// LincahWebhookEvent = hasil parse payload webhook Lincah.
type LincahWebhookEvent struct {
	NoOrder string
	Resi    string
	Status  string
	Message string
}

// LincahParseWebhook mengekstrak identitas & status dari payload JSON
// dalam berbagai bentuk (toleran terhadap evolusi format Lincah).
func LincahParseWebhook(raw []byte) (LincahWebhookEvent, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return LincahWebhookEvent{}, fmt.Errorf("payload bukan JSON: %w", err)
	}
	// Bentuk 1: {"no_order":..., "resi":..., "status":...} langsung
	// Bentuk 2: {"order": {...}} atau {"data": {...}} (envelope)
	var body map[string]json.RawMessage
	if nested, ok := root["order"]; ok {
		_ = json.Unmarshal(nested, &body)
	} else if nested, ok := root["data"]; ok {
		_ = json.Unmarshal(nested, &body)
	} else {
		body = root
	}
	getStr := func(key string) string {
		if v, ok := body[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}
	ev := LincahWebhookEvent{
		NoOrder: getStr("no_order"),
		Resi:    getStr("resi"),
		Status:  getStr("status"),
		Message: getStr("message"),
	}
	if ev.Resi == "" {
		ev.Resi = getStr("tracking_number")
	}
	if ev.Status == "" {
		ev.Status = getStr("event")
	}
	if ev.NoOrder == "" && ev.Resi == "" {
		return LincahWebhookEvent{}, fmt.Errorf("payload tidak memuat no_order/resi")
	}
	return ev, nil
}

// LincahProcessWebhook menerapkan event ke pesanan lokal + kirim notifikasi.
// Selalu kembalikan error TIDAK fatal (best effort) — caller merespon 200
// agar Lincah tidak melakukan retry beruntun.
func LincahProcessWebhook(raw []byte) (LincahWebhookEvent, error) {
	ev, err := LincahParseWebhook(raw)
	if err != nil {
		return ev, err
	}
	var order models.LincahOrder
	q := database.DB.Where("no_order = ? OR resi = ?", ev.NoOrder, ev.Resi)
	if err := q.Order("id DESC").First(&order).Error; err != nil {
		return ev, fmt.Errorf("pesanan tidak dikenal: %v", err)
	}
	if ev.Status != "" {
		order.WebhookStatus = ev.Status
		order.RawJSON = appendWebhookRaw(order.RawJSON, raw)
		if err := database.DB.Model(&models.LincahOrder{}).Where("id = ?", order.ID).Updates(map[string]any{
			"webhook_status": order.WebhookStatus,
			"raw_json":       order.RawJSON,
		}).Error; err != nil {
			return ev, err
		}
	}
	// Notifikasi ke pelanggan (bila aktif & status belum pernah dikabari).
	if ev.Status != "" && order.NotifiedStatus != ev.Status && lincahAutoNotify(order.AgentID) {
		text := lincahNotifyText(order, ev.Status, ev.Message)
		if err := lincahSendFn(order.AgentID, order.Sender, text); err == nil {
			now := time.Now()
			_ = database.DB.Model(&models.LincahOrder{}).Where("id = ?", order.ID).Updates(map[string]any{
				"notified_status": ev.Status,
				"last_notify_at":  now,
			}).Error
		}
	}
	return ev, nil
}

// appendWebhookRaw menyimpan log payload mentah (maks. 64KB terjaga).
func appendWebhookRaw(existing string, raw []byte) string {
	entry := strings.TrimSpace(string(raw))
	if len(entry) > 8000 {
		entry = entry[:8000]
	}
	if existing == "" {
		return entry
	}
	return existing + "\n---\n" + entry
}

// lincahAutoNotify — apakah agent ini menyalakan follow-up otomatis.
func lincahAutoNotify(agentID uint) bool {
	var cfg models.LincahConfig
	if err := database.DB.Where("agent_id = ?", agentID).First(&cfg).Error; err != nil {
		return false
	}
	return cfg.AutoNotify
}

// lincahNotifyText merangkai pesan WA dari template (default bila kosong).
func lincahNotifyText(order models.LincahOrder, status, message string) string {
	var cfg models.LincahConfig
	_ = database.DB.Where("agent_id = ?", order.AgentID).First(&cfg).Error
	tpl := strings.TrimSpace(cfg.NotifyTemplate)
	if tpl == "" {
		tpl = "Halo kak! 📦 Paket {{resi}} Anda: *{{status}}*.{{message}}"
	}
	repl := strings.NewReplacer(
		"{{resi}}", firstNonEmpty(order.Resi, order.NoOrder),
		"{{no_order}}", order.NoOrder,
		"{{status}}", status,
		"{{courier}}", order.Courier,
		"{{nama}}", order.Name,
	)
	out := repl.Replace(tpl)
	if message != "" && strings.Contains(tpl, "{{message}}") {
		out = strings.ReplaceAll(out, "{{message}}", " "+message)
	} else {
		out = strings.ReplaceAll(out, " {{message}}", "")
		out = strings.ReplaceAll(out, "{{message}}", "")
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
