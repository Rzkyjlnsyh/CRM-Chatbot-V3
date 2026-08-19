package handlers

import "testing"

// TestParseMediaLabels memastikan pemecahan label directive SEND_MEDIA
// menangani spasi, koma ganda, dan label kosong.
func TestParseMediaLabels(t *testing.T) {
	cases := []struct {
		raw  string
		want int
	}{
		{"katalog dtf", 1},
		{"katalog dtf,video dtf", 2},
		{" katalog dtf , video dtf ", 2},
		{"a,,b", 2},
		{"", 0},
		{" , ", 0},
	}
	for _, tc := range cases {
		if got := len(parseMediaLabels(tc.raw)); got != tc.want {
			t.Errorf("parseMediaLabels(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

// TestBuildFinalText memastikan teks sebelum/sesudah directive digabung
// dengan benar dan caption fallback dipakai hanya saat tidak ada teks.
func TestBuildFinalText(t *testing.T) {
	cases := []struct {
		before, after, caption string
		want                   string
	}{
		{"Ini katalognya ya", "", "", "Ini katalognya ya"},
		{"", "Semoga cocok!", "", "Semoga cocok!"},
		{"Ini ya", "Semoga cocok!", "", "Ini ya\n\nSemoga cocok!"},
		{"", "", "Caption default", "Caption default"},
		{"", "", "", ""},
		{"Teks saja", "", "Caption fallback", "Teks saja"},
	}
	for _, tc := range cases {
		if got := buildFinalText(tc.before, tc.after, tc.caption); got != tc.want {
			t.Errorf("buildFinalText(%q,%q,%q) = %q, want %q", tc.before, tc.after, tc.caption, got, tc.want)
		}
	}
}
