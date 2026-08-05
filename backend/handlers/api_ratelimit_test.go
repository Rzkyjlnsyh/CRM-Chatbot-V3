package handlers

import (
	"os"
	"testing"
)

// Burst penuh lalu tertahan: dengan burst=3, tiga permintaan pertama (dalam sekejap)
// lolos, sisanya ditolak karena isi ulang ~0.
func TestAPIRateLimitBurst(t *testing.T) {
	os.Setenv("API_RATE_PER_MIN", "60")
	os.Setenv("API_RATE_BURST", "3")
	defer os.Unsetenv("API_RATE_PER_MIN")
	defer os.Unsetenv("API_RATE_BURST")

	const agentID = uint(990001) // id unik supaya tak bentrok bucket test lain
	allowed := 0
	for i := 0; i < 5; i++ {
		if ok, _ := apiRateLimitAllow(agentID); ok {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("harusnya 3 permintaan lolos (burst), dapat %d", allowed)
	}
}

// Batas <=0 berarti tanpa batas: semua lolos.
func TestAPIRateLimitDisabled(t *testing.T) {
	os.Setenv("API_RATE_PER_MIN", "0")
	defer os.Unsetenv("API_RATE_PER_MIN")

	const agentID = uint(990002)
	for i := 0; i < 100; i++ {
		if ok, _ := apiRateLimitAllow(agentID); !ok {
			t.Fatalf("dengan batas 0 (tanpa batas), permintaan ke-%d tak boleh ditolak", i)
		}
	}
}
