package handlers

import "testing"

// TestShouldAllowHumanHandoff memastikan gate eskalasi handoff menerima
// permintaan CS eksplisit (termasuk frasa natural "butuh/mau/ingin/perlu")
// dan kata berisiko, tanpa false-positive pada pesan biasa.
func TestShouldAllowHumanHandoff(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		// Permintaan eksplisit (harus lolos).
		{"saya butuh cs", true},
		{"butuh admin", true},
		{"saya butuh bantuan", true},
		{"mau bicara sama cs", true},
		{"saya ingin hubungi customer service", true},
		{"minta cs", true},
		{"hubungi cs", true},
		{"bicara sama orang", true},
		{"sambungkan ke customer service", true},
		// Kata berisiko (harus lolos).
		{"saya mau komplain", true},
		{"refund dong", true},
		{"ini penipuan", true},
		{"akun saya diblokir", true},
		// Pesan biasa (harus TIDAK lolos — jangan asal eskalasi).
		{"halo kak", false},
		{"saya mau tanya harga", false},
		{"kapan pesanan saya dikirim", false},
		{"terima kasih", false},
		{"ongkirnya berapa ya", false},
	}
	for _, tc := range cases {
		if got := shouldAllowHumanHandoff(tc.msg); got != tc.want {
			t.Errorf("shouldAllowHumanHandoff(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}
