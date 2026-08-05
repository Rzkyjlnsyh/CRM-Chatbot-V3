package models

import "time"

// GroupGuardConfig = setelan moderasi anti-spam untuk satu grup milik sebuah agent.
type GroupGuardConfig struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	TenantID  uint   `gorm:"index" json:"tenant_id"`
	AgentID   uint   `gorm:"index;not null;uniqueIndex:idx_groupguard_agent_group,priority:1" json:"agent_id"`
	GroupJID  string `gorm:"column:group_jid;size:64;not null;uniqueIndex:idx_groupguard_agent_group,priority:2" json:"group_jid"`
	GroupName string `gorm:"size:128" json:"group_name"`
	Enabled   bool   `gorm:"not null;default:false" json:"enabled"`

	// Aksi saat spam terdeteksi.
	DeleteSpam  bool `gorm:"not null;default:true" json:"delete_spam"`   // hapus pesannya (butuh bot admin)
	FlagForKick bool `gorm:"not null;default:true" json:"flag_for_kick"` // buat antrean konfirmasi keluarkan
	AutoKick    bool `gorm:"not null;default:false" json:"auto_kick"`    // Fase 2: keluarkan otomatis

	// Aturan deteksi.
	BlockLinks     bool   `gorm:"not null;default:true" json:"block_links"`
	BlockPhones    bool   `gorm:"not null;default:false" json:"block_phones"`
	BlockWords     string `gorm:"type:text" json:"block_words"`           // dipisah baris/koma
	FloodCount     int    `gorm:"not null;default:0" json:"flood_count"`  // 0 = mati
	FloodWindowSec int    `gorm:"not null;default:10" json:"flood_window_sec"`
	AllowNumbers   string `gorm:"type:text" json:"allow_numbers"`         // nomor dikecualikan (dipisah baris/koma)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GroupModerationLog = catatan tiap aksi/deteksi moderasi (audit + feed konfirmasi).
type GroupModerationLog struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	AgentID    uint   `gorm:"index;not null" json:"agent_id"`
	GroupJID   string `gorm:"column:group_jid;size:64;index" json:"group_jid"`
	GroupName  string `gorm:"size:128" json:"group_name"`
	Sender     string `gorm:"size:32" json:"sender"`      // nomor pengirim (untuk tampil)
	SenderJID  string `gorm:"column:sender_jid;size:64" json:"-"`           // JID asli (untuk kick/revoke)
	SenderName string `gorm:"size:128" json:"sender_name"`
	WAMsgID    string `gorm:"size:64" json:"-"`           // id pesan WA (untuk revoke)
	Action     string `gorm:"size:16;index" json:"action"`  // flagged, deleted, kicked, warned, dismissed
	Reason     string `gorm:"size:128" json:"reason"`
	Excerpt    string `gorm:"size:280" json:"excerpt"`
	Status     string `gorm:"size:16;index;default:done" json:"status"` // pending (tunggu konfirmasi), done, dismissed
	ActedBy    string `gorm:"size:32" json:"acted_by"`                  // "auto" atau user id
	CreatedAt  time.Time `json:"created_at"`
}
