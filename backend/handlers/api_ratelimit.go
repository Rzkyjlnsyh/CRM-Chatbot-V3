package handlers

import (
	"math"
	"sync"
	"time"

	"wa-assistant/backend/config"
)

// Rate limiting REST API per-nomor (token bucket in-memory; server single-process).
// Knob env: API_RATE_PER_MIN (default 60, <=0 = tanpa batas) & API_RATE_BURST (default 20).
// Bucket dikunci per agentID sehingga jumlah entri terbatas (bukan per-request) — tak bocor.

type apiTokenBucket struct {
	tokens float64
	last   time.Time
}

var (
	apiRLMu      sync.Mutex
	apiRLBuckets = map[uint]*apiTokenBucket{}
)

// apiRateLimitAllow menerapkan token bucket per agent.
// Return: (boleh diteruskan, perkiraan detik tunggu bila ditolak).
func apiRateLimitAllow(agentID uint) (bool, int) {
	rate := float64(config.EnvInt("API_RATE_PER_MIN", 60)) / 60.0
	if rate <= 0 {
		return true, 0 // 0 / negatif = tanpa batas
	}
	burst := float64(config.EnvInt("API_RATE_BURST", 20))
	if burst < 1 {
		burst = 1
	}

	apiRLMu.Lock()
	defer apiRLMu.Unlock()
	now := time.Now()
	b := apiRLBuckets[agentID]
	if b == nil {
		b = &apiTokenBucket{tokens: burst, last: now}
		apiRLBuckets[agentID] = b
	}
	// Isi ulang sesuai waktu berlalu, dibatasi kapasitas burst.
	b.tokens = math.Min(burst, b.tokens+now.Sub(b.last).Seconds()*rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := int(math.Ceil((1 - b.tokens) / rate))
	if wait < 1 {
		wait = 1
	}
	return false, wait
}
