package handlers

import (
	"strings"
	"testing"
)

func TestSpinTextPilihSalahSatuOpsi(t *testing.T) {
	got := spinText("{Halo|Hai|Pagi} kak")
	if got != "Halo kak" && got != "Hai kak" && got != "Pagi kak" {
		t.Fatalf("hasil di luar opsi: %q", got)
	}
}

func TestSpinTextTanpaSpinTidakBerubah(t *testing.T) {
	for _, s := range []string{
		"pesan biasa tanpa kurung",
		"Halo {nama}, ada promo",       // placeholder personalize harus utuh
		"harga {masih dirahasiakan}",   // kurung tanpa '|' -> literal
		"kurung nyasar { tanpa tutup",  // tak berpasangan -> literal
		"tutup } duluan { juga aman",
	} {
		if got := spinText(s); got != s {
			t.Fatalf("spinText(%q) = %q, harusnya tidak berubah", s, got)
		}
	}
}

func TestSpinTextBersarang(t *testing.T) {
	got := spinText("{Hai {kak|gan}|Halo}")
	valid := map[string]bool{"Hai kak": true, "Hai gan": true, "Halo": true}
	if !valid[got] {
		t.Fatalf("hasil nested di luar opsi: %q", got)
	}
}

func TestSpinTextNamaDiDalamOpsi(t *testing.T) {
	got := spinText("{Halo {nama}|Hai}")
	if got != "Halo {nama}" && got != "Hai" {
		t.Fatalf("{nama} dalam opsi rusak: %q", got)
	}
}

func TestSpinTextVariasiNyata(t *testing.T) {
	// Dengan cukup banyak percobaan, minimal 2 varian harus muncul (bukti benar-benar acak).
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		seen[spinText("{A|B|C}")] = true
	}
	if len(seen) < 2 {
		t.Fatalf("50 percobaan hanya menghasilkan %v — tidak bervariasi", seen)
	}
	for v := range seen {
		if strings.ContainsAny(v, "{}|") {
			t.Fatalf("hasil masih mengandung sintaks spin: %q", v)
		}
	}
}
