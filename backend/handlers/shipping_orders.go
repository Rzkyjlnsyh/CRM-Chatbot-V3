package handlers

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// API: Cek Ongkir via Mengantar
// ---------------------------------------------------------------------------

// CheckShipping meng-handle request cek ongkir dari dashboard
// GET /api/agents/:id/shipping/estimate?destination=jakarta&weight=1000
func CheckShipping(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}

	var ag models.Agent
	if database.DB.First(&ag, id).Error != nil {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return
	}

	destQuery := strings.TrimSpace(c.Query("destination"))
	if destQuery == "" {
		c.JSON(400, gin.H{"error": "destination wajib (nama kota/kecamatan)"})
		return
	}

	weightStr := c.DefaultQuery("weight", "1000")
	weightGram, _ := strconv.Atoi(weightStr)
	if weightGram < 100 {
		weightGram = 100
	}
	weightKg := float64(weightGram) / 1000.0

	// Cari alamat tujuan
	addresses, err := services.SearchAddress(destQuery)
	if err != nil || len(addresses) == 0 {
		c.JSON(404, gin.H{"error": "Kota tidak ditemukan: " + destQuery})
		return
	}

	// Ambil origin — dari agent config dulu, fallback ke Mengantar API
	originID := strings.TrimSpace(ag.MengantarOriginAutofillID)
	if originID == "" {
		addrs, err := services.GetMyAddresses()
		if err == nil && len(addrs) > 0 {
			originID = addrs[0].PickupAutofill
			// Auto-save ke agent biar next time nggak fetch lagi
			ag.MengantarOriginAutofillID = originID
			ag.MengantarOriginAddressID = addrs[0].ID
			database.DB.Model(&ag).Updates(map[string]any{
				"mengantar_origin_autofill_id": originID,
				"mengantar_origin_address_id":  addrs[0].ID,
			})
			log.Printf("[shipping] Auto-config origin untuk agent %d: %s (%s)", id, addrs[0].PickupName, addrs[0].PickupCity)
		}
	}
	if originID == "" {
		c.JSON(400, gin.H{"error": "Alamat asal belum dikonfigurasi. Tambah alamat pickup di akun Mengantar (app.mengantar.com) terlebih dahulu."})
		return
	}

	type EstimateResult struct {
		Address   services.MengantarAddress          `json:"address"`
		Estimates map[string]any                     `json:"estimates"`
	}
	results := make([]EstimateResult, 0, len(addresses))
	if len(addresses) > 5 {
		addresses = addresses[:5]
	}

	for _, addr := range addresses {
		est, err := services.EstimateShipping(originID, addr.ID, "JNE", weightKg, 0)
		estData := map[string]any{}
		if err == nil && !est.Unsupported {
			estData["JNE"] = est
		}
		est2, err2 := services.EstimateShipping(originID, addr.ID, "JT", weightKg, 0)
		if err2 == nil && !est2.Unsupported {
			estData["JT"] = est2
		}

		results = append(results, EstimateResult{
			Address:   addr,
			Estimates: estData,
		})
	}

	c.JSON(200, gin.H{"success": true, "data": results})
}

// ---------------------------------------------------------------------------
// API: Search Address (Mengantar)
// ---------------------------------------------------------------------------

// SearchMengantarAddress mencari alamat via Mengantar API
// GET /api/shipping/search-address?keyword=jakarta
func SearchMengantarAddress(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("keyword"))
	if keyword == "" {
		c.JSON(400, gin.H{"error": "keyword wajib"})
		return
	}

	results, err := services.SearchAddress(keyword)
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal mencari alamat: " + err.Error()})
		return
	}

	if len(results) > 20 {
		results = results[:20]
	}
	c.JSON(200, gin.H{"success": true, "data": results})
}

// ---------------------------------------------------------------------------
// API: Get My Addresses
// ---------------------------------------------------------------------------

// GetMengantarAddresses mengambil daftar alamat pickup
// GET /api/shipping/addresses
func GetMengantarAddresses(c *gin.Context) {
	addrs, err := services.GetMyAddresses()
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal mengambil alamat: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": addrs})
}

// ---------------------------------------------------------------------------
// API: Get My Orders
// ---------------------------------------------------------------------------

// GetShippingOrders mengambil daftar pesanan pengiriman
// GET /api/agents/:id/shipping/orders?page=1&size=20&courier=JNE&status=active
func GetShippingOrders(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	query := database.DB.Where("agent_id = ?", id)

	if courier := c.Query("courier"); courier != "" {
		query = query.Where("courier = ?", courier)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	var total int64
	query.Model(&models.ShippingOrder{}).Count(&total)

	var orders []models.ShippingOrder
	query.Order("created_at DESC").Offset((page - 1) * size).Limit(size).Find(&orders)

	c.JSON(200, gin.H{
		"success": true,
		"data":    orders,
		"total":   total,
		"page":    page,
		"size":    size,
	})
}

// ---------------------------------------------------------------------------
// API: Get Shipping Order Detail (dengan tracking history)
// ---------------------------------------------------------------------------

// GetShippingOrderDetail mengambil detail satu order + tracking terbaru dari Mengantar
// GET /api/agents/:id/shipping/orders/:orderId
func GetShippingOrderDetail(c *gin.Context) {
	_, ok := resolveAgent(c)
	if !ok {
		return
	}

	orderID := c.Param("orderId")
	var order models.ShippingOrder
	if database.DB.Where("id = ? OR mengantar_order_id = ? OR cnote_no = ?", orderID, orderID, orderID).First(&order).Error != nil {
		c.JSON(404, gin.H{"error": "Order tidak ditemukan"})
		return
	}

	// Ambil tracking terbaru dari Mengantar
	trackingData := map[string]any{}
	if order.CnoteNo != "" {
		track, err := services.GetTracking(order.CnoteNo)
		if err == nil && track != nil {
			trackingData["history"] = track.History
			trackingData["status"] = track.Status
			trackingData["status_category"] = track.StatusCategory
			// Update local
			if len(track.History) > 0 {
				last := track.History[len(track.History)-1]
				database.DB.Model(&order).Updates(map[string]any{
					"status":              track.Status,
					"status_category":     track.StatusCategory,
					"last_tracking_date":  last.Date,
					"last_tracking_desc":  last.Desc,
				})
			}
		}
	}

	c.JSON(200, gin.H{
		"success":  true,
		"data":     order,
		"tracking": trackingData,
	})
}

// ---------------------------------------------------------------------------
// API: Create Shipping Order (auto-resi)
// ---------------------------------------------------------------------------

// CreateShippingOrder membuat pesanan pengiriman via Mengantar dan menyimpan ke DB
// POST /api/agents/:id/shipping/orders
func CreateShippingOrder(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}

	var req struct {
		Sender                string  `json:"sender"`                   // nomor WA customer
		Courier               string  `json:"courier"`                  // JNE atau JT
		CustomerName          string  `json:"customer_name"`
		CustomerAddress       string  `json:"customer_address"`
		CustomerPhone         string  `json:"customer_phone"`
		DestinationAddressID  string  `json:"destination_address_id"`   // _id dari /address/search
		OriginAddressID       string  `json:"origin_address_id"`        // _id saved address (optional)
		OriginAutofillID      string  `json:"origin_autofill_id"`       // PICKUP_AUTOFILL (optional)
		WeightGram            int     `json:"weight_gram"`
		Quantity              int     `json:"quantity"`
		ParcelContent         string  `json:"parcel_content"`
		GoodsValue            int     `json:"goods_value"`
		CodAmount             int     `json:"cod_amount"`
		DeliveryInstruction   string  `json:"delivery_instruction"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Data tidak valid"})
		return
	}

	if req.Courier == "" {
		req.Courier = "JNE"
	}
	req.Courier = strings.ToUpper(req.Courier)
	if req.Courier != "JNE" && req.Courier != "JT" {
		c.JSON(400, gin.H{"error": "Kurir hanya mendukung JNE dan JT"})
		return
	}
	if req.CustomerName == "" || req.CustomerAddress == "" || req.CustomerPhone == "" {
		c.JSON(400, gin.H{"error": "Nama, alamat, dan telepon customer wajib"})
		return
	}
	if req.DestinationAddressID == "" {
		c.JSON(400, gin.H{"error": "destination_address_id wajib (dari search address)"})
		return
	}
	if req.WeightGram < 100 {
		req.WeightGram = 1000
	}
	if req.Quantity < 1 {
		req.Quantity = 1
	}
	if req.ParcelContent == "" {
		req.ParcelContent = "Paket"
	}

	// Ambil origin address
	originAddrID := req.OriginAddressID
	originAutofillID := req.OriginAutofillID
	if originAddrID == "" || originAutofillID == "" {
		addrs, err := services.GetMyAddresses()
		if err == nil && len(addrs) > 0 {
			originAddrID = addrs[0].ID
			originAutofillID = addrs[0].PickupAutofill
		}
	}
	if originAddrID == "" || originAutofillID == "" {
		c.JSON(400, gin.H{"error": "Alamat asal tidak ditemukan. Konfigurasi alamat di Mengantar terlebih dahulu."})
		return
	}

	// Jadwalkan waktu pickup (H+1, jam 9 pagi)
	tomorrow := time.Now().Add(24 * time.Hour)
	dateStr := tomorrow.Format("01-02-2006")
	timeID, err := services.AddPickupTime(originAddrID, dateStr, "9:00")
	if err != nil {
		log.Printf("[shipping] Gagal jadwal pickup: %v — lanjut tanpa time_id", err)
		// Lanjut tanpa time_id
	}

	weightKg := float64(req.WeightGram) / 1000.0
	if weightKg < 0.1 {
		weightKg = 0.1
	}

	// Buat order via Mengantar
	orderReq := services.MengantarOrderRequest{
		Courier: req.Courier,
		Pickup: services.MengantarPickup{
			Type:      "scheduledPickup",
			Volume:    "volumeMotor",
			AddressID: originAddrID,
		},
		Orders: []services.MengantarOrderItem{{
			CustomerAddressDataID:  req.DestinationAddressID,
			CustomerAddress:        req.CustomerAddress,
			CustomerName:           req.CustomerName,
			CustomerPhone:          req.CustomerPhone,
			Weight:                 weightKg,
			Quantity:               req.Quantity,
			ParcelContent:          req.ParcelContent,
			GoodsValue:             req.GoodsValue,
			COD:                    req.CodAmount,
			DeliveryInstruction:    req.DeliveryInstruction,
			DontIncludeSubdistrict: false,
		}},
	}
	if timeID != nil {
		orderReq.Pickup.TimeID = timeID.ID
	}

	resp, err := services.CreateOrder(orderReq)
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal membuat order: " + err.Error()})
		return
	}

	// Simpan ke database lokal
	var savedOrders []models.ShippingOrder
	for _, o := range resp.Data {
		shippingCost := int(o.EstimatedPrice)
		shippingCostDiscounted := int(o.EstimatedSpecialPrice)
		so := models.ShippingOrder{
			AgentID:                  id,
			TenantID:                 1,
			ChatSender:               req.Sender,
			MengantarOrderID:         o.OrderID,
			MengantarBatchID:         o.BatchID,
			CnoteNo:                  o.CnoteNo,
			Courier:                  req.Courier,
			CustomerName:             req.CustomerName,
			CustomerAddress:          req.CustomerAddress,
			CustomerPhone:            req.CustomerPhone,
			DestinationAddressDataID: req.DestinationAddressID,
			OriginAddressID:          originAddrID,
			OriginAutofillID:         originAutofillID,
			WeightGram:               req.WeightGram,
			Quantity:                 req.Quantity,
			ParcelContent:            req.ParcelContent,
			GoodsValue:               req.GoodsValue,
			CodAmount:                req.CodAmount,
			ShippingCost:             shippingCost,
			ShippingCostDiscounted:   shippingCostDiscounted,
			Discount:                 shippingCost - shippingCostDiscounted,
			Status:                   o.Status,
			StatusCategory:           o.StatusCategory,
			IsPaid:                   o.IsPaid,
			CreatedAt:                time.Now(),
			UpdatedAt:                time.Now(),
		}
		database.DB.Create(&so)
		savedOrders = append(savedOrders, so)
	}

	c.JSON(200, gin.H{
		"success": true,
		"data":    savedOrders,
		"batch":   resp.Batch,
	})
}

// ---------------------------------------------------------------------------
// API: Sync Tracking (manual trigger)
// ---------------------------------------------------------------------------

// SyncShippingTracking memperbarui status tracking dari Mengantar
// POST /api/agents/:id/shipping/sync-tracking
func SyncShippingTracking(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}

	updated := syncTrackingForAgent(id)
	c.JSON(200, gin.H{"success": true, "updated": updated})
}

// ---------------------------------------------------------------------------
// Background: Auto-tracking sync
// ---------------------------------------------------------------------------

// StartShippingTrackingSync menjalankan sinkronisasi tracking secara periodik
func StartShippingTrackingSync() {
	go func() {
		ticker := time.NewTicker(3 * time.Hour)
		defer ticker.Stop()

		// Jalankan sekali saat startup
		time.Sleep(30 * time.Second)
		runTrackingSync()

		for range ticker.C {
			runTrackingSync()
		}
	}()
	log.Println("[shipping] Tracking sync started (every 3h)")
}

func runTrackingSync() {
	var agents []models.Agent
	database.DB.Find(&agents)
	totalUpdated := 0
	for _, ag := range agents {
		totalUpdated += syncTrackingForAgent(ag.ID)
	}
	if totalUpdated > 0 {
		log.Printf("[shipping] Tracking sync: %d orders updated", totalUpdated)
	}
}

func syncTrackingForAgent(agentID uint) int {
	var orders []models.ShippingOrder
	database.DB.Where("agent_id = ? AND status NOT IN (?, ?, ?)", agentID, "DELIVERED", "RTS", "CANCEL").Find(&orders)

	updated := 0
	for _, order := range orders {
		if order.CnoteNo == "" {
			continue
		}
		track, err := services.GetTracking(order.CnoteNo)
		if err != nil {
			continue
		}

		newStatus := track.Status
		if newStatus == order.Status && len(track.History) > 0 {
			last := track.History[len(track.History)-1]
			if last.Date == order.LastTrackingDate {
				continue // tidak ada perubahan
			}
		}

		updates := map[string]any{
			"status":          newStatus,
			"status_category": track.StatusCategory,
			"updated_at":      time.Now(),
		}
		if len(track.History) > 0 {
			last := track.History[len(track.History)-1]
			updates["last_tracking_date"] = last.Date
			updates["last_tracking_desc"] = last.Desc
		}

		database.DB.Model(&order).Updates(updates)
		updated++

		// Notifikasi ke customer kalau status berubah ke DELIVERED
		if newStatus == "DELIVERED" && !order.NotifiedDelivered {
			msg := fmt.Sprintf("Halo kak %s! Pesanan dengan resi *%s* (%s) sudah *TERKIRIM* ya kak. Terima kasih sudah order di kami! 🙏",
				order.CustomerName, order.CnoteNo, order.Courier)
			if order.ChatSender != "" {
				go func(sender, message string) {
					if err := services.WA(agentID).SendText(sender, message); err != nil {
						log.Printf("[shipping] Gagal kirim notif delivered ke %s: %v", sender, err)
					}
				}(order.ChatSender, msg)
			}
			database.DB.Model(&order).Update("notified_delivered", true)
		}

		// Notifikasi saat status PICKUP (pesanan sudah dijemput kurir)
		if (newStatus == "OUTGOING" || strings.Contains(newStatus, "PICK")) && !order.NotifiedPickedUp {
			msg := fmt.Sprintf("Halo kak %s! Pesanan dengan resi *%s* (%s) sudah dijemput kurir dan sedang dalam perjalanan. Estimasi: %s 🚚",
				order.CustomerName, order.CnoteNo, order.Courier, order.EstimatedDelivery)
			if order.ChatSender != "" {
				go func(sender, message string) {
					if err := services.WA(agentID).SendText(sender, message); err != nil {
						log.Printf("[shipping] Gagal kirim notif pickup ke %s: %v", sender, err)
					}
				}(order.ChatSender, msg)
			}
			database.DB.Model(&order).Update("notified_picked_up", true)
		}
	}
	return updated
}
