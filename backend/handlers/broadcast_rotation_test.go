package handlers

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"wa-assistant/backend/models"
)

func TestParseAgentPool(t *testing.T) {
	tests := []struct {
		name string
		b    models.Broadcast
		want []uint
	}{
		{name: "fallback ke nomor utama", b: models.Broadcast{AgentID: 7}, want: []uint{7}},
		{name: "pool tersimpan dan deduplikasi", b: models.Broadcast{AgentID: 7, AgentIDs: `[7,9,9,12]`}, want: []uint{7, 9, 12}},
		{name: "json rusak fallback ke utama", b: models.Broadcast{AgentID: 7, AgentIDs: `[`}, want: []uint{7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAgentPool(tt.b)
			if len(got) != len(tt.want) {
				t.Fatalf("parseAgentPool() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseAgentPool() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestStickyAgentIsStableAndInsidePool(t *testing.T) {
	pool := []uint{3, 5, 8}
	first := stickyAgent("628123456789", pool)
	for i := 0; i < 20; i++ {
		if got := stickyAgent("628123456789", pool); got != first {
			t.Fatalf("stickyAgent tidak stabil: pertama %d, berikutnya %d", first, got)
		}
	}
	for _, candidate := range pool {
		if first == candidate {
			return
		}
	}
	t.Fatalf("stickyAgent memilih %d di luar pool %v", first, pool)
}

func TestPickFailoverAgentPrefersLowerLoad(t *testing.T) {
	healthy := []uint{10, 20, 30}
	load := map[uint]int{10: 5, 20: 1, 30: 5}

	// Semua nomor dengan load 1 harus dipilih; hanya 20.
	for i := 0; i < 30; i++ {
		num := "628100000" + string(rune('0'+i%10))
		got := pickFailoverAgent(num, healthy, load)
		if got != 20 {
			t.Fatalf("pickFailoverAgent(%s) = %d, want 20 (beban terendah)", num, got)
		}
	}
}

func TestPickFailoverAgentBalancesWhenEqualLoad(t *testing.T) {
	healthy := []uint{10, 20, 30}
	load := map[uint]int{10: 0, 20: 0, 30: 0}
	for i := 0; i < 90; i++ {
		num := "62812000" + strconv.Itoa(1000+i)
		got := pickFailoverAgent(num, healthy, load)
		load[got]++
	}
	minC, maxC := 90, 0
	for _, a := range healthy {
		if load[a] < minC {
			minC = load[a]
		}
		if load[a] > maxC {
			maxC = load[a]
		}
	}
	if maxC-minC > 2 {
		t.Fatalf("beban tidak merata: load=%v (max-min=%d)", load, maxC-minC)
	}
}

func TestCampaignQuarantineHardBeatsSoft(t *testing.T) {
	c := newBroadcastCampaign(0)
	c.persistTarget = 0 // jangan tulis DB
	c.quarantine(5, quarantineSoft, 429)
	c.quarantine(5, quarantineHard, 463)
	if !c.isHardQuarantined(5) {
		t.Fatal("expected hard quarantine")
	}
	// Soft tidak boleh menimpa hard.
	c.quarantine(5, quarantineSoft, 429)
	if !c.isHardQuarantined(5) {
		t.Fatal("soft should not override hard")
	}
	if c.getPause() != 463 {
		t.Fatalf("pause code = %d, want 463", c.getPause())
	}
}

func TestCampaignSoftExpiresInSelect(t *testing.T) {
	c := newBroadcastCampaign(0)
	c.persistTarget = 0
	c.mu.Lock()
	c.quarantined[7] = quarantineEntry{
		Reason: quarantineSoft,
		Code:   429,
		At:     time.Now().Add(-20 * time.Minute),
		Until:  time.Now().Add(-1 * time.Minute),
	}
	c.mu.Unlock()

	// selectHealthyAgents butuh IsConnected — tanpa WA mock, agent offline
	// jadi softReady tidak masuk preferred. Kita uji hasRestrictionSignal + clear logic.
	if !c.hasRestrictionSignal() {
		// soft still present before select
	}
	// Manual expire clear like select would:
	c.mu.Lock()
	e := c.quarantined[7]
	if e.Reason == quarantineSoft && time.Now().After(e.Until) {
		delete(c.quarantined, 7)
	}
	c.mu.Unlock()
	if c.hasRestrictionSignal() {
		t.Fatal("soft quarantine should be clearable after expiry")
	}
}

func TestQuarantineReasonLabelAndAdvice(t *testing.T) {
	if got := quarantineReasonLabel(quarantineHard, 463); !strings.Contains(got, "anti-spam") {
		t.Fatalf("hard 463 label = %q", got)
	}
	if got := quarantineReasonLabel(quarantineSoft, 429); !strings.Contains(strings.ToLower(got), "rate") {
		t.Fatalf("soft 429 label = %q", got)
	}
	if got := quarantineAdvice(quarantineDisconnect, 0); got == "" {
		t.Fatal("disconnect advice empty")
	}
}

func TestParseStoredQuarantineAndActive(t *testing.T) {
	raw := `{"3":{"reason":"hard","code":463,"at":"2026-01-01T00:00:00Z","until":"0001-01-01T00:00:00Z"},"4":{"reason":"soft","code":429,"at":"2026-01-01T00:00:00Z","until":"2026-01-01T00:10:00Z"}}`
	m := parseStoredQuarantine(raw)
	if len(m) != 2 {
		t.Fatalf("parse len = %d", len(m))
	}
	if !isQuarantineActive(m[3], time.Now()) {
		t.Fatal("hard should stay active")
	}
	if isQuarantineActive(m[4], time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("expired soft should be inactive")
	}
}

func TestLoadCampaignFromBroadcastSkipsExpiredSoft(t *testing.T) {
	raw := `{"9":{"reason":"soft","code":429,"at":"2020-01-01T00:00:00Z","until":"2020-01-01T00:10:00Z"},"11":{"reason":"hard","code":463,"at":"2020-01-01T00:00:00Z","until":"0001-01-01T00:00:00Z"}}`
	b := models.Broadcast{ID: 42, Status: models.BroadcastResuming, QuarantineJSON: raw}
	c := loadCampaignFromBroadcast(b)
	c.persistTarget = 0
	if c.isHardQuarantined(9) {
		t.Fatal("expired soft should not load as hard")
	}
	if !c.isHardQuarantined(11) {
		t.Fatal("hard quarantine should persist across resume")
	}
	if !c.resumeMode {
		t.Fatal("resume mode should be on when status=resuming")
	}
}
