package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// GenerateProductAIContent membuat draft FAQ dan arahan percakapan khusus produk.
// FAQ melewati validator grounding yang sama dengan generator knowledge umum.
func GenerateProductAIContent(source string, checkoutEnabled bool) (string, string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", "", fmt.Errorf("data produk kosong")
	}
	faq, err := GenerateFAQFromText(source, "pelanggan yang menanyakan produk ini", 10)
	if err != nil {
		return "", "", err
	}
	if len(faq) == 0 {
		return "", "", fmt.Errorf("data produk belum cukup untuk membuat FAQ")
	}
	var knowledge strings.Builder
	for i, item := range faq {
		if i > 0 {
			knowledge.WriteString("\n\n")
		}
		knowledge.WriteString("Q: " + strings.TrimSpace(item.Question) + "\n")
		knowledge.WriteString("A: " + strings.TrimSpace(item.Answer))
	}

	checkoutRule := "Produk ini belum memiliki checkout. Jika pelanggan siap memproses, tawarkan bantuan yang tersedia tanpa mengklaim data sudah dicatat."
	if checkoutEnabled {
		checkoutRule = "Produk ini memiliki checkout resmi. Saat niat pelanggan sudah jelas, arahkan ke checkout; jangan meminta seluruh data checkout lewat chat bebas."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	resp, err := CreateAICompletion(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: `Kamu menyusun arahan singkat untuk AI customer service yang menawarkan SATU produk.
Aturan mutlak:
- tulis 3-6 poin tindakan dalam bahasa Indonesia;
- jangan menambah fakta, stok, varian, manfaat, pengiriman, promo, garansi, atau kebijakan baru;
- setelah menjawab, ajukan maksimal satu pertanyaan lanjutan yang relevan dan hanya tentang aspek yang memang ada di sumber;
- jangan gunakan penutup generik seperti "ada lagi yang ditanyakan";
- jangan mengulang data yang sudah diberikan pelanggan;
- jangan menulis jawaban untuk pelanggan, tulis instruksi internal untuk AI;
- output teks biasa tanpa judul pembuka.`},
		{Role: openai.ChatMessageRoleUser, Content: "SUMBER DATA PRODUK:\n" + source + "\n\nATURAN CHECKOUT:\n" + checkoutRule},
	}, 900, 0.15)
	if err != nil {
		return "", "", err
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return "", "", fmt.Errorf("AI tidak mengembalikan arahan produk")
	}
	guidance := strings.TrimSpace(resp.Choices[0].Message.Content)
	guidance = strings.TrimPrefix(guidance, "```")
	guidance = strings.TrimSuffix(guidance, "```")
	guidance = strings.TrimSpace(guidance)
	if runes := []rune(guidance); len(runes) > 8000 {
		guidance = string(runes[:8000])
	}
	return knowledge.String(), guidance, nil
}
