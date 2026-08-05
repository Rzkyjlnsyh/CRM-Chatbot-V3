package services

import (
	"strings"
	"testing"
)

func TestExtractHTTPURLs(t *testing.T) {
	text := "Lokasi toko di https://maps.app.goo.gl/AbCdEf ya kak, atau https://www.google.com/maps/place/Bandung/@-6.9,107.6,17z."
	got := extractHTTPURLs(text)
	if len(got) < 2 {
		t.Fatalf("harusnya ≥2 URL, dapat %v", got)
	}
	if !strings.Contains(got[0], "maps.app.goo.gl") {
		t.Fatalf("URL pertama: %s", got[0])
	}
}

func TestIsMapsURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://maps.app.goo.gl/xxx", true},
		{"https://www.google.com/maps/place/Foo", true},
		{"https://maps.google.com/?q=-6.2,106.8", true},
		{"https://tokopedia.com/foo", false},
		{"https://example.com", false},
	}
	for _, tc := range cases {
		if got := isMapsURL(tc.url); got != tc.want {
			t.Errorf("isMapsURL(%s)=%v mau %v", tc.url, got, tc.want)
		}
	}
}

func TestParseMapsURLPlaceAndCoords(t *testing.T) {
	u := "https://www.google.com/maps/place/Monas/@-6.1753924,106.8271528,17z"
	p := parseMapsURL(u)
	if !p.HasCoord {
		t.Fatal("harus ada koordinat")
	}
	if p.Latitude < -6.2 || p.Latitude > -6.1 {
		t.Fatalf("lat=%v", p.Latitude)
	}
	if p.PlaceName == "" || !strings.Contains(strings.ToLower(p.PlaceName), "monas") {
		t.Fatalf("place name=%q", p.PlaceName)
	}
}

func TestParseMapsURLQueryCoords(t *testing.T) {
	u := "https://maps.google.com/?q=-6.2000000,106.8166667"
	p := parseMapsURL(u)
	if !p.HasCoord {
		t.Fatal("harus parse q=lat,lng")
	}
}

func TestParseMapsURLBangCoords(t *testing.T) {
	u := "https://www.google.com/maps/place/Foo/data=!3d-6.2!4d106.8"
	p := parseMapsURL(u)
	if !p.HasCoord {
		t.Fatal("harus parse !3d!4d")
	}
}

func TestEnrichUserMessageForAIMapsWithoutNetwork(t *testing.T) {
	// URL panjang dengan koordinat di path — tidak bergantung resolve network.
	msg := "Alamat kirim di sini ya https://www.google.com/maps/place/Gedung+Sate/@-6.9024853,107.6187044,17z"
	got := EnrichUserMessageForAI(msg)
	if !strings.Contains(got, "[KONTEKS LINK LOKASI") {
		t.Fatalf("harus ada blok konteks lokasi:\n%s", got)
	}
	if !strings.Contains(got, "Koordinat:") {
		t.Fatalf("harus ada koordinat:\n%s", got)
	}
	if !strings.Contains(strings.ToLower(got), "gedung") {
		t.Fatalf("harus ada nama tempat:\n%s", got)
	}
	if !strings.Contains(got, "Instruksi:") {
		t.Fatalf("harus ada instruksi ke model:\n%s", got)
	}
}

func TestEnrichUserMessageForAINoURL(t *testing.T) {
	msg := "Halo kak, harga berapa?"
	if got := EnrichUserMessageForAI(msg); got != msg {
		t.Fatalf("tanpa URL harus tidak berubah: %q", got)
	}
}

func TestAssertPublicHTTPURLBlocksLocal(t *testing.T) {
	if err := assertPublicHTTPURL("http://127.0.0.1/admin"); err == nil {
		t.Fatal("localhost harus ditolak")
	}
	if err := assertPublicHTTPURL("http://192.168.1.1/"); err == nil {
		t.Fatal("private IP harus ditolak")
	}
}
