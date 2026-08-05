package handlers

import (
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
)

// handoffSoftTimeout: tanpa balasan manusia dalam durasi ini, AI full resume
// (customer tidak dibiarkan orphan). Internal: tetap antrian Butuh CS di dashboard
// sampai resume manual ATAU timeout.
const handoffSoftTimeout = 2 * time.Hour

// humanFacingHoldReply = penutup ke pelanggan saat butuh penanganan khusus.
// Satu persona CS — tidak menyebut petugas/admin/AI/bot/diteruskan.
const humanFacingHoldReply = "Baik kak, saya cek dulu ya biar informasinya tepat. Mohon ditunggu sebentar 🙏"

// humanFacingSoftSensitiveReply = topik sensitif saat masih nunggu cek internal.
const humanFacingSoftSensitiveReply = "Untuk ini saya pastikan dulu biar tidak salah ya kak. Mohon ditunggu sebentar 🙏"

// humanFacingSoftResumeHint = disuntik ke prompt sekali saat AI kembali full setelah timeout.
const humanFacingSoftResumeHint = `
MODE LANJUT LAYANAN:
- Kamu CS yang sama; pelanggan sempat menunggu. Lanjutkan natural tanpa minta maaf berlebihan.
- Jangan sebut petugas, admin, AI, bot, sistem, atau "diteruskan".
- Jawab dari knowledge; jangan mengarang.
`

// softHandoffPrompt = AI tetap melayani sebagai CS yang sama sambil flag Butuh CS internal aktif.
const softHandoffPrompt = `
MODE TUNGGU KONFIRMASI INTERNAL (pelanggan TIDAK boleh tahu ada antrean/bot):
- Kamu tetap staf CS yang sedang melayani di chat ini — satu orang, natural.
- Boleh jawab sapaan, basa-basi, dan pertanyaan info umum HANYA dari basis pengetahuan/produk.
- Untuk refund, komplain berat, salah transfer, penipuan, data pribadi, atau keputusan penting: bilang kamu cek/pastikan dulu sebentar. Jangan putuskan final.
- JANGAN pernah bilang: diteruskan ke petugas/admin/CS lain, hubungi nomor lain, AI, bot, sistem, database, knowledge.
- Jangan tutup dengan "ada yang ditanyakan lagi?" generik.
`

type handoffState struct {
	active     bool // masih dalam periode soft/hard handoff
	softAI     bool // AI boleh jawab mode soft
	fullResume bool // timeout lewat → hapus handoff, AI full
	prompt     string
}

// resolveHandoffState mengevaluasi antrian Butuh CS untuk satu kontak.
// - Manusia sudah balas sejak handoff → AI diam (hormati CS).
// - Belum ada balasan manusia & < timeout → soft AI.
// - Lewat timeout → hapus handoff, AI full (+ hint resume di prompt).
func resolveHandoffState(agentID uint, sender string) handoffState {
	var ho models.Handoff
	if database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).First(&ho).Error != nil {
		return handoffState{}
	}

	// CS (manusia) sudah ikut chat sejak handoff dibuat → bot diam.
	var humanReplies int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ? AND from_human = ? AND created_at >= ?", agentID, sender, true, ho.CreatedAt).
		Count(&humanReplies)
	if humanReplies > 0 {
		return handoffState{active: true, softAI: false}
	}

	// Timeout: customer tidak boleh orphan.
	if time.Since(ho.CreatedAt) >= handoffSoftTimeout {
		_ = database.DB.Delete(&ho).Error
		return handoffState{fullResume: true, prompt: humanFacingSoftResumeHint}
	}

	return handoffState{active: true, softAI: true, prompt: softHandoffPrompt}
}

func ensureHandoff(agentID uint, sender, lastMsg string) {
	_ = database.DB.Where(models.Handoff{AgentID: agentID, Sender: sender}).
		Assign(models.Handoff{LastMsg: lastMsg}).
		FirstOrCreate(&models.Handoff{}).Error
}

// isSoftHandoffSensitive: topik yang tetap "saya cek dulu" saat soft mode.
func isSoftHandoffSensitive(message string) bool {
	return shouldAllowHumanHandoff(message)
}

// stripInternalStaffSpeak: pagar terakhir di balasan yang terlihat pelanggan.
func stripInternalStaffSpeak(reply string) string {
	repl := []struct{ old, new string }{
		{"petugas", "saya"},
		{"Petugas", "Saya"},
		{"diteruskan ke CS", "saya bantu pastikan"},
		{"diteruskan ke cs", "saya bantu pastikan"},
		{"teruskan ke CS", "bantu pastikan"},
		{"teruskan ke cs", "bantu pastikan"},
		{"hubungi admin", "saya bantu"},
		{"Hubungi admin", "Saya bantu"},
		{"tim CS", "saya"},
		{"tim cs", "saya"},
	}
	out := reply
	for _, r := range repl {
		out = strings.ReplaceAll(out, r.old, r.new)
	}
	return out
}
