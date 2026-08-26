package database

import (
	"sort"
	"strings"
	"time"

	"wa-assistant/backend/models"
)

// builtinStageSeeds = tahap bawaan yang di-seed bila agent belum punya definisi.
// Urutan rank: unqualified < new < cold < warm < hot < customer (closing).
var builtinStageSeeds = []models.LeadStageDef{
	{Key: "unqualified", Name: "Tidak Relevan", Color: "#9E9E9E", Rank: 0,
		Description: "Salah sasaran, spam, atau tegas tidak membutuhkan layanan.", MinConfidence: 0.9},
	{Key: "new", Name: "Baru", Color: "#90A4AE", Rank: 1,
		Description: "Percakapan belum cukup untuk menilai minat, misalnya baru menyapa.", MinConfidence: 0.72},
	{Key: "cold", Name: "Dingin", Color: "#42A5F5", Rank: 2,
		Description: "Topik relevan tapi belum ada kebutuhan jelas; menunda atau cari info umum.", MinConfidence: 0.72},
	{Key: "warm", Name: "Hangat", Color: "#FFB300", Rank: 3,
		Description: "Ada kebutuhan/minat nyata: tanya produk, harga, stok, atau kecocokan.", MinConfidence: 0.72},
	{Key: "hot", Name: "Panas", Color: "#EF6C00", Rank: 4,
		Description: "Niat memproses jelas: mau beli, booking, daftar, atau mulai beri data transaksi.", MinConfidence: 0.82},
	{Key: "customer", Name: "Customer", Color: "#2E7D32", Rank: 5,
		Description: "Deal selesai/terkonfirmasi. HANYA dari aktivitas atau manual — AI tidak pernah menetapkan.", IsClosing: true, MinConfidence: 0.85},
}

// ReservedClosingKey = key tahap closing yang dilindungi (tidak bisa dihapus/diubah key-nya).
const ReservedClosingKey = "customer"

// EnsureDefaultStages men-seed definisi tahap bawaan bila agent belum punya sama sekali.
// Idempotent: bila sudah ada definisi (walau custom), tidak menimpa apa pun.
func EnsureDefaultStages(agentID uint) {
	var count int64
	if DB.Model(&models.LeadStageDef{}).Where("agent_id = ?", agentID).Count(&count); count > 0 {
		return
	}
	now := time.Now()
	for i := range builtinStageSeeds {
		def := builtinStageSeeds[i]
		def.AgentID = agentID

		def.IsDefault = true
		def.UpdatedAt = now
		DB.Create(&def)
	}
}

// GetStageDefs mengembalikan definisi tahap agent terurut rank. Bila kosong
// (agent baru / DB belum seed), kembalikan seed bawaan agar perilaku tetap deterministik.
func GetStageDefs(agentID uint) []models.LeadStageDef {
	var defs []models.LeadStageDef
	DB.Where("agent_id = ?", agentID).Order("rank asc, id asc").Find(&defs)
	if len(defs) == 0 {
		defs = builtinStageSeeds
		for i := range defs {
			defs[i].AgentID = agentID
		}
	}
	return defs
}

// GetStageDefMap = key -> def untuk lookup cepat.
func GetStageDefMap(agentID uint) map[string]models.LeadStageDef {
	defs := GetStageDefs(agentID)
	out := make(map[string]models.LeadStageDef, len(defs))
	for _, d := range defs {
		out[d.Key] = d
	}
	return out
}

// GetLeadLabelConfig mengambil (atau membuat) konfigurasi pelabelan agent.
func GetLeadLabelConfig(agentID uint) models.LeadLabelConfig {
	var cfg models.LeadLabelConfig
	if DB.Where("agent_id = ?", agentID).First(&cfg).Error == nil {
		return cfg
	}
	cfg = models.LeadLabelConfig{
		AgentID:            agentID,

		SmartLabelsEnabled: true,
	}
	DB.Create(&cfg)
	return cfg
}

// SaveLeadLabelConfig menyimpan konfigurasi pelabelan agent.
func SaveLeadLabelConfig(agentID uint, enabled bool, closingDef string) models.LeadLabelConfig {
	cfg := GetLeadLabelConfig(agentID)
	cfg.SmartLabelsEnabled = enabled
	cfg.ClosingDefinition = strings.TrimSpace(closingDef)
	cfg.UpdatedAt = time.Now()
	DB.Model(&cfg).Updates(map[string]any{
		"smart_labels_enabled": cfg.SmartLabelsEnabled,
		"closing_definition":   cfg.ClosingDefinition,
		"updated_at":           cfg.UpdatedAt,
	})
	return cfg
}

// validStageKey membatasi format key: huruf kecil, angka, underscore, maks 32.
func ValidStageKey(key string) bool {
	if key == "" || len(key) > 32 {
		return false
	}
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

// NormalizePipelineStages memvalidasi + menyimpan definisi tahap (bulk upsert).
// Menolak: key duplikat/invalid, name kosong, hapus/ubah key "customer",
// rank duplikat dirapikan otomatis.
func NormalizePipelineStages(agentID uint, incoming []models.LeadStageDef) ([]models.LeadStageDef, error) {
	now := time.Now()
	seenKey := map[string]bool{}
	cleaned := make([]models.LeadStageDef, 0, len(incoming))
	for i, in := range incoming {
		key := strings.ToLower(strings.TrimSpace(in.Key))
		name := strings.TrimSpace(in.Name)
		if !ValidStageKey(key) {
			return nil, errStageKeyInvalid(key)
		}
		if seenKey[key] {
			return nil, errStageKeyDuplicate(key)
		}
		seenKey[key] = true
		if name == "" {
			return nil, errStageNameEmpty(key)
		}
		// Key closing dilindungi: harus tetap "customer" dan tetap closing.
		if key == ReservedClosingKey && !in.IsClosing {
			return nil, errClosingMustStayClosing()
		}
		if in.Color == "" {
			in.Color = "#90A4AE"
		}
		if in.MinConfidence < 0.5 {
			in.MinConfidence = 0.5
		} else if in.MinConfidence > 0.98 {
			in.MinConfidence = 0.98
		}
		def := models.LeadStageDef{
			ID: in.ID, AgentID: agentID,
			Key: key, Name: name, Color: in.Color, Rank: i,
			Description: strings.TrimSpace(in.Description), IsClosing: in.IsClosing,
			MinConfidence: in.MinConfidence, IsDefault: in.IsDefault, UpdatedAt: now,
		}
		cleaned = append(cleaned, def)
	}

	var existing []models.LeadStageDef
	DB.Where("agent_id = ?", agentID).Find(&existing)
	existingKeys := map[string]models.LeadStageDef{}
	for _, d := range existing {
		existingKeys[d.Key] = d
	}
	// Key lama yang tidak lagi dikirim = dihapus (kecuali "customer").
	for _, d := range existing {
		if d.Key == ReservedClosingKey {
			continue
		}
		if !seenKey[d.Key] {
			DB.Delete(&models.LeadStageDef{}, d.ID)
		}
	}

	for _, def := range cleaned {
		if def.ID > 0 {
			DB.Model(&models.LeadStageDef{}).Where("id = ? AND agent_id = ?", def.ID, agentID).Updates(map[string]any{
				"key": def.Key, "name": def.Name, "color": def.Color, "rank": def.Rank,
				"description": def.Description, "is_closing": def.IsClosing,
				"min_confidence": def.MinConfidence, "is_default": def.IsDefault, "updated_at": def.UpdatedAt,
			})
		} else if old, exists := existingKeys[def.Key]; exists {
			// Upsert by key (mis. "customer" yang dilindungi) — tidak boleh CREATE duplikat.
			DB.Model(&models.LeadStageDef{}).Where("id = ? AND agent_id = ?", old.ID, agentID).Updates(map[string]any{
				"key": def.Key, "name": def.Name, "color": def.Color, "rank": def.Rank,
				"description": def.Description, "is_closing": def.IsClosing,
				"min_confidence": def.MinConfidence, "is_default": def.IsDefault, "updated_at": def.UpdatedAt,
			})
		} else {
			DB.Create(&def)
		}
	}
	// Rapikan rank sesuai urutan kiriman agar tidak ada duplikat.
	var all []models.LeadStageDef
	DB.Where("agent_id = ?", agentID).Order("rank asc, id asc").Find(&all)
	for i := range all {
		if all[i].Rank != i {
			DB.Model(&models.LeadStageDef{}).Where("id = ?", all[i].ID).Update("rank", i)
		}
	}
	return GetStageDefs(agentID), nil
}

func errStageKeyInvalid(key string) error {
	return &StageError{Msg: "key tahap tidak valid (huruf kecil/angka/underscore, maks 32): " + key}
}
func errStageKeyDuplicate(key string) error {
	return &StageError{Msg: "key tahap duplikat: " + key}
}
func errStageNameEmpty(key string) error {
	return &StageError{Msg: "nama tahap wajib diisi (key: " + key + ")"}
}
func errClosingMustStayClosing() error {
	return &StageError{Msg: "tahap 'customer' wajib tetap bertanda closing"}
}

// StageError = error validasi pipeline dengan pesan user-friendly.
type StageError struct{ Msg string }

func (e *StageError) Error() string { return e.Msg }

// StageRank dari definisi; dipakai agar perbandingan rank selalu dari config user.
func StageRankForAgent(agentID uint, key string) int {
	defs := GetStageDefMap(agentID)
	if d, ok := defs[key]; ok {
		return d.Rank
	}
	return -1
}

// SortedStageKeys untuk iterasi deterministik.
func SortedStageKeys(m map[string]models.LeadStageDef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
