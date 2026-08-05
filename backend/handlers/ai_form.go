package handlers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

const aiFormSessionTTL = 24 * time.Hour

type aiFormStepConfig struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // text | number | select
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

type aiFormRuntimeResult struct {
	handled bool
	reply   string
	buttons []services.ReplyButton
	handoff bool
}

func defaultAIFormSteps() []aiFormStepConfig {
	return []aiFormStepConfig{
		{Key: "name", Label: "Boleh dibantu nama lengkapnya?", Type: "text", Required: true},
		{Key: "need", Label: "Kebutuhan atau kendalanya apa?", Type: "text", Required: true},
		{Key: "schedule", Label: "Kapan waktu yang diinginkan?", Type: "text", Required: false},
		{Key: "note", Label: "Ada catatan tambahan? Ketik *lewati* jika tidak ada.", Type: "text", Required: false},
	}
}

func parseAIFormSteps(form models.AIForm) []aiFormStepConfig {
	var steps []aiFormStepConfig
	if json.Unmarshal([]byte(form.StepsJSON), &steps) != nil || len(steps) == 0 {
		return defaultAIFormSteps()
	}
	return steps
}

func parseAIFormHints(form models.AIForm) []string {
	var hints []string
	_ = json.Unmarshal([]byte(form.IntentHintsJSON), &hints)
	out := make([]string, 0, len(hints))
	for _, hint := range hints {
		if hint = strings.TrimSpace(hint); hint != "" {
			out = append(out, hint)
		}
	}
	return out
}

func validateAIFormConfig(stepsJSON, hintsJSON string) error {
	var steps []aiFormStepConfig
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil || len(steps) == 0 || len(steps) > 12 {
		return fmt.Errorf("Form AI membutuhkan 1-12 pertanyaan yang valid")
	}
	seen := map[string]bool{}
	for _, step := range steps {
		if !validConfigKey(step.Key) || seen[step.Key] || strings.TrimSpace(step.Label) == "" || len([]rune(step.Label)) > 500 {
			return fmt.Errorf("Pertanyaan Form AI tidak valid")
		}
		if step.Type != "text" && step.Type != "number" && step.Type != "select" {
			return fmt.Errorf("Jenis jawaban Form AI tidak valid")
		}
		if step.Type == "select" && (len(step.Options) < 2 || len(step.Options) > 10) {
			return fmt.Errorf("Pertanyaan pilihan membutuhkan 2-10 opsi")
		}
		seen[step.Key] = true
	}
	if strings.TrimSpace(hintsJSON) == "" {
		return nil
	}
	var hints []string
	if err := json.Unmarshal([]byte(hintsJSON), &hints); err != nil || len(hints) > 20 {
		return fmt.Errorf("Contoh kalimat Form AI tidak valid")
	}
	return nil
}

func ListAIForms(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	var rows []models.AIForm
	database.DB.Where("agent_id = ?", agentID).Order("id desc").Find(&rows)
	c.JSON(200, gin.H{"data": rows})
}

func CreateAIForm(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Name            string `json:"name"`
		Goal            string `json:"goal"`
		IntentHintsJSON string `json:"intent_hints_json"`
		StepsJSON       string `json:"steps_json"`
		Enabled         *bool  `json:"enabled"`
		Handoff         *bool  `json:"handoff"`
		SuccessMessage  string `json:"success_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Payload Form AI tidak valid"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(400, gin.H{"error": "Nama Form AI wajib diisi"})
		return
	}
	stepsJSON := strings.TrimSpace(req.StepsJSON)
	if stepsJSON == "" {
		encoded, _ := json.Marshal(defaultAIFormSteps())
		stepsJSON = string(encoded)
	}
	hintsJSON := strings.TrimSpace(req.IntentHintsJSON)
	if hintsJSON == "" {
		hintsJSON = "[]"
	}
	if err := validateAIFormConfig(stepsJSON, hintsJSON); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	enabled, handoff := true, true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.Handoff != nil {
		handoff = *req.Handoff
	}
	form := models.AIForm{
		TenantID: currentTenantID(c), AgentID: agentID, Name: name, Goal: strings.TrimSpace(req.Goal),
		IntentHintsJSON: hintsJSON, StepsJSON: stepsJSON, Enabled: enabled, Handoff: handoff,
		SuccessMessage: strings.TrimSpace(req.SuccessMessage),
	}
	if err := database.DB.Create(&form).Error; err != nil {
		c.JSON(500, gin.H{"error": "Form AI belum bisa disimpan"})
		return
	}
	c.JSON(201, gin.H{"data": form})
}

func UpdateAIForm(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	var form models.AIForm
	if database.DB.Where("agent_id = ?", agentID).First(&form, c.Param("fid")).Error != nil {
		c.JSON(404, gin.H{"error": "Form AI tidak ditemukan"})
		return
	}
	var req struct {
		Name            string `json:"name"`
		Goal            string `json:"goal"`
		IntentHintsJSON string `json:"intent_hints_json"`
		StepsJSON       string `json:"steps_json"`
		Enabled         *bool  `json:"enabled"`
		Handoff         *bool  `json:"handoff"`
		SuccessMessage  string `json:"success_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Payload Form AI tidak valid"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.JSON(400, gin.H{"error": "Nama Form AI wajib diisi"})
		return
	}
	if err := validateAIFormConfig(req.StepsJSON, req.IntentHintsJSON); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	form.Name = strings.TrimSpace(req.Name)
	form.Goal = strings.TrimSpace(req.Goal)
	form.IntentHintsJSON = strings.TrimSpace(req.IntentHintsJSON)
	form.StepsJSON = strings.TrimSpace(req.StepsJSON)
	form.SuccessMessage = strings.TrimSpace(req.SuccessMessage)
	if req.Enabled != nil {
		form.Enabled = *req.Enabled
	}
	if req.Handoff != nil {
		form.Handoff = *req.Handoff
	}
	if err := database.DB.Save(&form).Error; err != nil {
		c.JSON(500, gin.H{"error": "Form AI belum bisa diperbarui"})
		return
	}
	c.JSON(200, gin.H{"data": form})
}

func DeleteAIForm(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	result := database.DB.Where("agent_id = ?", agentID).Delete(&models.AIForm{}, c.Param("fid"))
	if result.Error != nil {
		c.JSON(500, gin.H{"error": "Form AI belum bisa dihapus"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "Form AI tidak ditemukan"})
		return
	}
	c.JSON(200, gin.H{"message": "Form AI dihapus"})
}

func ListAIFormSubmissions(c *gin.Context) {
	agentID, ok := resolveAgent(c)
	if !ok {
		return
	}
	var rows []models.AIFormSubmission
	database.DB.Preload("Form").Where("agent_id = ?", agentID).Order("id desc").Limit(100).Find(&rows)
	c.JSON(200, gin.H{"data": rows})
}

func inAIFormContext(agentID uint, sender, actionID string) bool {
	if strings.HasPrefix(actionID, "ai_form:") {
		return true
	}
	var session models.AIFormSession
	if database.DB.Where("agent_id = ? AND sender = ? AND status IN ?", agentID, sender, []string{"collecting", "confirming"}).First(&session).Error != nil {
		return false
	}
	return time.Now().Before(session.ExpiresAt)
}

func clearAIFormSession(agentID uint, sender string) {
	database.DB.Model(&models.AIFormSession{}).
		Where("agent_id = ? AND sender = ? AND status IN ?", agentID, sender, []string{"collecting", "confirming"}).
		Update("status", "cancelled")
}

func handleAIFormMessage(agentID uint, sender, text, actionID string) aiFormRuntimeResult {
	var session models.AIFormSession
	if database.DB.Where("agent_id = ? AND sender = ? AND status IN ?", agentID, sender, []string{"collecting", "confirming"}).First(&session).Error == nil {
		if time.Now().After(session.ExpiresAt) {
			database.DB.Model(&session).Update("status", "expired")
			return aiFormRuntimeResult{handled: true, reply: "Sesi form sebelumnya sudah berakhir. Silakan kirim ulang kebutuhanmu untuk memulai lagi."}
		}
		return handleAIFormAnswer(session, text, actionID)
	}
	if strings.HasPrefix(actionID, "ai_form:") {
		return handleAIFormAction(agentID, sender, actionID)
	}
	// Form baru dipilih oleh router AI memakai konteks percakapan penuh. Mencocokkan
	// kata pada satu pesan di sini terlalu agresif untuk pertanyaan informasi biasa.
	return aiFormRuntimeResult{}
}

func handleAIFormAction(agentID uint, sender, actionID string) aiFormRuntimeResult {
	parts := strings.Split(actionID, ":")
	if len(parts) < 3 {
		return aiFormRuntimeResult{}
	}
	if parts[1] == "start" {
		formID, err := strconv.ParseUint(parts[2], 10, 64)
		if err != nil {
			return aiFormRuntimeResult{}
		}
		var form models.AIForm
		if database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).First(&form, uint(formID)).Error != nil {
			return aiFormRuntimeResult{handled: true, reply: "Form ini sudah tidak tersedia."}
		}
		return startAIForm(form, sender)
	}
	return aiFormRuntimeResult{}
}

func startAIForm(form models.AIForm, sender string) aiFormRuntimeResult {
	clearFlowSession(form.AgentID, sender)
	clearProductCheckoutSession(form.AgentID, sender)
	markProductLead(form.AgentID, sender, "hot")
	now := time.Now()
	session := models.AIFormSession{
		TenantID: form.TenantID, AgentID: form.AgentID, Sender: sender, FormID: form.ID,
		StepIndex: 0, DataJSON: "{}", Status: "collecting", ExpiresAt: now.Add(aiFormSessionTTL),
	}
	var existing models.AIFormSession
	if database.DB.Where("agent_id = ? AND sender = ?", form.AgentID, sender).First(&existing).Error == nil {
		session.ID = existing.ID
		database.DB.Model(&existing).Updates(map[string]any{
			"tenant_id": form.TenantID, "form_id": form.ID, "step_index": 0,
			"data_json": "{}", "status": "collecting", "expires_at": session.ExpiresAt,
		})
	} else {
		database.DB.Create(&session)
	}
	steps := parseAIFormSteps(form)
	result := aiFormQuestion(session, form, steps[0], 0, len(steps))
	result.reply = fmt.Sprintf("Baik kak, supaya proses *%s* tercatat rapi, saya bantu lewat %d pertanyaan singkat ya.\n\n%s", form.Name, len(steps), result.reply)
	return result
}

func startAIFormEdit(form models.AIForm, submission models.AIFormSubmission, sender string) aiFormRuntimeResult {
	clearFlowSession(form.AgentID, sender)
	clearProductCheckoutSession(form.AgentID, sender)
	data := map[string]string{}
	_ = json.Unmarshal([]byte(submission.DataJSON), &data)
	data["_edit_submission_id"] = strconv.FormatUint(uint64(submission.ID), 10)
	encoded, _ := json.Marshal(data)
	steps := parseAIFormSteps(form)
	session := models.AIFormSession{
		TenantID: form.TenantID, AgentID: form.AgentID, Sender: sender, FormID: form.ID,
		StepIndex: len(steps), DataJSON: string(encoded), Status: "confirming", ExpiresAt: time.Now().Add(aiFormSessionTTL),
	}
	var existing models.AIFormSession
	if database.DB.Where("agent_id = ? AND sender = ?", form.AgentID, sender).First(&existing).Error == nil {
		session.ID = existing.ID
		database.DB.Model(&existing).Updates(map[string]any{
			"tenant_id": form.TenantID, "form_id": form.ID, "step_index": len(steps),
			"data_json": session.DataJSON, "status": "confirming", "expires_at": session.ExpiresAt,
		})
	} else {
		database.DB.Create(&session)
	}
	result := aiFormConfirmation(session, form, steps)
	result.reply = "Tentu kak, data yang sebelumnya tersimpan bisa diperbarui. Saya tampilkan ringkasannya dulu supaya kakak dapat memilih bagian yang ingin diubah.\n\n" + result.reply
	return result
}

func aiFormQuestion(session models.AIFormSession, form models.AIForm, step aiFormStepConfig, index, total int) aiFormRuntimeResult {
	reply := fmt.Sprintf("%s · langkah %d/%d\n%s\n\nKetik *batal* untuk membatalkan", form.Name, index+1, total, strings.TrimSpace(step.Label))
	if index > 0 {
		reply += " · *kembali* untuk langkah sebelumnya"
	}
	buttons := []services.ReplyButton{}
	if step.Type == "select" && len(step.Options) <= 3 {
		for optionIndex, option := range step.Options {
			buttons = append(buttons, services.ReplyButton{ID: fmt.Sprintf("ai_form:%d:option:%d", session.ID, optionIndex), Text: option})
		}
	} else if step.Type == "select" {
		lines := []string{}
		for optionIndex, option := range step.Options {
			lines = append(lines, fmt.Sprintf("%d. %s", optionIndex+1, option))
		}
		reply = fmt.Sprintf("%s · langkah %d/%d\n%s\n\n%s\n\nBalas nomor atau nama pilihannya. Ketik *batal* untuk membatalkan.", form.Name, index+1, total, strings.TrimSpace(step.Label), strings.Join(lines, "\n"))
	}
	return aiFormRuntimeResult{handled: true, reply: reply, buttons: buttons}
}

func handleAIFormAnswer(session models.AIFormSession, text, actionID string) aiFormRuntimeResult {
	var form models.AIForm
	if database.DB.Where("agent_id = ?", session.AgentID).First(&form, session.FormID).Error != nil {
		database.DB.Model(&session).Update("status", "cancelled")
		return aiFormRuntimeResult{handled: true, reply: "Form sudah tidak tersedia."}
	}
	steps := parseAIFormSteps(form)
	normalized := strings.ToLower(strings.TrimSpace(text))
	if strings.HasSuffix(actionID, ":cancel") || normalized == "batal" || normalized == "cancel" {
		database.DB.Model(&session).Update("status", "cancelled")
		data := map[string]string{}
		_ = json.Unmarshal([]byte(session.DataJSON), &data)
		if data["_edit_submission_id"] != "" {
			return aiFormRuntimeResult{handled: true, reply: "Baik kak, perubahan dibatalkan. Data sebelumnya tetap tersimpan dan tidak berubah."}
		}
		return aiFormRuntimeResult{handled: true, reply: "Form *" + form.Name + "* dibatalkan. Data sementara sudah dihapus."}
	}
	if strings.HasSuffix(actionID, ":confirm") {
		return confirmAIForm(session, form, steps)
	}
	if strings.HasSuffix(actionID, ":edit") {
		session.StepIndex = 0
		session.Status = "collecting"
		database.DB.Model(&session).Updates(map[string]any{"step_index": 0, "status": "collecting", "expires_at": time.Now().Add(aiFormSessionTTL)})
		return aiFormQuestion(session, form, steps[0], 0, len(steps))
	}
	if normalized == "kembali" || normalized == "back" {
		if session.StepIndex > 0 {
			session.StepIndex--
			database.DB.Model(&session).Updates(map[string]any{"step_index": session.StepIndex, "status": "collecting", "expires_at": time.Now().Add(aiFormSessionTTL)})
		}
		return aiFormQuestion(session, form, steps[session.StepIndex], session.StepIndex, len(steps))
	}
	if isGenericGreetingMessage(text) {
		if session.Status == "confirming" {
			return aiFormConfirmation(session, form, steps)
		}
		result := aiFormQuestion(session, form, steps[session.StepIndex], session.StepIndex, len(steps))
		result.reply = "Halo kak 👋 form ini masih aktif. " + result.reply
		return result
	}
	if session.Status == "confirming" {
		return aiFormConfirmation(session, form, steps)
	}
	if session.StepIndex < 0 || session.StepIndex >= len(steps) {
		return aiFormConfirmation(session, form, steps)
	}
	step := steps[session.StepIndex]
	answer, valid := aiFormAnswerValue(step, text, actionID, session.ID)
	if !valid {
		result := aiFormQuestion(session, form, step, session.StepIndex, len(steps))
		result.reply = "Jawaban belum sesuai. " + result.reply
		return result
	}
	data := map[string]string{}
	_ = json.Unmarshal([]byte(session.DataJSON), &data)
	data[step.Key] = answer
	encoded, _ := json.Marshal(data)
	session.StepIndex++
	session.DataJSON = string(encoded)
	session.ExpiresAt = time.Now().Add(aiFormSessionTTL)
	if session.StepIndex >= len(steps) {
		session.Status = "confirming"
		database.DB.Model(&session).Updates(map[string]any{"step_index": session.StepIndex, "data_json": session.DataJSON, "status": session.Status, "expires_at": session.ExpiresAt})
		return aiFormConfirmation(session, form, steps)
	}
	database.DB.Model(&session).Updates(map[string]any{"step_index": session.StepIndex, "data_json": session.DataJSON, "expires_at": session.ExpiresAt})
	return aiFormQuestion(session, form, steps[session.StepIndex], session.StepIndex, len(steps))
}

func handleAIFormImageAnswer(agentID uint, sender, answer, messageID string, needsHuman bool) (aiFormRuntimeResult, bool, bool) {
	var session models.AIFormSession
	if database.DB.Where("agent_id = ? AND sender = ? AND status = ?", agentID, sender, "collecting").First(&session).Error != nil || time.Now().After(session.ExpiresAt) {
		return aiFormRuntimeResult{}, false, false
	}
	var form models.AIForm
	if database.DB.Where("agent_id = ?", agentID).First(&form, session.FormID).Error != nil {
		return aiFormRuntimeResult{}, false, false
	}
	steps := parseAIFormSteps(form)
	if session.StepIndex < 0 || session.StepIndex >= len(steps) {
		return aiFormConfirmation(session, form, steps), true, false
	}
	step := steps[session.StepIndex]
	if _, valid := aiFormAnswerValue(step, answer, "", session.ID); !valid {
		result := aiFormQuestion(session, form, step, session.StepIndex, len(steps))
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
	return handleAIFormAnswer(session, answer, ""), true, true
}

func aiFormAnswerValue(step aiFormStepConfig, text, actionID string, sessionID uint) (string, bool) {
	value := strings.TrimSpace(text)
	if !step.Required && strings.EqualFold(value, "lewati") {
		return "", true
	}
	if strings.HasPrefix(actionID, fmt.Sprintf("ai_form:%d:option:", sessionID)) {
		raw := strings.TrimPrefix(actionID, fmt.Sprintf("ai_form:%d:option:", sessionID))
		index, err := strconv.Atoi(raw)
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
		return value, err == nil && n > 0 && n <= 999999
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

func aiFormSummary(form models.AIForm, steps []aiFormStepConfig, dataJSON string) string {
	data := map[string]string{}
	_ = json.Unmarshal([]byte(dataJSON), &data)
	lines := []string{"*Ringkasan " + form.Name + "*"}
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

func aiFormConfirmation(session models.AIFormSession, form models.AIForm, steps []aiFormStepConfig) aiFormRuntimeResult {
	return aiFormRuntimeResult{
		handled: true,
		reply:   aiFormSummary(form, steps, session.DataJSON) + "\n\nPastikan datanya sudah benar.",
		buttons: []services.ReplyButton{
			{ID: fmt.Sprintf("ai_form:%d:confirm", session.ID), Text: "Data sudah benar"},
			{ID: fmt.Sprintf("ai_form:%d:edit", session.ID), Text: "Ubah data"},
			{ID: fmt.Sprintf("ai_form:%d:cancel", session.ID), Text: "Batalkan"},
		},
	}
}

func confirmAIForm(session models.AIFormSession, form models.AIForm, steps []aiFormStepConfig) aiFormRuntimeResult {
	data := map[string]string{}
	_ = json.Unmarshal([]byte(session.DataJSON), &data)
	if editID, err := strconv.ParseUint(data["_edit_submission_id"], 10, 64); err == nil && editID > 0 {
		var existing models.AIFormSubmission
		if database.DB.Where("id = ? AND agent_id = ? AND sender = ? AND form_id = ?", uint(editID), form.AgentID, session.Sender, form.ID).First(&existing).Error != nil {
			return aiFormRuntimeResult{handled: true, reply: "Data sebelumnya tidak ditemukan. Silakan batalkan lalu mulai pengisian baru."}
		}
		delete(data, "_edit_submission_id")
		cleanJSON, _ := json.Marshal(data)
		summary := aiFormSummary(form, steps, string(cleanJSON))
		if err := database.DB.Model(&existing).Updates(map[string]any{"data_json": string(cleanJSON), "summary": summary, "status": "submitted"}).Error; err != nil {
			return aiFormRuntimeResult{handled: true, reply: "Perubahan belum berhasil disimpan. Silakan tekan konfirmasi lagi."}
		}
		database.DB.Model(&session).Updates(map[string]any{"status": "completed", "data_json": string(cleanJSON)})
		markProductLead(form.AgentID, session.Sender, "hot")
		return aiFormRuntimeResult{
			handled: true,
			reply:   "Siap kak, data *" + existing.Code + "* sudah diperbarui sesuai informasi terbaru.",
			handoff: form.Handoff || data["_vision_needs_human"] == "true",
		}
	}
	code := fmt.Sprintf("FORM-%s-%06d", time.Now().Format("060102"), session.ID)
	summary := aiFormSummary(form, steps, session.DataJSON)
	submission := models.AIFormSubmission{
		TenantID: form.TenantID, AgentID: form.AgentID, FormID: form.ID, Sender: session.Sender,
		Code: code, Status: "submitted", DataJSON: session.DataJSON, Summary: summary,
	}
	if err := database.DB.Create(&submission).Error; err != nil {
		return aiFormRuntimeResult{handled: true, reply: "Data belum berhasil disimpan. Silakan tekan Konfirmasi lagi."}
	}
	database.DB.Model(&session).Update("status", "completed")
	markProductLead(form.AgentID, session.Sender, "hot")
	message := strings.TrimSpace(form.SuccessMessage)
	if message == "" {
		message = "Data *{code}* berhasil dicatat. CS kami akan menindaklanjuti."
	} else if !strings.Contains(message, "{code}") {
		message += "\n\nKode data: *{code}*"
	}
	message = strings.ReplaceAll(message, "{code}", code)
	message = strings.ReplaceAll(message, "{form}", form.Name)
	return aiFormRuntimeResult{handled: true, reply: message, handoff: form.Handoff || data["_vision_needs_human"] == "true"}
}

// aiFormRoutingPrompt memberi AI daftar form resmi. AI hanya memilih rute; seluruh
// pengumpulan dan penyimpanan data tetap dijalankan mesin form secara deterministik.
func aiFormRoutingPrompt(agentID uint, sender string) string {
	var forms []models.AIForm
	database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).Order("id desc").Limit(20).Find(&forms)
	if len(forms) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("FORM AI RESMI YANG TERSEDIA:\n")
	sb.WriteString("ATURAN ROUTING NATURAL DAN WAJIB:\n")
	sb.WriteString("- [[START_FORM:ID]] hanya jika niat pelanggan untuk benar-benar memproses layanan sudah cukup jelas dari konteks. Jika masih sekadar bertanya atau ragu, jawab natural dan ajukan maksimal satu klarifikasi halus; jangan tampilkan form.\n")
	sb.WriteString("- [[EDIT_FORM:ID]] hanya jika pelanggan bermaksud mengoreksi, mengganti, atau melengkapi data layanan yang sebelumnya sudah tersimpan. Jangan membuat data baru untuk permintaan edit.\n")
	sb.WriteString("- Sapaan, ucapan terima kasih, pertanyaan informasi, dan obrolan biasa tidak boleh memicu token apa pun.\n")
	sb.WriteString("- Saat memilih token, balas HANYA satu token tanpa kalimat lain. Jangan menanyakan field form sendiri; mesin form akan membukanya dengan penjelasan yang ramah.\n")
	for _, form := range forms {
		sb.WriteString(fmt.Sprintf("- ID %d | %s", form.ID, strings.TrimSpace(form.Name)))
		if goal := strings.TrimSpace(form.Goal); goal != "" {
			sb.WriteString(" | tujuan: " + goal)
		}
		if hints := parseAIFormHints(form); len(hints) > 0 {
			sb.WriteString(" | contoh: " + strings.Join(hints, "; "))
		}
		steps := parseAIFormSteps(form)
		if len(steps) > 0 {
			fields := make([]string, 0, len(steps))
			for _, step := range steps {
				fields = append(fields, strings.TrimSpace(step.Label))
			}
			sb.WriteString(" | field resmi (jangan ditanyakan sendiri): " + strings.Join(fields, "; "))
		}
		sb.WriteString("\n")
	}
	var submissions []models.AIFormSubmission
	database.DB.Preload("Form").Where("agent_id = ? AND sender = ? AND status = ?", agentID, sender, "submitted").Order("id desc").Limit(8).Find(&submissions)
	if len(submissions) > 0 {
		sb.WriteString("DATA FORM PELANGGAN INI YANG SUDAH TERSIMPAN (hanya untuk memahami intent edit, jangan dibacakan seluruhnya tanpa diminta):\n")
		for _, submission := range submissions {
			summary := strings.Join(strings.Fields(submission.Summary), " ")
			if len([]rune(summary)) > 320 {
				summary = string([]rune(summary)[:320]) + "…"
			}
			sb.WriteString(fmt.Sprintf("- Form ID %d | kode %s | %s | data: %s\n", submission.FormID, submission.Code, submission.Form.Name, summary))
		}
	}
	return sb.String()
}

func startAIFormFromFreeCollection(agentID uint, sender, latestUser, conversationText, reply string) (aiFormRuntimeResult, bool) {
	if isGenericGreetingMessage(latestUser) {
		return aiFormRuntimeResult{}, false
	}
	if !replyRequestsStructuredData(reply) {
		return aiFormRuntimeResult{}, false
	}
	form, matched := matchAIForm(agentID, conversationText+"\n"+reply)
	// AI mengarang daftar field free-text: bila cuma satu form aktif, alihkan ke form itu
	// (domain-agnostic; menghindari chat bebas tanpa pencatatan resmi).
	if !matched {
		var forms []models.AIForm
		database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).Order("id desc").Limit(5).Find(&forms)
		if len(forms) == 1 {
			form, matched = forms[0], true
		}
	}
	if !matched || !replyOverlapsAIFormFields(form, reply) {
		return aiFormRuntimeResult{}, false
	}
	if messageHasEditIntent(latestUser) {
		var submission models.AIFormSubmission
		if database.DB.Where("agent_id = ? AND sender = ? AND form_id = ? AND status = ?", agentID, sender, form.ID, "submitted").Order("id desc").First(&submission).Error == nil {
			return startAIFormEdit(form, submission, sender), true
		}
	}
	return startAIForm(form, sender), true
}

func messageHasEditIntent(text string) bool {
	for _, token := range aiFormTokens(text) {
		switch token {
		case "ubah", "ganti", "koreksi", "revisi", "perbarui", "update", "edit", "salah", "keliru":
			return true
		}
	}
	return false
}

func isGenericGreetingMessage(text string) bool {
	words := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(text)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(words) == 0 || len(words) > 4 {
		return false
	}
	allowed := map[string]bool{
		"halo": true, "hallo": true, "hello": true, "hai": true, "hi": true,
		"selamat": true, "pagi": true, "siang": true, "sore": true, "malam": true,
		"assalamualaikum": true, "salam": true, "kak": true, "min": true, "admin": true,
	}
	hasGreeting := false
	for _, word := range words {
		if !allowed[word] {
			return false
		}
		if word != "kak" && word != "min" && word != "admin" {
			hasGreeting = true
		}
	}
	return hasGreeting
}

func replyRequestsStructuredData(reply string) bool {
	lower := strings.ToLower(reply)
	// Deteksi AI yang (salah) mengumpulkan data free-text alih-alih [[START_FORM:ID]].
	// Domain-agnostic: pola form isian, bukan nama produk.
	markers := []string{
		"silakan beri tahu", "silakan kirim", "silakan isi", "boleh dibantu",
		"mohon isi", "lengkapi data", "data berikut", "form pemesanan",
		"form order", "form booking", "isi form", "lengkapi formulir",
		"nama lengkap", "alamat lengkap", "data pesanan", "data pemesanan",
		"kirimkan data", "mohon dilengkapi",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	// Pola daftar field: beberapa baris "Label:" berturut-turut.
	colonFields := 0
	for _, ln := range strings.Split(reply, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if strings.HasSuffix(ln, ":") && len([]rune(ln)) <= 40 {
			colonFields++
		}
	}
	return colonFields >= 2
}

func replyOverlapsAIFormFields(form models.AIForm, reply string) bool {
	replyTokens := aiFormTokens(reply)
	for _, step := range parseAIFormSteps(form) {
		if aiFormOverlapScore(replyTokens, aiFormTokens(step.Label), 1) > 0 {
			return true
		}
	}
	// AI sering menamai field generik (nama/alamat/jumlah) meski label form beda sedikit.
	// Cukup bila reply meminta ≥2 field umum yang relevan form order/booking generik.
	lower := strings.ToLower(reply)
	generic := 0
	for _, g := range []string{"nama", "alamat", "jumlah", "ukuran", "warna", "produk", "penerima", "jadwal", "paket"} {
		if strings.Contains(lower, g) {
			generic++
		}
	}
	if generic >= 2 && (strings.Contains(strings.ToLower(form.Name+form.Goal), "order") ||
		strings.Contains(strings.ToLower(form.Name+form.Goal), "pesan") ||
		strings.Contains(strings.ToLower(form.Name+form.Goal), "booking") ||
		strings.Contains(strings.ToLower(form.Name+form.Goal), "daftar") ||
		strings.Contains(strings.ToLower(form.Name+form.Goal), "jemput") ||
		strings.Contains(strings.ToLower(form.Name+form.Goal), "donasi") ||
		len(parseAIFormSteps(form)) > 0) {
		return true
	}
	return false
}

var startFormDirectiveRe = regexp.MustCompile(`(?i)\[\[\s*START_FORM\s*:\s*([^\]]+?)\s*\]\]`)

func handleAIFormDirective(agentID uint, sender, reply string) (aiFormRuntimeResult, bool) {
	if formID, found, valid := parseEditAIFormDirective(reply); found {
		if !valid {
			return aiFormRuntimeResult{handled: true, reply: "Maaf kak, data yang ingin diubah belum dapat dikenali. Boleh sebutkan layanan yang dimaksud?"}, true
		}
		var form models.AIForm
		var submission models.AIFormSubmission
		if database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).First(&form, formID).Error != nil ||
			database.DB.Where("agent_id = ? AND sender = ? AND form_id = ? AND status = ?", agentID, sender, formID, "submitted").Order("id desc").First(&submission).Error != nil {
			return aiFormRuntimeResult{handled: true, reply: "Saya belum menemukan data lama untuk layanan itu. Jika ingin membuat data baru, sampaikan kebutuhannya ya kak."}, true
		}
		return startAIFormEdit(form, submission, sender), true
	}
	// Deteksi intent START_FORM (termasuk format longgar / placeholder "ID").
	if !startFormDirectiveRe.MatchString(reply) && !strings.Contains(strings.ToUpper(reply), "START_FORM") {
		return aiFormRuntimeResult{}, false
	}
	form, ok := resolveStartFormTarget(agentID, reply)
	if !ok {
		return aiFormRuntimeResult{handled: true, reply: "Maaf, form layanan belum bisa dimulai. Silakan tuliskan kebutuhanmu sekali lagi."}, true
	}
	return startAIForm(form, sender), true
}

// resolveStartFormTarget memilih form dari token [[START_FORM:…]] atau fallback
// aman (satu form aktif / cocok nama) — domain-agnostic.
func resolveStartFormTarget(agentID uint, reply string) (models.AIForm, bool) {
	var forms []models.AIForm
	database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).Order("id desc").Limit(50).Find(&forms)
	if len(forms) == 0 {
		return models.AIForm{}, false
	}
	// 1) ID numerik dari directive.
	if m := startFormDirectiveRe.FindStringSubmatch(reply); len(m) == 2 {
		raw := strings.TrimSpace(m[1])
		if id, err := strconv.ParseUint(raw, 10, 64); err == nil && id > 0 {
			for _, f := range forms {
				if uint64(f.ID) == id {
					return f, true
				}
			}
		}
		// 2) Model kadang menulis nama form: [[START_FORM:Order Kaos]]
		rawLower := strings.ToLower(raw)
		if rawLower != "" && rawLower != "id" {
			for _, f := range forms {
				if strings.EqualFold(strings.TrimSpace(f.Name), raw) ||
					strings.Contains(strings.ToLower(f.Name), rawLower) {
					return f, true
				}
			}
		}
	}
	// 3) Satu form aktif saja → pakai itu (hindari gagal total karena ID salah).
	if len(forms) == 1 {
		return forms[0], true
	}
	// 4) Cocokkan nama form yang disebut di reply di luar token.
	replyLower := strings.ToLower(reply)
	for _, f := range forms {
		name := strings.TrimSpace(strings.ToLower(f.Name))
		if name != "" && strings.Contains(replyLower, name) {
			return f, true
		}
	}
	return models.AIForm{}, false
}

func parseEditAIFormDirective(reply string) (uint, bool, bool) {
	const prefix = "[[EDIT_FORM:"
	start := strings.Index(reply, prefix)
	if start < 0 {
		return 0, false, false
	}
	remainder := reply[start+len(prefix):]
	end := strings.Index(remainder, "]]")
	if end < 1 {
		return 0, true, false
	}
	formID, err := strconv.ParseUint(strings.TrimSpace(remainder[:end]), 10, 64)
	return uint(formID), true, err == nil && formID > 0
}

type aiFormCandidate struct {
	form  models.AIForm
	score int
}

func matchAIForm(agentID uint, text string) (models.AIForm, bool) {
	qTokens := aiFormTokens(text)
	if len(qTokens) == 0 {
		return models.AIForm{}, false
	}
	var forms []models.AIForm
	database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).Order("id desc").Limit(50).Find(&forms)
	if len(forms) == 0 {
		return models.AIForm{}, false
	}
	ranked := []aiFormCandidate{}
	for _, form := range forms {
		score := aiFormIntentScore(form, text, qTokens)
		if score >= 5 {
			ranked = append(ranked, aiFormCandidate{form: form, score: score})
		}
	}
	if len(ranked) == 0 {
		return models.AIForm{}, false
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	return ranked[0].form, true
}

func aiFormIntentScore(form models.AIForm, raw string, qTokens []string) int {
	rawLower := strings.ToLower(raw)
	score := 0
	for _, hint := range parseAIFormHints(form) {
		hintLower := strings.ToLower(hint)
		if hintLower != "" && strings.Contains(rawLower, hintLower) {
			score += 10
		}
		score += aiFormOverlapScore(qTokens, aiFormTokens(hint), 3)
	}
	score += aiFormOverlapScore(qTokens, aiFormTokens(form.Name), 4)
	score += aiFormOverlapScore(qTokens, aiFormTokens(form.Goal), 2)
	if hasAIFormStartIntent(qTokens) {
		score += 2
	}
	return score
}

func aiFormOverlapScore(qTokens, targetTokens []string, weight int) int {
	if len(qTokens) == 0 || len(targetTokens) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, token := range targetTokens {
		set[token] = true
	}
	score := 0
	for _, token := range qTokens {
		if set[token] {
			score += weight
			continue
		}
		// Typo satu karakter pada kata bermakna (mis. "sedkah" -> "sedekah")
		// tetap dianggap cocok agar pelanggan tidak jatuh kembali ke chat AI bebas.
		for target := range set {
			if oneEditApart(token, target) {
				score += weight
				break
			}
		}
	}
	return score
}

func oneEditApart(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	if len(ar) < 5 || len(br) < 5 || len(ar)-len(br) > 1 || len(br)-len(ar) > 1 {
		return false
	}
	i, j, edits := 0, 0, 0
	for i < len(ar) && j < len(br) {
		if ar[i] == br[j] {
			i++
			j++
			continue
		}
		edits++
		if edits > 1 {
			return false
		}
		switch {
		case len(ar) > len(br):
			i++
		case len(br) > len(ar):
			j++
		default:
			i++
			j++
		}
	}
	if i < len(ar) || j < len(br) {
		edits++
	}
	return edits == 1
}

func hasAIFormStartIntent(tokens []string) bool {
	for _, token := range tokens {
		switch token {
		case "daftar", "booking", "pesan", "pesanan", "pemesanan", "order", "checkout",
			"konsultasi", "jadwal", "mulai", "minat", "mendaftar", "reservasi",
			"donasi", "sedekah", "jemput", "penjemputan", "form", "beli", "ambil":
			return true
		}
	}
	return false
}

var aiFormStopwords = map[string]bool{
	"yang": true, "dan": true, "atau": true, "dengan": true, "untuk": true, "dari": true,
	"ini": true, "itu": true, "saya": true, "aku": true, "kamu": true, "kak": true,
	"mau": true, "ingin": true, "bisa": true, "boleh": true, "dong": true, "ya": true,
	"apa": true, "apakah": true, "gimana": true, "bagaimana": true,
}

func aiFormTokens(value string) []string {
	fields := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := []string{}
	seen := map[string]bool{}
	for _, token := range fields {
		token = strings.TrimSpace(token)
		if len([]rune(token)) < 3 || aiFormStopwords[token] || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}
