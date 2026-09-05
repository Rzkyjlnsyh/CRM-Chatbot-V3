package handlers

import (
	"log"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

// migrateLIDSenders merapikan data lama: pengirim yang tersimpan sebagai LID diubah
// jadi nomor telepon asli (pakai pemetaan LID->PN milik whatsmeow). Idempoten —
// setelah semua terkonversi, panggilan berikutnya tidak menemukan kandidat lagi.
// Dipanggil saat agent tersambung (store & pemetaan LID sudah siap).
// healSenderIdentity menyatukan identitas ganda (LID → nomor asli) di semua
// tabel data kontak. Tidak menghapus pesan — hanya mengganti kunci identitas.
// Dipanggil secara kontinu setiap kali aktivitas dari LID tersentuh, sehingga
// pecahan identitas apa pun otomatis tersatukan (tanpa menunggu reconnect).
func healSenderIdentity(agentID uint, lid, pn string) {
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ?", agentID, lid).
		Update("sender", pn)
	// Tabel status satu-baris-per-kontak: gabungkan (bila baris PN sudah ada,
	// baris LID dihapus — mergeLIDState melakukan itu tanpa bentrok unik).
	mergeLIDState(&models.InboxReadState{}, "sender", agentID, lid, pn)
	mergeLIDState(&models.ConversationRead{}, "sender", agentID, lid, pn)
	mergeLIDState(&models.Handoff{}, "sender", agentID, lid, pn)
	mergeLIDState(&models.OptOut{}, "sender", agentID, lid, pn)
	mergeLIDState(&models.Contact{}, "number", agentID, lid, pn)
}

func migrateLIDSenders(agentID uint) {
	wa := services.WA(agentID)

	candidates := map[string]bool{}
	addDistinct := func(model interface{}, col string) {
		var vals []string
		database.DB.Model(model).Where("agent_id = ? AND "+col+" <> ''", agentID).Distinct().Pluck(col, &vals)
		for _, v := range vals {
			candidates[v] = true
		}
	}
	addDistinct(&models.ChatHistory{}, "sender")
	addDistinct(&models.Handoff{}, "sender")
	addDistinct(&models.OptOut{}, "sender")
	addDistinct(&models.Contact{}, "number")
	addDistinct(&models.InboxReadState{}, "sender")
	addDistinct(&models.ConversationRead{}, "sender")

	mapping := map[string]string{}
	for v := range candidates {
		if pn := wa.PNForLID(v); pn != "" && pn != v {
			mapping[v] = pn
		}
	}
	// Junk LID yang TIDAK bisa dipetakan: hapus dari inbox_read_states &
	// conversation_reads agar daftar chat tidak menampilkan "nomor" aneh.
	// (Tabel riwayat chat dibiarkan — tidak ada baris chat di bawahnya
	// biasanya; data tidak dihancurkan membabi buta.)
	for v := range candidates {
		if !services.LooksLikeLID(v) {
			continue
		}
		if _, ok := mapping[v]; ok {
			continue
		}
		database.DB.Where("agent_id = ? AND sender = ?", agentID, v).Delete(&models.InboxReadState{})
		database.DB.Where("agent_id = ? AND sender = ?", agentID, v).Delete(&models.ConversationRead{})
	}
	if len(mapping) == 0 {
		return
	}

	for lid, pn := range mapping {
		// Riwayat chat: SELALU ubah, jangan hapus — itu pesan asli (tak ada batasan unik).
		database.DB.Model(&models.ChatHistory{}).Where("agent_id = ? AND sender = ?", agentID, lid).Update("sender", pn)
		// Tabel status: gabungkan bila baris nomor telepon sudah ada (hindari bentrok unik).
		mergeLIDState(&models.Handoff{}, "sender", agentID, lid, pn)
		mergeLIDState(&models.OptOut{}, "sender", agentID, lid, pn)
		mergeLIDState(&models.Contact{}, "number", agentID, lid, pn)
		mergeLIDState(&models.InboxReadState{}, "sender", agentID, lid, pn)
		mergeLIDState(&models.ConversationRead{}, "sender", agentID, lid, pn)
	}
	log.Printf("Rapikan LID (agent %d): %d pengirim LID diubah ke nomor telepon", agentID, len(mapping))
}

// mergeLIDState mengubah nilai LID jadi nomor telepon pada tabel berstatus tunggal;
// kalau baris untuk nomor telepon itu sudah ada, baris LID dihapus (digabung).
func mergeLIDState(model interface{}, col string, agentID uint, lid, pn string) {
	var existing int64
	database.DB.Model(model).Where("agent_id = ? AND "+col+" = ?", agentID, pn).Limit(1).Count(&existing)
	if existing > 0 {
		database.DB.Where("agent_id = ? AND "+col+" = ?", agentID, lid).Delete(model)
		return
	}
	database.DB.Model(model).Where("agent_id = ? AND "+col+" = ?", agentID, lid).Update(col, pn)
}
