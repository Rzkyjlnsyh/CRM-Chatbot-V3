package services

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// ─────────────────────────────────────────────────────────────────────────
// AI Response Policy (pola v4) — budget panjang balasan per intent supaya
// jawaban WhatsApp tetap ringkas; balasan kelebihan budget dipadatkan SEKALI.
// ─────────────────────────────────────────────────────────────────────────

type AIResponsePolicy struct {
	Name         string
	MaxTokens    int
	MaxRunes     int
	MaxSentences int
}

var defaultAIResponsePolicy = AIResponsePolicy{Name: "conversation", MaxTokens: 220, MaxRunes: 300, MaxSentences: 3}

// selectAIResponsePolicy memilih policy dari pesan user.
// intent → budget: social paling pendek, factual/transaction sedang, catalog agak lega.
func selectAIResponsePolicy(userMsg, retrievalQuery, productContext string, knowledgeCount int) AIResponsePolicy {
	if strings.TrimSpace(userMsg) == "" {
		return defaultAIResponsePolicy
	}
	if looksLikeOrderProgressMessage(userMsg) {
		return AIResponsePolicy{Name: "transaction", MaxTokens: 220, MaxRunes: 240, MaxSentences: 3}
	}
	q := strings.ToLower(retrievalQuery)
	low := strings.ToLower(userMsg)
	switch {
	case knowledgeCount >= 3 && (strings.Contains(q, "katalog") || strings.Contains(q, "produk") ||
		strings.Contains(low, "katalog") || strings.Contains(low, "daftar harga")):
		return AIResponsePolicy{Name: "catalog", MaxTokens: 700, MaxRunes: 480, MaxSentences: 7}
	case knowledgeCount >= 2 && (strings.Contains(q, "faq") || strings.Contains(q, "tanya") ||
		strings.Contains(q, "harga") || strings.Contains(q, "cara")):
		return AIResponsePolicy{Name: "factual", MaxTokens: 300, MaxRunes: 330, MaxSentences: 4}
	case isSocialSmallTalk(low):
		return AIResponsePolicy{Name: "social", MaxTokens: 100, MaxRunes: 110, MaxSentences: 2}
	default:
		return defaultAIResponsePolicy
	}
}

func isSocialSmallTalk(low string) bool {
	if len([]rune(low)) > 40 {
		return false
	}
	for _, prefix := range []string{"halo", "hai", "hi ", "pagi", "siang", "sore", "malam", "assalam", "permisi", "test", "tes "} {
		if strings.HasPrefix(low, prefix) {
			return true
		}
	}
	return false
}

// sentenceBoundary = pemisah kalimat natural Indonesia.
var sentenceBoundary = regexp.MustCompile(`[.!?…\n]`)

// responseNeedsCondensing menilai apakah balasan melebihi budget policy.
func responseNeedsCondensing(reply string, p AIResponsePolicy) bool {
	trimmed := strings.TrimSpace(reply)
	if trimmed == "" {
		return false
	}
	if len([]rune(trimmed)) > p.MaxRunes {
		return true
	}
	if strings.Contains(trimmed, "[[") { // marka grounding v2 — jangan kirim mentah
		return true
	}
	nonEmpty := 0
	for _, line := range strings.Split(trimmed, "\n") {
		if strings.TrimSpace(line) != "" {
			nonEmpty++
		}
	}
	return nonEmpty > p.MaxSentences+2
}

// retryConciseReply meminta model menulis ulang jawaban sesuai budget policy.
// Dipakai SEBAGAI hasil final bila model patuh; gagal → panggil memakai yang asli.
func retryConciseReply(p aiPreset, messages []openai.ChatCompletionMessage, policy AIResponsePolicy) (string, bool) {
	concise := make([]openai.ChatCompletionMessage, len(messages))
	copy(concise, messages)
	if len(concise) == 0 {
		return "", false
	}
	concise[0].Content += `

KEBIJAKAN PANJANG JAWABAN (WAJIB):
- Tulis ulang jawaban secara langsung dan natural.
- Maksimal ` + strconv.Itoa(policy.MaxSentences) + ` kalimat.
- Jangan mengulang pertanyaan atau pembuka basa-basi.
- Jawab saja intinya.`
	resp, err := clientForPreset(p).CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{
		Model:       p.Model,
		Messages:    concise,
		MaxTokens:   policy.MaxTokens,
		Temperature: 0.2,
	})
	if err != nil || len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return "", false
	}
	out := strings.TrimSpace(resp.Choices[0].Message.Content)
	if strings.Contains(out, "[[") {
		return "", false
	}
	return out, true
}
