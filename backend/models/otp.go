package models

import "time"

// OTPCode = kode OTP yang dibuat & dikirim lewat REST API, lalu diverifikasi.
// Kode disimpan ter-hash (bukan plaintext). Keamanan sesungguhnya dari kombinasi
// masa berlaku (ExpiresAt), batas percobaan (Attempts), dan cooldown saat request.
type OTPCode struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   uint      `gorm:"index;not null" json:"agent_id"`
	Number    string    `gorm:"index;size:32;not null" json:"number"`
	CodeHash  string    `gorm:"size:64;not null" json:"-"`
	ExpiresAt time.Time `gorm:"index" json:"expires_at"`
	Attempts  int       `gorm:"not null;default:0" json:"attempts"`
	Used      bool      `gorm:"not null;default:false" json:"used"`
	CreatedAt time.Time `json:"created_at"`
}
