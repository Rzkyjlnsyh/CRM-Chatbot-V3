package handlers

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

const (
	leadStageNew         = "new"
	leadStageCold        = "cold"
	leadStageWarm        = "warm"
	leadStageHot         = "hot"
	leadStageCustomer    = "customer"
	leadStageUnqualified = "unqualified"
)

// normalizeLeadStage memvalidasi key tahap terhadap definisi pipeline agent
// (bukan hardcode) — tahap custom user ikut valid.
func normalizeLeadStage(agentID uint, raw string) (string, bool) {
	clean := strings.ToLower(strings.TrimSpace(raw))
	if clean == "" {
		return leadStageNew, true
	}
	defs := database.GetStageDefMap(agentID)
	if _, ok := defs[clean]; ok {
		return clean, true
	}
	return "", false
}

// emptyStageCounts membangun peta hitungan tahap dari definisi pipeline agent.
func emptyStageCounts(agentID uint) map[string]int {
	defs := database.GetStageDefMap(agentID)
	out := make(map[string]int, len(defs))
	for key := range defs {
		out[key] = 0
	}
	return out
}

// normalizeTags merapikan daftar tag: trim, buang kosong & duplikat, gabung dengan koma.
func normalizeTags(raw string) string {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, t := range strings.Split(raw, ",") {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return strings.Join(out, ",")
}

// tagList memecah string tag tersimpan jadi slice (sudah ter-trim, tanpa kosong).
func tagList(s string) []string {
	out := make([]string, 0)
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ListSavedContacts = buku kontak tersimpan (CRM ringan): cari, filter tag, paginasi,
// plus waktu chat terakhir tiap kontak. ?all=1 mengembalikan semua hasil tanpa paginasi
// (dipakai untuk menjadikan satu tag jadi target broadcast).
func ListSavedContacts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	tag := strings.ToLower(strings.TrimSpace(c.Query("tag")))
	stageFilter := ""
	if rawStage := strings.TrimSpace(c.Query("stage")); rawStage != "" {
		var valid bool
		stageFilter, valid = normalizeLeadStage(id, rawStage)
		if !valid {
			c.JSON(400, gin.H{"error": "Status CRM tidak valid"})
			return
		}
	}
	all := c.Query("all") == "1"
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	const limit = 20

	var contacts []models.Contact
	database.DB.Where("agent_id = ?", id).Order("name asc, number asc").Find(&contacts)

	// Waktu chat terakhir per nomor (satu query, dikelompokkan).
	type lastRow struct {
		Sender string
		Last   time.Time
	}
	var rows []lastRow
	database.DB.Model(&models.ChatHistory{}).
		Select("sender, MAX(created_at) as last").
		Where("agent_id = ?", id).Group("sender").Scan(&rows)
	lastAt := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		lastAt[r.Sender] = r.Last
	}

	tagSet := map[string]string{} // lower -> bentuk tampil
	stageCounts := emptyStageCounts(id)
	filtered := make([]models.Contact, 0, len(contacts))
	for _, ct := range contacts {
		tags := tagList(ct.Tags)
		for _, t := range tags {
			tagSet[strings.ToLower(t)] = t
		}
		if q != "" && !strings.Contains(strings.ToLower(ct.Name), q) && !strings.Contains(ct.Number, q) {
			continue
		}
		if tag != "" {
			has := false
			for _, t := range tags {
				if strings.ToLower(t) == tag {
					has = true
					break
				}
			}
			if !has {
				continue
			}
		}
		contactStage, _ := normalizeLeadStage(id, ct.LeadStage)
		stageCounts[contactStage]++
		if stageFilter != "" && contactStage != stageFilter {
			continue
		}
		filtered = append(filtered, ct)
	}

	total := len(filtered)
	pageItems := filtered
	if !all {
		start := (page - 1) * limit
		if start > total {
			start = total
		}
		end := start + limit
		if end > total {
			end = total
		}
		pageItems = filtered[start:end]
	}

	data := make([]gin.H, 0, len(pageItems))
	for _, ct := range pageItems {
		var la interface{}
		if t, ok := lastAt[ct.Number]; ok && !t.IsZero() {
			la = t
		}
		data = append(data, gin.H{
			"id": ct.ID, "number": ct.Number, "name": ct.Name,
			"notes": ct.Notes, "tags": ct.Tags, "lead_stage": normalizedContactStage(id, ct.LeadStage), "last_at": la,
			"lead_stage_source": ct.LeadStageSource, "lead_stage_reason": ct.LeadStageReason,
			"lead_stage_confidence": ct.LeadStageConfidence, "lead_stage_locked": ct.LeadStageLocked,
			"lead_stage_updated_at": ct.LeadStageUpdatedAt,
		})
	}

	allTags := make([]string, 0, len(tagSet))
	for _, disp := range tagSet {
		allTags = append(allTags, disp)
	}
	sort.Slice(allTags, func(i, j int) bool { return strings.ToLower(allTags[i]) < strings.ToLower(allTags[j]) })

	c.JSON(200, gin.H{"data": data, "total": total, "page": page, "limit": limit, "all_tags": allTags, "stage_counts": stageCounts})
}

func normalizedContactStage(agentID uint, raw string) string {
	stage, ok := normalizeLeadStage(agentID, raw)
	if !ok {
		return leadStageNew
	}
	return stage
}

func CreateSavedContact(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Number    string `json:"number"`
		Name      string `json:"name"`
		Notes     string `json:"notes"`
		Tags      string `json:"tags"`
		LeadStage string `json:"lead_stage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	num := services.NormalizePhone(req.Number)
	if num == "" {
		c.JSON(400, gin.H{"error": "Nomor wajib diisi"})
		return
	}
	var existing models.Contact
	if database.DB.Where("agent_id = ? AND number = ?", id, num).First(&existing).Error == nil {
		c.JSON(409, gin.H{"error": "Nomor ini sudah ada di kontak"})
		return
	}
	stage, validStage := normalizeLeadStage(id, req.LeadStage)
	if !validStage {
		c.JSON(400, gin.H{"error": "Status CRM tidak valid"})
		return
	}
	now := time.Now()
	ct := models.Contact{AgentID: id, Number: num, Name: strings.TrimSpace(req.Name), Notes: req.Notes, Tags: normalizeTags(req.Tags), LeadStage: stage,
		LeadStageSource: "manual", LeadStageReason: "Status dipilih saat kontak dibuat", LeadStageLocked: true, LeadStageUpdatedAt: &now}
	if err := database.DB.Create(&ct).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal"})
		return
	}
	c.JSON(201, gin.H{"data": ct})
}

// UpdateSavedContact mengubah nama/catatan/tag/status CRM. Nomor sengaja tidak bisa diubah
// (jadi kunci unik & terikat riwayat chat) — kalau salah, hapus lalu tambah ulang.
func UpdateSavedContact(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var ct models.Contact
	if database.DB.Where("agent_id = ?", id).First(&ct, c.Param("cid")).Error != nil {
		c.JSON(404, gin.H{"error": "Kontak tidak ditemukan"})
		return
	}
	var req struct {
		Name      *string `json:"name"`
		Notes     *string `json:"notes"`
		Tags      *string `json:"tags"`
		LeadStage *string `json:"lead_stage"`
		StageLock *bool   `json:"lead_stage_locked"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if req.Name != nil {
		ct.Name = strings.TrimSpace(*req.Name)
	}
	if req.Notes != nil {
		ct.Notes = *req.Notes
	}
	if req.Tags != nil {
		ct.Tags = normalizeTags(*req.Tags)
	}
	if req.LeadStage != nil {
		stage, valid := normalizeLeadStage(id, *req.LeadStage)
		if !valid {
			c.JSON(400, gin.H{"error": "Status CRM tidak valid"})
			return
		}
		ct.LeadStage = stage
		ct.LeadStageSource = "manual"
		ct.LeadStageReason = "Status diperbarui manual"
		ct.LeadStageConfidence = 1
		ct.LeadStageLocked = true
		now := time.Now()
		ct.LeadStageUpdatedAt = &now
	}
	if req.StageLock != nil {
		ct.LeadStageLocked = *req.StageLock
		if *req.StageLock {
			ct.LeadStageSource = "manual"
			ct.LeadStageReason = "Status dikunci manual"
			ct.LeadStageConfidence = 1
			now := time.Now()
			ct.LeadStageUpdatedAt = &now
		} else {
			ct.LeadStageReason = "Status manual saat ini; penilaian AI berikutnya diizinkan"
		}
	}
	if err := database.DB.Save(&ct).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan kontak"})
		return
	}
	c.JSON(200, gin.H{"data": ct})
}

func DeleteSavedContact(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	_ = database.DB.Where("agent_id = ?", id).Delete(&models.Contact{}, c.Param("cid")).Error
	c.JSON(200, gin.H{"message": "Deleted"})
}

// BulkTagSavedContacts menambahkan tag ke beberapa kontak sekaligus.
// Body: { ids: number[], tag: string }. Tag baru ditambahkan (append)
// tanpa menghapus tag yang sudah ada. Duplikat otomatis dibuang.
func BulkTagSavedContacts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		IDs []uint `json:"ids"`
		Tag string `json:"tag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	tag := strings.TrimSpace(req.Tag)
	if tag == "" {
		c.JSON(400, gin.H{"error": "Tag wajib diisi"})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(400, gin.H{"error": "Pilih minimal satu kontak"})
		return
	}

	var contacts []models.Contact
	database.DB.Where("agent_id = ? AND id IN ?", id, req.IDs).Find(&contacts)
	if len(contacts) == 0 {
		c.JSON(404, gin.H{"error": "Kontak tidak ditemukan"})
		return
	}

	updated := 0
	for _, ct := range contacts {
		existing := tagList(ct.Tags)
		lowerTag := strings.ToLower(tag)
		already := false
		for _, t := range existing {
			if strings.ToLower(t) == lowerTag {
				already = true
				break
			}
		}
		if already {
			continue
		}
		existing = append(existing, tag)
		ct.Tags = normalizeTags(strings.Join(existing, ","))
		_ = database.DB.Save(&ct).Error
		updated++
	}

	c.JSON(200, gin.H{"message": "Tag ditambahkan", "updated": updated, "total": len(contacts)})
}

// BulkStageSavedContacts mengubah status CRM beberapa kontak sekaligus.
func BulkStageSavedContacts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		IDs       []uint `json:"ids"`
		LeadStage string `json:"lead_stage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(400, gin.H{"error": "Pilih minimal satu kontak"})
		return
	}
	stage, valid := normalizeLeadStage(id, req.LeadStage)
	if !valid {
		c.JSON(400, gin.H{"error": "Status CRM tidak valid"})
		return
	}
	res := database.DB.Model(&models.Contact{}).
		Where("agent_id = ? AND id IN ?", id, req.IDs).
		Updates(map[string]any{
			"lead_stage": stage, "lead_stage_source": "manual", "lead_stage_reason": "Status diperbarui manual",
			"lead_stage_confidence": 1, "lead_stage_locked": true, "lead_stage_updated_at": time.Now(),
		})
	if res.Error != nil {
		c.JSON(500, gin.H{"error": "Gagal mengubah status CRM"})
		return
	}
	c.JSON(200, gin.H{"message": "Status CRM diperbarui", "updated": res.RowsAffected})
}

// ImportSavedContacts memasukkan banyak kontak sekaligus dari berbagai sumber
// (input manual, nomor terkoneksi, atau unggahan CSV). Body:
//
//	{ contacts: [{number, name}], tag?: string }
//
// Nomor yang sudah ada dilewati (tidak menimpa nama/tag lama). Jika `tag` diisi,
// tag itu dipasang ke semua kontak baru yang berhasil dibuat.
func ImportSavedContacts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Contacts []struct {
			Number string `json:"number"`
			Name   string `json:"name"`
		} `json:"contacts"`
		Tag string `json:"tag"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if len(req.Contacts) == 0 {
		c.JSON(400, gin.H{"error": "Tidak ada kontak untuk diimpor"})
		return
	}

	// Nomor yang sudah tersimpan, supaya tidak dobel.
	var existing []models.Contact
	database.DB.Where("agent_id = ?", id).Find(&existing)
	have := make(map[string]bool, len(existing))
	for _, ct := range existing {
		have[ct.Number] = true
	}

	tag := normalizeTags(req.Tag)
	fresh := make([]models.Contact, 0, len(req.Contacts))
	imported, skipped := 0, 0
	for _, r := range req.Contacts {
		num := services.NormalizePhone(r.Number)
		if num == "" {
			skipped++
			continue
		}
		if have[num] {
			skipped++
			continue
		}
		have[num] = true // cegah duplikat di dalam batch yang sama
		fresh = append(fresh, models.Contact{
			AgentID: id, Number: num, Name: strings.TrimSpace(r.Name), Tags: tag, LeadStage: leadStageNew,
		})
		imported++
	}

	if len(fresh) > 0 {
		if err := database.DB.CreateInBatches(fresh, 200).Error; err != nil {
			c.JSON(500, gin.H{"error": "Gagal menyimpan kontak"})
			return
		}
	}
	c.JSON(200, gin.H{"message": "Impor selesai", "imported": imported, "skipped": skipped})
}

// BulkDeleteSavedContacts menghapus banyak kontak sekaligus. Body:
//
//	{ ids: number[] }              -> hapus kontak terpilih
//	{ all: true, q?, tag? }        -> hapus SEMUA kontak yang cocok filter (atau semua)
//
// Mode `all` mengikuti filter pencarian/tag yang sedang aktif di UI, jadi user bisa
// "hapus semua hasil pencarian ini" tanpa harus mencentang satu per satu.
func BulkDeleteSavedContacts(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		IDs   []uint `json:"ids"`
		All   bool   `json:"all"`
		Q     string `json:"q"`
		Tag   string `json:"tag"`
		Stage string `json:"stage"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}

	if !req.All {
		if len(req.IDs) == 0 {
			c.JSON(400, gin.H{"error": "Pilih minimal satu kontak"})
			return
		}
		res := database.DB.Where("agent_id = ? AND id IN ?", id, req.IDs).Delete(&models.Contact{})
		if res.Error != nil {
			c.JSON(500, gin.H{"error": "Gagal menghapus kontak"})
			return
		}
		c.JSON(200, gin.H{"message": "Kontak dihapus", "deleted": res.RowsAffected})
		return
	}

	// Mode "hapus semua sesuai filter". Tanpa filter = kosongkan buku kontak.
	q := strings.ToLower(strings.TrimSpace(req.Q))
	tag := strings.ToLower(strings.TrimSpace(req.Tag))
	stage := ""
	if strings.TrimSpace(req.Stage) != "" {
		var valid bool
		stage, valid = normalizeLeadStage(id, req.Stage)
		if !valid {
			c.JSON(400, gin.H{"error": "Status CRM tidak valid"})
			return
		}
	}
	if q == "" && tag == "" && stage == "" {
		res := database.DB.Where("agent_id = ?", id).Delete(&models.Contact{})
		if res.Error != nil {
			c.JSON(500, gin.H{"error": "Gagal menghapus kontak"})
			return
		}
		c.JSON(200, gin.H{"message": "Semua kontak dihapus", "deleted": res.RowsAffected})
		return
	}

	// Ada filter: cari id yang cocok dulu (tag disimpan sebagai string koma, jadi
	// difilter di Go agar cocok per-tag persis, bukan substring).
	var contacts []models.Contact
	database.DB.Where("agent_id = ?", id).Find(&contacts)
	ids := make([]uint, 0)
	for _, ct := range contacts {
		if q != "" && !strings.Contains(strings.ToLower(ct.Name), q) && !strings.Contains(ct.Number, q) {
			continue
		}
		if tag != "" {
			has := false
			for _, t := range tagList(ct.Tags) {
				if strings.ToLower(t) == tag {
					has = true
					break
				}
			}
			if !has {
				continue
			}
		}
		if stage != "" && normalizedContactStage(id, ct.LeadStage) != stage {
			continue
		}
		ids = append(ids, ct.ID)
	}
	if len(ids) == 0 {
		c.JSON(200, gin.H{"message": "Tidak ada kontak yang cocok", "deleted": 0})
		return
	}
	res := database.DB.Where("agent_id = ? AND id IN ?", id, ids).Delete(&models.Contact{})
	if res.Error != nil {
		c.JSON(500, gin.H{"error": "Gagal menghapus kontak"})
		return
	}
	c.JSON(200, gin.H{"message": "Kontak dihapus", "deleted": res.RowsAffected})
}
