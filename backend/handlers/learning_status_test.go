package handlers

// Tes GetLearningStatus: "Learning terakhir" harus run yang paling AKHIR
// SELESAI (by completed_at), bukan oleh id — supaya kartu status menampilkan
// waktu aktivitas nyata (termasuk jam untuk learning realtime).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func getLearningStatus(t *testing.T) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/agents/:id/learning/status", func(c *gin.Context) {
		c.Set("tenant_id", uint(1))
		c.Set("agent_id", uint(77))
		GetLearningStatus(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agents/77/learning/status", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("status harus 200, dapat %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("respon tidak valid: %v — %s", err, w.Body.String())
	}
	return body.Data
}

func TestGetLearningStatusLastRunOrdering(t *testing.T) {
	// DB sendiri (DSN unik) — tes lain berbagi memori sqlite bersama, dan
	// agent 77 dipakai learning_date_test; isolasi mencegah saling kontaminasi.
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:learning-status-test?mode=memory&cache=shared"), &gorm.Config{})
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

	now := time.Now()

	// Run A: selesai 2 jam lalu (id lebih kecil).
	completedA := now.Add(-2 * time.Hour)
	database.DB.Create(&models.LearningRun{
		AgentID: 77, Status: "completed", HumanChats: 10, CreatedAt: completedA.Add(-10 * time.Minute),
		CompletedAt: &completedA,
	})
	// Run B: selesai 10 menit lalu (id lebih besar) — HARUS jadi "terakhir".
	completedB := now.Add(-10 * time.Minute)
	database.DB.Create(&models.LearningRun{
		AgentID: 77, Status: "completed", HumanChats: 20, CreatedAt: completedB.Add(-5 * time.Minute),
		CompletedAt: &completedB,
	})

	data := getLearningStatus(t)
	lr, ok := data["last_run"].(map[string]any)
	if !ok || lr == nil {
		t.Fatalf("last_run harus ada: %v", data["last_run"])
	}
	if lr["id"].(float64) != 2 || int(lr["human_chats"].(float64)) != 20 {
		t.Fatalf("last_run harus run B (selesai terakhir): %v", lr["id"])
	}
	if _, hasCompleted := lr["completed_at"]; !hasCompleted || lr["completed_at"] == nil {
		t.Fatalf("last_run harus membawa completed_at (dipakai kartu jam): %v", lr["completed_at"])
	}

	// Run C: masih running (completed_at null) tapi mulai paling akhir →
	// tetap muncul sebagai aktivitas terbaru (COALESCE fallback ke created_at).
	runningSince := now.Add(-1 * time.Minute)
	database.DB.Create(&models.LearningRun{
		AgentID: 77, Status: "running", CreatedAt: runningSince,
	})
	data2 := getLearningStatus(t)
	if lr2, ok := data2["last_run"].(map[string]any); !ok || lr2 == nil || lr2["status"] != "running" {
		t.Fatalf("run running terbaru harus jadi last_run: %v", data2["last_run"])
	}
}

// pastikan import database dipakai bila butuh cast lain
var _ = database.DB
