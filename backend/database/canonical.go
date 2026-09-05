package database

import (
	"fmt"
	"log"
	"strings"

	"wa-assistant/backend/models"
)

// canonical.go — identitas pesan kanonik (pola v4 chatloop-1.6-1.7, diadaptasi
// untuk SQLite/glebarez): dedup duplikat wa_msg_id + pertahankan data terbaik
// dari baris duplikat. Tujuan POV: pesan di Inbox TIDAK mungkin dobel walau
// history sync dan pesan live tiba bersamaan.

// normalizePhoneLocal menormalkan nomor telepon: hanya digit, 08→628, 8→628.
func normalizePhoneLocal(s string) string {
	digits := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
	}
	clean := string(digits)
	if strings.HasPrefix(clean, "08") {
		clean = "628" + clean[2:]
	} else if strings.HasPrefix(clean, "8") {
		clean = "628" + clean[1:]
	}
	return clean
}

// normalizedSenderFieldValue hanya mengubah JID personal legacy menjadi nomor.
// JID grup adalah identitas thread kanonik dan tidak boleh kehilangan @g.us.
func normalizedSenderFieldValue(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasSuffix(strings.ToLower(value), "@g.us") {
		return "", false
	}
	clean := strings.SplitN(value, "@", 2)[0]
	if clean == value || clean == "" {
		return "", false
	}
	normalized := normalizePhoneLocal(clean)
	return normalized, normalized != "" && normalized != value
}

// normalizeSenderFields memperbaiki sender personal yang mungkin tersimpan
// dengan format JID (@s.whatsapp.net) atau nomor yang belum ternormalisasi.
// JID grup sengaja dipertahankan utuh.
func normalizeSenderFields() {
	type senderPair struct{ Old, New string }
	var pairs []senderPair

	seen := make(map[string]struct{})
	for _, table := range []string{
		"chat_histories", "inbox_read_states", "handoffs",
		"conversation_memories", "ai_turns", "follow_ups",
	} {
		var values []string
		DB.Table(table).Distinct("sender").Where("sender LIKE ?", "%@%").Pluck("sender", &values)
		for _, v := range values {
			if _, ok := seen[v]; ok {
				continue
			}
			seen[v] = struct{}{}
			if normalized, ok := normalizedSenderFieldValue(v); ok {
				pairs = append(pairs, senderPair{Old: v, New: normalized})
			}
		}
	}
	for _, pair := range pairs {
		affected := DB.Table("chat_histories").
			Where("sender = ?", pair.Old).Update("sender", pair.New)
		if affected.Error != nil {
			log.Printf("[canonical] gagal normalisasi sender chat_histories %q: %v", pair.Old, affected.Error)
			continue
		}
		for _, table := range []string{"inbox_read_states", "handoffs", "conversation_memories", "ai_turns", "follow_ups"} {
			_ = DB.Table(table).Where("sender = ?", pair.Old).Update("sender", pair.New).Error
		}
	}
}

// deliveryRank = tingkat kekuatan status kirim (semakin besar semakin kuat).
// read tidak boleh diturunkan oleh delivered/sent.
func deliveryRank(status string) int {
	switch strings.TrimSpace(status) {
	case "played":
		return 6
	case "read":
		return 5
	case "read_inferred":
		return 4
	case "delivered":
		return 3
	case "sent":
		return 2
	case "pending_retry":
		return 1
	default:
		return 0
	}
}

// mergedCanonicalChatFields menggabungkan data terbaik dari baris duplikat
// ke baris "keeper": isi kosong diisi, status delivery tertinggi dipertahankan,
// timestamp kanonik = paling awal, revoke tidak pernah hilang.
func mergedCanonicalChatFields(keeper models.ChatHistory, duplicates []models.ChatHistory) map[string]interface{} {
	updates := make(map[string]interface{})
	bestCreatedAt := keeper.CreatedAt
	for _, row := range duplicates {
		if strings.TrimSpace(keeper.Message) == "" && strings.TrimSpace(keeper.Reply) == "" {
			if strings.TrimSpace(row.Message) != "" || strings.TrimSpace(row.Reply) != "" {
				keeper.Message, keeper.Reply, keeper.FromHuman = row.Message, row.Reply, row.FromHuman
				updates["message"], updates["reply"], updates["from_human"] = row.Message, row.Reply, row.FromHuman
			}
		}
		if keeper.MediaType == "" && row.MediaType != "" {
			keeper.MediaType = row.MediaType
			updates["media_type"] = row.MediaType
		}
		if keeper.MediaPath == "" && row.MediaPath != "" {
			keeper.MediaPath = row.MediaPath
			updates["media_path"] = row.MediaPath
		}
		if len(keeper.MediaMetadata) == 0 && len(row.MediaMetadata) > 0 {
			keeper.MediaMetadata = row.MediaMetadata
			updates["media_metadata"] = row.MediaMetadata
		}
		if keeper.MediaFetchStatus == "" && row.MediaFetchStatus != "" {
			keeper.MediaFetchStatus = row.MediaFetchStatus
			updates["media_fetch_status"] = row.MediaFetchStatus
		}
		if keeper.FileName == "" && row.FileName != "" {
			keeper.FileName = row.FileName
			updates["file_name"] = row.FileName
		}
		if keeper.Mimetype == "" && row.Mimetype != "" {
			keeper.Mimetype = row.Mimetype
			updates["mimetype"] = row.Mimetype
		}
		if keeper.ReplyTo == "" && row.ReplyTo != "" {
			keeper.ReplyTo = row.ReplyTo
			updates["reply_to"] = row.ReplyTo
		}
		if keeper.ReplyText == "" && row.ReplyText != "" {
			keeper.ReplyText = row.ReplyText
			updates["reply_text"] = row.ReplyText
		}
		if row.Revoked && !keeper.Revoked {
			keeper.Revoked = true
			updates["revoked"] = true
		}
		if deliveryRank(row.DeliveryStatus) > deliveryRank(keeper.DeliveryStatus) {
			keeper.DeliveryStatus = row.DeliveryStatus
			updates["delivery_status"] = row.DeliveryStatus
		}
		if !row.CreatedAt.IsZero() && (bestCreatedAt.IsZero() || row.CreatedAt.Before(bestCreatedAt)) {
			bestCreatedAt = row.CreatedAt
		}
	}
	if !bestCreatedAt.IsZero() && !bestCreatedAt.Equal(keeper.CreatedAt) {
		updates["created_at"] = bestCreatedAt
	}
	return updates
}

// EnsureCanonicalChatMessageIDs mendedup pesan dobel berdasarkan wa_msg_id
// (SQLite-safe; idempoten; aman dipanggil setiap startup). Setelah bersih,
// dipasang partial unique index agar duplikat TIDAK bisa muncul lagi.
func EnsureCanonicalChatMessageIDs() error {
	normalizeSenderFields()

	// 1) Cari grup wa_msg_id duplikat (non-kosong).
	var dupKeys []struct {
		AgentID uint   `gorm:"column:agent_id"`
		WAMsgID string `gorm:"column:wa_msg_id"`
		Cnt     int64  `gorm:"column:cnt"`
	}
	if err := DB.Raw(`
		SELECT agent_id, wa_msg_id, COUNT(*) AS cnt
		FROM chat_histories
		WHERE wa_msg_id IS NOT NULL AND TRIM(wa_msg_id) != ''
		GROUP BY agent_id, wa_msg_id
		HAVING cnt > 1
		LIMIT 2000
	`).Scan(&dupKeys).Error; err != nil {
		return fmt.Errorf("gagal memindai wa_msg_id duplikat: %w", err)
	}

	totalMerged := int64(0)
	totalDeleted := int64(0)
	for _, key := range dupKeys {
		var rows []models.ChatHistory
		if err := DB.Where("agent_id = ? AND wa_msg_id = ?", key.AgentID, key.WAMsgID).
			Order("id ASC").Find(&rows).Error; err != nil || len(rows) < 2 {
			continue
		}
		keeper := rows[0]
		updates := mergedCanonicalChatFields(keeper, rows[1:])
		if len(updates) > 0 {
			if res := DB.Model(&models.ChatHistory{}).Where("id = ?", keeper.ID).Updates(updates); res.Error == nil {
				totalMerged += res.RowsAffected
			}
		}
		ids := make([]uint, 0, len(rows)-1)
		for _, row := range rows[1:] {
			ids = append(ids, row.ID)
		}
		if res := DB.Where("id IN ?", ids).Delete(&models.ChatHistory{}); res.Error == nil {
			totalDeleted += res.RowsAffected
		}
	}
	if totalDeleted > 0 || totalMerged > 0 {
		log.Printf("[canonical] dedup pesan: %d baris digabung, %d duplikat dihapus", totalMerged, totalDeleted)
	}

	// 2) Partial unique index: cegah duplikat wa_msg_id sejak sekarang.
	if err := DB.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_chat_wa_msg_unique
		ON chat_histories (agent_id, wa_msg_id)
		WHERE wa_msg_id IS NOT NULL AND TRIM(wa_msg_id) != ''
	`).Error; err != nil {
		return fmt.Errorf("gagal memasang unique index pesan: %w", err)
	}
	return nil
}
