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
	c.JSON(200, gin.H{
		"partner_id": cfg.PartnerID,
		"base_url":   cfg.BaseURL,
		"token_set":  cfg.Token != "",
	})
}

// SaveLincahConfig — simpan kredensial dari UI (partner-id + token + mode).
func SaveLincahConfig(c *gin.Context) {
	agentID, ok := lincahAgentID(c)
	if !ok {
		return
	}
	var req struct {
		PartnerID string `json:"partner_id"`
		Token     string `json:"token"`
		BaseURL   string `json:"base_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "body tidak valid"})
		return
	}
	if err := services.LincahSaveConfig(agentID, strings.TrimSpace(req.PartnerID), strings.TrimSpace(req.Token), strings.TrimSpace(req.BaseURL)); err != nil {
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
