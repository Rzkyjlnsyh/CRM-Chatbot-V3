package handlers

// Tes pagination GetLearningPatterns: total utuh, halaman terpotong benar.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
)

func setupPatternsPaginationTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupLearningHandlerTest(t)
	db.AutoMigrate(&models.LearningPattern{})
	for i := 1; i <= 3; i++ {
		db.Create(&models.LearningPattern{
			AgentID: 77, LearningRunID: 1, PatternType: "phrase",
			TriggerContext: "trig", ResponseTemplate: "tpl", Status: "suggested",
		})
	}
}

func getPatterns(t *testing.T, query string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/agents/:id/learning/patterns", func(c *gin.Context) {
		c.Set("tenant_id", uint(1))
		c.Set("agent_id", uint(77))
		GetLearningPatterns(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agents/77/learning/patterns"+query, nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status harus 200, dapat %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Patterns []models.LearningPattern `json:"patterns"`
			Total    int64                    `json:"total"`
			Page     int                      `json:"page"`
			Limit    int                      `json:"limit"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("respon tidak valid: %v — %s", err, w.Body.String())
	}
	return len(body.Data.Patterns), map[string]any{
		"total": body.Data.Total, "page": body.Data.Page, "limit": body.Data.Limit,
	}
}

func TestGetLearningPatternsPagination(t *testing.T) {
	setupPatternsPaginationTest(t)

	// Halaman 1 limit 2 → 2 pola, total 3 (angka TETAP utuh).
	n, meta := getPatterns(t, "?status=suggested&page=1&limit=2")
	if n != 2 || meta["total"] != int64(3) || meta["page"] != 1 || meta["limit"] != 2 {
		t.Fatalf("halaman 1 salah: n=%d meta=%v", n, meta)
	}
	// Halaman 2 → 1 pola, total tetap 3.
	n2, meta2 := getPatterns(t, "?status=suggested&page=2&limit=2")
	if n2 != 1 || meta2["total"] != int64(3) {
		t.Fatalf("halaman 2 salah: n=%d meta=%v", n2, meta2)
	}
	// Halaman di luar jangkauan → 0 pola, total tetap 3.
	n3, meta3 := getPatterns(t, "?status=suggested&page=9&limit=2")
	if n3 != 0 || meta3["total"] != int64(3) {
		t.Fatalf("halaman kosong salah: n=%d meta=%v", n3, meta3)
	}
	// Status lain → total 0 (pola semua suggested).
	n4, meta4 := getPatterns(t, "?status=applied&page=1&limit=2")
	if n4 != 0 || meta4["total"] != int64(0) {
		t.Fatalf("filter status salah: n=%d meta=%v", n4, meta4)
	}
}

// pastikan database dipakai (guard kompilasi bila import berubah)
var _ = database.DB
