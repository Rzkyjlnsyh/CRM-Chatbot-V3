package handlers

import (
	"regexp"
	"strings"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

func GetNumberStatus(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	m := services.WA(id)
	number, name := m.GetInfo()
	c.JSON(200, gin.H{
		"status":     m.GetStatus(),
		"qr":         m.GetQR(),
		"qr_ttl":     m.GetQRTTL(),
		"pair_code":  m.GetPairCode(),
		"pair_error": m.GetPairError(),
		"number":     number,
		"name":       name,
	})
}

var nonDigit = regexp.MustCompile(`\D`)

// normalizePairPhone menyiapkan nomor untuk PairPhone: buang non-digit, dan ubah awalan lokal
// Indonesia "0" jadi kode negara "62" (mayoritas pengguna mengetik 08xx).
func normalizePairPhone(raw string) string {
	d := nonDigit.ReplaceAllString(raw, "")
	if strings.HasPrefix(d, "0") {
		return "62" + strings.TrimPrefix(d, "0")
	}
	return d
}

// ConnectPairingNumber menyambungkan nomor lewat kode pairing (alternatif QR).
func ConnectPairingNumber(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Phone string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	phone := normalizePairPhone(req.Phone)
	if len(phone) < 8 {
		c.JSON(400, gin.H{"error": "Masukkan nomor WhatsApp yang valid, lengkap dengan kode negara."})
		return
	}
	var a models.Agent
	database.DB.First(&a, id)
	status, err := services.WA(id).ConnectPairing(a.DeviceJID, phone)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": status})
}

func ConnectNumber(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	// Sesi WA selalu diizinkan — instalasi internal tanpa langganan.
	var a models.Agent
	database.DB.First(&a, id)
	status, err := services.WA(id).Connect(a.DeviceJID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": status})
}

// CheckNumbersOnWA memvalidasi apakah nomor-nomor benar terdaftar di WhatsApp (pra-blast).
func CheckNumbersOnWA(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Numbers []string `json:"numbers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Numbers) == 0 {
		c.JSON(400, gin.H{"error": "Daftar nomor wajib diisi"})
		return
	}
	if len(req.Numbers) > 1000 {
		c.JSON(400, gin.H{"error": "Maksimal 1000 nomor per pengecekan"})
		return
	}
	if !services.WA(id).IsConnected() {
		c.JSON(400, gin.H{"error": "WhatsApp belum tersambung"})
		return
	}
	result, err := services.WA(id).CheckOnWhatsApp(req.Numbers)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	registered := make([]string, 0, len(result))
	notRegistered := make([]string, 0)
	for num, isIn := range result {
		if isIn {
			registered = append(registered, num)
		} else {
			notRegistered = append(notRegistered, num)
		}
	}
	c.JSON(200, gin.H{"data": gin.H{
		"results":          result,
		"registered":       registered,
		"not_registered":   notRegistered,
		"total":            len(result),
		"registered_count": len(registered),
	}})
}

// LogoutNumber memutus & menghapus sesi WhatsApp agent, lalu mengosongkan device tersimpan
// supaya tidak auto-reconnect ke sesi lama. Untuk menyambung lagi perlu scan QR.
func LogoutNumber(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	_ = services.WA(id).Logout()
	var a models.Agent
	if database.DB.First(&a, id).Error == nil {
		a.DeviceJID = ""
		a.Number = ""
		database.DB.Save(&a)
	}
	c.JSON(200, gin.H{"status": "disconnected"})
}
