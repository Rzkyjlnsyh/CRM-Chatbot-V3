package handlers

// Tes perbaikan rentang tanggal learning: end-of-day, default 30 hari,
// validasi terbalik, dan format tanggal yang ditolak.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	sqlite "github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupLearningHandlerTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:learning-date-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.LearningRun{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&models.Agent{ID: 77, Name: "Tes", Number: "628000", TenantID: 1})
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

func postStartLearning(t *testing.T, agentID uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/agents/:id/learning/run", func(c *gin.Context) {
		c.Set("tenant_id", uint(1))
		c.Set("agent_id", agentID)
		StartLearning(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agents/77/learning/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func runRangeFromDB(t *testing.T, runID uint) (string, string) {
	t.Helper()
	var run models.LearningRun
	if err := database.DB.First(&run, runID).Error; err != nil {
		t.Fatalf("run tidak tersimpan: %v", err)
	}
	return run.SourceStartDate.UTC().Format("2006-01-02 15:04"), run.SourceEndDate.UTC().Format("2006-01-02 15:04")
}

func TestStartLearningEndOfDay(t *testing.T) {
	setupLearningHandlerTest(t)
	w := postStartLearning(t, 77, `{"start_date":"2026-08-25","end_date":"2026-08-26"}`)
	if w.Code != 202 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			RunID uint `json:"run_id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	start, end := runRangeFromDB(t, resp.Data.RunID)
	if start != "2026-08-25 00:00" {
		t.Fatalf("start=%s mau 2026-08-25 00:00", start)
	}
	// End harus 23:59 UTC (akhir hari penuh) — bukan tengah malam.
	if end != "2026-08-26 23:59" {
		t.Fatalf("end=%s mau 2026-08-26 23:59", end)
	}
}

func TestStartLearningDefaults30Days(t *testing.T) {
	setupLearningHandlerTest(t)
	w := postStartLearning(t, 77, `{}`)
	if w.Code != 202 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct{ RunID uint `json:"run_id"` } `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	start, end := runRangeFromDB(t, resp.Data.RunID)
	if start == "" || end == "" || start >= end {
		t.Fatalf("default 30 hari salah: start=%s end=%s", start, end)
	}
}

func TestStartLearningRejectsReversedRange(t *testing.T) {
	setupLearningHandlerTest(t)
	w := postStartLearning(t, 77, `{"start_date":"2026-08-27","end_date":"2026-08-26"}`)
	if w.Code != 400 {
		t.Fatalf("code=%d (mau 400) body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "terbalik") {
		t.Fatalf("pesan validasi kurang jelas: %s", w.Body.String())
	}
}

func TestStartLearningRejectsBadFormat(t *testing.T) {
	setupLearningHandlerTest(t)
	w := postStartLearning(t, 77, `{"start_date":"26-08-2026"}`)
	if w.Code != 400 {
		t.Fatalf("code=%d (mau 400) body=%s", w.Code, w.Body.String())
	}
}
