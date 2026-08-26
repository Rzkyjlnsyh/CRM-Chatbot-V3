package services

import (
	"strings"
	"testing"
)

// Anti-halu: parser harus menolak stage yang tidak ada di daftar user,
// reason kosong, dan JSON sampah — agar AI tidak pernah menciptakan tahap.

func TestParseCRMLeadAssessmentAcceptsOnlyAllowedKeys(t *testing.T) {
	allowed := map[string]bool{"new": true, "cold": true, "warm": true, "hot": true, "unqualified": true}
	got, err := parseCRMLeadAssessment(`{"stage":"hot","confidence":0.9,"reason":"pelanggan minta checkout"}`, allowed)
	if err != nil {
		t.Fatalf("parse valid gagal: %v", err)
	}
	if got.Stage != "hot" || got.Confidence != 0.9 || got.Reason == "" {
		t.Fatalf("hasil tidak sesuai: %+v", got)
	}
}

func TestParseCRMLeadAssessmentRejectsInventedStage(t *testing.T) {
	allowed := map[string]bool{"new": true, "warm": true, "hot": true}
	for _, raw := range []string{
		`{"stage":"closing_aja","confidence":0.99,"reason":"sudah deal"}`,
		`{"stage":"CUSTOM_FAKE","confidence":0.9,"reason":"ngarang"}`,
		`{"stage":"customer","confidence":0.95,"reason":"deel"}`,
	} {
		if _, err := parseCRMLeadAssessment(raw, allowed); err == nil {
			t.Fatalf("seharusnya menolak stage di luar daftar: %s", raw)
		}
	}
}

func TestParseCRMLeadAssessmentRequiresReason(t *testing.T) {
	allowed := map[string]bool{"warm": true}
	if _, err := parseCRMLeadAssessment(`{"stage":"warm","confidence":0.8,"reason":"  "}`, allowed); err == nil {
		t.Fatal("reason kosong seharusnya ditolak")
	}
	if _, err := parseCRMLeadAssessment(`{"stage":"warm","confidence":0.8}`, allowed); err == nil {
		t.Fatal("reason hilang seharusnya ditolak")
	}
	if _, err := parseCRMLeadAssessment(`ini bukan json`, allowed); err == nil {
		t.Fatal("JSON sampah seharusnya ditolak")
	}
}

func TestParseCRMLeadAssessmentClampsConfidence(t *testing.T) {
	allowed := map[string]bool{"warm": true}
	got, err := parseCRMLeadAssessment(`{"stage":"warm","confidence":7.5,"reason":"tes"}`, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if got.Confidence != 1 {
		t.Fatalf("confidence harus di-clamp ke 1, dapat %v", got.Confidence)
	}
	got, err = parseCRMLeadAssessment(`{"stage":"warm","confidence":-3,"reason":"tes"}`, allowed)
	if err != nil {
		t.Fatal(err)
	}
	if got.Confidence != 0 {
		t.Fatalf("confidence harus di-clamp ke 0, dapat %v", got.Confidence)
	}
}

func TestBuildCRMClassifierPromptEmbedsUserStagesAndClosing(t *testing.T) {
	stages := []CRMStageHint{
		{Key: "new", Name: "Baru", Description: "belum jelas", IsClosing: false},
		{Key: "deal", Name: "Deal", Description: "transfer diterima", IsClosing: true},
	}
	prompt := buildCRMClassifierPrompt(stages, "Closing = customer sudah transfer DP minimal 50rb", map[string]bool{"new": true, "deal": true})
	for _, want := range []string{"new", "Baru", "deal", "Deal", "transfer DP minimal 50rb", "JANGAN PERNAH dipakai AI"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt tidak memuat %q:\n%s", want, prompt)
		}
	}
}
