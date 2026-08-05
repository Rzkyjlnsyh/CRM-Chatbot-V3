package handlers

import (
	"encoding/json"
	"strings"
	"testing"

	"wa-assistant/backend/models"
)

func TestVisionFieldInstructionConstrainsSelectAnswer(t *testing.T) {
	got := visionFieldInstruction("checkout Kaos", "Pilih ukuran?", "select", []string{"S", "M", "L"})
	for _, expected := range []string{"checkout Kaos", "Pilih ukuran?", "persis salah satu pilihan", "S, M, L"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("instruksi vision tidak memuat %q: %s", expected, got)
		}
	}
}

func TestAIFormSummaryMarksAttachedImage(t *testing.T) {
	form := models.AIForm{Name: "Penjemputan"}
	steps := []aiFormStepConfig{{Key: "item", Label: "Barang apa?", Type: "text", Required: true}}
	got := aiFormSummary(form, steps, `{"item":"Sofa abu-abu","_media_item":"3EB0PHOTO"}`)
	if !strings.Contains(got, "Sofa abu-abu (foto terlampir)") {
		t.Fatalf("referensi foto tidak terlihat di ringkasan form: %s", got)
	}
}

func TestCheckoutSummaryMarksAttachedImage(t *testing.T) {
	product := models.Product{Name: "Cetak Desain"}
	steps := []checkoutStepConfig{{Key: "design", Label: "Kirim desain?", Type: "text", Required: true}}
	got := checkoutSummary(product, steps, `{"design":"Logo biru","_media_design":"3EB0PHOTO"}`)
	if !strings.Contains(got, "Logo biru (foto terlampir)") {
		t.Fatalf("referensi foto tidak terlihat di ringkasan checkout: %s", got)
	}
}

func TestFreeFormClosingIsRecognizedAsOfficialFormCollection(t *testing.T) {
	steps, _ := json.Marshal([]aiFormStepConfig{
		{Key: "name", Label: "Boleh dibantu nama lengkapnya?", Type: "text", Required: true},
		{Key: "address", Label: "Lokasi penjemputan atau alamat lengkapnya?", Type: "text", Required: true},
		{Key: "contact", Label: "Kontak yang bisa dihubungi?", Type: "text", Required: true},
		{Key: "schedule", Label: "Jam yang cocok untuk penjemputan?", Type: "text", Required: true},
	})
	form := models.AIForm{Name: "Penjemputan barang", Goal: "Mencatat sedekah barang", StepsJSON: string(steps)}
	reply := "Silakan beri tahu:\n1. Lokasi penjemputan\n2. Kontak yang bisa dihubungi\n3. Jam yang cocok"
	conversation := "Saya mau sedekah kipas. Kipas masih berfungsi baik kak."
	if !replyRequestsStructuredData(reply) || !replyOverlapsAIFormFields(form, reply) {
		t.Fatalf("pengumpulan data bebas tidak terdeteksi: %s", reply)
	}
	if score := aiFormIntentScore(form, conversation+"\n"+reply, aiFormTokens(conversation+"\n"+reply)); score < 5 {
		t.Fatalf("form penjemputan tidak cukup relevan, skor=%d", score)
	}
}

func TestGenericGreetingNeverMeansFormIntent(t *testing.T) {
	for _, message := range []string{"Hallo kak", "halo", "Hai min", "Selamat pagi", "Assalamualaikum admin"} {
		if !isGenericGreetingMessage(message) {
			t.Fatalf("sapaan umum tidak dikenali: %q", message)
		}
	}
	for _, message := range []string{"Halo kak saya mau sedekah", "Pagi, kipasnya berfungsi baik", "Saya mau booking"} {
		if isGenericGreetingMessage(message) {
			t.Fatalf("pesan berisi intent tidak boleh dianggap sapaan kosong: %q", message)
		}
	}
}

func TestEditFormDirectiveAndNaturalEditIntent(t *testing.T) {
	id, found, valid := parseEditAIFormDirective("[[EDIT_FORM:7]]")
	if id != 7 || !found || !valid {
		t.Fatalf("directive edit form tidak terbaca: id=%d found=%v valid=%v", id, found, valid)
	}
	for _, message := range []string{"alamatnya mau saya ganti", "jadwal kemarin salah", "boleh revisi data?"} {
		if !messageHasEditIntent(message) {
			t.Fatalf("intent edit tidak dikenali: %q", message)
		}
	}
	if messageHasEditIntent("saya mau tanya jadwal") {
		t.Fatal("pertanyaan informasi dianggap intent edit")
	}
}

func TestSingleClarificationDoesNotForceForm(t *testing.T) {
	if replyRequestsStructuredData("Apakah barangnya masih berfungsi dengan baik?") {
		t.Fatal("satu klarifikasi natural tidak boleh langsung memaksa form")
	}
	if !replyRequestsStructuredData("Silakan beri tahu lokasi, kontak, dan jadwal penjemputan.") {
		t.Fatal("pengumpulan data operasional bebas harus dialihkan ke mesin form")
	}
}

func TestAIFormConfirmationUsesFriendlyActions(t *testing.T) {
	form := models.AIForm{Name: "Penjemputan"}
	steps := []aiFormStepConfig{{Key: "name", Label: "Nama?", Type: "text", Required: true}}
	result := aiFormConfirmation(models.AIFormSession{ID: 4, DataJSON: `{"name":"Ega"}`}, form, steps)
	if len(result.buttons) != 3 || result.buttons[0].Text != "Data sudah benar" || result.buttons[1].Text != "Ubah data" {
		t.Fatalf("tombol konfirmasi form tidak ramah: %#v", result.buttons)
	}
}

func TestShortReplyUsesLastAssistantQuestion(t *testing.T) {
	history := []models.ChatHistory{{Reply: "Apakah kipasnya masih berfungsi dengan baik kak?"}}
	for _, reply := range []string{"Masih kak", "Iya", "Itu aja kak", "Belum"} {
		if !isShortReplyToAssistantQuestion(reply, history) {
			t.Fatalf("jawaban singkat tidak dikaitkan ke pertanyaan terakhir: %q", reply)
		}
	}
	if isShortReplyToAssistantQuestion("Saya ingin menjelaskan kebutuhan lain yang cukup panjang", history) {
		t.Fatal("pesan panjang tidak boleh dipaksa sebagai jawaban singkat")
	}
}

func TestHumanHandoffRequiresClearSignal(t *testing.T) {
	for _, message := range []string{"masih kak", "berapa harganya?", "saya mau tanya produk", "informasinya belum jelas", "saya mau tanya jam admin"} {
		if shouldAllowHumanHandoff(message) {
			t.Fatalf("percakapan biasa tidak boleh masuk Butuh CS: %q", message)
		}
	}
	for _, message := range []string{
		"tolong hubungkan ke CS",
		"saya mau bicara dengan petugas",
		"saya salah transfer",
		"mau ajukan refund",
		"tolong alihkan ke customer service",
		"saya mau ngomong sama orang",
	} {
		if !shouldAllowHumanHandoff(message) {
			t.Fatalf("sinyal handoff nyata tidak dikenali: %q", message)
		}
	}
}

func TestApplyEscalationPolicyNoopWhenNotEscalating(t *testing.T) {
	reply, escalate, turnError := applyEscalationPolicy(1, "persona", "ramah", "berapa harganya?", nil, "Jawaban normal", false, "")
	if escalate {
		t.Fatal("escalate=false tidak boleh diubah jadi true")
	}
	if reply != "Jawaban normal" || turnError != "" {
		t.Fatalf("policy noop gagal: reply=%q turnError=%q", reply, turnError)
	}
}

func TestApplyEscalationPolicyKeepsClearHumanRequest(t *testing.T) {
	// Jalur ini tidak memanggil API/DB — hanya mengecek sinyal handoff.
	reply, escalate, turnError := applyEscalationPolicy(1, "persona", "ramah", "tolong hubungkan ke CS", nil, "x", true, "")
	if !escalate {
		t.Fatalf("permintaan CS jelas harus tetap escalate, turnError=%q reply=%q", turnError, reply)
	}
}
