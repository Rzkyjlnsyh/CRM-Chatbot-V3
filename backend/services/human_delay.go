package services

// Simulasi jeda manusia sebelum membalas — biar balasan AI tidak terasa
// instan seperti bot. Jeda = 900ms dasar + 35ms per karakter (maks 4,2s),
// ±25% acak. Dinonaktifkan lewat AppSetting human_delay="0".

import (
	"math/rand"
	"time"

	"wa-assistant/backend/database"
)

const (
	humanDelayBasePerChar = 35 * time.Millisecond
	humanDelayBaseFixed   = 900 * time.Millisecond
	humanDelayMax         = 4200 * time.Millisecond
)

// HumanReplyDelay menunggu sejenak sebelum balasan dikirim (meniru waktu
// membaca + mengetik manusia). Aman dipanggil berkali-kali; pemanggil
// biasanya membatasi sekali per pesan.
func HumanReplyDelay(textLength int) {
	if database.GetAppSetting("human_delay", "1") == "0" {
		return
	}
	if textLength <= 0 {
		return
	}
	base := time.Duration(textLength) * humanDelayBasePerChar
	if base > humanDelayMax {
		base = humanDelayMax
	}
	if base < humanDelayBaseFixed {
		base = humanDelayBaseFixed
	}
	jitter := 1.0 + (rand.Float64()-0.5)*0.5 // ±25%
	time.Sleep(time.Duration(float64(base) * jitter))
}
