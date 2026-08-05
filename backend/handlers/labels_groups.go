package handlers

import (
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// OnLabelEdit dipanggil saat label dibuat/diubah/dihapus di WhatsApp (event app-state).
func OnLabelEdit(agentID uint, labelID, name string, color int, deleted bool) {
	if deleted {
		database.DB.Where("agent_id = ? AND label_id = ?", agentID, labelID).Delete(&models.Label{})
		database.DB.Where("agent_id = ? AND label_id = ?", agentID, labelID).Delete(&models.ChatLabel{})
		return
	}
	var l models.Label
	database.DB.Where(models.Label{AgentID: agentID, LabelID: labelID}).FirstOrCreate(&l)
	l.Name = name
	l.Color = color
	database.DB.Save(&l)
}

// OnLabelAssoc dipanggil saat chat diberi / dilepas label.
func OnLabelAssoc(agentID uint, sender, labelID string, labeled bool) {
	if sender == "" {
		return
	}
	if labeled {
		var cl models.ChatLabel
		database.DB.Where(models.ChatLabel{AgentID: agentID, LabelID: labelID, Sender: sender}).FirstOrCreate(&cl)
	} else {
		database.DB.Where("agent_id = ? AND label_id = ? AND sender = ?", agentID, labelID, sender).Delete(&models.ChatLabel{})
	}
}

// Labels = daftar label + jumlah kontak.
func Labels(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	c.JSON(200, gin.H{"data": labelResponseData(id)})
}

func labelResponseData(agentID uint) []gin.H {
	var labels []models.Label
	database.DB.Where("agent_id = ?", agentID).Order("name asc").Find(&labels)
	out := make([]gin.H, 0, len(labels))
	for _, l := range labels {
		var cnt int64
		database.DB.Model(&models.ChatLabel{}).Where("agent_id = ? AND label_id = ?", agentID, l.LabelID).Count(&cnt)
		out = append(out, gin.H{"label_id": l.LabelID, "name": l.Name, "color": l.Color, "count": cnt})
	}
	return out
}

// SyncLabels mengambil ulang snapshot label WhatsApp dan baru mengganti tabel
// lokal setelah seluruh snapshot berhasil diterima.
func SyncLabels(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	wa := services.WA(id)
	if !wa.IsConnected() {
		c.JSON(400, gin.H{"error": "WhatsApp belum tersambung"})
		return
	}
	labels, contacts, err := wa.SyncLabels(c.Request.Context())
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}

	tx := database.DB.Begin()
	if tx.Error != nil {
		c.JSON(500, gin.H{"error": "Gagal memulai penyimpanan label"})
		return
	}
	fail := func() {
		tx.Rollback()
		c.JSON(500, gin.H{"error": "Gagal menyimpan hasil sinkronisasi label"})
	}
	if err := tx.Where("agent_id = ?", id).Delete(&models.ChatLabel{}).Error; err != nil {
		fail()
		return
	}
	if err := tx.Where("agent_id = ?", id).Delete(&models.Label{}).Error; err != nil {
		fail()
		return
	}
	labelRows := make([]models.Label, 0, len(labels))
	for _, label := range labels {
		labelRows = append(labelRows, models.Label{AgentID: id, LabelID: label.LabelID, Name: label.Name, Color: label.Color})
	}
	if len(labelRows) > 0 {
		if err := tx.CreateInBatches(labelRows, 100).Error; err != nil {
			fail()
			return
		}
	}
	contactRows := make([]models.ChatLabel, 0, len(contacts))
	for _, contact := range contacts {
		contactRows = append(contactRows, models.ChatLabel{AgentID: id, LabelID: contact.LabelID, Sender: contact.Number})
	}
	if len(contactRows) > 0 {
		if err := tx.CreateInBatches(contactRows, 200).Error; err != nil {
			fail()
			return
		}
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyelesaikan penyimpanan label"})
		return
	}
	c.JSON(200, gin.H{
		"data":      labelResponseData(id),
		"synced_at": time.Now(),
		"labels":    len(labelRows),
		"contacts":  len(contactRows),
	})
}

// LabelContacts = nomor kontak dengan label tertentu (kecuali opt-out).
func LabelContacts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var senders []string
	database.DB.Model(&models.ChatLabel{}).Where("agent_id = ? AND label_id = ?", id, c.Query("label_id")).Pluck("sender", &senders)
	out := optedOutSet(id)
	names := contactNames(id)
	res := make([]gin.H, 0, len(senders))
	for _, s := range senders {
		if s != "" && !out[s] {
			res = append(res, gin.H{"number": s, "name": names[s]})
		}
	}
	c.JSON(200, gin.H{"data": res})
}

// Groups = daftar grup yang diikuti nomor.
func Groups(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	if !services.WA(id).IsConnected() {
		c.JSON(400, gin.H{"error": "WhatsApp belum tersambung"})
		return
	}
	groups, err := services.WA(id).GetGroups()
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	// Status penjaga (aktif/tidak) per grup, supaya kelihatan di daftar.
	var enabledJIDs []string
	database.DB.Model(&models.GroupGuardConfig{}).
		Where("agent_id = ? AND enabled = ?", id, true).Pluck("group_jid", &enabledJIDs)
	enabledSet := make(map[string]bool, len(enabledJIDs))
	for _, j := range enabledJIDs {
		enabledSet[j] = true
	}
	data := make([]gin.H, 0, len(groups))
	for _, g := range groups {
		data = append(data, gin.H{
			"jid": g.JID, "name": g.Name, "participants": g.Participants,
			"bot_is_admin": g.BotIsAdmin, "guard_enabled": enabledSet[g.JID],
		})
	}
	c.JSON(200, gin.H{"data": data})
}

// GroupMembers = nomor anggota grup (untuk dijadikan penerima broadcast).
func GroupMembers(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	jid := c.Query("jid")
	if jid == "" {
		c.JSON(400, gin.H{"error": "jid grup wajib"})
		return
	}
	members, err := services.WA(id).GetGroupMembers(jid)
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	out := optedOutSet(id)
	names := contactNames(id)
	res := make([]services.WAContact, 0, len(members))
	for _, m := range members {
		if out[m.Number] {
			continue
		}
		if m.Name == "" {
			m.Name = names[m.Number] // lengkapi dari kontak yang pernah chat
		}
		res = append(res, m)
	}
	c.JSON(200, gin.H{"data": res})
}
