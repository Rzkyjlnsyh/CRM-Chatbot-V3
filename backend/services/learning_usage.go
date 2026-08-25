package services

// Closed-loop learning: mengukur pola terpelajar mana yang benar-benar
// dipakai di percakapan dan diikuti closing — bukan hanya estimasi AI.
// Alur: LogLearningUsage saat pola dipakai membalas → MarkUsageClosed saat
// pelanggan closing → ClosingImpact pola = rasio closed/total pemakaian.

import (
	"log"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// LogLearningUsage mencatat knowledge hasil pembelajaran yang dipakai dalam
// satu balasan (knowledgeIDs = jejak retrieval ChatWithKnowledge).
func LogLearningUsage(agentID uint, sender string, knowledgeIDs string) {
	if sender == "" || strings.TrimSpace(knowledgeIDs) == "" {
		return
	}
	var kids []uint
	for _, part := range strings.Split(knowledgeIDs, ",") {
		if id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			kids = append(kids, uint(id))
		}
	}
	if len(kids) == 0 {
		return
	}

	// Hanya knowledge ber-sumber learning yang dihitung (pola terpelajar).
	var patterns []models.LearningPattern
	database.DB.Where("agent_id = ? AND knowledge_id IN ? AND status = ?", agentID, kids, "applied").
		Find(&patterns)
	if len(patterns) == 0 {
		return
	}

	// chatloop = single-tenant: tidak ada kolom tenant_id di model belajar.
	now := time.Now()
	for _, p := range patterns {
		database.DB.Model(&models.LearningPattern{}).Where("id = ?", p.ID).
			UpdateColumn("usage_count", p.UsageCount+1)
		database.DB.Create(&models.PatternUsageLog{
			PatternID: p.ID,
			AgentID:   agentID,
			Sender:    sender,
			CreatedAt: now,
		})
	}
}

// MarkUsageClosed dipanggil saat pelanggan closing (order terdeteksi): semua
// pola terpelajar yang pernah dipakai untuk pelanggan ini ditandai closed,
// lalu ClosingImpact setiap pola dihitung ulang dari data nyata.
func MarkUsageClosed(agentID uint, sender string) {
	if sender == "" {
		return
	}
	now := time.Now()
	var logs []models.PatternUsageLog
	database.DB.Where("agent_id = ? AND sender = ? AND closed = ?", agentID, sender, false).
		Find(&logs)
	if len(logs) == 0 {
		return
	}
	ids := make([]uint, 0, len(logs))
	for _, l := range logs {
		ids = append(ids, l.PatternID)
	}
	database.DB.Model(&models.PatternUsageLog{}).
		Where("agent_id = ? AND sender = ? AND closed = ?", agentID, sender, false).
		Updates(map[string]any{"closed": true, "closed_at": &now})

	// Hitung ulang closing impact tiap pola yang terdampak dari log nyata.
	var patterns []models.LearningPattern
	database.DB.Where("agent_id = ? AND id IN ?", agentID, ids).Find(&patterns)
	for _, p := range patterns {
		recomputeClosingImpact(p.AgentID, p.ID)
	}
}

// recomputeClosingImpact menghitung ulang ClosingImpact pola dari PatternUsageLog:
// rasio pemakaian yang diikuti closing (dengan pembobotan minimal 5 pemakaian
// agar rasio tidak liar saat sampel kecil; sampel kecil pakai rata-rata estimasi).
func recomputeClosingImpact(agentID, patternID uint) {
	var total, closed int64
	database.DB.Model(&models.PatternUsageLog{}).
		Where("agent_id = ? AND pattern_id = ?", agentID, patternID).Count(&total)
	database.DB.Model(&models.PatternUsageLog{}).
		Where("agent_id = ? AND pattern_id = ? AND closed = ?", agentID, patternID, true).Count(&closed)
	if total == 0 {
		return
	}
	var impact float64
	if total >= 5 {
		impact = float64(closed) / float64(total)
	} else {
		// Sampel kecil: gabungkan estimasi AI awal (60%) dengan rasio nyata (40%)
		// agar satu keberhasilan tidak langsung menjadikan pola "sempurna".
		var p models.LearningPattern
		if database.DB.First(&p, patternID).Error == nil {
			impact = 0.6*p.ClosingImpact + 0.4*(float64(closed)/float64(total))
		} else {
			impact = float64(closed) / float64(total)
		}
	}
	if err := database.DB.Model(&models.LearningPattern{}).Where("id = ?", patternID).
		UpdateColumn("closing_impact", impact).Error; err != nil {
		log.Printf("ClosedLoop: gagal update impact pola %d: %v", patternID, err)
	}
}
