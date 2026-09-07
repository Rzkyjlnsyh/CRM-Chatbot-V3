package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
	"unicode"

	"wa-assistant/backend/models"

	openai "github.com/sashabaranov/go-openai"
)

// ConversationBrief = ringkasan operasional untuk CS di inbox (bukan dikirim ke pelanggan).
const ConversationBriefVersion = 2

type ConversationBrief struct {
	Version      int      `json:"version"`       // versi algoritma/cache
	ContactHint  string   `json:"contact_hint"`  // nama/panggilan bila terdeteksi
	Intent       string   `json:"intent"`        // kebutuhan utama 1 kalimat
	CurrentState string   `json:"current_state"` // kondisi percakapan paling akhir
	WaitingFor   string   `json:"waiting_for"`   // cs|customer|none
	Products     []string `json:"products"`      // produk/layanan disebut
	KeyFacts     []string `json:"key_facts"`     // fakta penting (alamat, ukuran, budget…)
	OpenItems    []string `json:"open_items"`    // yang masih perlu ditindaklanjuti
	RiskFlags    []string `json:"risk_flags"`    // refund, komplain, dll.
	Stage        string   `json:"stage"`         // new|info|interest|transaction|issue|done
	Summary      string   `json:"summary"`       // 2–4 kalimat padat
	Source       string   `json:"source"`        // heuristic|ai|hybrid
	Enhancement  string   `json:"enhancement"`   // local|ai
	EnhanceNote  string   `json:"enhancement_note,omitempty"`
	MessageCount int      `json:"message_count"`
	LastChatID   uint     `json:"last_chat_id"`
	UpdatedAt    string   `json:"updated_at"`
	NeedsHuman   bool     `json:"needs_human"`
	Stale        bool     `json:"stale"` // true bila cache ketinggalan vs chat terbaru
	Confidence   float64  `json:"confidence"`
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

// BuildConversationBriefHeuristic merangkum percakapan tanpa panggilan AI.
// Dipakai di GET inbox agar buka chat tidak membebani server / menunda UI.
func BuildConversationBriefHeuristic(_ uint, _ string, msgs []models.ChatHistory, memory string, needsHuman bool) (ConversationBrief, error) {
	lastID := uint(0)
	if len(msgs) > 0 {
		lastID = msgs[len(msgs)-1].ID
	}
	transcript := buildBriefTranscript(msgs)
	groundSrc := transcript
	if strings.TrimSpace(memory) != "" {
		groundSrc = transcript + "\n" + memory
	}
	heuristic := extractBriefHeuristic(transcript, memory, needsHuman)
	heuristic.Version = ConversationBriefVersion
	heuristic.MessageCount = countBriefTurns(msgs)
	heuristic.LastChatID = lastID
	heuristic.NeedsHuman = needsHuman
	refineBriefFromChronology(&heuristic, msgs, memory)
	heuristic.UpdatedAt = time.Now().Format(time.RFC3339)
	heuristic.Source = "heuristic"
	heuristic.Enhancement = "local"
	normalizeBriefCollections(&heuristic)
	heuristic.Confidence = briefConfidence(heuristic, groundSrc)
	if heuristic.Confidence < 0.4 {
		heuristic.Confidence = 0.4
	}
	if heuristic.Summary == "" {
		heuristic.Summary = joinNonEmpty(" · ", heuristic.Intent, strings.Join(heuristic.OpenItems, "; "))
	}
	return heuristic, nil
}

// BuildConversationBrief menyusun brief akurat: ekstraksi heuristik + AI terstruktur + validasi grounding.
// force diabaikan di sini (cache diputus di handler); signature tetap untuk call-site yang eksplisit.
func BuildConversationBrief(agentID uint, sender string, msgs []models.ChatHistory, memory string, needsHuman bool, _ bool) (ConversationBrief, error) {
	// msgs: kronologis lama→baru
	heuristic, err := BuildConversationBriefHeuristic(agentID, sender, msgs, memory, needsHuman)
	if err != nil {
		return heuristic, err
	}
	if len(msgs) < 2 && strings.TrimSpace(memory) == "" {
		return heuristic, nil
	}

	transcript := buildBriefTranscript(msgs)
	groundSrc := transcript
	if strings.TrimSpace(memory) != "" {
		groundSrc = transcript + "\n" + memory
	}

	// AI brief (structured) — hanya path refresh/force.
	aiPart, err := generateAIBrief(heuristic, transcript, needsHuman)
	if err != nil {
		log.Printf("Ringkasan AI agent %d tidak tersedia, memakai ringkasan lokal: %v", agentID, err)
		heuristic.EnhanceNote = briefEnhancementNote(err)
		return heuristic, nil
	}

	merged := mergeBriefs(heuristic, aiPart, groundSrc)
	merged.MessageCount = heuristic.MessageCount
	merged.LastChatID = heuristic.LastChatID
	merged.NeedsHuman = needsHuman
	merged.Version = ConversationBriefVersion
	merged.UpdatedAt = time.Now().Format(time.RFC3339)
	merged.Source = "hybrid"
	merged.Enhancement = "ai"
	merged.EnhanceNote = ""
	normalizeBriefCollections(&merged)
	merged.Confidence = briefConfidence(merged, groundSrc)
	return merged, nil
}

func briefEnhancementNote(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "prompt tokens limit"), strings.Contains(message, "context length"):
		return "Batas konteks layanan AI tercapai; ringkasan lokal tetap digunakan."
	case strings.Contains(message, "402"), strings.Contains(message, "payment required"), strings.Contains(message, "credit"):
		return "Kuota layanan AI belum tersedia; ringkasan lokal tetap digunakan."
	case strings.Contains(message, "api key"):
		return "Layanan AI belum dikonfigurasi; ringkasan lokal tetap digunakan."
	case strings.Contains(message, "deadline"), strings.Contains(message, "timeout"):
		return "Layanan AI terlalu lama merespons; ringkasan lokal tetap digunakan."
	default:
		return "Layanan AI sedang tidak tersedia; ringkasan lokal tetap digunakan."
	}
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

type briefTurn struct {
	Role      string
	Text      string
	MediaType string
	FileName  string
}

func briefTurnsFromMessages(msgs []models.ChatHistory) []briefTurn {
	turns := make([]briefTurn, 0, len(msgs)*2)
	for _, msg := range msgs {
		if text := strings.TrimSpace(msg.Message); text != "" {
			turns = append(turns, briefTurn{Role: "customer", Text: text, MediaType: msg.MediaType, FileName: msg.FileName})
		}
		if text := strings.TrimSpace(msg.Reply); text != "" {
			mediaType, fileName := "", ""
			// Pada row media keluar, Message kosong dan Reply adalah caption/placeholder.
			// Pada row gabungan incoming+balasan AI, media hanya milik pelanggan.
			if strings.TrimSpace(msg.Message) == "" {
				mediaType, fileName = msg.MediaType, msg.FileName
			}
			turns = append(turns, briefTurn{Role: "cs", Text: text, MediaType: mediaType, FileName: fileName})
		}
	}
	return turns
}

func briefRecentText(turns []briefTurn, limit int) string {
	if limit <= 0 || limit > len(turns) {
		limit = len(turns)
	}
	var parts []string
	for _, turn := range turns[len(turns)-limit:] {
		parts = append(parts, turn.Text)
	}
	return strings.ToLower(strings.Join(parts, "\n"))
}

func containsAnyBrief(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func briefCustomerTurnsAreSmallTalk(turns []briefTurn) bool {
	foundCustomer := false
	for _, turn := range turns {
		if turn.Role != "customer" {
			continue
		}
		foundCustomer = true
		text := strings.ToLower(strings.TrimSpace(turn.Text))
		if len([]rune(text)) > 36 || containsAnyBrief(text,
			"harga", "order", "pesan", "beli", "booking", "refund", "komplain",
			"ongkir", "status", "resi", "internet", "gangguan", "tagihan", "bayar",
		) {
			return false
		}
	}
	return foundCustomer
}

func briefCustomerClosed(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" || containsAnyBrief(text, "belum selesai", "belum beres", "belum normal", "belum bisa") {
		return false
	}
	return containsAnyBrief(text,
		"terima kasih", "makasih", "trimakasih", "sudah jelas", "sudah beres",
		"sudah selesai", "sudah normal", "sudah bisa", "oke sip", "oke siap",
		"ok sip", "ok siap", "aman kak", "aman mas", "aman min",
	) || text == "oke" || text == "ok" || text == "siap"
}

func briefCSWaitsForCustomer(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(text, "?") || containsAnyBrief(text,
		"mohon konfirmasi", "tolong konfirmasi", "silakan cek", "silahkan cek",
		"coba cek", "coba restart", "mohon kirim", "tolong kirim", "boleh kirim",
		"boleh info", "mohon info", "tolong info", "kabari kami", "kabarin kami",
		"pilih yang", "silakan pilih", "silahkan pilih", "mohon diisi", "tolong isi",
	)
}

func briefCSPendingAction(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	if containsAnyBrief(text, "sudah kami cek", "sudah saya cek", "sudah diproses", "telah diproses", "sudah selesai") {
		return false
	}
	return containsAnyBrief(text,
		"akan kami cek", "akan saya cek", "kami cek dulu", "saya cek dulu", "sedang kami cek",
		"akan kami proses", "akan saya proses", "sedang diproses", "kami tindak lanjuti",
		"akan ditindaklanjuti", "saya koordinasikan", "kami koordinasikan", "mohon tunggu",
		"tunggu sebentar", "nanti kami kabari", "nanti saya kabari",
	)
}

func classifyCurrentBriefIntent(turns []briefTurn, fallbackIntent, fallbackStage string) (string, string) {
	if len(turns) == 0 {
		return fallbackIntent, fallbackStage
	}
	recent := briefRecentText(turns, 12)
	tail := briefRecentText(turns, 3)
	last := strings.ToLower(strings.TrimSpace(turns[len(turns)-1].Text))
	if turns[len(turns)-1].Role == "customer" && briefCustomerClosed(last) {
		return "Percakapan sudah ditangani", "done"
	}
	if containsAnyBrief(recent, "refund", "pengembalian dana", "komplain", "salah transfer", "penipuan", "barang rusak", "gangguan", "tidak bisa", "bermasalah") &&
		!containsAnyBrief(tail, "sudah beres", "sudah selesai", "sudah normal", "sudah bisa") {
		return "Keluhan atau kendala pelanggan", "issue"
	}
	if containsAnyBrief(recent, "status pesanan", "status order", "lacak", "resi") {
		return "Menanyakan status pesanan atau pengiriman", "transaction"
	}
	if briefIntentRe.MatchString(recent) && containsAnyBrief(recent, "order", "pesan", "beli", "booking") {
		return "Berminat melakukan pemesanan", "transaction"
	}
	if containsAnyBrief(recent, "ongkir", "ongkos kirim", "pengiriman") {
		return "Menanyakan pengiriman atau ongkir", "interest"
	}
	if containsAnyBrief(recent, "harga", "berapa", "paket apa", "informasi produk", "info produk") {
		return "Menanyakan harga atau informasi produk", "info"
	}
	if briefCustomerTurnsAreSmallTalk(turns) {
		return "Sapaan dan percakapan ringan", "new"
	}
	if strings.TrimSpace(fallbackIntent) == "" || fallbackIntent == "Sapaan / awal percakapan" {
		return "Percakapan umum dan tindak lanjut", "info"
	}
	return fallbackIntent, fallbackStage
}

func briefQuotedText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	value = truncateRunesBrief(value, limit)
	if value == "" {
		return ""
	}
	return "“" + value + "”"
}

func isBriefMediaPlaceholder(text, mediaType string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image":
		return text == "📷 foto" || text == "foto" || text == "[image]"
	case "video":
		return text == "🎥 video" || text == "video" || text == "[video]"
	case "audio":
		return text == "🎤 pesan suara" || text == "pesan suara" || text == "[audio]"
	case "sticker":
		return text == "🌟 stiker" || text == "stiker" || text == "[sticker]"
	case "document":
		return text == "📎 dokumen" || text == "dokumen" || text == "[document]"
	default:
		return false
	}
}

func describeBriefTurn(turn briefTurn) string {
	text := strings.TrimSpace(turn.Text)
	mediaType := strings.ToLower(strings.TrimSpace(turn.MediaType))
	if mediaType == "" {
		return briefQuotedText(text, 120)
	}
	label := map[string]string{
		"image": "gambar", "video": "video", "audio": "pesan suara",
		"sticker": "stiker", "document": "dokumen",
	}[mediaType]
	if label == "" {
		label = "lampiran"
	}
	if mediaType == "document" && strings.TrimSpace(turn.FileName) != "" {
		label += " " + briefQuotedText(turn.FileName, 70)
	}
	if text != "" && !isBriefMediaPlaceholder(text, mediaType) {
		return label + " dengan keterangan " + briefQuotedText(text, 100)
	}
	return label
}

func briefIntentLead(intent string) string {
	switch intent {
	case "Sapaan dan percakapan ringan":
		return "Percakapan masih berupa sapaan dan obrolan singkat."
	case "Percakapan sudah ditangani":
		return "Percakapan terlihat sudah ditangani."
	case "Keluhan atau kendala pelanggan":
		return "Pelanggan sedang menyampaikan keluhan atau kendala."
	case "Menanyakan status pesanan atau pengiriman":
		return "Pelanggan ingin mengetahui status pesanan atau pengiriman."
	case "Berminat melakukan pemesanan":
		return "Pelanggan menunjukkan minat untuk melakukan pemesanan."
	case "Menanyakan pengiriman atau ongkir":
		return "Pelanggan sedang menanyakan pengiriman atau ongkir."
	case "Menanyakan harga atau informasi produk":
		return "Pelanggan sedang mencari harga atau informasi produk."
	case "Percakapan umum dan tindak lanjut":
		return "Percakapan berisi komunikasi umum dan tindak lanjut dari CS."
	default:
		if strings.TrimSpace(intent) != "" {
			return strings.TrimSuffix(strings.TrimSpace(intent), ".") + "."
		}
		return ""
	}
}

func refineBriefFromChronology(brief *ConversationBrief, msgs []models.ChatHistory, memory string) {
	turns := briefTurnsFromMessages(msgs)
	brief.Intent, brief.Stage = classifyCurrentBriefIntent(turns, brief.Intent, brief.Stage)
	brief.OpenItems = nil
	if len(turns) == 0 {
		brief.WaitingFor = "none"
		brief.CurrentState = "Belum ada percakapan terbaru"
		if strings.TrimSpace(memory) != "" {
			brief.Summary = "Belum ada pesan terbaru. Konteks sebelumnya tetap tersedia pada riwayat pelanggan."
		} else {
			brief.Summary = "Belum ada isi percakapan yang dapat diringkas."
		}
		return
	}

	last := turns[len(turns)-1]
	customerClosed := last.Role == "customer" && briefCustomerClosed(last.Text)
	csWaitsForCustomer := last.Role == "cs" && briefCSWaitsForCustomer(last.Text)
	csPendingAction := last.Role == "cs" && briefCSPendingAction(last.Text)
	if brief.NeedsHuman {
		brief.WaitingFor = "cs"
		brief.CurrentState = "Perlu tindak lanjut CS"
		brief.OpenItems = appendUniqueBrief(brief.OpenItems, "Tindak lanjuti percakapan dari antrean Butuh CS")
	} else if customerClosed {
		brief.WaitingFor = "none"
		brief.CurrentState = "Percakapan selesai"
	} else if last.Role == "customer" {
		brief.WaitingFor = "cs"
		brief.CurrentState = "Menunggu balasan CS"
		brief.OpenItems = appendUniqueBrief(brief.OpenItems, "Balas pesan terakhir pelanggan: "+describeBriefTurn(last))
	} else if csWaitsForCustomer {
		brief.WaitingFor = "customer"
		brief.CurrentState = "Menunggu jawaban pelanggan"
		brief.OpenItems = appendUniqueBrief(brief.OpenItems, "Menunggu jawaban pelanggan atas "+briefQuotedText(last.Text, 100))
	} else if csPendingAction {
		brief.WaitingFor = "cs"
		brief.CurrentState = "Sedang ditindaklanjuti CS"
		brief.OpenItems = appendUniqueBrief(brief.OpenItems, "Selesaikan tindak lanjut yang disampaikan CS: "+briefQuotedText(last.Text, 100))
	} else {
		brief.WaitingFor = "none"
		brief.CurrentState = "Sudah ditanggapi CS"
	}

	parts := make([]string, 0, 3)
	if lead := briefIntentLead(brief.Intent); lead != "" {
		parts = append(parts, lead)
	}
	if customerClosed {
		parts = append(parts, "Pelanggan menutup percakapan dengan "+describeBriefTurn(last)+"; tidak ada balasan lanjutan yang perlu dikirim.")
	} else if last.Role == "customer" {
		if last.MediaType != "" {
			parts = append(parts, "Pesan terakhir pelanggan berupa "+describeBriefTurn(last)+" dan belum dibalas.")
		} else {
			parts = append(parts, "Pesan terakhir pelanggan, "+describeBriefTurn(last)+", belum dibalas.")
		}
	} else if csWaitsForCustomer {
		parts = append(parts, "CS sudah merespons dan sekarang menunggu jawaban pelanggan atas "+briefQuotedText(last.Text, 110)+".")
	} else if csPendingAction {
		parts = append(parts, "CS sudah merespons dan masih perlu menuntaskan tindak lanjut yang disampaikan melalui "+briefQuotedText(last.Text, 110)+".")
	} else if last.MediaType != "" {
		parts = append(parts, "CS sudah menanggapi dan terakhir mengirim "+describeBriefTurn(last)+".")
	} else {
		parts = append(parts, "CS sudah menanggapi; balasan terakhirnya "+describeBriefTurn(last)+".")
	}
	if brief.NeedsHuman {
		parts = append(parts, "Percakapan masih perlu diselesaikan oleh CS.")
	}
	brief.Summary = truncateRunesBrief(strings.Join(parts, " "), 520)
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

func generateAIBrief(base ConversationBrief, transcript string, needsHuman bool) (briefAIPayload, error) {
	p := activePreset()
	if apiKeyForPreset(p) == "" {
		return briefAIPayload{}, fmt.Errorf("API key belum dikonfigurasi")
	}
	// OpenRouter akun terbatas dapat menolak prompt di atas 766 token. Heuristik
	// lokal lebih dahulu memadatkan seluruh percakapan; AI hanya menerima basis
	// terstruktur dan ekor chat aktual untuk memperhalus bahasa/konteks terbaru.
	transcript = tailRunesBrief(transcript, 700)
	basePayload := struct {
		Intent       string   `json:"intent"`
		CurrentState string   `json:"state"`
		WaitingFor   string   `json:"waiting_for"`
		Summary      string   `json:"summary"`
		KeyFacts     []string `json:"facts"`
		OpenItems    []string `json:"open"`
		RiskFlags    []string `json:"risks"`
	}{
		Intent:       truncateRunesBrief(base.Intent, 80),
		CurrentState: truncateRunesBrief(base.CurrentState, 60),
		WaitingFor:   base.WaitingFor,
		Summary:      truncateRunesBrief(base.Summary, 240),
		KeyFacts:     compactBriefList(base.KeyFacts, 3, 60),
		OpenItems:    compactBriefList(base.OpenItems, 2, 80),
		RiskFlags:    compactBriefList(base.RiskFlags, 2, 50),
	}
	baseJSON, _ := json.Marshal(basePayload)
	sys := `Kamu analis internal Customer Service. Dari BASIS dan CHAT terbaru, keluarkan HANYA JSON:
{
  "contact_hint":"", "intent":"", "products":[], "key_facts":[],
  "open_items":[], "risk_flags":[],
  "stage": "new|info|interest|transaction|issue|done",
  "summary": "2-3 kalimat natural dan padat"
}
Aturan: terbaru mengalahkan konteks lama; bedakan Pelanggan dan CS; yang sudah dijawab/selesai bukan open item; media keluar adalah tindakan CS, bukan eskalasi; jangan mengarang fakta/angka; jangan sebut AI/bot; gunakan bahasa Indonesia luwes tanpa label kaku.`

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	resp, err := clientForPreset(p).CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: p.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: fmt.Sprintf(
				"BUTUH_CS=%t\nBASIS=%s\nCHAT_LAMA_KE_BARU:\n%s",
				needsHuman, baseJSON, transcript,
			)},
		},
		MaxTokens:   420,
		Temperature: 0.1,
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
	if !validBriefStage(out.Stage) {
		out.Stage = ""
	}
	return out, nil
}

func mergeBriefs(h ConversationBrief, ai briefAIPayload, transcript string) ConversationBrief {
	srcTokens := contentTokenSet(transcript)
	srcNumbers := normalizedFactNumbers(transcript)

	out := h
	if strings.TrimSpace(ai.ContactHint) != "" && briefPhraseGrounded(ai.ContactHint, transcript, srcNumbers, srcTokens, 0.45) {
		out.ContactHint = strings.TrimSpace(ai.ContactHint)
	}
	// Intent dan stage dari kronologi lebih dapat dipercaya daripada tebakan AI.
	// AI hanya boleh memperjelas intent yang masih generik; status akhir tetap
	// berasal dari urutan pesan nyata di refineBriefFromChronology.
	if (strings.TrimSpace(h.Intent) == "" || h.Intent == "Percakapan umum dan tindak lanjut") &&
		strings.TrimSpace(ai.Intent) != "" &&
		briefPhraseGrounded(ai.Intent, transcript, srcNumbers, srcTokens, 0.3) &&
		briefActionGrounded(ai.Intent, transcript) {
		out.Intent = strings.TrimSpace(ai.Intent)
	}
	if s := strings.TrimSpace(ai.Summary); s != "" &&
		briefPhraseGrounded(s, transcript, srcNumbers, srcTokens, 0.14) &&
		briefActionGrounded(s, transcript) {
		out.Summary = truncateRunesBrief(s, 600)
	}

	// Products: union, max 8
	for _, p := range ai.Products {
		p = strings.TrimSpace(p)
		if p != "" && briefPhraseGrounded(p, transcript, srcNumbers, srcTokens, 0.35) {
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
		o = normalizeBriefOpenItem(o)
		if o != "" &&
			briefPhraseGrounded(o, transcript, srcNumbers, srcTokens, 0.2) &&
			briefActionGrounded(o, transcript) &&
			briefOpenItemMatchesState(o, h.WaitingFor) {
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
		if r != "" && briefPhraseGrounded(r, transcript, srcNumbers, srcTokens, 0.18) {
			risks = appendUniqueBrief(risks, r)
		}
	}
	for _, r := range h.RiskFlags {
		risks = appendUniqueBrief(risks, r)
	}
	out.RiskFlags = risks
	return out
}

func validBriefStage(stage string) bool {
	switch strings.ToLower(strings.TrimSpace(stage)) {
	case "new", "info", "interest", "transaction", "issue", "done":
		return true
	default:
		return false
	}
}

var briefGroundingStopwords = map[string]bool{
	"ada": true, "agar": true, "akan": true, "atau": true, "bahwa": true,
	"belum": true, "bisa": true, "buat": true, "dalam": true, "dan": true,
	"dari": true, "dengan": true, "dia": true, "ini": true, "itu": true,
	"jadi": true, "juga": true, "karena": true, "kepada": true, "lagi": true,
	"masih": true, "mereka": true, "oleh": true, "pada": true, "pelanggan": true,
	"percakapan": true, "saat": true, "sampai": true, "saya": true, "sebagai": true,
	"sekarang": true, "sedang": true, "setelah": true, "sudah": true, "telah": true,
	"terakhir": true, "tersebut": true, "tidak": true, "untuk": true, "yang": true,
	"ingin": true, "perlu": true, "terlihat": true, "menjadi": true, "customer": true,
	"manusia": true,
}

func briefSubstantiveTokens(value string) map[string]bool {
	tokens := contentTokenSet(value)
	for token := range tokens {
		if briefGroundingStopwords[token] {
			delete(tokens, token)
		}
	}
	return tokens
}

// briefPhraseGrounded menjaga hasil AI tetap menempel pada teks sumber. Selain
// memblokir angka baru, minimal satu kata bermakna harus benar-benar ada di
// percakapan. Ini sengaja lebih ketat daripada sekadar valid JSON.
func briefPhraseGrounded(
	value, source string,
	srcNumbers map[string]bool,
	_ map[string]bool,
	minOverlap float64,
) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for number := range normalizedFactNumbers(value) {
		if !srcNumbers[number] {
			return false
		}
	}
	normalizedValue := strings.ToLower(strings.Join(strings.Fields(value), " "))
	normalizedSource := strings.ToLower(strings.Join(strings.Fields(source), " "))
	if normalizedValue != "" && strings.Contains(normalizedSource, normalizedValue) {
		return true
	}
	candidateTokens := briefSubstantiveTokens(value)
	if len(candidateTokens) == 0 {
		return false
	}
	sourceTokens := briefSubstantiveTokens(source)
	hits := 0
	for token := range candidateTokens {
		if sourceTokens[token] {
			hits++
		}
	}
	if hits == 0 {
		return false
	}
	return float64(hits)/float64(len(candidateTokens)) >= minOverlap
}

// Pernyataan tindakan sensitif tidak boleh muncul hanya karena parafrase AI.
// Bila AI menyebut beli/refund/selesai, sumber harus memuat sinyal sejenis.
func briefActionGrounded(value, source string) bool {
	value = strings.ToLower(value)
	source = strings.ToLower(source)
	checks := []struct {
		claims []string
		source []string
	}{
		{[]string{"beli", "memesan", "pemesanan", "order", "booking"}, []string{"beli", "pesan", "pemesanan", "order", "booking", "mau ambil"}},
		{[]string{"refund", "pengembalian dana"}, []string{"refund", "pengembalian dana", "uang kembali"}},
		{[]string{"sudah selesai", "sudah beres", "sudah normal", "telah selesai"}, []string{"sudah selesai", "sudah beres", "sudah normal", "sudah bisa", "terima kasih", "makasih"}},
		{[]string{"penipuan", "ditipu"}, []string{"penipuan", "ditipu", "penipu"}},
	}
	for _, check := range checks {
		if containsAnyBrief(value, check.claims...) && !containsAnyBrief(source, check.source...) {
			return false
		}
	}
	return true
}

func briefOpenItemMatchesState(item, waitingFor string) bool {
	item = strings.ToLower(strings.TrimSpace(item))
	if waitingFor != "customer" {
		return true
	}
	// Sesudah CS bertanya, pekerjaan aktif ada di pelanggan. Jangan ubah menjadi
	// instruksi palsu agar CS membalas lagi.
	return containsAnyBrief(item, "menunggu", "tunggu jawaban", "jawaban pelanggan") &&
		!containsAnyBrief(item, "balas pelanggan", "hubungi pelanggan", "kirim pelanggan")
}

func normalizeBriefOpenItem(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSpace(strings.TrimLeft(value, "-*•0123456789. )"))
	value = strings.Join(strings.Fields(value), " ")
	value = strings.TrimRight(value, ";")
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return truncateRunesBrief(string(runes), 180)
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

func normalizeBriefCollections(brief *ConversationBrief) {
	if brief.Enhancement == "" {
		if brief.Source == "hybrid" || brief.Source == "ai" {
			brief.Enhancement = "ai"
		} else {
			brief.Enhancement = "local"
		}
	}
	if brief.Products == nil {
		brief.Products = []string{}
	}
	if brief.KeyFacts == nil {
		brief.KeyFacts = []string{}
	}
	if brief.OpenItems == nil {
		brief.OpenItems = []string{}
	}
	if brief.RiskFlags == nil {
		brief.RiskFlags = []string{}
	}
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

func tailRunesBrief(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if limit <= 0 || len(runes) <= limit {
		return string(runes)
	}
	return "…" + string(runes[len(runes)-limit:])
}

func compactBriefList(values []string, maxItems, maxRunes int) []string {
	if maxItems <= 0 || len(values) == 0 {
		return []string{}
	}
	if len(values) < maxItems {
		maxItems = len(values)
	}
	out := make([]string, 0, maxItems)
	for _, value := range values[:maxItems] {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, truncateRunesBrief(value, maxRunes))
		}
	}
	return out
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
	normalizeBriefCollections(&b)
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
