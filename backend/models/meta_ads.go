package models

import "time"

// MetaAdsSnapshot = snapshot hasil iklan Meta (Marketing API Insights).
// Disimpan berkala supaya ada riwayat walau token Meta kadaluwarsa.
type MetaAdsSnapshot struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AccountID string    `gorm:"size:40;index" json:"account_id"`
	DateFrom  string    `gorm:"size:16" json:"date_from"`
	DateTo    string    `gorm:"size:16" json:"date_to"`
	Payload   string    `gorm:"type:text" json:"-"` // JSON insights per campaign
	CreatedAt time.Time `json:"created_at"`
}
