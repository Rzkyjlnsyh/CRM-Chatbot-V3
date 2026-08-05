package models

import "time"

// MetaConversionEvent adalah outbox untuk event Conversions API.
// EventID unik menjaga event bisnis yang sama tidak dikirim dua kali.
type MetaConversionEvent struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	TenantID       uint       `gorm:"index" json:"tenant_id"`
	EventID        string     `gorm:"size:100;uniqueIndex;not null" json:"event_id"`
	EventName      string     `gorm:"size:48;index;not null" json:"event_name"`
	EventTime      time.Time  `gorm:"index;not null" json:"event_time"`
	SourceURL      string     `gorm:"type:text" json:"source_url"`
	UserDataJSON   string     `gorm:"type:text" json:"-"`
	CustomDataJSON string     `gorm:"type:text" json:"-"`
	Status         string     `gorm:"size:16;index;default:pending" json:"status"`
	Attempts       int        `gorm:"not null;default:0" json:"attempts"`
	NextAttemptAt  time.Time  `gorm:"index" json:"next_attempt_at"`
	LastError      string     `gorm:"type:text" json:"last_error,omitempty"`
	MetaResponse   string     `gorm:"type:text" json:"-"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
