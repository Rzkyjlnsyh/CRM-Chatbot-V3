package database

import "wa-assistant/backend/models"

// GetAppSetting membaca setting platform; mengembalikan def bila kosong/tak ada.
func GetAppSetting(key, def string) string {
	var s models.AppSetting
	if DB.First(&s, "`key` = ?", key).Error != nil || s.Value == "" {
		return def
	}
	return s.Value
}

// SetAppSetting menyimpan (upsert) setting platform.
func SetAppSetting(key, value string) {
	DB.Save(&models.AppSetting{Key: key, Value: value})
}
