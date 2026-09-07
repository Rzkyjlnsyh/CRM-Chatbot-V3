package services

import (
	"testing"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestLearningIncludesHistorySync — riwayat impor = balasan CS asli, IKUT jadi
// materi belajar (dedup wa_msg_id & pola ternormalisasi mencegah dobel).
func TestLearningIncludesHistorySync(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:learn-hist-fork?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.ChatHistory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.DB = db

	now := time.Now().Add(-30 * time.Minute)
	// Balasan CS ASLI (live).
	live := models.ChatHistory{AgentID: 1, Sender: "6281", Reply: "Baik kak, kami proses ya", FromHuman: true, CreatedAt: now}
	// Balasan impor riwayat — JUGA materi belajar.
	hist := models.ChatHistory{AgentID: 1, Sender: "6281", Reply: "Baik kak, kami proses ya (lama)", FromHuman: true, ReplySource: "history_sync", CreatedAt: now.Add(-5 * time.Minute)}
	if err := db.Create(&live).Error; err != nil {
		t.Fatalf("seed live: %v", err)
	}
	if err := db.Create(&hist).Error; err != nil {
		t.Fatalf("seed hist: %v", err)
	}

	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)
	chats, err := loadHumanCSChats(1, &start, &end)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(chats) != 2 {
		t.Fatalf("riwayat impor harus ikut materi belajar: dapat %d (harus 2)", len(chats))
	}
}
