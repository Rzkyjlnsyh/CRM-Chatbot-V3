package models

import "time"

// MediaAsset adalah file media yang bisa dikirim otomatis oleh AI via directive [[SEND_MEDIA:id]].
// Tabel: media_assets
type MediaAsset struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	AgentID     uint      `gorm:"index" json:"agent_id"`
	TenantID    uint      `gorm:"index;default:1" json:"tenant_id"`
	Name        string    `gorm:"size:200" json:"name"`         // nama untuk referensi
	FileName    string    `gorm:"size:200" json:"file_name"`    // nama file asli
	MediaType   string    `gorm:"size:20" json:"media_type"`    // image, video, document
	MimeType    string    `gorm:"size:100" json:"mime_type"`
	FilePath    string    `gorm:"size:500" json:"file_path"`    // path file di disk
	Caption     string    `gorm:"type:text" json:"caption"`     // caption default
	FileSize    int64     `json:"file_size"`
	TriggerKeys string    `gorm:"size:500" json:"trigger_keys"` // kata kunci pemicu (pisah koma)
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (MediaAsset) TableName() string {
	return "media_assets"
}
