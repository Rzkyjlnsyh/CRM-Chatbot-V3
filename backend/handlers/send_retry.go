package handlers

import (
	"context"
	"log"
	"strings"
	"time"

	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

func deliveryFields(sendErr error) (status string, errMsg string, nextRetryAt *time.Time) {
	if sendErr == nil {
		return "sent", "", nil
	}
	next := time.Now().Add(retryDelay(0))
	return "pending_retry", sendErr.Error(), &next
}

func retryDelay(retryCount int) time.Duration {
	base := time.Duration(config.EnvInt("SEND_RETRY_BASE_SECONDS", 60)) * time.Second
	if base <= 0 {
		base = time.Minute
	}
	if retryCount < 0 {
		retryCount = 0
	}
	multiplier := 1 << minInt(retryCount, 5)
	return time.Duration(multiplier) * base
}

func maxSendRetries() int {
	if n := config.EnvInt("SEND_RETRY_MAX", 3); n >= 0 {
		return n
	}
	return 3
}

func sendRetryInterval() time.Duration {
	interval := time.Duration(config.EnvInt("SEND_RETRY_INTERVAL_SECONDS", 30)) * time.Second
	if interval <= 0 {
		return 30 * time.Second
	}
	return interval
}

func sendRetryBatchSize() int {
	batch := config.EnvInt("SEND_RETRY_BATCH", 20)
	if batch <= 0 {
		return 20
	}
	if batch > 200 {
		return 200
	}
	return batch
}

// StartFailedSendRetry mencoba ulang chat AI/manual yang gagal terkirim ke WhatsApp.
func StartFailedSendRetry(ctx context.Context) {
	go func() {
		safeRun("retryFailedSends", retryFailedSends)
		ticker := time.NewTicker(sendRetryInterval())
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("Failed-send retry worker berhenti")
				return
			case <-ticker.C:
				safeRun("retryFailedSends", retryFailedSends)
			}
		}
	}()
}

func retryFailedSends() {
	var rows []models.ChatHistory
	if err := database.DB.Where("delivery_status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?", "pending_retry", time.Now()).
		Order("next_retry_at asc").Limit(sendRetryBatchSize()).Find(&rows).Error; err != nil {
		log.Printf("failed-send retry query error: %v", err)
		return
	}
	for _, row := range rows {
		if strings.TrimSpace(row.Reply) == "" || row.Sender == "" {
			markSendFailed(row, "payload retry kosong")
			continue
		}
		if !services.WA(row.AgentID).IsConnected() {
			rescheduleSendRetry(row, "client WA tidak terhubung")
			continue
		}
		if err := services.WA(row.AgentID).SendText(row.Sender, row.Reply); err != nil {
			rescheduleSendRetry(row, err.Error())
			continue
		}
		if err := database.DB.Model(&models.ChatHistory{}).Where("id = ?", row.ID).Updates(map[string]any{
			"delivery_status": "sent",
			"send_error":      "",
			"next_retry_at":   nil,
		}).Error; err != nil {
			log.Printf("failed-send retry update sent error (chat %d): %v", row.ID, err)
		}
	}
}

func rescheduleSendRetry(row models.ChatHistory, errMsg string) {
	nextCount := row.RetryCount + 1
	if nextCount > maxSendRetries() {
		markSendFailed(row, errMsg)
		return
	}
	next := time.Now().Add(retryDelay(nextCount))
	if err := database.DB.Model(&models.ChatHistory{}).Where("id = ? AND delivery_status = ?", row.ID, "pending_retry").Updates(map[string]any{
		"retry_count":   nextCount,
		"send_error":    errMsg,
		"next_retry_at": &next,
	}).Error; err != nil {
		log.Printf("failed-send retry reschedule error (chat %d): %v", row.ID, err)
	}
}

func markSendFailed(row models.ChatHistory, errMsg string) {
	if err := database.DB.Model(&models.ChatHistory{}).Where("id = ?", row.ID).Updates(map[string]any{
		"delivery_status": "failed_send",
		"send_error":      errMsg,
		"next_retry_at":   nil,
	}).Error; err != nil {
		log.Printf("failed-send retry mark failed error (chat %d): %v", row.ID, err)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
