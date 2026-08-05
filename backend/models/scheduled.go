package models

import "time"

// ScheduledMessage = broadcast/pesan yang dijadwalkan untuk dikirim pada waktu tertentu.
// Penerima sudah di-resolve saat dibuat (disimpan JSON) lalu dijalankan lewat mesin broadcast.
type ScheduledMessage struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	TenantID           uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID            uint      `gorm:"index;not null" json:"agent_id"`
	RunAt              time.Time `gorm:"index" json:"run_at"`
	Message            string    `gorm:"type:text" json:"message"`
	ProductID          uint      `gorm:"index" json:"product_id,omitempty"`
	ProductButtonsJSON string    `gorm:"type:text" json:"product_buttons_json,omitempty"`
	Recipients         string    `gorm:"type:longtext" json:"-"` // JSON [{number,name}]; untuk grup, "number" berisi JID grup.
	RecipientCount     int       `json:"recipient_count"`
	// TargetType: "number" (default, kirim ke nomor pribadi) atau "group" (post ke dalam grup).
	TargetType string `gorm:"size:16;default:number;index" json:"target_type"`
	// Lampiran opsional.
	MediaType        string     `gorm:"size:16" json:"media_type"`
	MediaPath        string     `json:"-"`
	FileName         string     `json:"file_name"`
	Mimetype         string     `json:"mimetype"`
	MinDelay         int        `json:"min_delay"`
	MaxDelay         int        `json:"max_delay"`
	RestEvery        int        `json:"rest_every"`                                    // jeda istirahat tiap N pesan (0 = mati)
	RestDuration     int        `json:"rest_duration"`                                 // lama istirahat (detik)
	Status           string     `gorm:"size:16;default:scheduled;index" json:"status"` // scheduled, running, resuming, wa_restricted, done, cancelled, interrupted
	ConsentCategory  string     `gorm:"size:32;index" json:"consent_category"`
	ConsentSource    string     `gorm:"size:48" json:"consent_source"`
	RiskLevel        string     `gorm:"size:16;index" json:"risk_level"`
	RiskReasons      string     `gorm:"type:text" json:"-"`
	RiskAcknowledged bool       `gorm:"not null;default:false" json:"risk_acknowledged"`
	OverrideReason   string     `gorm:"type:text" json:"override_reason,omitempty"`
	OverrideBy       *uint      `gorm:"index" json:"override_by,omitempty"`
	OverrideAt       *time.Time `json:"override_at,omitempty"`
	BroadcastID      *uint      `json:"broadcast_id"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ScheduledStatus = postingan WhatsApp Status (Story) yang dijadwalkan / diposting.
// Berbeda dari broadcast: satu postingan 24 jam ke status@broadcast, bukan pesan japri
// per penerima — whatsmeow otomatis menyusun audiens dari privasi status + kontak.
type ScheduledStatus struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TenantID  uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID   uint      `gorm:"index;not null" json:"agent_id"`
	RunAt     time.Time `gorm:"index" json:"run_at"`
	Text      string    `gorm:"type:text" json:"text"`
	MediaType string    `gorm:"size:16" json:"media_type"` // "" (teks) atau "image"
	MediaPath string    `json:"-"`
	Mimetype  string    `json:"mimetype"`
	Status    string    `gorm:"size:16;default:scheduled;index" json:"status"` // scheduled, done, failed, cancelled, interrupted
	Error     string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
