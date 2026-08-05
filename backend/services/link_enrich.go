package services

import (
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// EnrichUserMessageForAI memperkaya pesan pelanggan dengan konteks dari link
// (terutama Google Maps): resolve short link, parse koordinat/nama tempat.
// Tidak mengganti teks asli; menambahkan blok [KONTEKS LINK ...] di akhir.
// Aman SSRF: hanya http(s), host publik, timeout & batas unduhan.
func EnrichUserMessageForAI(userMsg string) string {
	msg := strings.TrimSpace(userMsg)
	if msg == "" {
		return userMsg
	}
	urls := extractHTTPURLs(msg)
	if len(urls) == 0 {
		return userMsg
	}

	var blocks []string
	seen := map[string]bool{}
	for _, raw := range urls {
		if len(blocks) >= 2 { // maksimal 2 link per pesan (latency)
			break
		}
		key := strings.ToLower(strings.TrimSpace(raw))
		if seen[key] {
			continue
		}
		seen[key] = true

		if isMapsURL(raw) {
			if block := enrichMapsLink(raw); block != "" {
				blocks = append(blocks, block)
			}
			continue
		}
		// Link non-Maps: ambil judul halaman ringan (bukan full scrape).
		if block := enrichGenericLinkTitle(raw); block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(blocks) == 0 {
		return userMsg
	}
	return msg + "\n\n" + strings.Join(blocks, "\n\n")
}

var httpURLRe = regexp.MustCompile(`https?://[^\s<>"')\]]+`)

func extractHTTPURLs(text string) []string {
	matches := httpURLRe.FindAllString(text, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		m = strings.TrimRight(m, ".,;:!?）】」』\"'")
		if m != "" {
			out = append(out, m)
		}
	}
	return out
}

func isMapsURL(raw string) bool {
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "maps.app.goo.gl/") ||
		strings.Contains(lower, "goo.gl/maps/") ||
		strings.Contains(lower, "google.com/maps") ||
		strings.Contains(lower, "maps.google.") ||
		strings.Contains(lower, "google.co.id/maps") ||
		strings.Contains(lower, "maps.app.goo.gl")
}

type mapsParseResult struct {
	FinalURL  string
	PlaceName string
	Latitude  float64
	Longitude float64
	HasCoord  bool
	Query     string
}

func enrichMapsLink(raw string) string {
	resolved, err := resolvePublicURL(raw, 8)
	if err != nil {
		// Tetap beri petunjuk minimal meski resolve gagal.
		return "[KONTEKS LINK LOKASI]\n" +
			"URL: " + raw + "\n" +
			"Catatan: Link Google Maps terdeteksi tetapi belum bisa dibuka otomatis. " +
			"Anggap pelanggan sudah membagikan lokasi; jangan minta kirim pin ulang kecuali benar-benar ambigu."
	}
	parsed := parseMapsURL(resolved)
	if parsed.FinalURL == "" {
		parsed.FinalURL = resolved
	}
	// Coba juga parse URL asli (kadang short link tidak expand penuh di path).
	if !parsed.HasCoord && parsed.PlaceName == "" && parsed.Query == "" {
		parsed = parseMapsURL(raw)
		if parsed.FinalURL == "" {
			parsed.FinalURL = resolved
		}
	}

	var lines []string
	lines = append(lines, "[KONTEKS LINK LOKASI TERBACA]")
	lines = append(lines, "URL asli: "+raw)
	if parsed.FinalURL != "" && !strings.EqualFold(parsed.FinalURL, raw) {
		lines = append(lines, "URL final: "+parsed.FinalURL)
	}
	if parsed.PlaceName != "" {
		lines = append(lines, "Nama tempat (dari link): "+parsed.PlaceName)
	}
	if parsed.Query != "" && !strings.EqualFold(parsed.Query, parsed.PlaceName) {
		lines = append(lines, "Query lokasi: "+parsed.Query)
	}
	if parsed.HasCoord {
		lines = append(lines, fmt.Sprintf("Koordinat: %.7f, %.7f", parsed.Latitude, parsed.Longitude))
		lines = append(lines, fmt.Sprintf("Google Maps: https://maps.google.com/?q=%.7f,%.7f", parsed.Latitude, parsed.Longitude))
	}
	lines = append(lines,
		"Instruksi: Gunakan data di atas sebagai titik/alamat tujuan yang sudah diberikan pelanggan. "+
			"Jangan meminta mengirim atau mengonfirmasi lokasi lagi kecuali data jelas tidak cukup untuk layanan.")
	return strings.Join(lines, "\n")
}

func parseMapsURL(raw string) mapsParseResult {
	var out mapsParseResult
	u, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return out
	}
	out.FinalURL = u.String()

	// ?q=lat,lng atau ?q=alamat
	if q := strings.TrimSpace(u.Query().Get("q")); q != "" {
		if lat, lng, ok := parseLatLngPair(q); ok {
			out.Latitude, out.Longitude, out.HasCoord = lat, lng, true
		} else {
			out.Query = cleanPlaceName(q)
			if out.PlaceName == "" {
				out.PlaceName = out.Query
			}
		}
	}
	if q := strings.TrimSpace(u.Query().Get("query")); q != "" {
		if lat, lng, ok := parseLatLngPair(q); ok {
			out.Latitude, out.Longitude, out.HasCoord = lat, lng, true
		} else if out.Query == "" {
			out.Query = cleanPlaceName(q)
			if out.PlaceName == "" {
				out.PlaceName = out.Query
			}
		}
	}
	// destination= untuk link directions
	if d := strings.TrimSpace(u.Query().Get("destination")); d != "" {
		if lat, lng, ok := parseLatLngPair(d); ok {
			out.Latitude, out.Longitude, out.HasCoord = lat, lng, true
		} else if out.PlaceName == "" {
			out.PlaceName = cleanPlaceName(d)
		}
	}
	// ll= atau center=
	for _, key := range []string{"ll", "center"} {
		if v := strings.TrimSpace(u.Query().Get(key)); v != "" {
			if lat, lng, ok := parseLatLngPair(v); ok {
				out.Latitude, out.Longitude, out.HasCoord = lat, lng, true
			}
		}
	}

	path := u.Path
	// /place/Nama+Tempat/@lat,lng,17z
	if strings.Contains(path, "/place/") {
		rest := path[strings.Index(path, "/place/")+len("/place/"):]
		namePart := rest
		if i := strings.Index(rest, "/"); i >= 0 {
			namePart = rest[:i]
		}
		if name := cleanPlaceName(namePart); name != "" && out.PlaceName == "" {
			out.PlaceName = name
		}
		if lat, lng, ok := extractAtCoords(rest); ok {
			out.Latitude, out.Longitude, out.HasCoord = lat, lng, true
		}
	}
	// /@lat,lng,zoom
	if !out.HasCoord {
		if lat, lng, ok := extractAtCoords(path); ok {
			out.Latitude, out.Longitude, out.HasCoord = lat, lng, true
		}
	}
	// /search/Nama
	if strings.Contains(path, "/search/") {
		rest := path[strings.Index(path, "/search/")+len("/search/"):]
		namePart := rest
		if i := strings.Index(rest, "/"); i >= 0 {
			namePart = rest[:i]
		}
		if name := cleanPlaceName(namePart); name != "" && out.PlaceName == "" {
			out.PlaceName = name
		}
	}
	// !3dLAT!4dLNG pattern di data maps
	if !out.HasCoord {
		if lat, lng, ok := extractBangCoords(u.String()); ok {
			out.Latitude, out.Longitude, out.HasCoord = lat, lng, true
		}
	}
	return out
}

var atCoordRe = regexp.MustCompile(`@(-?\d+\.?\d*),(-?\d+\.?\d*)`)
var bangCoordRe = regexp.MustCompile(`!3d(-?\d+\.?\d*)!4d(-?\d+\.?\d*)`)

func extractAtCoords(s string) (float64, float64, bool) {
	m := atCoordRe.FindStringSubmatch(s)
	if len(m) != 3 {
		return 0, 0, false
	}
	return parseLatLngPair(m[1] + "," + m[2])
}

func extractBangCoords(s string) (float64, float64, bool) {
	m := bangCoordRe.FindStringSubmatch(s)
	if len(m) != 3 {
		return 0, 0, false
	}
	return parseLatLngPair(m[1] + "," + m[2])
}

func parseLatLngPair(s string) (float64, float64, bool) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	parts := strings.Split(s, ",")
	if len(parts) < 2 {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(parts[0], 64)
	lng, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, false
	}
	return lat, lng, true
}

func cleanPlaceName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if unesc, err := neturl.QueryUnescape(s); err == nil {
		s = unesc
	}
	s = strings.ReplaceAll(s, "+", " ")
	s = strings.Join(strings.Fields(s), " ")
	// Buang fragment aneh
	s = strings.Trim(s, "/-_")
	if len([]rune(s)) < 2 {
		return ""
	}
	// Jangan anggap koordinat sebagai nama
	if _, _, ok := parseLatLngPair(s); ok {
		return ""
	}
	return s
}

// resolvePublicURL mengikuti redirect (short link Maps) dengan proteksi SSRF.
func resolvePublicURL(raw string, maxRedirects int) (string, error) {
	if err := assertPublicHTTPURL(raw); err != nil {
		return "", err
	}
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("terlalu banyak redirect")
			}
			return assertPublicHTTPURL(req.URL.String())
		},
	}
	// HEAD dulu; beberapa short link hanya kooperatif di GET.
	req, err := http.NewRequest(http.MethodHead, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "SlaluDiskonBot/1.0 (+location-link; customer-service)")
	resp, err := client.Do(req)
	if err == nil {
		final := resp.Request.URL.String()
		resp.Body.Close()
		if final != "" && (resp.StatusCode < 400 || resp.StatusCode == http.StatusMethodNotAllowed) {
			// Beberapa host menolak HEAD → coba GET terbatas
			if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusForbidden {
				return final, nil
			}
		}
	}

	req, err = http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "SlaluDiskonBot/1.0 (+location-link; customer-service)")
	resp, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	// Buang body (kita hanya butuh URL final setelah redirect)
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.Request != nil && resp.Request.URL != nil {
		return resp.Request.URL.String(), nil
	}
	return raw, nil
}

func assertPublicHTTPURL(raw string) error {
	u, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return fmt.Errorf("URL tidak valid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("hanya http/https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("host kosong")
	}
	// Blok host lokal umum tanpa DNS
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") || strings.HasSuffix(lower, ".internal") {
		return fmt.Errorf("host internal")
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("host tidak dapat di-resolve")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("host mengarah ke jaringan internal")
		}
	}
	return nil
}

var htmlTitleRe = regexp.MustCompile(`(?is)<title[^>]*>\s*([^<]{1,200})\s*</title>`)

// enrichGenericLinkTitle membuka link non-Maps secara terbatas untuk mengambil <title>.
func enrichGenericLinkTitle(raw string) string {
	if err := assertPublicHTTPURL(raw); err != nil {
		return ""
	}
	client := &http.Client{
		Timeout: 6 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("terlalu banyak redirect")
			}
			return assertPublicHTTPURL(req.URL.String())
		},
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "SlaluDiskonBot/1.0 (+link-preview; customer-service)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return ""
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "text/plain") {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 48<<10))
	if err != nil || len(body) == 0 {
		return ""
	}
	title := ""
	if m := htmlTitleRe.FindSubmatch(body); len(m) == 2 {
		title = cleanHTMLText(string(m[1]))
	}
	final := raw
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL.String()
	}
	if title == "" {
		return ""
	}
	var lines []string
	lines = append(lines, "[KONTEKS LINK TERBACA]")
	lines = append(lines, "URL: "+raw)
	if final != raw {
		lines = append(lines, "URL final: "+final)
	}
	lines = append(lines, "Judul halaman: "+title)
	lines = append(lines, "Instruksi: Gunakan judul/halaman di atas sebagai konteks tambahan. Jangan mengarang isi lengkap halaman yang tidak terbaca.")
	return strings.Join(lines, "\n")
}

func cleanHTMLText(s string) string {
	s = strings.TrimSpace(s)
	repl := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'",
		"\n", " ", "\r", " ", "\t", " ",
	)
	s = repl.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	// Buang karakter kontrol
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	runes := []rune(s)
	if len(runes) > 160 {
		s = string(runes[:160]) + "…"
	}
	return s
}
