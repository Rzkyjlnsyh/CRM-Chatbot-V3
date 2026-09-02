package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func seedChat(t *testing.T, agentID uint, sender string, fromHuman bool, id uint) {
	t.Helper()
	row := models.ChatHistory{AgentID: agentID, Sender: sender, FromHuman: fromHuman, Message: "hi"}
	if fromHuman {
		row.Reply = "halo"
	}
	if id > 0 {
		row.ID = id
	}
	if err := database.DB.Create(&row).Error; err != nil {
		t.Fatalf("seed chat: %v", err)
	}
}

func readRow(agentID uint, sender string, lastChatID uint) {
	database.DB.Create(&models.ConversationRead{AgentID: agentID, Sender: sender, LastReadChatID: lastChatID})
}

// TestInboxEventHubPublishSubscribe — event sampai ke subscriber dengan revision naik.
func TestInboxEventHubPublishSubscribe(t *testing.T) {
	sub, rev0 := hub.subscribe(7)
	defer hub.unsubscribe(7, sub)

	PublishInboxEvent(7, "new_message", "6281", "M1")
	ev := <-sub.ch
	if ev.Kind != "new_message" || ev.Sender != "6281" || ev.MessageID != "M1" {
		t.Fatalf("event salah: %+v", ev)
	}
	if ev.Revision != rev0+1 {
		t.Fatalf("revision %d (harus %d)", ev.Revision, rev0+1)
	}
	// Revision per-agent terpisah.
	sub2, rev0b := hub.subscribe(8)
	defer hub.unsubscribe(8, sub2)
	if rev0b != 0 {
		t.Fatalf("agent 8 harus mulai dari 0, dapat %d", rev0b)
	}
	PublishInboxEvent(8, "typing", "6281", "composing")
	if ev2 := <-sub2.ch; ev2.Kind != "typing" {
		t.Fatalf("event agent 8 salah: %+v", ev2)
	}
}

// TestInboxUnreadSummary — summary memakai ConversationRead (mark-read fork).
func TestInboxUnreadSummary(t *testing.T) {
	// DB sendiri (bukan shared history-sync) agar tidak tercemar test lain.
	db, err := gorm.Open(sqlite.Open("file:inbox-events-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.ChatHistory{}, &models.Contact{}, &models.ConversationRead{}, &models.Tenant{}, &models.Agent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where(&models.Tenant{ID: 1}).FirstOrCreate(&models.Tenant{ID: 1})
	db.Where(&models.Agent{ID: 1, TenantID: 1}).FirstOrCreate(&models.Agent{ID: 1, TenantID: 1})
	database.DB = db

	gin.SetMode(gin.TestMode)
	// 2 pesan masuk untuk 6281, 1 untuk 6282 (belum pernah dibaca).
	seedChat(t, 1, "6281", false, 0)
	seedChat(t, 1, "6281", false, 0)
	seedChat(t, 1, "6282", false, 0)

	r := gin.New()
	r.GET("/agents/:id/inbox/unread-summary", func(c *gin.Context) {
		c.Set("tenant_id", uint(1))
		InboxUnreadSummary(c)
	})
	// Baca 6281 sampai id pesan kedua (ambil id asli dari DB).
	var last models.ChatHistory
	db.Where("agent_id = 1 AND sender = ?", "6281").Order("id desc").First(&last)
	readRow(1, "6281", last.ID)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/agents/1/inbox/unread-summary", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("kode %d", w.Code)
	}
	var body struct {
		Total int      `json:"total"`
		IDs   []string `json:"senders"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	// 6281 sudah dibaca semua → sisa 1 pesan 6282.
	if body.Total != 1 {
		t.Fatalf("total unread %d (harus 1)", body.Total)
	}
	if len(body.IDs) != 1 || body.IDs[0] != "6282" {
		t.Fatalf("senders salah: %v", body.IDs)
	}
}
