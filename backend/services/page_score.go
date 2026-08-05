package services

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Ambang rekomendasi: skor 0–100. Di atas ini halaman auto-centang untuk dilatih.
const (
	recommendScoreMin     = 42
	recommendScoreStrong  = 65 // "sangat direkomendasikan"
	minUsefulContentRunes = 120
)

// PageTrainScore = hasil penilaian multi-sinyal kelayakan halaman untuk knowledge CS.
type PageTrainScore struct {
	Score       int      // 0–100
	Recommended bool     // true bila layak auto-centang
	Tier        string   // skip | weak | good | strong
	Reasons     []string // alasan singkat (untuk UI)
}

// ScorePageForCSTraining menilai URL+judul+teks bersih untuk training knowledge CS.
// Algoritma multi-sinyal (bukan AI): panjang, densitas topik CS, harga/kontak/lokasi,
// kualitas path/title, penalti privacy/hub/listing/tipis/navigasi.
func ScorePageForCSTraining(pageURL, title, content string) PageTrainScore {
	content = strings.TrimSpace(content)
	title = strings.TrimSpace(title)
	runes := []rune(content)
	n := len(runes)

	// Hard skip: gagal fetch / kosong
	if n < 40 {
		return PageTrainScore{Score: 0, Recommended: false, Tier: "skip", Reasons: []string{"Konten terlalu sedikit"}}
	}

	// Hard skip low-value legal pages (kecuali ada sinyal CS kuat — privacy jarang)
	if isLowValueURL(pageURL) && !hasStrongCSContactSignal(content) {
		return PageTrainScore{Score: 8, Recommended: false, Tier: "skip", Reasons: []string{"Halaman legal/privasi/syarat"}}
	}

	score := 0.0
	var reasons []string
	add := func(pts float64, reason string) {
		if pts == 0 {
			return
		}
		score += pts
		if reason != "" && pts > 0 {
			reasons = append(reasons, reason)
		}
	}
	penalize := func(pts float64, reason string) {
		if pts <= 0 {
			return
		}
		score -= pts
		if reason != "" {
			reasons = append(reasons, reason)
		}
	}

	// ── 1. Panjang konten (0–22) — log-ish: 200→8, 800→14, 2000→18, 5000+→22
	switch {
	case n < minUsefulContentRunes:
		penalize(15, "Konten tipis")
	case n < 200:
		add(6, "")
	case n < 500:
		add(12, "Konten cukup")
	case n < 1500:
		add(17, "Konten memadai")
	case n < 4000:
		add(20, "Konten kaya")
	default:
		add(22, "Konten sangat kaya")
	}

	lowerContent := strings.ToLower(content)
	lowerTitle := strings.ToLower(title)
	lowerURL := strings.ToLower(pageURL)

	// ── 2. Sinyal topik CS di konten (0–28)
	csHits := countKeywordHits(lowerContent, csTopicKeywords)
	switch {
	case csHits >= 12:
		add(28, "Banyak topik CS (harga/order/kontak/dll.)")
	case csHits >= 7:
		add(22, "Topik CS jelas")
	case csHits >= 4:
		add(15, "Beberapa topik CS")
	case csHits >= 2:
		add(9, "Sedikit sinyal CS")
	case csHits == 1:
		add(4, "")
	}

	// ── 3. Pola fakta operasional (0–20)
	if pricePattern.MatchString(content) {
		add(8, "Ada indikasi harga")
	}
	if phoneOrWAPattern.MatchString(content) {
		add(5, "Ada kontak/telepon/WA")
	}
	if hoursPattern.MatchString(lowerContent) {
		add(4, "Ada jam operasional")
	}
	if addressPattern.MatchString(lowerContent) {
		add(3, "Ada alamat/lokasi")
	}

	// ── 4. Judul (0–12)
	titleHits := countKeywordHits(lowerTitle, csTitleKeywords)
	if titleHits >= 2 {
		add(12, "Judul relevan CS")
	} else if titleHits == 1 {
		add(7, "Judul cukup relevan")
	} else if len([]rune(title)) >= 8 && !isGenericTitle(lowerTitle) {
		add(3, "")
	}

	// ── 5. Path URL (0–14 / penalti)
	pathScore, pathReason, pathPenalty, pathPenaltyReason := scoreURLPath(lowerURL)
	if pathScore > 0 {
		add(pathScore, pathReason)
	}
	if pathPenalty > 0 {
		penalize(pathPenalty, pathPenaltyReason)
	}

	// ── 6. Listing hub / archive
	if isListingHubURL(pageURL) {
		// Hub murni: penalti besar kecuali konten sangat kaya produk
		if csHits >= 6 && n >= 800 {
			penalize(6, "Halaman hub, tapi konten cukup")
		} else {
			penalize(18, "Halaman indeks/katalog hub")
		}
	}

	// ── 7. Densitas navigasi / boilerplate (penalti)
	navRatio := navigationNoiseRatio(lowerContent)
	if navRatio > 0.35 {
		penalize(12, "Banyak teks navigasi/boilerplate")
	} else if navRatio > 0.22 {
		penalize(6, "Cukup banyak navigasi")
	}

	// ── 8. Kekayaan leksikal (unique tokens bermakna) 0–8
	uniq := uniqueContentTokens(content)
	switch {
	case uniq >= 80:
		add(8, "Kosakata beragam")
	case uniq >= 40:
		add(5, "")
	case uniq >= 20:
		add(3, "")
	case uniq < 12 && n > 200:
		penalize(5, "Teks repetitif")
	}

	// ── 9. Struktur mirip FAQ (0–6)
	if faqStructureScore(content) >= 2 {
		add(6, "Struktur tanya-jawab")
	} else if faqStructureScore(content) == 1 {
		add(3, "")
	}

	// ── 10. Home page boost ringan
	if isHomeURL(pageURL) && n >= 200 {
		add(5, "Beranda")
	}

	// Clamp 0–100
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	s := int(score + 0.5)

	tier := "weak"
	recommended := s >= recommendScoreMin && n >= minUsefulContentRunes && !isHardSkipRecommended(pageURL, n)
	switch {
	case !recommended && s < 25:
		tier = "skip"
	case !recommended:
		tier = "weak"
	case s >= recommendScoreStrong:
		tier = "strong"
	default:
		tier = "good"
	}

	// Batasi alasan (prioritas: yang paling informatif)
	reasons = trimReasons(reasons, 4)
	if len(reasons) == 0 {
		if recommended {
			reasons = []string{"Cukup relevan untuk knowledge CS"}
		} else {
			reasons = []string{"Kurang sinyal info pelanggan"}
		}
	}

	return PageTrainScore{Score: s, Recommended: recommended, Tier: tier, Reasons: reasons}
}

func isHardSkipRecommended(pageURL string, n int) bool {
	if n < minUsefulContentRunes {
		return true
	}
	if isLowValueURL(pageURL) {
		return true
	}
	return false
}

var csTopicKeywords = []string{
	// produk & harga
	"harga", "price", "biaya", "tarif", "diskon", "promo", "rp", "rupiah",
	"produk", "product", "layanan", "jasa", "paket", "varian", "stok", "ready",
	// order
	"order", "pesan", "pemesanan", "booking", "beli", "checkout", "cara order", "cara pesan",
	// bayar & kirim
	"bayar", "pembayaran", "transfer", "cod", "dp", "rekening", "payment",
	"ongkir", "pengiriman", "kirim", "ekspedisi", "jne", "jnt", "sicepat", "gosend", "grab",
	// operasional
	"jam buka", "jam operasional", "buka pukul", "tutup", "hari kerja", "senin", "minggu",
	"alamat", "lokasi", "maps", "cabang", "toko", "studio", "workshop",
	// kontak
	"whatsapp", "hubungi", "kontak", "telepon", "email", "cs ", "customer service",
	// kebijakan
	"garansi", "refund", "retur", "tukar", "kebijakan", "syarat order", "minimal order",
	"faq", "tanya jawab", "tentang kami", "about us", "visi", "misi",
}

var csTitleKeywords = []string{
	"harga", "price", "produk", "product", "layanan", "jasa", "paket",
	"kontak", "contact", "tentang", "about", "faq", "order", "pesan",
	"kirim", "ongkir", "lokasi", "alamat", "jam", "buka", "katalog",
	"menu", "shop", "store", "layanan kami", "cara order", "pembayaran",
}

func countKeywordHits(lower string, kws []string) int {
	hits := 0
	for _, kw := range kws {
		if strings.Contains(lower, kw) {
			hits++
		}
	}
	return hits
}

var (
	pricePattern     = regexp.MustCompile(`(?i)(?:rp\.?\s*[\d.,]+|[\d.,]+\s*(?:ribu|rb|juta|jt)\b|\$\s*[\d.,]+)`)
	phoneOrWAPattern = regexp.MustCompile(`(?i)(?:\+?62|08)\d{8,14}|whatsapp|wa\.me|hubungi\s*kami`)
	hoursPattern     = regexp.MustCompile(`(?i)(?:jam\s*(?:buka|operasional)|pukul\s*\d{1,2}|buka\s*(?:setiap|hari)|\d{1,2}[.:]\d{2}\s*[-–]\s*\d{1,2})`)
	addressPattern   = regexp.MustCompile(`(?i)(?:jl\.|jalan\s|alamat\s*:|lokasi\s*:|google\s*maps|kec\.|kab\.|jakarta|bandung|surabaya|medan|semarang|yogyakarta|bali)`)
)

func hasStrongCSContactSignal(content string) bool {
	return phoneOrWAPattern.MatchString(content) || pricePattern.MatchString(content)
}

func scoreURLPath(lowerURL string) (boost float64, boostReason string, penalty float64, penaltyReason string) {
	u, err := url.Parse(lowerURL)
	if err != nil {
		return 0, "", 0, ""
	}
	path := u.Path
	// Positive path segments
	positives := []struct {
		kw  string
		pts float64
		why string
	}{
		{"/produk", 10, "URL produk"},
		{"/product", 10, "URL produk"},
		{"/harga", 10, "URL harga"},
		{"/price", 8, "URL harga"},
		{"/layanan", 9, "URL layanan"},
		{"/service", 8, "URL layanan"},
		{"/kontak", 9, "URL kontak"},
		{"/contact", 9, "URL kontak"},
		{"/tentang", 7, "URL tentang"},
		{"/about", 7, "URL tentang"},
		{"/faq", 10, "URL FAQ"},
		{"/cara-order", 10, "URL cara order"},
		{"/cara-pesan", 10, "URL cara pesan"},
		{"/pengiriman", 8, "URL pengiriman"},
		{"/ongkir", 9, "URL ongkir"},
		{"/paket", 7, "URL paket"},
		{"/katalog", 7, "URL katalog"},
		{"/menu", 6, "URL menu"},
		{"/lokasi", 7, "URL lokasi"},
		{"/jam-", 5, ""},
	}
	for _, p := range positives {
		if strings.Contains(path, p.kw) {
			if p.pts > boost {
				boost, boostReason = p.pts, p.why
			}
		}
	}
	// Negative
	negatives := []struct {
		kw  string
		pts float64
		why string
	}{
		{"/privacy", 20, "Halaman privasi"},
		{"/privasi", 20, "Halaman privasi"},
		{"/terms", 18, "Syarat & ketentuan"},
		{"/syarat", 16, "Syarat"},
		{"/disclaimer", 16, "Disclaimer"},
		{"/author/", 14, "Halaman author"},
		{"/tag/", 10, "Tag arsip"},
		{"/page/", 8, "Paginasi"},
		{"/cart", 20, "Keranjang"},
		{"/checkout", 20, "Checkout"},
		{"/login", 20, "Login"},
		{"/wp-admin", 25, "Admin"},
		{"/feed", 15, "Feed"},
	}
	for _, n := range negatives {
		if strings.Contains(path, n.kw) {
			if n.pts > penalty {
				penalty, penaltyReason = n.pts, n.why
			}
		}
	}
	return boost, boostReason, penalty, penaltyReason
}

func isGenericTitle(lower string) bool {
	generics := []string{"home", "beranda", "untitled", "page", "halaman", "index", "welcome"}
	t := strings.TrimSpace(lower)
	for _, g := range generics {
		if t == g {
			return true
		}
	}
	return false
}

func isHomeURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	p := strings.Trim(u.Path, "/")
	return p == "" || p == "index.html" || p == "index.php" || p == "home" || p == "beranda"
}

func navigationNoiseRatio(lower string) float64 {
	// Estimasi kasar: proporsi baris/frasa yang terlihat seperti nav
	navKW := []string{
		"home", "beranda", "menu", "login", "daftar", "keranjang", "wishlist",
		"copyright", "all rights reserved", "powered by", "follow us", "subscribe",
		"cookie", "privacy policy", "terms of service", "skip to content",
	}
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return 0
	}
	// hitung frekuensi token nav vs total (kasar)
	navCount := 0
	joined := lower
	for _, kw := range navKW {
		navCount += strings.Count(joined, kw)
	}
	// normalisasi kasar
	r := float64(navCount) / float64(len(fields)/8+1)
	if r > 1 {
		r = 1
	}
	return r
}

func uniqueContentTokens(content string) int {
	seen := map[string]bool{}
	for _, f := range strings.FieldsFunc(strings.ToLower(content), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len([]rune(f)) < 4 {
			continue
		}
		seen[f] = true
	}
	return len(seen)
}

func faqStructureScore(content string) int {
	// Deteksi pola Q/A atau "apa", "bagaimana", "berapa" berulang
	score := 0
	lower := strings.ToLower(content)
	qMarks := strings.Count(content, "?")
	if qMarks >= 3 {
		score++
	}
	if qMarks >= 6 {
		score++
	}
	qaStarters := 0
	for _, w := range []string{"apa ", "apakah ", "berapa ", "bagaimana ", "dimana ", "di mana ", "kapan ", "mengapa "} {
		qaStarters += strings.Count(lower, w)
	}
	if qaStarters >= 3 {
		score++
	}
	if score > 3 {
		score = 3
	}
	return score
}

func trimReasons(reasons []string, max int) []string {
	if len(reasons) <= max {
		return reasons
	}
	// Prefer longer/more specific reasons (already roughly ordered by importance)
	return reasons[:max]
}

// RankAndSelectRecommended menyesuaikan flag Recommended dalam satu job:
// ambang absolut + promosi relatif bila terlalu sedikit + cap top 30.
// Input diurutkan tidak wajib; output panjang sama dengan input (index sejajar).
func RankAndSelectRecommended(scores []PageTrainScore) []PageTrainScore {
	out := make([]PageTrainScore, len(scores))
	copy(out, scores)
	if len(out) == 0 {
		return out
	}

	type pair struct {
		idx, score int
		rec        bool
		tier       string
	}
	arr := make([]pair, len(out))
	for i, sc := range out {
		arr[i] = pair{i, sc.Score, sc.Recommended, sc.Tier}
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].score > arr[j].score })

	recommendedCount := 0
	for _, p := range arr {
		if out[p.idx].Recommended {
			recommendedCount++
		}
	}
	// Promosi relatif: situs tipis sering punya skor 35–41
	if recommendedCount < 3 {
		for _, p := range arr {
			if recommendedCount >= 8 {
				break
			}
			sc := out[p.idx]
			if sc.Score >= 35 && sc.Tier != "skip" && !sc.Recommended {
				sc.Recommended = true
				if sc.Tier == "weak" || sc.Tier == "" {
					sc.Tier = "good"
				}
				sc.Reasons = append([]string{"Dipromosikan: skor relatif terbaik di situs"}, sc.Reasons...)
				if len(sc.Reasons) > 4 {
					sc.Reasons = sc.Reasons[:4]
				}
				out[p.idx] = sc
				recommendedCount++
			}
		}
	}
	// Cap top 30 recommended by score
	var recIdx []int
	for _, p := range arr {
		if out[p.idx].Recommended {
			recIdx = append(recIdx, p.idx)
		}
	}
	// arr already sorted by score; rebuild recommended order
	recIdx = recIdx[:0]
	for _, p := range arr {
		if out[p.idx].Recommended {
			recIdx = append(recIdx, p.idx)
		}
	}
	if len(recIdx) > 30 {
		for _, idx := range recIdx[30:] {
			sc := out[idx]
			sc.Recommended = false
			if sc.Tier == "good" || sc.Tier == "strong" {
				sc.Tier = "weak"
			}
			sc.Reasons = append(sc.Reasons, "Di luar top 30 rekomendasi")
			if len(sc.Reasons) > 4 {
				sc.Reasons = sc.Reasons[len(sc.Reasons)-4:]
			}
			out[idx] = sc
		}
	}
	return out
}
