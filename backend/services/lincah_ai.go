// Lincah AI Shipping — jembatan antara pertanyaan ongkir customer dan data
// Lincah NYATA. Angka tarif SELALU diambil dari API Lincah (tidak pernah
// dikarang), lalu dirangkai menjadi blok ONGKIR_REALTIME yang dipahami
// persona AI (pola yang sudah ada di sistem untuk Mengantar).
//
// Alur:
//  1. Customer tanya ongkir → deteksi intent + teks tujuan (di handlers)
//  2. LincahSearchDistrict: teks tujuan → kode kecamatan Lincah
//     (GET /district/search?q=...)
//  3. Gudang terdekat: geocode alamat/kota customer (OSM Nominatim, cache)
//     → pilih gudang terdekat dari GET /address (haversine)
//  4. Lincah /ongkir dari gudang terdekat; bila kurir tidak menjangkau
//     (kosong), coba gudang berikutnya (maks. 3)
//  5. Susun blok dengan ekspedisi UTAMA & FALLBACK sesuai konfigurasi
//     per agent → AI hanya merangkai kalimat dari angka resmi ini.
package services

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// ---------------------------------------------------------------------------
// Geocoding (OSM Nominatim) — untuk memilih gudang terdekat customer
// ---------------------------------------------------------------------------

type geoPoint struct {
	Lat, Long float64
}

var geoCache sync.Map // key: query → geoPoint (berakhir 24 jam)

type geoCacheEntry struct {
	Point     geoPoint
	ExpiresAt time.Time
}

// geocodeOSM mengubah teks lokasi → koordinat via OpenStreetMap Nominatim
// (tanpa kunci API; batas pakai wajar ≈ 1 req/detik + cache 24 jam).
func geocodeOSM(place string) (geoPoint, error) {
	key := strings.ToLower(strings.TrimSpace(place))
	if cached, ok := geoCache.Load(key); ok {
		entry := cached.(geoCacheEntry)
		if time.Now().Before(entry.ExpiresAt) {
			return entry.Point, nil
		}
		geoCache.Delete(key)
	}
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest("GET", "https://nominatim.openstreetmap.org/search", nil)
	if err != nil {
		return geoPoint{}, err
	}
	q := req.URL.Query()
	q.Set("q", place)
	q.Set("format", "json")
	q.Set("limit", "1")
	q.Set("countrycodes", "id")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "CRM-Dashboard-Shipping/1.0 (contact: admin@example.com)")
	resp, err := client.Do(req)
	if err != nil {
		return geoPoint{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return geoPoint{}, err
	}
	var out []struct {
		Lat string `json:"lat"`
		Lon string `json:"lon"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out) == 0 {
		return geoPoint{}, fmt.Errorf("lokasi tidak ditemukan")
	}
	var pt geoPoint
	fmt.Sscanf(out[0].Lat, "%f", &pt.Lat)
	fmt.Sscanf(out[0].Lon, "%f", &pt.Long)
	geoCache.Store(key, geoCacheEntry{Point: pt, ExpiresAt: time.Now().Add(24 * time.Hour)})
	return pt, nil
}

// haversineKm = jarak permukaan bumi dalam kilometer.
func haversineKm(a, b geoPoint) float64 {
	const r = 6371.0
	dLat := (b.Lat - a.Lat) * math.Pi / 180
	dLong := (b.Long - a.Long) * math.Pi / 180
	la1 := a.Lat * math.Pi / 180
	la2 := b.Lat * math.Pi / 180
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dLong/2)*math.Sin(dLong/2)
	return 2 * r * math.Asin(math.Sqrt(h))
}

// ---------------------------------------------------------------------------
// Konfigurasi AI Ongkir (bagian dari LincahConfig)
// ---------------------------------------------------------------------------

// lincahAISettings — preferensi ekspedisi AI per agent.
type lincahAISettings struct {
	Preferred []string // urutan ekspedisi utama (mis. ["jne","sap"])
	Fallback  []string // ekspedisi cadangan bila utama tidak menjangkau
	Mode      string   // "nearest" | "first" | "fixed"
	FixedWh   string   // id gudang bila Mode = fixed
}

func parseCourierList(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// LincahAIEnabled — apakah agent ini mengaktifkan AI-ongkir Lincah.
func LincahAIEnabled(agentID uint) bool {
	var cfg models.LincahConfig
	if err := database.DB.Where("agent_id = ?", agentID).First(&cfg).Error; err != nil {
		return false
	}
	return cfg.AIEnabled
}

func lincahAISettingsFor(agentID uint) lincahAISettings {
	var cfg models.LincahConfig
	if err := database.DB.Where("agent_id = ?", agentID).First(&cfg).Error; err != nil {
		return lincahAISettings{Mode: "nearest"}
	}
	mode := cfg.WarehouseMode
	if mode == "" {
		mode = "nearest"
	}
	return lincahAISettings{
		Preferred: parseCourierList(cfg.PreferredCouriers),
		Fallback:  parseCourierList(cfg.FallbackCouriers),
		Mode:      mode,
		FixedWh:   cfg.FixedWarehouseID,
	}
}

// geocodePlace dapat diganti di tes (stub) agar tidak menyentuh jaringan.
var geocodePlace = geocodeOSM

// LincahShippingBlock membangun blok konteks ongkir dari data Lincah nyata.
// Return (blok, ok): ok=true berarti blok REALTIME dengan tarif asli siap
// dipakai AI; ok=false berarti blok tidak diproduksi (fallthrough ke
// penyedia lain atau tanpa blok).
func LincahShippingBlock(agentID uint, destText string) (string, bool) {
	cfg := LincahGetConfig(agentID)
	if cfg.Token == "" {
		return "", false
	}
	settings := lincahAISettingsFor(agentID)

	// 1) Resolusi kecamatan tujuan
	districts, err := LincahSearchDistrict(agentID, destText)
	if err != nil || len(districts) == 0 {
		return "\n\nONGKIR_NOTFOUND: Customer menanyakan ongkir ke \"" + destText + "\" tapi lokasi tersebut tidak dikenali sistem pengiriman. Minta customer mengecek ulang nama kota/kecamatan (JANGAN mengarang tarif).", true
	}
	d := districts[0]
	districtCode := d.Code
	districtLabel := d.Name
	if d.City != "" {
		districtLabel = fmt.Sprintf("%s, %s %s", d.Name, d.CityType, d.City)
	}

	// 2) Daftar gudang + urutan (terdekat bila memungkinkan)
	warehouses, err := LincahAddresses(agentID)
	if err != nil || len(warehouses) == 0 {
		return "\n\nONGKIR_EMPTY: Sistem pengiriman aktif tapi belum ada gudang/alamat pengirim terdaftar di akun Lincah. Jangan mengarang tarif; sampaikan bahwa gudang pengirim belum dikonfigurasi.", true
	}
	ordered := orderWarehouses(agentID, warehouses, destText, settings)

	// 3) Cek ongkir dari gudang terbaik (maks 3 percobaan bila tak menjangkau)
	var chosen LincahAddress
	var costs []LincahOngkirCost
	for i, wh := range ordered {
		if i >= 3 {
			break
		}
		res, err := LincahOngkir(agentID, LincahOngkirRequest{
			IsPickup: true, IsCod: false,
			Dimensions:  []int{10, 10, 10},
			Weight:      1000, // asumsi 1 kg standar pertanyaan ongkir
			Origin:      wh.OriginID,
			Destination: districtCode,
		})
		if err == nil && len(res) > 0 {
			chosen, costs = wh, res
			break
		}
	}
	if len(costs) == 0 {
		return "\n\nONGKIR_NOTFOUND: Tarif ke " + districtLabel + " belum tersedia dari gudang yang terdaftar (ekspedisi mungkin belum menjangkau area tujuan). Katakan jujur bahwa area tersebut belum terjangkau — JANGAN mengarang tarif.", true
	}

	// 4) Susun tarif urut preferensi: UTAMA dulu, lalu FALLBACK, lalu sisanya
	rows := orderedCostRows(costs, settings)
	if len(rows) == 0 {
		return "", false
	}

	var sb strings.Builder
	sb.WriteString("\n\nONGKIR_REALTIME:\n")
	sb.WriteString("Tujuan: " + districtLabel + " (kode " + districtCode + "). Gudang pengirim: " + warehouseLabel(chosen) + ".\n")
	sb.WriteString("TARIF RESMI Lincah (jangan mengarang angka lain):\n")
	for _, r := range rows {
		sb.WriteString(fmt.Sprintf("- %s %s: Rp %s\n", r.CourierName, r.Service, formatRupiah(r.Cost)))
	}
	if len(settings.Preferred) > 0 {
		sb.WriteString("EKSPEDISI UTAMA: " + strings.ToUpper(strings.Join(settings.Preferred, ", ")) + ". ")
	}
	if len(settings.Fallback) > 0 {
		sb.WriteString("FALLBACK (jika utama tak tersedia): " + strings.ToUpper(strings.Join(settings.Fallback, ", ")) + ". ")
	}
	sb.WriteString("\nOVERRIDE PERSONA: data ONGKIR_REALTIME ini adalah sumber RESMI dan FINAL. TAMPILKAN tarif LANGSUNG ke customer — JANGAN bilang 'akan dicek'. Jangan mengarang ekspedisi atau harga lain. Sebutkan nama kurirnya.")
	return sb.String(), true
}

type costRow struct {
	CourierName string
	Service     string
	Cost        int64
}

func orderedCostRows(costs []LincahOngkirCost, settings lincahAISettings) []costRow {
	rank := func(code string) int {
		for i, c := range settings.Preferred {
			if strings.EqualFold(code, c) {
				return i
			}
		}
		for i, c := range settings.Fallback {
			if strings.EqualFold(code, c) {
				return 100 + i
			}
		}
		return 200
	}
	rows := []costRow{}
	for _, c := range costs {
		for _, item := range c.Costs {
			rows = append(rows, costRow{CourierName: c.Name, Service: item.Type, Cost: item.Cost})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := rank(costsCodeOf(rows[i].CourierName, costs)), rank(costsCodeOf(rows[j].CourierName, costs))
		if ri != rj {
			return ri < rj
		}
		return rows[i].Cost < rows[j].Cost
	})
	return rows
}

func costsCodeOf(name string, costs []LincahOngkirCost) string {
	for _, c := range costs {
		if strings.EqualFold(c.Name, name) {
			return c.Code
		}
	}
	return name
}

// orderWarehouses mengurutkan gudang: fixed → id tertentu; first → urutan
// daftar; nearest → terdekat dengan lokasi customer (geocode; gagal → first).
func orderWarehouses(agentID uint, warehouses []LincahAddress, destText string, settings lincahAISettings) []LincahAddress {
	if settings.Mode == "fixed" && settings.FixedWh != "" {
		out := []LincahAddress{}
		for _, w := range warehouses {
			if w.ID == settings.FixedWh {
				out = append(out, w)
			}
		}
		for _, w := range warehouses {
			if w.ID != settings.FixedWh {
				out = append(out, w)
			}
		}
		return out
	}
	if settings.Mode != "nearest" {
		return warehouses
	}
	pt, err := geocodePlace(destText)
	if err != nil {
		return warehouses // tak bisa geocode → urutan daftar
	}
	type scored struct {
		addr LincahAddress
		km   float64
	}
	list := make([]scored, 0, len(warehouses))
	for _, w := range warehouses {
		wp := geoPoint{Lat: w.Geoloc.Lat, Long: w.Geoloc.Long}
		if wp.Lat == 0 && wp.Long == 0 {
			continue // gudang tanpa koordinat tidak bisa dinilai
		}
		list = append(list, scored{addr: w, km: haversineKm(wp, pt)})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].km < list[j].km })
	out := make([]LincahAddress, 0, len(warehouses))
	for _, s := range list {
		out = append(out, s.addr)
	}
	for _, w := range warehouses {
		found := false
		for _, s := range list {
			if s.addr.ID == w.ID {
				found = true
				break
			}
		}
		if !found {
			out = append(out, w)
		}
	}
	return out
}

func warehouseLabel(w LincahAddress) string {
	if w.Name != "" {
		return w.Name
	}
	if w.Address != "" {
		return w.Address
	}
	return w.ID
}
