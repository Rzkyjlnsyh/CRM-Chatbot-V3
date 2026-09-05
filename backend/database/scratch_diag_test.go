package database

import (
	"fmt"
	"testing"

	"wa-assistant/backend/models"
)

// TestScratchSenderDiag — DIAGNOSTIK SEMENTARA (dihapus setelah selesai).
// Membaca database MySQL ASLI via config .env untuk analisa nomor & urutan.
func TestScratchSenderDiag(t *testing.T) {
	Init() // koneksi persis seperti server (MySQL dari .env)

	fmt.Println("== [1] distribusi sender ==")
	var rows []struct {
		Sender string
		Cnt    int64
	}
	DB.Raw(`SELECT sender, COUNT(*) AS cnt FROM chat_histories GROUP BY sender ORDER BY cnt DESC LIMIT 15`).Scan(&rows)
	for _, r := range rows {
		fmt.Printf("  sender=%q cnt=%d\n", r.Sender, r.Cnt)
	}

	fmt.Println("== [2] contoh baris sender aneh ==")
	var sample []models.ChatHistory
	DB.Where("sender = ?", "22930998186098").Order("id ASC").Limit(6).Find(&sample)
	for _, s := range sample {
		fmt.Printf("  id=%d wa=%q from_human=%v created=%s msg=%q\n", s.ID, s.WAMsgID, s.FromHuman, s.CreatedAt.Format("15:04:05"), trunc(s.Message, 30))
	}
	var last models.ChatHistory
	DB.Where("sender = ?", "22930998186098").Order("id DESC").First(&last)
	fmt.Printf("  TERBARU: id=%d created=%s\n", last.ID, last.CreatedAt.Format("15:04:05"))

	fmt.Println("== [3] inbox_read_states senders ==")
	var irs []struct{ Sender string }
	DB.Raw(`SELECT sender FROM inbox_read_states ORDER BY last_msg_at DESC LIMIT 10`).Scan(&irs)
	for _, r := range irs {
		fmt.Printf("  irs=%q\n", r.Sender)
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
