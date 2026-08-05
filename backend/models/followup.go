package models

import "time"

// FollowUp = urutan pesan susulan (drip). Kontak "didaftarkan", lalu tiap langkah
// dikirim otomatis setelah jeda tertentu dari waktu pendaftaran.
type FollowUp struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	TenantID    uint      `gorm:"index;not null" json:"tenant_id"`
	AgentID     uint      `gorm:"index;not null" json:"agent_id"`
	Name        string    `gorm:"size:160;not null" json:"name"`
	Enabled     bool      `gorm:"not null;default:true" json:"enabled"`
	StopOnReply bool      `gorm:"not null;default:true" json:"stop_on_reply"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// FollowUpStep = satu langkah dalam urutan: pesan + jeda (jam) dari saat pendaftaran.
// Jika AiGenerated=true, Message menjadi instruksi untuk AI (bukan teks literal).
type FollowUpStep struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	FollowUpID    uint      `gorm:"index;not null" json:"follow_up_id"`
	StepOrder     int       `gorm:"not null;default:0" json:"step_order"`
	DelayHours    int       `gorm:"not null;default:0" json:"delay_hours"` // jeda dari waktu enroll
	Message       string    `gorm:"type:text" json:"message"`
	AiGenerated   bool      `gorm:"not null;default:false" json:"ai_generated"`
	AiInstruction string    `gorm:"type:text" json:"ai_instruction"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// FollowUpEnrollment = satu kontak yang sedang menjalani sebuah urutan follow-up.
type FollowUpEnrollment struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	FollowUpID    uint       `gorm:"not null;uniqueIndex:idx_enroll,priority:1" json:"follow_up_id"`
	Number        string     `gorm:"size:32;not null;uniqueIndex:idx_enroll,priority:2" json:"number"`
	TenantID      uint       `gorm:"index;not null" json:"tenant_id"`
	AgentID       uint       `gorm:"index;not null" json:"agent_id"`
	Name          string     `json:"name"`
	EnrolledAt    time.Time  `json:"enrolled_at"`
	NextStep      int        `gorm:"not null;default:0" json:"next_step"`
	Status        string     `gorm:"size:16;default:active;index" json:"status"` // active, completed, stopped
	StoppedReason string     `gorm:"size:32" json:"stopped_reason"`
	LastSentAt    *time.Time `json:"last_sent_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
