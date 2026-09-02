package handlers

import (
	"testing"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/glebarez/sqlite"
	"go.mau.fi/whatsmeow/types"
	"gorm.io/gorm"
)

func TestOnWAOwnMessageDedupAndMediaUpdate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:ownmsg-fork-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := db.AutoMigrate(&models.ChatHistory{}, &models.Contact{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.DB = db

	agentID := uint(1)
	num := "628123456789"
	jid := types.NewJID(num, types.DefaultUserServer)
	in := services.IncomingMessage{Text: "Halo dari HP", WAMsgID: "WAID-FORK-1", Timestamp: time.Now().Add(-5 * time.Minute)}

	OnWAOwnMessage(agentID, jid, in)
	// Kirim ulang event yang sama (echo) — TIDAK boleh dobel.
	OnWAOwnMessage(agentID, jid, in)

	var cnt int64
	db.Model(&models.ChatHistory{}).Where("agent_id = ? AND sender = ?", agentID, num).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("dedup gagal: %d baris (harus 1)", cnt)
	}
	var row models.ChatHistory
	db.Where("agent_id = ? AND sender = ?", agentID, num).First(&row)
	if row.Reply != "Halo dari HP" || !row.FromHuman {
		t.Fatalf("isi baris salah: %+v", row)
	}
	if row.CreatedAt.Unix() != in.Timestamp.Unix() {
		t.Fatalf("timestamp WA tidak dipakai: got %v want %v", row.CreatedAt, in.Timestamp)
	}

	// Echo dengan media yang datang belakangan → UPDATE baris lama, bukan insert baru.
	in2 := services.IncomingMessage{Text: "", MediaType: "image", FileName: "foto.jpg", Mimetype: "image/jpeg", WAMsgID: "WAID-FORK-1", Timestamp: in.Timestamp}
	OnWAOwnMessage(agentID, jid, in2)

	db.Model(&models.ChatHistory{}).Where("agent_id = ? AND sender = ?", agentID, num).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("media echo bikin baris baru: %d (harus 1)", cnt)
	}
	db.Where("agent_id = ? AND sender = ?", agentID, num).First(&row)
	if row.MediaType != "image" || row.FileName != "foto.jpg" {
		t.Fatalf("media echo tidak meng-update baris lama: %+v", row)
	}
}
