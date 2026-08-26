package handlers

import (
	"testing"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	sqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// normalizeLeadStage kini berbasis definisi pipeline agent (DB). Tes ini
// men-seed tahap bawaan lalu memvalidasi perilakunya.
func TestNormalizeLeadStage(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:contacts-stage-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.LeadStageDef{}, &models.LeadLabelConfig{}); err != nil {
		t.Fatal(err)
	}
	old := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = old })
	database.EnsureDefaultStages(1)

	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"", leadStageNew, true},
		{" HOT ", leadStageHot, true},
		{"customer", leadStageCustomer, true},
		{"unqualified", leadStageUnqualified, true},
		{"prospek", "", false},
	}
	for _, test := range tests {
		got, ok := normalizeLeadStage(1, test.input)
		if got != test.want || ok != test.ok {
			t.Fatalf("normalizeLeadStage(%q) = (%q, %v), ingin (%q, %v)", test.input, got, ok, test.want, test.ok)
		}
	}
}

func TestNormalizeTagsRemovesCaseInsensitiveDuplicates(t *testing.T) {
	if got := normalizeTags(" VIP, reseller, vip, , Reseller "); got != "VIP,reseller" {
		t.Fatalf("normalizeTags tidak sesuai: %q", got)
	}
}
