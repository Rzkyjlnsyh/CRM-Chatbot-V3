package handlers

import (
	"log"
	"math"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

const testAITurnSender = "__test__"

func logAITurn(agentID uint, sender, userMessage, aiReply, model string, knowledgeUsedCount int, usedShippingTool, escalated bool, errText string, latencyMs int64, trace services.RetrievalTrace) {
	if agentID == 0 {
		return
	}
	// knowledgeUsedCount argument dipertahankan untuk call-site lama; prefer Trace bila terisi.
	kbCount := knowledgeUsedCount
	if trace.KnowledgeUsedCount > 0 {
		kbCount = trace.KnowledgeUsedCount
	}
	if err := database.DB.Create(&models.AITurn{
		AgentID:            agentID,
		Sender:             sender,
		UserMessage:        userMessage,
		AIReply:            aiReply,
		Model:              model,
		PromptVersion:      "v2-grounding",
		KnowledgeUsedCount: kbCount,
		KnowledgeIDs:       trace.KnowledgeIDs,
		TopSimilarity:      trace.TopSimilarity,
		AnswerOverlap:      trace.AnswerOverlap,
		ProductUsedCount:   trace.ProductUsedCount,
		ProductIDs:         trace.ProductIDs,
		RetrievalMode:      trace.RetrievalMode,
		RetrievalQuery:     trace.RetrievalQuery,
		GroundingRetried:   trace.GroundingRetried,
		GroundingFallback:  trace.GroundingFallback,
		UsedShippingTool:   usedShippingTool,
		Escalated:          escalated,
		Error:              errText,
		LatencyMs:          latencyMs,
	}).Error; err != nil {
		log.Printf("Gagal mencatat AITurn (agent %d, %s): %v", agentID, sender, err)
	}
}

func AgentAIMetrics(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}

	now := time.Now()
	since := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)
	var totalIncoming, aiReplies, escalated, toolShippingSuccess, toolShippingError, closingDetected, closingExported, aiErrors int64
	var knowledgeHits, groundingRetried, groundingFallback, productHits int64

	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND message <> '' AND created_at >= ?", id, since).
		Count(&totalIncoming)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND ai_reply <> '' AND created_at >= ?", id, testAITurnSender, since).
		Count(&aiReplies)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND escalated = ? AND created_at >= ?", id, testAITurnSender, true, since).
		Count(&escalated)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND used_shipping_tool = ? AND (`error` = '' OR `error` NOT LIKE ?) AND created_at >= ?", id, testAITurnSender, true, "shipping:%", since).
		Count(&toolShippingSuccess)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND used_shipping_tool = ? AND `error` LIKE ? AND created_at >= ?", id, testAITurnSender, true, "shipping:%", since).
		Count(&toolShippingError)
	database.DB.Model(&models.ClosingRecord{}).
		Where("agent_id = ? AND created_at >= ?", id, since).
		Count(&closingDetected)
	database.DB.Model(&models.ClosingRecord{}).
		Where("agent_id = ? AND status = ? AND created_at >= ?", id, "exported", since).
		Count(&closingExported)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND `error` LIKE ? AND created_at >= ?", id, testAITurnSender, "ai:%", since).
		Count(&aiErrors)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND knowledge_used_count > 0 AND created_at >= ?", id, testAITurnSender, since).
		Count(&knowledgeHits)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND grounding_retried = ? AND created_at >= ?", id, testAITurnSender, true, since).
		Count(&groundingRetried)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND grounding_fallback = ? AND created_at >= ?", id, testAITurnSender, true, since).
		Count(&groundingFallback)
	database.DB.Model(&models.AITurn{}).
		Where("agent_id = ? AND sender <> ? AND product_used_count > 0 AND created_at >= ?", id, testAITurnSender, since).
		Count(&productHits)

	type avgRow struct {
		AvgOverlap float64
		AvgSim     float64
	}
	var avgs avgRow
	database.DB.Model(&models.AITurn{}).
		Select("AVG(CASE WHEN knowledge_used_count > 0 THEN answer_overlap END) as avg_overlap, AVG(CASE WHEN top_similarity > 0 THEN top_similarity END) as avg_sim").
		Where("agent_id = ? AND sender <> ? AND created_at >= ?", id, testAITurnSender, since).
		Scan(&avgs)

	escalationRate := 0.0
	if totalIncoming > 0 {
		escalationRate = math.Round((float64(escalated)/float64(totalIncoming))*10000) / 100
	}
	knowledgeHitRate := 0.0
	if aiReplies > 0 {
		knowledgeHitRate = math.Round((float64(knowledgeHits)/float64(aiReplies))*10000) / 100
	}

	type totalRow struct {
		Date  string
		Total int64
	}
	type escalatedRow struct {
		Date      string
		Escalated int64
	}
	var totalRows []totalRow
	var escalatedRows []escalatedRow
	database.DB.Model(&models.ChatHistory{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') as date, COUNT(*) as total").
		Where("agent_id = ? AND message <> '' AND created_at >= ?", id, since).
		Group("date").
		Scan(&totalRows)
	database.DB.Model(&models.AITurn{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') as date, SUM(CASE WHEN escalated = true THEN 1 ELSE 0 END) as escalated").
		Where("agent_id = ? AND sender <> ? AND created_at >= ?", id, testAITurnSender, since).
		Group("date").
		Scan(&escalatedRows)

	totalsByDate := map[string]int64{}
	for _, row := range totalRows {
		totalsByDate[row.Date] = row.Total
	}
	escalatedByDate := map[string]int64{}
	for _, row := range escalatedRows {
		escalatedByDate[row.Date] = row.Escalated
	}

	trend := make([]gin.H, 0, 7)
	for i := 6; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		trend = append(trend, gin.H{
			"date":      date,
			"total":     totalsByDate[date],
			"escalated": escalatedByDate[date],
		})
	}

	c.JSON(200, gin.H{
		"total_incoming":        totalIncoming,
		"ai_replies":            aiReplies,
		"escalated":             escalated,
		"escalation_rate":       escalationRate,
		"tool_shipping_success": toolShippingSuccess,
		"tool_shipping_error":   toolShippingError,
		"closing_detected":      closingDetected,
		"closing_exported":      closingExported,
		"ai_errors":             aiErrors,
		"knowledge_hits":        knowledgeHits,
		"knowledge_hit_rate":    knowledgeHitRate,
		"avg_answer_overlap":    math.Round(avgs.AvgOverlap*1000) / 1000,
		"avg_top_similarity":    math.Round(avgs.AvgSim*1000) / 1000,
		"grounding_retried":     groundingRetried,
		"grounding_fallback":    groundingFallback,
		"product_hits":          productHits,
		"trend":                 trend,
	})
}
