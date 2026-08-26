package handlers

import (
	"encoding/json"
	"sort"
	"strings"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// GetPipeline mengembalikan definisi tahap + konfigurasi + aturan pelabelan agent.
func GetPipeline(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	database.EnsureDefaultStages(id)
	cfg := database.GetLeadLabelConfig(id)
	defs := database.GetStageDefs(id)

	var rules []models.LabelRule
	database.DB.Where("agent_id = ?", id).Order("priority asc, id asc").Find(&rules)

	c.JSON(200, gin.H{
		"stages": defs,
		"config": gin.H{
			"smart_labels_enabled": cfg.SmartLabelsEnabled,
			"closing_definition":   cfg.ClosingDefinition,
		},
		"rules": rules,
	})
}

// SavePipelineStages menyimpan seluruh definisi tahap (bulk upsert).
func SavePipelineStages(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Stages []models.LeadStageDef `json:"stages"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if len(req.Stages) == 0 {
		c.JSON(400, gin.H{"error": "Minimal satu tahap wajib ada"})
		return
	}
	if len(req.Stages) > 12 {
		c.JSON(400, gin.H{"error": "Maksimal 12 tahap"})
		return
	}
	defs, err := database.NormalizePipelineStages(id, req.Stages)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"stages": defs})
}

// SavePipelineConfig menyimpan konfigurasi pelabelan pintar.
func SavePipelineConfig(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		SmartLabelsEnabled bool   `json:"smart_labels_enabled"`
		ClosingDefinition  string `json:"closing_definition"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	if len([]rune(strings.TrimSpace(req.ClosingDefinition))) > 2000 {
		c.JSON(400, gin.H{"error": "Definisi closing maksimal 2000 karakter"})
		return
	}
	cfg := database.SaveLeadLabelConfig(id, req.SmartLabelsEnabled, req.ClosingDefinition)
	c.JSON(200, gin.H{
		"config": gin.H{
			"smart_labels_enabled": cfg.SmartLabelsEnabled,
			"closing_definition":   cfg.ClosingDefinition,
		},
	})
}

// SaveLabelRule membuat atau memperbarui satu aturan pelabelan.
func SaveLabelRule(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		ID              uint     `json:"id"`
		Name            string   `json:"name"`
		Enabled         bool     `json:"enabled"`
		Priority        int      `json:"priority"`
		TriggerKeywords []string `json:"trigger_keywords"`
		TriggerStage    string   `json:"trigger_stage"`
		ActionStage     string   `json:"action_stage"`
		ActionWALabel   string   `json:"action_wa_label"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(400, gin.H{"error": "Nama aturan wajib diisi"})
		return
	}
	keywords := normalizeRuleKeywords(req.TriggerKeywords)
	if len(keywords) == 0 {
		c.JSON(400, gin.H{"error": "Minimal satu kata kunci pemicu wajib diisi (min. 3 karakter)"})
		return
	}
	defs := database.GetStageDefMap(id)
	if req.TriggerStage != "" {
		if _, valid := defs[req.TriggerStage]; !valid {
			c.JSON(400, gin.H{"error": "Tahap pemicu tidak valid"})
			return
		}
	}
	if req.ActionStage != "" {
		def, valid := defs[req.ActionStage]
		if !valid {
			c.JSON(400, gin.H{"error": "Tahap aksi tidak valid"})
			return
		}
		// Invariant: closing hanya dari aktivitas terkonfirmasi/manual — tidak dari aturan.
		if def.IsClosing {
			c.JSON(400, gin.H{"error": "Tahap closing tidak boleh jadi aksi aturan (closing hanya dari transaksi terkonfirmasi)"})
			return
		}
	}
	kwJSON, _ := json.Marshal(keywords)
	if req.ActionStage == "" && strings.TrimSpace(req.ActionWALabel) == "" {
		c.JSON(400, gin.H{"error": "Pilih minimal satu aksi: ubah tahap dan/atau beri label WhatsApp"})
		return
	}

	if req.ID > 0 {
		var existing models.LabelRule
		if database.DB.Where("id = ? AND agent_id = ?", req.ID, id).First(&existing).Error != nil {
			c.JSON(404, gin.H{"error": "Aturan tidak ditemukan"})
			return
		}
		updates := map[string]any{
			"name": req.Name, "enabled": req.Enabled, "priority": req.Priority,
			"trigger_keywords": string(kwJSON), "trigger_stage": req.TriggerStage,
			"action_stage": req.ActionStage, "action_wa_label": strings.TrimSpace(req.ActionWALabel),
		}
		database.DB.Model(&existing).Updates(updates)
		database.DB.First(&existing, existing.ID)
		c.JSON(200, gin.H{"rule": existing})
		return
	}

	rule := models.LabelRule{
		AgentID: id, Name: req.Name, Enabled: req.Enabled,
		Priority: req.Priority, TriggerKeywords: string(kwJSON), TriggerStage: req.TriggerStage,
		ActionStage: req.ActionStage, ActionWALabel: strings.TrimSpace(req.ActionWALabel),
	}
	if err := database.DB.Create(&rule).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan aturan"})
		return
	}
	c.JSON(200, gin.H{"rule": rule})
}

// DeleteLabelRule menghapus satu aturan.
func DeleteLabelRule(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	ruleID := c.Param("rid")
	if ruleID == "" {
		c.JSON(400, gin.H{"error": "ID aturan wajib"})
		return
	}
	res := database.DB.Where("id = ? AND agent_id = ?", ruleID, id).Delete(&models.LabelRule{})
	if res.Error != nil {
		c.JSON(500, gin.H{"error": "Gagal menghapus aturan"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "Aturan tidak ditemukan"})
		return
	}
	c.JSON(200, gin.H{"message": "Aturan dihapus"})
}

// TestLabelRules menjalankan aturan secara DRY-RUN terhadap teks contoh
// (tanpa mengubah data apa pun) supaya user bisa memverifikasi sebelum aktif.
func TestLabelRules(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Text) == "" {
		c.JSON(400, gin.H{"error": "Teks contoh wajib diisi"})
		return
	}
	var rules []models.LabelRule
	database.DB.Where("agent_id = ? AND enabled = ?", id, true).Order("priority asc, id asc").Find(&rules)
	text := strings.ToLower(strings.TrimSpace(req.Text))
	matched := make([]gin.H, 0, len(rules))
	for _, rule := range rules {
		var keywords []string
		if err := json.Unmarshal([]byte(rule.TriggerKeywords), &keywords); err != nil || len(keywords) == 0 {
			continue
		}
		if !services.MatchRuleKeywords(keywords, text) {
			continue
		}
		matched = append(matched, gin.H{
			"id": rule.ID, "name": rule.Name, "priority": rule.Priority,
			"trigger_stage": rule.TriggerStage, "action_stage": rule.ActionStage,
			"action_wa_label": rule.ActionWALabel,
		})
	}
	c.JSON(200, gin.H{"matched": matched, "count": len(matched)})
}

// normalizeRuleKeywords merapikan kata kunci: trim, lowercase, dedup, buang < 3 karakter.
func normalizeRuleKeywords(raw []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, k := range raw {
		k = strings.ToLower(strings.TrimSpace(k))
		if len([]rune(k)) < 3 || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
