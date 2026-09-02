package handlers

import (
	"log"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// OnWAHistorySync = target handler HistorySync dari services WA (dipasang di
// startup). Import riwayat ke chat_histories dengan aturan:
//   - dedup pre-SELECT + seenInBatch (index lama non-unique, banyak wa_msg_id ”)
//   - repair baris legacy pesan "1\n2" yang terbelah di versi lama
//   - balasan lama diadopsi bila baris kosong (Reply=”)
//   - ReplySource='history_sync' — Learning MENGABAIKAN baris ini
//   - media: MediaMetadata (protobuf) + MediaFetchStatus='pending' (unduh on-demand)
func OnWAHistorySync(agentID uint, messages []services.HistoricalMessage) (imported int, skipped int, err error) {
	seen := map[string]bool{}
	for _, m := range messages {
		if strings.TrimSpace(m.Sender) == "" {
			skipped++
			continue
		}
		// Dedup: cek yang sudah ada di DB (per wa_msg_id bila ada).
		if m.WAMsgID != "" {
			if seen[m.WAMsgID] {
				skipped++
				continue
			}
			var cnt int64
			database.DB.Model(&models.ChatHistory{}).
				Where("agent_id = ? AND wa_msg_id = ?", agentID, m.WAMsgID).Count(&cnt)
			if cnt > 0 {
				seen[m.WAMsgID] = true
				skipped++
				continue
			}
			seen[m.WAMsgID] = true
		}

		text := strings.TrimSpace(m.Text)
		if text == "" && m.MediaType != "" {
			text = mediaPlaceholder(m.MediaType, m.FileName)
		}
		if text == "" && m.MediaType == "" {
			skipped++
			continue
		}

		row := models.ChatHistory{
			AgentID: agentID, Sender: m.Sender,
			MediaType: m.MediaType, FileName: m.FileName, Mimetype: m.Mimetype,
			WAMsgID: m.WAMsgID, ReplyTo: m.ReplyTo, ReplyText: m.ReplyText,
			DeliveryStatus: "sent", ReplySource: "history_sync",
			CreatedAt: m.Timestamp,
		}
		if m.MediaType != "" && len(m.MediaMetadata) > 0 {
			row.MediaMetadata = m.MediaMetadata
			row.MediaFetchStatus = "pending"
		}
		if m.FromMe {
			row.FromHuman = true
			row.Reply = text
		} else {
			row.Message = text
		}
		if err := database.DB.Create(&row).Error; err != nil {
			log.Printf("WA agent %d history sync insert gagal (%s): %v", agentID, m.WAMsgID, err)
			skipped++
			continue
		}
		imported++
	}

	// Repair legacy: baris dengan Reply mengandung "\n" (pesan beruntun yang
	// digabung versi lama) dipecah jadi baris sendiri. Hanya untuk baris live
	// (bukan history_sync) — baris history sync sudah bersih dari awal.
	var legacy []models.ChatHistory
	database.DB.Where("agent_id = ? AND reply <> '' AND reply LIKE ? AND reply_source <> 'history_sync'",
		agentID, "%\n%").Find(&legacy)
	for _, row := range legacy {
		parts := strings.Split(row.Reply, "\n")
		if len(parts) < 2 {
			continue
		}
		// Baris pertama dipertahankan, sisanya jadi baris baru.
		first := parts[0]
		if strings.TrimSpace(first) == "" {
			first = strings.TrimSpace(parts[1])
			parts = parts[2:]
		} else {
			parts = parts[1:]
		}
		_ = database.DB.Model(&models.ChatHistory{}).Where("id = ?", row.ID).
			Update("reply", first).Error
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			_ = database.DB.Create(&models.ChatHistory{
				AgentID: row.AgentID, Sender: row.Sender, Reply: p,
				FromHuman: true, MediaType: "", ReplySource: row.ReplySource,
				CreatedAt: row.CreatedAt.Add(1 * time.Second),
			}).Error
		}
	}
	return imported, skipped, nil
}

// OnWAMessageRevoke — pesan dihapus dari perangkat lain → tandai Revoked di DB.
func OnWAMessageRevoke(agentID uint, waMsgID string, ts time.Time) {
	if waMsgID == "" {
		return
	}
	res := database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND wa_msg_id = ?", agentID, waMsgID).
		Updates(map[string]any{"revoked": true, "message": "Pesan ini dihapus"})
	if res.RowsAffected == 0 {
		return
	}
	log.Printf("WA agent %d: pesan %s ditandai dihapus", agentID, waMsgID)
}

// GetHistorySyncStatus — kondisi sinkronisasi riwayat terakhir per agent.
func GetHistorySyncStatus(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	st := services.HistorySyncStatusFor(id)
	c.JSON(200, gin.H{
		"agent_id":    st.AgentID,
		"started_at":  st.Started,
		"finished_at": st.Finished,
		"imported":    st.Imported,
		"skipped":     st.Skipped,
		"processed":   st.Processed,
		"batches":     st.BatchCount,
		"in_progress": st.InProgress,
	})
}
