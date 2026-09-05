package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

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

// TestInboxEventHubPublishSubscribe — event live sampai ke subscriber dengan revision naik.
func TestInboxEventHubPublishSubscribe(t *testing.T) {
	ch, cancel := inboxEvents.subscribe(7)
	defer cancel()

	PublishInboxEvent(7, "incoming", "6281", "M1")
	ev := <-ch
	if ev.Kind != "incoming" || ev.Sender != "6281" || ev.MessageID != "M1" {
		t.Fatalf("event salah: %+v", ev)
	}
	if ev.Revision == 0 {
		t.Fatalf("revision harus > 0, dapat %d", ev.Revision)
	}
}

// TestInboxEventHubReplayFromRevision — reconnect menerima event yang terlewat
// dari history ring-buffer; event typing (transient) TIDAK di-replay.
func TestInboxEventHubReplayFromRevision(t *testing.T) {
	// Bersihkan history agent khusus test agar tidak tercemar test lain.
	inboxEvents.mu.Lock()
	inboxEvents.history[99] = nil
	inboxEvents.mu.Unlock()

	publishIncomingInboxEvent(99, "6282", "H1")
	publishIncomingInboxEvent(99, "6282", "H2")
	publishInboxTypingEvent(99, "6282", true) // transient — tidak boleh masuk replay

	ch, cancel := inboxEvents.subscribeFrom(99, 0, true)
	defer cancel()

	var got []InboxEvent
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case ev := <-ch:
			got = append(got, ev)
		case <-timeout:
			t.Fatalf("replay tidak lengkap: %d event (harus 2)", len(got))
		}
	}
	for _, ev := range got {
		if ev.Kind == "typing" {
			t.Fatalf("event typing transient TIDAK boleh di-replay: %+v", ev)
		}
		if ev.Kind != "incoming" {
			t.Fatalf("event replay salah: %+v", ev)
		}
	}
}

// TestInboxUnreadSummary — summary memakai InboxReadState (konsolidasi A6).
func TestInboxUnreadSummary(t *testing.T) {
	// DB sendiri (bukan shared history-sync) agar tidak tercemar test lain.
	db, err := gorm.Open(sqlite.Open("file:inbox-events-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.ChatHistory{}, &models.Contact{}, &models.ConversationRead{}, &models.InboxReadState{}, &models.Tenant{}, &models.Agent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Where(&models.Tenant{ID: 1}).FirstOrCreate(&models.Tenant{ID: 1})
	db.Where(&models.Agent{ID: 1, TenantID: 1}).FirstOrCreate(&models.Agent{ID: 1, TenantID: 1})
	database.DB = db

	gin.SetMode(gin.TestMode)
	// Unread WA: 6281 = 3, 6282 = 2, 6283 = 0 (dibaca semua).
	now := time.Now()
	db.Create(&models.InboxReadState{AgentID: 1, Sender: "6281", WhatsAppUnreadCount: 3, LastMsgAt: &now})
	db.Create(&models.InboxReadState{AgentID: 1, Sender: "6282", WhatsAppUnreadCount: 2, LastMsgAt: &now})
	db.Create(&models.InboxReadState{AgentID: 1, Sender: "6283", WhatsAppUnreadCount: 0, LastMsgAt: &now})

	r := gin.New()
	r.GET("/agents/:id/inbox/unread-summary", func(c *gin.Context) {
		c.Set("tenant_id", uint(1))
		InboxUnreadSummary(c)
	})

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
	// 3 + 2 = 5; 6283 tidak masuk (unread 0).
	if body.Total != 5 {
		t.Fatalf("total unread %d (harus 5)", body.Total)
	}
	if len(body.IDs) != 2 || body.IDs[0] != "6281" || body.IDs[1] != "6282" {
		t.Fatalf("senders salah: %v", body.IDs)
	}
}
