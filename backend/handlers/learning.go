package handlers

import (
	"context"
	"log"
	"strconv"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// --- Learning API Handlers ---

// StartLearning godoc
// POST /api/agents/:id/learning/run
func StartLearning(c *gin.Context) {
	agentID := currentAgentID(c)

	var req struct {
		StartDate string `json:"start_date"` // optional, format "2006-01-02"
		EndDate   string `json:"end_date"`   // optional
	}
	c.ShouldBindJSON(&req)

	var startDate, endDate *time.Time
	if req.StartDate != "" {
		t, err := time.Parse("2006-01-02", req.StartDate)
		if err != nil {
			c.JSON(400, gin.H{"error": "Format start_date salah. Gunakan YYYY-MM-DD"})
			return
		}
		startDate = &t
	}
	if req.EndDate != "" {
		t, err := time.Parse("2006-01-02", req.EndDate)
		if err != nil {
			c.JSON(400, gin.H{"error": "Format end_date salah. Gunakan YYYY-MM-DD"})
			return
		}
		endDate = &t
	}

	result, err := services.EnqueueLearningRun(agentID, startDate, endDate)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"data": gin.H{
		"run_id":  result.ID,
		"status":  "pending",
		"message": "Learning dimulai di background. Cek status secara berkala.",
	}})
}

// GetLearningRuns godoc
// GET /api/agents/:id/learning/runs
func GetLearningRuns(c *gin.Context) {
	agentID := currentAgentID(c)
	var runs []models.LearningRun
	database.DB.Where("agent_id = ?", agentID).Order("created_at desc").Limit(20).Find(&runs)
	c.JSON(200, gin.H{"data": runs})
}

// GetLearningRun godoc
// GET /api/agents/:id/learning/runs/:rid
func GetLearningRun(c *gin.Context) {
	agentID := currentAgentID(c)
	runID, err := strconv.ParseUint(c.Param("rid"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "ID run tidak valid"})
		return
	}

	var run models.LearningRun
	if database.DB.Where("agent_id = ? AND id = ?", agentID, runID).First(&run).Error != nil {
		c.JSON(404, gin.H{"error": "Learning run tidak ditemukan"})
		return
	}

	var patterns []models.LearningPattern
	database.DB.Where("learning_run_id = ?", run.ID).Order("confidence desc").Find(&patterns)

	c.JSON(200, gin.H{"data": gin.H{"run": run, "patterns": patterns}})
}

// GetLearningPatterns godoc
// GET /api/agents/:id/learning/patterns
func GetLearningPatterns(c *gin.Context) {
	agentID := currentAgentID(c)
	status := c.DefaultQuery("status", "suggested") // suggested, applied, rejected, all

	var patterns []models.LearningPattern
	q := database.DB.Where("agent_id = ?", agentID)
	if status != "all" {
		q = q.Where("status = ?", status)
	}
	q.Order("confidence desc").Limit(50).Find(&patterns)
	c.JSON(200, gin.H{"data": patterns})
}

// ApplyLearningPattern godoc
// POST /api/agents/:id/learning/patterns/:pid/apply
func ApplyLearningPattern(c *gin.Context) {
	agentID := currentAgentID(c)
	patternID, err := strconv.ParseUint(c.Param("pid"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "ID pola tidak valid"})
		return
	}

	k, err := services.ApplyPattern(agentID, uint(patternID))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": gin.H{"pattern_id": patternID, "knowledge": k, "message": "Pola berhasil diterapkan ke knowledge base"}})
}

// RejectLearningPattern godoc
// POST /api/agents/:id/learning/patterns/:pid/reject
func RejectLearningPattern(c *gin.Context) {
	agentID := currentAgentID(c)
	patternID, err := strconv.ParseUint(c.Param("pid"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "ID pola tidak valid"})
		return
	}

	if err := services.RejectPattern(agentID, uint(patternID)); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": gin.H{"message": "Pola ditolak"}})
}

// ApplyAllPatterns menerapkan SEMUA pola suggested dgn confidence >= threshold.
// POST /api/agents/:id/learning/patterns/apply-all
func ApplyAllPatterns(c *gin.Context) {
	agentID := currentAgentID(c)

	var req struct {
		MinConfidence float64 `json:"min_confidence"`
	}
	c.ShouldBindJSON(&req)
	if req.MinConfidence <= 0 {
		req.MinConfidence = 0.6
	}

	var patterns []models.LearningPattern
	database.DB.Where("agent_id = ? AND status = ? AND confidence >= ?", agentID, "suggested", req.MinConfidence).
		Order("confidence desc").Find(&patterns)

	applied := 0
	for _, p := range patterns {
		if _, err := services.ApplyPattern(agentID, p.ID); err == nil {
			applied++
		}
	}

	c.JSON(200, gin.H{"data": gin.H{"total": len(patterns), "applied": applied, "message": "Pola berhasil diterapkan"}})
}

// --- Snapshots ---

// GetSnapshots godoc
// GET /api/agents/:id/learning/snapshots
func GetSnapshots(c *gin.Context) {
	agentID := currentAgentID(c)
	var snaps []models.LearningSnapshot
	database.DB.Where("agent_id = ?", agentID).Order("created_at desc").Limit(20).Find(&snaps)
	c.JSON(200, gin.H{"data": snaps})
}

// CreateSnapshotAPI godoc
// POST /api/agents/:id/learning/snapshots
func CreateSnapshotAPI(c *gin.Context) {
	agentID := currentAgentID(c)

	var req struct {
		Label string `json:"label"`
	}
	c.ShouldBindJSON(&req)
	if req.Label == "" {
		req.Label = time.Now().Format("Backup 2006-01-02 15:04")
	}

	snap, err := services.CreateSnapshot(agentID, req.Label, nil)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"data": snap})
}

// RollbackSnapshot godoc
// POST /api/agents/:id/learning/snapshots/:sid/rollback
func RollbackSnapshot(c *gin.Context) {
	agentID := currentAgentID(c)
	snapID, err := strconv.ParseUint(c.Param("sid"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "ID snapshot tidak valid"})
		return
	}

	snap, err := services.RollbackToSnapshot(agentID, uint(snapID))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": snap})
}

// --- Config ---

// GetLearningConfigAPI godoc
// GET /api/agents/:id/learning/config
func GetLearningConfigAPI(c *gin.Context) {
	agentID := currentAgentID(c)
	cfg := services.GetLearningConfig(agentID)
	c.JSON(200, gin.H{"data": cfg})
}

// SaveLearningConfigAPI godoc
// PUT /api/agents/:id/learning/config
func SaveLearningConfigAPI(c *gin.Context) {
	agentID := currentAgentID(c)

	var req struct {
		Enabled                 *bool    `json:"enabled"`
		AutoApply               *bool    `json:"auto_apply"`
		MinConfidence           *float64 `json:"min_confidence"`
		MinUsageCount           *int     `json:"min_usage_count"`
		MaxPatternsPerRun       *int     `json:"max_patterns_per_run"`
		PreserveManualKnowledge *bool    `json:"preserve_manual_knowledge"`
		ScheduleEnabled         *bool    `json:"schedule_enabled"`
		ScheduleCron            *string  `json:"schedule_cron"`
		LookbackDays            *int     `json:"lookback_days"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}

	cfg := services.GetLearningConfig(agentID)

	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.AutoApply != nil {
		cfg.AutoApply = *req.AutoApply
	}
	if req.MinConfidence != nil {
		cfg.MinConfidence = *req.MinConfidence
	}
	if req.MinUsageCount != nil {
		cfg.MinUsageCount = *req.MinUsageCount
	}
	if req.MaxPatternsPerRun != nil {
		cfg.MaxPatternsPerRun = *req.MaxPatternsPerRun
	}
	if req.PreserveManualKnowledge != nil {
		cfg.PreserveManualKnowledge = *req.PreserveManualKnowledge
	}
	if req.ScheduleEnabled != nil {
		cfg.ScheduleEnabled = *req.ScheduleEnabled
	}
	if req.ScheduleCron != nil {
		cfg.ScheduleCron = *req.ScheduleCron
	}
	if req.LookbackDays != nil {
		if *req.LookbackDays < 1 {
			c.JSON(400, gin.H{"error": "Lookback days minimal 1"})
			return
		}
		cfg.LookbackDays = *req.LookbackDays
	}

	if err := services.SaveLearningConfig(cfg); err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan konfigurasi"})
		return
	}
	c.JSON(200, gin.H{"data": cfg})
}

// GetLearningStatus godoc
// GET /api/agents/:id/learning/status
func GetLearningStatus(c *gin.Context) {
	agentID := currentAgentID(c)

	// Dapatkan run terakhir + statistik
	var lastRun models.LearningRun
	database.DB.Where("agent_id = ?", agentID).Order("id desc").First(&lastRun)

	var suggestedCount, appliedCount, rejectedCount int64
	database.DB.Model(&models.LearningPattern{}).Where("agent_id = ? AND status = ?", agentID, "suggested").Count(&suggestedCount)
	database.DB.Model(&models.LearningPattern{}).Where("agent_id = ? AND status = ?", agentID, "applied").Count(&appliedCount)
	database.DB.Model(&models.LearningPattern{}).Where("agent_id = ? AND status = ?", agentID, "rejected").Count(&rejectedCount)

	var snapCount int64
	database.DB.Model(&models.LearningSnapshot{}).Where("agent_id = ?", agentID).Count(&snapCount)

	cfg := services.GetLearningConfig(agentID)

	c.JSON(200, gin.H{"data": gin.H{
		"last_run":           lastRun,
		"patterns_suggested": suggestedCount,
		"patterns_applied":   appliedCount,
		"patterns_rejected":  rejectedCount,
		"snapshot_count":     snapCount,
		"config":             cfg,
	}})
}

// --- Inbox reply tracking ---

// TrackHumanReply mencatat bahwa reply dari inbox adalah dari CS manusia.
// Dipanggil saat CS mengirim balasan manual dari dashboard inbox.
func TrackHumanReply(agentID uint, chatID uint, source string) {
	database.DB.Model(&models.ChatHistory{}).
		Where("id = ? AND agent_id = ?", chatID, agentID).
		Update("reply_source", source)
}

// --- Scheduler Learning Otomatis ---

// StartLearningScheduler menjalankan learning otomatis untuk semua agent yang
// mengaktifkan schedule (LearningConfig.Enabled && ScheduleEnabled). Cek tiap
// jam; tiap agent minimal 23 jam antar run (hindari dobel & biaya AI berlebih).
func StartLearningScheduler(ctx context.Context) {
	runDueLearning() // catch-up saat worker boot
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("Learning scheduler berhenti")
			return
		case <-ticker.C:
			runDueLearning()
		}
	}
}

// runDueLearning mengantrikan learning untuk agent dengan jadwal aktif.
// Skip bila masih ada run pending/running, atau run terakhir < 23 jam lalu.
func runDueLearning() {
	var cfgs []models.LearningConfig
	database.DB.Where("enabled = ? AND schedule_enabled = ?", true, true).Find(&cfgs)
	for _, cfg := range cfgs {
		var active int64
		database.DB.Model(&models.LearningRun{}).
			Where("agent_id = ? AND status IN ?", cfg.AgentID, []string{"pending", "running"}).Count(&active)
		if active > 0 {
			continue
		}
		var last models.LearningRun
		database.DB.Where("agent_id = ?", cfg.AgentID).Order("id desc").First(&last)
		if last.ID > 0 && time.Since(last.CreatedAt) < 23*time.Hour {
			continue
		}
		lookback := cfg.LookbackDays
		if lookback <= 0 {
			lookback = 30
		}
		end := time.Now()
		start := end.AddDate(0, 0, -lookback)
		if _, err := services.EnqueueLearningRun(cfg.AgentID, &start, &end); err != nil {
			log.Printf("LearningScheduler: agent %d gagal diantri: %v", cfg.AgentID, err)
		} else {
			log.Printf("LearningScheduler: agent %d diantri (lookback %d hari)", cfg.AgentID, lookback)
		}
	}
}
