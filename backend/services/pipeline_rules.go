package services

import (
	"encoding/json"
	"log"
	"sort"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// RuleMatchResult = hasil penerapan satu aturan pelabelan otomatis.
type RuleMatchResult struct {
	RuleID         uint   `json:"rule_id"`
	RuleName       string `json:"rule_name"`
	StageChanged   bool   `json:"stage_changed"`
	Stage          string `json:"stage,omitempty"`
	WALabelApplied bool   `json:"wa_label_applied"`
}

// ApplyLabelRules menjalankan aturan deterministic untuk satu pesan customer.
// BUKAN AI — murni kecocokan keyword yang ditulis user, jadi tidak mungkin "halu".
// Guardrail:
//   - kontak terkunci manual (lead_stage_locked) → semua aturan dilewati
//   - kontak pada tahap closing → semua aturan dilewati (sticky, deal selesai tak diganggu)
//   - tahap aksi harus valid di definisi pipeline agent
func ApplyLabelRules(agentID uint, sender, messageText string) []RuleMatchResult {
	text := strings.ToLower(strings.TrimSpace(messageText))
	if text == "" {
		return nil
	}
	var contact models.Contact
	if database.DB.Where("agent_id = ? AND number = ?", agentID, sender).First(&contact).Error != nil {
		return nil
	}
	if contact.LeadStageLocked {
		return nil
	}
	defs := database.GetStageDefMap(agentID)
	if d, ok := defs[contact.LeadStage]; ok && d.IsClosing {
		return nil
	}

	var rules []models.LabelRule
	database.DB.Where("agent_id = ? AND enabled = ?", agentID, true).Order("priority asc, id asc").Find(&rules)
	results := make([]RuleMatchResult, 0, len(rules))
	for _, rule := range rules {
		var keywords []string
		if err := json.Unmarshal([]byte(rule.TriggerKeywords), &keywords); err != nil || len(keywords) == 0 {
			continue
		}
		if rule.TriggerStage != "" && rule.TriggerStage != contact.LeadStage {
			continue
		}
		if !MatchRuleKeywords(keywords, text) {
			continue
		}
		res := RuleMatchResult{RuleID: rule.ID, RuleName: rule.Name}
		if rule.ActionStage != "" && rule.ActionStage != contact.LeadStage {
			// Defense in depth: closing tidak pernah dijadikan aksi aturan.
			if d, ok := defs[rule.ActionStage]; ok && !d.IsClosing {
				applyRuleStage(agentID, sender, contact.ID, rule.ActionStage, rule.Name)
				res.StageChanged = true
				res.Stage = rule.ActionStage
				contact.LeadStage = rule.ActionStage
			}
		}
		if rule.ActionWALabel != "" {
			if ApplyWALabelByName(agentID, sender, rule.ActionWALabel) {
				res.WALabelApplied = true
			}
		}
		results = append(results, res)
	}
	return results
}

// MatchRuleKeywords = kecocokan substring lowercase. Keyword < 3 karakter diabaikan
// agar kata umum ("di", "ke", "ya") tidak memicu label palsu.
func MatchRuleKeywords(keywords []string, text string) bool {
	for _, k := range keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if len([]rune(k)) < 3 {
			continue
		}
		if strings.Contains(text, k) {
			return true
		}
	}
	return false
}

// applyRuleStage menerapkan tahap dari aturan. Sumber = "rule" agar UI bisa membedakan
// dari penilaian AI. Guardrail manual-lock & closing sudah dicek pemanggil;
// di sini dicek ulang lewat UPDATE bersyarat agar aman dari race.
func applyRuleStage(agentID uint, sender string, contactID uint, stage, ruleName string) {
	now := time.Now()
	res := database.DB.Model(&models.Contact{}).
		Where("id = ? AND agent_id = ? AND lead_stage_locked = ?", contactID, agentID, false).
		Updates(map[string]any{
			"lead_stage":            stage,
			"lead_stage_source":     "rule",
			"lead_stage_reason":     "Aturan otomatis: " + ruleName,
			"lead_stage_confidence": 1,
			"lead_stage_updated_at": &now,
		})
	if res.Error != nil {
		log.Printf("[label-rule] gagal terapkan tahap %s ke %s: %v", stage, sender, res.Error)
	}
}

// ApplyWALabelByName mencari label WhatsApp berdasarkan nama dan menempelkannya
// ke kontak (sync dua arah: server WA + DB lokal). Tidak error bila WA offline —
// label tetap tersimpan lokal agar sinkron saat WA kembali tersedia.
func ApplyWALabelByName(agentID uint, sender, labelName string) bool {
	var label models.Label
	if database.DB.Where("agent_id = ? AND name LIKE ?", agentID, "%"+strings.TrimSpace(labelName)+"%").First(&label).Error != nil {
		log.Printf("[label-rule] label WA '%s' tidak ditemukan untuk agent %d", labelName, agentID)
		return false
	}
	if err := WA(agentID).ApplyLabel(sender, label.LabelID, true); err != nil {
		log.Printf("[label-rule] gagal sync label '%s' ke WA (tetap simpan lokal): %v", labelName, err)
	}
	cl := models.ChatLabel{AgentID: agentID, LabelID: label.LabelID, Sender: sender}
	if database.DB.Where(cl).FirstOrCreate(&cl).Error != nil {
		return false
	}
	return true
}

// SortRules mengurutkan aturan berdasarkan priority (untuk tampilan & pengujian).
func SortRules(rules []models.LabelRule) {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
}
