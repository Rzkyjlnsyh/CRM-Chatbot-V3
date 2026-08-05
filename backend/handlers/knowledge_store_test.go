package handlers

import (
	"errors"
	"testing"
)

func TestCanonicalKnowledgeQuestion(t *testing.T) {
	left := canonicalKnowledgeQuestion("  Berapa Harga Produknya? ")
	right := canonicalKnowledgeQuestion("berapa   harga produknya")
	if left != right {
		t.Fatalf("pertanyaan ekuivalen tidak sama: %q != %q", left, right)
	}
	if canonicalKnowledgeQuestion("Harga / Produk - Terbaru") != canonicalKnowledgeQuestion("harga produk terbaru?") {
		t.Fatal("tanda baca internal harus dianggap sebagai pemisah kata")
	}
}

func TestNormalizeKnowledgeTags(t *testing.T) {
	got := normalizeKnowledgeTags(" Harga,produk, harga ", "web")
	if got != "harga,produk,web" {
		t.Fatalf("tags = %q", got)
	}
}

func TestKnowledgeLooksDuplicateAcrossWording(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  bool
	}{
		{"urutan dan sapaan berbeda", "Berapa harga produknya, Kak?", "Harga produk ini berapa?", true},
		{"sinonim order", "Bagaimana cara melakukan pemesanan?", "Cara order produk bagaimana?", true},
		{"sinonim ongkir", "Berapa biaya kirim ke Bandung?", "Berapa ongkir ke Bandung?", true},
		{"topik berbeda", "Berapa harga produknya?", "Kapan jam operasional toko?", false},
		{"produk spesifik berbeda", "Berapa harga kaos merah ukuran XL?", "Berapa harga sepatu putih ukuran 42?", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := knowledgeLooksDuplicate(tt.left, "", tt.right, ""); got != tt.want {
				t.Fatalf("knowledgeLooksDuplicate(%q, %q) = %v, mau %v", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestKnowledgeLooksDuplicateByAnswer(t *testing.T) {
	if !knowledgeLooksDuplicate(
		"Apa jam buka toko?", "Toko buka pukul 08.00 sampai 17.00.",
		"Kapan customer service tersedia?", "Toko buka pukul 08.00 sampai 17.00!",
	) {
		t.Fatal("jawaban faktual identik harus dianggap duplikat")
	}
}

func TestKnowledgeSourcePriority(t *testing.T) {
	order := []string{"manual", "text", "import", "wizard", "web"}
	for i := 0; i < len(order)-1; i++ {
		if knowledgeSourcePriority(order[i]) <= knowledgeSourcePriority(order[i+1]) {
			t.Fatalf("prioritas %s harus lebih tinggi dari %s", order[i], order[i+1])
		}
	}
}

func TestBuildFallbackFAQOnlyUsesProvidedSetupFacts(t *testing.T) {
	items := buildFallbackFAQ(SetupWizardReq{
		BizName: "Toko Contoh", Products: "Kaos polos", Payment: "QRIS",
		Location: "Bandung", Policies: "Penukaran maksimal 3 hari",
	})
	if len(items) != 5 {
		t.Fatalf("fallback harus berisi profil, produk, pembayaran, lokasi, kebijakan; dapat %+v", items)
	}
	for _, item := range items {
		if item.Answer == "" {
			t.Fatalf("fallback tidak boleh membuat jawaban kosong: %+v", item)
		}
	}
}

func TestWebTrainingErrorMessage(t *testing.T) {
	if got := webTrainingErrorMessage(errors.New("401 unauthorized")); got != "API key AI belum diisi atau tidak valid" {
		t.Fatalf("error 401 = %q", got)
	}
	if got := webTrainingErrorMessage(errors.New("quota exceeded")); got != "Kuota/kredit provider AI habis atau sedang dibatasi" {
		t.Fatalf("error quota = %q", got)
	}
}
