package models

import "time"

// Template = pesan siap-pakai (quick reply) milik agent.
// Dipakai ulang di Inbox, Broadcast, dan Kalender. Body mendukung {nama}.
type Template struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   uint      `gorm:"index;not null" json:"agent_id"`
	Title     string    `gorm:"size:120;not null" json:"title"`
	Body      string    `gorm:"type:text" json:"body"`
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
