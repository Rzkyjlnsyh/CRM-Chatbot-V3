package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

func setupMetaAdsTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MetaAdsSnapshot{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	gin.SetMode(gin.TestMode)
}

func TestMetaAdsConfigRoundtrip(t *testing.T) {
	setupMetaAdsTest(t)
	r := gin.New()
	r.PUT("/config", MetaAdsConfigHandler)
	r.GET("/config", MetaAdsConfigGet)

	// Simpan config
	body, _ := json.Marshal(map[string]any{
		"ad_account_id": "act_1234567890",
		"access_token":  "tokensecret123",
		"target_roas":   3.5,
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/config", bytes.NewReader(body))
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("simpan: %d %s", w.Code, w.Body.String())
	}

	// Baca kembali — token TIDAK boleh bocor (hanya has_token)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/config", nil))
	var got struct {
		Data struct {
			AdAccountID string  `json:"ad_account_id"`
			HasToken    bool    `json:"has_token"`
			TargetROAS  float64 `json:"target_roas"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &got)
	if got.Data.AdAccountID != "act_1234567890" || !got.Data.HasToken || got.Data.TargetROAS != 3.5 {
		t.Fatalf("config salah: %+v", got.Data)
	}
	if strings.Contains(w2.Body.String(), "tokensecret123") {
		t.Fatal("token plaintext bocor di respons")
	}

	// Validasi: tanpa ad_account_id → 400
	w3 := httptest.NewRecorder()
	bad, _ := json.Marshal(map[string]any{"target_roas": 2.0})
	r.ServeHTTP(w3, httptest.NewRequest("PUT", "/config", bytes.NewReader(bad)))
	if w3.Code != 400 {
		t.Fatalf("harus 400 saat account id kosong, dapat %d", w3.Code)
	}
}

func TestMetaAdsRefreshAndData(t *testing.T) {
	setupMetaAdsTest(t)
	// Mock Marketing API
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != "tokentest" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":{"message":"token salah","code":190}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[
			{"campaign_id":"1","campaign_name":"Kampanye A","objective":"CONVERSIONS","spend":"500000","impressions":"10000","clicks":"300","ctr":"3.0","cpc":"1666.67","cpm":"50000","reach":"8000","frequency":"1.25","results":"20","cost_per_result":"25000","purchase_roas":"4.2"},
			{"campaign_id":"2","campaign_name":"Kampanye B","objective":"TRAFFIC","spend":"200000","impressions":"5000","clicks":"100","ctr":"2.0","cpc":"2000","cpm":"40000","reach":"4500","frequency":"1.11","results":"5","cost_per_result":"40000","purchase_roas":"0.5"}
		]}`))
	}))
	defer srv.Close()
	oldBase := metaGraphBase
	metaGraphBase = srv.URL
	t.Cleanup(func() { metaGraphBase = oldBase })

	// Simpan config dengan token terenkripsi
	database.SetAppSetting("meta_ads_account_id", "act_1")
	enc, err := services.EncryptSecret("tokentest")
	if err != nil {
		t.Fatal(err)
	}
	database.SetAppSetting("meta_ads_token", enc)

	r := gin.New()
	r.POST("/refresh", MetaAdsRefreshHandler)
	r.GET("/data", MetaAdsDataHandler)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/refresh", nil))
	if w.Code != 200 {
		t.Fatalf("refresh: %d %s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/data", nil))
	var got struct {
		Data struct {
			Campaigns []map[string]any `json:"campaigns"`
			Summary   map[string]any   `json:"summary"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &got)
	if len(got.Data.Campaigns) != 2 {
		t.Fatalf("harus 2 kampanye, dapat %d", len(got.Data.Campaigns))
	}
	// Kampanye A harus posisi pertama (spend terbesar)
	if got.Data.Campaigns[0]["campaign_name"] != "Kampanye A" {
		t.Fatalf("urutan salah: %+v", got.Data.Campaigns[0])
	}
	if got.Data.Summary["spend"].(float64) != 700000 {
		t.Fatalf("total spend salah: %v", got.Data.Summary["spend"])
	}
}
