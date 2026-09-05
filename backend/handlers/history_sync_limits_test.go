package handlers

import (
	"testing"
	"time"
)

func TestAutomaticReconciliationStaysWithinInteractiveBudget(t *testing.T) {
	if unreadBootstrapLimit > 12 {
		t.Fatalf("bootstrap unread terlalu banyak: %d", unreadBootstrapLimit)
	}
	if unreadBootstrapProbeWait > 6*time.Second {
		t.Fatalf("probe unread terlalu lama: %s", unreadBootstrapProbeWait)
	}
	if unreadBootstrapReadyWait > 30*time.Second {
		t.Fatalf("tunggu bootstrap terlalu lama: %s", unreadBootstrapReadyWait)
	}
	if automaticCatchUpLimit > 6 {
		t.Fatalf("catch-up otomatis terlalu banyak: %d", automaticCatchUpLimit)
	}
	if automaticCatchUpMessageCount > 100 {
		t.Fatalf("jendela catch-up otomatis terlalu besar: %d", automaticCatchUpMessageCount)
	}
}
