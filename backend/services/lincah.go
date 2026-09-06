// Lincah API Service — integrasi pengiriman paket via Lincah (OpenAPI v1.1.6).
// Dokumentasi: "Lincah Open API Doc - v1.1.6 - QAHIRA.pdf"
//
// Konfigurasi (partner-id + token + base URL) dibaca dari database (model
// LincahConfig) sehingga bisa diatur dari UI dashboard — bukan env.
// Semua request wajib header: Authorization: Bearer <token> + partner-id.
//
// Fitur yang dimapping dari dokumen:
//   - Me             GET  /me                → info akun partner
//   - Balance        GET  /balance           → saldo
//   - Couriers       GET  /courier           → daftar kurir
//   - Addresses      GET  /address           → daftar gudang/alamat sender
//   - Ongkir         POST /ongkir            → estimasi tarif semua kurir
//   - CreateOrder    POST /order             → buat pesanan + resi
//   - GetOrder       GET  /order/:id         → detail pesanan (id/no_order)
//   - Track          GET  /order/:id/track   → lacak status (berbagai kurir)
//   - CancelOrder    POST /order/cancel      → batalkan pesanan
//   - PrintOrder     POST /order/print       → label resi (URL PDF)
//   - Regenerate     POST /order/regenerate  → ulang generate no. resi

package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// ---------------------------------------------------------------------------
// Konfigurasi & klien HTTP
// ---------------------------------------------------------------------------

const lincahDefaultBase = "https://dev-api.lincah.id/openapi"

// LincahConfigProfile = snapshot kredensial yang dipakai untuk 1 seri request.
type LincahConfigProfile struct {
	PartnerID string
	Token     string
	BaseURL   string
}

// LincahGetConfig mengambil kredensial dari DB (per agent); bila belum ada,
// fallback ke env (mengantisipasi deployment yang konfigurasinya di env).
func LincahGetConfig(agentID uint) LincahConfigProfile {
	var cfg models.LincahConfig
	err := database.DB.Where("agent_id = ?", agentID).First(&cfg).Error
	if err != nil || cfg.Token == "" {
		return LincahConfigProfile{
			PartnerID: config.Env("LINCAH_PARTNER_ID", ""),
			Token:     config.Env("LINCAH_TOKEN", ""),
			BaseURL:   config.Env("LINCAH_BASE_URL", lincahDefaultBase),
		}
	}
	base := cfg.BaseURL
	if base == "" {
		base = lincahDefaultBase
	}
	return LincahConfigProfile{PartnerID: cfg.PartnerID, Token: cfg.Token, BaseURL: base}
}

// LincahSaveConfig menyimpan kredensial ke DB (upsert per agent).
func LincahSaveConfig(agentID uint, partnerID, token, baseURL string) error {
	if baseURL == "" {
		baseURL = lincahDefaultBase
	}
	var cfg models.LincahConfig
	err := database.DB.Where("agent_id = ?", agentID).First(&cfg).Error
	if err != nil {
		cfg = models.LincahConfig{AgentID: agentID}
	}
	cfg.PartnerID = partnerID
	cfg.Token = token
	cfg.BaseURL = baseURL
	if cfg.ID == 0 {
		return database.DB.Create(&cfg).Error
	}
	return database.DB.Save(&cfg).Error
}

// LincahAPIError = galat yang dikembalikan server Lincah (status != 2xx).
type LincahAPIError struct {
	Status  int
	Message string
}

func (e *LincahAPIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("lincah: %s (HTTP %d)", e.Message, e.Status)
	}
	return fmt.Sprintf("lincah: HTTP %d", e.Status)
}

type lincahRequest struct {
	cfg     LincahConfigProfile
	method  string
	path    string
	payload any
	query   map[string]string
}

// lincahClient = satu klien HTTP dengan timeout & header standar.
func lincahClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

// lincahDo menjalankan request ke API Lincah dan mengembalikan body mentah + HTTP status.
func lincahDo(req lincahRequest) ([]byte, int, error) {
	if req.cfg.Token == "" {
		return nil, 0, fmt.Errorf("lincah: token belum diatur (isi Pengaturan → Integrasi Lincah)")
	}
	body, err := json.Marshal(req.payload)
	if err != nil {
		return nil, 0, err
	}
	var reader io.Reader
	if req.method == http.MethodPost || req.method == http.MethodPut {
		reader = bytes.NewReader(body)
	}
	url := req.cfg.BaseURL + req.path
	if len(req.query) > 0 {
		url += "?" + urlQuery(req.query)
	}
	hreq, err := http.NewRequest(req.method, url, reader)
	if err != nil {
		return nil, 0, err
	}
	hreq.Header.Set("Authorization", "Bearer "+req.cfg.Token)
	if req.cfg.PartnerID != "" {
		hreq.Header.Set("partner-id", req.cfg.PartnerID)
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json")
	resp, err := lincahClient().Do(hreq)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return raw, resp.StatusCode, &LincahAPIError{Status: resp.StatusCode, Message: lincahErrMessage(raw)}
	}
	return raw, resp.StatusCode, nil
}

func lincahErrMessage(raw []byte) string {
	var v struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(raw, &v) == nil {
		if v.Message != "" {
			return v.Message
		}
		if v.Error != "" {
			return v.Error
		}
	}
	return string(raw)
}

func urlQuery(m map[string]string) string {
	q := ""
	for k, v := range m {
		if q != "" {
			q += "&"
		}
		q += k + "=" + v
	}
	return q
}

// ---------------------------------------------------------------------------
// Tipe data API (sesuai dokumen v1.1.6)
// ---------------------------------------------------------------------------

type LincahMeResult struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Phone string `json:"phone"`
	Name  string `json:"name"`
}

type LincahBalanceResult struct {
	Balance int64 `json:"balance"`
}

type LincahCourier struct {
	ID       string   `json:"code,omitempty"`
	Code     string   `json:"_id,omitempty"`
	Name     string   `json:"name"`
	Services []string `json:"services,omitempty"`
}

type LincahAddress struct {
	ID       string `json:"_id"`
	Name     string `json:"name"`
	Zipcode  string `json:"zipcode"`
	Address  string `json:"address"`
	OriginID string `json:"origin_id"`
	Geoloc   struct {
		Lat  float64 `json:"lat"`
		Long float64 `json:"long"`
	} `json:"geoloc"`
}

// LincahOngkirRequest = body POST /ongkir (lihat dok: isPickup, isCod,
// dimensions, weight, origin.code, destination.code, logistics, services).
type LincahOngkirRequest struct {
	IsPickup     bool     `json:"isPickup"`
	IsCod        bool     `json:"isCod"`
	Dimensions   []int    `json:"dimensions"`
	Weight       int64    `json:"weight"` // gram (mengikuti contoh dokumen)
	PackagePrice int64    `json:"packagePrice,omitempty"`
	Origin       string   `json:"origin_code"`
	Destination  string   `json:"destination_code"`
	Logistics    []string `json:"logistics,omitempty"`
	Services     []string `json:"services,omitempty"`
}

type LincahOngkirCost struct {
	Code     string                 `json:"code"`
	Name     string                 `json:"name"`
	Courier  string                 `json:"courier,omitempty"`
	Costs    []LincahOngkirCostItem `json:"costs"`
	OriginID string                 `json:"origin_id,omitempty"`
	DestID   string                 `json:"dst_id,omitempty"`
}

type LincahOngkirCostItem struct {
	Type     string `json:"type"`
	Code     string `json:"code"`
	Cost     int64  `json:"cost"`
	CostReal int64  `json:"costReal"`
	Etc      string `json:"etc,omitempty"`
}

type LincahOrderPayload struct {
	SenderType     string `json:"sender_type"`
	AddressRef     string `json:"address_ref"`
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Address        string `json:"address"`
	Destination    string `json:"destination"`
	Type           string `json:"type"` // 'cod' | 'regular'
	Courier        string `json:"courier"`
	CourierService string `json:"courier_service"`
	CodPrice       int64  `json:"cod_price,omitempty"`
	ProductPrice   int64  `json:"product_price,omitempty"`
	Weight         int64  `json:"weight"` // kg
	Quantity       int64  `json:"quantity"`
	Volume         string `json:"volume,omitempty"` // "PxLxT"
	ProductName    string `json:"product_name"`
	Note           string `json:"note,omitempty"`
	Email          string `json:"email,omitempty"`
	PickedUpTime   string `json:"picked_up_time,omitempty"`
	SenderName     string `json:"sender_name,omitempty"`
	SenderPhone    string `json:"sender_phone,omitempty"`
	PrintName      string `json:"print_name,omitempty"`
	PrintPhone     string `json:"print_phone,omitempty"`
	IsInsurance    bool   `json:"isInsurance,omitempty"`
}

type LincahOnkir struct {
	Fee       int64 `json:"fee"`
	FeeReal   int64 `json:"feeReal"`
	CodFee    int64 `json:"codFee"`
	Discount  int64 `json:"discount"`
	Insurance int64 `json:"insurance"`
}

type LincahSender struct {
	Name     string        `json:"name"`
	Phone    string        `json:"phone"`
	Address  string        `json:"address"`
	OriginID string        `json:"origin_id"`
	Zipcode  string        `json:"zipcode"`
	Geoloc   *LincahGeoloc `json:"geoloc"`
}

type LincahGeoloc struct {
	Lat  float64 `json:"lat"`
	Long float64 `json:"long"`
}

type LincahOrderResult struct {
	ID             string        `json:"id"`
	NoOrder        string        `json:"no_order"`
	User           string        `json:"user"`
	Name           string        `json:"name"`
	Phone          string        `json:"phone"`
	Address        string        `json:"address"`
	Weight         int64         `json:"weight"`
	Courier        string        `json:"courier"`
	CourierService string        `json:"courier_service"`
	Quantity       int64         `json:"quantity"`
	ProductPrice   int64         `json:"product_price"`
	ProductName    string        `json:"product_name"`
	Note           string        `json:"note"`
	Status         string        `json:"status"`
	Type           string        `json:"type"`
	Destination    string        `json:"destination_text"`
	DestinationID  string        `json:"destination_id"`
	SenderType     string        `json:"sender_type"`
	Sender         *LincahSender `json:"sender"`
	Resi           string        `json:"resi"`
	TLC            string        `json:"tlc"`
	Onkir          *LincahOnkir  `json:"ongkir"`
}

type LincahTrackingEvent struct {
	Status  string   `json:"status"`
	Time    string   `json:"time"`
	Message string   `json:"message"`
	Images  []string `json:"images"`
}

type LincahTracking struct {
	Order LincahTrackingOrder   `json:"order"`
	Data  []LincahTrackingEvent `json:"data"`
}

type LincahTrackingOrder struct {
	NoOrder         string       `json:"no_order"`
	Resi            string       `json:"resi"`
	Weight          int64        `json:"weight"`
	Volume          string       `json:"volume"`
	Courier         string       `json:"courier"`
	CourierService  string       `json:"courier_service"`
	Onkir           *LincahOnkir `json:"ongkir"`
	OriginText      string       `json:"origin_text"`
	OriginID        string       `json:"origin_id"`
	DestinationText string       `json:"destination_text"`
	DestinationID   string       `json:"destination_id"`
}

type lincahEnvelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Order   json.RawMessage `json:"order"`
}

// ---------------------------------------------------------------------------
// Operasi API — satu fungsi per endpoint dokumen
// ---------------------------------------------------------------------------

// LincahMe — info akun partner (pemakaian pertama: uji kredensial).
func LincahMe(agentID uint) (*LincahMeResult, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "GET", path: "/me"})
	if err != nil {
		return nil, err
	}
	var env lincahEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Success && len(env.Data) > 0 {
		raw = env.Data
	}
	var out LincahMeResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lincah parse me: %w", err)
	}
	return &out, nil
}

// LincahBalance — saldo akun.
func LincahBalance(agentID uint) (*LincahBalanceResult, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "GET", path: "/balance"})
	if err != nil {
		return nil, err
	}
	var env lincahEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Success && len(env.Data) > 0 {
		raw = env.Data
	}
	var out LincahBalanceResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lincah parse balance: %w", err)
	}
	return &out, nil
}

// LincahCouriers — daftar kurir yang tersedia di akun.
func LincahCouriers(agentID uint) ([]LincahCourier, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "GET", path: "/courier"})
	if err != nil {
		return nil, err
	}
	var env lincahEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Success && len(env.Data) > 0 {
		raw = env.Data
	}
	var out []LincahCourier
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lincah parse couriers: %w", err)
	}
	return out, nil
}

// LincahAddresses — daftar gudang/alamat sender yang terdaftar (GET /address).
func LincahAddresses(agentID uint) ([]LincahAddress, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "GET", path: "/address"})
	if err != nil {
		return nil, err
	}
	var env lincahEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Success && len(env.Data) > 0 {
		raw = env.Data
	}
	var out []LincahAddress
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lincah parse addresses: %w", err)
	}
	return out, nil
}

// LincahOngkir — cek tarif semua kurir untuk pasangan asal → tujuan.
func LincahOngkir(agentID uint, req LincahOngkirRequest) ([]LincahOngkirCost, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "POST", path: "/ongkir", payload: req})
	if err != nil {
		return nil, err
	}
	var env lincahEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Success && len(env.Data) > 0 {
		raw = env.Data
	}
	var out []LincahOngkirCost
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lincah parse ongkir: %w", err)
	}
	return out, nil
}

// LincahCreateOrder — buat pesanan pengiriman (otomatis dapat no resi).
func LincahCreateOrder(agentID uint, req LincahOrderPayload) (*LincahOrderResult, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "POST", path: "/order", payload: req})
	if err != nil {
		return nil, err
	}
	var env lincahEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Success && len(env.Data) > 0 {
		raw = env.Data
	}
	var out LincahOrderResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lincah parse create order: %w", err)
	}
	return &out, nil
}

// LincahGetOrder — detail pesanan berdasarkan id/no_order.
func LincahGetOrder(agentID uint, id string) (*LincahOrderResult, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "GET", path: "/order/" + id})
	if err != nil {
		return nil, err
	}
	var env lincahEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Success && len(env.Data) > 0 {
		raw = env.Data
	}
	var out LincahOrderResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lincah parse get order: %w", err)
	}
	return &out, nil
}

// LincahTrack — lacak status pengiriman (id = no_order atau resi).
func LincahTrack(agentID uint, id string) (*LincahTracking, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "GET", path: "/order/" + id + "/track"})
	if err != nil {
		return nil, err
	}
	var out LincahTracking
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("lincah parse track: %w", err)
	}
	return &out, nil
}

// LincahCancelOrder — batalkan pesanan (id/no_order).
type lincahIDPayload struct {
	ID string `json:"id"`
}

func LincahCancelOrder(agentID uint, id string) error {
	cfg := LincahGetConfig(agentID)
	_, status, err := lincahDo(lincahRequest{cfg: cfg, method: "POST", path: "/order/cancel", payload: lincahIDPayload{ID: id}})
	if err != nil {
		return err
	}
	if status >= 300 {
		return &LincahAPIError{Status: status}
	}
	return nil
}

// LincahPrintOrder — URL PDF label resi.
func LincahPrintOrder(agentID uint, id string) (string, error) {
	cfg := LincahGetConfig(agentID)
	raw, _, err := lincahDo(lincahRequest{cfg: cfg, method: "POST", path: "/order/print", payload: lincahIDPayload{ID: id}})
	if err != nil {
		return "", err
	}
	var out struct {
		Success bool   `json:"success"`
		Data    string `json:"data"`
		URL     string `json:"url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("lincah parse print: %w", err)
	}
	if out.URL != "" {
		return out.URL, nil
	}
	return out.Data, nil
}

// LincahRegenerate — minta nomor resi ulang bila pembuatan sempat gagal.
func LincahRegenerate(agentID uint, id string) error {
	cfg := LincahGetConfig(agentID)
	_, status, err := lincahDo(lincahRequest{cfg: cfg, method: "POST", path: "/order/regenerate", payload: lincahIDPayload{ID: id}})
	if err != nil {
		return err
	}
	if status >= 300 {
		return &LincahAPIError{Status: status}
	}
	return nil
}
