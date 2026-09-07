package handlers

import (
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// StartMediaCleanup menghapus file media lama secara berkala agar disk VPS tidak penuh.
func StartMediaCleanup(retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	run := func() { safeRun("cleanupMedia", func() { cleanupMedia(retentionDays) }) }
	go func() {
		run()
		t := time.NewTicker(24 * time.Hour)
		for range t.C {
			run()
		}
	}()
}

// CleanupBroadcastJunk menghapus thread sistem (@broadcast/@newsletter) yang bocor ke
// chat_histories — dipanggil sekali saat startup (pola v4; diport ke fork klien).
func CleanupBroadcastJunk() {
	affected := database.DB.Where("sender LIKE ? OR sender LIKE ?", "%@broadcast", "%@newsletter").
		Delete(&models.ChatHistory{}).RowsAffected
	if affected > 0 {
		log.Printf("[cleanup] %d baris thread broadcast/newsletter dihapus", affected)
	}
}

func cleanupMedia(days int) {
	cutoff := time.Now().AddDate(0, 0, -days)
	removed := 0
	filepath.WalkDir("data/media", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil && info.ModTime().Before(cutoff) {
			if os.Remove(path) == nil {
				removed++
			}
		}
		return nil
	})
	if removed > 0 {
		log.Printf("Media cleanup: %d file lama dihapus (> %d hari)", removed, days)
	}
}
