package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	openai "github.com/sashabaranov/go-openai"
)

// KBItem = satu knowledge dengan vektor embedding yang sudah di-parse (siap pakai).
type KBItem struct {
	K   models.Knowledge
	Vec []float32
}

// Cache knowledge per-agent di memori: hindari query DB + unmarshal JSON tiap pesan masuk.
var (
	kbCache = map[uint][]KBItem{}
	kbDirty = map[uint]bool{}
	kbMu    sync.RWMutex
)

// InvalidateKB menandai cache knowledge sebuah agent perlu dimuat ulang (dipanggil saat ada perubahan).
func InvalidateKB(agentID uint) {
	kbMu.Lock()
	kbDirty[agentID] = true
	kbMu.Unlock()
}

// KnowledgeFor mengembalikan knowledge agent dari cache memori (embedding sudah di-parse).
// DB hanya di-query saat pertama kali atau setelah ada perubahan (create/update/delete).
func KnowledgeFor(agentID uint) []KBItem {
	kbMu.RLock()
	items, ok := kbCache[agentID]
	dirty := kbDirty[agentID]
	kbMu.RUnlock()
	if ok && !dirty {
		return items
	}

	var rows []models.Knowledge
	database.DB.Where("agent_id = ?", agentID).Find(&rows)
	items = make([]KBItem, 0, len(rows))
	for _, r := range rows {
		var vec []float32
		if r.Embedding != "" {
			_ = json.Unmarshal([]byte(r.Embedding), &vec)
		}
		r.Embedding = "" // hemat memori: vektor sudah disimpan terpisah di Vec
		items = append(items, KBItem{K: r, Vec: vec})
	}
	kbMu.Lock()
	kbCache[agentID] = items
	kbDirty[agentID] = false
	kbMu.Unlock()
	return items
}

var (
	embClient  *openai.Client
	embModel   string
	embDims    int
	embEnabled bool
	embMu      sync.RWMutex
)

const defaultEmbeddingModel = "openai/text-embedding-3-small"

// InitEmbedding memakai API key OpenRouter yang sama dengan seluruh fitur AI.
// Model dipilih dari dashboard dan dapat dimuat ulang tanpa restart backend.
func InitEmbedding() {
	key := apiKeyFromDB("api_key", "OPENROUTER_API_KEY")
	model := apiConfigFromDB("embedding_model", "", defaultEmbeddingModel)
	embMu.Lock()
	defer embMu.Unlock()
	if key == "" {
		embClient = nil
		embModel = model
		embDims = 0
		embEnabled = false
		log.Println("Embedding: API key OpenRouter kosong -> semantic search nonaktif (pakai keyword match)")
		return
	}
	cfg := openai.DefaultConfig(key)
	cfg.BaseURL = openRouterBase
	embClient = openai.NewClientWithConfig(cfg)
	embModel = model
	// Gunakan dimensi native model agar pergantian model dari dashboard aman.
	embDims = 0
	embEnabled = true
	log.Printf("Embedding OpenRouter aktif: model=%s", embModel)
}

func EmbeddingEnabled() bool {
	embMu.RLock()
	defer embMu.RUnlock()
	return embEnabled
}

// embSignature mengidentifikasi konfigurasi embedding aktif (model + dimensi). Disimpan
// bersama tiap vektor agar perubahan model/dimensi terdeteksi & knowledge di-embed ulang
// otomatis — mencegah retrieval "mati senyap" karena dimensi vektor tak cocok.
func embSignature() string {
	embMu.RLock()
	model, dims := embModel, embDims
	embMu.RUnlock()
	if dims > 0 {
		return fmt.Sprintf("%s:%d", model, dims)
	}
	return model
}

// Embed menghitung vektor embedding untuk satu teks.
func Embed(text string) ([]float32, error) {
	embMu.RLock()
	client, model, dims, enabled := embClient, embModel, embDims, embEnabled
	embMu.RUnlock()
	if !enabled || client == nil {
		return nil, fmt.Errorf("embedding OpenRouter belum dikonfigurasi")
	}
	req := openai.EmbeddingRequest{
		Input: []string{text},
		Model: openai.EmbeddingModel(model),
	}
	if dims > 0 {
		req.Dimensions = dims
	}
	resp, err := client.CreateEmbeddings(context.Background(), req)
	if err != nil {
		return nil, err
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("embedding kosong")
	}
	return resp.Data[0].Embedding, nil
}

// EmbeddingModelInfo adalah opsi model yang dibaca langsung dari katalog OpenRouter.
type EmbeddingModelInfo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length,omitempty"`
}

// ListOpenRouterEmbeddingModels mengambil katalog terbaru sehingga pilihan model
// di dashboard tidak ditanam permanen di kode maupun environment.
func ListOpenRouterEmbeddingModels(ctx context.Context) ([]EmbeddingModelInfo, error) {
	key := apiKeyFromDB("api_key", "OPENROUTER_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("API key OpenRouter belum dikonfigurasi")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterBase+"/embeddings/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gagal mengambil model embedding OpenRouter: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("OpenRouter mengembalikan status %d", resp.StatusCode)
	}
	var payload struct {
		Data []EmbeddingModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("respons katalog model OpenRouter tidak valid: %w", err)
	}
	sort.Slice(payload.Data, func(i, j int) bool {
		return payload.Data[i].Name < payload.Data[j].Name
	})
	return payload.Data, nil
}

// cosineSim menghitung kemiripan kosinus dua vektor (-1..1).
func cosineSim(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(na) * math.Sqrt(nb)))
}

func knowledgeText(k *models.Knowledge) string {
	return k.Question + "\n" + k.Answer + "\n" + k.Tags
}

// IndexKnowledge menghitung embedding satu knowledge lalu menyimpannya ke kolom embedding.
func IndexKnowledge(k *models.Knowledge) {
	InvalidateKB(k.AgentID) // isi knowledge berubah -> cache agent ini perlu di-refresh
	if !EmbeddingEnabled() {
		return
	}
	vec, err := Embed(knowledgeText(k))
	if err != nil {
		log.Printf("Embedding: gagal embed knowledge #%d: %v", k.ID, err)
		return
	}
	b, _ := json.Marshal(vec)
	k.Embedding = string(b)
	k.EmbeddingModel = embSignature()
	if err := database.DB.Model(k).Updates(map[string]any{
		"embedding":       k.Embedding,
		"embedding_model": k.EmbeddingModel,
	}).Error; err != nil {
		log.Printf("Embedding: gagal simpan embedding knowledge #%d: %v", k.ID, err)
	}
}

// BackfillEmbeddings mengisi embedding untuk knowledge yang belum punya, ATAU yang dibuat
// dengan model/dimensi berbeda dari konfigurasi sekarang (mis. model dashboard diganti) —
// supaya retrieval tidak mati senyap akibat dimensi vektor tak cocok. Dipanggil di startup.
func BackfillEmbeddings() {
	if !EmbeddingEnabled() {
		return
	}
	sig := embSignature()
	var rows []models.Knowledge
	database.DB.Where("embedding = '' OR embedding IS NULL OR embedding_model IS NULL OR embedding_model <> ?", sig).Find(&rows)
	if len(rows) > 0 {
		log.Printf("Embedding: backfill/re-index %d knowledge (signature=%s)...", len(rows), sig)
		for i := range rows {
			IndexKnowledge(&rows[i])
		}
		log.Println("Embedding: backfill knowledge selesai")
	}
	BackfillProductEmbeddings()
}

// ProductItem = satu produk dengan vektor embedding ter-parse untuk hybrid retrieval katalog.
type ProductItem struct {
	P   models.Product
	Vec []float32
}

var (
	productCache = map[uint][]ProductItem{}
	productDirty = map[uint]bool{}
	productMu    sync.RWMutex
)

// InvalidateProducts menandai cache produk agent perlu dimuat ulang.
func InvalidateProducts(agentID uint) {
	productMu.Lock()
	productDirty[agentID] = true
	productMu.Unlock()
}

// ProductsFor mengembalikan katalog agent dari cache memori (embedding sudah di-parse).
func ProductsFor(agentID uint) []ProductItem {
	productMu.RLock()
	items, ok := productCache[agentID]
	dirty := productDirty[agentID]
	productMu.RUnlock()
	if ok && !dirty {
		return items
	}

	var rows []models.Product
	database.DB.Where("agent_id = ?", agentID).Order("id desc").Limit(200).Find(&rows)
	items = make([]ProductItem, 0, len(rows))
	for _, r := range rows {
		var vec []float32
		if r.Embedding != "" {
			_ = json.Unmarshal([]byte(r.Embedding), &vec)
		}
		r.Embedding = ""
		items = append(items, ProductItem{P: r, Vec: vec})
	}
	productMu.Lock()
	productCache[agentID] = items
	productDirty[agentID] = false
	productMu.Unlock()
	return items
}

func productEmbedText(p *models.Product) string {
	parts := []string{
		strings.TrimSpace(p.Name),
		strings.TrimSpace(p.ProductType),
		strings.TrimSpace(p.Price),
		strings.TrimSpace(p.Description),
		productDetailsText(p.DetailsJSON),
		strings.TrimSpace(p.Knowledge),
	}
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(part)
	}
	return b.String()
}

// IndexProduct menghitung embedding satu produk lalu menyimpannya ke DB + invalidate cache.
func IndexProduct(p *models.Product) {
	if p == nil || p.ID == 0 {
		return
	}
	InvalidateProducts(p.AgentID)
	if !EmbeddingEnabled() {
		return
	}
	text := productEmbedText(p)
	if strings.TrimSpace(text) == "" {
		return
	}
	vec, err := Embed(text)
	if err != nil {
		log.Printf("Embedding: gagal embed produk #%d: %v", p.ID, err)
		return
	}
	b, _ := json.Marshal(vec)
	p.Embedding = string(b)
	p.EmbeddingModel = embSignature()
	if err := database.DB.Model(p).Updates(map[string]any{
		"embedding":       p.Embedding,
		"embedding_model": p.EmbeddingModel,
	}).Error; err != nil {
		log.Printf("Embedding: gagal simpan embedding produk #%d: %v", p.ID, err)
		return
	}
	InvalidateProducts(p.AgentID)
}

// BackfillProductEmbeddings mengisi/re-index embedding katalog produk.
func BackfillProductEmbeddings() {
	if !EmbeddingEnabled() {
		return
	}
	sig := embSignature()
	var rows []models.Product
	database.DB.Where("embedding = '' OR embedding IS NULL OR embedding_model IS NULL OR embedding_model <> ?", sig).Find(&rows)
	if len(rows) == 0 {
		return
	}
	log.Printf("Embedding: backfill/re-index %d produk (signature=%s)...", len(rows), sig)
	for i := range rows {
		IndexProduct(&rows[i])
	}
	log.Println("Embedding: backfill produk selesai")
}
