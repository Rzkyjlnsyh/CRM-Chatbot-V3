package services

import "testing"

func TestSelectAIResponsePolicy(t *testing.T) {
	// Social (sapa singkat): budget paling pendek.
	p := selectAIResponsePolicy("Halo kak", "", "", 0)
	if p.Name != "social" {
		t.Fatalf("sapa singkat harus social, dapat %q", p.Name)
	}
	// Factual (FAQ harga): knowledgeCount>=2 → factual.
	p = selectAIResponsePolicy("Berapa harga paket premium?", "harga paket", "", 2)
	if p.Name != "factual" {
		t.Fatalf("harga harus factual, dapat %q", p.Name)
	}
	// Catalog (3+ knowledge, kata katalog).
	p = selectAIResponsePolicy("Kirim katalog produknya dong", "katalog produk", "", 3)
	if p.Name != "catalog" {
		t.Fatalf("katalog harus catalog, dapat %q", p.Name)
	}
	// Default conversation.
	p = selectAIResponsePolicy("Bisa minta saran warna baju?", "", "", 0)
	if p.Name != "conversation" {
		t.Fatalf("umum harus conversation, dapat %q", p.Name)
	}
}

func TestResponseNeedsCondensing(t *testing.T) {
	factual := AIResponsePolicy{Name: "factual", MaxTokens: 300, MaxRunes: 330, MaxSentences: 4}
	// Pendek → tidak dipadatkan.
	if responseNeedsCondensing("Baik kak, kami proses ya 🙏", factual) {
		t.Fatal("balasan pendek tidak perlu dipadatkan")
	}
	// Panjang (rune > 330) → dipadatkan.
	long := ""
	for i := 0; i < 40; i++ {
		long += "Kalimat panjang sekali ini menjelaskan banyak hal penting. "
	}
	if !responseNeedsCondensing(long, factual) {
		t.Fatal("balasan panjang harus dipadatkan")
	}
	// Marka grounding [[…]] → dipadatkan (tidak boleh dikirim mentah).
	if !responseNeedsCondensing("Harga [[Rp100.000]] sudah termasuk ongkir", factual) {
		t.Fatal("marka [[ ]] harus memicu condensing")
	}
	// 7 baris > MaxSentences+2 (6) → dipadatkan walau rune di bawah.
	reply := "a\nb\nc\nd\ne\nf\ng"
	if !responseNeedsCondensing(reply, factual) {
		t.Fatal("banyak baris harus memicu condensing")
	}
}
