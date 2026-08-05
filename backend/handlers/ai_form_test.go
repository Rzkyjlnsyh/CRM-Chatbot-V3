package handlers

import "testing"

func TestOneEditApart(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{a: "sedkah", b: "sedekah", want: true},
		{a: "booking", b: "boking", want: true},
		{a: "donasi", b: "donatur", want: false},
		{a: "saya", b: "sapa", want: false}, // kata pendek sengaja tidak difuzzy-kan
		{a: "sedekah", b: "sedekah", want: false},
	}
	for _, tt := range tests {
		if got := oneEditApart(tt.a, tt.b); got != tt.want {
			t.Fatalf("oneEditApart(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestAIFormOverlapScoreToleratesOneCharacterTypo(t *testing.T) {
	if got := aiFormOverlapScore([]string{"sedkah"}, []string{"sedekah"}, 3); got != 3 {
		t.Fatalf("score typo = %d, want 3", got)
	}
}

func TestReplyRequestsStructuredDataDetectsFreeTextForm(t *testing.T) {
	// Pola yang muncul di production: AI mengarang daftar field alih-alih [[START_FORM:ID]].
	reply := "Baik kak, silakan isi form pemesanan berikut ya:\n\nNama lengkap:\nAlamat lengkap:\nNo. HP:\nJumlah kaos:"
	if !replyRequestsStructuredData(reply) {
		t.Fatal("daftar field free-text harus terdeteksi sebagai permintaan data terstruktur")
	}
	if replyRequestsStructuredData("Harga kaosnya Rp65.250 kak") {
		t.Fatal("jawaban harga biasa tidak boleh dianggap form free-text")
	}
}

func TestStartFormDirectiveRegex(t *testing.T) {
	cases := []string{
		"[[START_FORM:2]]",
		"Baik kak [[START_FORM: 2 ]]",
		"[[start_form:2]]",
	}
	for _, c := range cases {
		if !startFormDirectiveRe.MatchString(c) {
			t.Fatalf("harus match START_FORM: %q", c)
		}
	}
	if startFormDirectiveRe.MatchString("halo kak harga berapa") {
		t.Fatal("teks biasa tidak boleh match START_FORM")
	}
}
