package handlers

// Tes fitur inbox baru: unread, mark-read, label per percakapan, filter label.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	sqlite "github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupInboxLabelTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:inbox-label-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.ChatHistory{}, &models.Contact{}, &models.Handoff{},
		&models.Label{}, &models.ChatLabel{}, &models.ConversationRead{},
		&models.Agent{},
	); err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })

	// Data: 2 kontak. Kontak A berlabel "Lunas", kontak B tanpa label.
	now := time.Now()
	db.Create(&models.Agent{ID: 1, TenantID: 1, Name: "Tes", Number: "628000"})
	db.Create(&models.Label{LabelID: "l1", Name: "Lunas", AgentID: 1, Color: 5})
	db.Create(&models.ChatLabel{AgentID: 1, LabelID: "l1", Sender: "628111"})
	db.Create(&models.ChatHistory{AgentID: 1, Sender: "628111", Message: "hai", FromHuman: false, CreatedAt: now})
	db.Create(&models.ChatHistory{AgentID: 1, Sender: "628111", Message: "pesan kedua", FromHuman: false, CreatedAt: now.Add(time.Second)})
	db.Create(&models.ChatHistory{AgentID: 1, Sender: "628222", Message: "tes", FromHuman: false, CreatedAt: now})
	return db
}

func inboxGET(t *testing.T, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := gin.New()
	r.GET("/agents/:id/contacts", func(c *gin.Context) {
		c.Set("agent_id", uint(1))
		c.Set("tenant_id", uint(1))
		InboxContacts(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agents/1/contacts"+query, nil)
	r.ServeHTTP(w, req)
	return w
}

func TestInboxShowsLabelsAndUnread(t *testing.T) {
	setupInboxLabelTest(t)
	w := inboxGET(t, "")
	if w.Code != 200 {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			Sender      string `json:"sender"`
			Labels      []struct {
				Name string `json:"name"`
			} `json:"labels"`
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	var a, b bool
	for _, r := range resp.Data {
		switch r.Sender {
		case "628111":
			a = true
			if len(r.Labels) != 1 || r.Labels[0].Name != "Lunas" {
				t.Fatalf("label kontak A salah: %+v", r.Labels)
			}
			if r.UnreadCount != 2 {
				t.Fatalf("unread A = %d, mau 2", r.UnreadCount)
			}
		case "628222":
			b = true
			if len(r.Labels) != 0 {
				t.Fatalf("kontak B tak seharusnya berlabel: %+v", r.Labels)
			}
		}
	}
	if !a || !b {
		t.Fatalf("dua kontak tidak muncul: %+v", resp.Data)
	}
}

func TestInboxLabelFilter(t *testing.T) {
	setupInboxLabelTest(t)
	w := inboxGET(t, "?label_id=l1")
	var resp struct {
		Data []struct {
			Sender string `json:"sender"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 1 || resp.Data[0].Sender != "628111" {
		t.Fatalf("filter label salah: %+v", resp.Data)
	}
}

func TestMarkConversationReadClearsUnread(t *testing.T) {
	setupInboxLabelTest(t)
	r := gin.New()
	r.POST("/agents/:id/inbox/:sender/read", func(c *gin.Context) {
		c.Set("agent_id", uint(1))
		c.Set("tenant_id", uint(1))
		MarkConversationRead(c)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/agents/1/inbox/628111/read", strings.NewReader(""))
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("mark read gagal: %d %s", w.Code, w.Body.String())
	}
	// Setelah ditandai baca, unread = 0.
	w2 := inboxGET(t, "")
	var resp struct {
		Data []struct {
			Sender      string `json:"sender"`
			UnreadCount int    `json:"unread_count"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	for _, r := range resp.Data {
		if r.Sender == "628111" && r.UnreadCount != 0 {
			t.Fatalf("unread A setelah dibaca = %d, mau 0", r.UnreadCount)
		}
	}
}
