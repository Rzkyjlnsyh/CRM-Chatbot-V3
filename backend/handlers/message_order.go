package handlers

import (
	"strings"
	"time"

	"wa-assistant/backend/services"
)

// message_order — helper urutan & metadata pesan (pola v4 chatloop-1.6-1.7):
// pesan yang terbelah/tergabung saat debounce dipisah dengan benar dan
// timestamp WA asli dipertahankan per pesan.

// pendingPart = satu pesan masuk yang sedang menunggu ditulis ke Inbox.
type pendingPart struct {
	Text      string
	ID        string
	Timestamp time.Time
	ReplyTo   string
	ReplyText string
}

// messageTime memakai timestamp WhatsApp; fallback ke now bila kosong.
func messageTime(ts time.Time) time.Time {
	if ts.IsZero() {
		return time.Now()
	}
	return ts
}

func pendingPartFrom(in services.IncomingMessage) pendingPart {
	id := strings.TrimSpace(in.WAMsgID)
	if id == "" && len(in.WAMsgIDs) > 0 {
		id = strings.TrimSpace(in.WAMsgIDs[0])
	}
	return pendingPart{
		Text:      in.Text,
		ID:        id,
		Timestamp: messageTime(in.Timestamp),
		ReplyTo:   in.ReplyTo,
		ReplyText: in.ReplyText,
	}
}

func nonEmptyMessageLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// extractMergedLine memisahkan baris target dari teks hasil merge debounce ("a\nb").
// Mengembalikan sisa teks (baris lain digabung ulang) bila target adalah salah satu baris.
func extractMergedLine(merged, target string) (remaining string, ok bool) {
	merged = strings.TrimSpace(merged)
	target = strings.TrimSpace(target)
	if merged == "" || target == "" || !strings.Contains(merged, "\n") {
		return "", false
	}
	if merged == target {
		return "", false
	}
	lines := nonEmptyMessageLines(merged)
	kept := make([]string, 0, len(lines))
	found := false
	for _, line := range lines {
		if !found && line == target {
			found = true
			continue
		}
		kept = append(kept, line)
	}
	if !found || len(kept) == 0 {
		return "", false
	}
	return strings.Join(kept, "\n"), true
}
