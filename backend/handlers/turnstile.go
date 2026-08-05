package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
)

// verifyTurnstile memeriksa token Turnstile ke Cloudflare.
// Kalau TURNSTILE_SECRET_KEY belum diset, skip (dev mode).
func verifyTurnstile(token string) bool {
	if token == "" {
		return false
	}
	secret := os.Getenv("TURNSTILE_SECRET_KEY")
	if secret == "" {
		return true
	}
	resp, err := http.PostForm("https://challenges.cloudflare.com/turnstile/v0/siteverify", url.Values{
		"secret":   {secret},
		"response": {token},
	})
	if err != nil {
		log.Printf("Turnstile verify error: %v", err)
		return false
	}
	defer resp.Body.Close()
	var result struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false
	}
	return result.Success
}
