package models

import "time"

// AutoReply = aturan balasan instan berbasis kata kunci (tanpa AI). Dicek sebelum AI.
type AutoReply struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   uint      `gorm:"index;not null" json:"agent_id"`
	Keywords  string    `gorm:"type:text" json:"keywords"`                  // dipisah koma
	MatchType string    `gorm:"size:16;default:contains" json:"match_type"` // contains, exact, prefix
	Reply     string    `gorm:"type:text" json:"reply"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	SortOrder int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
