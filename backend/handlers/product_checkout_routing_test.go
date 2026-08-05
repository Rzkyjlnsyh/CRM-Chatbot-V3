package handlers

import (
	"strings"
	"testing"

	"wa-assistant/backend/models"
)

func TestProductCheckoutIntent(t *testing.T) {
	for _, text := range []string{"Saya mau beli sofa", "mau sedekah kursi", "tolong jemput meja"} {
		if !productCheckoutIntent(text) {
			t.Fatalf("niat checkout tidak dikenali: %q", text)
		}
	}
	for _, text := range []string{"berapa harga sofa?", "sofa tersedia?"} {
		if productCheckoutIntent(text) {
			t.Fatalf("pertanyaan informasi dianggap checkout: %q", text)
		}
	}
}

func TestNormalizeProductDetails(t *testing.T) {
	got, err := normalizeProductDetails(`[
		{"label":"Ukuran","value":"S, M, L"},
		{"label":"Kosong","value":""}
	]`)
	if err != nil || !strings.Contains(got, "Ukuran") || strings.Contains(got, "Kosong") {
		t.Fatalf("detail tidak dinormalisasi: %q err=%v", got, err)
	}
	if _, err := normalizeProductDetails(`[{"label":"Ukuran","value":"S"},{"label":"ukuran","value":"M"}]`); err == nil {
		t.Fatal("label detail duplikat harus ditolak")
	}
}

func TestNormalizeProductType(t *testing.T) {
	if got := normalizeProductType("digital"); got != "digital" {
		t.Fatalf("jenis digital berubah menjadi %q", got)
	}
	if got := normalizeProductType("tidak-valid"); got != "physical" {
		t.Fatalf("fallback jenis produk = %q", got)
	}
}

func TestProductTextScorePrioritizesProductName(t *testing.T) {
	sofa := models.Product{Name: "Penjemputan Sofa", Description: "Barang bekas layak pakai"}
	chair := models.Product{Name: "Penjemputan Kursi", Description: "Barang bekas layak pakai"}
	message := "Saya mau sedekah kursi"
	if productTextScore(chair, message) <= productTextScore(sofa, message) {
		t.Fatal("produk dengan nama yang disebut pelanggan harus mendapat skor lebih tinggi")
	}
}

func TestProductDirectiveIDAllowsIntroText(t *testing.T) {
	id, found, valid := productDirectiveID("Baik kak. [[START_PRODUCT:12]]")
	if id != 12 || !found || !valid {
		t.Fatalf("directive tidak terbaca: id=%d found=%v valid=%v", id, found, valid)
	}
}

func TestEditProductDirectiveID(t *testing.T) {
	id, found, valid := editProductDirectiveID("Baik kak [[EDIT_PRODUCT:12]]")
	if id != 12 || !found || !valid {
		t.Fatalf("directive edit produk tidak terbaca: id=%d found=%v valid=%v", id, found, valid)
	}
}

func TestCheckoutConfirmationUsesFriendlyActions(t *testing.T) {
	product := models.Product{Name: "Kaos"}
	steps := []checkoutStepConfig{{Key: "size", Label: "Ukuran?", Type: "text", Required: true}}
	result := checkoutConfirmation(models.ProductCheckoutSession{ID: 9, DataJSON: `{"size":"L"}`}, product, steps)
	if len(result.buttons) != 3 || result.buttons[0].Text != "Data sudah benar" || result.buttons[1].Text != "Ubah data" {
		t.Fatalf("tombol konfirmasi checkout tidak ramah: %#v", result.buttons)
	}
}

func TestProductButtonDisplayText(t *testing.T) {
	if got := productButtonDisplayText(productButtonConfig{Label: "Pesan", Action: "checkout"}); got != "🛒 Pesan" {
		t.Fatalf("ikon bawaan checkout = %q", got)
	}
	if got := productButtonDisplayText(productButtonConfig{Label: "Hubungi", Icon: "📞", Action: "handoff"}); got != "📞 Hubungi" {
		t.Fatalf("ikon pilihan = %q", got)
	}
	if got := productButtonDisplayText(productButtonConfig{Label: "Info", Icon: "none", Action: "reply"}); got != "Info" {
		t.Fatalf("tanpa ikon = %q", got)
	}
}

func TestParseProductButtonsJSONPreservesBlastSnapshot(t *testing.T) {
	buttons := parseProductButtonsJSON(`[{"key":"buy","label":"Beli","icon":"🎁","action":"checkout"}]`)
	if len(buttons) != 1 || buttons[0].Key != "buy" || productButtonDisplayText(buttons[0]) != "🎁 Beli" {
		t.Fatalf("snapshot tombol produk tidak terbaca: %#v", buttons)
	}
}
