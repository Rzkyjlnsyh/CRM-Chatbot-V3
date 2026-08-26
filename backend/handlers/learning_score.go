package handlers

// Skor agen: satu angka 0-100 dari sinyal nyata — close rate 30 hari,
// dampak pola terpelajar, dan kedisiplinan review — supaya peningkatan
// kualitas agent bisa dilihat trennya, bukan sekadar klaim.

import (
	"net/http"
	"strconv"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// LearningScoreData = komponen skor agent (dikembalikan ke dashboard).
type LearningScoreData struct {
	CloseRatePct       float64 `json:"close_rate_pct"`        // % kontak aktif 30h yang closing
	ClosingContacts    int64   `json:"closing_contacts"`      // kontak unik closing 30h
	ActiveContacts     int64   `json:"active_contacts"`       // kontak unik aktif 30h
	PatternsApplied    int64   `json:"patterns_applied"`      // pola terpelajar diterapkan
	PatternsPending    int64   `json:"patterns_pending"`      // pola menunggu review
	AvgClosingImpact   float64 `json:"avg_closing_impact"`    // rata-rata dampak pola applied
	Score              float64 `json:"score"`                 // skor gabungan 0-100
}

// GetLearningScore menghitung skor agent. Skor = rata-rata berbobot:
// close rate (60%), dampak pola terpelajar (25%), kesehatan review (15%).
func GetLearningScore(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || agentID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id agent tidak valid"})
		return
	}
	aid := uint(agentID)
	since := time.Now().AddDate(0, 0, -30)

	var activeSet = map[string]bool{}
	var activeList []string
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND created_at >= ?", aid, since).
		Distinct("sender").Pluck("sender", &activeList)
	for _, s := range activeList {
		activeSet[s] = true
	}
	// Closing = gabungan ClosingRecord (transaksi) + label WA closing (dipasang
	// CS manusia) — dibatasi ke kontak aktif 30 hari.
	closingSet := services.ClosingSenderSet(aid, &since)
	closing := int64(0)
	for s := range closingSet {
		if activeSet[s] {
			closing++
		}
	}
	active := int64(len(activeSet))

	var applied, pending int64
	database.DB.Model(&models.LearningPattern{}).
		Where("agent_id = ? AND status = ?", aid, "applied").Count(&applied)
	database.DB.Model(&models.LearningPattern{}).
		Where("agent_id = ? AND status = ?", aid, "suggested").Count(&pending)

	var avgImpact float64
	database.DB.Model(&models.LearningPattern{}).
		Where("agent_id = ? AND status = ?", aid, "applied").
		Select("COALESCE(AVG(closing_impact),0)").Scan(&avgImpact)

	closeRate := 0.0
	if active > 0 {
		closeRate = float64(closing) / float64(active)
	}
	reviewHealth := 0.0
	if applied+pending > 0 {
		reviewHealth = float64(applied) / float64(applied+pending)
	}
	score := closeRate*60 + avgImpact*25 + reviewHealth*15
	if score > 100 {
		score = 100
	}

	c.JSON(http.StatusOK, gin.H{"data": LearningScoreData{
		CloseRatePct:     closeRate * 100,
		ClosingContacts:  closing,
		ActiveContacts:   active,
		PatternsApplied:  applied,
		PatternsPending:  pending,
		AvgClosingImpact: avgImpact,
		Score:            score,
	}})
}
