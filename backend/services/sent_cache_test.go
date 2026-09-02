package services

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

func TestSentCacheMarkAndLookup(t *testing.T) {
	w := &waInstance{sentBySystem: make(map[string]time.Time)}
	id := types.MessageID("ABC123")
	if w.isSystemSent(string(id)) {
		t.Fatal("harusnya belum ter-cache")
	}
	w.markSystemSent(id)
	if !w.isSystemSent(string(id)) {
		t.Fatal("harusnya ter-cache setelah mark")
	}
}

func TestSentCacheExpiry(t *testing.T) {
	w := &waInstance{sentBySystem: make(map[string]time.Time)}
	id := types.MessageID("OLD1")
	w.markSystemSent(id)
	w.mu.Lock()
	w.sentBySystem["OLD1"] = time.Now().Add(-sentBySystemCacheTTL - time.Minute)
	w.mu.Unlock()
	if w.isSystemSent("OLD1") {
		t.Fatal("entri kadaluarsa harus dianggap bukan kiriman sistem")
	}
}

func TestMarkSentRecordsID(t *testing.T) {
	w := &waInstance{sentBySystem: make(map[string]time.Time)}
	if err := markSent(w, whatsmeow.SendResponse{ID: types.MessageID("K1")}, nil); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !w.isSystemSent("K1") {
		t.Fatal("markSent harus mencatat ID kiriman sistem")
	}
}
