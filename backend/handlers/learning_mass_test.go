package handlers

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

func setupLearningMassTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Agent{}, &models.Knowledge{}, &models.LearningConfig{}); err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	gin.SetMode(gin.TestMode)
}

// ctxUntukAgent membangun konteks gin dengan tenant + param id (pola currentAgentID).
func ctxUntukAgent(t *testing.T, agentID uint) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Params = gin.Params{{Key: "id", Value: json.Number(itoa(agentID)).String()}}
	c.Set("tenant_id", uint(77))
	return c, w
}

func itoa(n uint) string {
	return strconv.FormatUint(uint64(n), 10)
}

func TestCloneLearningProfileToAll(t *testing.T) {
	setupLearningMassTest(t)
	database.DB.Create(&models.Agent{ID: 1, TenantID: 77, Name: "WA A", Number: "6281", SystemPrompt: "Persona Mbak Hani"})
	database.DB.Create(&models.Agent{ID: 2, TenantID: 77, Name: "WA B", Number: "6282", SystemPrompt: "Persona lama B"})
	database.DB.Create(&models.Knowledge{AgentID: 1, Question: "Bahan label?", Answer: "DTF", Tags: "produk"})
	database.DB.Create(&models.Knowledge{AgentID: 1, Question: "Harga?", Answer: "39rb", Tags: "harga"})
	database.DB.Create(&models.LearningConfig{AgentID: 1, Enabled: true, AutoApply: true, LookbackDays: 30})

	c, w := ctxUntukAgent(t, 1)
	CloneLearningProfileToAll(c)
	if w.Code != 200 {
		t.Fatalf("clone: %d %s", w.Code, w.Body.String())
	}
	var got struct {
		Copied          int `json:"copied"`
		KnowledgeCopied int `json:"knowledge_copied"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Copied != 1 || got.KnowledgeCopied != 2 {
		t.Fatalf("clone salah: %+v", got)
	}
	// Verifikasi target ter-copy
	var b models.Agent
	database.DB.First(&b, 2)
	if b.SystemPrompt != "Persona Mbak Hani" {
		t.Fatalf("persona target tidak disalin: %q", b.SystemPrompt)
	}
	var kcount int64
	database.DB.Model(&models.Knowledge{}).Where("agent_id = ?", 2).Count(&kcount)
	if kcount != 2 {
		t.Fatalf("knowledge target salah: %d", kcount)
	}
	var cfg models.LearningConfig
	database.DB.Where("agent_id = ?", 2).First(&cfg)
	if !cfg.Enabled || !cfg.AutoApply {
		t.Fatalf("config learning target tidak disalin: %+v", cfg)
	}
}

func TestEnableLearningForAll(t *testing.T) {
	setupLearningMassTest(t)
	database.DB.Create(&models.Agent{ID: 1, TenantID: 77, Name: "WA A", Number: "6281"})
	database.DB.Create(&models.Agent{ID: 2, TenantID: 77, Name: "WA B", Number: "6282"})
	database.DB.Create(&models.Agent{ID: 3, TenantID: 77, Name: "WA C", Number: "6283"})

	c, w := ctxUntukAgent(t, 1)
	c.Request = httptest.NewRequest("POST", "/", bytes.NewReader([]byte(`{"auto_apply":true,"schedule_enabled":true}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	EnableLearningForAll(c)
	if w.Code != 200 {
		t.Fatalf("enable-all: %d %s", w.Code, w.Body.String())
	}
	var got struct {
		Enabled int `json:"enabled"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Enabled != 3 {
		t.Fatalf("harus 3 agent aktif, dapat %d", got.Enabled)
	}
	var cnt int64
	database.DB.Model(&models.LearningConfig{}).Where("enabled = ? AND schedule_enabled = ?", true, true).Count(&cnt)
	if cnt != 3 {
		t.Fatalf("config massal salah: %d", cnt)
	}
}
