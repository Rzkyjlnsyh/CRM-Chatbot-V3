package handlers

import "testing"

func TestWhatsAppAccountChanged(t *testing.T) {
	tests := []struct {
		name     string
		previous string
		current  string
		want     bool
	}{
		{name: "same international", previous: "628123456789", current: "628123456789", want: false},
		{name: "same local format", previous: "08123456789", current: "628123456789", want: false},
		{name: "different account", previous: "628123456789", current: "628987654321", want: true},
		{name: "first link", previous: "", current: "628123456789", want: false},
		{name: "missing new number", previous: "628123456789", current: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := whatsappAccountChanged(tc.previous, tc.current); got != tc.want {
				t.Fatalf("whatsappAccountChanged(%q, %q) = %v, want %v", tc.previous, tc.current, got, tc.want)
			}
		})
	}
}
