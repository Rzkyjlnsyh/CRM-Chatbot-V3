// LincahWebhookHandler — endpoint PUBLIK yang dipanggil Lincah.
// URL untuk ditempel di portal Lincah (Webhook Settings):
//
//	https://<domain>/api/lincah/webhook
//
// Opsional: isi env LINCAH_WEBHOOK_SECRET dan minta developer Lincah
// menambahkan header X-Lincah-Secret senilai itu (validasi ringan).
package handlers

import (
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"wa-assistant/backend/config"
	"wa-assistant/backend/services"
)

// LincahWebhook menerima event status dari Lincah dan meneruskannya ke
// mesin follow-up. SELALU balas 200 (best effort) — mencegah retry spam.
func LincahWebhook(c *gin.Context) {
	secret := config.Env("LINCAH_WEBHOOK_SECRET", "")
	if secret != "" {
		if c.GetHeader("X-Lincah-Secret") != secret {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "secret tidak valid"})
			return
		}
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body tidak terbaca"})
		return
	}
	if strings.TrimSpace(string(raw)) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body kosong"})
		return
	}
	_, perr := services.LincahProcessWebhook(raw)
	if perr != nil {
		// Best effort: tetap 200 + catat ringkasan error untuk diagnostik.
		c.JSON(http.StatusOK, gin.H{"ok": true, "processed": false, "reason": perr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "processed": true})
}
