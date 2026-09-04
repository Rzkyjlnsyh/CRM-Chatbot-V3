package handlers

import (
	"log"
	"sort"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

const (
	unreadBootstrapLimit     = 12
	unreadBootstrapReadyWait = 30 * time.Second
)

// reconcileUnreadAfterConnect melengkapi status percakapan yang tidak
// tercakup HistorySync awal. Menunggu HistorySync selesai, kemudian
// menandai InboxReadState yang belum ter-sync agar terisi saat pesan
// masuk berikutnya. Berjalan async setelah agent connected.
func reconcileUnreadAfterConnect(agentID uint) {
	time.Sleep(5 * time.Second)
	deadline := time.Now().Add(unreadBootstrapReadyWait)
	for time.Now().Before(deadline) {
		if !services.WA(agentID).IsConnected() {
			return
		}
		st := services.HistorySyncStatusFor(agentID)
		if !st.InProgress && (st.Processed > 0 || !st.Started.IsZero()) {
			break // HistorySync sudah selesai
		}
		var historyCount int64
		database.DB.Model(&models.ChatHistory{}).Where("agent_id = ?", agentID).Count(&historyCount)
		if historyCount > 0 && !st.InProgress {
			break
		}
		time.Sleep(4 * time.Second)
	}
	time.Sleep(2 * time.Second)

	// Pastikan semua sender yang ada di chat_histories punya InboxReadState.
	// Ini menjamin backfill untuk instalasi yang diupgrade dari v1.2.0 lama.
	type senderRow struct {
		Sender  string
		LastAt  time.Time
		LastMsg string
	}
	var candidates []senderRow
	if err := database.DB.Raw(`
		SELECT ch.sender,
		       MAX(ch.created_at) AS last_at,
		       MAX(ch.wa_msg_id)  AS last_msg
		FROM chat_histories ch
		LEFT JOIN inbox_read_states rs
		       ON rs.agent_id = ch.agent_id AND rs.sender = ch.sender
		WHERE ch.agent_id = ? AND ch.sender <> ''
		      AND ch.sender NOT LIKE '%@g.us'
		      AND (rs.id IS NULL OR COALESCE(rs.whats_app_synced, 0) = 0)
		GROUP BY ch.sender
		ORDER BY last_at DESC
		LIMIT ?
	`, agentID, unreadBootstrapLimit).Scan(&candidates).Error; err != nil {
		log.Printf("WA agent %d: gagal menyiapkan bootstrap status unread: %v", agentID, err)
		return
	}
	if len(candidates) == 0 {
		log.Printf("WA agent %d: status unread awal sudah lengkap", agentID)
		return
	}

	synced := 0
	for _, c := range candidates {
		if !services.WA(agentID).IsConnected() {
			break
		}
		// ensureInboxReadState membuat baris InboxReadState bila belum ada,
		// lalu touchInboxLastMsg memajukan last_msg_at agar urutan inbox akurat.
		if err := ensureInboxReadState(agentID, c.Sender); err != nil {
			continue
		}
		touchInboxLastMsg(agentID, c.Sender, c.LastAt)
		synced++
	}
	log.Printf("WA agent %d: bootstrap status unread selesai (%d/%d percakapan)", agentID, synced, len(candidates))
}

// OnWAHistorySync = target handler HistorySync dari services WA (dipasang di startup).
// Import riwayat ke chat_histories dengan aturan:
//   - dedup pre-SELECT per WAMsgID — 1 batch query, bukan N query
//   - batch insert dengan ON CONFLICT DO NOTHING untuk performa
//   - ReplySource='history_sync' — Learning MENGABAIKAN baris ini
//   - LiveIncoming=false — cursor notifikasi tidak terpicu pesan lama
//   - media: MediaMetadata (protobuf) + MediaFetchStatus='pending' (unduh on-demand)
//   - setInboxLastMsgFromWA per sender untuk urutan daftar chat yang akurat
func OnWAHistorySync(agentID uint, messages []services.HistoricalMessage) (imported int, skipped int, err error) {
	if len(messages) == 0 {
		return 0, 0, nil
	}

	// Buat lookup existing WAMsgID untuk batch ini sekaligus (1 query, bukan N).
	var msgIDs []string
	for _, m := range messages {
		if m.WAMsgID != "" {
			msgIDs = append(msgIDs, m.WAMsgID)
		}
	}
	existing := make(map[string]models.ChatHistory)
	if len(msgIDs) > 0 {
		var existRows []models.ChatHistory
		database.DB.Where("agent_id = ? AND wa_msg_id IN ?", agentID, msgIDs).
			Select("id", "wa_msg_id").Find(&existRows)
		for _, r := range existRows {
			existing[r.WAMsgID] = r
		}
	}

	// Gunakan setInboxLastMsgFromWA untuk update last_msg_at per sender.
	markChanged := func(sender string, ts time.Time) {
		if !services.IsGroupJID(sender) {
			setInboxLastMsgFromWA(agentID, sender, ts)
		}
	}

	var rows []models.ChatHistory
	contactsByNumber := make(map[string]models.Contact)

	for _, msg := range messages {
		if strings.TrimSpace(msg.Sender) == "" {
			skipped++
			continue
		}
		if msg.WAMsgID != "" {
			if _, dup := existing[msg.WAMsgID]; dup {
				markChanged(msg.Sender, msg.Timestamp)
				skipped++
				continue
			}
		}
		// Lindungi dari ID ganda di batch HistorySync yang sama.
		existing[msg.WAMsgID] = models.ChatHistory{WAMsgID: msg.WAMsgID}
		// Tanpa timestamp resmi, timeline akan tampak sinkron padahal tanggalnya rekaan.
		if msg.Timestamp.IsZero() || msg.Timestamp.Year() < 2020 {
			skipped++
			continue
		}
		row := models.ChatHistory{
			AgentID: agentID, Sender: msg.Sender, FromHuman: msg.FromMe,
			MediaType: msg.MediaType, FileName: msg.FileName, Mimetype: msg.Mimetype,
			WAMsgID: msg.WAMsgID, ReplyTo: msg.ReplyTo, ReplyText: msg.ReplyText,
			DeliveryStatus: "sent", ReplySource: "history_sync",
			LiveIncoming: false, // pesan history TIDAK memicu cursor notifikasi
			CreatedAt:    msg.Timestamp,
		}
		if len(msg.MediaMetadata) > 0 {
			row.MediaMetadata = msg.MediaMetadata
			row.MediaFetchStatus = "pending"
		}
		if msg.FromMe {
			row.Reply = msg.Text
		} else {
			row.Message = msg.Text
		}
		rows = append(rows, row)
		if !services.IsGroupJID(msg.Sender) {
			contact := models.Contact{
				AgentID: agentID, Number: msg.Sender, Name: strings.TrimSpace(msg.PushName),
				LeadStage: "new", LeadStageSource: "system",
				LeadStageReason: "Kontak dari sinkronisasi riwayat WhatsApp",
			}
			if old, ok := contactsByNumber[msg.Sender]; !ok || (old.Name == "" && contact.Name != "") {
				contactsByNumber[msg.Sender] = contact
			}
		}
	}
	if len(rows) == 0 {
		return 0, skipped, nil
	}

	// Urutkan berdasarkan waktu agar ID lokal masuk akal.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt.Before(rows[j].CreatedAt) })

	contacts := make([]models.Contact, 0, len(contactsByNumber))
	for _, c := range contactsByNumber {
		contacts = append(contacts, c)
	}
	tx := database.DB.Begin()
	if tx.Error != nil {
		return 0, skipped, tx.Error
	}
	if len(contacts) > 0 {
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&contacts, 250).Error; e != nil {
			tx.Rollback()
			return 0, skipped, e
		}
	}
	createResult := tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(&rows, 250)
	if createResult.Error != nil {
		tx.Rollback()
		return 0, skipped, createResult.Error
	}
	if e := tx.Commit().Error; e != nil {
		return 0, skipped, e
	}
	imported = int(createResult.RowsAffected)
	if imported < len(rows) {
		skipped += len(rows) - imported
	}

	// Majukan last_msg_at per sender dari timestamp WA.
	latestBySender := make(map[string]time.Time, len(contactsByNumber))
	for _, row := range rows {
		if t, ok := latestBySender[row.Sender]; !ok || row.CreatedAt.After(t) {
			latestBySender[row.Sender] = row.CreatedAt
		}
	}
	for sender, ts := range latestBySender {
		markChanged(sender, ts)
	}
	return imported, skipped, nil
}

// OnWAHistoryChatState dipanggil engine WA saat menerima snapshot status chat
// dari HistorySync (jumlah unread, waktu pesan terakhir). Semua update dilakukan
// atomik melalui advanceInboxWAState agar event paralel tidak saling menimpa.
func OnWAHistoryChatState(agentID uint, states []services.HistoryChatState) {
	for _, state := range states {
		state.Sender = services.NormalizeInboxSender(state.Sender)
		if state.Sender == "" {
			continue
		}
		if !state.Timestamp.IsZero() && state.Timestamp.Year() < 2020 {
			state.Timestamp = time.Time{}
		}
		count := state.UnreadCount
		if state.MarkedUnread && count == 0 {
			count = 1
		}
		updates := map[string]interface{}{
			"whats_app_unread_count": count,
		}
		if !state.Timestamp.IsZero() {
			// Cari boundary last_read_at berdasarkan unread count WA.
			var boundary models.ChatHistory
			q := database.DB.Where("agent_id = ? AND sender = ? AND TRIM(COALESCE(message, '')) <> ''", agentID, state.Sender).
				Order("created_at DESC, id DESC")
			if count > 0 {
				q = q.Offset(count)
			}
			if q.First(&boundary).Error == nil {
				updates["last_read_at"] = boundary.CreatedAt
			} else if count == 0 {
				updates["last_read_at"] = state.Timestamp
			}
			updates["last_msg_at"] = state.Timestamp
		}
		changed, err := advanceInboxWAState(agentID, state.Sender, state.Timestamp, updates)
		if err != nil {
			log.Printf("WA agent %d: gagal update InboxReadState (%s): %v", agentID, state.Sender, err)
		}
		if changed {
			publishInboxEvent(agentID, state.Sender, "state")
		}
	}
}

// OnWAWhatsAppReadState dipanggil engine WA saat HP mengirim sinyal "sudah dibaca".
func OnWAWhatsAppReadState(agentID uint, sender string, read bool, timestamp time.Time) {
	sender = services.NormalizeInboxSender(sender)
	if sender == "" || agentID == 0 {
		return
	}
	updates := map[string]interface{}{}
	if read {
		updates["whats_app_unread_count"] = 0
		if !timestamp.IsZero() {
			updates["last_read_at"] = timestamp
		}
	}
	changed, err := advanceInboxWAState(agentID, sender, timestamp, updates)
	if err != nil {
		log.Printf("WA agent %d: gagal update read state (%s): %v", agentID, sender, err)
		return
	}
	if changed {
		publishInboxEvent(agentID, sender, "state")
	}
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
