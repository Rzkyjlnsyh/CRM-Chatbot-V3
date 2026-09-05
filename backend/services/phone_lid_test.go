package services

import "testing"

// TestLooksLikeLID — ambang deteksi identitas LID WhatsApp:
// LID 15–17 digit, ID grup 18+ digit, nomor asing 13–14 digit, nomor
// Indonesia ≤14 digit. Ambang ini melindungi data nyata (grup & pelanggan
// internasional) dari migrasi/penghapusan junk.
func TestLooksLikeLID(t *testing.T) {
	lids := []string{
		"188515224649897",  // 15 digit
		"6288683088748742", // 16 digit berawalan 62
		"262225537306813",  // 15 digit
	}
	for _, s := range lids {
		if !LooksLikeLID(s) {
			t.Errorf("LID %q harus terdeteksi", s)
		}
	}

	notLIDs := []string{
		"120363314264721268",      // ID grup 18 digit (tanpa @g.us)
		"120363243909862389",      // ID grup 18 digit
		"8613544480517",           // nomor China 13 digit
		"8801805042296",           // nomor Bangladesh 13 digit
		"6285649500668",           // nomor Indonesia 13 digit
		"08123456789",             // nomor lokal
		"120363314264721268@g.us", // grup dengan sufiks
		"abc123",                  // bukan digit
	}
	for _, s := range notLIDs {
		if LooksLikeLID(s) {
			t.Errorf("%q TIDAK boleh dikira LID", s)
		}
	}
}

// TestNormalizePhoneDoesNotMangleLID — digit non-Indonesia TIDAK boleh
// dipaksa jadi nomor 62xx palsu.
func TestNormalizePhoneDoesNotMangleLID(t *testing.T) {
	cases := map[string]string{
		"08123456789":        "628123456789",       // 0 → 62
		"8123456789":         "628123456789",       // 8 → 628
		"628123456789":       "628123456789",       // sudah 62 → tetap
		"188515224649897":    "188515224649897",    // LID: apa adanya, BUKAN 62-palsu
		"120363314264721268": "120363314264721268", // grup: apa adanya
		"8613544480517":      "8613544480517",      // asing: apa adanya
	}
	for in, want := range cases {
		if got := NormalizePhone(in); got != want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", in, got, want)
		}
	}
}
