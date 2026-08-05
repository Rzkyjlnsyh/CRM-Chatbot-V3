package handlers

import (
	"encoding/json"
	"strconv"
	"strings"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// apiBroadcastReq = body untuk POST /api/v1/broadcasts (broadcast massal via API).
type apiBroadcastReq struct {
	Message      string                    `json:"message"`
	Recipients   []broadcastGuardRecipient `json:"recipients"`
	MinDelay     int                       `json:"min_delay"`
	MaxDelay     int                       `json:"max_delay"`
	RestEvery    int                       `json:"rest_every"`
	RestDuration int                       `json:"rest_duration"`
	AgentIDs     []uint                    `json:"agent_ids"`
}

// APICreateBroadcast membuat broadcast massal lewat API. Memakai ULANG pipeline broadcast yang
// sama (startBroadcastWorker) sehingga semua pengaman ikut berjalan: opt-out (STOP) dilewati,
// plus jitter & istirahat berkala antar pesan.
func APICreateBroadcast(c *gin.Context) {
	agent := apiAgent(c)
	id, tid := agent.ID, agent.TenantID

	var req apiBroadcastReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "JSON tidak valid."})
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		c.JSON(400, gin.H{"error": "message wajib diisi."})
		return
	}
	if len(req.Recipients) == 0 {
		c.JSON(400, gin.H{"error": "recipients wajib diisi."})
		return
	}
	if len(req.Recipients) > 1000 {
		c.JSON(400, gin.H{"error": "Maksimal 1000 penerima per broadcast."})
		return
	}
	if !services.WA(id).IsConnected() {
		c.JSON(400, gin.H{"error": "WhatsApp belum tersambung."})
		return
	}

	// Rotasi nomor opsional: semua harus milik tenant & sedang tersambung.
	pool := []uint{id}
	seen := map[uint]bool{id: true}
	for _, aid := range req.AgentIDs {
		if aid == 0 || seen[aid] {
			continue
		}
		var cnt int64
		database.DB.Model(&models.Agent{}).Where("id = ? AND tenant_id = ?", aid, tid).Count(&cnt)
		if cnt == 0 {
			c.JSON(400, gin.H{"error": "Ada nomor rotasi yang tidak dikenal."})
			return
		}
		if !services.WA(aid).IsConnected() {
			c.JSON(400, gin.H{"error": "Semua nomor rotasi harus tersambung dulu."})
			return
		}
		seen[aid] = true
		pool = append(pool, aid)
	}

	// Tolak bila ada broadcast yang dijeda WhatsApp — harus dituntaskan dulu.
	var paused int64
	database.DB.Model(&models.Broadcast{}).
		Where("agent_id = ? AND status = ?", id, models.BroadcastWARestricted).Count(&paused)
	if paused > 0 {
		c.JSON(409, gin.H{"error": "Ada broadcast yang dijeda WhatsApp. Lanjutkan atau batalkan dulu."})
		return
	}

	recips := normalizeGuardRecipients(req.Recipients)
	if len(recips) == 0 {
		c.JSON(400, gin.H{"error": "Tidak ada nomor valid."})
		return
	}

	minD, maxD := normalizeBroadcastDelay(req.MinDelay, req.MaxDelay)
	restEvery, restDuration := normalizeBroadcastRest(req.RestEvery, req.RestDuration)

	b := models.Broadcast{
		TenantID:     tid,
		AgentID:      id,
		Message:      req.Message,
		Status:       "pending",
		Total:        len(recips),
		MinDelay:     minD,
		MaxDelay:     maxD,
		RestEvery:    restEvery,
		RestDuration: restDuration,
	}
	if len(pool) > 1 {
		if poolJSON, err := json.Marshal(pool); err == nil {
			b.AgentIDs = string(poolJSON)
		}
	}

	rows := make([]models.BroadcastRecipient, 0, len(recips))
	for _, r := range recips {
		rows = append(rows, models.BroadcastRecipient{Number: r.Number, Name: r.Name, AgentID: stickyAgent(r.Number, pool), Status: "pending"})
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&b).Error; err != nil {
			return err
		}
		for i := range rows {
			rows[i].BroadcastID = b.ID
		}
		return tx.Create(&rows).Error
	}); err != nil {
		c.JSON(500, gin.H{"error": "Broadcast belum bisa dibuat."})
		return
	}

	startBroadcastWorker(b.ID, id, minD, maxD)
	c.JSON(200, gin.H{"id": b.ID, "total": b.Total, "status": b.Status})
}

// APIBroadcastStatus mengembalikan progres sebuah broadcast (untuk polling via API).
func APIBroadcastStatus(c *gin.Context) {
	agent := apiAgent(c)
	var b models.Broadcast
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("id"), agent.ID).First(&b).Error != nil {
		c.JSON(404, gin.H{"error": "Broadcast tidak ditemukan."})
		return
	}
	c.JSON(200, gin.H{
		"id": b.ID, "status": b.Status, "total": b.Total,
		"sent": b.Sent, "failed": b.Failed, "skipped": b.Skipped,
		"message": b.Message, "created_at": b.CreatedAt, "updated_at": b.UpdatedAt,
	})
}

func APIListBroadcasts(c *gin.Context) {
	agent := apiAgent(c)
	p := parseAPIPage(c, 20, 100)
	q := database.DB.Model(&models.Broadcast{}).Where("agent_id = ?", agent.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	q.Count(&total)
	var rows []models.Broadcast
	q.Order("id desc").Offset(p.Offset).Limit(p.PerPage).Find(&rows)
	c.JSON(200, gin.H{"data": rows, "meta": apiPageMeta(p, total)})
}

func APIBroadcastRecipients(c *gin.Context) {
	agent := apiAgent(c)
	var broadcast models.Broadcast
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("id"), agent.ID).First(&broadcast).Error != nil {
		c.JSON(404, gin.H{"error": "Broadcast tidak ditemukan."})
		return
	}
	p := parseAPIPage(c, 100, 500)
	q := database.DB.Model(&models.BroadcastRecipient{}).Where("broadcast_id = ?", broadcast.ID)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	q.Count(&total)
	var rows []models.BroadcastRecipient
	q.Order("id asc").Offset(p.Offset).Limit(p.PerPage).Find(&rows)
	c.JSON(200, gin.H{"data": rows, "meta": apiPageMeta(p, total)})
}

func APICancelBroadcast(c *gin.Context) {
	agent := apiAgent(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "ID broadcast tidak valid."})
		return
	}
	var broadcast models.Broadcast
	if database.DB.Where("id = ? AND agent_id = ?", id, agent.ID).First(&broadcast).Error != nil {
		c.JSON(404, gin.H{"error": "Broadcast tidak ditemukan."})
		return
	}
	switch broadcast.Status {
	case models.BroadcastDone, models.BroadcastFailed, models.BroadcastCancelled:
		c.JSON(409, gin.H{"error": "Broadcast sudah selesai dan tidak bisa dibatalkan."})
		return
	}
	if broadcast.Status == models.BroadcastPending || broadcast.Status == models.BroadcastInterrupted || broadcast.Status == models.BroadcastWARestricted {
		finalizeCancelledBroadcast(broadcast.ID)
		c.JSON(200, gin.H{"status": models.BroadcastCancelled})
		return
	}
	result := database.DB.Model(&models.Broadcast{}).
		Where("id = ? AND status IN ?", broadcast.ID, []string{models.BroadcastRunning, models.BroadcastResuming}).
		Update("status", models.BroadcastCancelRequested)
	if result.Error != nil {
		c.JSON(500, gin.H{"error": "Broadcast belum bisa dibatalkan."})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(409, gin.H{"error": "Status broadcast sudah berubah. Ambil status terbaru lalu coba lagi."})
		return
	}
	c.JSON(200, gin.H{"status": models.BroadcastCancelRequested})
}
