package handlers

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"
)

type followUpStepReq struct {
	DelayHours    int    `json:"delay_hours"`
	Message       string `json:"message"`
	AiGenerated   bool   `json:"ai_generated"`
	AiInstruction string `json:"ai_instruction"`
}

// stepsWithCounts merangkai respons follow-up lengkap dengan langkah & ringkasan pendaftaran.
func followUpResponse(fu models.FollowUp) gin.H {
	var steps []models.FollowUpStep
	database.DB.Where("follow_up_id = ?", fu.ID).Order("step_order asc, id asc").Find(&steps)
	var active, completed, stopped int64
	database.DB.Model(&models.FollowUpEnrollment{}).Where("follow_up_id = ? AND status = ?", fu.ID, "active").Count(&active)
	database.DB.Model(&models.FollowUpEnrollment{}).Where("follow_up_id = ? AND status = ?", fu.ID, "completed").Count(&completed)
	database.DB.Model(&models.FollowUpEnrollment{}).Where("follow_up_id = ? AND status = ?", fu.ID, "stopped").Count(&stopped)
	return gin.H{
		"id": fu.ID, "name": fu.Name, "enabled": fu.Enabled, "stop_on_reply": fu.StopOnReply,
		"steps":  steps,
		"counts": gin.H{"active": active, "completed": completed, "stopped": stopped},
	}
}

func ListFollowUps(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var fus []models.FollowUp
	database.DB.Where("agent_id = ?", id).Order("id desc").Find(&fus)
	out := make([]gin.H, 0, len(fus))
	for _, fu := range fus {
		out = append(out, followUpResponse(fu))
	}
	c.JSON(200, gin.H{"data": out})
}

func saveSteps(followUpID uint, steps []followUpStepReq) {
	database.DB.Where("follow_up_id = ?", followUpID).Delete(&models.FollowUpStep{})
	for i, s := range steps {
		if s.Message == "" {
			continue
		}
		delay := s.DelayHours
		if delay < 0 {
			delay = 0
		}
		database.DB.Create(&models.FollowUpStep{
			FollowUpID: followUpID, StepOrder: i, DelayHours: delay,
			Message: s.Message, AiGenerated: s.AiGenerated, AiInstruction: s.AiInstruction,
		})
	}
}

func CreateFollowUp(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	tid := currentTenantID(c)
	var req struct {
		Name        string            `json:"name"`
		StopOnReply *bool             `json:"stop_on_reply"`
		Steps       []followUpStepReq `json:"steps"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if req.Name == "" {
		c.JSON(400, gin.H{"error": "Nama urutan wajib diisi"})
		return
	}
	if !anyStep(req.Steps) {
		c.JSON(400, gin.H{"error": "Minimal satu langkah dengan pesan"})
		return
	}
	stop := true
	if req.StopOnReply != nil {
		stop = *req.StopOnReply
	}
	fu := models.FollowUp{TenantID: tid, AgentID: id, Name: req.Name, Enabled: true, StopOnReply: stop}
	if err := database.DB.Create(&fu).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal membuat follow-up"})
		return
	}
	saveSteps(fu.ID, req.Steps)
	c.JSON(201, gin.H{"data": followUpResponse(fu)})
}

func UpdateFollowUp(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var fu models.FollowUp
	if database.DB.Where("agent_id = ?", id).First(&fu, c.Param("fid")).Error != nil {
		c.JSON(404, gin.H{"error": "Urutan tidak ditemukan"})
		return
	}
	var req struct {
		Name        *string            `json:"name"`
		Enabled     *bool              `json:"enabled"`
		StopOnReply *bool              `json:"stop_on_reply"`
		Steps       *[]followUpStepReq `json:"steps"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if req.Name != nil {
		fu.Name = *req.Name
	}
	if req.Enabled != nil {
		fu.Enabled = *req.Enabled
	}
	if req.StopOnReply != nil {
		fu.StopOnReply = *req.StopOnReply
	}
	_ = database.DB.Save(&fu).Error
	if req.Steps != nil {
		saveSteps(fu.ID, *req.Steps)
	}
	c.JSON(200, gin.H{"data": followUpResponse(fu)})
}

func DeleteFollowUp(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var fu models.FollowUp
	if database.DB.Where("agent_id = ?", id).First(&fu, c.Param("fid")).Error != nil {
		c.JSON(404, gin.H{"error": "Urutan tidak ditemukan"})
		return
	}
	_ = database.DB.Where("follow_up_id = ?", fu.ID).Delete(&models.FollowUpStep{}).Error
	_ = database.DB.Where("follow_up_id = ?", fu.ID).Delete(&models.FollowUpEnrollment{}).Error
	_ = database.DB.Delete(&fu).Error
	c.JSON(200, gin.H{"message": "Deleted"})
}

// EnrollFollowUp mendaftarkan kontak ke sebuah urutan. Lewati nomor yang sudah opt-out
// atau sudah aktif di urutan ini. Kontak yang dulu pernah ikut & sudah selesai bisa diikutkan lagi.
func EnrollFollowUp(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	tid := currentTenantID(c)
	var fu models.FollowUp
	if database.DB.Where("agent_id = ?", id).First(&fu, c.Param("fid")).Error != nil {
		c.JSON(404, gin.H{"error": "Urutan tidak ditemukan"})
		return
	}
	var req struct {
		Recipients []scheduleRecipient `json:"recipients"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Recipients) == 0 {
		c.JSON(400, gin.H{"error": "Penerima wajib diisi"})
		return
	}
	optedOut := optedOutSet(id)
	now := time.Now()
	var added, skipped int
	seen := map[string]bool{}
	for _, r := range req.Recipients {
		num := services.NormalizePhone(r.Number)
		if num == "" || seen[num] {
			continue
		}
		seen[num] = true
		if optedOut[num] {
			skipped++
			continue
		}
		// Sudah aktif di urutan ini? lewati.
		var existing models.FollowUpEnrollment
		if database.DB.Where("follow_up_id = ? AND number = ?", fu.ID, num).First(&existing).Error == nil {
			if existing.Status == "active" {
				skipped++
				continue
			}
			// daftar ulang: reset enrollment lama.
			_ = database.DB.Model(&existing).Updates(map[string]any{
				"name": r.Name, "enrolled_at": now, "next_step": 0,
				"status": "active", "stopped_reason": "", "last_sent_at": nil,
			})
			added++
			continue
		}
		database.DB.Create(&models.FollowUpEnrollment{
			FollowUpID: fu.ID, Number: num, TenantID: tid, AgentID: id,
			Name: r.Name, EnrolledAt: now, NextStep: 0, Status: "active",
		})
		added++
	}
	c.JSON(200, gin.H{"added": added, "skipped": skipped})
}

func anyStep(steps []followUpStepReq) bool {
	for _, s := range steps {
		if s.Message != "" {
			return true
		}
	}
	return false
}

// ---- Worker ----

var followUpSweeping sync.Mutex

// processDueFollowUps mengirim langkah follow-up yang jatuh tempo. Dipanggil tiap menit
// dari scheduler. Dijaga mutex agar tidak ada dua sweep berbarengan (cegah dobel kirim).
func processDueFollowUps() {
	if !followUpSweeping.TryLock() {
		return
	}
	defer followUpSweeping.Unlock()

	var enrolls []models.FollowUpEnrollment
	database.DB.Where("status = ?", "active").Order("id asc").Find(&enrolls)

	const maxPerSweep = 40
	sent := 0
	for _, e := range enrolls {
		if sent >= maxPerSweep {
			break
		}
		var fu models.FollowUp
		if database.DB.First(&fu, e.FollowUpID).Error != nil || !fu.Enabled {
			continue
		}
		var steps []models.FollowUpStep
		database.DB.Where("follow_up_id = ?", fu.ID).Order("step_order asc, id asc").Find(&steps)
		if e.NextStep >= len(steps) {
			database.DB.Model(&models.FollowUpEnrollment{}).Where("id = ?", e.ID).Update("status", "completed")
			continue
		}
		step := steps[e.NextStep]
		if time.Now().Before(e.EnrolledAt.Add(time.Duration(step.DelayHours) * time.Hour)) {
			continue // belum waktunya
		}
		// Opt-out -> stop.
		if followUpOptedOut(e.AgentID, e.Number) {
			stopEnrollment(e.ID, "opt-out")
			continue
		}
		// Kontak sudah membalas setelah didaftarkan -> stop (kalau diaktifkan).
		if fu.StopOnReply && repliedSince(e.AgentID, e.Number, e.EnrolledAt) {
			stopEnrollment(e.ID, "dibalas")
			continue
		}
		if !services.WA(e.AgentID).IsConnected() {
			continue // tunda, coba menit berikutnya
		}

		msg := personalize(spinText(step.Message), e.Name)

		// AI-generated: gunakan instruksi sebagai prompt, personalisasi dengan konteks.
		if step.AiGenerated {
			if aiMsg, ok := generateAIFollowUpMsg(e.AgentID, e.Number, e.Name, step); ok {
				msg = aiMsg
			}
			// fallback: kalau AI gagal, pakai Message literal seperti biasa.
		}

		if err := services.WA(e.AgentID).SendText(e.Number, msg); err != nil {
			continue // gagal kirim -> jangan maju, coba lagi nanti
		}
		logTurn(e.AgentID, e.Number, "", msg, true, "", "")
		sent++

		now := time.Now()
		nextStep := e.NextStep + 1
		status := "active"
		if nextStep >= len(steps) {
			status = "completed"
		}
		database.DB.Model(&models.FollowUpEnrollment{}).Where("id = ?", e.ID).
			Updates(map[string]any{"next_step": nextStep, "status": status, "last_sent_at": &now})

		// Jeda kecil antar kirim agar lembut (anti-banned).
		time.Sleep(6 * time.Second)
	}
}

func followUpOptedOut(agentID uint, number string) bool {
	var n int64
	database.DB.Model(&models.OptOut{}).Where("agent_id = ? AND sender = ?", agentID, number).Count(&n)
	return n > 0
}

// repliedSince = true bila ada pesan MASUK dari kontak setelah waktu tertentu.
func repliedSince(agentID uint, number string, since time.Time) bool {
	var n int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ? AND message <> '' AND created_at > ?", agentID, number, since).
		Count(&n)
	return n > 0
}

func stopEnrollment(enrollID uint, reason string) {
	database.DB.Model(&models.FollowUpEnrollment{}).Where("id = ?", enrollID).
		Updates(map[string]any{"status": "stopped", "stopped_reason": reason})
}

// generateAIFollowUpMsg membuat pesan follow-up personal pakai AI.
// Menggabungkan instruksi user, riwayat chat, dan persona agent.
func generateAIFollowUpMsg(agentID uint, number, name string, step models.FollowUpStep) (string, bool) {
	instruction := strings.TrimSpace(step.AiInstruction)
	if instruction == "" {
		instruction = strings.TrimSpace(step.Message)
	}
	if instruction == "" {
		return "", false
	}

	// Ambil riwayat chat terakhir (maks 5 pesan masuk terakhir).
	var history []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ? AND message <> ''", agentID, number).
		Order("created_at desc").Limit(5).Find(&history)
	// Balik urutan jadi kronologis.
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}

	// Ambil persona agent untuk konteks bisnis.
	var agent models.Agent
	database.DB.Select("system_prompt, name").First(&agent, agentID)

	var sb strings.Builder
	sb.WriteString("Kamu adalah asisten WhatsApp bisnis yang bertugas mengirim pesan follow-up ke pelanggan. ")
	sb.WriteString("Tulis pesan WhatsApp yang personal, ramah, dan natural — seperti manusia, bukan template. ")
	sb.WriteString("Sapa dengan nama bila wajar. Ringkas, 1-3 kalimat. JANGAN mengarang detail spesifik (harga, promo, stok) yang tidak disebut di instruksi.\n")
	sb.WriteString("PENTING: Output HANYA teks pesan WhatsApp yang akan dikirim. JANGAN sertakan catatan, penjelasan, analisis, tanda kutip, prefix \"Pesan:\", asterisk, atau format markdown apapun. Langsung teks mentah saja.\n")
	if strings.TrimSpace(agent.SystemPrompt) != "" {
		sb.WriteString("\nKONTEKS BISNIS:\n" + strings.TrimSpace(agent.SystemPrompt) + "\n")
	}
	sb.WriteString("\nPENERIMA: " + name)
	if number != "" {
		sb.WriteString(" (" + number + ")")
	}
	sb.WriteString("\n\nINSTRUKSI FOLLOW-UP:\n" + instruction)

	if len(history) > 0 {
		sb.WriteString("\n\nRIWAYAT CHAT TERAKHIR:\n")
		for _, h := range history {
			if h.Message != "" {
				sb.WriteString("- Pelanggan: " + h.Message + "\n")
			}
			if h.Reply != "" {
				sb.WriteString("- CS: " + h.Reply + "\n")
			}
		}
	}

	prompt := sb.String()
	log.Printf("FollowUp AI: generating for %s (agent=%d)", number, agentID)

	resp, err := services.CreateAICompletion(context.Background(), []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}, 500, 0.7)
	if err != nil {
		log.Printf("FollowUp AI: gagal generate untuk %s: %v", number, err)
		return "", false
	}
	if len(resp.Choices) == 0 {
		return "", false
	}
	reply := strings.TrimSpace(resp.Choices[0].Message.Content)
	if reply == "" {
		return "", false
	}
	// Sanitasi: hapus prefix umum & bocoran internal AI.
	reply = strings.TrimPrefix(reply, "Pesan:")
	reply = strings.TrimPrefix(reply, "\"")
	reply = strings.TrimSuffix(reply, "\"")
	// Potong di section "Catatan:" atau "*Catatan:" kalau AI masih bandel.
	if idx := strings.Index(reply, "\nCatatan:"); idx >= 0 {
		reply = strings.TrimSpace(reply[:idx])
	}
	if idx := strings.Index(reply, "\n*Catatan:"); idx >= 0 {
		reply = strings.TrimSpace(reply[:idx])
	}
	if idx := strings.Index(reply, "\n**Catatan:**"); idx >= 0 {
		reply = strings.TrimSpace(reply[:idx])
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", false
	}
	log.Printf("FollowUp AI: generated untuk %s (%d chars)", number, len(reply))
	return reply, true
}
