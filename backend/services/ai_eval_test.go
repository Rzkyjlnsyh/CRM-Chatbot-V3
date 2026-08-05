package services

import (
	"strings"
	"testing"

	"wa-assistant/backend/models"
)

// Golden set Q&A untuk regresi retrieval (tanpa panggilan network/embedding).
// Fokus: parafrase Indonesia, follow-up pendek, dan skor hybrid produk.
func evalKnowledgeFixture() []KBItem {
	return []KBItem{
		{K: models.Knowledge{ID: 1, Question: "Berapa harga kaos polos?", Answer: "Harga kaos polos Rp75.000 per pcs.", Tags: "harga,kaos"}},
		{K: models.Knowledge{ID: 2, Question: "Jam operasional toko?", Answer: "Kami buka Senin-Sabtu pukul 08.00-17.00 WIB.", Tags: "jam,operasional"}},
		{K: models.Knowledge{ID: 3, Question: "Cara pengiriman barang?", Answer: "Pengiriman via JNE, J&T, dan SiCepat ke seluruh Indonesia.", Tags: "kirim,ongkir,pengiriman"}},
		{K: models.Knowledge{ID: 4, Question: "Apakah ada garansi produk?", Answer: "Garansi 14 hari untuk cacat produksi, bukan salah pakai.", Tags: "garansi"}},
		{K: models.Knowledge{ID: 5, Question: "Lokasi toko di mana?", Answer: "Toko kami di Bandung, Jl. Asia Afrika No. 10.", Tags: "lokasi,alamat"}},
	}
}

func TestEvalKnowledgeKeywordRetrievalGoldenSet(t *testing.T) {
	items := evalKnowledgeFixture()
	cases := []struct {
		query    string
		wantTop  uint
		minScore bool // true = harus ada hasil
	}{
		{"harga kaos berapa ya", 1, true},
		{"kaos polosnya berapa?", 1, true},
		{"biaya kirimnya bagaimana", 3, true},
		{"ongkos kirim pakai apa", 3, true},
		{"jam buka toko", 2, true},
		{"kapan toko buka", 2, true},
		{"ada garansi tidak", 4, true},
		{"alamat toko dimana", 5, true},
		{"lokasi cabang bandung", 5, true},
		// Query tanpa overlap bermakna tidak boleh asal comot.
		{"cuaca di luar sedang bagaimana", 0, false},
	}
	for _, tc := range cases {
		got := keywordSearch(tc.query, items)
		if !tc.minScore {
			if len(got) != 0 {
				t.Errorf("query %q harus kosong, dapat %d item (top=%d)", tc.query, len(got), got[0].ID)
			}
			continue
		}
		if len(got) == 0 {
			t.Errorf("query %q harus menemukan knowledge, kosong", tc.query)
			continue
		}
		if got[0].ID != tc.wantTop {
			t.Errorf("query %q top=%d mau %d (got=%+v)", tc.query, got[0].ID, tc.wantTop, idsOf(got))
		}
	}
}

func idsOf(items []models.Knowledge) []uint {
	out := make([]uint, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}

func TestEvalBuildRetrievalQueryFollowUps(t *testing.T) {
	hist := []models.ChatHistory{
		{Message: "Mau tanya soal kaos", Reply: "Baik kak, kaos yang mana?"},
		{Message: "Yang polos hitam", Reply: "Ukuran apa kak?"},
	}
	// Follow-up pendek harus mengait ke konteks USER (bukan monolog bot).
	q := buildRetrievalQuery("berapa harganya?", hist)
	if !strings.Contains(strings.ToLower(q), "polos") {
		t.Fatalf("follow-up harga harus diperkaya pesan user sebelumnya: %q", q)
	}
	// Jawaban singkat ke pertanyaan asisten → pakai pesan user terakhir.
	q2 := buildRetrievalQuery("XL", []models.ChatHistory{
		{Message: "Mau kaos", Reply: "Ukuran apa yang diinginkan kak?"},
	})
	if !strings.Contains(strings.ToLower(q2), "kaos") && !strings.Contains(q2, "Mau kaos") {
		t.Fatalf("jawaban ukuran singkat tidak diperkaya user context: %q", q2)
	}
}

// Regresi: monolog katalog asisten tidak boleh menenggelamkan intent topik user
// (berlaku lintas bisnis: merchandise, jasa, donasi, dll.).
func TestBuildRetrievalQueryDoesNotPolluteTopicWithBotCatalog(t *testing.T) {
	catalogReply := "Halo kak! Kami punya layanan A, B, C, paket premium, paket basic, " +
		"dan program tambahan X Y Z. Ada yang mau ditanyakan lebih lanjut?"
	hist := []models.ChatHistory{
		{Message: "Hallo kak jual apa", Reply: catalogReply},
	}
	// Intent topik singkat — cukup mandiri, jangan digabung katalog bot.
	q := buildRetrievalQuery("Mau paket premium wilayah jakarta", hist)
	if strings.Contains(q, "program tambahan") || strings.Contains(q, "paket basic") {
		t.Fatalf("query topik tidak boleh dipenuhi katalog bot: %q", q)
	}
	if !strings.Contains(strings.ToLower(q), "premium") {
		t.Fatalf("query harus mempertahankan topik user: %q", q)
	}

	// Follow-up atribut generik mewarisi topik dari PESAN USER, bukan katalog bot.
	hist2 := []models.ChatHistory{
		{Message: "Hallo", Reply: catalogReply},
		{Message: "Saya minat paket premium jakarta", Reply: "Baik kak, ada yang ingin ditanyakan?"},
	}
	q2 := buildRetrievalQuery("Boleh cek kak ada variant apa aja", hist2)
	if !strings.Contains(strings.ToLower(q2), "premium") {
		t.Fatalf("follow-up atribut harus membawa topik user sebelumnya: %q", q2)
	}
	if strings.Contains(q2, "program tambahan") {
		t.Fatalf("follow-up tidak boleh terkontaminasi katalog bot: %q", q2)
	}
}

func TestRetrievalQuerySelfSufficientIsDomainAgnostic(t *testing.T) {
	// Dua token topik — produk ATAU jasa.
	for _, msg := range []string{"sedekah beras", "servis ac", "les matematika", "golang minimalist"} {
		if !retrievalQuerySelfSufficient(msg) {
			t.Fatalf("harus self-sufficient: %q", msg)
		}
	}
	// Follow-up atribut murni — butuh history.
	for _, msg := range []string{"berapa harganya", "ada variant apa aja", "jadwalnya kapan"} {
		if retrievalQuerySelfSufficient(msg) {
			t.Fatalf("follow-up atribut tidak boleh self-sufficient: %q", msg)
		}
	}
}

func TestEvalProductHybridScorePrefersSemanticParaphrase(t *testing.T) {
	// Keyword 0 + sim tinggi harus mengalahkan keyword 0 + sim rendah.
	high := productHybridScore(0, 0.62)
	low := productHybridScore(0, 0.20)
	if high <= 0 {
		t.Fatalf("parafrase semantik kuat harus lolos, score=%.3f", high)
	}
	if low != 0 {
		t.Fatalf("sim di bawah floor harus 0, score=%.3f", low)
	}
	// Keyword nama produk + sim sedang harus unggul vs pure weak semantic.
	named := productHybridScore(10, 0.40)
	if named <= high {
		t.Fatalf("keyword kuat + sim harus unggul: named=%.3f highSem=%.3f", named, high)
	}
}

func TestEvalProductKeywordRankingStillWorks(t *testing.T) {
	kaos := models.Product{ID: 1, Name: "Kaos Polos Premium", Description: "Cotton combed 24s", Price: "Rp75.000"}
	celana := models.Product{ID: 2, Name: "Celana Chino Slim", Description: "Bahan katun stretch", Price: "Rp185.000"}
	q := tokenizeQuery("harga kaos polos berapa")
	if productRelevanceScore(kaos, q) <= productRelevanceScore(celana, q) {
		t.Fatalf("kaos harus unggul untuk query kaos, kaos=%d celana=%d",
			productRelevanceScore(kaos, q), productRelevanceScore(celana, q))
	}
	// Parafrase tanpa kata 'celana' di query tetap bisa diangkat lewat hybrid score semantik.
	// (sim disuntik; tanpa network)
	if productHybridScore(0, 0.55) <= 0 {
		t.Fatal("hybrid semantic harus bisa mengangkat produk tanpa overlap keyword")
	}
}

func TestEvalAnswerOverlapAndGroundingThreshold(t *testing.T) {
	kb := []models.Knowledge{
		{Question: "Harga kaos", Answer: "Harga kaos polos Rp75.000 cotton combed"},
	}
	good := "Kaos polos harganya Rp75.000 ya kak, bahan cotton combed."
	bad := "Diskon 50% dan free ongkir ke seluruh dunia tanpa syarat."
	if answerKnowledgeOverlap(good, kb) < knowledgeOverlapMin {
		t.Fatalf("jawaban grounded harus overlap >= %.2f, dapat %.3f", knowledgeOverlapMin, answerKnowledgeOverlap(good, kb))
	}
	if answerKnowledgeOverlap(bad, kb) >= knowledgeOverlapMin && !looksLikeInventedSpecifics(bad, kb) {
		t.Fatalf("jawaban ngawur harus terdeteksi lemah: overlap=%.3f invented=%v",
			answerKnowledgeOverlap(bad, kb), looksLikeInventedSpecifics(bad, kb))
	}
	if !looksLikeInventedSpecifics(bad, kb) {
		t.Fatal("angka 50 yang tidak ada di knowledge harus dianggap invented")
	}
}

func TestEvalFactualVsChitchatForGroundingGate(t *testing.T) {
	// Gate anti-halusinasi hanya untuk pertanyaan faktual.
	for _, msg := range []string{"Halo kak", "Ok siap", "Terima kasih ya"} {
		if looksFactualUserMessage(msg) {
			t.Fatalf("%q tidak boleh faktual", msg)
		}
	}
	for _, msg := range []string{"Berapa harga kaos?", "Jam operasional toko", "Ada garansi berapa lama"} {
		if !looksFactualUserMessage(msg) {
			t.Fatalf("%q harus faktual", msg)
		}
	}
}

func TestEvalMergeKnowledgeDedupesNearDuplicateAnswers(t *testing.T) {
	primary := []models.Knowledge{
		{ID: 1, Answer: "Harga kaos Rp75.000"},
		{ID: 2, Answer: "Buka pukul delapan"},
	}
	secondary := []models.Knowledge{
		{ID: 3, Answer: "Harga kaos Rp75.000"}, // duplikat jawaban
		{ID: 4, Answer: "Kirim via JNE"},
	}
	got := mergeKnowledgeResults(primary, secondary, 3)
	if len(got) != 3 {
		t.Fatalf("len=%d mau 3", len(got))
	}
	for _, item := range got {
		if item.ID == 3 {
			t.Fatal("jawaban duplikat tidak boleh masuk")
		}
	}
}
