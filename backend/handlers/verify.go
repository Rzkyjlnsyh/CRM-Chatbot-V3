package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// VerifyEmail memproses tautan verifikasi email.
// GET /api/verify-email?token=xxx
func VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(400, gin.H{"error": "Token tidak ditemukan"})
		return
	}
	var user models.User
	if err := database.DB.Where("email_verify_token = ?", token).First(&user).Error; err != nil {
		c.JSON(404, gin.H{"error": "Token tidak valid atau sudah kadaluarsa"})
		return
	}
	user.EmailVerified = true
	user.EmailVerifyToken = ""
	if err := database.DB.Save(&user).Error; err != nil {
		log.Printf("Gagal verifikasi email user %d: %v", user.ID, err)
		c.JSON(500, gin.H{"error": "Gagal menyimpan"})
		return
	}
	html := `<!DOCTYPE html><html><head><meta charset="utf-8"><title>Email Terverifikasi</title><meta name="viewport" content="width=device-width,initial-scale=1"><style>body{font-family:-apple-system,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f0fdf4}div{text-align:center;padding:2rem}span{font-size:4rem}button{margin-top:1rem;padding:12px 32px;background:#16a34a;color:#fff;border:none;border-radius:8px;font-size:16px;cursor:pointer}a{text-decoration:none;color:#fff}</style></head><body><div><span>✅</span><h1 style="color:#166534">Email Terverifikasi</h1><p style="color:#4ade80">Akun kamu sudah aktif. Silakan login untuk melanjutkan.</p><a href="/login"><button>Login Sekarang</button></a></div></body></html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// ForgotPassword mengirim tautan reset password via email.
// POST /api/forgot-password
func ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		c.JSON(400, gin.H{"error": "Email wajib diisi"})
		return
	}
	var user models.User
	if err := database.DB.Where("email = ?", strings.TrimSpace(req.Email)).First(&user).Error; err != nil {
		// Jangan bocorkan apakah email terdaftar (security)
		c.JSON(200, gin.H{"message": "Kalau email terdaftar, tautan reset sudah dikirim"})
		return
	}

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		c.JSON(500, gin.H{"error": "Gagal menghasilkan token"})
		return
	}
	tokenStr := hex.EncodeToString(token)
	expiry := time.Now().Add(1 * time.Hour)
	user.PasswordResetToken = tokenStr
	user.PasswordResetExpiry = &expiry
	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan"})
		return
	}

	resetURL := config.Env("APP_URL", "http://localhost:8080") + "/reset-password?token=" + tokenStr
	resetHTML := `<div style="font-family:sans-serif;max-width:480px;margin:0 auto;padding:24px"><h2 style="color:#16a34a">Reset Password</h2><p>Klik tombol di bawah untuk mengatur ulang password kamu:</p><a href="` + resetURL + `" style="display:inline-block;padding:12px 24px;background:#16a34a;color:#fff;border-radius:8px;text-decoration:none;font-weight:600">Reset Password</a><p style="color:#6b7280;font-size:14px;margin-top:16px">Tautan berlaku 1 jam. Kalau kamu tidak meminta reset ini, abaikan saja.</p></div>`
	services.Go("SendEmail:reset", func() {
		if err := services.SendEmail(user.Email, "Reset Password SlaluDiskon", resetHTML); err != nil {
			log.Printf("Gagal kirim email reset ke %s: %v", user.Email, err)
		}
	})

	c.JSON(200, gin.H{"message": "Kalau email terdaftar, tautan reset sudah dikirim"})
}

// ResetPassword memproses perubahan password dari token reset.
// POST /api/reset-password
func ResetPassword(c *gin.Context) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Data tidak valid"})
		return
	}
	if len(req.Password) < 8 {
		c.JSON(400, gin.H{"error": "Password minimal 8 karakter"})
		return
	}
	var user models.User
	if err := database.DB.Where("password_reset_token = ?", req.Token).First(&user).Error; err != nil {
		c.JSON(404, gin.H{"error": "Token tidak valid"})
		return
	}
	if user.PasswordResetExpiry == nil || time.Now().After(*user.PasswordResetExpiry) {
		c.JSON(410, gin.H{"error": "Token sudah kadaluarsa"})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	user.Password = string(hash)
	user.PasswordResetToken = ""
	user.PasswordResetExpiry = nil
	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan"})
		return
	}
	c.JSON(200, gin.H{"message": "Password berhasil diubah. Silakan login."})
}

// generateVerifyToken membuat token acak untuk verifikasi email.
func generateVerifyToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// sendVerifyEmail mengirim email verifikasi (async). Token harus sudah terisi di user.
func sendVerifyEmail(user models.User) {
	if user.EmailVerifyToken == "" || user.Email == "" {
		return
	}
	verifyURL := config.Env("APP_URL", "https://slaludiskon.com") + "/api/verify-email?token=" + user.EmailVerifyToken
	go func() {
		defer services.RecoverGo("SendEmail:verify")
		if err := services.SendEmail(user.Email, "Verifikasi Email SlaluDiskon",
			`<div style="font-family:sans-serif;max-width:480px;margin:0 auto;padding:24px"><h2 style="color:#16a34a">Verifikasi Email</h2><p>Terima kasih sudah mendaftar di SlaluDiskon! Klik tombol di bawah untuk mengaktifkan akun kamu:</p><a href="`+verifyURL+`" style="display:inline-block;padding:12px 24px;background:#16a34a;color:#fff;border-radius:8px;text-decoration:none;font-weight:600">Verifikasi Email</a><p style="color:#6b7280;font-size:14px;margin-top:16px">Kalau kamu tidak mendaftar, abaikan email ini.</p></div>`); err != nil {
			log.Printf("Gagal kirim email verifikasi ke %s: %v", user.Email, err)
		}
	}()
}

// ResendVerification mengirim ulang email verifikasi.
// POST /api/resend-verification
func ResendVerification(c *gin.Context) {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Email) == "" {
		c.JSON(400, gin.H{"error": "Email wajib diisi"})
		return
	}
	var user models.User
	if err := database.DB.Where("email = ?", strings.TrimSpace(req.Email)).First(&user).Error; err == nil && !user.EmailVerified {
		user.EmailVerifyToken = generateVerifyToken()
		if err := database.DB.Save(&user).Error; err == nil {
			sendVerifyEmail(user)
		}
	}
	// Pesan generik supaya tidak membocorkan status pendaftaran email.
	c.JSON(200, gin.H{"message": "Kalau email terdaftar dan belum terverifikasi, link verifikasi sudah dikirim ulang."})
}
