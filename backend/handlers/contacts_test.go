package handlers

import "testing"

func TestNormalizeLeadStage(t *testing.T) {
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
		got, ok := normalizeLeadStage(test.input)
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
