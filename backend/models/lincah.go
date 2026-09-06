package models

import "time"

// LincahConfig = kredensial & mode integrasi Lincah per agent (CS).
// Token disimpan di DB (dapat dikonfigurasi dari UI), bukan env — pola
// yang memungkinkan setiap CS memakai akun Lincah-nya sendiri.
type LincahConfig struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	AgentID   uint      `gorm:"uniqueIndex:idx_lincah_config_agent;not null" json:"agent_id"`
	PartnerID string    `gorm:"size:128" json:"partner_id"`
	Token     string    `gorm:"size:512" json:"token"`
	BaseURL   string    `gorm:"size:160;default:https://dev-api.lincah.id/openapi" json:"base_url"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LincahOrder = audit trail pesanan yang dibuat lewat Lincah dari dashboard.
// Detail mentah dari Lincah disimpan di RawJSON agar tidak kehilangan data
// tambahan saat API berevolusi (fleksibel tanpa migrasi berulang).
type LincahOrder struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	AgentID       uint      `gorm:"index:idx_lincah_order_agent_sender,priority:1;not null" json:"agent_id"`
	Sender        string    `gorm:"size:32;index:idx_lincah_order_agent_sender,priority:2" json:"sender"`
	LincahOrderID string    `gorm:"size:64;index;not null" json:"lincah_order_id"`
	NoOrder       string    `gorm:"size:64;index" json:"no_order"`
	Resi          string    `gorm:"size:64;index" json:"resi"`
	Status        string    `gorm:"size:32;index" json:"status"`
	Courier       string    `gorm:"size:32" json:"courier"`
	CourierSvc    string    `gorm:"size:64" json:"courier_service"`
	Name          string    `gorm:"size:128" json:"name"`
	Phone         string    `gorm:"size:32" json:"phone"`
	Address       string    `gorm:"type:text" json:"address"`
	Destination   string    `gorm:"size:64" json:"destination"`
	ProductName   string    `gorm:"size:128" json:"product_name"`
	ProductPrice  int64     `json:"product_price"`
	Weight        int64     `json:"weight"`
	Fee           int64     `json:"fee"`
	RawJSON       string    `gorm:"type:longtext" json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
