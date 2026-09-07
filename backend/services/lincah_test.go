package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// ---------------------------------------------------------------------------
// Mock HTTP: satu server memeriksa header auth + menjawab sesuai path.
// ---------------------------------------------------------------------------

func lincahMockServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	mux := http.NewServeMux()
	checkAuth := func(w http.ResponseWriter, r *http.Request) bool {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer tokentest" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"success":false,"message":"Your token key is wrong."}`))
			return false
		}
		if r.Header.Get("partner-id") != "partnerid-abc" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"success":false,"message":"partner-id header required"}`))
			return false
		}
		return true
	}
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"x1","email":"partner.raizen@lincah.id","phone":"628123","name":"Raizen Partner"}}`))
	})
	mux.HandleFunc("/balance", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"balance":1250000}}`))
	})
	mux.HandleFunc("/courier", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"_id":"jne","name":"JNE"},{"_id":"sap","name":"SAP Logistic"}]}`))
	})
	mux.HandleFunc("/address", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"_id":"addr1","name":"Gudang Jakarta","address":"Jl. Nusantara 10","origin_id":"36.03.12","zipcode":"15560","geoloc":{"lat":-6.2,"long":106.6}}]}`))
	})
	mux.HandleFunc("/district/search", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		if len(r.URL.Query().Get("q")) < 3 {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"success":false,"message":"q min 3"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"code":"34.02.01","province":"DI Yogyakarta","city":"Bantul","name":"Bantul","id":"d1","fullName":"Bantul, Kab. Bantul, DI Yogyakarta"}]}`))
	})
	mux.HandleFunc("/ongkir", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		var pay map[string]any
		_ = json.NewDecoder(r.Body).Decode(&pay)
		if pay["origin_code"] == "" || pay["destination_code"] == "" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"success":false,"message":"origin & destination required"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"code":"jne","name":"JNE","costs":[{"type":"Regular","code":"JRE","cost":14670,"costReal":16200,"etc":"3-5 days"}]},{"code":"sap","name":"SAP Logistic","costs":[{"type":"Regular","code":"UDRREG","cost":8830,"costReal":11000,"etc":"2-4 days"}]}]}`))
	})
	mux.HandleFunc("/order", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		if r.Method == "POST" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"ord123","no_order":"2410A7EHAD31","name":"John Doe","phone":"628568807076","address":"Jl. Remaja No. 20, Kediri","weight":1,"courier":"jne","courier_service":"JRE","quantity":1,"product_price":100000,"product_name":"2 pcs kaos","status":"waiting","type":"regular","destination_text":"Bantul, Kab. Bantul, DI Yogyakarta","destination_id":"34.02.01","sender_type":"picked-up","sender":{"name":"Gudang Jakarta","phone":"628123123","address":"Jl. Nusantara 10","origin_id":"36.03.12","zipcode":"15560"},"resi":"LNCHS2410A7EHAD31","ongkir":{"fee":11340,"feeReal":16200,"discount":4860,"insurance":0}}}`))
			return
		}
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"success":false,"message":"not found"}`))
	})
	mux.HandleFunc("/order/print", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":"https://api.lincah.id/pdf/resi/abc123.pdf"}`))
	})
	mux.HandleFunc("/order/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	mux.HandleFunc("/order/regenerate", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	})
	mux.HandleFunc("/order/", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		path := r.URL.Path
		if len(path) > 6 && path[len(path)-6:] == "/track" {
			_, _ = w.Write([]byte(`{"success":true,"order":{"no_order":"2408A7EHAH6X","resi":"LNCHS2408A7EHAH6X","weight":1,"volume":"10x20x30","courier":"ninja","courier_service":"Standard","ongkir":{"fee":8830,"feeReal":11000,"codFee":3330,"discount":5500,"insurance":0},"origin_text":"Klaten Utara","origin_id":"33.10.24","destination_text":"Bantul","destination_id":"34.02.08"},"data":[{"status":"In Transit to Origin Hub","time":"2024-09-10T04:23:15.000Z","message":null},{"status":"Pending Reschedule","time":"2024-09-10T04:23:23.000Z","message":"Alamat tidak lengkap","images":["https://cdn/x.png"]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"ord123","no_order":"2410A7EHAD31","resi":"LNCHS2410A7EHAD31","status":"waiting","courier":"sap","courier_service":"UDRREG"}}`))
	})
	srv := httptest.NewServer(mux)
	return srv, &calls
}

// setupLincahTestDB — DB SQLite in-memory MILIK SENDIRI per test.
// PENTING: di suite penuh, database.DB global di-clobber oleh test paket
// lain (pola learning_test); tanpa DB sendiri, tabel lincah_configs tidak
// ada → tes gagal beruntun atau panic.
func setupLincahTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:lincah-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gagal buka db tes: %v", err)
	}
	if err := db.AutoMigrate(&models.LincahConfig{}, &models.LincahTenantConfig{}, &models.Agent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
}

// registerConfig — set kredensial mock di DB milik sendiri sebelum tiap test.
func registerConfig(t *testing.T, base string) {
	t.Helper()
	setupLincahTestDB(t)
	if err := LincahSaveConfig(1, "partnerid-abc", "tokentest", base); err != nil {
		t.Fatalf("gagal simpan config: %v", err)
	}
}

func TestLincahTenantConfigFallback(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	setupLincahTestDB(t)
	// Agent 1 milik tenant 77; kredensial di level TENANT (1 akun utk semua WA).
	database.DB.Create(&models.Agent{ID: 1, TenantID: 77, Name: "WA Test", Number: "6281"})
	if err := LincahSaveTenantConfig(77, "partnerid-abc", "tokentest", srv.URL); err != nil {
		t.Fatalf("tenant save: %v", err)
	}
	cfg := LincahGetConfig(1)
	if cfg.Token != "tokentest" || cfg.PartnerID != "partnerid-abc" {
		t.Fatalf("harus fallback ke config tenant, dapat: %+v", cfg)
	}
	me, err := LincahMe(1)
	if err != nil || me.Name != "Raizen Partner" {
		t.Fatalf("Me via tenant: err=%v me=%+v", err, me)
	}
	// Agent dengan token SENDIRI menimpa tenant.
	if err := LincahSaveConfig(1, "p-own", "token-own", srv.URL); err != nil {
		t.Fatal(err)
	}
	cfg2 := LincahGetConfig(1)
	if cfg2.Token != "token-own" {
		t.Fatalf("token agent sendiri harus menang: %+v", cfg2)
	}
}

func TestLincahMeAuthHeader(t *testing.T) {
	srv, calls := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)

	me, err := LincahMe(1)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.Name != "Raizen Partner" || me.Email != "partner.raizen@lincah.id" {
		t.Fatalf("Me tak sesuai: %+v", me)
	}
	if len(*calls) != 1 || (*calls)[0] != "GET /me" {
		t.Fatalf("panggilan tak sesuai: %v", *calls)
	}
}

func TestLincahBalance(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	bal, err := LincahBalance(1)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	if bal.Balance != 1250000 {
		t.Fatalf("saldo salah: %d", bal.Balance)
	}
}

func TestLincahAddressesWarehouses(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	addrs, err := LincahAddresses(1)
	if err != nil {
		t.Fatalf("Addresses: %v", err)
	}
	if len(addrs) != 1 || addrs[0].Name != "Gudang Jakarta" || addrs[0].OriginID != "36.03.12" {
		t.Fatalf("gudang tak sesuai: %+v", addrs)
	}
}

func TestLincahOngkirPayloads(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	costs, err := LincahOngkir(1, LincahOngkirRequest{
		IsPickup: true, IsCod: false, Dimensions: []int{10, 10, 10}, Weight: 1000,
		Origin: "36.03.12", Destination: "34.02.01",
		Logistics: []string{"JNE", "SAP Logistic"}, Services: []string{"Regular"},
	})
	if err != nil {
		t.Fatalf("Ongkir: %v", err)
	}
	if len(costs) != 2 || costs[0].Code != "jne" || costs[0].Costs[0].Cost != 14670 {
		t.Fatalf("tarif tak sesuai: %+v", costs)
	}
}

func TestLincahCreateOrder(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	// DEBUG: lihat respons mentah mock utk POST /order
	cfg := LincahGetConfig(1)
	raw, st, rerr := lincahDo(lincahRequest{cfg: cfg, method: "POST", path: "/order", payload: LincahOrderPayload{Name: "x", Phone: "y", Address: "z", Destination: "34.02.01", Courier: "jne"}})
	t.Logf("DEBUG raw=%q status=%d err=%v", raw, st, rerr)
	res, err := LincahCreateOrder(1, LincahOrderPayload{
		SenderType: "picked up", AddressRef: "addr1",
		Name: "John Doe", Phone: "628568807076", Address: "Jl. Remaja No. 20, Kediri",
		Destination: "34.02.01", Type: "regular", Courier: "jne", CourierService: "JRE",
		ProductPrice: 100000, Weight: 1, Quantity: 1, Volume: "10x10x10",
		ProductName: "2 pcs kaos", IsInsurance: false,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if res.ID != "ord123" || res.Resi != "LNCHS2410A7EHAD31" || res.Status != "waiting" {
		t.Fatalf("order tak sesuai: %+v", res)
	}
	if res.ProductName != "2 pcs kaos" || res.Weight != 1 {
		t.Fatalf("field produk hilang: %+v", res)
	}
}

func TestLincahTrack(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	tr, err := LincahTrack(1, "LNCHS2408A7EHAH6X")
	if err != nil {
		t.Fatalf("Track: %v", err)
	}
	if tr.Order.Resi != "LNCHS2408A7EHAH6X" || len(tr.Data) != 2 {
		t.Fatalf("track tak sesuai: %+v", tr)
	}
	if tr.Data[1].Message == "" || len(tr.Data[1].Images) != 1 {
		t.Fatalf("event tak sesuai: %+v", tr.Data[1])
	}
}

func TestLincahPrintAndCancel(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	url, err := LincahPrintOrder(1, "ord123")
	if err != nil {
		t.Fatalf("Print: %v", err)
	}
	if url == "" || !stringsContains(url, "pdf/resi") {
		t.Fatalf("url print salah: %q", url)
	}
	if err := LincahCancelOrder(1, "ord123"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestLincahQuoteForChat(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	q, err := LincahQuoteForChat(1, "bantul", 1000, []int{10, 10, 10})
	if err != nil {
		t.Fatalf("QuoteForChat: %v", err)
	}
	if q.Destination != "34.02.01" || q.OriginID != "36.03.12" {
		t.Fatalf("asal/tujuan salah: %+v", q)
	}
	if len(q.Options) == 0 || q.Options[0].Cost != 8830 {
		t.Fatalf("opsi termurah harus SAP 8830: %+v", q.Options)
	}
	if !stringsContains(q.Text, "Bantul") || !stringsContains(q.Text, "8.830") {
		t.Fatalf("teks quote salah: %q", q.Text)
	}
}

func TestLincahQuoteForChatInvalidInput(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	if _, err := LincahQuoteForChat(1, "ab", 1000, nil); err == nil {
		t.Fatal("harus menolak tujuan < 3 huruf")
	}
	if _, err := LincahQuoteForChat(1, "bantul", 50, nil); err == nil {
		t.Fatal("harus menolak berat < 100 gram")
	}
}

func TestLincahSearchDistrict(t *testing.T) {
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	registerConfig(t, srv.URL)
	items, err := LincahSearchDistrict(1, "bantul")
	if err != nil {
		t.Fatalf("SearchDistrict: %v", err)
	}
	if len(items) != 1 || items[0].Code != "34.02.01" {
		t.Fatalf("district salah: %+v", items)
	}
}

func TestLincahUnauthorizedPropagates(t *testing.T) {
	// Simulasi token SALAH: gunakan mock yang mengharapkan tokentest tapi
	// simpan token lain → server lihat header salah → 401 → error berisi
	// pesan dari Lincah (bukan kesalahan parsing lokal).
	srv, _ := lincahMockServer(t)
	defer srv.Close()
	setupLincahTestDB(t)
	if err := LincahSaveConfig(2, "partnerid-abc", "tokenSALAH", srv.URL); err != nil {
		t.Fatalf("config: %v", err)
	}
	_, err := LincahMe(2)
	if err == nil {
		t.Fatal("harus error saat token salah")
	}
	if apiErr, ok := err.(*LincahAPIError); !ok || apiErr.Status != 401 {
		t.Fatalf("harus LincahAPIError 401, dapat: %v", err)
	}
}

func TestLincahNoTokenGivesFriendlyError(t *testing.T) {
	// Tanpa token (config kosong & env kosong) → pesan ramah, bukan panic.
	setupLincahTestDB(t)
	if err := LincahSaveConfig(3, "", "", ""); err != nil {
		t.Fatalf("config: %v", err)
	}
	_, err := LincahMe(3)
	if err == nil {
		t.Fatal("harus error saat token kosong")
	}
	if want := "token belum diatur"; !stringsContains(err.Error(), want) {
		t.Fatalf("pesan salah: %v", err)
	}
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
