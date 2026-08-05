package services

import "testing"

func TestGroundedFAQRejectsInventedNumbers(t *testing.T) {
	source := "Kaos tersedia dengan harga Rp75.000. Toko buka pukul 08.00 sampai 17.00."
	items := []QAPair{
		{Question: "Berapa harga kaos?", Answer: "Harga kaos Rp75.000.", Tags: "harga"},
		{Question: "Apakah ada diskon?", Answer: "Ada diskon 20 persen.", Tags: "diskon"},
	}
	got := groundedFAQ(source, items)
	if len(got) != 1 || got[0].Question != items[0].Question {
		t.Fatalf("groundedFAQ = %+v, harus hanya mempertahankan fakta dari sumber", got)
	}
}

func TestGroundedFAQUnderstandsIndonesianPriceScale(t *testing.T) {
	source := "Harga mulai 75 ribu sampai 2 juta rupiah."
	items := []QAPair{
		{Question: "Berapa harga termurah?", Answer: "Harga mulai Rp75.000.", Tags: "harga"},
		{Question: "Berapa harga tertinggi?", Answer: "Harga tertinggi Rp2.000.000.", Tags: "harga"},
	}
	if got := groundedFAQ(source, items); len(got) != 2 {
		t.Fatalf("skala ribu/juta harus dikenali, dapat %+v", got)
	}
}

func TestGroundedFAQRemovesDuplicateOutput(t *testing.T) {
	items := []QAPair{
		{Question: "Apa produknya?", Answer: "Produk kami adalah kaos.", Tags: "produk"},
		{Question: "Apa produknya?", Answer: "Produk kami adalah kaos.", Tags: "produk"},
	}
	if got := groundedFAQ("Kami menjual kaos.", items); len(got) != 1 {
		t.Fatalf("output duplikat harus digabung, dapat %d", len(got))
	}
}
