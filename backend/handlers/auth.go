package handlers

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var jwtSecret = mustJWTSecret()

const (
	loginMaxPairFailures = 5
	loginMaxIPFailures   = 25
	loginGenericError    = "Login belum berhasil"
)

// Durasi yang bisa diatur via env (default sama seperti sebelumnya).
var (
	loginWindow       = time.Duration(config.EnvInt("LOGIN_WINDOW_MIN", 10)) * time.Minute
	loginLockDuration = time.Duration(config.EnvInt("LOGIN_LOCK_MIN", 10)) * time.Minute
	dummyLoginHash    = []byte("$2a$10$QEUEZpKWWd3xV1qX7Q9BceA5.CgHCMOaOy3MpF8M/OIWYK8MKioJm")
)

type loginThrottleKey struct {
	key string
	max int
}

func mustJWTSecret() []byte {
	secret := strings.TrimSpace(config.EnvRequired("JWT_SECRET"))
	lower := strings.ToLower(secret)
	if len(secret) < 32 || lower == "wa-assistant-secret-change-me" || lower == "ganti_dengan_string_acak_min_32_char" || lower == "changeme" || lower == "change-me" || lower == "secret" {
		log.Fatal("ERROR: JWT_SECRET tidak aman; set minimal 32 karakter random dan jangan gunakan default")
	}
	return []byte(secret)
}

// CORS membatasi origin lewat env CORS_ALLOWED_ORIGINS (daftar dipisah koma).
// Default "*" hanya untuk development; di production wajib set origin asli.
func CORS() gin.HandlerFunc {
	allowed := config.Env("CORS_ALLOWED_ORIGINS", "*")
	if strings.EqualFold(config.Env("APP_ENV", "development"), "production") && allowed == "*" {
		log.Fatal("ERROR: CORS_ALLOWED_ORIGINS tidak boleh '*' saat APP_ENV=production")
	}
	var origins []string
	if allowed != "*" {
		for _, o := range strings.Split(allowed, ",") {
			if o = strings.TrimSpace(o); o != "" {
				origins = append(origins, o)
			}
		}
	}
	return func(c *gin.Context) {
		if allowed == "*" {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if origin := c.GetHeader("Origin"); origin != "" {
			for _, o := range origins {
				if origin == o {
					c.Header("Access-Control-Allow-Origin", origin)
					c.Header("Vary", "Origin")
					break
				}
			}
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}

// AuthMiddleware memvalidasi JWT dan menaruh identitas (user, tenant, role) ke context.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(401, gin.H{"error": "Unauthorized"})
			return
		}
		token, err := jwt.Parse(auth[7:], func(t *jwt.Token) (interface{}, error) { return jwtSecret, nil }, jwt.WithValidMethods([]string{"HS256"}))
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(401, gin.H{"error": "Invalid token"})
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(401, gin.H{"error": "Invalid token claims"})
			return
		}
		uidFloat, ok := claims["user_id"].(float64)
		if !ok || uidFloat <= 0 {
			c.AbortWithStatusJSON(401, gin.H{"error": "Invalid token claims"})
			return
		}

		var user models.User
		if err := database.DB.First(&user, uint(uidFloat)).Error; err != nil {
			c.AbortWithStatusJSON(401, gin.H{"error": "User tidak valid"})
			return
		}

		c.Set("user_id", user.ID)
		if user.TenantID != nil && *user.TenantID > 0 {
			c.Set("tenant_id", *user.TenantID)
		} else if user.IsSuperAdmin {
			c.Set("tenant_id", uint(1)) // superadmin tanpa tenant → fallback ke tenant default
		}
		c.Set("role", user.Role)
		c.Set("is_super_admin", user.IsSuperAdmin)
		c.Next()
	}
}

// RequireSuperAdmin memblokir request dari user non-super-admin.
// WAJIB dipasang SETELAH AuthMiddleware (mengandalkan flag "is_super_admin" di context).
// Dipakai untuk endpoint konfigurasi global (mis. API key AI) agar user biasa
// tidak bisa membaca/menimpa rahasia milik seluruh instalasi.
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSuper, _ := c.Get("is_super_admin"); isSuper != true {
			c.AbortWithStatusJSON(403, gin.H{"error": "Akses khusus super admin"})
			return
		}
		c.Next()
	}
}

// currentTenantID = tenant pemilik request (0 untuk super admin tanpa tenant).
func currentTenantID(c *gin.Context) uint {
	if v, ok := c.Get("tenant_id"); ok {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}

// currentTenantID = tenant pemilik request.
// Dipakai endpoint media karena <img>/<a> tidak bisa mengirim header Authorization.
func tenantFromToken(tokenStr string) (uint, bool) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) { return jwtSecret, nil }, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil || !token.Valid {
		return 0, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, false
	}
	if tid, ok := claims["tenant_id"].(float64); ok && tid > 0 {
		return uint(tid), true
	}
	return 0, false
}

// issueToken membuat JWT berisi identitas user (24 jam).
func issueToken(u models.User) string {
	claims := jwt.MapClaims{
		"user_id":        u.ID,
		"role":           u.Role,
		"is_super_admin": u.IsSuperAdmin,
		"exp":            time.Now().Add(time.Duration(config.EnvInt("TOKEN_TTL_HOURS", 24)) * time.Hour).Unix(),
	}
	if u.TenantID != nil {
		claims["tenant_id"] = *u.TenantID
	} else if u.IsSuperAdmin {
		claims["tenant_id"] = uint(1) // superadmin fallback ke tenant default
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	if err != nil {
		log.Printf("JWT issue token error: %v", err)
		return ""
	}
	return token
}

// issueMediaToken membuat JWT berumur pendek (30 menit) khusus akses media.
// Dipakai di URL <img>/<a> agar token utama (24 jam) tidak ikut bocor ke log/Referer.
func issueMediaToken(tenantID uint) string {
	claims := jwt.MapClaims{
		"tenant_id": tenantID,
		"scope":     "media",
		"exp":       time.Now().Add(time.Duration(config.EnvInt("MEDIA_TOKEN_TTL_MIN", 30)) * time.Minute).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	if err != nil {
		log.Printf("JWT issue media token error: %v", err)
		return ""
	}
	return token
}

func userResponse(u models.User) gin.H {
	return gin.H{
		"id":             u.ID,
		"name":           u.Name,
		"username":       u.Username,
		"email":          u.Email,
		"phone":          u.Phone,
		"email_verified": u.EmailVerified,
		"role":           u.Role,
		"is_super_admin": u.IsSuperAdmin,
		"tenant_id":      u.TenantID,
	}
}

func loginThrottleKeys(ip, username string) []loginThrottleKey {
	username = strings.ToLower(strings.TrimSpace(username))
	keys := make([]loginThrottleKey, 0, 2)
	if ip != "" {
		keys = append(keys, loginThrottleKey{key: "ip:" + ip, max: loginMaxIPFailures})
	}
	if ip != "" && username != "" {
		keys = append(keys, loginThrottleKey{key: "pair:" + ip + ":" + username, max: loginMaxPairFailures})
	}
	return keys
}

func checkLoginThrottle(ip, username string, now time.Time) time.Duration {
	var wait time.Duration
	for _, k := range loginThrottleKeys(ip, username) {
		var entry models.LoginThrottle
		err := database.DB.Where("`key` = ?", k.key).First(&entry).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			log.Printf("login throttle read error (%s): %v", k.key, err)
			continue
		}
		expired := now.Sub(entry.FirstSeen) > loginWindow && now.After(entry.LockedUntil)
		if expired {
			_ = database.DB.Delete(&entry).Error
			continue
		}
		if now.Before(entry.LockedUntil) {
			if w := entry.LockedUntil.Sub(now); w > wait {
				wait = w
			}
		}
	}
	return wait
}

func recordLoginFailure(ip, username string, now time.Time) {
	for _, k := range loginThrottleKeys(ip, username) {
		if err := database.DB.Transaction(func(tx *gorm.DB) error {
			var entry models.LoginThrottle
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("`key` = ?", k.key).First(&entry).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				entry = models.LoginThrottle{Key: k.key, FirstSeen: now}
			} else if err != nil {
				return err
			} else if now.Sub(entry.FirstSeen) > loginWindow && now.After(entry.LockedUntil) {
				entry.Failures = 0
				entry.FirstSeen = now
				entry.LockedUntil = time.Time{}
			}

			entry.Failures++
			if entry.Failures >= k.max {
				entry.LockedUntil = now.Add(loginLockDuration)
			}
			if entry.ID == 0 {
				return tx.Create(&entry).Error
			}
			return tx.Save(&entry).Error
		}); err != nil {
			log.Printf("login throttle write error (%s): %v", k.key, err)
		}
	}
}

func clearLoginPairThrottle(ip, username string) {
	username = strings.ToLower(strings.TrimSpace(username))
	if ip == "" || username == "" {
		return
	}
	_ = database.DB.Where("`key` = ?", "pair:"+ip+":"+username).Delete(&models.LoginThrottle{}).Error
}

// StartLoginThrottleSweeper menghapus entry throttle yang sudah kadaluarsa secara berkala,
// supaya tabel tidak tumbuh tanpa batas saat diserang banyak IP/username unik (botnet).
func StartLoginThrottleSweeper() {
	go func() {
		safeRun("cleanupLoginThrottle", cleanupLoginThrottle)
		t := time.NewTicker(loginWindow)
		defer t.Stop()
		for range t.C {
			safeRun("cleanupLoginThrottle", cleanupLoginThrottle)
		}
	}()
}

func cleanupLoginThrottle() {
	now := time.Now()
	cutoff := now.Add(-loginWindow)
	if err := database.DB.Where("first_seen < ? AND locked_until < ?", cutoff, now).Delete(&models.LoginThrottle{}).Error; err != nil {
		log.Printf("login throttle cleanup error: %v", err)
	}
}

func throttleLogin(c *gin.Context, wait time.Duration) {
	seconds := int(wait.Round(time.Second).Seconds())
	if seconds < 1 {
		seconds = 1
	}
	c.Header("Retry-After", strconv.Itoa(seconds))
	c.JSON(429, gin.H{"error": "Terlalu banyak percobaan. Coba lagi nanti."})
}

func Login(c *gin.Context) {
	start := time.Now()
	var req struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Turnstile string `json:"turnstile"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": loginGenericError})
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	ip := c.ClientIP()
	if wait := checkLoginThrottle(ip, req.Username, start); wait > 0 {
		throttleLogin(c, wait)
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": loginGenericError})
		return
	}

	var user models.User
	passwordHash := dummyLoginHash
	foundUser := false
	if err := database.DB.Where("username = ?", req.Username).First(&user).Error; err == nil {
		foundUser = true
		passwordHash = []byte(user.Password)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("Login DB lookup error: %v", err)
		c.JSON(500, gin.H{"error": loginGenericError})
		return
	}
	passwordOK := bcrypt.CompareHashAndPassword(passwordHash, []byte(req.Password)) == nil
	if !foundUser || !passwordOK {
		recordLoginFailure(ip, req.Username, start)
		c.JSON(401, gin.H{"error": loginGenericError})
		return
	}

	clearLoginPairThrottle(ip, req.Username)
	// Wajib verifikasi email sebelum masuk (super-admin dikecualikan).
	if !user.IsSuperAdmin && !user.EmailVerified {
		c.JSON(403, gin.H{
			"error":                 "Email kamu belum diverifikasi. Cek inbox atau folder spam untuk link aktivasi.",
			"verification_required": true,
			"email":                 user.Email,
		})
		return
	}
	c.JSON(200, gin.H{"token": issueToken(user), "user": userResponse(user)})
}

// Register tidak tersedia — instalasi internal perusahaan, user dibuat oleh superadmin.
// Gunakan SUPERADMIN_USERNAME / SUPERADMIN_PASSWORD dari .env untuk login pertama.

func Me(c *gin.Context) {
	var user models.User
	if database.DB.First(&user, c.GetUint("user_id")).Error != nil {
		c.JSON(404, gin.H{"error": "User tidak ditemukan"})
		return
	}
	resp := userResponse(user)
	if user.TenantID != nil {
		var t models.Tenant
		if database.DB.First(&t, *user.TenantID).Error == nil {
			resp["tenant"] = t
		}
	}
	c.JSON(200, resp)
}

// UpdateProfile hanya mengizinkan update Nama. Email & nomor tidak bisa diubah.
func UpdateProfile(c *gin.Context) {
	var user models.User
	if database.DB.First(&user, c.GetUint("user_id")).Error != nil {
		c.JSON(404, gin.H{"error": "User tidak ditemukan"})
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	user.Name = strings.TrimSpace(req.Name)
	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan"})
		return
	}
	c.JSON(200, gin.H{"user": userResponse(user)})
}

// ChangePassword mengganti password user (wajib verifikasi password lama).
func ChangePassword(c *gin.Context) {
	var user models.User
	if database.DB.First(&user, c.GetUint("user_id")).Error != nil {
		c.JSON(404, gin.H{"error": "User tidak ditemukan"})
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if req.OldPassword == "" || req.NewPassword == "" {
		c.JSON(400, gin.H{"error": "Password lama & baru wajib diisi"})
		return
	}
	if len(req.NewPassword) < 8 {
		c.JSON(400, gin.H{"error": "Password baru minimal 8 karakter"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)) != nil {
		c.JSON(400, gin.H{"error": "Password lama salah"})
		return
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	user.Password = string(hash)
	if err := database.DB.Save(&user).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan password"})
		return
	}
	c.JSON(200, gin.H{"message": "Password berhasil diganti"})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
