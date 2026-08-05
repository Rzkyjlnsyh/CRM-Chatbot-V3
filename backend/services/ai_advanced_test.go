package services

import (
	"strings"
	"testing"
	"time"

	"wa-assistant/backend/models"
)

func TestTrimPersonaForPromptShortUnchanged(t *testing.T) {
	p := "Saya CS toko kaos ramah."
	if got := trimPersonaForPrompt(p); got != p {
		t.Fatalf("got %q", got)
	}
}

func TestTrimPersonaForPromptLongCutsAtSentence(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 80; i++ {
		b.WriteString("Kalimat persona nomor ")
		b.WriteString(strings.Repeat("x", 20))
		b.WriteString(". ")
	}
	got := trimPersonaForPrompt(b.String())
	if len([]rune(got)) > personaMaxRunes+80 {
		t.Fatalf("persona terlalu panjang setelah trim: %d", len([]rune(got)))
	}
	if !strings.Contains(got, "dipotong") {
		t.Fatal("harus ada catatan potong")
	}
}

func TestSourceAndFreshnessBoost(t *testing.T) {
	if sourcePriorityBoost("manual") < sourcePriorityBoost("web") {
		t.Fatal("manual harus > web")
	}
	if freshnessBoost(time.Now()) < freshnessBoost(time.Now().AddDate(-2, 0, 0)) {
		t.Fatal("baru harus > lama")
	}
}

func TestKeywordScoreRawAndHybrid(t *testing.T) {
	k := models.Knowledge{Question: "Harga kaos polos?", Answer: "Rp75.000", Tags: "harga,kaos"}
	q := tokenizeQuery("berapa harga kaos")
	if keywordScoreRaw(q, k) <= 0 {
		t.Fatal("keyword score harus positif")
	}
	// Semantic tinggi + keyword
	s1 := hybridKnowledgeScore(0.7, 8, 1.0, 1.0, true)
	s2 := hybridKnowledgeScore(0.2, 0, 1.0, 1.0, true)
	if s1 <= s2 {
		t.Fatalf("score relevan harus unggul: %.3f vs %.3f", s1, s2)
	}
	if hybridKnowledgeScore(0.1, 0, 1, 1, true) != 0 {
		t.Fatal("tanpa sinyal relevan harus 0")
	}
}

func TestKnowledgePairConflictsOnDifferentPrices(t *testing.T) {
	a := models.Knowledge{Question: "Harga kaos polos", Answer: "Harga kaos polos Rp75.000", Tags: "harga,kaos"}
	b := models.Knowledge{Question: "Berapa harga kaos polos", Answer: "Kaos polos seharga Rp99.000", Tags: "harga,kaos"}
	if !knowledgePairConflicts(a, b) {
		t.Fatal("harga beda topik sama harus konflik")
	}
	c := models.Knowledge{Question: "Jam buka", Answer: "Buka jam 08.00-17.00", Tags: "jam"}
	if knowledgePairConflicts(a, c) {
		t.Fatal("topik beda tidak boleh konflik")
	}
	// Jawaban pelengkap angka sama tidak konflik
	d := models.Knowledge{Question: "Harga kaos", Answer: "Kaos polos Rp75.000 cotton combed 24s", Tags: "harga"}
	if knowledgePairConflicts(a, d) {
		t.Fatal("angka sama + topik sama bukan konflik")
	}
}

func TestResolveKnowledgeConflictsDropsLower(t *testing.T) {
	in := []scoredKnowledgeAdv{
		{k: models.Knowledge{ID: 1, Question: "Harga kaos", Answer: "Rp75.000", Tags: "harga,kaos", Source: "manual"}, score: 0.9},
		{k: models.Knowledge{ID: 2, Question: "Harga kaos polos", Answer: "Rp99.000", Tags: "harga,kaos", Source: "web"}, score: 0.5},
		{k: models.Knowledge{ID: 3, Question: "Jam buka", Answer: "08.00-17.00", Tags: "jam"}, score: 0.4},
	}
	out, dropped := resolveKnowledgeConflicts(in)
	if dropped != 1 {
		t.Fatalf("dropped=%d mau 1", dropped)
	}
	if len(out) != 2 || out[0].k.ID != 1 || out[1].k.ID != 3 {
		t.Fatalf("out=%+v", out)
	}
}

func TestSelectKnowledgeAdvancedKeywordPath(t *testing.T) {
	// Tanpa embedding: path keyword multi-sinyal.
	items := []KBItem{
		{K: models.Knowledge{ID: 1, Question: "Harga kaos polos?", Answer: "Rp75.000", Tags: "harga,kaos", Source: "manual", CreatedAt: time.Now()}},
		{K: models.Knowledge{ID: 2, Question: "Harga kaos polos lama", Answer: "Rp99.000", Tags: "harga,kaos", Source: "web", CreatedAt: time.Now().AddDate(-1, 0, 0)}},
		{K: models.Knowledge{ID: 3, Question: "Jam operasional", Answer: "08-17", Tags: "jam", Source: "manual", CreatedAt: time.Now()}},
	}
	got, mode, _ := selectKnowledgeAdvanced("berapa harga kaos polos", items)
	if len(got) == 0 {
		t.Fatal("harus ada hasil")
	}
	// Konflik 75 vs 99 → keep manual lebih tinggi
	for _, g := range got {
		if g.ID == 2 {
			t.Fatalf("knowledge konflik harga lama tidak boleh lolos: mode=%s got=%v", mode, idsOf(got))
		}
	}
	if got[0].ID != 1 {
		t.Fatalf("top harus id=1, got=%v mode=%s", idsOf(got), mode)
	}
}

func TestReplyHasUngroundedNumbers(t *testing.T) {
	kb := []models.Knowledge{{Question: "Harga", Answer: "Rp75.000"}}
	if replyHasUngroundedNumbers("Harganya Rp75.000 kak", kb, "") {
		t.Fatal("angka grounded")
	}
	if !replyHasUngroundedNumbers("Harganya Rp99.000 kak", kb, "") {
		t.Fatal("99k harus ungrounded")
	}
	// Produk context menutup angka
	if replyHasUngroundedNumbers("Harga Rp120.000", nil, "Nama: Jaket\nHarga: Rp120.000") {
		t.Fatal("angka di produk harus grounded")
	}
}

func TestAnswerGroundingOK(t *testing.T) {
	kb := []models.Knowledge{{Question: "Harga kaos", Answer: "Harga kaos polos Rp75.000 cotton combed"}}
	ov, ok, reason := answerGroundingOK("Kaos polos harganya Rp75.000 ya kak, bahan cotton combed.", kb, "")
	if !ok {
		t.Fatalf("harus lolos ok=%v reason=%s ov=%.3f", ok, reason, ov)
	}
	_, ok, reason = answerGroundingOK("Diskon 50% free ongkir ke seluruh dunia.", kb, "")
	if ok {
		t.Fatalf("jawaban ngawur harus gagal, reason=%s", reason)
	}
}

func TestFormatKnowledgeBlockPriority(t *testing.T) {
	items := []models.Knowledge{
		{Question: "Harga?", Answer: "75rb", Source: "manual", CreatedAt: time.Now()},
	}
	block := formatKnowledgeBlock(items)
	if !strings.Contains(block, "prioritas utama") || !strings.Contains(block, "BASIS PENGETAHUAN TERPILIH") {
		t.Fatalf("block=%s", block)
	}
}

func TestFactPriorityInSystemPrompt(t *testing.T) {
	p := buildSystemPrompt(0, "Persona panjang test.")
	if !strings.Contains(p, "PRIORITAS FAKTA") {
		t.Fatal("harus ada prioritas fakta")
	}
	if !strings.Contains(p, "BUKAN sumber angka") {
		t.Fatal("persona harus ditandai bukan sumber angka")
	}
}

func TestLooksLikeInventedSpecificsNormalized(t *testing.T) {
	// 75000 di knowledge sebagai Rp75.000 — jawaban 75.000 harus OK via normalisasi
	relevant := []models.Knowledge{{Question: "Harga", Answer: "Harga kaos polos Rp75.000"}}
	if looksLikeInventedSpecifics("Harga kaos Rp75.000", relevant) {
		t.Fatal("format angka setara tidak boleh invented")
	}
	if !looksLikeInventedSpecifics("Harga Rp88000", relevant) {
		t.Fatal("88000 harus invented")
	}
}
