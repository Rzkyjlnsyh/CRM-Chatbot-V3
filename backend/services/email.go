package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
)

// SendEmail mengirim email via Resend (https://resend.com).
// Konfigurasi: RESEND_API_KEY di .env, EMAIL_FROM opsional (default "SlaluDiskon <admin@slaludiskon.com>").
func SendEmail(to, subject, htmlBody string) error {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		log.Println("SendEmail: RESEND_API_KEY belum diset, skip kirim. Gunakan app di production dengan kunci nyata.")
		return nil
	}
	from := os.Getenv("EMAIL_FROM")
	if from == "" {
		from = "SlaluDiskon <admin@slaludiskon.com>"
	}

	payload := map[string]any{
		"from":    from,
		"to":      []string{to},
		"subject": subject,
		"html":    htmlBody,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("gagal membuat request Resend: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("gagal mengirim email via Resend: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp struct{ Message string }
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("Resend error %d: %s", resp.StatusCode, errResp.Message)
	}
	return nil
}
