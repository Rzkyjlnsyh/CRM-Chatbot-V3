package handlers

import (
	"testing"
	"time"

	"wa-assistant/backend/services"
)

func TestMessageTimeUsesWhatsAppTimestamp(t *testing.T) {
	ts := time.Date(2026, 7, 28, 16, 18, 0, 0, time.Local)
	got := messageTime(ts)
	if !got.Equal(ts) {
		t.Fatalf("messageTime harus memakai timestamp WA, got %v", got)
	}
	if messageTime(time.Time{}).IsZero() {
		t.Fatal("messageTime zero harus fallback ke now, bukan zero")
	}
}

func TestPendingPartFromPreservesPerMessageMetadata(t *testing.T) {
	ts := time.Date(2026, 7, 28, 16, 25, 0, 0, time.Local)
	in := services.IncomingMessage{
		Text: "Siap", WAMsgID: "ABC123", Timestamp: ts,
		ReplyTo: "QUOTE1", ReplyText: "sebelumnya",
	}
	part := pendingPartFrom(in)
	if part.Text != "Siap" || part.ID != "ABC123" || !part.Timestamp.Equal(ts) {
		t.Fatalf("pendingPartFrom tidak menyimpan metadata: %+v", part)
	}
	if part.ReplyTo != "QUOTE1" || part.ReplyText != "sebelumnya" {
		t.Fatalf("reply context hilang: %+v", part)
	}
}

func TestExtractMergedLine(t *testing.T) {
	remaining, ok := extractMergedLine("aapanelnya sy install di server lokal, gmn tuh cara remotenya\nSiap", "Siap")
	if !ok {
		t.Fatal("harusnya mendeteksi baris Siap di merge")
	}
	if remaining != "aapanelnya sy install di server lokal, gmn tuh cara remotenya" {
		t.Fatalf("sisa merge salah: %q", remaining)
	}
	if _, ok := extractMergedLine("satu baris saja", "satu baris saja"); ok {
		t.Fatal("teks tunggal bukan merge")
	}
	if _, ok := extractMergedLine("foo\nbar", "baz"); ok {
		t.Fatal("target yang tidak ada tidak boleh ok")
	}
}
