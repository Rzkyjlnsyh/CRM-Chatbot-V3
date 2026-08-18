package models

import "time"

// MetaConversion = dedup + log event CAPI Meta yang sudah dikirim.
// Satu kontak + satu label = satu event (anti dobel kirim).
type MetaConversion struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   uint      `gorm:"uniqueIndex:idx_meta_conv,priority:1;not null" json:"agent_id"`
	Sender    string    `gorm:"uniqueIndex:idx_meta_conv,priority:2;size:32;not null" json:"sender"`
	LabelID   string    `gorm:"uniqueIndex:idx_meta_conv,priority:3;size:64;not null" json:"label_id"`
	EventName string    `gorm:"size:32" json:"event_name"`
	Status    string    `gorm:"size:12;default:sent" json:"status"` // sent / failed
	Response  string    `gorm:"size:255" json:"response"`
	SentAt    time.Time `json:"sent_at"`
}
