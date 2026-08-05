package handlers

import "log"

// logIfErr mencatat error ke log jika tidak nil. Dipakai untuk operasi best-effort
// yang tidak bisa gagal secara fatal (cleanup, update status, dll).
func logIfErr(err error, msg string) {
	if err != nil {
		log.Printf("WARN: %s: %v", msg, err)
	}
}

// ignoreErr sengaja membuang error — hanya untuk operasi yang benar-benar
// tidak perlu diketahui hasilnya (contoh: delete resource yang sudah tak ada).
// Hindari pemakaian di jalur kritis tempat error bisa menyebabkan korupsi data.
//
// Deprecated: gunakan logIfErr agar error setidaknya tercatat di log.
func ignoreErr(_ error) {}
