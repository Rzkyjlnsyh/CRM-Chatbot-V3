package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"
)

// maybeExtractAndExportClosing dijalankan async setelah AI membalas customer.
// Mengecek apakah agent punya closing form + sheet sync enabled,
// lalu memanggil AI extractor untuk mendapatkan data terstruktur,
// validasi, simpan ke DB, dan append ke Google Sheets.
func maybeExtractAndExportClosing(agentID uint, sender string) {
	var agent models.Agent
	if database.DB.First(&agent, agentID).Error != nil {
		return
	}
	if !agent.SheetSyncEnabled || agent.SpreadsheetURL == "" {
		return
	}

	var form models.ClosingForm
	if database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).First(&form).Error != nil {
		return
	}

	sheetID := services.ParseSpreadsheetID(agent.SpreadsheetURL)
	if sheetID == "" {
		log.Printf("[closing] Agent %d: URL spreadsheet tidak valid", agentID)
		return
	}
	sheetName := agent.SpreadsheetSheetName
	if sheetName == "" {
		sheetName = "Leads"
	}

	// Bangun prompt extractor dari schema + summary + chat history, lalu jalankan AI extractor.
	result, err := extractClosingData(buildExtractorPrompt(agentID, sender, agent, form))
	if err != nil {
		log.Printf("[closing] Agent %d: extractor gagal: %v", agentID, err)
		return
	}
	// Nomor WA pelanggan = nomor pengirim chat (otomatis). Isi field bertipe phone yang kosong
	// supaya tak bergantung pada pelanggan mengetik nomornya.
	fillPhoneFromSender(form.SchemaJSON, result.Data, sender)

	if result.Confidence < closingMinConfidence {
		log.Printf("[closing] Agent %d: confidence rendah %.2f, skip", agentID, result.Confidence)
		return
	}

	// Validasi required fields.
	if !validateRequiredFields(form.SchemaJSON, result.Data) {
		log.Printf("[closing] Agent %d: required field belum lengkap", agentID)
		return
	}

	dataJSON, _ := json.Marshal(result.Data)
	summaryJSON, _ := json.Marshal(result)
	rowVals := buildSheetRow(form.SchemaJSON, result.Data, agent, sender)

	// UPSERT: 1 baris per pelanggan per sesi order. Kalau pelanggan ini sudah punya order dalam
	// ~12 jam terakhir (order sama yang berkembang), UPDATE baris itu. Kalau tidak, order baru -> baris baru.
	cutoff := time.Now().Add(-12 * time.Hour)
	var existing models.ClosingRecord
	if database.DB.Where("agent_id = ? AND sender = ? AND created_at >= ?", agentID, sender, cutoff).
		Order("id desc").First(&existing).Error == nil {
		if existing.DataJSON == string(dataJSON) {
			return // tidak ada perubahan -> tak perlu tulis ulang
		}
		database.DB.Model(&existing).Updates(map[string]any{
			"data_json": string(dataJSON), "raw_summary": string(summaryJSON), "confidence": result.Confidence,
		})
		if existing.SheetRow > 0 {
			if err := services.UpdateRow(sheetID, sheetName, existing.SheetRow, rowVals); err != nil {
				log.Printf("[closing] Agent %d: gagal update baris sheet %d: %v", agentID, existing.SheetRow, err)
			} else {
				log.Printf("[closing] Agent %d: order pelanggan %s diperbarui (baris %d)", agentID, sender, existing.SheetRow)
			}
		}
		return
	}

	// Order baru -> buat record + append baris baru, simpan nomor barisnya untuk update berikutnya.
	rec := models.ClosingRecord{
		AgentID:        agentID,
		Sender:         sender,
		Status:         "detected",
		Confidence:     result.Confidence,
		DataJSON:       string(dataJSON),
		RawSummary:     string(summaryJSON),
		IdempotencyKey: fmt.Sprintf("%d-%s-%d", agentID, sender, time.Now().UnixNano()),
	}
	database.DB.Create(&rec)

	go func() {
		defer services.RecoverGo("closingSheetExport")
		row, sheetErr := services.AppendRow(sheetID, sheetName, rowVals)
		now := time.Now()
		updates := map[string]any{"exported_at": &now}
		if sheetErr != nil {
			log.Printf("[closing] Agent %d: gagal append sheet: %v", agentID, sheetErr)
			updates["status"], updates["sheet_error"] = "failed", sheetErr.Error()
		} else {
			updates["status"], updates["sheet_row"] = "exported", row
			log.Printf("[closing] Agent %d: order baru pelanggan %s tercatat (baris %d)", agentID, sender, row)
		}
		database.DB.Model(&rec).Updates(updates)
	}()
}

// ClosingResult = output AI extractor.
type ClosingResult struct {
	Confidence float64                `json:"confidence"`
	Data       map[string]interface{} `json:"data"`
}

const closingExtractorSystem = `Kamu adalah data extractor order. Baca SELURUH percakapan dan ekstrak data sesuai schema.
Aturan:
- Ambil nama pelanggan & produk dari MANA SAJA di percakapan (sering disebut di awal), bukan cuma pesan terakhir.
- Jika pelanggan memesan BEBERAPA item, gabungkan semua item ke field produk, pisahkan dengan koma.
- Beri "confidence" TINGGI (>=0.8) bila ada nama pelanggan DAN minimal satu produk yang jelas, walaupun harga/tanggal belum disebut.
- Beri confidence rendah (<0.5) HANYA bila percakapan jelas belum ada niat order atau belum ada produk.
- Kalau ada field yang belum diketahui, tetap sertakan field lain yang ada. Jangan menambah field di luar schema.
Output HANYA JSON: {"confidence": 0.0-1.0, "data": {...}}.`

// extractClosingData menjalankan AI extractor pada satu prompt dan mengembalikan hasil terstruktur.
// Mencoba hingga 3x karena model kadang flaky membalas kosong / JSON tak lengkap.
func extractClosingData(prompt string) (*ClosingResult, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := services.CreateAICompletion(
			context.Background(),
			[]openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: closingExtractorSystem},
				{Role: openai.ChatMessageRoleUser, Content: prompt},
			},
			3000,
			0.2,
		)
		if err != nil {
			lastErr = err
			continue
		}
		if len(resp.Choices) == 0 {
			lastErr = fmt.Errorf("extractor tidak mengembalikan jawaban")
			continue
		}
		content := strings.TrimSpace(resp.Choices[0].Message.Content)
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
		if content == "" {
			lastErr = fmt.Errorf("extractor balas kosong (finish=%s)", resp.Choices[0].FinishReason)
			log.Printf("[closing] extractor kosong, retry %d/3 (finish=%s)", attempt, resp.Choices[0].FinishReason)
			continue
		}
		var result ClosingResult
		if err := json.Unmarshal([]byte(content), &result); err != nil {
			lastErr = fmt.Errorf("parse JSON gagal (len=%d): %w", len(content), err)
			log.Printf("[closing] parse gagal, retry %d/3: %v", attempt, err)
			continue
		}
		return &result, nil
	}
	return nil, lastErr
}

// closingMinConfidence = ambang minimum keyakinan extractor. Cukup 0.6 karena required-field
// (nama+produk+nomor) sudah jadi penjaga utama; 0.7 dulu kelewat ketat & melewatkan order asli.
const closingMinConfidence = 0.6

// fillPhoneFromSender mengisi field bertipe "phone" yang masih kosong dengan nomor pengirim,
// karena nomor WhatsApp pelanggan otomatis diketahui dari chat.
func fillPhoneFromSender(schemaJSON string, data map[string]interface{}, sender string) {
	if data == nil || strings.TrimSpace(sender) == "" {
		return
	}
	var schema struct {
		Fields []struct {
			Key  string `json:"key"`
			Type string `json:"type"`
		} `json:"fields"`
	}
	if json.Unmarshal([]byte(schemaJSON), &schema) != nil {
		return
	}
	for _, f := range schema.Fields {
		if f.Type != "phone" {
			continue
		}
		if v, ok := data[f.Key]; !ok || v == nil || v == "" {
			data[f.Key] = sender
		}
	}
}

// orderIntentKeywords = sinyal kasar bahwa percakapan menuju order (untuk hemat token di simulator).
var orderIntentKeywords = []string{
	"pesan", "order", "beli", "checkout", "ambil", "booking", "dp", "transfer",
	"nama", "wa", "whatsapp", "alamat", "atas nama", "no hp", "nomor",
}

func looksLikeOrderIntent(text string) bool {
	t := strings.ToLower(text)
	for _, k := range orderIntentKeywords {
		if strings.Contains(t, k) {
			return true
		}
	}
	return false
}

func buildConvoText(history []models.ChatHistory, latestUser, latestReply string) string {
	var sb strings.Builder
	for _, h := range history {
		if strings.TrimSpace(h.Message) != "" {
			sb.WriteString("Customer: " + h.Message + "\n")
		}
		if strings.TrimSpace(h.Reply) != "" {
			sb.WriteString("AI: " + h.Reply + "\n")
		}
	}
	if strings.TrimSpace(latestUser) != "" {
		sb.WriteString("Customer: " + latestUser + "\n")
	}
	if strings.TrimSpace(latestReply) != "" {
		sb.WriteString("AI: " + latestReply + "\n")
	}
	return sb.String()
}

func buildExtractorPromptFromConvo(form models.ClosingForm, convo string) string {
	var sb strings.Builder
	sb.WriteString("Schema data yang harus diekstrak:\n")
	sb.WriteString(form.SchemaJSON)
	sb.WriteString("\n\nPercakapan:\n")
	sb.WriteString(convo)
	sb.WriteString("\nEkstrak data sesuai schema di atas. Output JSON: {\"confidence\": 0.0-1.0, \"data\": {...}}")
	return sb.String()
}

// missingRequiredFields mengembalikan label/key field wajib yang masih kosong.
func missingRequiredFields(schemaJSON string, data map[string]interface{}) []string {
	var schema struct {
		Fields []struct {
			Key      string `json:"key"`
			Label    string `json:"label"`
			Required bool   `json:"required"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return nil
	}
	var missing []string
	for _, f := range schema.Fields {
		if !f.Required {
			continue
		}
		if val, ok := data[f.Key]; !ok || val == nil || val == "" {
			label := f.Label
			if label == "" {
				label = f.Key
			}
			missing = append(missing, label)
		}
	}
	return missing
}

// previewClosing menjalankan extractor closing sebagai DRY-RUN (tanpa simpan DB / tulis Sheets),
// dipakai simulator "Coba Chat" agar user bisa menguji deteksi order tanpa WhatsApp asli.
// Mengembalikan nil bila tak ada closing form aktif atau belum ada sinyal order (hemat token).
func previewClosing(agentID uint, agent models.Agent, history []models.ChatHistory, latestUser, latestReply string) map[string]any {
	var form models.ClosingForm
	if database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).First(&form).Error != nil {
		return nil
	}
	convo := buildConvoText(history, latestUser, latestReply)
	if !looksLikeOrderIntent(convo) {
		return nil
	}
	result, err := extractClosingData(buildExtractorPromptFromConvo(form, convo))
	if err != nil {
		log.Printf("[closing-preview] agent %d: %v", agentID, err)
		return nil
	}
	// Di chat asli nomor diisi otomatis dari pengirim; di simulator pakai placeholder agar
	// pratinjau tidak salah menandai "Nomor WA" sebagai kurang.
	fillPhoneFromSender(form.SchemaJSON, result.Data, "(otomatis dari nomor pelanggan)")
	missing := missingRequiredFields(form.SchemaJSON, result.Data)
	complete := len(missing) == 0
	return map[string]any{
		"detected":         complete && result.Confidence >= closingMinConfidence,
		"complete":         complete,
		"confidence":       result.Confidence,
		"missing":          missing,
		"data":             result.Data,
		"sheet_configured": agent.SheetSyncEnabled && agent.SpreadsheetURL != "",
	}
}

// buildExtractorPrompt membuat prompt untuk AI extractor.
func buildExtractorPrompt(agentID uint, sender string, agent models.Agent, form models.ClosingForm) string {
	var chats []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).
		Order("created_at desc").Limit(30).Find(&chats)

	var sb strings.Builder
	sb.WriteString("Schema data yang harus diekstrak:\n")
	sb.WriteString(form.SchemaJSON)
	sb.WriteString("\n\nNomor WhatsApp pelanggan (pengirim chat ini): " + sender + " — pakai untuk field bertipe phone bila pelanggan tidak menyebut nomor lain.\n")
	sb.WriteString("\nRingkasan percakapan sebelumnya:\n")
	var mem models.ConversationMemory
	if database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).First(&mem).Error == nil && mem.Summary != "" {
		sb.WriteString(mem.Summary)
	}
	sb.WriteString("\n\nPercakapan (urut lama->baru):\n")
	for i := len(chats) - 1; i >= 0; i-- {
		c := chats[i]
		if strings.TrimSpace(c.Message) != "" {
			sb.WriteString("Customer: " + c.Message + "\n")
		}
		if strings.TrimSpace(c.Reply) != "" {
			sb.WriteString("AI: " + c.Reply + "\n")
		}
	}
	sb.WriteString("\nEkstrak data sesuai schema di atas. Output JSON: {\"confidence\": 0.0-1.0, \"data\": {...}}")
	return sb.String()
}

// validateRequiredFields cek apakah semua field required ada isinya.
func validateRequiredFields(schemaJSON string, data map[string]interface{}) bool {
	var schema struct {
		Fields []struct {
			Key      string `json:"key"`
			Required bool   `json:"required"`
		} `json:"fields"`
	}
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return false
	}
	for _, f := range schema.Fields {
		if !f.Required {
			continue
		}
		val, ok := data[f.Key]
		if !ok || val == nil || val == "" {
			return false
		}
	}
	return true
}

// buildSheetRow membuat baris spreadsheet dari data + schema.
func buildSheetRow(schemaJSON string, data map[string]interface{}, agent models.Agent, sender string) []string {
	var schema struct {
		Fields []struct {
			Key string `json:"key"`
		} `json:"fields"`
	}
	json.Unmarshal([]byte(schemaJSON), &schema)

	row := make([]string, 0, len(schema.Fields)+3)
	row = append(row, time.Now().Format("2006-01-02 15:04"))
	row = append(row, agent.Name)
	row = append(row, sender)
	for _, f := range schema.Fields {
		v := fmt.Sprintf("%v", data[f.Key])
		if v == "<nil>" {
			v = ""
		}
		row = append(row, v)
	}
	return row
}

// TestSheetConnection menguji koneksi ke Google Sheet.
func TestSheetConnection(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var agent models.Agent
	if database.DB.First(&agent, id).Error != nil {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return
	}
	if agent.SpreadsheetURL == "" {
		c.JSON(400, gin.H{"error": "URL spreadsheet belum diisi"})
		return
	}
	sheetID := services.ParseSpreadsheetID(agent.SpreadsheetURL)
	if sheetID == "" {
		c.JSON(400, gin.H{"error": "URL spreadsheet tidak valid"})
		return
	}
	sheetName := agent.SpreadsheetSheetName
	if sheetName == "" {
		sheetName = "Leads"
	}
	email := os.Getenv("GOOGLE_SERVICE_ACCOUNT_EMAIL")
	if email == "" {
		email = "(set GOOGLE_SERVICE_ACCOUNT_EMAIL di .env)"
	}
	if err := services.TestConnection(sheetID, sheetName); err != nil {
		c.JSON(200, gin.H{
			"status":  "gagal",
			"message": "Gagal koneksi. Pastikan spreadsheet sudah di-share ke: " + email,
			"error":   err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{"status": "ok", "message": "Koneksi berhasil!"})
}

// ListSheetNames mengembalikan daftar nama tab/sheet dari URL sheet agent.
func ListSheetNames(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var agent models.Agent
	if database.DB.First(&agent, id).Error != nil {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return
	}
	if agent.SpreadsheetURL == "" {
		c.JSON(400, gin.H{"error": "URL spreadsheet belum diisi"})
		return
	}
	sheetID := services.ParseSpreadsheetID(agent.SpreadsheetURL)
	if sheetID == "" {
		c.JSON(400, gin.H{"error": "URL spreadsheet tidak valid"})
		return
	}
	names, err := services.GetSheetNames(sheetID)
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal membaca sheet: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": names})
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
