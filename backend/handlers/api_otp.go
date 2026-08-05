package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// OTP lewat REST API: generate + kirim + verifikasi. Auth API key per-nomor.

const otpRequestCooldown = 60 * time.Second // jeda minimal antar-permintaan OTP ke satu nomor
const otpMaxAttempts = 5                     // batas percobaan verifikasi salah sebelum kode dibuang

// generateOTPCode membuat kode numerik acak (crypto/rand) sepanjang n digit.
func generateOTPCode(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// Fallback yang sangat jarang terpakai; tetap numerik dari nanodetik.
		s := strconv.FormatInt(time.Now().UnixNano(), 10)
		for len(s) < n {
			s += "0"
		}
		return s[:n]
	}
	for i := range b {
		b[i] = digits[int(b[i])%10]
	}
	return string(b)
}

// hashOTP menghasilkan hash kode yang di-salt dengan agent+nomor (disimpan di DB).
func hashOTP(agentID uint, number, code string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s", agentID, number, code)))
	return hex.EncodeToString(sum[:])
}

// APIRequestOTP — POST /api/v1/otp/request
// Body: {"to","length"(4-8,def 6),"minutes"(1-30,def 5),"message"(opsional, pakai {code}/{minutes})}
func APIRequestOTP(c *gin.Context) {
	agent := apiAgent(c)
	var req struct {
		To      string `json:"to"`
		Length  int    `json:"length"`
		Minutes int    `json:"minutes"`
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Body JSON tidak valid."})
		return
	}
	to := services.NormalizePhone(req.To)
	if to == "" {
		c.JSON(400, gin.H{"error": "Field 'to' (nomor tujuan) wajib diisi."})
		return
	}
	length := req.Length
	if length < 4 || length > 8 {
		length = 6
	}
	minutes := req.Minutes
	if minutes < 1 || minutes > 30 {
		minutes = 5
	}

	// Cooldown anti-spam: tolak bila baru saja membuat kode untuk nomor ini.
	var recent models.OTPCode
	if database.DB.Where("agent_id = ? AND number = ? AND created_at > ?", agent.ID, to, time.Now().Add(-otpRequestCooldown)).
		Order("id desc").First(&recent).Error == nil {
		c.JSON(429, gin.H{"error": "Terlalu cepat. Tunggu sebentar sebelum meminta OTP lagi."})
		return
	}
	if !services.WA(agent.ID).IsConnected() {
		c.JSON(409, gin.H{"error": "Nomor WhatsApp sedang tidak tersambung."})
		return
	}

	code := generateOTPCode(length)
	// Hanya satu kode aktif per nomor: batalkan kode lama yang belum terpakai.
	database.DB.Model(&models.OTPCode{}).
		Where("agent_id = ? AND number = ? AND used = ?", agent.ID, to, false).Update("used", true)
	rec := models.OTPCode{
		AgentID:   agent.ID,
		Number:    to,
		CodeHash:  hashOTP(agent.ID, to, code),
		ExpiresAt: time.Now().Add(time.Duration(minutes) * time.Minute),
	}
	if err := database.DB.Create(&rec).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal membuat OTP"})
		return
	}

	msg := strings.TrimSpace(req.Message)
	if msg == "" || !strings.Contains(msg, "{code}") {
		msg = fmt.Sprintf("Kode verifikasi kamu: %s\n\nBerlaku %d menit. Jangan bagikan kode ini ke siapa pun.", code, minutes)
	} else {
		msg = strings.ReplaceAll(msg, "{code}", code)
		msg = strings.ReplaceAll(msg, "{minutes}", strconv.Itoa(minutes))
	}
	if err := services.WA(agent.ID).SendText(to, msg); err != nil {
		c.JSON(502, gin.H{"error": "Gagal mengirim OTP: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "sent", "to": to, "expires_in": minutes * 60})
}

// APIVerifyOTP — POST /api/v1/otp/verify  Body: {"to","code"}
// Selalu 200; hasil ada di field "verified" (true/false) + "reason" bila gagal.
func APIVerifyOTP(c *gin.Context) {
	agent := apiAgent(c)
	var req struct {
		To   string `json:"to"`
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Body JSON tidak valid."})
		return
	}
	to := services.NormalizePhone(req.To)
	code := strings.TrimSpace(req.Code)
	if to == "" || code == "" {
		c.JSON(400, gin.H{"error": "Field 'to' dan 'code' wajib diisi."})
		return
	}

	var rec models.OTPCode
	if database.DB.Where("agent_id = ? AND number = ? AND used = ?", agent.ID, to, false).
		Order("id desc").First(&rec).Error != nil {
		c.JSON(200, gin.H{"verified": false, "reason": "Tidak ada OTP aktif untuk nomor ini."})
		return
	}
	if time.Now().After(rec.ExpiresAt) {
		database.DB.Model(&rec).Update("used", true)
		c.JSON(200, gin.H{"verified": false, "reason": "OTP sudah kedaluwarsa."})
		return
	}
	if rec.Attempts >= otpMaxAttempts {
		database.DB.Model(&rec).Update("used", true)
		c.JSON(200, gin.H{"verified": false, "reason": "Terlalu banyak percobaan salah."})
		return
	}
	database.DB.Model(&rec).Update("attempts", rec.Attempts+1)

	if subtle.ConstantTimeCompare([]byte(rec.CodeHash), []byte(hashOTP(agent.ID, to, code))) != 1 {
		c.JSON(200, gin.H{"verified": false, "reason": "Kode salah."})
		return
	}
	database.DB.Model(&rec).Update("used", true) // sekali pakai
	c.JSON(200, gin.H{"verified": true})
}
