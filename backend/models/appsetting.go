package models

// AppSetting = setting platform global (key-value), diatur super-admin.
// Mis. key "ai_preset" -> model AI aktif untuk seluruh tenant.
type AppSetting struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text" json:"value"`
}
