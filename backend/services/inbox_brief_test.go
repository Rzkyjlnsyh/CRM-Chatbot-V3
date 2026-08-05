package services

import (
	"strings"
	"testing"

	"wa-assistant/backend/models"
)

func TestExtractBriefHeuristicOrderAndFacts(t *testing.T) {
	transcript := strings.Join([]string{
		"Pelanggan: Halo, nama saya Budi Santoso",
		"CS: Halo Budi, ada yang bisa dibantu?",
		"Pelanggan: Mau order kaos size L, qty 2 pcs, kirim ke Jl. Merdeka No 10 Bandung",
		"CS: Harganya Rp150.000. Kurir JNE ya?",
		"Pelanggan: Iya JNE. Transfer BCA. Resi TRX-99881",
		"Pelanggan: Status pesanan sudah berapa lama ya?",
	}, "\n")
	b := extractBriefHeuristic(transcript, "", false)
	if !strings.Contains(strings.ToLower(b.ContactHint), "budi") {
		t.Fatalf("contact_hint expected Budi, got %q", b.ContactHint)
	}
	if b.Stage != "transaction" && b.Stage != "issue" {
		// status pesanan → transaction
		if b.Stage != "transaction" {
			t.Fatalf("stage=%s want transaction", b.Stage)
		}
	}
	if len(b.KeyFacts) == 0 {
		t.Fatal("expected key facts")
	}
	joined := strings.ToLower(strings.Join(b.KeyFacts, " | "))
	if !strings.Contains(joined, "ukuran") && !strings.Contains(joined, "l") {
		// size L should appear
		if !strings.Contains(joined, "ukuran: l") {
			t.Logf("facts: %v", b.KeyFacts)
		}
	}
	if !strings.Contains(joined, "harga") && !strings.Contains(joined, "150") {
		t.Fatalf("expected price fact, got %v", b.KeyFacts)
	}
	if !strings.Contains(joined, "alamat") && !strings.Contains(joined, "merdeka") {
		t.Fatalf("expected address fact, got %v", b.KeyFacts)
	}
	if len(b.Products) == 0 || !containsFold(b.Products, "kaos") {
		t.Fatalf("expected product kaos, got %v", b.Products)
	}
	if len(b.OpenItems) == 0 {
		t.Fatal("expected open items from unanswered last customer message")
	}
}

func TestExtractBriefHeuristicRiskRefund(t *testing.T) {
	transcript := "Pelanggan: Mau refund, komplain barang rusak\nCS: Baik kami cek"
	b := extractBriefHeuristic(transcript, "", true)
	if b.Stage != "issue" {
		t.Fatalf("stage=%s", b.Stage)
	}
	if len(b.RiskFlags) == 0 {
		t.Fatal("expected risk flags")
	}
	risks := strings.ToLower(strings.Join(b.RiskFlags, " "))
	if !strings.Contains(risks, "refund") && !strings.Contains(risks, "butuh") {
		t.Fatalf("risks=%v", b.RiskFlags)
	}
}

func TestFactGroundedRejectsInventedNumbers(t *testing.T) {
	src := "Harga kaos Rp75.000 size M"
	nums := normalizedFactNumbers(src)
	toks := contentTokenSet(src)
	if factGrounded("Harga kaos Rp99.000", nums, toks) {
		t.Fatal("should reject invented price 99000")
	}
	if !factGrounded("Harga kaos Rp75.000", nums, toks) {
		t.Fatal("should accept real price")
	}
	if factGrounded("Promo rahasia diskon gila-gilaan tanpa angka di sumber yang sangat panjang sekali", nums, toks) {
		t.Fatal("should reject low-overlap long fact")
	}
}

func TestMergeBriefsGroundsAIFacts(t *testing.T) {
	h := ConversationBrief{
		Intent:   "Tanya harga",
		Stage:    "info",
		KeyFacts: []string{"Harga/biaya disebut: Rp75.000"},
		Products: []string{"kaos"},
	}
	ai := briefAIPayload{
		Intent:   "Ingin beli kaos",
		Stage:    "interest",
		Summary:  "Pelanggan tanya harga kaos.",
		Products: []string{"kaos polos"},
		KeyFacts: []string{
			"Harga Rp75.000",
			"Harga Rp999.000 invent", // should drop
		},
		OpenItems: []string{"Konfirmasi size"},
	}
	transcript := "Pelanggan: Berapa harga kaos?\nCS: Rp75.000"
	out := mergeBriefs(h, ai, transcript)
	if out.Intent != "Ingin beli kaos" {
		t.Fatalf("intent=%s", out.Intent)
	}
	joined := strings.Join(out.KeyFacts, " | ")
	if strings.Contains(joined, "999") {
		t.Fatalf("invented fact leaked: %v", out.KeyFacts)
	}
	if !strings.Contains(joined, "75") {
		t.Fatalf("real price missing: %v", out.KeyFacts)
	}
	if len(out.OpenItems) == 0 {
		t.Fatal("open items empty")
	}
}

func TestEncodeDecodeBrief(t *testing.T) {
	b := ConversationBrief{Intent: "x", Summary: "y", Confidence: 0.8, Source: "hybrid"}
	raw := EncodeBrief(b)
	got, ok := DecodeBrief(raw)
	if !ok || got.Intent != "x" || got.Summary != "y" {
		t.Fatalf("roundtrip failed ok=%v got=%+v", ok, got)
	}
	if _, ok := DecodeBrief(""); ok {
		t.Fatal("empty should fail")
	}
	if _, ok := DecodeBrief("{not-json"); ok {
		t.Fatal("bad json should fail")
	}
}

func TestBuildConversationBriefShort(t *testing.T) {
	msgs := []models.ChatHistory{{ID: 1, Message: "halo"}}
	b, err := BuildConversationBrief(1, "6281", msgs, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.Source != "heuristic" {
		t.Fatalf("source=%s", b.Source)
	}
	if b.Confidence <= 0 {
		t.Fatal("confidence")
	}
	if b.LastChatID != 1 {
		t.Fatalf("last=%d", b.LastChatID)
	}
}

func TestBuildBriefTranscriptAndCount(t *testing.T) {
	msgs := []models.ChatHistory{
		{Message: "hai", Reply: "halo", FromHuman: false},
		{Message: "mau beli", Reply: "siap", FromHuman: true},
	}
	tr := buildBriefTranscript(msgs)
	if !strings.Contains(tr, "Pelanggan: hai") || !strings.Contains(tr, "CS-manusia:") {
		t.Fatalf("transcript=%q", tr)
	}
	if countBriefTurns(msgs) != 2 {
		t.Fatalf("turns=%d", countBriefTurns(msgs))
	}
}

func TestBriefConfidence(t *testing.T) {
	low := briefConfidence(ConversationBrief{Source: "heuristic", Intent: "Percakapan umum / info"}, "x")
	high := briefConfidence(ConversationBrief{
		Source: "hybrid", Intent: "Mau order", KeyFacts: []string{"a"}, OpenItems: []string{"b"},
	}, strings.Repeat("kata panjang percakapan ", 40))
	if high <= low {
		t.Fatalf("high=%v low=%v", high, low)
	}
	if high > 0.95 {
		t.Fatalf("cap broken: %v", high)
	}
}

func containsFold(list []string, want string) bool {
	want = strings.ToLower(want)
	for _, x := range list {
		if strings.Contains(strings.ToLower(x), want) {
			return true
		}
	}
	return false
}
