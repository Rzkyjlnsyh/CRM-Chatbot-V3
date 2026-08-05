package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	openai "github.com/sashabaranov/go-openai"
)

// QAPair = satu pasangan tanya-jawab FAQ hasil olahan AI dari konten web.
type QAPair struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
	Tags     string `json:"tags"`
}

var factualNumberPattern = regexp.MustCompile(`[0-9][0-9.,:/-]*`)
var scaledFactPattern = regexp.MustCompile(`(?i)([0-9]+)\s*(ribu|rb|k|juta|jt)\b`)

func normalizedFactNumbers(value string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range factualNumberPattern.FindAllString(value, -1) {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, raw)
		// Angka satu digit terlalu umum (jumlah item/list) untuk dijadikan validator.
		if len(digits) >= 2 {
			out[digits] = true
		}
	}
	for _, match := range scaledFactPattern.FindAllStringSubmatch(value, -1) {
		base, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			continue
		}
		multiplier := int64(1000)
		unit := strings.ToLower(match[2])
		if unit == "juta" || unit == "jt" {
			multiplier = 1_000_000
		}
		out[strconv.FormatInt(base*multiplier, 10)] = true
	}
	return out
}

// groundedFAQ membuang FAQ duplikat, jawaban dengan angka tidak ada di sumber,
// dan jawaban yang hampir tidak overlap token dengan sumber (parafrase liar).
func groundedFAQ(source string, items []QAPair) []QAPair {
	sourceNumbers := normalizedFactNumbers(source)
	sourceTokens := contentTokenSet(source)
	seenQuestion := map[string]bool{}
	seenAnswer := map[string]bool{}
	out := make([]QAPair, 0, len(items))
	for _, item := range items {
		item.Question = strings.TrimSpace(item.Question)
		item.Answer = strings.TrimSpace(item.Answer)
		item.Tags = strings.TrimSpace(item.Tags)
		if item.Question == "" || item.Answer == "" {
			continue
		}
		grounded := true
		for number := range normalizedFactNumbers(item.Answer) {
			if !sourceNumbers[number] {
				grounded = false
				break
			}
		}
		if !grounded {
			log.Printf("[faq] buang jawaban dengan angka yang tidak ada di sumber: %q", item.Question)
			continue
		}
		// Overlap token: jawaban harus menempel ke sumber (hindari fiksi non-numerik).
		if ov := tokenOverlapRatio(item.Answer, sourceTokens); ov < 0.12 && len([]rune(item.Answer)) > 40 {
			log.Printf("[faq] buang jawaban overlap rendah (%.2f) thd sumber: %q", ov, item.Question)
			continue
		}
		questionKey := strings.ToLower(strings.Join(strings.Fields(item.Question), " "))
		answerKey := strings.ToLower(strings.Join(strings.Fields(item.Answer), " "))
		if seenQuestion[questionKey] || seenAnswer[answerKey] {
			continue
		}
		seenQuestion[questionKey] = true
		seenAnswer[answerKey] = true
		out = append(out, item)
	}
	return out
}

func contentTokenSet(text string) map[string]bool {
	out := map[string]bool{}
	var b strings.Builder
	flush := func() {
		tok := b.String()
		b.Reset()
		if len([]rune(tok)) >= 3 {
			out[tok] = true
		}
	}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

func tokenOverlapRatio(answer string, sourceTokens map[string]bool) float64 {
	if len(sourceTokens) == 0 {
		return 1
	}
	ansTokens := contentTokenSet(answer)
	if len(ansTokens) == 0 {
		return 0
	}
	hit := 0
	for t := range ansTokens {
		if sourceTokens[t] {
			hit++
		}
	}
	return float64(hit) / float64(len(ansTokens))
}

const (
	webFAQMaxInputRunes     = 6000 // batas konten per halaman yang dikirim ke AI (kontrol token)
	webPersonaMaxInputRunes = 5000
)

const webFAQSystem = `Kamu ahli menyusun FAQ knowledge base customer service dari konten website bisnis.
Dari konten yang diberikan, ambil SEMUA informasi yang berguna untuk pelanggan: produk/layanan,
harga, promo, cara order, pembayaran, pengiriman/ongkir, jam operasional, lokasi, kontak, garansi,
kebijakan. BUANG menu navigasi, footer, copyright, teks hukum panjang, dan basa-basi marketing kosong.
Jangan mengarang—hanya dari teks yang diberikan. Tulis pertanyaan seperti cara pelanggan bertanya,
jawaban ringkas & faktual, bahasa Indonesia natural.
PENTING: Halaman produk/layanan yang memuat nama produk, HARGA, stok, atau cara pesan HAMPIR PASTI
punya info berguna—WAJIB diekstrak, jangan dilewati. Kembalikan array kosong [] HANYA bila konten
benar-benar cuma menu/navigasi tanpa satu pun fakta tentang produk, harga, layanan, atau kontak.
Berikan 2-5 tags singkat yang benar-benar mewakili topik agar pencarian knowledge akurat.
Output HANYA JSON array: [{"question":"...","answer":"...","tags":"harga,produk"}].`

// GenerateWebFAQ mengubah konten satu halaman web menjadi pasangan Q&A FAQ yang bersih.
// Mengembalikan slice kosong (bukan error) bila konten benar-benar tak mengandung info berguna.
// Mencoba hingga 2x karena model kadang flaky mengembalikan [] untuk konten yang sama.
func GenerateWebFAQ(title, content string) ([]QAPair, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil
	}
	if r := []rune(content); len(r) > webFAQMaxInputRunes {
		content = string(r[:webFAQMaxInputRunes])
	}
	p := activePreset()
	if apiKeyForPreset(p) == "" {
		return nil, fmt.Errorf("API key AI belum dikonfigurasi")
	}
	userMsg := "Judul halaman: " + title + "\n\nKonten:\n" + content

	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		resp, err := clientForPreset(p).CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: p.Model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: webFAQSystem},
				{Role: openai.ChatMessageRoleUser, Content: userMsg},
			},
			MaxTokens:   2500, // beri ruang ekstra: model reasoning makan token utk "mikir" dulu
			Temperature: 0.2,
		})
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		if len(resp.Choices) == 0 {
			continue
		}
		if qa := groundedFAQ(content, parseQAJSON(resp.Choices[0].Message.Content)); len(qa) > 0 {
			return qa, nil
		}
		if attempt == 1 {
			log.Printf("[faq] percobaan 1 kosong untuk %q (%d char) — retry", title, len([]rune(content)))
		}
	}
	return nil, lastErr // (nil,nil) = benar-benar kosong; (nil,err) = gangguan API yang harus ditampilkan ke user
}

// GenerateFAQFromText mengubah informasi bisnis bebas menjadi FAQ faktual. Dipakai oleh
// Tulis Info dan Setup Cepat agar keduanya memakai provider/runtime AI yang sama dengan chat.
func GenerateFAQFromText(source, audience string, count int) ([]QAPair, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("informasi sumber kosong")
	}
	if count < 1 {
		count = 10
	}
	if count > 30 {
		count = 30
	}
	if r := []rune(source); len(r) > 12000 {
		source = string(r[:12000])
	}
	p := activePreset()
	if apiKeyForPreset(p) == "" {
		return nil, fmt.Errorf("API key AI belum dikonfigurasi")
	}
	system := `Kamu ahli menyusun knowledge base customer service. Buat FAQ yang faktual dan mandiri:
- hanya gunakan fakta dari sumber, jangan menambah metode bayar, garansi, stok, promo, atau kebijakan yang tidak tertulis;
- pertanyaan harus natural seperti bahasa pelanggan dan tidak duplikat;
- jawaban langsung, lengkap konteksnya, dan tetap benar bila dibaca tanpa dokumen sumber;
- prioritaskan produk/layanan, harga, variasi, cara order, pembayaran, pengiriman, jam, lokasi, kebijakan, dan batasan;
- berikan 2-5 tags singkat untuk retrieval.
Output HANYA JSON array: [{"question":"...","answer":"...","tags":"produk,harga"}].`
	user := fmt.Sprintf("Buat maksimal %d FAQ untuk %s.\n\nSUMBER FAKTA:\n%s", count, audience, source)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		resp, err := clientForPreset(p).CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: p.Model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: system},
				{Role: openai.ChatMessageRoleUser, Content: user},
			},
			MaxTokens: 3000, Temperature: 0.15,
		})
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf("AI tidak mengembalikan respons")
			continue
		}
		if items := groundedFAQ(source, parseQAJSON(resp.Choices[0].Message.Content)); len(items) > 0 {
			if len(items) > count {
				items = items[:count]
			}
			return items, nil
		}
		lastErr = fmt.Errorf("respons AI tidak berisi FAQ JSON yang valid")
	}
	return nil, lastErr
}

// GenerateBusinessPersona menyusun persona dari profil/deskripsi bisnis dengan generator
// yang sama seperti website, sehingga batasan faktual dan format persona tetap konsisten.
func GenerateBusinessPersona(profile string) (string, error) {
	return GenerateWebPersona([]string{strings.TrimSpace(profile)})
}

const webPersonaSystem = `Kamu prompt engineer. Buat SYSTEM PROMPT persona SINGKAT untuk AI customer service
WhatsApp, berdasarkan konten website. Bahasa Indonesia, RINGKAS & UTUH — sekitar 6-10 kalimat,
MAKSIMAL ~1200 karakter, dan WAJIB selesaikan kalimat terakhir (jangan terpotong).
Cakup secara ringkas: (1) identitas — nama brand yang BENAR (bukan judul SEO panjang) & bidang usaha;
(2) JENIS produk/layanan secara umum (sebut kategorinya saja); (3) area/jam layanan & cara kontak bila ada;
(4) peran dan batasan layanan; (5) cara order; (6) hal yang TIDAK boleh dijanjikan (kirim
file/katalog/gambar lewat chat, atau harga/stok/detail yang tidak pasti).
PENTING: JANGAN menyalin daftar kode produk atau harga satu per satu. Detail itu sudah tersimpan di basis
pengetahuan dan diambil otomatis saat pelanggan bertanya — persona cukup menyebut kategori produk umum.
JANGAN menentukan tone, gaya bahasa, sapaan, atau penggunaan emoji karena semuanya diatur terpisah.
Jangan mengarang. Output HANYA teks persona, tanpa kalimat pembuka/penutup.`

// GenerateWebPersona menyusun system prompt persona dari beberapa cuplikan konten web (Home/About).
// Mencoba hingga 2x dan menyimpan hasil terlengkap, karena model kadang berhenti terlalu dini
// (persona terpotong di tengah, mis. berhenti tepat di sebuah heading).
func GenerateWebPersona(samples []string) (string, error) {
	joined := strings.TrimSpace(strings.Join(samples, "\n\n---\n\n"))
	if joined == "" {
		return "", nil
	}
	if r := []rune(joined); len(r) > webPersonaMaxInputRunes {
		joined = string(r[:webPersonaMaxInputRunes])
	}
	p := activePreset()
	if apiKeyForPreset(p) == "" {
		return "", fmt.Errorf("API key AI belum dikonfigurasi")
	}
	userMsg := "Konten website:\n" + joined

	var best string
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		resp, err := clientForPreset(p).CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model: p.Model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: webPersonaSystem},
				{Role: openai.ChatMessageRoleUser, Content: userMsg},
			},
			MaxTokens:   3000, // ruang lega: persona + token "mikir" model reasoning, agar tak kosong/terpotong
			Temperature: 0.4,
		})
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		if len(resp.Choices) == 0 {
			continue
		}
		out := strings.TrimSpace(resp.Choices[0].Message.Content)
		fr := string(resp.Choices[0].FinishReason)
		if len([]rune(out)) > len([]rune(best)) {
			best = out // simpan yang terpanjang sebagai cadangan
		}
		// finish=length berarti kehabisan token (terpotong di tengah) -> jangan diterima, ulangi.
		if fr != "length" && personaLooksComplete(out) {
			return out, nil
		}
		log.Printf("[persona] belum lengkap (len=%d, finish=%s) — retry", len([]rune(out)), fr)
	}
	// Best-effort: kalau tetap terpotong, rapikan ke akhir kalimat utuh (jangan berhenti di tengah kata).
	return cleanTruncatedPersona(best), lastErr
}

// cleanTruncatedPersona memotong teks ke akhir kalimat terakhir bila persona tampak terpotong,
// supaya tidak pernah berhenti di tengah kata.
func cleanTruncatedPersona(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || personaLooksComplete(s) {
		return s
	}
	if idx := strings.LastIndexAny(s, ".!?"); idx > 200 {
		return strings.TrimSpace(s[:idx+1])
	}
	return s
}

// personaLooksComplete menolak persona yang jelas terpotong (terlalu pendek atau berhenti di heading).
func personaLooksComplete(s string) bool {
	s = strings.TrimSpace(s)
	if len([]rune(s)) < 350 {
		return false
	}
	lines := strings.Split(s, "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	// Berhenti tepat di heading markdown ("## ...") atau label tanpa isi = terpotong.
	if strings.HasPrefix(last, "#") || strings.HasSuffix(last, ":") {
		return false
	}
	return true
}

// parseQAJSON mengekstrak array JSON [{question,answer}] dari output AI (toleran markdown/teks ekstra).
func parseQAJSON(raw string) []QAPair {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start == -1 || end == -1 || start >= end {
		return nil
	}
	var items []QAPair
	if json.Unmarshal([]byte(s[start:end+1]), &items) != nil {
		return nil
	}
	out := make([]QAPair, 0, len(items))
	for _, it := range items {
		q, a, tags := strings.TrimSpace(it.Question), strings.TrimSpace(it.Answer), strings.TrimSpace(it.Tags)
		if q != "" && a != "" {
			out = append(out, QAPair{Question: q, Answer: a, Tags: tags})
		}
	}
	return out
}
