package handlers

import (
	"strings"
	"testing"
)

func TestNormalizeFollowUpSteps(t *testing.T) {
	steps, err := normalizeFollowUpSteps([]followUpStepReq{
		{DelayHours: 0, Message: "  Halo {nama}  "},
		{DelayHours: 24, AiGenerated: true, AiInstruction: "  Tanyakan apakah masih perlu bantuan  "},
	})
	if err != nil {
		t.Fatalf("normalisasi langkah gagal: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("jumlah langkah = %d, ingin 2", len(steps))
	}
	if steps[0].Message != "Halo {nama}" {
		t.Fatalf("pesan manual tidak dirapikan: %q", steps[0].Message)
	}
	if steps[1].Message != "Tanyakan apakah masih perlu bantuan" ||
		steps[1].AiInstruction != "Tanyakan apakah masih perlu bantuan" {
		t.Fatalf("instruksi AI tidak dinormalisasi: %+v", steps[1])
	}
}

func TestNormalizeFollowUpStepsRejectsInvalidOrder(t *testing.T) {
	_, err := normalizeFollowUpSteps([]followUpStepReq{
		{DelayHours: 24, Message: "Langkah pertama"},
		{DelayHours: 2, Message: "Langkah kedua"},
	})
	if err == nil {
		t.Fatal("urutan waktu mundur seharusnya ditolak")
	}
}

func TestFallbackAIFollowUpDoesNotLeakInstruction(t *testing.T) {
	instruction := "Tawarkan diskon internal yang belum dipublikasikan"
	message := fallbackAIFollowUpMessage("Budi")
	if strings.Contains(message, instruction) {
		t.Fatalf("fallback membocorkan instruksi internal: %q", message)
	}
	if !strings.Contains(message, "Budi") {
		t.Fatalf("fallback tidak memakai nama penerima: %q", message)
	}
}
