package handlers

import (
	"fmt"
	"log"
	"strings"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

type GenerateReq struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

// GenerateKnowledge mengubah informasi bebas menjadi FAQ terstruktur dan idempotent.
func GenerateKnowledge(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req GenerateReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		c.JSON(400, gin.H{"error": "Informasi bisnis wajib diisi"})
		return
	}
	if req.Count <= 0 {
		req.Count = 10
	}
	if req.Count > 30 {
		req.Count = 30
	}
	items, err := services.GenerateFAQFromText(req.Text, "pelanggan yang membutuhkan informasi bisnis tersebut", req.Count)
	if err != nil {
		log.Printf("GenerateKnowledge agent %d gagal: %v", aid, err)
		c.JSON(502, gin.H{"error": "AI belum bisa mengolah informasi. Periksa konfigurasi API AI lalu coba lagi."})
		return
	}
	writer := newKnowledgeUpserter(aid)
	var created []models.Knowledge
	for _, item := range items {
		k, covered, saveErr := writer.save(item.Question, item.Answer, item.Tags, "text", "")
		if saveErr != nil {
			c.JSON(500, gin.H{"error": "Sebagian FAQ belum bisa disimpan"})
			return
		}
		if covered {
			created = append(created, k)
		}
	}
	if len(created) == 0 {
		c.JSON(422, gin.H{"error": "Tidak ditemukan fakta yang cukup untuk dijadikan FAQ"})
		return
	}

	c.JSON(201, gin.H{"data": created, "knowledge": len(created)})
}

// ImportKnowledge mengimpor banyak Q&A sekaligus (format JSON) ke knowledge agent,
// lalu menghitung embedding-nya. Upsert berdasarkan (agent_id, question).
func ImportKnowledge(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Items []struct {
			Question string `json:"question"`
			Answer   string `json:"answer"`
			Tags     string `json:"tags"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "JSON tidak valid"})
		return
	}

	created, updated := 0, 0
	writer := newKnowledgeUpserter(aid)
	for _, it := range req.Items {
		before := writer.byQuestion[canonicalKnowledgeQuestion(it.Question)]
		_, covered, err := writer.save(it.Question, it.Answer, it.Tags, "import", "")
		if err != nil {
			c.JSON(500, gin.H{"error": "Knowledge belum bisa diimpor"})
			return
		}
		if !covered {
			continue
		}
		if before != nil {
			updated++
		} else {
			created++
		}
	}
	c.JSON(200, gin.H{"created": created, "updated": updated})
}

// SetupWizardReq = input profil bisnis dari user.
type SetupWizardReq struct {
	BizName    string `json:"biz_name"`
	BizType    string `json:"biz_type"`
	Products   string `json:"products"`
	PriceRange string `json:"price_range"`
	OrderFlow  string `json:"order_flow"`
	Payment    string `json:"payment"`
	Shipping   string `json:"shipping"`
	Location   string `json:"location"`
	Hours      string `json:"hours"`
	Policies   string `json:"policies"`
	CSName     string `json:"cs_name"`
}

// buildFallbackFAQ hanya memakai field yang benar-benar diisi. Tidak ada asumsi COD,
// pembayaran, garansi, atau kebijakan lain yang berisiko menjadi jawaban palsu.
func buildFallbackFAQ(req SetupWizardReq) []services.QAPair {
	items := []services.QAPair{{
		Question: "Bisnis apa ini?", Answer: req.BizName, Tags: "profil,bisnis",
	}}
	if req.Products != "" {
		items = append(items, services.QAPair{Question: "Produk atau layanan apa yang tersedia?", Answer: req.Products, Tags: "produk,layanan"})
	}
	if req.PriceRange != "" {
		items = append(items, services.QAPair{Question: "Berapa kisaran harganya?", Answer: req.PriceRange, Tags: "harga"})
	}
	if req.OrderFlow != "" {
		items = append(items, services.QAPair{Question: "Bagaimana cara melakukan pemesanan?", Answer: req.OrderFlow, Tags: "order,pemesanan"})
	}
	if req.Payment != "" {
		items = append(items, services.QAPair{Question: "Metode pembayaran apa yang tersedia?", Answer: req.Payment, Tags: "pembayaran"})
	}
	if req.Shipping != "" {
		items = append(items, services.QAPair{Question: "Bagaimana proses pengirimannya?", Answer: req.Shipping, Tags: "pengiriman"})
	}
	if req.Location != "" {
		items = append(items, services.QAPair{Question: "Di mana lokasi bisnisnya?", Answer: req.Location, Tags: "lokasi,alamat"})
	}
	if req.Hours != "" {
		items = append(items, services.QAPair{Question: "Kapan jam operasionalnya?", Answer: req.Hours, Tags: "jam,operasional"})
	}
	if req.Policies != "" {
		items = append(items, services.QAPair{Question: "Apa kebijakan penting yang perlu diketahui pelanggan?", Answer: req.Policies, Tags: "kebijakan,syarat"})
	}
	return items
}

// SetupWizard — satu form profil bisnis, auto-generate System Prompt + Knowledge.
func SetupWizard(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req SetupWizardReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format profil bisnis tidak valid"})
		return
	}
	req.BizName = strings.TrimSpace(req.BizName)
	req.BizType = strings.TrimSpace(req.BizType)
	req.Products = strings.TrimSpace(req.Products)
	req.PriceRange = strings.TrimSpace(req.PriceRange)
	req.OrderFlow = strings.TrimSpace(req.OrderFlow)
	req.Payment = strings.TrimSpace(req.Payment)
	req.Shipping = strings.TrimSpace(req.Shipping)
	req.Location = strings.TrimSpace(req.Location)
	req.Hours = strings.TrimSpace(req.Hours)
	req.Policies = strings.TrimSpace(req.Policies)
	req.CSName = strings.TrimSpace(req.CSName)
	if req.BizName == "" || req.Products == "" {
		c.JSON(400, gin.H{"error": "Nama bisnis dan produk/layanan wajib diisi"})
		return
	}
	profile := wizardProfileText(req)

	// 1. Persona dan FAQ memakai provider yang sama dengan chat utama.
	systemPrompt, personaErr := services.GenerateBusinessPersona(profile)
	if personaErr != nil {
		log.Printf("[SetupWizard] persona AI gagal, pakai fallback faktual: %v", personaErr)
	}
	if systemPrompt == "" {
		systemPrompt = fallbackWizardPersona(req)
	}
	if systemPrompt != "" {
		if err := database.DB.Model(&models.Agent{}).Where("id = ?", aid).Update("system_prompt", systemPrompt).Error; err != nil {
			c.JSON(500, gin.H{"error": "Persona belum bisa disimpan"})
			return
		}
	}

	items, err := services.GenerateFAQFromText(profile, "pelanggan bisnis "+req.BizName, 15)
	if err != nil {
		log.Printf("[SetupWizard] FAQ AI gagal, pakai fallback faktual: %v", err)
		items = buildFallbackFAQ(req)
	}

	// Hanya refresh hasil Setup Cepat. FAQ manual, website, impor, dan Tulis Info tetap aman.
	if err := database.DB.Where("agent_id = ? AND source = ?", aid, "wizard").Delete(&models.Knowledge{}).Error; err != nil {
		c.JSON(500, gin.H{"error": "Knowledge lama belum bisa diperbarui"})
		return
	}
	writer := newKnowledgeUpserter(aid)
	created := 0
	for _, item := range items {
		_, covered, saveErr := writer.save(item.Question, item.Answer, item.Tags, "wizard", "")
		if saveErr != nil {
			c.JSON(500, gin.H{"error": "FAQ Setup Cepat belum bisa disimpan"})
			return
		}
		if covered {
			created++
		}
	}
	services.InvalidateKB(aid)

	// Reset ringkasan percakapan semua kontak — bisnis udah ganti, konteks lama gak relevan.
	database.DB.Where("agent_id = ?", aid).Delete(&models.ConversationMemory{})

	c.JSON(200, gin.H{
		"message":       "Setup selesai. Persona dan FAQ awal sudah diperbarui.",
		"system_prompt": systemPrompt,
		"knowledge":     created,
	})
}

func wizardProfileText(req SetupWizardReq) string {
	lines := []string{"Nama bisnis: " + req.BizName, "Produk atau layanan: " + req.Products}
	optional := []struct{ label, value string }{
		{"Jenis bisnis", req.BizType},
		{"Kisaran harga", req.PriceRange},
		{"Cara order", req.OrderFlow},
		{"Metode pembayaran", req.Payment},
		{"Pengiriman", req.Shipping},
		{"Lokasi", req.Location},
		{"Jam operasional", req.Hours},
		{"Kebijakan penting", req.Policies},
		{"Nama asisten/CS", req.CSName},
	}
	for _, field := range optional {
		if field.value != "" {
			lines = append(lines, field.label+": "+field.value)
		}
	}
	return strings.Join(lines, "\n")
}

func fallbackWizardPersona(req SetupWizardReq) string {
	name := req.CSName
	if name == "" {
		name = "asisten customer service"
	}
	parts := []string{fmt.Sprintf("Kamu adalah %s untuk %s.", name, req.BizName)}
	if req.Products != "" {
		parts = append(parts, "Bantu pelanggan memahami dan memilih dari produk atau layanan berikut: "+req.Products+".")
	}
	if req.OrderFlow != "" {
		parts = append(parts, "Ikuti alur pemesanan ini: "+req.OrderFlow+".")
	}
	parts = append(parts, "Gunakan hanya fakta dari basis pengetahuan, jangan mengarang harga, stok, promo, kebijakan, atau janji layanan. Jika informasi spesifik tidak tersedia, nyatakan belum ada data dan alihkan untuk pemeriksaan manusia.")
	return strings.Join(parts, " ")
}
