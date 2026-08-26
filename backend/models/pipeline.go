package models

import "time"

// LeadStageDef = definisi tahap pipeline CRM per agent. User bisa menambah,
// mengubah nama/warna/urutan, dan menentukan ambang keyakinan AI per tahap.
// Tahap bawaan (new/cold/warm/hot/unqualified/customer) di-seed otomatis.
type LeadStageDef struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	AgentID       uint      `gorm:"not null;uniqueIndex:idx_stagedef_agent_key,priority:1" json:"agent_id"`

	Key           string    `gorm:"size:32;not null;uniqueIndex:idx_stagedef_agent_key,priority:2" json:"key"`
	Name          string    `gorm:"size:64;not null" json:"name"`
	Color         string    `gorm:"size:16;not null;default:#90A4AE" json:"color"`
	Rank          int       `gorm:"not null;default:0" json:"rank"`
	Description   string    `gorm:"type:text" json:"description"`                // definisi tahap untuk prompt AI
	IsClosing     bool      `gorm:"not null;default:false" json:"is_closing"`    // deal selesai — TIDAK PERNAH ditetapkan AI
	MinConfidence float64   `gorm:"not null;default:0.72" json:"min_confidence"` // ambang keyakinan AI
	IsDefault     bool      `gorm:"not null;default:false" json:"is_default"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// LeadLabelConfig = pengaturan pelabelan pintar per agent.
type LeadLabelConfig struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	AgentID            uint      `gorm:"uniqueIndex;not null" json:"agent_id"`

	SmartLabelsEnabled bool      `gorm:"not null;default:true" json:"smart_labels_enabled"` // matikan = AI tak menilai tahap
	ClosingDefinition  string    `gorm:"type:text" json:"closing_definition"`               // arti "closing" utk bisnis ini
	UpdatedAt          time.Time `json:"updated_at"`
}

// LabelRule = aturan pelabelan otomatis deterministic (bukan AI).
// Diproses berurutan sesuai priority; cocok = aksi diterapkan.
type LabelRule struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	AgentID         uint      `gorm:"index;not null" json:"agent_id"`

	Name            string    `gorm:"size:80;not null" json:"name"`
	Enabled         bool      `gorm:"not null;default:true" json:"enabled"`
	Priority        int       `gorm:"not null;default:0" json:"priority"`
	TriggerKeywords string    `gorm:"type:text;not null" json:"trigger_keywords"` // JSON array string (lowercase)
	TriggerStage    string    `gorm:"size:32" json:"trigger_stage"`               // "" = semua tahap
	ActionStage     string    `gorm:"size:32" json:"action_stage"`                // "" = tak ubah tahap
	ActionWALabel   string    `gorm:"size:80" json:"action_wa_label"`             // nama label WhatsApp; "" = tak beri label
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
