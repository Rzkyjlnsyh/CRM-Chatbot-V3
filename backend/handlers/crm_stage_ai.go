package handlers

import (
	"log"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

// maybeAssessCRMLeadStage menilai tahap minat dengan AI memakai definisi tahap
// milik user. Fail-closed: error apa pun → tahap lama dipertahankan.
func maybeAssessCRMLeadStage(agentID uint, sender string, latestChatID uint) {
	database.EnsureDefaultStages(agentID)
	cfg := database.GetLeadLabelConfig(agentID)
	if !cfg.SmartLabelsEnabled {
		return
	}
	defs := database.GetStageDefMap(agentID)

	var contact models.Contact
	if database.DB.Where("agent_id = ? AND number = ?", agentID, sender).First(&contact).Error != nil {
		return
	}
	// Kontak terkunci manual, sudah closing (sticky), atau chat sudah dianalisa → lewati.
	if contact.LeadStageLocked || stageIsClosing(defs, contact.LeadStage) || latestChatID <= contact.LeadStageAnalyzedChatID {
		return
	}
	var history []models.ChatHistory
	database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).
		Order("id desc").Limit(16).Find(&history)
	for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
		history[i], history[j] = history[j], history[i]
	}
	var memory models.ConversationMemory
	database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).First(&memory)

	hints := make([]services.CRMStageHint, 0, len(defs))
	for _, key := range database.SortedStageKeys(defs) {
		d := defs[key]
		hints = append(hints, services.CRMStageHint{Key: d.Key, Name: d.Name, Description: d.Description, IsClosing: d.IsClosing})
	}
	services.SortStageHints(hints, func(key string) int {
		if d, ok := defs[key]; ok {
			return d.Rank
		}
		return 999
	})

	assessment, err := services.ClassifyCRMLead(history, memory.Summary, hints, cfg.ClosingDefinition)
	if err != nil {
		log.Printf("CRM AI gagal menilai agent %d kontak %s: %v", agentID, sender, err)
		return
	}
	now := time.Now()
	updates := map[string]any{"lead_stage_analyzed_chat_id": latestChatID}
	if canApplyAILeadStage(contact, assessment, defs) {
		updates["lead_stage"] = assessment.Stage
		updates["lead_stage_source"] = "ai"
		updates["lead_stage_reason"] = assessment.Reason
		updates["lead_stage_confidence"] = assessment.Confidence
		updates["lead_stage_updated_at"] = &now
	}
	// Kondisi ini mencegah hasil AI yang datang terlambat menimpa edit manual.
	database.DB.Model(&models.Contact{}).
		Where("id = ? AND agent_id = ? AND lead_stage_locked = ? AND lead_stage_analyzed_chat_id < ?", contact.ID, agentID, false, latestChatID).
		Updates(updates)
}

// stageIsClosing mengecek apakah key tahap bertanda closing di definisi user.
func stageIsClosing(defs map[string]models.LeadStageDef, key string) bool {
	d, ok := defs[key]
	return ok && d.IsClosing
}

// canApplyAILeadStage = guardrail penerapan hasil AI:
//  1. AI tidak pernah menetapkan tahap closing (hanya transaksi terkonfirmasi/manual).
//  2. Ambang keyakinan per-tahap dari definisi user (bukan hardcode tunggal).
//  3. Monotonic: AI boleh menaikkan minat, penurunan dilakukan manual/aturan.
//  4. Sinyal aktivitas eksplisit (form/checkout) tidak boleh diturunkan AI.
func canApplyAILeadStage(contact models.Contact, assessment services.CRMLeadAssessment, defs map[string]models.LeadStageDef) bool {
	if contact.LeadStageLocked {
		return false
	}
	// AI dilarang menghasilkan closing/customer.
	if stageIsClosing(defs, assessment.Stage) {
		return false
	}
	if stageIsClosing(defs, contact.LeadStage) {
		return false
	}
	if assessment.Stage == contact.LeadStage {
		// Nilai sama: boleh simpan alasan selama sumber bukan aktivitas (tak menimpa).
		return contact.LeadStageSource != "activity"
	}
	// Ambang keyakinan per-tahap dari definisi user.
	threshold := 0.72
	if d, ok := defs[assessment.Stage]; ok && d.MinConfidence > 0 {
		threshold = d.MinConfidence
	}
	if assessment.Confidence < threshold {
		return false
	}
	// Monotonic: AI hanya boleh menaikkan minat. Pengecualian tunggal: penurunan
	// ke tahap TERENDAH (rank 0 = bucket negatif spam/tidak relevan) diizinkan
	// dengan ambang keyakinan tahap itu (default 0.9) agar deteksi spam tetap ada.
	targetRank := crmStageRankWithDefs(assessment.Stage, defs)
	currentRank := crmStageRankWithDefs(contact.LeadStage, defs)
	if targetRank < currentRank && targetRank != 0 {
		return false
	}
	return true
}

func crmStageRankWithDefs(stage string, defs map[string]models.LeadStageDef) int {
	if d, ok := defs[stage]; ok {
		return d.Rank
	}
	return -1
}
