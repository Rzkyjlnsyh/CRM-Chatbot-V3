package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	openai "github.com/sashabaranov/go-openai"
)

type VisionAnalysisResult struct {
	Analysis   string  `json:"analysis"`
	Reply      string  `json:"reply"`
	Answer     string  `json:"answer"`
	ProductID  uint    `json:"product_id"`
	Confidence float64 `json:"confidence"`
	NeedsHuman bool    `json:"needs_human"`
	Model      string  `json:"-"`
}

// AnalyzeCustomerImage menganalisis foto dengan model vision OpenRouter. Gambar yang
// cukup jelas dapat ditangani otomatis; keputusan berisiko ditandai needs_human (internal).
func AnalyzeCustomerImage(agentID uint, persona, tone, caption, instruction, mimetype string, data []byte, history []models.ChatHistory) (VisionAnalysisResult, error) {
	model := strings.TrimSpace(apiConfigFromDB("vision_model", "", ""))
	if model == "" {
		return VisionAnalysisResult{}, fmt.Errorf("model vision OpenRouter belum dipilih")
	}
	if len(data) == 0 {
		return VisionAnalysisResult{}, fmt.Errorf("data gambar kosong")
	}
	if len(data) > 10*1024*1024 {
		return VisionAnalysisResult{}, fmt.Errorf("gambar melebihi batas analisis 10 MB")
	}
	if !strings.HasPrefix(strings.ToLower(mimetype), "image/") {
		return VisionAnalysisResult{}, fmt.Errorf("media bukan gambar")
	}

	queryParts := []string{strings.TrimSpace(caption)}
	for i := len(history) - 1; i >= 0 && i >= len(history)-6; i-- {
		queryParts = append(queryParts, history[i].Message, history[i].Reply)
	}
	query := strings.TrimSpace(strings.Join(queryParts, "\n"))
	if query == "" {
		query = "pelanggan mengirim gambar untuk diperiksa"
	}

	var knowledge strings.Builder
	if productContext, _ := productKnowledgeContext(agentID, query); productContext != "" {
		knowledge.WriteString(productContext)
	}
	if relevant, _, _ := searchKnowledge(agentID, query); len(relevant) > 0 {
		knowledge.WriteString("\n\nKNOWLEDGE BISNIS YANG RELEVAN:\n")
		for _, item := range relevant {
			knowledge.WriteString("Q: " + item.Question + "\nA: " + item.Answer + "\n")
		}
	}
	productCatalog := visionProductCatalog(agentID)
	if productCatalog != "" {
		knowledge.WriteString("\n\nKATALOG PRODUK YANG BOLEH DIREKOMENDASIKAN:\n" + productCatalog)
	}
	task := strings.TrimSpace(instruction)
	if task == "" {
		task = "Pahami kebutuhan pelanggan dari gambar dan caption. Jika cocok, rekomendasikan maksimal satu produk katalog yang paling relevan."
	}

	system := buildSystemPrompt(agentID, persona) + toneInstruction(tone) + `

TUGAS ANALISIS GAMBAR:
- Konteks tugas saat ini: ` + task + `
- Jelaskan hanya hal yang benar-benar terlihat; bedakan observasi dengan dugaan.
- Cocokkan dengan knowledge bisnis/produk hanya bila relevan dan tidak bertentangan dengan gambar.
- Jangan memastikan keaslian bukti pembayaran, identitas, dokumen, merek, kondisi tersembunyi, keamanan, kelayakan, atau persetujuan akhir hanya dari gambar.
- Jangan menyatakan barang pasti diterima, pembayaran sah, atau proses selesai. Keputusan final yang berisiko: needs_human true dan di chat bilang kamu cek dulu (jangan sebut petugas/admin/AI).
- analysis adalah catatan internal faktual untuk CS, 1-5 kalimat.
- reply adalah balasan WhatsApp natural, 1-3 kalimat: akui gambar diterima, sebut observasi yang aman, lalu jawab atau ajukan maksimal satu klarifikasi relevan.
- answer adalah jawaban ringkas untuk pertanyaan form/checkout pada konteks tugas. Isi hanya jika dapat diekstrak secara valid dari gambar/caption; jika tidak relevan atau tidak yakin, isi string kosong.
- product_id adalah ID produk katalog yang paling cocok berdasarkan hal yang terlihat dan fakta katalog. Isi 0 jika tidak ada kecocokan yang cukup kuat. Jangan mengarang ID.
- confidence adalah 0 sampai 1 sesuai kejernihan dan kepastian observasi.
- needs_human false jika gambar terbaca jelas dan dapat dibantu dengan aman tanpa keputusan berisiko.
- needs_human true hanya jika gambar tidak jelas, informasi tidak cukup, pelanggan meminta validasi/keputusan final, atau menyangkut bukti pembayaran, identitas, dokumen, keamanan, keaslian, kelayakan, dan kondisi tersembunyi.
- Di reply ke pelanggan: jangan sebut petugas, admin, AI, bot, atau diteruskan. Tetap persona CS manusia.
Output HANYA JSON: {"analysis":"...","reply":"...","answer":"...","product_id":0,"confidence":0.0,"needs_human":true}.` + knowledge.String()

	userText := "Analisis gambar pelanggan ini."
	if strings.TrimSpace(caption) != "" {
		userText += "\nCaption pelanggan: " + strings.TrimSpace(caption)
	}
	dataURL := "data:" + mimetype + ";base64," + base64.StdEncoding.EncodeToString(data)
	messages := []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleSystem, Content: system}}
	for _, item := range history {
		if item.Message != "" {
			messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: item.Message})
		}
		if item.Reply != "" {
			messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: item.Reply})
		}
	}
	messages = append(messages, openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleUser,
		MultiContent: []openai.ChatMessagePart{
			{Type: openai.ChatMessagePartTypeText, Text: userText},
			{Type: openai.ChatMessagePartTypeImageURL, ImageURL: &openai.ChatMessageImageURL{URL: dataURL, Detail: openai.ImageURLDetailAuto}},
		},
	})
	preset := activePreset()
	preset.Model = model
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	resp, err := clientForPreset(preset).CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: model, Messages: messages, MaxTokens: 1200, Temperature: 0.15,
	})
	if err != nil {
		return VisionAnalysisResult{}, err
	}
	if len(resp.Choices) == 0 {
		return VisionAnalysisResult{}, fmt.Errorf("model vision tidak mengembalikan jawaban")
	}
	raw := strings.TrimSpace(resp.Choices[0].Message.Content)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var result VisionAnalysisResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return VisionAnalysisResult{}, fmt.Errorf("format analisis vision tidak valid: %w", err)
	}
	result.Analysis = strings.TrimSpace(result.Analysis)
	result.Reply = strings.TrimSpace(result.Reply)
	result.Answer = strings.TrimSpace(result.Answer)
	result.Model = model
	if result.Analysis == "" || result.Reply == "" {
		return VisionAnalysisResult{}, fmt.Errorf("analisis vision kosong")
	}
	if result.Confidence < 0 {
		result.Confidence = 0
	}
	if result.Confidence > 1 {
		result.Confidence = 1
	}
	if len([]rune(result.Answer)) > 1000 {
		result.Answer = string([]rune(result.Answer)[:1000])
	}
	if result.ProductID != 0 {
		var count int64
		database.DB.Model(&models.Product{}).Where("agent_id = ? AND id = ?", agentID, result.ProductID).Count(&count)
		if count == 0 {
			result.ProductID = 0
		}
	}
	return result, nil
}

func visionProductCatalog(agentID uint) string {
	var products []models.Product
	database.DB.Where("agent_id = ?", agentID).Order("updated_at desc").Limit(40).Find(&products)
	var lines []string
	for _, product := range products {
		facts := strings.Join(strings.Fields(product.Description+" "+product.DetailsJSON+" "+product.Knowledge), " ")
		runes := []rune(facts)
		if len(runes) > 360 {
			facts = string(runes[:360]) + "…"
		}
		line := fmt.Sprintf("- ID %d | %s", product.ID, strings.TrimSpace(product.Name))
		if strings.TrimSpace(product.Price) != "" {
			line += " | harga: " + strings.TrimSpace(product.Price)
		}
		if facts != "" {
			line += " | fakta: " + facts
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
