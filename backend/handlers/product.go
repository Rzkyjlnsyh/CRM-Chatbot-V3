package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

func ListProducts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var products []models.Product
	database.DB.Where("agent_id = ?", id).Order("id desc").Find(&products)
	attachProductImageURLs(c, id, products)
	c.JSON(200, gin.H{"data": products})
}

func attachProductImageURLs(c *gin.Context, agentID uint, products []models.Product) {
	token := issueMediaToken(currentTenantID(c))
	if token == "" {
		return
	}
	for i := range products {
		if products[i].ImagePath != "" {
			products[i].ImageURL = fmt.Sprintf("/api/agents/%d/products/%d/image?token=%s", agentID, products[i].ID, token)
		}
	}
}

func ServeProductImage(c *gin.Context) {
	tid, ok := tenantFromToken(c.Query("token"))
	if !ok {
		c.AbortWithStatus(401)
		return
	}
	agentID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	var agent models.Agent
	if database.DB.Select("id").Where("id = ? AND tenant_id = ?", agentID, tid).First(&agent).Error != nil {
		c.AbortWithStatus(404)
		return
	}
	var product models.Product
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("pid"), agentID).First(&product).Error != nil || product.ImagePath == "" {
		c.AbortWithStatus(404)
		return
	}
	c.File(product.ImagePath)
}

func productBroadcastPayload(agentID uint, productIDRaw, message string) (string, string, string, string, string, error) {
	productIDRaw = strings.TrimSpace(productIDRaw)
	if productIDRaw == "" || productIDRaw == "0" {
		return message, "", "", "", "", nil
	}
	productID, err := strconv.ParseUint(productIDRaw, 10, 64)
	if err != nil || productID == 0 {
		return "", "", "", "", "", fmt.Errorf("Produk tidak valid")
	}
	var product models.Product
	if database.DB.Where("agent_id = ?", agentID).First(&product, uint(productID)).Error != nil {
		return "", "", "", "", "", fmt.Errorf("Produk tidak ditemukan")
	}
	if strings.TrimSpace(message) == "" {
		message = buildProductCaption(product)
	}
	if strings.TrimSpace(product.ImagePath) == "" {
		return message, "", "", "", "", nil
	}
	data, err := os.ReadFile(product.ImagePath)
	if err != nil {
		return "", "", "", "", "", fmt.Errorf("Gambar produk tidak terbaca")
	}
	mime := strings.TrimSpace(product.ImageMime)
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	fileName := filepath.Base(product.ImagePath)
	mediaPath := storeMedia(agentID, data, mime, fileName)
	if mediaPath == "" {
		return "", "", "", "", "", fmt.Errorf("Gagal menyiapkan gambar produk")
	}
	return message, "image", mediaPath, fileName, mime, nil
}

func productBroadcastButtons(agentID uint, productIDRaw string) (uint, string, error) {
	productIDRaw = strings.TrimSpace(productIDRaw)
	if productIDRaw == "" || productIDRaw == "0" {
		return 0, "", nil
	}
	productID, err := strconv.ParseUint(productIDRaw, 10, 64)
	if err != nil || productID == 0 {
		return 0, "", fmt.Errorf("Produk tidak valid")
	}
	var product models.Product
	if database.DB.Where("agent_id = ?", agentID).First(&product, uint(productID)).Error != nil {
		return 0, "", fmt.Errorf("Produk tidak ditemukan")
	}
	buttons := parseProductButtons(product)
	encoded, err := json.Marshal(buttons)
	if err != nil {
		return 0, "", fmt.Errorf("Tombol produk tidak valid")
	}
	return product.ID, string(encoded), nil
}

func CreateProduct(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	tid := currentTenantID(c)

	name := strings.TrimSpace(c.PostForm("name"))
	productType := normalizeProductType(c.PostForm("product_type"))
	price := strings.TrimSpace(c.PostForm("price"))
	description := strings.TrimSpace(c.PostForm("description"))
	detailsJSON, detailsErr := normalizeProductDetails(c.PostForm("details_json"))
	if detailsErr != nil {
		c.JSON(400, gin.H{"error": detailsErr.Error()})
		return
	}
	knowledge := strings.TrimSpace(c.PostForm("knowledge"))
	aiSalesGuidance := strings.TrimSpace(c.PostForm("ai_sales_guidance"))
	buttonsJSON, stepsJSON, checkoutHandoff, successMessage, configErr := productConfigFromForm(c, nil)
	if configErr != nil {
		c.JSON(400, gin.H{"error": configErr.Error()})
		return
	}

	if name == "" {
		c.JSON(400, gin.H{"error": "Nama produk wajib diisi"})
		return
	}
	if len([]rune(knowledge)) > 20000 || len([]rune(aiSalesGuidance)) > 8000 {
		c.JSON(400, gin.H{"error": "Knowledge produk atau arahan AI terlalu panjang"})
		return
	}

	p := models.Product{
		TenantID: tid, AgentID: id,
		Name: name, ProductType: productType, Price: price, Description: description, DetailsJSON: detailsJSON,
		Knowledge: knowledge, AISalesGuidance: aiSalesGuidance,
		ButtonsJSON: buttonsJSON, CheckoutStepsJSON: stepsJSON,
		CheckoutHandoff: checkoutHandoff, CheckoutSuccessMessage: successMessage,
	}

	// Handle image upload
	if file, header, err := c.Request.FormFile("image"); err == nil {
		defer file.Close()
		data, readErr := io.ReadAll(file)
		if readErr != nil {
			c.JSON(400, gin.H{"error": "Gagal membaca gambar"})
			return
		}
		mime := http.DetectContentType(data)
		if !strings.HasPrefix(mime, "image/") {
			c.JSON(400, gin.H{"error": "File harus berupa gambar (jpg, png, webp)"})
			return
		}
		filename := fmt.Sprintf("product_%d_%d%s", id, time.Now().UnixNano(), filepath.Ext(header.Filename))
		path := filepath.Join("data", "products", filename)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			c.JSON(500, gin.H{"error": "Gagal menyimpan gambar"})
			return
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			c.JSON(500, gin.H{"error": "Gagal menyimpan gambar"})
			return
		}
		p.ImagePath = path
		p.ImageMime = mime
	}

	if err := database.DB.Create(&p).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan produk"})
		return
	}
	services.IndexProduct(&p)
	c.JSON(201, gin.H{"data": p})
}

// GenerateProductAIContent menghasilkan draft saja. Pengguna tetap meninjau dan
// menekan Simpan Produk sehingga AI tidak dapat mengubah katalog secara otomatis.
func GenerateProductAIContent(c *gin.Context) {
	if _, ok := resolveAgent(c); !ok {
		return
	}
	var req struct {
		Name              string `json:"name"`
		ProductType       string `json:"product_type"`
		Price             string `json:"price"`
		Description       string `json:"description"`
		DetailsJSON       string `json:"details_json"`
		ExistingKnowledge string `json:"existing_knowledge"`
		CheckoutEnabled   bool   `json:"checkout_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Data produk tidak valid"})
		return
	}
	parts := []string{}
	if value := strings.TrimSpace(req.Name); value != "" {
		parts = append(parts, "Nama produk: "+value)
	}
	parts = append(parts, "Jenis produk: "+productTypeLabel(normalizeProductType(req.ProductType)))
	if value := strings.TrimSpace(req.Price); value != "" {
		parts = append(parts, "Harga: "+value)
	}
	if value := strings.TrimSpace(req.Description); value != "" {
		parts = append(parts, "Deskripsi dan fakta sumber:\n"+value)
	}
	if detailsJSON, err := normalizeProductDetails(req.DetailsJSON); err == nil {
		var details []models.ProductDetail
		_ = json.Unmarshal([]byte(detailsJSON), &details)
		lines := []string{}
		for _, detail := range details {
			if detail.Label != "" && detail.Value != "" {
				lines = append(lines, "- "+detail.Label+": "+detail.Value)
			}
		}
		if len(lines) > 0 {
			parts = append(parts, "Informasi terstruktur:\n"+strings.Join(lines, "\n"))
		}
	}
	if value := strings.TrimSpace(req.ExistingKnowledge); value != "" {
		parts = append(parts, "Fakta tambahan yang sudah ditulis pengguna:\n"+value)
	}
	source := strings.Join(parts, "\n\n")
	if strings.TrimSpace(req.Name) == "" || len([]rune(source)) < 30 {
		c.JSON(400, gin.H{"error": "Isi nama dan deskripsi produk terlebih dahulu agar AI memiliki fakta yang cukup"})
		return
	}
	if runes := []rune(source); len(runes) > 12000 {
		source = string(runes[:12000])
	}
	knowledge, guidance, err := services.GenerateProductAIContent(source, req.CheckoutEnabled)
	if err != nil {
		log.Printf("Generate knowledge produk gagal: %v", err)
		c.JSON(502, gin.H{"error": "AI belum bisa membuat knowledge produk. Periksa konfigurasi OpenRouter lalu coba lagi."})
		return
	}
	c.JSON(200, gin.H{"knowledge": knowledge, "ai_sales_guidance": guidance})
}

func UpdateProduct(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var p models.Product
	if database.DB.Where("agent_id = ?", aid).First(&p, c.Param("pid")).Error != nil {
		c.JSON(404, gin.H{"error": "Produk tidak ditemukan"})
		return
	}

	name := strings.TrimSpace(c.PostForm("name"))
	productType := normalizeProductType(c.PostForm("product_type"))
	price := strings.TrimSpace(c.PostForm("price"))
	description := strings.TrimSpace(c.PostForm("description"))
	detailsJSON, detailsErr := normalizeProductDetails(c.PostForm("details_json"))
	if detailsErr != nil {
		c.JSON(400, gin.H{"error": detailsErr.Error()})
		return
	}
	knowledge := strings.TrimSpace(c.PostForm("knowledge"))
	aiSalesGuidance := strings.TrimSpace(c.PostForm("ai_sales_guidance"))
	buttonsJSON, stepsJSON, checkoutHandoff, successMessage, configErr := productConfigFromForm(c, &p)
	if configErr != nil {
		c.JSON(400, gin.H{"error": configErr.Error()})
		return
	}

	if name != "" {
		p.Name = name
	}
	p.ProductType = productType
	if len([]rune(knowledge)) > 20000 || len([]rune(aiSalesGuidance)) > 8000 {
		c.JSON(400, gin.H{"error": "Knowledge produk atau arahan AI terlalu panjang"})
		return
	}
	p.Price = price
	p.Description = description
	p.DetailsJSON = detailsJSON
	p.Knowledge = knowledge
	p.AISalesGuidance = aiSalesGuidance
	p.ButtonsJSON = buttonsJSON
	p.CheckoutStepsJSON = stepsJSON
	p.CheckoutHandoff = checkoutHandoff
	p.CheckoutSuccessMessage = successMessage

	// Handle new image upload (ganti gambar lama)
	if file, header, err := c.Request.FormFile("image"); err == nil {
		defer file.Close()
		data, readErr := io.ReadAll(file)
		if readErr != nil {
			c.JSON(400, gin.H{"error": "Gagal membaca gambar"})
			return
		}
		mime := http.DetectContentType(data)
		filename := fmt.Sprintf("product_%d_%d%s", aid, time.Now().UnixNano(), filepath.Ext(header.Filename))
		path := filepath.Join("data", "products", filename)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			c.JSON(500, gin.H{"error": "Gagal menyimpan gambar"})
			return
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			c.JSON(500, gin.H{"error": "Gagal menyimpan gambar"})
			return
		}
		// Hapus gambar lama
		if p.ImagePath != "" {
			_ = os.Remove(p.ImagePath)
		}
		p.ImagePath = path
		p.ImageMime = mime
	}

	if err := database.DB.Save(&p).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan produk"})
		return
	}
	services.IndexProduct(&p)
	c.JSON(200, gin.H{"data": p})
}

func DeleteProduct(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var p models.Product
	if database.DB.Where("agent_id = ?", aid).First(&p, c.Param("pid")).Error != nil {
		c.JSON(404, gin.H{"error": "Produk tidak ditemukan"})
		return
	}
	if p.ImagePath != "" {
		_ = os.Remove(p.ImagePath)
	}
	if err := database.DB.Delete(&p).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menghapus produk"})
		return
	}
	services.InvalidateProducts(aid)
	c.JSON(200, gin.H{"message": "Produk dihapus"})
}

// SendProduct mengirim gambar produk + tombol interaktif ke nomor tujuan via WA.
func SendProduct(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var p models.Product
	if database.DB.Where("agent_id = ?", aid).First(&p, c.Param("pid")).Error != nil {
		c.JSON(404, gin.H{"error": "Produk tidak ditemukan"})
		return
	}

	var req struct {
		To string `json:"to"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.To == "" {
		c.JSON(400, gin.H{"error": "Nomor tujuan wajib diisi"})
		return
	}

	to := services.NormalizePhone(req.To)
	if to == "" {
		c.JSON(400, gin.H{"error": "Format nomor tidak valid"})
		return
	}

	wa := services.WA(aid)
	if !wa.IsConnected() {
		c.JSON(400, gin.H{"error": "WhatsApp belum tersambung"})
		return
	}

	caption := buildProductCaption(p)

	// 1. Kirim gambar produk dulu
	if p.ImagePath != "" {
		data, err := os.ReadFile(p.ImagePath)
		if err != nil {
			log.Printf("SendProduct: gagal baca gambar %s: %v", p.ImagePath, err)
		} else {
			mime := p.ImageMime
			if mime == "" {
				mime = "image/jpeg"
			}
			if err := wa.SendImage(to, caption, mime, data); err != nil {
				c.JSON(500, gin.H{"error": "Gagal mengirim gambar: " + err.Error()})
				return
			}
		}
	} else {
		if err := wa.SendText(to, caption); err != nil {
			c.JSON(500, gin.H{"error": "Gagal mengirim pesan"})
			return
		}
	}

	// 2. Kirim pilihan sebagai tombol Native Flow. Jika WhatsApp menolak tipe
	// interaktif, tetap berikan pilihan lewat teks agar pelanggan tidak buntu.
	configuredButtons := parseProductButtons(p)
	buttonBody := "Pilih tindakan untuk produk ini.\nJika tombol tidak terlihat, balas dengan nama pilihannya."
	buttons := make([]services.ReplyButton, 0, len(configuredButtons))
	for _, button := range configuredButtons {
		buttons = append(buttons, services.ReplyButton{
			ID: fmt.Sprintf("product:%d:%s", p.ID, button.Key), Text: productButtonDisplayText(button),
		})
	}
	if err := wa.SendButtons(to, buttonBody, "Pilih salah satu", buttons); err != nil {
		log.Printf("SendProduct: tombol interaktif gagal ke %s, pakai teks: %v", to, err)
		fallbackLines := []string{"Pilih tindakan untuk produk ini:"}
		for index, button := range configuredButtons {
			fallbackLines = append(fallbackLines, fmt.Sprintf("%d. %s", index+1, productButtonDisplayText(button)))
		}
		fallback := strings.Join(fallbackLines, "\n")
		if fallbackErr := wa.SendText(to, fallback); fallbackErr != nil {
			log.Printf("SendProduct: CTA teks gagal ke %s: %v", to, fallbackErr)
			c.JSON(500, gin.H{"error": "Produk terkirim, tetapi pilihan tindakan gagal dikirim"})
			return
		}
	}

	logTurn(aid, to, "", caption, true, "", "")
	c.JSON(200, gin.H{"message": "Produk terkirim"})
}

func normalizeProductType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "physical", "digital", "service", "subscription", "event", "donation", "other":
		return value
	default:
		return "physical"
	}
}

func productTypeLabel(value string) string {
	switch normalizeProductType(value) {
	case "digital":
		return "Produk digital"
	case "service":
		return "Jasa atau layanan"
	case "subscription":
		return "Langganan atau membership"
	case "event":
		return "Acara atau kelas"
	case "donation":
		return "Donasi atau penjemputan"
	case "other":
		return "Lainnya"
	default:
		return "Produk fisik"
	}
}

func normalizeProductDetails(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]", nil
	}
	var details []models.ProductDetail
	if err := json.Unmarshal([]byte(raw), &details); err != nil || len(details) > 30 {
		return "", fmt.Errorf("Informasi terstruktur produk tidak valid")
	}
	out := make([]models.ProductDetail, 0, len(details))
	seen := map[string]bool{}
	for _, detail := range details {
		label := strings.TrimSpace(detail.Label)
		value := strings.TrimSpace(detail.Value)
		if label == "" || value == "" {
			continue
		}
		if len([]rune(label)) > 80 || len([]rune(value)) > 2000 {
			return "", fmt.Errorf("Label atau isi informasi produk terlalu panjang")
		}
		key := strings.ToLower(label)
		if seen[key] {
			return "", fmt.Errorf("Label informasi produk tidak boleh duplikat")
		}
		seen[key] = true
		out = append(out, models.ProductDetail{Label: label, Value: value})
	}
	encoded, _ := json.Marshal(out)
	return string(encoded), nil
}

func productConfigFromForm(c *gin.Context, existing *models.Product) (buttonsJSON, stepsJSON string, handoff bool, successMessage string, err error) {
	buttonsJSON = strings.TrimSpace(c.PostForm("buttons_json"))
	stepsJSON = strings.TrimSpace(c.PostForm("checkout_steps_json"))
	successMessage = strings.TrimSpace(c.PostForm("checkout_success_message"))
	if buttonsJSON == "" && existing != nil && existing.ButtonsJSON != "" {
		buttonsJSON = existing.ButtonsJSON
	}
	if stepsJSON == "" && existing != nil && existing.CheckoutStepsJSON != "" {
		stepsJSON = existing.CheckoutStepsJSON
	}
	if buttonsJSON == "" {
		encoded, _ := json.Marshal(defaultProductButtons())
		buttonsJSON = string(encoded)
	}
	if stepsJSON == "" {
		encoded, _ := json.Marshal(defaultCheckoutSteps())
		stepsJSON = string(encoded)
	}
	handoff = true
	if raw := strings.TrimSpace(c.PostForm("checkout_handoff")); raw != "" {
		if parsed, parseErr := strconv.ParseBool(raw); parseErr == nil {
			handoff = parsed
		}
	} else if existing != nil {
		handoff = existing.CheckoutHandoff
	}
	if successMessage == "" && existing != nil {
		successMessage = existing.CheckoutSuccessMessage
	}
	if configErr := validateProductConfig(buttonsJSON, stepsJSON); configErr != nil {
		err = configErr
	}
	return
}

func buildProductCaption(p models.Product) string {
	var sb strings.Builder
	sb.WriteString("*" + p.Name + "*")
	if p.Price != "" {
		sb.WriteString("\n" + p.Price)
	}
	if p.Description != "" {
		sb.WriteString("\n\n" + p.Description)
	}
	return sb.String()
}
