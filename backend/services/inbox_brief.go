package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"wa-assistant/backend/models"

	openai "github.com/sashabaranov/go-openai"
)

// ConversationBrief = ringkasan operasional untuk CS di inbox (bukan dikirim ke pelanggan).
type ConversationBrief struct {
	ContactHint   string   `json:"contact_hint"`   // nama/panggilan bila terdeteksi
	Intent        string   `json:"intent"`         // kebutuhan utama 1 kalimat
	Products      []string `json:"products"`       // produk/layanan disebut
	KeyFacts      []string `json:"key_facts"`      // fakta penting (alamat, ukuran, budget…)
	OpenItems     []string `json:"open_items"`     // yang masih perlu ditindaklanjuti
	RiskFlags     []string `json:"risk_flags"`     // refund, komplain, dll.
	Stage         string   `json:"stage"`          // new|info|interest|transaction|issue|done
	Summary       string   `json:"summary"`        // 2–4 kalimat padat
	Source        string   `json:"source"`         // heuristic|ai|hybrid
	MessageCount  int      `json:"message_count"`
	LastChatID    uint     `json:"last_chat_id"`
	UpdatedAt     string   `json:"updated_at"`
	NeedsHuman    bool     `json:"needs_human"`
	Stale         bool     `json:"stale"` // true bila cache ketinggalan vs chat terbaru
	Confidence    float64  `json:"confidence"`
}

type briefAIPayload struct {
	ContactHint string   `json:"contact_hint"`
	Intent      string   `json:"intent"`
	Products    []string `json:"products"`
	KeyFacts    []string `json:"key_facts"`
	OpenItems   []string `json:"open_items"`
	RiskFlags   []string `json:"risk_flags"`
	Stage       string   `json:"stage"`
	Summary     string   `json:"summary"`
}

// BuildConversationBrief menyusun brief akurat: ekstraksi heuristik + AI terstruktur + validasi grounding.
// force diabaikan di sini (cache diputus di handler); signature tetap untuk call-site yang eksplisit.
func BuildConversationBrief(_ uint, _ string, msgs []models.ChatHistory, memory string, needsHuman bool, _ bool) (ConversationBrief, error) {
	// msgs: kronologis lama→baru
	lastID := uint(0)
	if len(msgs) > 0 {
		lastID = msgs[len(msgs)-1].ID
	}
	transcript := buildBriefTranscript(msgs)
	// Grounding memakai transkrip + memori jangka panjang (fakta lama yang sudah diringkas).
	groundSrc := transcript
	if strings.TrimSpace(memory) != "" {
		groundSrc = transcript + "\n" + memory
	}
	heuristic := extractBriefHeuristic(transcript, memory, needsHuman)
	heuristic.MessageCount = countBriefTurns(msgs)
	heuristic.LastChatID = lastID
	heuristic.NeedsHuman = needsHuman
	heuristic.UpdatedAt = time.Now().Format(time.RFC3339)

	if len(msgs) < 2 && strings.TrimSpace(memory) == "" {
		heuristic.Source = "heuristic"
		heuristic.Summary = "Percakapan masih singkat. Belum cukup konteks untuk ringkasan lengkap."
		heuristic.Confidence = 0.35
		return heuristic, nil
	}

	// AI brief (structured)
	aiPart, err := generateAIBrief(memory, transcript)
	if err != nil {
		// Fallback murni heuristik
		heuristic.Source = "heuristic"
		heuristic.Confidence = briefConfidence(heuristic, groundSrc)
		if heuristic.Confidence < 0.45 {
			heuristic.Confidence = 0.45
		}
		if heuristic.Summary == "" {
			heuristic.Summary = joinNonEmpty(" · ", heuristic.Intent, strings.Join(heuristic.OpenItems, "; "))
		}
		return heuristic, nil
	}

	merged := mergeBriefs(heuristic, aiPart, groundSrc)
	merged.MessageCount = heuristic.MessageCount
	merged.LastChatID = lastID
	merged.NeedsHuman = needsHuman
	merged.UpdatedAt = time.Now().Format(time.RFC3339)
	merged.Source = "hybrid"
	merged.Confidence = briefConfidence(merged, groundSrc)
	return merged, nil
}

func buildBriefTranscript(msgs []models.ChatHistory) string {
	var sb strings.Builder
	// Ambil paling banyak 50 turn terakhir untuk token
	start := 0
	if len(msgs) > 50 {
		start = len(msgs) - 50
	}
	for _, m := range msgs[start:] {
		if m.Message != "" {
			sb.WriteString("Pelanggan: ")
			sb.WriteString(strings.TrimSpace(m.Message))
			sb.WriteByte('\n')
		}
		if m.Reply != "" {
			who := "CS"
			if m.FromHuman {
				who = "CS-manusia"
			}
			sb.WriteString(who)
			sb.WriteString(": ")
			sb.WriteString(strings.TrimSpace(m.Reply))
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func countBriefTurns(msgs []models.ChatHistory) int {
	n := 0
	for _, m := range msgs {
		if strings.TrimSpace(m.Message) != "" || strings.TrimSpace(m.Reply) != "" {
			n++
		}
	}
	return n
}

var (
	briefPhoneRe   = regexp.MustCompile(`(?i)(?:\+?62|08)\d{8,14}`)
	briefPriceRe   = regexp.MustCompile(`(?i)(?:rp\.?\s*[\d.]+(?:\s*(?:ribu|rb|juta|jt))?|[\d.]+\s*(?:ribu|rb|juta|jt)\b)`)
	briefEmailRe   = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	briefNameRe    = regexp.MustCompile(`(?i)(?:nama\s*(?:saya|aku|:)\s*|saya\s+)([A-Za-zÀ-ÿ][A-Za-zÀ-ÿ\s'.]{1,40})`)
	briefAddrRe    = regexp.MustCompile(`(?i)(?:alamat\s*:?\s*|kirim ke\s+|domisili\s+)([^\n]{8,120})`)
	briefSizeRe    = regexp.MustCompile(`(?i)\b(size|ukuran)\s*[:\s]?\s*(xs|s|m|l|xl|xxl|xxxl|\d{1,3})\b`)
	briefIntentRe  = regexp.MustCompile(`(?i)\b(mau\s+(?:order|pesan|beli|booking)|langsung\s+(?:order|pesan)|refund|komplain|batal|cek\s+ongkir|tanya\s+harga|status\s+(?:pesanan|order)|lacak|resi)\b`)
	briefOrderRe   = regexp.MustCompile(`(?i)\b(?:order|invoice|inv|pesanan|trx|resi)[\s#:.-]*([A-Z0-9\-]{5,24})\b`)
	briefQtyRe     = regexp.MustCompile(`(?i)\b(\d{1,3})\s*(pcs|buah|unit|botol|paket|lembar)\b`)
	briefCourierRe = regexp.MustCompile(`(?i)\b(jne|j&t|jnt|sicepat|anteraja|ninja|pos indonesia|gosend|grab express)\b`)
	// Tanggal: wajib hari+bulan (angka/slash lengkap ATAU nama bulan). Hindari match "10-20" dari rentang harga.
	briefDateRe = regexp.MustCompile(`(?i)\b(\d{1,2}[/\-.]\d{1,2}[/\-.]\d{2,4}|\d{1,2}\s+(?:jan(?:uari)?|feb(?:ruari)?|mar(?:et)?|apr(?:il)?|mei|jun(?:i)?|jul(?:i)?|agu(?:stus)?|sep(?:tember)?|okt(?:ober)?|nov(?:ember)?|des(?:ember)?)\s+\d{2,4})\b`)
	// Jam: hanya dengan kata "jam" agar "10.000" / "100.000" tidak jadi "Jam disebut".
	briefTimeRe    = regexp.MustCompile(`(?i)\bjam\s*([01]?\d|2[0-3])[.:]([0-5]\d)\b`)
	briefPaymentRe = regexp.MustCompile(`(?i)\b(transfer|bca|bni|bri|mandiri|gopay|ovo|dana|qris|cod|dp|pelunasan)\b`)
)

func extractBriefHeuristic(transcript, memory string, needsHuman bool) ConversationBrief {
	text := transcript + "\n" + memory
	lower := strings.ToLower(text)
	b := ConversationBrief{Source: "heuristic", Stage: "info", KeyFacts: nil, Products: nil, OpenItems: nil, RiskFlags: nil}

	if m := briefNameRe.FindStringSubmatch(transcript); len(m) > 1 {
		b.ContactHint = cleanBriefName(strings.TrimSpace(m[1]))
		// potong di kata sapaan
		for _, stop := range []string{" kak", " ya", " mau", " dari", " yang", ","} {
			if i := strings.Index(strings.ToLower(b.ContactHint), stop); i > 1 {
				b.ContactHint = strings.TrimSpace(b.ContactHint[:i])
			}
		}
		if len([]rune(b.ContactHint)) < 2 {
			b.ContactHint = ""
		}
	}

	// Entitas factual (multi-signal)
	for _, p := range briefPriceRe.FindAllString(text, 5) {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Harga/biaya disebut: "+strings.TrimSpace(p))
	}
	for _, p := range briefPhoneRe.FindAllString(text, 3) {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Nomor: "+p)
	}
	for _, e := range briefEmailRe.FindAllString(text, 2) {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Email: "+e)
	}
	if m := briefAddrRe.FindStringSubmatch(transcript); len(m) > 1 {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Alamat/kirim: "+strings.TrimSpace(m[1]))
	}
	if m := briefSizeRe.FindStringSubmatch(lower); len(m) > 2 {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Ukuran: "+strings.ToUpper(m[2]))
	}
	for _, m := range briefOrderRe.FindAllStringSubmatch(text, 3) {
		if len(m) > 1 {
			b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Referensi order/resi: "+strings.TrimSpace(m[1]))
		}
	}
	for _, m := range briefQtyRe.FindAllStringSubmatch(lower, 3) {
		if len(m) > 2 {
			b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Qty: "+m[1]+" "+m[2])
		}
	}
	for _, c := range briefCourierRe.FindAllString(lower, 2) {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Kurir: "+strings.ToUpper(c))
	}
	for _, d := range briefDateRe.FindAllString(text, 3) {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Tanggal disebut: "+strings.TrimSpace(d))
	}
	for _, tm := range briefTimeRe.FindAllString(text, 2) {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Jam disebut: "+strings.TrimSpace(tm))
	}
	for _, pay := range briefPaymentRe.FindAllString(lower, 3) {
		b.KeyFacts = appendUniqueBrief(b.KeyFacts, "Pembayaran: "+pay)
	}

	// Produk: katalog kata + pola "mau X" / "beli X"
	for _, kw := range []string{"kaos", "celana", "sepatu", "jaket", "tas", "topi", "hoodie", "kemeja",
		"paket", "layanan", "jasa", "kursus", "konsultasi", "booking", "sewa", "produk", "membership"} {
		if strings.Contains(lower, kw) {
			b.Products = appendUniqueBrief(b.Products, kw)
		}
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:mau|beli|pesan|order|booking)\s+([a-z0-9][\w\-]{2,24})`),
		regexp.MustCompile(`(?i)produk\s+([a-z0-9][\w\-]{2,24})`),
	} {
		for _, m := range re.FindAllStringSubmatch(lower, 4) {
			if len(m) > 1 {
				tok := strings.TrimSpace(m[1])
				if tok != "order" && tok != "pesan" && tok != "dong" && tok != "ya" && tok != "kak" {
					b.Products = appendUniqueBrief(b.Products, tok)
				}
			}
		}
	}

	// Intent + stage (prioritas risiko > transaksi > tracking > minat)
	switch {
	case strings.Contains(lower, "refund") || strings.Contains(lower, "komplain") || strings.Contains(lower, "salah transfer") || strings.Contains(lower, "penipuan"):
		b.Intent = "Isu/keluhan atau permintaan penanganan khusus"
		b.Stage = "issue"
	case strings.Contains(lower, "status pesanan") || strings.Contains(lower, "status order") || strings.Contains(lower, "lacak") || strings.Contains(lower, "resi"):
		b.Intent = "Cek status pesanan/pengiriman"
		b.Stage = "transaction"
	case briefIntentRe.MatchString(lower) && (strings.Contains(lower, "order") || strings.Contains(lower, "pesan") || strings.Contains(lower, "beli") || strings.Contains(lower, "booking")):
		b.Intent = "Minat order/transaksi"
		b.Stage = "transaction"
	case strings.Contains(lower, "ongkir") || strings.Contains(lower, "pengiriman"):
		b.Intent = "Cek ongkir/pengiriman"
		b.Stage = "interest"
	case strings.Contains(lower, "harga") || strings.Contains(lower, "berapa"):
		b.Intent = "Tanya harga/info produk"
		b.Stage = "info"
	case strings.Contains(lower, "terima kasih") || strings.Contains(lower, "sudah beres") || strings.Contains(lower, "selesai"):
		b.Intent = "Percakapan mendekati selesai"
		b.Stage = "done"
	case strings.Contains(lower, "halo") || strings.Contains(lower, "hai") || strings.Contains(lower, "assalam"):
		b.Intent = "Sapaan / awal percakapan"
		b.Stage = "new"
	}

	if needsHuman {
		b.RiskFlags = appendUniqueBrief(b.RiskFlags, "Ditandai butuh penanganan CS (internal)")
		b.OpenItems = appendUniqueBrief(b.OpenItems, "Tindaklanjuti dari antrian Butuh CS")
		if b.Stage == "info" || b.Stage == "new" || b.Stage == "" {
			b.Stage = "issue"
		}
	}
	for _, risk := range []string{"refund", "salah transfer", "penipuan", "komplain", "batal pesanan", "data pribadi", "marah", "kecewa", "ganti rugi"} {
		if strings.Contains(lower, risk) {
			b.RiskFlags = appendUniqueBrief(b.RiskFlags, risk)
		}
	}

	// Open items: pertanyaan terakhir + pesan pelanggan belum dijawab di ujung transcript
	lines := strings.Split(transcript, "\n")
	var lastCustomer, lastAgent string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Pelanggan:") {
			lastCustomer = strings.TrimSpace(strings.TrimPrefix(line, "Pelanggan:"))
		} else if strings.HasPrefix(line, "CS") {
			lastAgent = strings.TrimSpace(line[strings.Index(line, ":")+1:])
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "Pelanggan:") && strings.Contains(line, "?") {
			q := strings.TrimSpace(strings.TrimPrefix(line, "Pelanggan:"))
			if len([]rune(q)) > 8 {
				b.OpenItems = appendUniqueBrief(b.OpenItems, "Pertanyaan terakhir: "+truncateRunesBrief(q, 120))
			}
			break
		}
	}
	// Jika ujung chat masih dari pelanggan (belum ada balasan setelahnya di urutan lastCustomer > empty lastAgent check via line order)
	if lastCustomer != "" {
		// Cari apakah baris terakhir non-kosong adalah pelanggan
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "Pelanggan:") {
				msg := strings.TrimSpace(strings.TrimPrefix(line, "Pelanggan:"))
				if len([]rune(msg)) > 6 {
					b.OpenItems = appendUniqueBrief(b.OpenItems, "Menunggu balasan CS: "+truncateRunesBrief(msg, 100))
				}
			}
			break
		}
	}
	_ = lastAgent

	if b.Intent == "" {
		b.Intent = "Percakapan umum / info"
	}
	// Batas facts/products
	if len(b.KeyFacts) > 10 {
		b.KeyFacts = b.KeyFacts[:10]
	}
	if len(b.Products) > 8 {
		b.Products = b.Products[:8]
	}
	parts := []string{b.Intent}
	if b.ContactHint != "" {
		parts = append(parts, "Kontak: "+b.ContactHint)
	}
	if len(b.Products) > 0 {
		parts = append(parts, "Produk: "+strings.Join(b.Products, ", "))
	}
	if len(b.OpenItems) > 0 {
		parts = append(parts, b.OpenItems[0])
	}
	b.Summary = strings.Join(parts, ". ")
	return b
}

func generateAIBrief(memory, transcript string) (briefAIPayload, error) {
	p := activePreset()
	if apiKeyForPreset(p) == "" {
		return briefAIPayload{}, fmt.Errorf("API key belum dikonfigurasi")
	}
	// Batasi transcript
	if r := []rune(transcript); len(r) > 10000 {
		transcript = string(r[len(r)-10000:])
	}
	if r := []rune(memory); len(r) > 2000 {
		memory = string(r[:2000])
	}
	sys := `Kamu asisten operasional CS. Buat RINGKASAN INTERNAL untuk petugas inbox (bukan balasan ke pelanggan).
Tulis ramah, jelas, mudah dibaca cepat di HP/desktop. Hanya dari MEMORI + TRANSKRIP. Jangan mengarang.
Output HANYA JSON objek:
{
  "contact_hint": "nama/panggilan jika ada, else empty",
  "intent": "1 kalimat: apa yang diinginkan pelanggan (bahasa sehari-hari)",
  "products": ["produk/layanan disebut"],
  "key_facts": ["fakta penting dalam frasa pendek: alamat, ukuran, budget, jadwal — max 8"],
  "open_items": ["tugas CS yang actionable, diawali kata kerja: Konfirmasi…, Cek…, Balas… — max 5"],
  "risk_flags": ["refund/komplain/penipuan/dll bila ada — label singkat"],
  "stage": "new|info|interest|transaction|issue|done",
  "summary": "2-3 kalimat padat: konteks + status + apa yang penting diingat"
}
Aturan ketat:
- angka/harga/jam/nomor hanya jika tertulis di sumber
- open_items = yang masih perlu dikerjakan, bukan riwayat
- jangan sebut "AI" atau "bot" di summary
- bahasa Indonesia natural, tanpa jargon teknis`

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	resp, err := clientForPreset(p).CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: "MEMORI:\n" + memory + "\n\nTRANSKRIP:\n" + transcript},
		},
		MaxTokens:   800,
		Temperature: 0.15,
	})
	if err != nil {
		return briefAIPayload{}, err
	}
	if len(resp.Choices) == 0 {
		return briefAIPayload{}, fmt.Errorf("respons kosong")
	}
	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	// Ambil objek JSON pertama
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}
	var out briefAIPayload
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return briefAIPayload{}, err
	}
	return out, nil
}

func mergeBriefs(h ConversationBrief, ai briefAIPayload, transcript string) ConversationBrief {
	srcTokens := contentTokenSet(transcript)
	srcNumbers := normalizedFactNumbers(transcript)

	out := h
	if strings.TrimSpace(ai.ContactHint) != "" {
		out.ContactHint = strings.TrimSpace(ai.ContactHint)
	}
	if strings.TrimSpace(ai.Intent) != "" {
		out.Intent = strings.TrimSpace(ai.Intent)
	}
	if st := strings.ToLower(strings.TrimSpace(ai.Stage)); st != "" {
		out.Stage = st
	}
	if s := strings.TrimSpace(ai.Summary); s != "" {
		out.Summary = s
	}

	// Products: union, max 8
	for _, p := range ai.Products {
		p = strings.TrimSpace(p)
		if p != "" {
			out.Products = appendUniqueBrief(out.Products, p)
		}
	}
	if len(out.Products) > 8 {
		out.Products = out.Products[:8]
	}

	// Key facts: AI first (grounded), lalu heuristic yang tidak redundant
	var facts []string
	for _, f := range ai.KeyFacts {
		f = cleanBriefFact(f)
		if f == "" {
			continue
		}
		if !factGrounded(f, srcNumbers, srcTokens) {
			continue
		}
		facts = appendUniqueBrief(facts, f)
	}
	for _, f := range h.KeyFacts {
		f = cleanBriefFact(f)
		if f == "" || factRedundant(f, facts) {
			continue
		}
		facts = appendUniqueBrief(facts, f)
	}
	if len(facts) > 10 {
		facts = facts[:10]
	}
	out.KeyFacts = facts

	var opens []string
	for _, o := range ai.OpenItems {
		o = strings.TrimSpace(o)
		if o != "" {
			opens = appendUniqueBrief(opens, o)
		}
	}
	for _, o := range h.OpenItems {
		opens = appendUniqueBrief(opens, o)
	}
	if len(opens) > 6 {
		opens = opens[:6]
	}
	out.OpenItems = opens

	var risks []string
	for _, r := range ai.RiskFlags {
		r = strings.TrimSpace(r)
		if r != "" {
			risks = appendUniqueBrief(risks, r)
		}
	}
	for _, r := range h.RiskFlags {
		risks = appendUniqueBrief(risks, r)
	}
	out.RiskFlags = risks
	return out
}

func factGrounded(fact string, srcNumbers map[string]bool, srcTokens map[string]bool) bool {
	for n := range normalizedFactNumbers(fact) {
		if !srcNumbers[n] {
			return false
		}
	}
	// Fakta panjang harus ada overlap token
	if len([]rune(fact)) > 25 && tokenOverlapRatio(fact, srcTokens) < 0.1 {
		return false
	}
	return true
}

func cleanBriefFact(f string) string {
	f = strings.TrimSpace(f)
	f = strings.TrimPrefix(f, "•")
	f = strings.TrimPrefix(f, "-")
	f = strings.TrimPrefix(f, "*")
	f = strings.TrimSpace(f)
	// Buang prefix label yang bikin list berisik
	for _, p := range []string{
		"Harga/biaya disebut:", "Harga disebut:", "Tanggal disebut:", "Jam disebut:",
		"Pembayaran:", "Kurir:", "Qty:", "Nomor:", "Email:", "Alamat/kirim:", "Ukuran:",
		"Referensi order/resi:",
	} {
		if strings.HasPrefix(strings.ToLower(f), strings.ToLower(p)) {
			rest := strings.TrimSpace(f[len(p):])
			if rest != "" {
				// Label pendek yang jelas untuk CS
				switch {
				case strings.HasPrefix(p, "Harga"):
					f = "Harga " + rest
				case strings.HasPrefix(p, "Tanggal"):
					f = "Tanggal " + rest
				case strings.HasPrefix(p, "Jam"):
					f = "Jam " + rest
				case strings.HasPrefix(p, "Alamat"):
					f = "Alamat " + rest
				case strings.HasPrefix(p, "Referensi"):
					f = "Order/resi " + rest
				default:
					f = strings.TrimSuffix(p, ":") + " " + rest
				}
			}
			break
		}
	}
	return strings.TrimSpace(f)
}

// factRedundant: heuristic "Harga 1jt" tidak perlu jika AI sudah sebut 1jt.
func factRedundant(candidate string, existing []string) bool {
	cLow := strings.ToLower(candidate)
	cNums := normalizedFactNumbers(candidate)
	cToks := contentTokenSet(candidate)
	for _, e := range existing {
		eLow := strings.ToLower(e)
		if eLow == cLow || strings.Contains(eLow, cLow) || strings.Contains(cLow, eLow) {
			return true
		}
		// Overlap token tinggi + angka sama → redundant
		overlap := 0
		for t := range cToks {
			if contentTokenSet(e)[t] {
				overlap++
			}
		}
		if len(cToks) > 0 && float64(overlap)/float64(len(cToks)) >= 0.55 {
			sameNum := true
			eNums := normalizedFactNumbers(e)
			for n := range cNums {
				if !eNums[n] {
					sameNum = false
					break
				}
			}
			if sameNum && (len(cNums) > 0 || overlap >= 2) {
				return true
			}
		}
	}
	return false
}

func briefConfidence(b ConversationBrief, transcript string) float64 {
	c := 0.4
	if b.Intent != "" && b.Intent != "Percakapan umum / info" {
		c += 0.1
	}
	if len(b.KeyFacts) > 0 {
		c += 0.15
	}
	if len(b.OpenItems) > 0 {
		c += 0.1
	}
	if b.Source == "hybrid" {
		c += 0.15
	}
	if len([]rune(transcript)) > 400 {
		c += 0.1
	}
	if c > 0.95 {
		c = 0.95
	}
	return c
}

func appendUniqueBrief(list []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return list
	}
	low := strings.ToLower(item)
	for _, x := range list {
		if strings.ToLower(x) == low {
			return list
		}
	}
	return append(list, item)
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return strings.Join(out, sep)
}

func truncateRunesBrief(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// Encode/decode brief cache
func EncodeBrief(b ConversationBrief) string {
	raw, _ := json.Marshal(b)
	return string(raw)
}

func DecodeBrief(raw string) (ConversationBrief, bool) {
	var b ConversationBrief
	if strings.TrimSpace(raw) == "" {
		return b, false
	}
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return b, false
	}
	return b, true
}

// Ensure unused unicode import doesn't fail - use in name clean
func cleanBriefName(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsSpace(r) || r == '\'' || r == '.' {
			return r
		}
		return -1
	}, s)
}
