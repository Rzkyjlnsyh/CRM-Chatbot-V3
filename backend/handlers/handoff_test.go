package handlers

import (
	"strings"
	"testing"
)

func TestHumanFacingHoldReplyIsHumanCSVoice(t *testing.T) {
	lower := strings.ToLower(humanFacingHoldReply)
	for _, bad := range []string{"petugas", "admin", "diteruskan", " bot", "bot ", "cs lain", "operator"} {
		if strings.Contains(lower, bad) {
			t.Fatalf("hold reply membongkar persona: %q di %q", bad, humanFacingHoldReply)
		}
	}
	// Hindari false positive substring "ai" di kata "tepat".
	if strings.Contains(lower, " sebagai ai") || strings.Contains(lower, "saya ai") {
		t.Fatalf("hold reply menyebut AI: %q", humanFacingHoldReply)
	}
	if !strings.Contains(lower, "cek") && !strings.Contains(lower, "tunggu") {
		t.Fatalf("hold reply harus natural cek/tunggu: %q", humanFacingHoldReply)
	}
}

func TestStripInternalStaffSpeak(t *testing.T) {
	in := "Baik, percakapan diteruskan ke CS ya. Petugas akan bantu."
	got := stripInternalStaffSpeak(in)
	lower := strings.ToLower(got)
	if strings.Contains(lower, "diteruskan ke cs") || strings.Contains(lower, "petugas") {
		t.Fatalf("masih ada istilah internal: %q", got)
	}
}

func TestSoftHandoffSensitiveUsesHandoffSignals(t *testing.T) {
	if !isSoftHandoffSensitive("tolong hubungkan ke CS") {
		t.Fatal("permintaan manusia harus sensitif di soft mode")
	}
	if isSoftHandoffSensitive("berapa harga kaos?") {
		t.Fatal("FAQ harga tidak boleh diblok sebagai sensitif")
	}
}
