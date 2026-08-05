package services

import (
	"strings"
	"time"
)

const BroadcastOverridePhrase = "SAYA PAHAM RISIKONYA"

func ValidBroadcastConsentCategory(category string) bool {
	switch category {
	case "marketing", "order_update", "reminder", "service_info":
		return true
	default:
		return false
	}
}

func ValidBroadcastConsentSource(source string) bool {
	switch source {
	case "form", "checkout", "customer_request", "event", "other":
		return true
	default:
		return false
	}
}

// ValidBroadcastConsentEvidence memvalidasi pernyataan izin dari pengirim.
// Inti yang wajib: pengirim mencentang pernyataan (confirmed) + jenis pesan valid.
// Sumber & tanggal bersifat OPSIONAL (hanya untuk catatan/audit) — kalau diisi harus masuk akal:
// sumber harus dari daftar yang dikenal, tanggal tidak boleh di masa depan.
// Ini sengaja honest: ini deklarasi pengirim, bukan verifikasi sistem/WhatsApp.
func ValidBroadcastConsentEvidence(category, source string, grantedAt time.Time, confirmed bool, now time.Time) bool {
	if !confirmed || !ValidBroadcastConsentCategory(category) {
		return false
	}
	if source != "" && !ValidBroadcastConsentSource(source) {
		return false
	}
	if !grantedAt.IsZero() && grantedAt.After(now.Add(24*time.Hour)) {
		return false
	}
	return true
}

func ValidateBroadcastRiskConfirmation(level string, canProceed, acknowledged bool, phrase, reason, blockedTitle string) string {
	if !canProceed {
		return blockedTitle
	}
	if level == "medium" && !acknowledged {
		return "Baca dan setujui peringatan sebelum melanjutkan"
	}
	if level == "high" {
		if strings.TrimSpace(phrase) != BroadcastOverridePhrase {
			return "Ketik kalimat konfirmasi risiko dengan tepat"
		}
		if strings.TrimSpace(reason) == "" {
			return "Alasan melanjutkan wajib diisi"
		}
	}
	return ""
}
