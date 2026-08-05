package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// Manajemen API key & webhook per-nomor (dipakai dashboard, autentikasi JWT).

// generateToken menghasilkan token acak aman (hex). Dipakai untuk API key & webhook secret.
func generateToken(nbytes int) string {
	b := make([]byte, nbytes)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}

// keyHint menampilkan sebagian kecil token untuk identifikasi tanpa membocorkannya.
func keyHint(k string) string {
	if len(k) < 10 {
		return ""
	}
	return k[:6] + "…" + k[len(k)-4:]
}

// GetAPISettings — GET /agents/:id/api  (status key & webhook, tersamar)
func GetAPISettings(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var a models.Agent
	if database.DB.First(&a, id).Error != nil {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return
	}
	c.JSON(200, gin.H{"data": gin.H{
		"allowed":             tenantPlanAllows(currentTenantID(c), featAPI),
		"connected":           services.WA(id).IsConnected(),
		"has_key":             a.APIKey != "",
		"key_hint":            keyHint(a.APIKey),
		"webhook_url":         a.WebhookURL,
		"has_webhook_secret":  a.WebhookSecret != "",
		"webhook_secret_hint": keyHint(a.WebhookSecret),
	}})
}

// RotateAPIKey — POST /agents/:id/api/key  (buat/putar ulang; plaintext dikembalikan SEKALI)
func RotateAPIKey(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	if !tenantPlanAllows(currentTenantID(c), featAPI) {
		c.JSON(403, gin.H{"error": planFeatureMessage})
		return
	}
	key := "wai_" + generateToken(24)
	if err := database.DB.Model(&models.Agent{}).Where("id = ?", id).Update("api_key", key).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal membuat API key"})
		return
	}
	c.JSON(200, gin.H{"api_key": key, "note": "Simpan sekarang — key hanya ditampilkan sekali."})
}

// RevokeAPIKey — DELETE /agents/:id/api/key
func RevokeAPIKey(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	if err := database.DB.Model(&models.Agent{}).Where("id = ?", id).Update("api_key", "").Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal mencabut API key"})
		return
	}
	c.JSON(200, gin.H{"status": "revoked"})
}

// SaveWebhook — PUT /agents/:id/api/webhook  Body: {"webhook_url":"https://..."}
func SaveWebhook(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		WebhookURL string `json:"webhook_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	url := strings.TrimSpace(req.WebhookURL)
	if url != "" {
		if err := isSafeRemoteURL(url); err != nil {
			c.JSON(400, gin.H{"error": "URL webhook tidak valid: " + err.Error()})
			return
		}
	}
	updates := map[string]any{"webhook_url": url}
	// Pastikan ada secret untuk tanda tangan HMAC saat webhook diaktifkan pertama kali.
	secretNew := ""
	if url != "" {
		var a models.Agent
		database.DB.First(&a, id)
		// Saat webhook baru diaktifkan (termasuk setelah sempat nonaktif), buat secret
		// baru dan tampilkan sekali agar user pasti pernah menerima plaintext-nya.
		if a.WebhookSecret == "" || strings.TrimSpace(a.WebhookURL) == "" {
			secretNew = "whsec_" + generateToken(20)
			updates["webhook_secret"] = secretNew
		}
	}
	if err := database.DB.Model(&models.Agent{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		c.JSON(500, gin.H{"error": "Webhook belum bisa disimpan"})
		return
	}
	resp := gin.H{"status": "saved", "webhook_url": url}
	if secretNew != "" {
		resp["webhook_secret"] = secretNew
		resp["note"] = "Simpan secret ini — dipakai untuk verifikasi tanda tangan X-Signature."
	}
	c.JSON(200, resp)
}

// RotateWebhookSecret — POST /agents/:id/api/webhook-secret
func RotateWebhookSecret(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	secret := "whsec_" + generateToken(20)
	if err := database.DB.Model(&models.Agent{}).Where("id = ?", id).Update("webhook_secret", secret).Error; err != nil {
		c.JSON(500, gin.H{"error": "Secret webhook belum bisa diperbarui"})
		return
	}
	c.JSON(200, gin.H{"webhook_secret": secret, "note": "Simpan sekarang — secret hanya ditampilkan sekali."})
}

// TestAPIMessage — POST /agents/:id/api/test-message
// Uji jalur pengiriman yang sama dengan REST API publik (tanpa mengekspos API key ke browser).
// Body: {"to":"628...","text":"..."} — type default text.
func TestAPIMessage(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	if !tenantPlanAllows(currentTenantID(c), featAPI) {
		c.JSON(403, gin.H{"error": planFeatureMessage})
		return
	}
	var agent models.Agent
	if database.DB.First(&agent, id).Error != nil {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return
	}
	if strings.TrimSpace(agent.APIKey) == "" {
		c.JSON(400, gin.H{"error": "Buat API key terlebih dahulu sebelum uji kirim."})
		return
	}
	if !services.WA(id).IsConnected() {
		c.JSON(409, gin.H{"error": "Nomor WhatsApp sedang tidak tersambung. Hubungkan dulu di Dashboard."})
		return
	}
	var req struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	to := services.NormalizePhone(strings.TrimSpace(req.To))
	if to == "" {
		c.JSON(400, gin.H{"error": "Nomor tujuan tidak valid. Gunakan format 08… atau 62…"})
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		text = "Uji REST API SlaluDiskon — pesan dari dashboard."
	}
	msgID, code, errMsg := deliverAPIMessage(id, to, apiMessageReq{To: to, Type: "text", Text: text})
	if errMsg != "" {
		c.JSON(code, gin.H{"error": errMsg})
		return
	}
	c.JSON(200, gin.H{
		"status":     "sent",
		"to":         to,
		"type":       "text",
		"message_id": msgID,
		"note":       "Jalur pengiriman sama dengan POST /api/v1/messages. Integrasi eksternal tetap memakai Authorization: Bearer <API_KEY>.",
	})
}

// TestWebhook mengirim event contoh bertanda tangan agar user dapat memvalidasi endpointnya dari dashboard.
func TestWebhook(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var agent models.Agent
	if database.DB.First(&agent, id).Error != nil || strings.TrimSpace(agent.WebhookURL) == "" {
		c.JSON(400, gin.H{"error": "Simpan URL webhook terlebih dahulu."})
		return
	}
	body, _ := json.Marshal(gin.H{
		"event": "webhook.test", "agent_id": agent.ID, "number": agent.Number,
		"timestamp": time.Now().Unix(), "message": "Webhook SlaluDiskon berhasil terhubung.",
	})
	req, err := newSignedWebhookRequest(agent.WebhookURL, agent.WebhookSecret, bytes.NewReader(body))
	if err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyiapkan request webhook."})
		return
	}
	resp, err := webhookClient.Do(req)
	if err != nil {
		c.JSON(502, gin.H{"error": "Endpoint webhook tidak dapat dihubungi: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.JSON(502, gin.H{"error": fmt.Sprintf("Endpoint webhook membalas HTTP %d.", resp.StatusCode)})
		return
	}
	c.JSON(200, gin.H{"status": "delivered", "http_status": resp.StatusCode})
}
