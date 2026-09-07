package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// lincahAIMockServer — mock untuk /district/search, /address, /ongkir dengan
// 3 gudang (Jakarta jauh, Jogja dekat, Surabaya tengah) + coverage selektif:
// gudang Jogja TIDAK melayani ongkir (respons kosong) → uji fallback.
func lincahAIMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	writeJSON := func(w http.ResponseWriter, s string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(s))
	}
	mux.HandleFunc("/district/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if strings.Contains(strings.ToLower(q), "bantul") {
			writeJSON(w, `{"success":true,"data":[{"code":"34.02.01","province":"DI Yogyakarta","city_type":"Kabupaten","city":"Bantul","name":"Bantul"}]}`)
			return
		}
		writeJSON(w, `{"success":true,"data":[]}`)
	})
	mux.HandleFunc("/address", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"success":true,"data":[
			{"_id":"wh-jkt","name":"Gudang Jakarta","address":"Jakarta Barat","origin_id":"31.73.01","geoloc":{"lat":-6.2000,"long":106.8166}},
			{"_id":"wh-jogja","name":"Gudang Yogyakarta","address":"Sleman","origin_id":"34.04.01","geoloc":{"lat":-7.7956,"long":110.3695}},
			{"_id":"wh-sby","name":"Gudang Surabaya","address":"Rungkut","origin_id":"35.78.14","geoloc":{"lat":-7.2575,"long":112.7521}}
		]}`)
	})
	mux.HandleFunc("/ongkir", func(w http.ResponseWriter, r *http.Request) {
		var pay map[string]any
		_ = json.NewDecoder(r.Body).Decode(&pay)
		// Gudang Jogja TIDAK menjangkau (respons kosong) → fallback diuji.
		if pay["origin_code"] == "34.04.01" {
			writeJSON(w, `{"success":true,"data":[]}`)
			return
		}
		writeJSON(w, `{"success":true,"data":[
			{"code":"jne","name":"JNE","costs":[{"type":"Regular","code":"JRE","cost":14670,"costReal":16200}]},
			{"code":"sap","name":"SAP Logistic","costs":[{"type":"Regular","code":"UDRREG","cost":8830,"costReal":11000}]},
			{"code":"ninja","name":"Ninja Xpress","costs":[{"type":"Standard","code":"STANDARD","cost":12500,"costReal":12500}]}
		]}`)
	})
	return httptest.NewServer(mux)
}

func lincahAISetup(t *testing.T, base string) {
	t.Helper()
	setupLincahTestDB(t)
	if err := LincahSaveConfigFull(1, LincahConfigUpdate{
		PartnerID: "partnerid-abc", Token: "tokentest", BaseURL: base,
		AIEnabled: true, Preferred: "sap", Fallback: "jne,ninja",
		WhMode: "nearest",
	}); err != nil {
		t.Fatalf("config: %v", err)
	}
	// Stub geocode → koordinat Bantul (dekat gudang Jogja).
	geocodePlace = func(string) (geoPoint, error) {
		return geoPoint{Lat: -7.9500, Long: 110.3400}, nil
	}
	t.Cleanup(func() { geocodePlace = geocodeOSM })
}

func TestLincahAIDistrictSearch(t *testing.T) {
	srv := lincahAIMockServer(t)
	defer srv.Close()
	setupLincahTestDB(t)
	if err := LincahSaveConfig(1, "partnerid-abc", "tokentest", srv.URL); err != nil {
		t.Fatal(err)
	}
	districts, err := LincahSearchDistrict(1, "bantul")
	if err != nil {
		t.Fatalf("district: %v", err)
	}
	if len(districts) == 0 || districts[0].Code != "34.02.01" || !strings.Contains(districts[0].Name, "Bantul") {
		t.Fatalf("district salah: %+v", districts)
	}
	none, err := LincahSearchDistrict(1, "atlantis")
	if err != nil || len(none) != 0 {
		t.Fatal("harus kosong saat kecamatan tak dikenal")
	}
}

func TestLincahAINearestWarehouseFallback(t *testing.T) {
	srv := lincahAIMockServer(t)
	defer srv.Close()
	lincahAISetup(t, srv.URL)

	block, ok := LincahShippingBlock(1, "Bantul")
	if !ok {
		t.Fatalf("harus menghasilkan blok realtime")
	}
	if !strings.Contains(block, "ONGKIR_REALTIME") {
		t.Fatalf("blok salah: %q", block[:200])
	}
	// Gudang Jogja (terdekat) TIDAK menjangkau → harus fallback ke gudang
	// terdekat BERIKUTNYA (Surabaya), bukan Jogja.
	if strings.Contains(block, "Gudang Yogyakarta") {
		t.Fatalf("seharusnya FALLBACK gudang, dapat: %q", block[:300])
	}
	if !strings.Contains(block, "Gudang Surabaya") {
		t.Fatalf("harus pakai gudang Surabaya: %q", block[:300])
	}
	// Tarif asli harus ada & SAP (preferred) tampil lebih dulu dari JNE.
	posSAP := strings.Index(block, "SAP Logistic")
	posJNE := strings.Index(block, "JNE")
	if posSAP < 0 || posJNE < 0 || posSAP > posJNE {
		t.Fatalf("urutan preferensi salah: %q", block[:400])
	}
	if !strings.Contains(block, "Rp 8.830") || !strings.Contains(block, "Rp 14.670") {
		t.Fatalf("tarif asli hilang: %q", block[:500])
	}
	if !strings.Contains(block, "FALLBACK") || !strings.Contains(block, "EKSPEDISI UTAMA: SAP") {
		t.Fatalf("instruksi kurir hilang: %q", block[:500])
	}
}

func TestLincahAINotFoundBlock(t *testing.T) {
	srv := lincahAIMockServer(t)
	defer srv.Close()
	lincahAISetup(t, srv.URL)
	block, ok := LincahShippingBlock(1, "Atlantis")
	if !ok {
		t.Fatal("blok NOTFOUND harus tetap diproduksi (ok=true)")
	}
	if !strings.Contains(block, "ONGKIR_NOTFOUND") {
		t.Fatalf("harus ONGKIR_NOTFOUND: %q", block[:200])
	}
}

func TestLincahAINoTokenFallsThrough(t *testing.T) {
	setupLincahTestDB(t)
	if err := LincahClearConfig(1); err != nil {
		t.Fatal(err)
	}
	if _, ok := LincahShippingBlock(1, "Bantul"); ok {
		t.Fatal("tanpa token harus fallthrough (ok=false)")
	}
}

func TestHaversineAndOrdering(t *testing.T) {
	jkt := geoPoint{Lat: -6.2, Long: 106.8166}
	jogja := geoPoint{Lat: -7.7956, Long: 110.3695}
	bantul := geoPoint{Lat: -7.95, Long: 110.34}
	if haversineKm(bantul, jogja) > haversineKm(bantul, jkt) {
		t.Fatal("jarak Jogja harus lebih dekat ke Bantul daripada Jakarta")
	}
	// parseCourierList
	list := parseCourierList(" JNE , sap, ")
	if len(list) != 2 || list[0] != "jne" || list[1] != "sap" {
		t.Fatalf("parse courier salah: %v", list)
	}
	// formatRupiah
	if formatRupiah(14670) != "14.670" || formatRupiah(8830) != "8.830" {
		t.Fatalf("rupiah salah: %s %s", formatRupiah(14670), formatRupiah(8830))
	}
	fmt.Println("")
}
