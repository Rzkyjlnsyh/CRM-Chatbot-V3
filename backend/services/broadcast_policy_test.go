package services

import (
	"testing"
	"time"
)

func TestValidBroadcastConsentEvidence(t *testing.T) {
	now := time.Now()

	// Valid: sumber & tanggal opsional (audit), yang wajib hanya confirmed + jenis pesan valid.
	valid := []struct {
		name      string
		category  string
		source    string
		grantedAt time.Time
		confirmed bool
	}{
		{"lengkap", "marketing", "form", now.Add(-time.Hour), true},
		{"tanpa sumber & tanggal", "marketing", "", time.Time{}, true},
		{"tanpa sumber, ada tanggal", "reminder", "", now.Add(-24 * time.Hour), true},
		{"ada sumber, tanpa tanggal", "order_update", "checkout", time.Time{}, true},
	}
	for _, test := range valid {
		if !ValidBroadcastConsentEvidence(test.category, test.source, test.grantedAt, test.confirmed, now) {
			t.Fatalf("%q should be valid", test.name)
		}
	}

	// Invalid: belum dicentang, jenis pesan ngawur, sumber dikenal tapi salah, tanggal masa depan.
	invalid := []struct {
		name      string
		category  string
		source    string
		grantedAt time.Time
		confirmed bool
	}{
		{"belum dicentang", "marketing", "form", now, false},
		{"jenis pesan ngawur", "unknown", "form", now, true},
		{"sumber diisi tapi tak dikenal", "marketing", "unknown", now, true},
		{"tanggal masa depan", "marketing", "form", now.Add(48 * time.Hour), true},
	}
	for _, test := range invalid {
		if ValidBroadcastConsentEvidence(test.category, test.source, test.grantedAt, test.confirmed, now) {
			t.Fatalf("%q should be invalid", test.name)
		}
	}
}

func TestValidateBroadcastRiskConfirmation(t *testing.T) {
	if got := ValidateBroadcastRiskConfirmation("medium", true, false, "", "", "blocked"); got == "" {
		t.Fatal("medium risk should require acknowledgement")
	}
	if got := ValidateBroadcastRiskConfirmation("medium", true, true, "", "", "blocked"); got != "" {
		t.Fatalf("acknowledged medium risk should pass: %s", got)
	}
	if got := ValidateBroadcastRiskConfirmation("high", true, true, "SALAH", "alasan", "blocked"); got == "" {
		t.Fatal("high risk should require the exact phrase")
	}
	if got := ValidateBroadcastRiskConfirmation("high", true, false, BroadcastOverridePhrase, "Penerima baru mendaftar", "blocked"); got != "" {
		t.Fatalf("complete high-risk override should pass: %s", got)
	}
	if got := ValidateBroadcastRiskConfirmation("low", false, false, "", "", "Belum bisa dikirim"); got != "Belum bisa dikirim" {
		t.Fatalf("blocked assessment should return its title, got %q", got)
	}
}
