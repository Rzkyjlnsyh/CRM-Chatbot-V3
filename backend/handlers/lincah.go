// Lincah handlers — endpoint dashboard untuk integrasi pengiriman Lincah.
// Semua endpoint di bawah /api/agents/:id/lincah/... dengan auth.
package handlers

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

// lint: helpers tenantFromAgentID berulang di banyak handler — pola yang sama.

func lincahAgentID(c *gin.Context) (uint, bool) {
	agentID := currentAgentID(c)
	if agentID == 0 {
		c.JSON(401, gin.H{"error": "sesi tidak valid"})
		return 0, false
	}
	return agentID, true
}

// ---------------------------------------------------------------------------
// Konfigurasi
// ---------------------------------------------------------------------------

// GetLincahConfig — baca konfigurasi tersimpan (token TIDAK dikirim ke
// browser; hanya ketersediaannya: "token_set").
func GetLincahConfig(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	var cfg models.LincahConfig
	_ = database.DB.Where("agent_id = ?", agentID).First(&cfg).Error
	var tcfg models.LincahTenantConfig
	_ = database.DB.Where("tenant_id = ?", currentTenantID(c)).First(&tcfg).Error
	c.JSON(200, gin.H{
		"partner_id":         cfg.PartnerID,
		"base_url":           cfg.BaseURL,
		"token_set":          cfg.Token != "",
		"ai_enabled":         cfg.AIEnabled,
		"preferred_couriers": cfg.PreferredCouriers,
		"fallback_couriers":  cfg.FallbackCouriers,
		"warehouse_mode":     cfg.WarehouseMode,
		"fixed_warehouse_id": cfg.FixedWarehouseID,
		// Kredensial bersama (satu akun Lincah untuk semua nomor WA)
		"tenant_partner_id": tcfg.PartnerID,
		"tenant_token_set":  tcfg.Token != "",
		"tenant_base_url":   tcfg.BaseURL,
	})
}

// SaveLincahConfig — simpan kredensial dari UI. scope=tenant → kredensial
// dipakai BERSAMA semua agent (1 akun Lincah); default → khusus agent ini.
func SaveLincahConfig(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	var req struct {
		PartnerID         string `json:"partner_id"`
		Token             string `json:"token"`
		BaseURL           string `json:"base_url"`
		AIEnabled         bool   `json:"ai_enabled"`
		PreferredCouriers string `json:"preferred_couriers"`
		FallbackCouriers  string `json:"fallback_couriers"`
		WarehouseMode     string `json:"warehouse_mode"`
		FixedWarehouseID  string `json:"fixed_warehouse_id"`
		Scope             string `json:"scope"` // "" | "agent" | "tenant"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "body tidak valid"})
		return
	}
	if req.Scope == "tenant" {
		if err := services.LincahSaveTenantConfig(currentTenantID(c),
			strings.TrimSpace(req.PartnerID), strings.TrimSpace(req.Token), strings.TrimSpace(req.BaseURL)); err != nil {
			c.JSON(500, gin.H{"error": "gagal menyimpan kredensial bersama: " + err.Error()})
			return
		}
	}
	if err := services.LincahSaveConfigFull(agentID, services.LincahConfigUpdate{
		PartnerID: strings.TrimSpace(req.PartnerID),
		Token:     strings.TrimSpace(req.Token),
		BaseURL:   strings.TrimSpace(req.BaseURL),
		AIEnabled: req.AIEnabled,
		Preferred: req.PreferredCouriers,
		Fallback:  req.FallbackCouriers,
		WhMode:    req.WarehouseMode,
		FixedWh:   req.FixedWarehouseID,
	}); err != nil {
		c.JSON(500, gin.H{"error": "gagal menyimpan: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// TestLincahConnection — uji kredensial dengan GET /me + /balance.
func TestLincahConnection(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	me, err := services.LincahMe(agentID)
	if err != nil {
		c.JSON(400, gin.H{"ok": false, "error": err.Error()})
		return
	}
	bal, _ := services.LincahBalance(agentID)
	out := gin.H{"ok": true, "name": me.Name, "email": me.Email, "phone": me.Phone}
	if bal != nil {
		out["balance"] = bal.Balance
	}
	c.JSON(200, out)
}

// ---------------------------------------------------------------------------
// Data Lincah
// ---------------------------------------------------------------------------

// LincahListAddresses — daftar gudang/alamat sender (GET /address).
func LincahListAddresses(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	addrs, err := services.LincahAddresses(agentID)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": addrs})
}

// LincahListCouriers — daftar kurir.
func LincahListCouriers(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	items, err := services.LincahCouriers(agentID)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": items})
}

// LincahCheckOngkir — POST /ongkir (tarif semua kurir).
func LincahCheckOngkir(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	var req services.LincahOngkirRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "body tidak valid"})
		return
	}
	if req.Origin == "" || req.Destination == "" || req.Weight <= 0 {
		c.JSON(400, gin.H{"error": "asal, tujuan, dan berat wajib diisi"})
		return
	}
	costs, err := services.LincahOngkir(agentID, req)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": costs})
}

// ---------------------------------------------------------------------------
// Pesanan
// ---------------------------------------------------------------------------

// LincahCreateOrder — buat pesanan + simpan audit trail di tabel lokal.
func LincahCreateOrder(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	var req services.LincahOrderPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "body tidak valid"})
		return
	}
	if req.Name == "" || req.Phone == "" || req.Address == "" || req.Destination == "" || req.Courier == "" {
		c.JSON(400, gin.H{"error": "nama, telepon, alamat, tujuan, dan kurir wajib diisi"})
		return
	}
	res, err := services.LincahCreateOrder(agentID, req)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	rawJSON, _ := json.Marshal(res)
	record := models.LincahOrder{
		AgentID:       agentID,
		Sender:        c.Query("sender"),
		LincahOrderID: res.ID,
		NoOrder:       res.NoOrder,
		Resi:          res.Resi,
		Status:        res.Status,
		Courier:       res.Courier,
		CourierSvc:    res.CourierService,
		Name:          res.Name,
		Phone:         res.Phone,
		Address:       res.Address,
		Destination:   res.Destination,
		ProductName:   res.ProductName,
		ProductPrice:  res.ProductPrice,
		Weight:        res.Weight,
		Fee:           feeOf(res.Onkir),
		RawJSON:       string(rawJSON),
	}
	_ = database.DB.Create(&record).Error
	c.JSON(200, gin.H{"data": res, "local_order_id": record.ID})
}

func feeOf(onkir *services.LincahOnkir) int64 {
	if onkir == nil {
		return 0
	}
	return onkir.Fee
}

// LincahListLocalOrders — pesanan yang dibuat dari dashboard ini.
func LincahListLocalOrders(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	var rows []models.LincahOrder
	q := database.DB.Where("agent_id = ?", agentID)
	if s := strings.TrimSpace(c.Query("sender")); s != "" {
		q = q.Where("sender = ?", s)
	}
	if err := q.Order("id DESC").Limit(100).Find(&rows).Error; err != nil {
		c.JSON(500, gin.H{"error": "gagal baca pesanan"})
		return
	}
	c.JSON(200, gin.H{"data": rows})
}

// LincahOrderDetail — detail pesanan dari Lincah (id = lincah order id/no_order).
func LincahOrderDetail(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	res, err := services.LincahGetOrder(agentID, c.Param("id"))
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": res})
}

// LincahOrderTrack — lacak status pengiriman.
func LincahOrderTrack(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	tr, err := services.LincahTrack(agentID, c.Param("id"))
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": tr})
}

// LincahOrderCancel — batalkan pesanan.
func LincahOrderCancel(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	if err := services.LincahCancelOrder(agentID, c.Param("id")); err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

// LincahSearchDistricts — cari kecamatan (min. 3 huruf) untuk form tujuan.
func LincahSearchDistricts(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	q := strings.TrimSpace(c.Query("q"))
	if len([]rune(q)) < 3 {
		c.JSON(400, gin.H{"error": "ketik minimal 3 huruf"})
		return
	}
	items, err := services.LincahSearchDistrict(agentID, q)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": items})
}

// LincahChatQuote — satu panggilan siap-chat: gudang pertama, tujuan dari
// teks, tarif semua kurir, teks ringkas. Dipakai tombol 'Cek Ongkir' di chat
// maupun (nanti) jalur AI.
func LincahChatQuote(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	var req struct {
		Dest       string  `json:"dest"`
		WeightKg   float64 `json:"weight_kg"`
		Dimensions []int   `json:"dimensions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "body tidak valid"})
		return
	}
	grams := int64(req.WeightKg * 1000)
	if grams < 100 {
		grams = 1000 // default 1 kg bila tak diisi
	}
	quote, err := services.LincahQuoteForChat(agentID, req.Dest, grams, req.Dimensions)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": quote})
}

// LincahOrderPrint — URL PDF label resi.
func LincahOrderPrint(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	url, err := services.LincahPrintOrder(agentID, c.Param("id"))
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"pdf_url": url})
}

// bantu: guard time — dipakai oleh produser untuk mengikuti pola lain.
var _ = time.Now
