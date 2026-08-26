package database

import (
	"testing"

	"wa-assistant/backend/models"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func pipelineDBTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.LeadStageDef{}, &models.LeadLabelConfig{}, &models.LabelRule{}); err != nil {
		t.Fatal(err)
	}
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
}

func TestEnsureDefaultStagesSeedsBuiltinsOnce(t *testing.T) {
	pipelineDBTest(t)
	EnsureDefaultStages(7)
	defs := GetStageDefs(7)
	if len(defs) != 6 {
		t.Fatalf("harus ada 6 tahap bawaan, dapat %d", len(defs))
	}
	if defs[0].Key != "unqualified" || defs[0].Rank != 0 {
		t.Fatalf("urutan seed salah: %+v", defs[0])
	}
	var closing *models.LeadStageDef
	for i := range defs {
		if defs[i].Key == "customer" {
			closing = &defs[i]
		}
	}
	if closing == nil || !closing.IsClosing {
		t.Fatal("tahap customer harus bertanda closing")
	}
	// Idempotent: dipanggil lagi tidak menambah baris.
	EnsureDefaultStages(7)
	if len(GetStageDefs(7)) != 6 {
		t.Fatal("seed harus idempotent")
	}
}

func TestNormalizePipelineStagesRejectsInvalid(t *testing.T) {
	pipelineDBTest(t)
	EnsureDefaultStages(1)

	// Key dengan spasi/karakter aneh → tolak.
	if _, err := NormalizePipelineStages(1, []models.LeadStageDef{{Key: "Hot Stage", Name: "X"}}); err == nil {
		t.Fatal("key dengan spasi harus ditolak")
	}
	// Duplikat key → tolak.
	if _, err := NormalizePipelineStages(1, []models.LeadStageDef{
		{Key: "hot", Name: "Panas"}, {Key: "hot", Name: "Panas 2"},
	}); err == nil {
		t.Fatal("key duplikat harus ditolak")
	}
	// Nama kosong → tolak.
	if _, err := NormalizePipelineStages(1, []models.LeadStageDef{{Key: "hot", Name: "  "}}); err == nil {
		t.Fatal("nama kosong harus ditolak")
	}
	// customer tanpa closing → tolak.
	if _, err := NormalizePipelineStages(1, []models.LeadStageDef{
		{Key: "new", Name: "Baru"}, {Key: "customer", Name: "Customer", IsClosing: false},
	}); err == nil {
		t.Fatal("customer wajib tetap closing")
	}
}

func TestNormalizePipelineStagesKeepsCustomAndCustomer(t *testing.T) {
	pipelineDBTest(t)
	EnsureDefaultStages(2)

	stages, err := NormalizePipelineStages(2, []models.LeadStageDef{
		{Key: "new", Name: "Baru", Rank: 0, MinConfidence: 0.7},
		{Key: "warm", Name: "Hangat", Rank: 1, MinConfidence: 0.7},
		{Key: "hot", Name: "Panas", Rank: 2, MinConfidence: 0.85},
		{Key: "deal", Name: "Deal Transfer", Rank: 3, MinConfidence: 0.9}, // custom
		{Key: "customer", Name: "Customer", Rank: 4, IsClosing: true},
	})
	if err != nil {
		t.Fatalf("simpan tahap valid gagal: %v", err)
	}
	if len(stages) != 5 {
		t.Fatalf("harus tersisa 5 tahap (cold/unqualified terhapus), dapat %d", len(stages))
	}
	keys := map[string]bool{}
	for _, s := range stages {
		keys[s.Key] = true
	}
	for _, want := range []string{"new", "warm", "hot", "deal", "customer"} {
		if !keys[want] {
			t.Fatalf("tahap %s hilang: %+v", want, stages)
		}
	}
	// Cold + unqualified (tidak dikirim) harus terhapus.
	for _, gone := range []string{"cold", "unqualified"} {
		if keys[gone] {
			t.Fatalf("tahap %s seharusnya terhapus karena tidak dikirim ulang", gone)
		}
	}
}

func TestNormalizePipelineStagesProtectsCustomerDeletion(t *testing.T) {
	pipelineDBTest(t)
	EnsureDefaultStages(3)
	// Kirim daftar TANPA customer → customer harus tetap ada (dilindungi).
	stages, err := NormalizePipelineStages(3, []models.LeadStageDef{
		{Key: "new", Name: "Baru"}, {Key: "hot", Name: "Panas"},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range stages {
		if s.Key == "customer" {
			found = true
		}
	}
	if !found {
		t.Fatal("customer tidak boleh terhapus walau tidak dikirim")
	}
}

func TestGetLeadLabelConfigCreatesDefault(t *testing.T) {
	pipelineDBTest(t)
	cfg := GetLeadLabelConfig(9)
	if !cfg.SmartLabelsEnabled {
		t.Fatal("default smart_labels_enabled harus true")
	}
	saved := SaveLeadLabelConfig(9, false, "Closing = transfer DP diterima")
	if saved.SmartLabelsEnabled || saved.ClosingDefinition != "Closing = transfer DP diterima" {
		t.Fatalf("config tidak tersimpan: %+v", saved)
	}
	again := GetLeadLabelConfig(9)
	if again.SmartLabelsEnabled || again.ClosingDefinition == "" {
		t.Fatal("config harus persisten")
	}
}
