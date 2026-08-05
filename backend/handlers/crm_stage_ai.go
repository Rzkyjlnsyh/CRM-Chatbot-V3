package handlers

import (
	"log"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

func maybeAssessCRMLeadStage(agentID uint, sender string, latestChatID uint) {
	var contact models.Contact
	if database.DB.Where("agent_id = ? AND number = ?", agentID, sender).First(&contact).Error != nil {
		return
	}
	if contact.LeadStageLocked || contact.LeadStage == leadStageCustomer || latestChatID <= contact.LeadStageAnalyzedChatID {
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
	assessment, err := services.ClassifyCRMLead(history, memory.Summary)
	if err != nil {
		log.Printf("CRM AI gagal menilai agent %d kontak %s: %v", agentID, sender, err)
		return
	}
	now := time.Now()
	updates := map[string]any{"lead_stage_analyzed_chat_id": latestChatID}
	if canApplyAILeadStage(contact, assessment) {
		updates["lead_stage"] = assessment.Stage
		updates["lead_stage_source"] = "ai"
		updates["lead_stage_reason"] = assessment.Reason
		updates["lead_stage_confidence"] = assessment.Confidence
		updates["lead_stage_updated_at"] = &now
	}
	// Kondisi ini mencegah hasil AI yang datang terlambat menimpa edit manual.
	database.DB.Model(&models.Contact{}).
		Where("id = ? AND agent_id = ? AND lead_stage_locked = ? AND lead_stage != ? AND lead_stage_analyzed_chat_id < ?", contact.ID, agentID, false, leadStageCustomer, latestChatID).
		Updates(updates)
}

func canApplyAILeadStage(contact models.Contact, assessment services.CRMLeadAssessment) bool {
	if contact.LeadStageLocked || contact.LeadStage == leadStageCustomer || assessment.Stage == leadStageCustomer || assessment.Stage == leadStageNew {
		return false
	}
	threshold := 0.72
	if assessment.Stage == leadStageHot {
		threshold = 0.82
	} else if assessment.Stage == leadStageUnqualified {
		threshold = 0.9
	}
	if assessment.Confidence < threshold {
		return false
	}
	// Sinyal aktivitas eksplisit (form/checkout) tidak boleh diturunkan AI.
	if contact.LeadStageSource == "activity" && crmStageRank(assessment.Stage) < crmStageRank(contact.LeadStage) {
		return false
	}
	// Hindari status berayun. AI boleh menaikkan minat; penurunan dilakukan manual.
	if contact.LeadStageSource == "ai" && assessment.Stage != leadStageUnqualified && crmStageRank(assessment.Stage) < crmStageRank(contact.LeadStage) {
		return false
	}
	// Jika nilainya sama, AI tetap boleh menyimpan alasan pada status lama/system
	// atau status manual yang sudah dibuka kembali. Sumber aktivitas dipertahankan.
	return assessment.Stage != contact.LeadStage || contact.LeadStageSource != "activity"
}

func crmStageRank(stage string) int {
	switch stage {
	case leadStageCold:
		return 1
	case leadStageWarm:
		return 2
	case leadStageHot:
		return 3
	case leadStageCustomer:
		return 4
	default:
		return 0
	}
}
