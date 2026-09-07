package handlers

import (
	"testing"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupHistorySyncTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file:hist-sync-fork?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.ChatHistory{}, &models.Contact{}, &models.ConversationRead{}, &models.Tenant{}, &models.Agent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// DB memori shared antar-test → seed idempoten.
	if err := db.Where(&models.Tenant{ID: 1}).FirstOrCreate(&models.Tenant{ID: 1}).Error; err != nil {
		t.Fatalf("tenant: %v", err)
	}
	if err := db.Where(&models.Agent{ID: 1, TenantID: 1}).FirstOrCreate(&models.Agent{ID: 1, TenantID: 1}).Error; err != nil {
		t.Fatalf("agent: %v", err)
	}
	database.DB = db
	return db
}

func TestOnWAHistorySyncIngestAndDedup(t *testing.T) {
	db := setupHistorySyncTestDB(t)
	now := time.Now().Add(-48 * time.Hour)
	msgs := []services.HistoricalMessage{
		{Sender: "628111", Text: "Halo min", FromMe: false, WAMsgID: "H1", Timestamp: now},
		{Sender: "628111", Text: "Baik, ditunggu ya", FromMe: true, WAMsgID: "H2", Timestamp: now.Add(time.Minute)},
	}
	imported, skipped, err := OnWAHistorySync(1, msgs)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if imported != 2 || skipped != 0 {
		t.Fatalf("import=%d skip=%d (harus 2/0)", imported, skipped)
	}
	// Ulang batch yang sama → semua skip (dedup).
	imported2, skipped2, _ := OnWAHistorySync(1, msgs)
	if imported2 != 0 || skipped2 != 2 {
		t.Fatalf("dedup gagal: import=%d skip=%d (harus 0/2)", imported2, skipped2)
	}
	var reply models.ChatHistory
	db.Where("agent_id = 1 AND wa_msg_id = ?", "H2").First(&reply)
	if !reply.FromHuman || reply.ReplySource != "history_sync" {
		t.Fatalf("baris balasan salah: %+v", reply)
	}
	// Learning wajib MENGABAIKAN baris history_sync → dicek di test services.
}

func TestOnWAMessageRevoke(t *testing.T) {
	setupHistorySyncTestDB(t)
	row := models.ChatHistory{AgentID: 2, Sender: "628222", Message: "halo", WAMsgID: "R1"}
	if err := database.DB.Create(&row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	OnWAMessageRevoke(2, "R1", time.Now())
	var got models.ChatHistory
	database.DB.First(&got, row.ID)
	// v4: handler menandai flag revoked; frontend menampilkan "Pesan ini dihapus"
	// berdasarkan flag tersebut (bukan teks Message).
	if !got.Revoked {
		t.Fatalf("revoke tidak diterapkan: %+v", got)
	}
}
