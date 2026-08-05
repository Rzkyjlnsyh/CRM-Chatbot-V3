package models

import "time"

// AIForm adalah alur pengumpulan data untuk layanan non-produk seperti booking,
// pendaftaran, konsultasi, survey, atau lead qualification.
type AIForm struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TenantID        uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID         uint      `gorm:"index;not null" json:"agent_id"`
	Name            string    `gorm:"size:160;not null" json:"name"`
	Goal            string    `gorm:"type:text" json:"goal"`
	IntentHintsJSON string    `gorm:"type:text" json:"intent_hints_json"`
	StepsJSON       string    `gorm:"type:text" json:"steps_json"`
	Enabled         bool      `gorm:"not null;default:true" json:"enabled"`
	Handoff         bool      `gorm:"not null;default:true" json:"handoff"`
	SuccessMessage  string    `gorm:"type:text" json:"success_message"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AIFormSession struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID   uint      `gorm:"uniqueIndex:idx_ai_form_agent_sender;not null" json:"agent_id"`
	Sender    string    `gorm:"uniqueIndex:idx_ai_form_agent_sender;size:32;not null" json:"sender"`
	FormID    uint      `gorm:"index;not null" json:"form_id"`
	StepIndex int       `gorm:"not null;default:0" json:"step_index"`
	DataJSON  string    `gorm:"type:text" json:"data_json"`
	Status    string    `gorm:"size:24;index;not null;default:collecting" json:"status"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AIFormSubmission struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID   uint      `gorm:"index;not null" json:"agent_id"`
	FormID    uint      `gorm:"index;not null" json:"form_id"`
	Sender    string    `gorm:"index;size:32;not null" json:"sender"`
	Code      string    `gorm:"uniqueIndex;size:32;not null" json:"code"`
	Status    string    `gorm:"size:24;index;not null;default:submitted" json:"status"`
	DataJSON  string    `gorm:"type:text" json:"data_json"`
	Summary   string    `gorm:"type:text" json:"summary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Form      AIForm    `gorm:"foreignKey:FormID" json:"form"`
}
