// Mengantar API Service — integrasi penuh dengan app.mengantar.com
// Dokumentasi: https://app.mengantar.com/docs/
//
// Fitur:
//   - SearchAddress: cari kota/kecamatan/kelurahan
//   - GetMyAddresses: daftar alamat pickup tersimpan
//   - AddAddress: tambah alamat pickup
//   - AddPickupTime: jadwalkan waktu pickup
//   - EstimateShipping: cek ongkir (single courier)
//   - EstimateAllPublic: cek ongkir semua kurir (public, no auth)
//   - CreateOrder: booking pengiriman → dapat nomor resi otomatis
//   - GetOrders: daftar pesanan + filter + tracking
//   - GetTracking: lacak resi (single order + history lengkap)
//   - PayUnpaid: bayar order yang belum terbayar
//   - GetMyUsers: daftar assignee
//   - GetInvoices: saldo & tagihan

package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"wa-assistant/backend/config"
)

const mengantarBaseURL = "https://app.mengantar.com"

// ---------------------------------------------------------------------------
// Konfigurasi
// ---------------------------------------------------------------------------

func mengantarAPIKey() string {
	return config.Env("MENGANTAR_API_KEY", "")
}

func mengantarClient() *http.Client {
	return &http.Client{Timeout: 25 * time.Second}
}

// ---------------------------------------------------------------------------
// Tipe data bersama
// ---------------------------------------------------------------------------

// MengantarAddress adalah hasil search address
type MengantarAddress struct {
	ID                 string `json:"_id"`
	ProvinceName       string `json:"PROVINCE_NAME"`
	CityName           string `json:"CITY_NAME"`
	CityNameSI         string `json:"CITY_NAME_SI"`
	DistrictName       string `json:"DISTRICT_NAME"`
	SubdistrictName    string `json:"SUBDISTRICT_NAME"`
	ZipCode            string `json:"ZIP_CODE"`
	DestinationCode    string `json:"DESTINATION_CODE"`
	DestinationCodeSI  string `json:"DESTINATION_CODE_SI"`
	OriginCode         string `json:"ORIGIN_CODE"`
	OriginCodeSI       string `json:"ORIGIN_CODE_SI"`
	CodeSAP            string `json:"CODE_SAP"`
	CountryName        string `json:"COUNTRY_NAME"`
	ClosedSi           bool   `json:"closedSi"`
	UnsupportedSi      bool   `json:"unsupportedSi"`
}

// MengantarSavedAddress adalah alamat tersimpan user
type MengantarSavedAddress struct {
	ID                     string `json:"_id"`
	PickupName             string `json:"PICKUP_NAME"`
	PickupPIC              string `json:"PICKUP_PIC"`
	PickupPICPhone         string `json:"PICKUP_PIC_PHONE"`
	PickupAddress          string `json:"PICKUP_ADDRESS"`
	PickupDistrict         string `json:"PICKUP_DISTRICT"`
	PickupSubdistrict      string `json:"PICKUP_SUBDISTRICT"`
	PickupRegion           string `json:"PICKUP_REGION"`
	PickupCity             string `json:"PICKUP_CITY"`
	PickupCitySI           string `json:"PICKUP_CITY_SI"`
	PickupZip              string `json:"PICKUP_ZIP"`
	PickupAutofill         string `json:"PICKUP_AUTOFILL"`
	PickupDestinationCode  string `json:"PICKUP_DESTINATION_CODE"`
	PickupDestinationCodeSI string `json:"PICKUP_DESTINATION_CODE_SI"`
	PickupOriginCode       string `json:"PICKUP_ORIGIN_CODE"`
	PickupOriginCodeSI     string `json:"PICKUP_ORIGIN_CODE_SI"`
	PickupSAPCode          string `json:"PICKUP_SAP_CODE"`
	PickupFullAutofill     string `json:"PICKUP_FULL_AUTOFILL"`
	UserID                 string `json:"user_id"`
}

// ShippingEstimateResult hasil cek ongkir
type ShippingEstimateResult struct {
	Price                    int    `json:"price"`
	Currency                 string `json:"currency"`
	DiscountPercent          int    `json:"discountPercent"`
	Discount                 int    `json:"discount"`
	CodFee                   int    `json:"codFee"`
	EstimatedPrice           int    `json:"estimatedPrice"`
	EstimatedSpecialPrice    int    `json:"estimatedSpecialPrice"`
	EstimatedDate            string `json:"estimatedDate"`
	EstimateDelivery         string `json:"estimate_delivery"`
	Unsupported              bool   `json:"unsupported"`
	UnsupportedCod           bool   `json:"unsupported_cod"`
	IsFlat                   bool   `json:"isFlat,omitempty"`
	Weight                   string `json:"weight,omitempty"`
	CargoDiscountPercent     int    `json:"cargoDiscountPercent,omitempty"`
	CargoDiscount            int    `json:"cargoDiscount,omitempty"`
	CargoEstimatedPrice      int    `json:"cargoEstimatedPrice,omitempty"`
	CargoEstimatedSpecialPrice int  `json:"cargoEstimatedSpecialPrice,omitempty"`
	DiscountExtraPercent     int    `json:"discountExtraPercent,omitempty"`
	DiscountExtra            int    `json:"discountExtra,omitempty"`
	CargoDiscountExtraPercent int   `json:"cargoDiscountExtraPercent,omitempty"`
	CargoDiscountExtra       int    `json:"cargoDiscountExtra,omitempty"`
}

// AllEstimateResult adalah hasil cek ongkir semua kurir (public)
type AllEstimateResult map[string]ShippingEstimateResult

// ---------------------------------------------------------------------------
// Order types
// ---------------------------------------------------------------------------

// MengantarOrderRequest adalah payload untuk membuat order
type MengantarOrderRequest struct {
	Courier string              `json:"courier"`
	Pickup  MengantarPickup     `json:"pickup"`
	Orders  []MengantarOrderItem `json:"orders"`
}

// MengantarPickup data pickup
type MengantarPickup struct {
	Type      string `json:"type"`      // "scheduledPickup" | "dropOff"
	Volume    string `json:"volume"`    // "volumeMotor"
	AddressID string `json:"address_id"` // _id dari saved address
	TimeID    string `json:"time_id"`   // _id dari POST /time
}

// MengantarOrderItem satu item dalam order
type MengantarOrderItem struct {
	Assignee               string                   `json:"assignee,omitempty"`
	COD                    int                      `json:"COD,omitempty"`
	GoodsValue             int                      `json:"goodsValue,omitempty"`
	CustomerAddressDataID  string                   `json:"customerAddressDataId"`
	CustomerAddress        string                   `json:"customerAddress"`
	CustomerName           string                   `json:"customerName"`
	CustomerPhone          string                   `json:"customerPhone"`
	Weight                 float64                  `json:"weight"`
	Quantity               int                      `json:"quantity"`
	ParcelContent          string                   `json:"parcelContent"`
	DestinationMark        string                   `json:"destinationMark,omitempty"`
	DeliveryInstruction    string                   `json:"deliveryInstruction,omitempty"`
	DontIncludeSubdistrict bool                     `json:"dontIncludeSubdistrict"`
	Cargo                  bool                     `json:"cargo,omitempty"`
	CustomProducts         []MengantarCustomProduct `json:"customProducts,omitempty"`
}

// MengantarCustomProduct rincian produk dalam order
type MengantarCustomProduct struct {
	Name    string  `json:"name"`
	Variant string  `json:"variant,omitempty"`
	Qty     int     `json:"qty"`
	Price   int     `json:"price,omitempty"`
	Weight  float64 `json:"weight,omitempty"`
}

// MengantarOrderResult hasil create order
type MengantarOrderResult struct {
	ID                       string  `json:"_id"`
	OrderID                  string  `json:"ORDER_ID"`
	CnoteNo                  string  `json:"cnote_no"`
	Batch                    string  `json:"batch"`
	BatchID                  string  `json:"batch_id"`
	Courier                  string  `json:"courier"`
	Status                   string  `json:"status"`
	StatusCategory           string  `json:"statusCategory"`
	QueueStatus              string  `json:"queueStatus"`
	IsPaid                   bool    `json:"isPaid"`
	CodAmount                int     `json:"COD_AMOUNT"`
	CodFee                   float64 `json:"COD_FEE,omitempty"`
	GoodsAmount              int     `json:"GOODS_AMOUNT"`
	Weight                   float64 `json:"WEIGHT"`
	EstimatedPrice           float64 `json:"estimatedPrice"`
	EstimatedSpecialPrice    float64 `json:"estimatedSpecialPrice"`
	EstimateDelivery         string  `json:"estimate_delivery"`
	Discount                 float64 `json:"discount"`
	Error                    any     `json:"error"`
	CreatedAt                string  `json:"createdAt"`
	CreatedDate              string  `json:"createdDate"`
	PickupName               string  `json:"PICKUP_NAME"`
	PickupAddress            string  `json:"PICKUP_ADDRESS"`
	PickupCity               string  `json:"PICKUP_CITY"`
	PickupRegion             string  `json:"PICKUP_REGION"`
	PickupDistrict           string  `json:"PICKUP_DISTRICT"`
	PickupDate               string  `json:"PICKUP_DATE"`
	PickupTime               string  `json:"PICKUP_TIME"`
	PickupPICPhone           string  `json:"PICKUP_PIC_PHONE"`
	ServiceCode              string  `json:"SERVICE_CODE"`
	Cargo                    bool    `json:"cargo"`
	IsPreviouslyError        bool    `json:"isPreviouslyError"`
	IsReconciliated          bool    `json:"isReconciliated"`
}

// MengantarCreateOrderResponse adalah wrapper response
type MengantarCreateOrderResponse struct {
	Success bool                    `json:"success"`
	Data    []MengantarOrderResult  `json:"data"`
	Batch   string                  `json:"batch"`
	BatchID string                  `json:"batch_id"`
	Courier string                  `json:"courier"`
	Errors  []any                   `json:"errors"`
}

// ---------------------------------------------------------------------------
// Order list
// ---------------------------------------------------------------------------

// MengantarOrder adalah satu record order
type MengantarOrder struct {
	ID                 string                 `json:"_id"`
	OrderID            string                 `json:"ORDER_ID"`
	CnoteNo            string                 `json:"cnote_no"`
	Batch              string                 `json:"batch"`
	BatchID            string                 `json:"batch_id"`
	Courier            string                 `json:"courier"`
	Status             string                 `json:"status"`
	StatusCategory     string                 `json:"statusCategory"`
	CodAmount          int                    `json:"COD_AMOUNT"`
	GoodsAmount        int                    `json:"GOODS_AMOUNT"`
	Weight             float64                `json:"WEIGHT"`
	ReceiverName       string                 `json:"RECEIVER_NAME"`
	ReceiverAddr       string                 `json:"RECEIVER_ADDR1"`
	ReceiverCity       string                 `json:"RECEIVER_CITY"`
	ReceiverRegion     string                 `json:"RECEIVER_REGION"`
	ReceiverDistrict   string                 `json:"RECEIVER_DISTRICT"`
	ReceiverSubdistrict string                `json:"RECEIVER_SUBDISTRICT"`
	ReceiverPhone      string                 `json:"RECEIVER_PHONE"`
	GoodsDesc          string                 `json:"GOODS_DESC"`
	OriginCode         string                 `json:"ORIGIN_CODE"`
	DestinationCode    string                 `json:"DESTINATION_CODE"`
	EstimatedPrice     float64                `json:"estimatedPrice"`
	EstimatedSpecialPrice float64             `json:"estimatedSpecialPrice"`
	EstimateDelivery   string                 `json:"estimate_delivery"`
	Discount           float64                `json:"discount"`
	ShipperName        string                 `json:"SHIPPER_NAME"`
	ShipperContact     string                 `json:"SHIPPER_CONTACT"`
	CreatedAt          string                 `json:"createdAt"`
	LastStatusChange   string                 `json:"lastStatusChange"`
	IsDeleted          bool                   `json:"isDeleted"`
	TicketStatus       string                 `json:"ticketStatus"`
	History            []MengantarTrackingHistory `json:"history,omitempty"`
}

// MengantarTrackingHistory adalah satu entry lacak resi
type MengantarTrackingHistory struct {
	Date string `json:"date"` // format: "DD-MM-YYYY HH:mm"
	Desc string `json:"desc"`
}

// ---------------------------------------------------------------------------
// User / Assignee
// ---------------------------------------------------------------------------

// MengantarUser adalah satu assignee
type MengantarUser struct {
	ID    string `json:"_id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ---------------------------------------------------------------------------
// Pickup Time
// ---------------------------------------------------------------------------

// MengantarPickupTime adalah waktu pickup
type MengantarPickupTime struct {
	ID      string `json:"_id"`
	Date    string `json:"date"`
	Time    string `json:"time"`
	Status  string `json:"status"`
	IsSunday bool  `json:"isSunday"`
}

// ---------------------------------------------------------------------------
// API Wrapper
// ---------------------------------------------------------------------------

type mengantarAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func mengantarGet(path string, params url.Values) ([]byte, error) {
	key := mengantarAPIKey()
	if key == "" {
		return nil, fmt.Errorf("MENGANTAR_API_KEY belum dikonfigurasi")
	}
	u, _ := url.Parse(mengantarBaseURL + "/api/public/" + key + path)
	if params != nil {
		u.RawQuery = params.Encode()
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := mengantarClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi Mengantar: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Mengantar HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func mengantarPost(path string, payload any) ([]byte, error) {
	key := mengantarAPIKey()
	if key == "" {
		return nil, fmt.Errorf("MENGANTAR_API_KEY belum dikonfigurasi")
	}
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gagal marshal payload: %w", err)
	}
	u := mengantarBaseURL + "/api/public/" + key + path
	req, err := http.NewRequest("POST", u, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("gagal membuat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := mengantarClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal menghubungi Mengantar: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Mengantar HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// ---------------------------------------------------------------------------
// Public API (tanpa validasi API key — sesuai dokumentasi Mengantar)
// ---------------------------------------------------------------------------

// SearchAddressPublic mencari alamat tanpa otentikasi
func SearchAddressPublic(keyword string) ([]MengantarAddress, error) {
	u := mengantarBaseURL + "/api/public/any/address/search?keyword=" + url.QueryEscape(keyword)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := mengantarClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal search address: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var wrapper struct {
		Success bool               `json:"success"`
		Data    []MengantarAddress `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	return wrapper.Data, nil
}

// ---------------------------------------------------------------------------
// Authenticated API
// ---------------------------------------------------------------------------

// SearchAddress mencari alamat berdasarkan keyword
func SearchAddress(keyword string) ([]MengantarAddress, error) {
	body, err := mengantarGet("/address/search", url.Values{"keyword": {keyword}})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Success bool               `json:"success"`
		Data    []MengantarAddress `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	return wrapper.Data, nil
}

// GetMyAddresses mengambil daftar alamat pickup tersimpan
func GetMyAddresses() ([]MengantarSavedAddress, error) {
	body, err := mengantarGet("/address", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Success bool                    `json:"success"`
		Data    []MengantarSavedAddress `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	return wrapper.Data, nil
}

// AddAddress menambah alamat pickup baru
func AddAddress(req struct {
	PickupAutofill  string `json:"PICKUP_AUTOFILL"`
	PickupAddress   string `json:"PICKUP_ADDRESS"`
	PickupPICPhone  string `json:"PICKUP_PIC_PHONE"`
	PickupPIC       string `json:"PICKUP_PIC"`
	PickupName      string `json:"PICKUP_NAME"`
}) (*MengantarSavedAddress, error) {
	body, err := mengantarPost("/address", req)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Success bool                  `json:"success"`
		Data    MengantarSavedAddress `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	if !wrapper.Success {
		return nil, fmt.Errorf("gagal menambah alamat")
	}
	return &wrapper.Data, nil
}

// EstimateShipping cek ongkir untuk satu kurir
func EstimateShipping(originID, destinationID, courier string, weight float64, codAmount int) (*ShippingEstimateResult, error) {
	params := url.Values{
		"origin_id":      {originID},
		"destination_id": {destinationID},
		"courier":        {strings.ToUpper(courier)},
		"weight":         {fmt.Sprintf("%.1f", weight)},
	}
	if codAmount > 0 {
		params.Set("COD_AMOUNT", fmt.Sprintf("%d", codAmount))
	}
	body, err := mengantarGet("/order/estimate", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Success bool                    `json:"success"`
		Message string                  `json:"message"`
		Data    *ShippingEstimateResult `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	if !wrapper.Success || wrapper.Data == nil {
		msg := wrapper.Message
		if msg == "" {
			msg = "gagal cek ongkir"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return wrapper.Data, nil
}

// EstimateAllPublic cek ongkir semua kurir (public endpoint, diskon flat 20%)
func EstimateAllPublic(originID, destinationID string, weight float64, codAmount int) (AllEstimateResult, error) {
	u := mengantarBaseURL + "/api/order/allEstimatePublic"
	params := url.Values{
		"origin_id":      {originID},
		"destination_id": {destinationID},
		"weight":         {fmt.Sprintf("%.1f", weight)},
	}
	if codAmount > 0 {
		params.Set("COD_AMOUNT", fmt.Sprintf("%d", codAmount))
	}
	u += "?" + params.Encode()
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := mengantarClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal cek ongkir public: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var wrapper struct {
		Success bool             `json:"success"`
		Data    AllEstimateResult `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	return wrapper.Data, nil
}

// CreateOrder membuat pesanan pengiriman dan mendapatkan nomor resi
func CreateOrder(req MengantarOrderRequest) (*MengantarCreateOrderResponse, error) {
	body, err := mengantarPost("/order", req)
	if err != nil {
		return nil, err
	}
	var resp MengantarCreateOrderResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("gagal membuat order")
	}
	return &resp, nil
}

// GetOrders mengambil daftar pesanan dengan filter
func GetOrders(page, size int, courier, cod, statusJSON, startDate, endDate, batchID, trackingID, orderID string) ([]MengantarOrder, error) {
	params := url.Values{
		"page": {fmt.Sprintf("%d", page)},
		"size": {fmt.Sprintf("%d", size)},
	}
	if courier != "" {
		params.Set("courier", courier)
	}
	if cod != "" {
		params.Set("cod", cod)
	}
	if statusJSON != "" {
		params.Set("status", statusJSON)
	}
	if startDate != "" && endDate != "" {
		params.Set("dateRange", fmt.Sprintf(`{"startDate":"%s","endDate":"%s"}`, startDate, endDate))
	}
	if batchID != "" {
		params.Set("batch", batchID)
	}
	if trackingID != "" {
		params.Set("tracking_id", trackingID)
	}
	if orderID != "" {
		params.Set("order_id", orderID)
	}

	body, err := mengantarGet("/order", params)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Success bool             `json:"success"`
		Data    []MengantarOrder `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	return wrapper.Data, nil
}

// GetTracking melacak satu resi dengan history lengkap
func GetTracking(trackingID string) (*MengantarOrder, error) {
	orders, err := GetOrders(1, 1, "", "", "", "", "", "", trackingID, "")
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, fmt.Errorf("resi %s tidak ditemukan", trackingID)
	}
	return &orders[0], nil
}

// GetTrackingByOrderID melacak order via ORDER_ID
func GetTrackingByOrderID(orderID string) (*MengantarOrder, error) {
	orders, err := GetOrders(1, 1, "", "", "", "", "", "", "", orderID)
	if err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return nil, fmt.Errorf("order %s tidak ditemukan", orderID)
	}
	return &orders[0], nil
}

// PayUnpaid membayar order yang belum terbayar
func PayUnpaid(courier, batchID string) (int, []string, error) {
	body, err := mengantarPost("/order/pay-unpaid", map[string]string{
		"courier":  courier,
		"batch_id": batchID,
	})
	if err != nil {
		return 0, nil, err
	}
	var wrapper struct {
		Success  bool     `json:"success"`
		Data     int      `json:"data"`
		CnoteNos []string `json:"cnote_no"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return 0, nil, fmt.Errorf("response tidak valid: %w", err)
	}
	return wrapper.Data, wrapper.CnoteNos, nil
}

// GetMyUsers mengambil daftar assignee
func GetMyUsers() ([]MengantarUser, error) {
	body, err := mengantarGet("/my-users", nil)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Success bool            `json:"success"`
		Data    []MengantarUser `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	return wrapper.Data, nil
}

// AddPickupTime menjadwalkan waktu pickup
func AddPickupTime(addressID, date, timeStr string) (*MengantarPickupTime, error) {
	body, err := mengantarPost("/time", map[string]string{
		"address_id": addressID,
		"date":       date,    // format MM-DD-YYYY
		"time":       timeStr, // 9:00, 10:00, ..., 18:00
	})
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Success bool               `json:"success"`
		Data    MengantarPickupTime `json:"data"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("response tidak valid: %w", err)
	}
	return &wrapper.Data, nil
}

// DeleteOrder menghapus order
func DeleteOrder(ids []string) error {
	_, err := mengantarPost("/order/delete", map[string]any{
		"ids": ids,
	})
	return err
}

// ---------------------------------------------------------------------------
// Konversi / helper
// ---------------------------------------------------------------------------

// ResolveMengantarCity mencari kota dari keyword dan mengembalikan hasil terbaik
func ResolveMengantarCity(query string) ([]MengantarAddress, error) {
	return SearchAddress(query)
}

// BestAddress mengambil address _id terbaik dari hasil search (cocokkan nama kota/kecamatan)
func BestAddress(results []MengantarAddress, targetDistrict, targetCity string) *MengantarAddress {
	// Cari kecocokan eksak dulu
	for i := range results {
		if strings.EqualFold(results[i].DistrictName, targetDistrict) &&
			strings.EqualFold(results[i].CityName, targetCity) {
			return &results[i]
		}
	}
	// Fallback: cocokkan city saja
	for i := range results {
		if strings.EqualFold(results[i].CityName, targetCity) {
			return &results[i]
		}
	}
	// Fallback terakhir: ambil hasil pertama
	if len(results) > 0 {
		return &results[0]
	}
	return nil
}
