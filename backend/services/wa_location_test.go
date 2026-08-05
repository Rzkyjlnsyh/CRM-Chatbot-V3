package services

import (
	"strings"
	"testing"
)

func TestLocationContextUsesSharedLocationAsFinalDestination(t *testing.T) {
	got := locationContext("Sherlok", "Wedomartani, Sleman", "depan masjid", "", -7.7321, 110.4012, false)

	for _, want := range []string{
		"jangan meminta pelanggan mengirim atau mengonfirmasi lokasi lagi",
		"Nama lokasi: Sherlok",
		"Alamat: Wedomartani, Sleman",
		"Catatan: depan masjid",
		"Koordinat: -7.7321000, 110.4012000",
		"https://maps.google.com/?q=-7.7321000,110.4012000",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("locationContext() tidak memuat %q; hasil: %s", want, got)
		}
	}
}

func TestLocationContextKeepsWhatsAppURL(t *testing.T) {
	const sharedURL = "https://maps.google.com/maps?q=-7.1,110.2"
	got := locationContext("", "", "", sharedURL, -7.1, 110.2, true)
	if !strings.Contains(got, "Live location") || !strings.Contains(got, sharedURL) {
		t.Fatalf("locationContext() tidak mempertahankan live location/link: %s", got)
	}
}

func TestNormalizeLocationLinkText(t *testing.T) {
	shared := "Lokasinya https://maps.app.goo.gl/abc123"
	got := normalizeLocationLinkText(shared)
	if !strings.Contains(got, "jangan meminta pelanggan mengirim atau mengonfirmasi lokasi lagi") || !strings.Contains(got, shared) {
		t.Fatalf("normalizeLocationLinkText() tidak memberi instruksi lokasi final: %s", got)
	}
	plain := "Alamat saya di Wedomartani"
	if got := normalizeLocationLinkText(plain); got != plain {
		t.Fatalf("pesan biasa berubah: %s", got)
	}
}
