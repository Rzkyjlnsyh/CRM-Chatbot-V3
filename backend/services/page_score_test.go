package services

import (
	"strings"
	"testing"
)

func TestScorePageProductWithPriceRecommended(t *testing.T) {
	url := "https://toko.example.com/produk/kaos-polos"
	title := "Kaos Polos Premium - Harga & Cara Order"
	content := `
Kami menjual kaos polos cotton combed.
Harga kaos Rp75.000 per pcs.
Cara order: hubungi WhatsApp 081234567890.
Pengiriman via JNE dan J&T ke seluruh Indonesia.
Jam operasional Senin-Sabtu pukul 08.00-17.00.
Alamat toko di Jl. Asia Afrika No. 10 Bandung.
Garansi 14 hari untuk cacat produksi.
`
	sc := ScorePageForCSTraining(url, title, content)
	if !sc.Recommended {
		t.Fatalf("halaman produk+harga harus recommended, score=%d tier=%s reasons=%v", sc.Score, sc.Tier, sc.Reasons)
	}
	if sc.Score < recommendScoreMin {
		t.Fatalf("score terlalu rendah: %d", sc.Score)
	}
	if sc.Tier != "good" && sc.Tier != "strong" {
		t.Fatalf("tier=%s", sc.Tier)
	}
}

func TestScorePagePrivacyNotRecommended(t *testing.T) {
	url := "https://toko.example.com/privacy-policy"
	title := "Kebijakan Privasi"
	content := strings.Repeat("Kebijakan privasi dan perlindungan data pribadi pengguna layanan kami. ", 40)
	sc := ScorePageForCSTraining(url, title, content)
	if sc.Recommended {
		t.Fatalf("privacy tidak boleh recommended: score=%d %v", sc.Score, sc.Reasons)
	}
}

func TestScorePageThinNotRecommended(t *testing.T) {
	sc := ScorePageForCSTraining("https://x.com/a", "Halo", "teks pendek saja")
	if sc.Recommended {
		t.Fatal("konten tipis tidak recommended")
	}
}

func TestScorePageListingHubPenalized(t *testing.T) {
	url := "https://toko.example.com/shop"
	title := "Shop"
	content := strings.Repeat("Lihat koleksi kami di menu. ", 30) // navigasi, sedikit CS
	sc := ScorePageForCSTraining(url, title, content)
	// Boleh recommended jika konten kuat; hub murni harus skor lebih rendah dari produk
	product := ScorePageForCSTraining(
		"https://toko.example.com/produk/kaos",
		"Kaos",
		"Harga kaos Rp50.000. Order via WhatsApp 081234567890. Kirim JNE. Alamat Jakarta.",
	)
	if sc.Score >= product.Score {
		t.Fatalf("hub shop score=%d tidak boleh ≥ produk score=%d", sc.Score, product.Score)
	}
}

func TestRankAndSelectPromotesWhenFew(t *testing.T) {
	scores := []PageTrainScore{
		{Score: 38, Recommended: false, Tier: "weak", Reasons: []string{"a"}},
		{Score: 36, Recommended: false, Tier: "weak", Reasons: []string{"b"}},
		{Score: 10, Recommended: false, Tier: "skip", Reasons: []string{"c"}},
	}
	got := RankAndSelectRecommended(scores)
	rec := 0
	for _, s := range got {
		if s.Recommended {
			rec++
		}
	}
	if rec < 2 {
		t.Fatalf("harusnya promote minimal 2 halaman mid-score, rec=%d %+v", rec, got)
	}
	if got[2].Recommended {
		t.Fatal("skip tier tidak boleh dipromote")
	}
}

func TestTokenOverlapRatioGrounding(t *testing.T) {
	src := contentTokenSet("Kami menjual kaos polos harga tujuh puluh lima ribu di bandung")
	// High overlap
	if ov := tokenOverlapRatio("Harga kaos polos di bandung", src); ov < 0.3 {
		t.Fatalf("overlap tinggi diharapkan, dapat %.2f", ov)
	}
	// Low overlap fiction
	if ov := tokenOverlapRatio("Pesawat luar angkasa dan galaksi andromeda tersedia gratis", src); ov >= 0.12 {
		t.Fatalf("overlap fiksi harus rendah, dapat %.2f", ov)
	}
}
