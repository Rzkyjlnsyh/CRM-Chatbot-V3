package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"wa-assistant/backend/models"

	openai "github.com/sashabaranov/go-openai"
)

// CRMLeadAssessment adalah rekomendasi internal. Nilai ini tidak dikirim kepada
// pelanggan dan baru diterapkan handler setelah melewati ambang keyakinan.
type CRMLeadAssessment struct {
	Stage      string  `json:"stage"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func ClassifyCRMLead(history []models.ChatHistory, memory string) (CRMLeadAssessment, error) {
	if len(history) == 0 {
		return CRMLeadAssessment{}, fmt.Errorf("riwayat percakapan kosong")
	}
	var transcript strings.Builder
	for _, item := range history {
		if text := strings.TrimSpace(item.Message); text != "" {
			transcript.WriteString("Pelanggan: " + text + "\n")
		}
		if text := strings.TrimSpace(item.Reply); text != "" {
			transcript.WriteString("CS: " + text + "\n")
		}
	}
	prompt := `Klasifikasikan minat pelanggan untuk CRM berdasarkan seluruh konteks yang diberikan.
Definisi:
- new: percakapan belum cukup untuk menilai minat, misalnya baru menyapa atau basa-basi.
- cold: topik bisnis relevan tetapi belum ada kebutuhan/minat yang jelas, menunda, atau hanya mencari informasi umum.
- warm: ada kebutuhan/minat nyata; menanyakan produk, layanan, harga, ketersediaan, rekomendasi, atau kecocokan.
- hot: niat memproses sudah jelas; ingin membeli, booking, mendaftar, meminta penjemputan/penawaran, atau mulai memberikan data transaksi.
- unqualified: jelas salah sasaran, spam, tidak relevan dengan bisnis, atau secara tegas tidak membutuhkan layanan.

Aturan:
- Nilai maksud pelanggan, bukan keramahan bahasa CS.
- Sapaan, "iya", "oke", atau jawaban singkat tanpa konteks bukan bukti minat.
- Jangan memberi hot hanya karena CS menawarkan form atau closing.
- Jangan pernah menghasilkan customer; status customer hanya berasal dari transaksi yang benar-benar terkonfirmasi.
- Gunakan konteks lama agar jawaban singkat tetap nyambung.
- Keluarkan JSON saja: {"stage":"new|cold|warm|hot|unqualified","confidence":0.0,"reason":"alasan faktual singkat dalam bahasa Indonesia"}`

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := CreateAICompletion(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: prompt},
		{Role: openai.ChatMessageRoleUser, Content: "MEMORI LAMA:\n" + strings.TrimSpace(memory) + "\n\nPERCAKAPAN TERBARU:\n" + transcript.String()},
	}, 220, 0.1)
	if err != nil {
		return CRMLeadAssessment{}, err
	}
	if len(resp.Choices) == 0 {
		return CRMLeadAssessment{}, fmt.Errorf("AI tidak mengembalikan klasifikasi CRM")
	}
	return parseCRMLeadAssessment(resp.Choices[0].Message.Content)
}

func parseCRMLeadAssessment(raw string) (CRMLeadAssessment, error) {
	clean := strings.TrimSpace(raw)
	if start, end := strings.Index(clean, "{"), strings.LastIndex(clean, "}"); start >= 0 && end > start {
		clean = clean[start : end+1]
	}
	var out CRMLeadAssessment
	if err := json.Unmarshal([]byte(clean), &out); err != nil {
		return CRMLeadAssessment{}, fmt.Errorf("format klasifikasi CRM tidak valid: %w", err)
	}
	out.Stage = strings.ToLower(strings.TrimSpace(out.Stage))
	switch out.Stage {
	case "new", "cold", "warm", "hot", "unqualified":
	default:
		return CRMLeadAssessment{}, fmt.Errorf("status CRM AI tidak valid: %q", out.Stage)
	}
	if out.Confidence < 0 {
		out.Confidence = 0
	} else if out.Confidence > 1 {
		out.Confidence = 1
	}
	out.Reason = strings.TrimSpace(out.Reason)
	for utf8.RuneCountInString(out.Reason) > 240 {
		_, size := utf8.DecodeLastRuneInString(out.Reason)
		out.Reason = out.Reason[:len(out.Reason)-size]
	}
	if out.Reason == "" {
		return CRMLeadAssessment{}, fmt.Errorf("alasan klasifikasi CRM kosong")
	}
	return out, nil
}
