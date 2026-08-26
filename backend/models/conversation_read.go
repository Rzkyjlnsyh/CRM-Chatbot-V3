package models

import "time"

// ConversationRead = posisi baca CS per percakapan (inbox). Unread = pesan
// masuk (from_human=false) dengan id lebih besar dari LastReadChatID.
type ConversationRead struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	AgentID        uint      `gorm:"not null;uniqueIndex:idx_convread_agent_sender,priority:1" json:"agent_id"`
	Sender         string    `gorm:"size:32;not null;uniqueIndex:idx_convread_agent_sender,priority:2" json:"sender"`
	LastReadChatID uint      `gorm:"not null;default:0" json:"last_read_chat_id"`
	UpdatedAt      time.Time `json:"updated_at"`
}
