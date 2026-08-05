package handlers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

const checkoutSessionTTL = 24 * time.Hour

type productButtonConfig struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Icon     string `json:"icon,omitempty"`
	Action   string `json:"action"` // checkout | ai | reply | handoff
	Response string `json:"response,omitempty"`
}

type checkoutStepConfig struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // text | number | select
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

func defaultProductButtons() []productButtonConfig {
	return []productButtonConfig{
		{Key: "order", Label: "Pesan Sekarang", Icon: "🛒", Action: "checkout"},
		{Key: "ask", Label: "Tanya Detail", Icon: "💬", Action: "ai"},
	}
}

func defaultCheckoutSteps() []checkoutStepConfig {
	return []checkoutStepConfig{
		{Key: "quantity", Label: "Mau pesan berapa?", Type: "number", Required: true},
		{Key: "customer_name", Label: "Pesanan ini atas nama siapa?", Type: "text", Required: true},
		{Key: "address", Label: "Kirim ke alamat lengkap mana?", Type: "text", Required: true},
		{Key: "note", Label: "Ada catatan untuk pesanan? Ketik *lewati* jika tidak ada.", Type: "text", Required: false},
	}
}

func parseProductButtons(p models.Product) []productButtonConfig {
	return parseProductButtonsJSON(p.ButtonsJSON)
}

func parseProductButtonsJSON(raw string) []productButtonConfig {
	var buttons []productButtonConfig
	if json.Unmarshal([]byte(raw), &buttons) != nil || len(buttons) == 0 {
		return defaultProductButtons()
	}
	return buttons
}

func productButtonDisplayText(button productButtonConfig) string {
	icon := strings.TrimSpace(button.Icon)
	if icon == "none" {
		return strings.TrimSpace(button.Label)
	}
	if icon == "" {
		switch button.Action {
		case "checkout":
			icon = "🛒"
		case "ai":
			icon = "💬"
		case "handoff":
			icon = "👤"
		default:
			icon = "ℹ️"
		}
	}
	return strings.TrimSpace(icon + " " + strings.TrimSpace(button.Label))
}

func parseCheckoutSteps(p models.Product) []checkoutStepConfig {
	var steps []checkoutStepConfig
	if json.Unmarshal([]byte(p.CheckoutStepsJSON), &steps) != nil || len(steps) == 0 {
		return defaultCheckoutSteps()
	}
	return steps
}

func validConfigKey(value string) bool {
	if value == "" || len(value) > 40 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_') {
			return false
		}
	}
	return true
}

func validateProductConfig(buttonsJSON, stepsJSON string) error {
	var buttons []productButtonConfig
	if err := json.Unmarshal([]byte(buttonsJSON), &buttons); err != nil || len(buttons) == 0 || len(buttons) > 3 {
		return fmt.Errorf("produk membutuhkan 1-3 tombol yang valid")
	}
	seen := map[string]bool{}
	validActions := map[string]bool{"checkout": true, "ai": true, "reply": true, "handoff": true}
	for _, button := range buttons {
		if !validConfigKey(button.Key) || seen[button.Key] || strings.TrimSpace(button.Label) == "" || len([]rune(button.Label)) > 24 || !validActions[button.Action] {
			return fmt.Errorf("konfigurasi tombol produk tidak valid")
		}
		if len([]rune(strings.TrimSpace(button.Icon))) > 8 {
			return fmt.Errorf("ikon tombol produk terlalu panjang")
		}
		if button.Action == "reply" && strings.TrimSpace(button.Response) == "" {
			return fmt.Errorf("tombol jawaban manual membutuhkan teks balasan")
		}
		seen[button.Key] = true
	}

	var steps []checkoutStepConfig
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil || len(steps) == 0 || len(steps) > 12 {
		return fmt.Errorf("checkout membutuhkan 1-12 langkah yang valid")
	}
	seen = map[string]bool{}
	for _, step := range steps {
		if !validConfigKey(step.Key) || seen[step.Key] || strings.TrimSpace(step.Label) == "" || len([]rune(step.Label)) > 500 {
			return fmt.Errorf("konfigurasi langkah checkout tidak valid")
		}
		if step.Type != "text" && step.Type != "number" && step.Type != "select" {
			return fmt.Errorf("jenis jawaban checkout tidak valid")
		}
		if step.Type == "select" && (len(step.Options) < 2 || len(step.Options) > 10) {
			return fmt.Errorf("langkah pilihan membutuhkan 2-10 opsi")
		}
		seen[step.Key] = true
	}
	return nil
}

type productInteractionResult struct {
	handled   bool
	reply     string
	buttons   []services.ReplyButton
	handoff   bool
	aiContext string
}

func inCheckoutContext(agentID uint, sender, actionID string) bool {
	if strings.HasPrefix(actionID, "product:") || strings.HasPrefix(actionID, "checkout:") {
		return true
	}
	var session models.ProductCheckoutSession
	if database.DB.Where("agent_id = ? AND sender = ? AND status IN ?", agentID, sender, []string{"collecting", "confirming"}).First(&session).Error != nil {
		return false
	}
	return time.Now().Before(session.ExpiresAt)
}

func clearProductCheckoutSession(agentID uint, sender string) {
	database.DB.Model(&models.ProductCheckoutSession{}).
		Where("agent_id = ? AND sender = ? AND status IN ?", agentID, sender, []string{"collecting", "confirming"}).
		Update("status", "cancelled")
}

func handleProductInteraction(agentID uint, sender, text, actionID string) productInteractionResult {
	if strings.HasPrefix(actionID, "product:") {
		return handleProductButton(agentID, sender, actionID)
	}
	if actionID != "" && !strings.HasPrefix(actionID, "checkout:") {
		return productInteractionResult{}
	}
	var session models.ProductCheckoutSession
	if database.DB.Where("agent_id = ? AND sender = ? AND status IN ?", agentID, sender, []string{"collecting", "confirming"}).First(&session).Error != nil {
		// Percakapan bebas diputuskan router AI dari konteks penuh. Hanya tombol
		// produk yang boleh memulai checkout langsung tanpa klasifikasi intent.
		return productInteractionResult{}
	}
	if time.Now().After(session.ExpiresAt) {
		database.DB.Model(&session).Update("status", "expired")
		return productInteractionResult{handled: true, reply: "Sesi checkout sebelumnya sudah berakhir. Silakan pilih *Pesan Sekarang* lagi untuk memulai."}
	}
	return handleCheckoutAnswer(session, text, actionID)
}

func handleProductButton(agentID uint, sender, actionID string) productInteractionResult {
	parts := strings.Split(actionID, ":")
	if len(parts) != 3 {
		return productInteractionResult{}
	}
	productID, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return productInteractionResult{}
	}
	var product models.Product
	var agent models.Agent
	if database.DB.Select("tenant_id").First(&agent, agentID).Error != nil ||
		database.DB.Where("tenant_id = ?", agent.TenantID).First(&product, uint(productID)).Error != nil {
		return productInteractionResult{handled: true, reply: "Maaf, produk ini sudah tidak tersedia."}
	}
	var selected *productButtonConfig
	for _, button := range parseProductButtons(product) {
		if button.Key == parts[2] {
			copyButton := button
			selected = &copyButton
			break
		}
	}
	if selected == nil {
		return productInteractionResult{handled: true, reply: "Pilihan produk tidak dikenali. Silakan kirim produknya kembali."}
	}
	markProductLead(agentID, sender, "warm")
	switch selected.Action {
	case "checkout":
		return startProductCheckoutForAgent(product, agentID, sender)
	case "reply":
		return productInteractionResult{handled: true, reply: selected.Response}
	case "handoff":
		markProductLead(agentID, sender, "hot")
		message := strings.TrimSpace(selected.Response)
		if message == "" {
			// Satu persona CS — tidak menyebut diteruskan ke orang lain.
			message = "Baik kak, saya cek dulu detail *" + product.Name + "* biar bantuannya pas. Mohon sebentar ya 🙏"
		}
		return productInteractionResult{handled: true, reply: message, handoff: true}
	default: // ai
		context := fmt.Sprintf("\n\nKONTEKS PRODUK YANG DIPILIH PELANGGAN:\nNama: %s\nHarga: %s\nDeskripsi: %s\nJawab hanya berdasarkan data produk ini dan basis pengetahuan. Jika fakta yang ditanya tidak tersedia, katakan bagian yang belum bisa dipastikan secara natural; jangan mengarang. Jangan pernah menyebut data, basis pengetahuan, knowledge, AI, bot, model, atau sistem kepada pelanggan. Berbicaralah sebagai staf bisnis yang sedang membantu pelanggan.", product.Name, product.Price, product.Description)
		return productInteractionResult{aiContext: context}
	}
}

func markProductLead(agentID uint, sender, stage string) {
	var contact models.Contact
	result := database.DB.Where("agent_id = ? AND number = ?", agentID, sender).First(&contact)
	now := time.Now()
	reason := map[string]string{
		"warm":     "Pelanggan berinteraksi dengan produk",
		"hot":      "Pelanggan memulai form atau proses checkout",
		"customer": "Pesanan berhasil dikonfirmasi",
	}[stage]
	if result.Error != nil {
		database.DB.Create(&models.Contact{AgentID: agentID, Number: sender, LeadStage: stage, LeadStageSource: "activity", LeadStageReason: reason, LeadStageUpdatedAt: &now})
		return
	}
	// Pilihan manual pengguna selalu menang sampai kunci dibuka dari menu Kontak.
	if contact.LeadStageLocked {
		return
	}
	// Jangan menurunkan customer atau hot menjadi warm.
	if contact.LeadStage == "customer" || (contact.LeadStage == "hot" && stage == "warm") {
		return
	}
	database.DB.Model(&contact).Updates(map[string]any{
		"lead_stage": stage, "lead_stage_source": "activity", "lead_stage_reason": reason,
		"lead_stage_confidence": 1, "lead_stage_updated_at": &now,
	})
}

func startProductCheckout(product models.Product, sender string) productInteractionResult {
	return startProductCheckoutForAgent(product, product.AgentID, sender)
}

func startProductCheckoutForAgent(product models.Product, runtimeAgentID uint, sender string) productInteractionResult {
	clearFlowSession(runtimeAgentID, sender)
	clearAIFormSession(runtimeAgentID, sender)
	markProductLead(runtimeAgentID, sender, "hot")
	now := time.Now()
	session := models.ProductCheckoutSession{
		TenantID: product.TenantID, AgentID: runtimeAgentID, Sender: sender, ProductID: product.ID,
		StepIndex: 0, DataJSON: "{}", Status: "collecting", ExpiresAt: now.Add(checkoutSessionTTL),
	}
	var existing models.ProductCheckoutSession
	if database.DB.Where("agent_id = ? AND sender = ?", runtimeAgentID, sender).First(&existing).Error == nil {
		session.ID = existing.ID
		session.CreatedAt = existing.CreatedAt
		database.DB.Model(&existing).Updates(map[string]any{
			"tenant_id": product.TenantID, "product_id": product.ID, "step_index": 0,
			"data_json": "{}", "status": "collecting", "expires_at": session.ExpiresAt,
		})
		session.ID = existing.ID
	} else {
		database.DB.Create(&session)
	}
	steps := parseCheckoutSteps(product)
	result := checkoutQuestion(session, product, steps[0], 0)
	result.reply = fmt.Sprintf("Siap kak, agar proses *%s* rapi dan tidak ada data yang terlewat, saya bantu lewat %d pertanyaan singkat ya.\n\n%s", product.Name, len(steps), result.reply)
	return result
}

func startProductOrderEdit(product models.Product, order models.ProductOrder, runtimeAgentID uint, sender string) productInteractionResult {
	clearFlowSession(runtimeAgentID, sender)
	clearAIFormSession(runtimeAgentID, sender)
	data := map[string]string{}
	_ = json.Unmarshal([]byte(order.DataJSON), &data)
	data["_edit_order_id"] = strconv.FormatUint(uint64(order.ID), 10)
	encoded, _ := json.Marshal(data)
	steps := parseCheckoutSteps(product)
	session := models.ProductCheckoutSession{
		TenantID: product.TenantID, AgentID: runtimeAgentID, Sender: sender, ProductID: product.ID,
		StepIndex: len(steps), DataJSON: string(encoded), Status: "confirming", ExpiresAt: time.Now().Add(checkoutSessionTTL),
	}
	var existing models.ProductCheckoutSession
	if database.DB.Where("agent_id = ? AND sender = ?", runtimeAgentID, sender).First(&existing).Error == nil {
		session.ID = existing.ID
		database.DB.Model(&existing).Updates(map[string]any{
			"tenant_id": product.TenantID, "product_id": product.ID, "step_index": len(steps),
			"data_json": session.DataJSON, "status": "confirming", "expires_at": session.ExpiresAt,
		})
	} else {
		database.DB.Create(&session)
	}
	result := checkoutConfirmation(session, product, steps)
	result.reply = "Tentu kak, pesanan yang sudah tersimpan bisa diperbarui. Saya tampilkan data terakhirnya dulu supaya kakak dapat memilih bagian yang ingin diubah.\n\n" + result.reply
	return result
}

func productHasCheckout(product models.Product) bool {
	for _, button := range parseProductButtons(product) {
		if button.Action == "checkout" {
			return true
		}
	}
	return false
}

func productCheckoutIntent(text string) bool {
	tokens := aiFormTokens(text)
	for _, token := range tokens {
		switch token {
		case "beli", "pesan", "order", "checkout", "booking", "daftar", "ambil", "sewa", "donasi", "sedekah", "jemput", "penjemputan":
			return true
		}
	}
	return false
}

func productTextScore(product models.Product, text string) int {
	query := aiFormTokens(text)
	name := aiFormTokens(product.Name)
	description := aiFormTokens(product.Description + " " + productDetailsForRouting(product.DetailsJSON) + " " + product.Knowledge)
	return aiFormOverlapScore(query, name, 6) + aiFormOverlapScore(query, description, 1)
}

// matchProductCheckout menangani niat yang sudah jelas tanpa menunggu model AI.
// Nama produk wajib cocok agar pertanyaan informasi biasa tidak langsung membuka form.
func matchProductCheckout(agentID uint, text string) (models.Product, bool) {
	if !productCheckoutIntent(text) {
		return models.Product{}, false
	}
	var products []models.Product
	database.DB.Where("agent_id = ?", agentID).Order("id desc").Limit(100).Find(&products)
	bestScore := 0
	var best models.Product
	for _, product := range products {
		if !productHasCheckout(product) {
			continue
		}
		if score := productTextScore(product, text); score > bestScore {
			best, bestScore = product, score
		}
	}
	return best, bestScore >= 6
}

// productCheckoutRoutingPrompt memungkinkan AI memilih checkout produk dari konteks
// percakapan lanjutan. Instruksi produk sengaja diletakkan sebelum Form AI umum.
func productCheckoutRoutingPrompt(agentID uint, sender, conversationText string) string {
	var products []models.Product
	database.DB.Where("agent_id = ?", agentID).Order("id desc").Limit(200).Find(&products)
	sort.SliceStable(products, func(i, j int) bool {
		return productTextScore(products[i], conversationText) > productTextScore(products[j], conversationText)
	})
	var sb strings.Builder
	count := 0
	for _, product := range products {
		if !productHasCheckout(product) {
			continue
		}
		if count == 0 {
			sb.WriteString("CHECKOUT PRODUK RESMI (PRIORITAS DI ATAS FORM AI UMUM):\n")
			sb.WriteString("- Gunakan [[START_PRODUCT:ID]] hanya saat niat pelanggan untuk benar-benar membeli, memesan, menyewa, berdonasi, atau memproses produk sudah cukup jelas. Jika masih bertanya atau membandingkan, jawab natural dan bila perlu ajukan satu klarifikasi halus.\n")
			sb.WriteString("- Gunakan [[EDIT_PRODUCT:ID]] hanya saat pelanggan ingin mengoreksi, mengganti, atau melengkapi pesanan produk yang sebelumnya sudah tersimpan.\n")
			sb.WriteString("- Jangan memicu checkout dari sapaan, pertanyaan informasi, atau minat yang belum jelas. Jika memilih directive, balas HANYA satu token. Jangan memilih Form AI umum untuk produk yang memiliki checkout resmi.\n")
		}
		if count >= 40 {
			break
		}
		sb.WriteString(fmt.Sprintf("- ID %d | %s", product.ID, strings.TrimSpace(product.Name)))
		if context := productRoutingSummary(product); context != "" {
			sb.WriteString(" | " + context)
		}
		sb.WriteString("\n")
		count++
	}
	var orders []models.ProductOrder
	database.DB.Preload("Product").Where("agent_id = ? AND sender = ?", agentID, sender).Order("id desc").Limit(8).Find(&orders)
	if len(orders) > 0 {
		sb.WriteString("PESANAN PELANGGAN INI YANG SUDAH TERSIMPAN (gunakan hanya untuk memahami intent edit):\n")
		for _, order := range orders {
			summary := strings.Join(strings.Fields(order.Summary), " ")
			if len([]rune(summary)) > 320 {
				summary = string([]rune(summary)[:320]) + "…"
			}
			sb.WriteString(fmt.Sprintf("- Produk ID %d | kode %s | %s | data: %s\n", order.ProductID, order.OrderCode, order.Product.Name, summary))
		}
	}
	return sb.String()
}

func productRoutingSummary(product models.Product) string {
	value := strings.TrimSpace(product.Description + " " + productDetailsForRouting(product.DetailsJSON) + " " + product.Knowledge)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 240 {
		value = string(runes[:240]) + "…"
	}
	return value
}

func productDetailsForRouting(raw string) string {
	var details []models.ProductDetail
	if json.Unmarshal([]byte(raw), &details) != nil {
		return ""
	}
	parts := make([]string, 0, len(details))
	for _, detail := range details {
		parts = append(parts, detail.Label+" "+detail.Value)
	}
	return strings.Join(parts, " ")
}

func productDirectiveID(reply string) (uint, bool, bool) {
	const prefix = "[[START_PRODUCT:"
	start := strings.Index(reply, prefix)
	if start < 0 {
		return 0, false, false
	}
	remainder := reply[start+len(prefix):]
	end := strings.Index(remainder, "]]")
	if end < 1 {
		return 0, true, false
	}
	id, err := strconv.ParseUint(strings.TrimSpace(remainder[:end]), 10, 64)
	if err != nil || id == 0 {
		return 0, true, false
	}
	return uint(id), true, true
}

func editProductDirectiveID(reply string) (uint, bool, bool) {
	const prefix = "[[EDIT_PRODUCT:"
	start := strings.Index(reply, prefix)
	if start < 0 {
		return 0, false, false
	}
	remainder := reply[start+len(prefix):]
	end := strings.Index(remainder, "]]")
	if end < 1 {
		return 0, true, false
	}
	id, err := strconv.ParseUint(strings.TrimSpace(remainder[:end]), 10, 64)
	return uint(id), true, err == nil && id > 0
}

func handleProductCheckoutDirective(agentID uint, sender, reply string) (productInteractionResult, bool) {
	if productID, found, valid := editProductDirectiveID(reply); found {
		if !valid {
			return productInteractionResult{handled: true, reply: "Maaf kak, pesanan yang ingin diubah belum dapat dikenali. Boleh sebutkan produknya?"}, true
		}
		var product models.Product
		var order models.ProductOrder
		if database.DB.Where("agent_id = ?", agentID).First(&product, productID).Error != nil || !productHasCheckout(product) ||
			database.DB.Where("agent_id = ? AND sender = ? AND product_id = ?", agentID, sender, productID).Order("id desc").First(&order).Error != nil {
			return productInteractionResult{handled: true, reply: "Saya belum menemukan pesanan lama untuk produk itu. Jika ingin membuat pesanan baru, sampaikan kebutuhannya ya kak."}, true
		}
		return startProductOrderEdit(product, order, agentID, sender), true
	}
	productID, found, valid := productDirectiveID(reply)
	if !found {
		return productInteractionResult{}, false
	}
	if !valid {
		return productInteractionResult{handled: true, reply: "Maaf, checkout produk belum bisa dimulai. Silakan sebutkan produknya sekali lagi."}, true
	}
	var product models.Product
	if database.DB.Where("agent_id = ?", agentID).First(&product, productID).Error != nil || !productHasCheckout(product) {
		return productInteractionResult{handled: true, reply: "Maaf, checkout produk tersebut sudah tidak tersedia."}, true
	}
	return startProductCheckout(product, sender), true
}

func startProductFromFreeCollection(agentID uint, sender, latestUser, conversationText, reply string) (productInteractionResult, bool) {
	if isGenericGreetingMessage(latestUser) || !replyRequestsStructuredData(reply) {
		return productInteractionResult{}, false
	}
	var products []models.Product
	database.DB.Where("agent_id = ?", agentID).Order("id desc").Limit(100).Find(&products)
	bestScore := 0
	var best models.Product
	combined := conversationText + "\n" + reply
	for _, product := range products {
		if !productHasCheckout(product) || !replyOverlapsCheckoutFields(product, reply) {
			continue
		}
		if score := productTextScore(product, combined); score > bestScore {
			best, bestScore = product, score
		}
	}
	if bestScore < 6 {
		return productInteractionResult{}, false
	}
	if messageHasEditIntent(latestUser) {
		var order models.ProductOrder
		if database.DB.Where("agent_id = ? AND sender = ? AND product_id = ?", agentID, sender, best.ID).Order("id desc").First(&order).Error == nil {
			return startProductOrderEdit(best, order, agentID, sender), true
		}
	}
	return startProductCheckoutForAgent(best, agentID, sender), true
}

func replyOverlapsCheckoutFields(product models.Product, reply string) bool {
	replyTokens := aiFormTokens(reply)
	for _, step := range parseCheckoutSteps(product) {
		if aiFormOverlapScore(replyTokens, aiFormTokens(step.Label), 1) > 0 {
			return true
		}
	}
	return false
}

func checkoutQuestion(session models.ProductCheckoutSession, product models.Product, step checkoutStepConfig, index int) productInteractionResult {
	prefix := fmt.Sprintf("Checkout *%s* · langkah %d/%d\n", product.Name, index+1, len(parseCheckoutSteps(product)))
	reply := prefix + strings.TrimSpace(step.Label) + "\n\nKetik *batal* untuk membatalkan"
	if index > 0 {
		reply += " · *kembali* untuk langkah sebelumnya"
	}
	buttons := []services.ReplyButton{}
	if step.Type == "select" && len(step.Options) <= 3 {
		for optionIndex, option := range step.Options {
			buttons = append(buttons, services.ReplyButton{
				ID: fmt.Sprintf("checkout:%d:option:%d", session.ID, optionIndex), Text: option,
			})
		}
	} else if step.Type == "select" {
		var lines []string
		for optionIndex, option := range step.Options {
			lines = append(lines, fmt.Sprintf("%d. %s", optionIndex+1, option))
		}
		reply = prefix + strings.TrimSpace(step.Label) + "\n\n" + strings.Join(lines, "\n") + "\n\nBalas nomor atau nama pilihannya. Ketik *batal* untuk membatalkan."
	}
	return productInteractionResult{handled: true, reply: reply, buttons: buttons}
}

func handleCheckoutAnswer(session models.ProductCheckoutSession, text, actionID string) productInteractionResult {
	var product models.Product
	if database.DB.Where("tenant_id = ?", session.TenantID).First(&product, session.ProductID).Error != nil {
		database.DB.Model(&session).Update("status", "cancelled")
		return productInteractionResult{handled: true, reply: "Produk checkout sudah tidak tersedia."}
	}
	steps := parseCheckoutSteps(product)
	normalized := strings.ToLower(strings.TrimSpace(text))
	if strings.HasSuffix(actionID, ":cancel") || normalized == "batal" || normalized == "cancel" {
		database.DB.Model(&session).Update("status", "cancelled")
		data := map[string]string{}
		_ = json.Unmarshal([]byte(session.DataJSON), &data)
		if data["_edit_order_id"] != "" {
			return productInteractionResult{handled: true, reply: "Baik kak, perubahan dibatalkan. Pesanan sebelumnya tetap tersimpan dan tidak berubah."}
		}
		return productInteractionResult{handled: true, reply: "Checkout *" + product.Name + "* dibatalkan. Data sementara sudah dihapus."}
	}
	if strings.HasSuffix(actionID, ":confirm") {
		return confirmProductOrder(session, product, steps)
	}
	if strings.HasSuffix(actionID, ":edit") {
		session.StepIndex = 0
		session.Status = "collecting"
		database.DB.Model(&session).Updates(map[string]any{"step_index": 0, "status": "collecting", "expires_at": time.Now().Add(checkoutSessionTTL)})
		return checkoutQuestion(session, product, steps[0], 0)
	}
	if normalized == "kembali" || normalized == "back" {
		if session.StepIndex > 0 {
			session.StepIndex--
			database.DB.Model(&session).Updates(map[string]any{"step_index": session.StepIndex, "status": "collecting", "expires_at": time.Now().Add(checkoutSessionTTL)})
			return checkoutQuestion(session, product, steps[session.StepIndex], session.StepIndex)
		}
		return checkoutQuestion(session, product, steps[0], 0)
	}
	if isGenericGreetingMessage(text) {
		if session.Status == "confirming" {
			return checkoutConfirmation(session, product, steps)
		}
		result := checkoutQuestion(session, product, steps[session.StepIndex], session.StepIndex)
		result.reply = "Halo kak 👋 checkout ini masih aktif. " + result.reply
		return result
	}
	if session.Status == "confirming" {
		return checkoutConfirmation(session, product, steps)
	}
	if session.StepIndex < 0 || session.StepIndex >= len(steps) {
		return checkoutConfirmation(session, product, steps)
	}
	step := steps[session.StepIndex]
	answer, valid := checkoutAnswerValue(step, text, actionID, session.ID)
	if !valid {
		result := checkoutQuestion(session, product, step, session.StepIndex)
		result.reply = "Jawaban belum sesuai. " + result.reply
		return result
	}
	data := map[string]string{}
	_ = json.Unmarshal([]byte(session.DataJSON), &data)
	data[step.Key] = answer
	encoded, _ := json.Marshal(data)
	session.StepIndex++
	session.DataJSON = string(encoded)
	session.ExpiresAt = time.Now().Add(checkoutSessionTTL)
	if session.StepIndex >= len(steps) {
		session.Status = "confirming"
		database.DB.Model(&session).Updates(map[string]any{"step_index": session.StepIndex, "data_json": session.DataJSON, "status": session.Status, "expires_at": session.ExpiresAt})
		return checkoutConfirmation(session, product, steps)
	}
	database.DB.Model(&session).Updates(map[string]any{"step_index": session.StepIndex, "data_json": session.DataJSON, "expires_at": session.ExpiresAt})
	return checkoutQuestion(session, product, steps[session.StepIndex], session.StepIndex)
}

func handleCheckoutImageAnswer(agentID uint, sender, answer, messageID string, needsHuman bool) (productInteractionResult, bool, bool) {
	var session models.ProductCheckoutSession
	if database.DB.Where("agent_id = ? AND sender = ? AND status = ?", agentID, sender, "collecting").First(&session).Error != nil || time.Now().After(session.ExpiresAt) {
		return productInteractionResult{}, false, false
	}
	var product models.Product
	if database.DB.Where("tenant_id = ?", session.TenantID).First(&product, session.ProductID).Error != nil {
		return productInteractionResult{}, false, false
	}
	steps := parseCheckoutSteps(product)
	if session.StepIndex < 0 || session.StepIndex >= len(steps) {
		return checkoutConfirmation(session, product, steps), true, false
	}
	step := steps[session.StepIndex]
	if _, valid := checkoutAnswerValue(step, answer, "", session.ID); !valid {
		result := checkoutQuestion(session, product, step, session.StepIndex)
		result.reply = "Isi foto belum dapat dijadikan jawaban yang sesuai. " + result.reply
		return result, true, false
	}
	data := map[string]string{}
	_ = json.Unmarshal([]byte(session.DataJSON), &data)
	if messageID != "" {
		data["_media_"+step.Key] = messageID
	}
	if needsHuman {
		data["_vision_needs_human"] = "true"
	}
	encoded, _ := json.Marshal(data)
	session.DataJSON = string(encoded)
	database.DB.Model(&session).Update("data_json", session.DataJSON)
	return handleCheckoutAnswer(session, answer, ""), true, true
}

func checkoutAnswerValue(step checkoutStepConfig, text, actionID string, sessionID uint) (string, bool) {
	value := strings.TrimSpace(text)
	if !step.Required && strings.EqualFold(value, "lewati") {
		return "", true
	}
	if strings.HasPrefix(actionID, fmt.Sprintf("checkout:%d:option:", sessionID)) {
		optionRaw := strings.TrimPrefix(actionID, fmt.Sprintf("checkout:%d:option:", sessionID))
		index, err := strconv.Atoi(optionRaw)
		if err == nil && index >= 0 && index < len(step.Options) {
			return step.Options[index], true
		}
	}
	if value == "" {
		return "", !step.Required
	}
	switch step.Type {
	case "number":
		n, err := strconv.Atoi(value)
		return value, err == nil && n > 0 && n <= 999
	case "select":
		if n, err := strconv.Atoi(value); err == nil && n >= 1 && n <= len(step.Options) {
			return step.Options[n-1], true
		}
		for _, option := range step.Options {
			if strings.EqualFold(strings.TrimSpace(option), value) {
				return option, true
			}
		}
		return "", false
	default:
		return value, len([]rune(value)) <= 1000
	}
}

func checkoutSummary(product models.Product, steps []checkoutStepConfig, dataJSON string) string {
	data := map[string]string{}
	_ = json.Unmarshal([]byte(dataJSON), &data)
	lines := []string{"*Ringkasan Pesanan*", "Produk: " + product.Name}
	if product.Price != "" {
		lines = append(lines, "Harga: "+product.Price)
	}
	for _, step := range steps {
		value := strings.TrimSpace(data[step.Key])
		if value == "" {
			continue
		}
		label := strings.TrimSpace(strings.TrimSuffix(step.Label, "?"))
		if data["_media_"+step.Key] != "" {
			value += " (foto terlampir)"
		}
		lines = append(lines, label+": "+value)
	}
	return strings.Join(lines, "\n")
}

func checkoutConfirmation(session models.ProductCheckoutSession, product models.Product, steps []checkoutStepConfig) productInteractionResult {
	summary := checkoutSummary(product, steps, session.DataJSON)
	return productInteractionResult{
		handled: true,
		reply:   summary + "\n\nPastikan datanya sudah benar.",
		buttons: []services.ReplyButton{
			{ID: fmt.Sprintf("checkout:%d:confirm", session.ID), Text: "Data sudah benar"},
			{ID: fmt.Sprintf("checkout:%d:edit", session.ID), Text: "Ubah data"},
			{ID: fmt.Sprintf("checkout:%d:cancel", session.ID), Text: "Batalkan"},
		},
	}
}

func confirmProductOrder(session models.ProductCheckoutSession, product models.Product, steps []checkoutStepConfig) productInteractionResult {
	if session.Status == "completed" {
		return productInteractionResult{handled: true, reply: "Pesanan ini sudah dikonfirmasi sebelumnya."}
	}
	data := map[string]string{}
	_ = json.Unmarshal([]byte(session.DataJSON), &data)
	if editID, err := strconv.ParseUint(data["_edit_order_id"], 10, 64); err == nil && editID > 0 {
		var existing models.ProductOrder
		if database.DB.Where("id = ? AND agent_id = ? AND sender = ? AND product_id = ?", uint(editID), session.AgentID, session.Sender, product.ID).First(&existing).Error != nil {
			return productInteractionResult{handled: true, reply: "Pesanan sebelumnya tidak ditemukan. Silakan batalkan lalu mulai pesanan baru."}
		}
		delete(data, "_edit_order_id")
		cleanJSON, _ := json.Marshal(data)
		summary := checkoutSummary(product, steps, string(cleanJSON))
		if err := database.DB.Model(&existing).Updates(map[string]any{"data_json": string(cleanJSON), "summary": summary, "status": "confirmed"}).Error; err != nil {
			return productInteractionResult{handled: true, reply: "Perubahan pesanan belum berhasil disimpan. Silakan tekan konfirmasi lagi."}
		}
		database.DB.Model(&session).Updates(map[string]any{"status": "completed", "data_json": string(cleanJSON)})
		markProductLead(session.AgentID, session.Sender, "customer")
		return productInteractionResult{
			handled: true,
			reply:   "Siap kak, pesanan *" + existing.OrderCode + "* sudah diperbarui sesuai informasi terbaru.",
			handoff: product.CheckoutHandoff || data["_vision_needs_human"] == "true",
		}
	}
	orderCode := fmt.Sprintf("ORD-%s-%06d", time.Now().Format("060102"), session.ID)
	summary := checkoutSummary(product, steps, session.DataJSON)
	order := models.ProductOrder{
		TenantID: product.TenantID, AgentID: session.AgentID, ProductID: product.ID, Sender: session.Sender,
		OrderCode: orderCode, Status: "confirmed", DataJSON: session.DataJSON, Summary: summary,
	}
	if err := database.DB.Create(&order).Error; err != nil {
		return productInteractionResult{handled: true, reply: "Maaf, pesanan belum berhasil disimpan. Silakan tekan Konfirmasi lagi."}
	}
	database.DB.Model(&session).Update("status", "completed")
	markProductLead(product.AgentID, session.Sender, "customer")
	message := strings.TrimSpace(product.CheckoutSuccessMessage)
	if message == "" {
		message = "Pesanan *{order_code}* berhasil dicatat. Terima kasih, CS kami akan memeriksa dan melanjutkan pesanan ini."
	}
	message = strings.ReplaceAll(message, "{order_code}", orderCode)
	message = strings.ReplaceAll(message, "{product}", product.Name)
	if product.CheckoutHandoff || data["_vision_needs_human"] == "true" {
		return productInteractionResult{handled: true, reply: message, handoff: true}
	}
	return productInteractionResult{handled: true, reply: message}
}

func ListProductOrders(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	var orders []models.ProductOrder
	database.DB.Preload("Product").Where("agent_id = ?", agentID).Order("id desc").Limit(100).Find(&orders)
	c.JSON(200, gin.H{"data": orders})
}
