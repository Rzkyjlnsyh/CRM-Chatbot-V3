package services

import (
	"testing"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestLearningIgnoresHistorySync — riwayat impor TIDAK boleh jadi materi belajar.
func TestLearningIgnoresHistorySync(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:learn-hist-fork?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.ChatHistory{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.DB = db

	now := time.Now().Add(-2 * time.Hour)
	// Balasan CS ASLI (live) — harus masuk materi belajar.
	live := models.ChatHistory{AgentID: 1, Sender: "6281", Reply: "Baik kak, kami proses ya", FromHuman: true, CreatedAt: now}
	// Balasan impor riwayat — HARUS diabaikan.
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
	if len(chats) != 1 {
		t.Fatalf("harusnya hanya 1 chat live (dapat %d) — history_sync bocor ke learning!", len(chats))
	}
	if chats[0].ReplySource == "history_sync" {
		t.Fatal("baris history_sync lolos filter learning")
	}
}
