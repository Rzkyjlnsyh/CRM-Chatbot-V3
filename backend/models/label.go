package models

import "time"

// Label = label WhatsApp (Business) yang disinkron dari event app-state.
type Label struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   uint      `gorm:"not null;uniqueIndex:idx_label_agent,priority:1" json:"agent_id"`
	LabelID   string    `gorm:"size:64;not null;uniqueIndex:idx_label_agent,priority:2" json:"label_id"`
	Name      string    `json:"name"`
	Color     int       `json:"color"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChatLabel = relasi kontak <-> label (chat yang diberi label tertentu).
type ChatLabel struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	AgentID uint   `gorm:"not null;uniqueIndex:idx_chatlabel,priority:1" json:"agent_id"`
	LabelID string `gorm:"size:64;not null;uniqueIndex:idx_chatlabel,priority:2" json:"label_id"`
	Sender  string `gorm:"size:32;not null;uniqueIndex:idx_chatlabel,priority:3" json:"sender"`
}
