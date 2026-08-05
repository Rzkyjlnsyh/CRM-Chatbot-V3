package services

import (
	"log"
	"runtime/debug"
)

// Paket ini menyediakan pemulihan panic di batas goroutine. Praktik standar untuk
// server yang berjalan lama & multi-tenant: panic di satu pekerjaan tidak boleh
// menjatuhkan seluruh proses (mirip cara net/http memulihkan tiap request). Nilai
// panic + stack trace SELALU dicatat — pulihkan, jangan telan diam-diam, agar bug
// tetap bisa didiagnosa. Recover hanya untuk batas goroutine, bukan pengganti error.

// RecoverGo memulihkan panic dan mencatat nilai + stack trace. Pakai langsung
// sebagai baris pertama sebuah goroutine: `defer services.RecoverGo("nama")`.
// recover() valid di sini karena RecoverGo sendiri yang di-defer.
func RecoverGo(name string) {
	if r := recover(); r != nil {
		log.Printf("PANIC dipulihkan di %s: %v\n%s", name, r, debug.Stack())
	}
}

// Go menjalankan fn di goroutine baru yang sudah dilindungi pemulihan panic.
// Pakai untuk pekerjaan async sekali-jalan: `services.Go("nama", fn)`.
func Go(name string, fn func()) {
	go func() {
		defer RecoverGo(name)
		fn()
	}()
}

// Safe menjalankan fn secara sinkron (di goroutine pemanggil) dengan pemulihan
// panic. Pakai di dalam loop periodik supaya satu iterasi yang panic tidak
// mematikan loop selamanya: `services.Safe("nama", fn)` tiap tick.
func Safe(name string, fn func()) {
	defer RecoverGo(name)
	fn()
}
