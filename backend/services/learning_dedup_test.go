package services

import (
	"testing"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupLearningServiceTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:learning-dedup-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.LearningPattern{}, &models.LearningConfig{}, &models.LearningRun{}); err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	return db
}

// TestPatternDedupKeyNormalization membuktikan variasi penulisan AI (kapital,
// spasi, tanda baca ujung) dianggap POLA SAMA — mencegah baris dobel.
func TestPatternDedupKeyNormalization(t *testing.T) {
	base := patternDedupKey("customer minta harga", "Boleh, minyaknya Rp75.000 ya kak")
	cases := map[string][2]string{
		"kapital beda":     {"CUSTOMER MINTA HARGA", "Boleh, minyaknya rp75.000 ya kak"},
		"spasi dobel":      {"customer  minta   harga", "Boleh,  minyaknya Rp75.000 ya  kak"},
		"tanda baca ujung": {"customer minta harga!", "Boleh, minyaknya Rp75.000 ya kak..."},
		"ellipsis & emoji": {"customer minta harga", "Boleh, minyaknya Rp75.000 ya kak 🙏..."},
	}
	for name, pair := range cases {
		if patternDedupKey(pair[0], pair[1]) != base {
			t.Fatalf("%s: kunci harus sama dengan basis", name)
		}
	}
	beda := patternDedupKey("customer komplain pengiriman", "Mohon maaf kak, saya cek dulu ya")
	if beda == base {
		t.Fatal("pola beda harus punya kunci beda")
	}
}

// TestSavePatternsSkipsNormalizedDuplicates membuktikan savePatterns TIDAK
// menambah baris baru bila pola identik/varian sudah ada di suggested.
func TestSavePatternsSkipsNormalizedDuplicates(t *testing.T) {
	db := setupLearningServiceTest(t)
	agentID := uint(901)

	existing := loadSuggestedDedupKeys(agentID)
	pat := func(trigger, template string) ExtractedPattern {
		return ExtractedPattern{PatternType: "phrase", TriggerContext: trigger, ResponseTemplate: template, Confidence: 0.8}
	}
	savePatterns(1, agentID, "", "", "human", []ExtractedPattern{
		pat("customer minta harga", "Boleh, minyaknya Rp75.000 ya kak"),
	}, existing)

	var n int64
	db.Model(&models.LearningPattern{}).Where("agent_id = ? AND status = ?", agentID, "suggested").Count(&n)
	if n != 1 {
		t.Fatalf("harus 1 pola tersimpan, dapat %d", n)
	}

	// Run berikutnya (window tumpang tindih): varian penulisan → TIDAK menumpuk.
	existing2 := loadSuggestedDedupKeys(agentID)
	savePatterns(2, agentID, "", "", "human", []ExtractedPattern{
		pat("CUSTOMER MINTA HARGA", "Boleh, minyaknya Rp75.000 ya kak..."),
	}, existing2)
	db.Model(&models.LearningPattern{}).Where("agent_id = ? AND status = ?", agentID, "suggested").Count(&n)
	if n != 1 {
		t.Fatalf("varian harus didedup, dapat %d", n)
	}

	// Pola baru → masuk.
	existing3 := loadSuggestedDedupKeys(agentID)
	savePatterns(2, agentID, "", "", "human", []ExtractedPattern{
		pat("customer tanya stok", "Stok masih ada kak, siap kirim hari ini"),
	}, existing3)
	db.Model(&models.LearningPattern{}).Where("agent_id = ? AND status = ?", agentID, "suggested").Count(&n)
	if n != 2 {
		t.Fatalf("pola baru harus masuk, dapat %d", n)
	}

	// Dedup tidak menutup status lain: applied boleh punya pola serupa (beda status).
	existing4 := loadSuggestedDedupKeys(agentID)
	savePatterns(3, agentID, "", "", "human", []ExtractedPattern{
		pat("customer tanya stok", "Stok masih ada kak, siap kirim hari ini"),
	}, existing4)
	db.Model(&models.LearningPattern{}).Where("agent_id = ? AND status = ?", agentID, "suggested").Count(&n)
	if n != 2 {
		t.Fatalf("duplikat suggested harus tetap didedup, dapat %d", n)
	}
}

// TestRealtimeWindowStart membuktikan window realtime = chat baru sejak
// kursor; kursor lama/nil/future → clamp 24 jam.
func TestRealtimeWindowStart(t *testing.T) {
	now := time.Date(2026, 8, 28, 15, 0, 0, 0, time.Local)
	d24 := now.Add(-24 * time.Hour)

	// Kursor 10 menit lalu → mulai dari kursor.
	cur := now.Add(-10 * time.Minute)
	if got := realtimeWindowStart(models.LearningConfig{LastRealtimeRunAt: &cur}, now); !got.Equal(cur) {
		t.Fatalf("kursor fresh harus dipakai: got %v", got)
	}
	// Kursor 3 hari lalu → clamp 24 jam.
	old := now.Add(-72 * time.Hour)
	if got := realtimeWindowStart(models.LearningConfig{LastRealtimeRunAt: &old}, now); !got.Equal(d24) {
		t.Fatalf("kursor lama harus clamp 24 jam: got %v", got)
	}
	// Tanpa kursor → 24 jam.
	if got := realtimeWindowStart(models.LearningConfig{}, now); !got.Equal(d24) {
		t.Fatalf("tanpa kursor harus 24 jam: got %v", got)
	}
	// Kursor di masa depan (jam server mundur) → 24 jam.
	fut := now.Add(1 * time.Hour)
	if got := realtimeWindowStart(models.LearningConfig{LastRealtimeRunAt: &fut}, now); !got.Equal(d24) {
		t.Fatalf("kursor masa depan harus clamp 24 jam: got %v", got)
	}
}
