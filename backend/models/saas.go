package models

import "time"

// Tenant = satu instalasi internal perusahaan. Selalu hanya ada satu (ID=1).
type Tenant struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `json:"name"` // nama perusahaan
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
