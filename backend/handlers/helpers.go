package handlers

import (
	"log"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// logIfErr mencatat error ke log jika tidak nil. Dipakai untuk operasi best-effort
// yang tidak bisa gagal secara fatal (cleanup, update status, dll).
func logIfErr(err error, msg string) {
	if err != nil {
		log.Printf("WARN: %s: %v", msg, err)
	}
}

// ignoreErr sengaja membuang error — hanya untuk operasi yang benar-benar
// tidak perlu diketahui hasilnya (contoh: delete resource yang sudah tak ada).
// Hindari pemakaian di jalur kritis tempat error bisa menyebabkan korupsi data.
//
// Deprecated: gunakan logIfErr agar error setidaknya tercatat di log.
func ignoreErr(_ error) {}

// ─────────────────────────────────────────────────────────────────────────────
// InboxReadState Helpers (v4 — ported dari chatloop-1.6-1.7)
// ─────────────────────────────────────────────────────────────────────────────

// ensureInboxReadState memastikan record InboxReadState untuk pasangan
// (agentID, sender) sudah ada di DB. Operasi ini idempoten dan aman dipanggil
// berkali-kali dari goroutine berbeda berkat ON CONFLICT DO NOTHING.
func ensureInboxReadState(agentID uint, sender string) error {
	return database.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.InboxReadState{
		AgentID: agentID,
		Sender:  sender,
	}).Error
}

// touchInboxLastMsg memajukan last_msg_at percakapan agar daftar chat Inbox
// selaras urutan WhatsApp Web. Hanya maju (tidak mundur) — untuk event pesan baru.
func touchInboxLastMsg(agentID uint, sender string, ts time.Time) {
	sender = strings.TrimSpace(sender)
	if agentID == 0 || sender == "" {
		return
	}
	// Penyatuan identitas (pola v4): bila pengirim adalah LID & pemetaan ke nomor
	// asli sudah ada, seluruh data kontak ini dirapikan ke SATU identitas.
	// Tanpa ini, kontak yang sama muncul dua kali di sidebar (PN + LID) dengan
	// riwayat terpecah & pratinjau salah (fenomena nyata dari WA pribadi).
	if services.LooksLikeLID(sender) {
		if pn := resolveSenderAliasPN(agentID, sender); pn != "" {
			healSenderIdentity(agentID, sender, pn)
			sender = pn
		}
	}
	if ts.IsZero() {
		ts = time.Now()
	} else {
		ts = ts.UTC()
		// Stempel waktu mustahil (masa depan jauh / sebelum 2000) = data korup;
		// jangan sampai menggeser urutan sidebar.
		if ts.After(time.Now().Add(10*time.Minute)) || ts.Year() < 2000 {
			ts = time.Now()
		}
	}
	ts = ts.Truncate(time.Millisecond)
	if err := ensureInboxReadState(agentID, sender); err != nil {
		return
	}
	// Satu UPDATE atomik mencegah event lama menimpa tip baru ketika dua callback
	// berjalan paralel. Publish tetap dilakukan walau nilainya tidak berubah:
	// touch kedua biasanya terjadi setelah ChatHistory selesai di-commit.
	if err := database.DB.Model(&models.InboxReadState{}).
		Where("agent_id = ? AND sender = ?", agentID, sender).
		Updates(map[string]interface{}{
			"last_msg_at": gorm.Expr(
				"CASE WHEN last_msg_at IS NULL OR last_msg_at < ? THEN ? ELSE last_msg_at END",
				ts, ts,
			),
			"updated_at": time.Now(),
		}).Error; err == nil {
		publishInboxEvent(agentID, sender, "message")
	}
}

// setInboxLastMsgFromWA menulis last_msg_at dari LastMsgTimestamp resmi WhatsApp.
// Snapshot history dapat datang terlambat, jadi tip tetap monotonik dan tidak
// boleh menggeser chat baru ke bawah daftar.
func setInboxLastMsgFromWA(agentID uint, sender string, ts time.Time) {
	sender = strings.TrimSpace(sender)
	if agentID == 0 || sender == "" || ts.IsZero() || ts.Year() < 2000 {
		return
	}
	if err := ensureInboxReadState(agentID, sender); err != nil {
		return
	}
	if err := database.DB.Model(&models.InboxReadState{}).
		Where("agent_id = ? AND sender = ?", agentID, sender).
		Updates(map[string]interface{}{
			"last_msg_at": gorm.Expr(
				"CASE WHEN last_msg_at IS NULL OR last_msg_at < ? THEN ? ELSE last_msg_at END",
				ts, ts,
			),
			"updated_at": time.Now(),
		}).Error; err == nil {
		publishInboxEvent(agentID, sender, "state")
	}
}

// advanceInboxWAState menjalankan perubahan state WhatsApp secara atomik.
// Event WhatsApp dapat selesai di goroutine berbeda; event yang lebih tua tidak
// boleh menghapus unread atau menimpa snapshot yang lebih baru. Timestamp sama
// tetap diterima karena beberapa pesan WA memang berbagi detik yang sama.
func advanceInboxWAState(agentID uint, sender string, eventAt time.Time, updates map[string]interface{}) (bool, error) {
	sender = strings.TrimSpace(sender)
	if agentID == 0 || sender == "" {
		return false, nil
	}
	eventAt = inboxWAEventTime(eventAt)

	if err := ensureInboxReadState(agentID, sender); err != nil {
		return false, err
	}
	if updates == nil {
		updates = make(map[string]interface{})
	}
	updates["whats_app_synced"] = true
	updates["whats_app_state_at"] = eventAt
	updates["updated_at"] = time.Now()

	result := database.DB.Model(&models.InboxReadState{}).
		Where("agent_id = ? AND sender = ?", agentID, sender).
		Where("whats_app_state_at IS NULL OR whats_app_state_at <= ?", eventAt).
		Updates(updates)
	return result.RowsAffected > 0, result.Error
}

func inboxWAEventTime(eventAt time.Time) time.Time {
	if eventAt.IsZero() || eventAt.Year() < 2020 {
		eventAt = time.Now()
	}
	return eventAt.UTC().Truncate(time.Millisecond)
}

// recordIncomingWAUnread menghitung setiap pesan live unik yang berada setelah
// batas baca. Tidak memakai WHERE state_at: callback pesan kedua dapat selesai
// lebih dulu, tetapi pesan pertama tetap harus ikut dihitung. Sebaliknya, receipt
// baca yang lebih baru memajukan last_read_at sehingga event lama tidak menyalakan
// badge kembali.
func recordIncomingWAUnread(agentID uint, sender string, eventAt time.Time) error {
	sender = strings.TrimSpace(sender)
	if agentID == 0 || sender == "" {
		return nil
	}
	eventAt = inboxWAEventTime(eventAt)
	if err := ensureInboxReadState(agentID, sender); err != nil {
		return err
	}
	return database.DB.Model(&models.InboxReadState{}).
		Where("agent_id = ? AND sender = ?", agentID, sender).
		Updates(map[string]interface{}{
			"whats_app_unread_count": gorm.Expr(
				"whats_app_unread_count + CASE WHEN last_read_at IS NULL OR last_read_at < ? THEN 1 ELSE 0 END",
				eventAt,
			),
			"whats_app_synced": true,
			"whats_app_state_at": gorm.Expr(
				"CASE WHEN whats_app_state_at IS NULL OR whats_app_state_at < ? THEN ? ELSE whats_app_state_at END",
				eventAt, eventAt,
			),
			"updated_at": time.Now(),
		}).Error
}
