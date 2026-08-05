package license

import (
	"context"
	"log"
	"math/rand"
	"time"
)

// StartHeartbeat runs until ctx is cancelled or the license becomes invalid.
// onInvalid should initiate the application's graceful shutdown.
func StartHeartbeat(ctx context.Context, minInterval, maxInterval time.Duration, onInvalid func(string)) {
	if minInterval <= 0 {
		minInterval = 6 * time.Hour
	}
	if maxInterval <= minInterval {
		maxInterval = minInterval * 2
	}

	go func() {
		if !waitForHeartbeat(ctx, time.Duration(rand.Intn(60))*time.Second) {
			return
		}

		for {
			interval := nextHeartbeatInterval(minInterval, maxInterval)
			if !waitForHeartbeat(ctx, interval) {
				return
			}

			if !Heartbeat() {
				stateMu.RLock()
				message := VerifyMessage
				status := VerifyStatus
				stateMu.RUnlock()
				log.Printf("[license] Lisensi tidak valid status=%s — shutdown: %s", status, message)
				if onInvalid != nil {
					onInvalid(message)
				}
				return
			}
		}
	}()
}

func waitForHeartbeat(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextHeartbeatInterval(minInterval, maxInterval time.Duration) time.Duration {
	interval := randomInterval(minInterval, maxInterval)
	stateMu.RLock()
	lastOK := lastHeartbeatOK
	lastSuccess := lastHeartbeatAt
	stateMu.RUnlock()
	if lastOK || lastSuccess.IsZero() {
		return interval
	}
	remaining := time.Until(lastSuccess.Add(offlineGraceDuration()))
	if remaining <= 0 {
		return time.Millisecond
	}
	if remaining < interval {
		return remaining
	}
	return interval
}

func randomInterval(min, max time.Duration) time.Duration {
	if min >= max {
		return min
	}
	delta := max - min
	return min + time.Duration(rand.Int63n(int64(delta)))
}
