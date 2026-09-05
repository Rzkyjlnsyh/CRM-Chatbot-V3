package database

import (
	"fmt"
	"testing"
)

func looksLikeLIDLocal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	if len(s) >= 15 && len(s) <= 17 && (s[0] == '6' && s[1] == '2') {
		return true
	}
	if s[0] == '6' && s[1] == '2' {
		return false
	}
	return len(s) >= 13 && len(s) <= 17
}

// TestScratchVerifyLID — VERIFIKASI SEMENTARA: apakah junk LID sudah bersih?
func TestScratchVerifyLID(t *testing.T) {
	Init()
	fmt.Println("== inbox_read_states: sender mirip LID ==")
	var irs []struct{ Sender string }
	DB.Raw(`SELECT sender FROM inbox_read_states GROUP BY sender`).Scan(&irs)
	junk := 0
	for _, r := range irs {
		if looksLikeLIDLocal(r.Sender) {
			junk++
			fmt.Printf("  JUNK: %q\n", r.Sender)
		}
	}
	fmt.Printf("total irs=%d, junk lid=%d\n", len(irs), junk)

	fmt.Println("== chat_histories: sender mirip LID ==")
	var chs []struct{ Sender string }
	DB.Raw(`SELECT sender FROM chat_histories GROUP BY sender`).Scan(&chs)
	junk2 := 0
	for _, r := range chs {
		if looksLikeLIDLocal(r.Sender) {
			junk2++
			fmt.Printf("  JUNK-CH: %q\n", r.Sender)
		}
	}
	fmt.Printf("total ch-senders=%d, junk lid=%d\n", len(chs), junk2)

	fmt.Println("== grup (18+ digit) ==")
	for _, r := range chs {
		if len(r.Sender) >= 18 {
			fmt.Printf("  GRUP: %q (len=%d)\n", r.Sender, len(r.Sender))
		}
	}
}

