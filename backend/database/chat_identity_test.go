package database

import (
	"testing"
	"time"

	"wa-assistant/backend/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupCanonicalTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:canonical-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ChatHistory{}); err != nil {
		t.Fatal(err)
	}
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	return db
}

func TestNormalizedSenderFieldValuePreservesGroupThread(t *testing.T) {
	if got, changed := normalizedSenderFieldValue("120363425256238999@g.us"); changed || got != "" {
		t.Fatalf("JID grup tidak boleh dinormalisasi menjadi nomor: got=%q changed=%v", got, changed)
	}
	if got, changed := normalizedSenderFieldValue("6281220990678@s.whatsapp.net"); !changed || got != "6281220990678" {
		t.Fatalf("JID personal legacy harus dinormalisasi: got=%q changed=%v", got, changed)
	}
}

func TestMergedCanonicalChatFieldsPreservesBestDuplicateData(t *testing.T) {
	newer := time.Date(2026, 7, 28, 14, 16, 5, 0, time.UTC)
	older := newer.Add(-5 * time.Second)
	keeper := models.ChatHistory{
		ID: 1, AgentID: 3, WAMsgID: "WA-1", Message: "🌟 Stiker",
		DeliveryStatus: "sent", CreatedAt: newer,
	}
	metadata := []byte{1, 2, 3}
	updates := mergedCanonicalChatFields(keeper, []models.ChatHistory{{
		ID: 2, AgentID: 3, WAMsgID: "WA-1", MediaType: "sticker",
		MediaMetadata: metadata, MediaFetchStatus: "pending",
		DeliveryStatus: "read", CreatedAt: older,
	}})

	if updates["media_type"] != "sticker" {
		t.Fatalf("tipe media duplikat tidak digabung: %#v", updates)
	}
	if got, ok := updates["media_metadata"].([]byte); !ok || len(got) != len(metadata) {
		t.Fatalf("metadata media duplikat tidak dipertahankan: %#v", updates)
	}
	if updates["delivery_status"] != "read" {
		t.Fatalf("status delivery tertinggi tidak dipertahankan: %#v", updates)
	}
	if got, ok := updates["created_at"].(time.Time); !ok || !got.Equal(older) {
		t.Fatalf("timestamp kanonik harus mempertahankan timestamp paling awal: %#v", updates)
	}
}

func TestDeliveryRankNeverDowngradesRead(t *testing.T) {
	if deliveryRank("read") <= deliveryRank("delivered") {
		t.Fatal("read harus lebih tinggi dari delivered")
	}
	if deliveryRank("played") <= deliveryRank("read") {
		t.Fatal("played harus lebih tinggi dari read")
	}
}

// TestEnsureCanonicalChatMessageIDsDedupsAndProtects — dedup nyata di SQLite:
// dua baris wa_msg_id sama → data terbaik digabung, duplikat dihapus, dan
// unique index mencegah duplikat berikutnya.
func TestEnsureCanonicalChatMessageIDsDedupsAndProtects(t *testing.T) {
	db := setupCanonicalTest(t)
	ts := time.Now()
	db.Create(&models.ChatHistory{AgentID: 7, Sender: "6281", WAMsgID: "WA-DUP", Message: "hai", DeliveryStatus: "sent", CreatedAt: ts})
	db.Create(&models.ChatHistory{AgentID: 7, Sender: "6281", WAMsgID: "WA-DUP", MediaType: "image", DeliveryStatus: "read", CreatedAt: ts.Add(time.Second)})

	if err := EnsureCanonicalChatMessageIDs(); err != nil {
		t.Fatalf("canonical dedup gagal: %v", err)
	}

	var rows []models.ChatHistory
	db.Where("agent_id = 7 AND wa_msg_id = ?", "WA-DUP").Find(&rows)
	if len(rows) != 1 {
		t.Fatalf("duplikat tidak dibersihkan: %d baris tersisa", len(rows))
	}
	kept := rows[0]
	if kept.MediaType != "image" || kept.DeliveryStatus != "read" {
		t.Fatalf("data terbaik tidak digabung ke keeper: %+v", kept)
	}
	// Unique index aktif: duplikat baru harus DITOLAK.
	if err := db.Create(&models.ChatHistory{AgentID: 7, Sender: "6281", WAMsgID: "WA-DUP", Message: "lagi"}).Error; err == nil {
		t.Fatal("unique index tidak mencegah duplikat wa_msg_id baru")
	}
}
