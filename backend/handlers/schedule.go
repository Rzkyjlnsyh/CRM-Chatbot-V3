package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

type scheduleRecipient struct {
	Number string `json:"number"`
	Name   string `json:"name"`
}

// CreateSchedule menjadwalkan broadcast untuk waktu tertentu (multipart, bisa dengan lampiran).
func CreateSchedule(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	tid := currentTenantID(c)

	productIDRaw := strings.TrimSpace(c.PostForm("product_id"))
	message, productMediaType, productMediaPath, productFileName, productMimetype, productErr := productBroadcastPayload(id, productIDRaw, c.PostForm("message"))
	if productErr != nil {
		c.JSON(400, gin.H{"error": productErr.Error()})
		return
	}
	productID, productButtonsJSON, productButtonsErr := productBroadcastButtons(id, productIDRaw)
	if productButtonsErr != nil {
		c.JSON(400, gin.H{"error": productButtonsErr.Error()})
		return
	}
	if strings.TrimSpace(message) == "" {
		c.JSON(400, gin.H{"error": "Pesan wajib diisi"})
		return
	}
	runAt, err := time.Parse(time.RFC3339, c.PostForm("run_at"))
	if err != nil {
		c.JSON(400, gin.H{"error": "Waktu jadwal tidak valid"})
		return
	}
	if runAt.Before(time.Now().Add(-time.Minute)) {
		c.JSON(400, gin.H{"error": "Waktu jadwal sudah lewat"})
		return
	}

	// target_type: "group" = post ke dalam grup (penerima = JID grup), selain itu nomor pribadi.
	targetType := "number"
	if c.PostForm("target_type") == "group" {
		targetType = "group"
	}

	var reqRecipients []scheduleRecipient
	if json.Unmarshal([]byte(c.PostForm("recipients")), &reqRecipients) != nil || len(reqRecipients) == 0 {
		c.JSON(400, gin.H{"error": "Penerima wajib diisi"})
		return
	}
	if len(reqRecipients) > 1000 {
		c.JSON(400, gin.H{"error": "Maksimal 1000 penerima per jadwal"})
		return
	}
	seen := map[string]bool{}
	clean := make([]scheduleRecipient, 0, len(reqRecipients))
	for _, r := range reqRecipients {
		var key string
		if targetType == "group" {
			// Penerima grup: kolom Number berisi JID grup. Jangan normalisasi sebagai nomor.
			key = strings.TrimSpace(r.Number)
			if key == "" || !services.IsGroupJID(key) || seen[key] {
				continue
			}
		} else {
			key = services.NormalizePhone(r.Number)
			if key == "" || seen[key] {
				continue
			}
		}
		seen[key] = true
		clean = append(clean, scheduleRecipient{Number: key, Name: strings.TrimSpace(r.Name)})
	}
	if len(clean) == 0 {
		if targetType == "group" {
			c.JSON(400, gin.H{"error": "Tidak ada grup valid"})
		} else {
			c.JSON(400, gin.H{"error": "Tidak ada nomor valid"})
		}
		return
	}
	minD, _ := strconv.Atoi(c.PostForm("min_delay"))
	maxD, _ := strconv.Atoi(c.PostForm("max_delay"))
	minD, maxD = normalizeBroadcastDelay(minD, maxD)
	restEvery, _ := strconv.Atoi(c.PostForm("rest_every"))
	restDuration, _ := strconv.Atoi(c.PostForm("rest_duration"))
	restEvery, restDuration = normalizeBroadcastRest(restEvery, restDuration)

	// Jadwal murni: simpan apa adanya tanpa gating consent/risiko. Pengaman tetap
	// berjalan saat jadwal dieksekusi (fireScheduled -> runBroadcast): opt-out
	// (STOP) dilewati, kuota & jeda dihormati.
	recJSON, err := json.Marshal(clean)
	if err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyiapkan penerima"})
		return
	}

	s := models.ScheduledMessage{
		TenantID: tid, AgentID: id, RunAt: runAt, Message: message, TargetType: targetType,
		ProductID: productID, ProductButtonsJSON: productButtonsJSON,
		Recipients: string(recJSON), RecipientCount: len(clean),
		MinDelay: minD, MaxDelay: maxD, RestEvery: restEvery, RestDuration: restDuration, Status: "scheduled",
	}
	if productMediaPath != "" {
		s.MediaType = productMediaType
		s.MediaPath = productMediaPath
		s.FileName = productFileName
		s.Mimetype = productMimetype
	}
	if fh, ferr := c.FormFile("file"); ferr == nil {
		if productMediaPath != "" {
			c.JSON(400, gin.H{"error": "Produk tidak bisa digabung dengan lampiran file manual"})
			return
		}
		f, e := fh.Open()
		if e != nil {
			c.JSON(400, gin.H{"error": "Gagal membaca lampiran"})
			return
		}
		defer f.Close()
		data, readErr := io.ReadAll(f)
		if readErr != nil {
			c.JSON(400, gin.H{"error": "Gagal membaca lampiran"})
			return
		}
		s.Mimetype = fh.Header.Get("Content-Type")
		if s.Mimetype == "" {
			s.Mimetype = "application/octet-stream"
		}
		s.FileName = fh.Filename
		s.MediaType = "document"
		if strings.HasPrefix(s.Mimetype, "image/") {
			s.MediaType = "image"
		} else if strings.HasPrefix(s.Mimetype, "video/") {
			s.MediaType = "video"
		}
		s.MediaPath = storeMedia(id, data, s.Mimetype, fh.Filename)
	}
	if err := database.DB.Create(&s).Error; err != nil {
		log.Printf("Gagal membuat jadwal agent %d: %v", id, err)
		c.JSON(500, gin.H{"error": "Gagal membuat jadwal"})
		return
	}
	c.JSON(200, gin.H{"data": s})
}

// ListSchedules = daftar jadwal agent.
func ListSchedules(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var ss []models.ScheduledMessage
	database.DB.Where("agent_id = ?", id).Order("run_at asc").Limit(300).Find(&ss)
	c.JSON(200, gin.H{"data": ss})
}

// CancelSchedule membatalkan jadwal yang belum jalan.
func CancelSchedule(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	database.DB.Model(&models.ScheduledMessage{}).
		Where("id = ? AND agent_id = ? AND status = ?", c.Param("sid"), id, "scheduled").
		Update("status", "cancelled")
	c.JSON(200, gin.H{"ok": true})
}

// StartScheduler mengecek jadwal & follow-up yang jatuh tempo tiap menit & menjalankannya.
func StartScheduler() {
	StartSchedulerCtx(context.Background())
}

// StartSchedulerCtx adalah versi lifecycle-aware dari scheduler; berhenti saat ctx dibatalkan.
func StartSchedulerCtx(ctx context.Context) {
	go func() {
		safeRun("processDueSchedules", processDueSchedules)
		go safeRun("processDueFollowUps", processDueFollowUps)
		t := time.NewTicker(1 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("Scheduler berhenti")
				return
			case <-t.C:
				safeRun("processDueSchedules", processDueSchedules)
				// Follow-up dijalankan terpisah agar tak menahan jadwal.
				go safeRun("processDueFollowUps", processDueFollowUps)
			}
		}
	}()
}

func processDueSchedules() {
	var due []models.ScheduledMessage
	if err := database.DB.Where("status = ? AND run_at <= ?", "scheduled", time.Now()).Find(&due).Error; err != nil {
		log.Printf("Scheduler query error: %v", err)
		return
	}
	for _, s := range due {
		if !services.WA(s.AgentID).IsConnected() {
			continue // WA belum tersambung -> tunda, coba lagi menit berikutnya
		}
		if err := database.DB.Model(&models.ScheduledMessage{}).Where("id = ? AND status = ?", s.ID, "scheduled").Update("status", "running").Error; err != nil {
			log.Printf("Scheduler gagal update jadwal %d: %v", s.ID, err)
			continue
		}
		fireScheduled(s)
	}
}

// fireScheduled membuat broadcast dari jadwal lalu menjalankannya.
func fireScheduled(s models.ScheduledMessage) {
	var recs []scheduleRecipient
	if err := json.Unmarshal([]byte(s.Recipients), &recs); err != nil {
		log.Printf("Scheduler gagal parse penerima jadwal %d: %v", s.ID, err)
		_ = database.DB.Model(&models.ScheduledMessage{}).Where("id = ?", s.ID).Update("status", "failed").Error
		return
	}

	b := models.Broadcast{
		TenantID: s.TenantID, AgentID: s.AgentID, Message: s.Message, Status: "pending", TargetType: s.TargetType,
		ProductID: s.ProductID, ProductButtonsJSON: s.ProductButtonsJSON,
		MediaType: s.MediaType, MediaPath: s.MediaPath, FileName: s.FileName, Mimetype: s.Mimetype,
		ConsentCategory: s.ConsentCategory, ConsentSource: s.ConsentSource,
		RiskLevel: s.RiskLevel, RiskReasons: s.RiskReasons, RiskAcknowledged: s.RiskAcknowledged,
		OverrideReason: s.OverrideReason, OverrideBy: s.OverrideBy, OverrideAt: s.OverrideAt,
		MinDelay: s.MinDelay, MaxDelay: s.MaxDelay, RestEvery: s.RestEvery, RestDuration: s.RestDuration,
	}
	var recipients []models.BroadcastRecipient
	for _, r := range recs {
		recipients = append(recipients, models.BroadcastRecipient{Number: r.Number, Name: r.Name, Status: "pending"})
	}
	b.Total = len(recipients)
	if err := database.DB.Create(&b).Error; err != nil {
		log.Printf("Scheduler gagal membuat broadcast jadwal %d: %v", s.ID, err)
		_ = database.DB.Model(&models.ScheduledMessage{}).Where("id = ?", s.ID).Update("status", "failed").Error
		return
	}
	for i := range recipients {
		recipients[i].BroadcastID = b.ID
	}
	if len(recipients) > 0 {
		if err := database.DB.Create(&recipients).Error; err != nil {
			log.Printf("Scheduler gagal membuat penerima broadcast %d: %v", b.ID, err)
			_ = database.DB.Model(&models.ScheduledMessage{}).Where("id = ?", s.ID).Update("status", "failed").Error
			return
		}
	}
	// Status jadwal tetap "running"; disinkronkan ke hasil akhir broadcast oleh finishBroadcast.
	if err := database.DB.Model(&models.ScheduledMessage{}).Where("id = ?", s.ID).Update("broadcast_id", b.ID).Error; err != nil {
		log.Printf("Scheduler gagal link broadcast jadwal %d: %v", s.ID, err)
	}

	startBroadcastWorker(b.ID, s.AgentID, s.MinDelay, s.MaxDelay)
}

// CleanupStuckSchedules menandai jadwal yang "running" saat server mati sebagai interrupted.
func CleanupStuckSchedules() {
	if err := database.DB.Model(&models.ScheduledMessage{}).Where("status = ?", "running").Update("status", "interrupted").Error; err != nil {
		log.Printf("Cleanup stuck schedule gagal: %v", err)
	}
}
