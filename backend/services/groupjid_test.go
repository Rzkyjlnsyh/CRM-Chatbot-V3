package services

import "testing"

func TestIsGroupJID(t *testing.T) {
	cases := map[string]bool{
		"120363012345678901@g.us":  true,
		"628123-1600000000@g.us":   true,
		"628123456789@s.whatsapp.net": false,
		"628123456789":             false,
		"":                         false,
		"@g.us":                    true, // suffix cocok; validasi format lanjut ada di whatsmeow ParseJID
	}
	for in, want := range cases {
		if got := IsGroupJID(in); got != want {
			t.Errorf("IsGroupJID(%q) = %v, mau %v", in, got, want)
		}
	}
}
