package services

import (
	"context"
	"log"
	"time"
)

// StartReconnectWatchdogCtx memantau semua sesi WA dan berhenti bersih saat context dibatalkan.
// Ini dipakai oleh main agar shutdown tidak meninggalkan goroutine ticker hidup tanpa kontrol.
func StartReconnectWatchdogCtx(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 90 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("WA reconnect watchdog berhenti")
				return
			case <-t.C:
				Safe("reconnectWatchdog", func() {
					globalMu.Lock()
					list := make([]*waInstance, 0, len(instances))
					for _, w := range instances {
						list = append(list, w)
					}
					globalMu.Unlock()
					for _, w := range list {
						w.reconnectIfNeeded()
					}
				})
			}
		}
	}()
}
