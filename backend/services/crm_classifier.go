package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"wa-assistant/backend/models"

	openai "github.com/sashabaranov/go-openai"
)

// CRMLeadAssessment adalah rekomendasi internal. Nilai ini tidak dikirim kepada
// pelanggan dan baru diterapkan handler setelah melewati ambang keyakinan + validasi
// terhadap definisi tahap milik user (anti-halu: AI tidak boleh menciptakan tahap).
type CRMLeadAssessment struct {
	Stage      string  `json:"stage"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

// CRMStageHint = definisi tahap yang dikirim ke prompt AI (dari LeadStageDef user).
type CRMStageHint struct {
	Key         string
	Name        string
	Description string
	IsClosing   bool
}

// ClassifyCRMLead menilai tahap minat pelanggan memakai definisi tahap MILIK USER
// (bukan hardcode). closingDefinition menjelaskan arti "closing" untuk bisnis ini.
func ClassifyCRMLead(history []models.ChatHistory, memory string, stages []CRMStageHint, closingDefinition string) (CRMLeadAssessment, error) {
	if len(history) == 0 {
		return CRMLeadAssessment{}, fmt.Errorf("riwayat percakapan kosong")
	}
	if len(stages) == 0 {
		return CRMLeadAssessment{}, fmt.Errorf("definisi tahap pipeline kosong")
	}
	allowed := make(map[string]bool, len(stages))
	for _, s := range stages {
		allowed[s.Key] = true
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

	prompt := buildCRMClassifierPrompt(stages, closingDefinition, allowed)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Retry dengan budget membesar bertahap: model DeepSeek generasi baru memakai
	// sebagian jatah token untuk reasoning internal — budget kecil bisa habis
	// sebelum JSON tertulis (finish_reason=length, content kosong). Fail-closed:
	// bila semua gagal, kembalikan error → tahap lama dipertahankan.
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: prompt},
		{Role: openai.ChatMessageRoleUser, Content: "MEMORI LAMA:\n" + strings.TrimSpace(memory) + "\n\nPERCAKAPAN TERBARU:\n" + transcript.String()},
	}
	var lastErr error
	for attempt, budget := range []int{220, 700, 1500} {
		resp, err := CreateAICompletion(ctx, messages, budget, 0.1)
		if err != nil {
			lastErr = err
			continue
		}
		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf("AI tidak mengembalikan klasifikasi CRM")
			continue
		}
		content := resp.Choices[0].Message.Content
		if strings.TrimSpace(content) == "" {
			lastErr = fmt.Errorf("respons AI kosong (finish=%s) pada percobaan %d", resp.Choices[0].FinishReason, attempt+1)
			continue
		}
		assessment, perr := parseCRMLeadAssessment(content, allowed)
		if perr == nil {
			return assessment, nil
		}
		lastErr = perr
	}
	return CRMLeadAssessment{}, lastErr
}

// buildCRMClassifierPrompt menyusun prompt dari definisi tahap user. Aturan anti-halu
// ditulis eksplisit agar model tidak menciptakan tahap atau asal menyimpulkan.
func buildCRMClassifierPrompt(stages []CRMStageHint, closingDefinition string, allowed map[string]bool) string {
	var sb strings.Builder
	sb.WriteString("Klasifikasikan minat pelanggan untuk CRM berdasarkan seluruh konteks yang diberikan.\n")
	sb.WriteString("TAHAP YANG TERSEDIA (HANYA ini yang boleh dipakai):\n")
	for _, s := range stages {
		name := s.Name
		if name == "" {
			name = s.Key
		}
		desc := strings.TrimSpace(s.Description)
		if desc == "" {
			desc = "-"
		}
		closing := ""
		if s.IsClosing {
			closing = " [CLOSING — JANGAN PERNAH dipakai AI, hanya dari transaksi terkonfirmasi]"
		}
		sb.WriteString(fmt.Sprintf("- %s (%s): %s%s\n", s.Key, name, desc, closing))
	}
	if cd := strings.TrimSpace(closingDefinition); cd != "" {
		sb.WriteString("\nDEFINISI CLOSING BISNIS INI: " + cd + "\n")
	}
	sb.WriteString(`
Aturan:
- Nilai maksud pelanggan, bukan keramahan bahasa CS.
- Sapaan, "iya", "oke", atau jawaban singkat tanpa konteks bukan bukti minat.
- Jangan menaikkan tahap hanya karena CS menawarkan form atau closing.
- Jangan pernah memakai tahap closing — closing hanya dari transaksi yang benar-benar terkonfirmasi.
- Tahap yang dipakai WAJIB salah satu dari daftar di atas. Jangan menciptakan tahap baru.
- Bila ragu atau informasi kurang, pakai tahap paling rendah yang masuk akal, bukan yang tinggi.
- Gunakan konteks lama agar jawaban singkat tetap nyambung.
- Keluarkan JSON saja: {"stage":"<key dari daftar>","confidence":0.0,"reason":"alasan faktual singkat dalam bahasa Indonesia"}`)
	return sb.String()
}

// parseCRMLeadAssessment memvalidasi output AI secara ketat (fail-closed):
// stage harus persis salah satu key yang diizinkan, confidence 0-1, reason wajib.
func parseCRMLeadAssessment(raw string, allowed map[string]bool) (CRMLeadAssessment, error) {
	clean := strings.TrimSpace(raw)
	if start, end := strings.Index(clean, "{"), strings.LastIndex(clean, "}"); start >= 0 && end > start {
		clean = clean[start : end+1]
	}
	var out CRMLeadAssessment
	if err := json.Unmarshal([]byte(clean), &out); err != nil {
		return CRMLeadAssessment{}, fmt.Errorf("format klasifikasi CRM tidak valid: %w", err)
	}
	out.Stage = strings.ToLower(strings.TrimSpace(out.Stage))
	if !allowed[out.Stage] {
		return CRMLeadAssessment{}, fmt.Errorf("tahap CRM AI tidak dikenali: %q (bukan dari daftar tahap aktif)", out.Stage)
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

// SortStageHints mengurutkan hint sesuai rank definisi user.
func SortStageHints(stages []CRMStageHint, rankOf func(key string) int) {
	sort.SliceStable(stages, func(i, j int) bool {
		return rankOf(stages[i].Key) < rankOf(stages[j].Key)
	})
}
