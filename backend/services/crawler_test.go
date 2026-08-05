package services

import (
	"encoding/xml"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestSitemapDecode(t *testing.T) {
	var us sitemapDoc
	if err := xml.Unmarshal([]byte(`<urlset><url><loc>https://x.com/a</loc></url><url><loc>https://x.com/b</loc></url></urlset>`), &us); err != nil {
		t.Fatal(err)
	}
	if len(us.URLs) != 2 || us.URLs[0].Loc != "https://x.com/a" {
		t.Errorf("urlset decode salah: %+v", us.URLs)
	}
	var idx sitemapDoc
	if err := xml.Unmarshal([]byte(`<sitemapindex><sitemap><loc>https://x.com/sm1.xml</loc></sitemap></sitemapindex>`), &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Sitemaps) != 1 || idx.Sitemaps[0].Loc != "https://x.com/sm1.xml" {
		t.Errorf("sitemapindex decode salah: %+v", idx.Sitemaps)
	}
}

func TestChunkText(t *testing.T) {
	if ChunkText("   ") != nil {
		t.Error("teks kosong harus nil")
	}
	if got := ChunkText("halo dunia"); len(got) != 1 || got[0] != "halo dunia" {
		t.Errorf("teks pendek harus 1 chunk utuh, dapat %v", got)
	}
	long := strings.Repeat("a", chunkSize*2+50)
	got := ChunkText(long)
	if len(got) < 2 {
		t.Fatalf("teks panjang harus terpecah >1 chunk, dapat %d", len(got))
	}
	// Tiap chunk tidak melebihi ukuran maksimum.
	for i, c := range got {
		if len([]rune(c)) > chunkSize {
			t.Errorf("chunk %d melebihi %d rune (%d)", i, chunkSize, len([]rune(c)))
		}
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"https://x.com/a/?q=1#frag": "https://x.com/a",
		"https://x.com/a/":          "https://x.com/a",
		"http://x.com":              "http://x.com",
		"ftp://x.com/file":          "", // skema tidak didukung
		"/relatif":                  "", // tanpa host
	}
	for in, want := range cases {
		if got := normalizeURL(in); got != want {
			t.Errorf("normalizeURL(%q) = %q, mau %q", in, got, want)
		}
	}
}

func TestCanonicalHost(t *testing.T) {
	for in, want := range map[string]string{
		"www.Toko.com":   "toko.com",
		"toko.com:8080":  "toko.com",
		"WWW.toko.co.id": "toko.co.id",
	} {
		if got := canonicalHost(in); got != want {
			t.Errorf("canonicalHost(%q) = %q, mau %q", in, got, want)
		}
	}
}

func TestExtractTitleText(t *testing.T) {
	doc, _ := html.Parse(strings.NewReader(`
		<html><head><title>Toko Kaos</title><style>.x{color:red}</style></head>
		<body><h1>Promo</h1><p>Diskon 20%</p><script>alert('x')</script></body></html>`))
	title, text := extractTitleText(doc)
	if title != "Toko Kaos" {
		t.Errorf("title = %q, mau \"Toko Kaos\"", title)
	}
	if !strings.Contains(text, "Promo") || !strings.Contains(text, "Diskon 20%") {
		t.Errorf("text harus berisi konten body, dapat %q", text)
	}
	if strings.Contains(text, "alert") || strings.Contains(text, "color:red") {
		t.Errorf("text tidak boleh memuat isi script/style, dapat %q", text)
	}
}

func TestExtractLinks(t *testing.T) {
	base, _ := url.Parse("https://toko.com/produk")
	doc, _ := html.Parse(strings.NewReader(
		`<a href="/kontak">k</a><a href="https://toko.com/about">a</a><a href="https://lain.com/x">x</a>`))
	links := extractLinks(doc, base)
	joined := strings.Join(links, " ")
	if !strings.Contains(joined, "https://toko.com/kontak") {
		t.Errorf("link relatif harus di-resolve absolut, dapat %v", links)
	}
	if !strings.Contains(joined, "https://toko.com/about") {
		t.Errorf("link absolut hilang, dapat %v", links)
	}
}

func TestParseRobots(t *testing.T) {
	body := "User-agent: *\nDisallow: /admin\nDisallow: /cart\n\nUser-agent: Googlebot\nDisallow: /\n"
	d := parseRobots(body)
	if len(d) != 2 || d[0] != "/admin" || d[1] != "/cart" {
		t.Fatalf("disallow * = %v, mau [/admin /cart]", d)
	}
	if !pathDisallowed("https://x.com/admin/login", d) {
		t.Error("/admin/login harus disallowed")
	}
	if pathDisallowed("https://x.com/produk", d) {
		t.Error("/produk tidak boleh disallowed")
	}
}
