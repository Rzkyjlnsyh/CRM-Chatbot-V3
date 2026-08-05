package services

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestPhoneNumberForRegularJID(t *testing.T) {
	got, err := phoneNumberForJID(context.Background(), nil, types.NewJID("628123456789", types.DefaultUserServer))
	if err != nil || got != "628123456789" {
		t.Fatalf("nomor reguler = %q, err=%v", got, err)
	}
}

func TestPhoneNumberForNonContactJID(t *testing.T) {
	got, err := phoneNumberForJID(context.Background(), nil, types.NewJID("12345", types.GroupServer))
	if err != nil || got != "" {
		t.Fatalf("JID grup seharusnya dilewati, dapat %q, err=%v", got, err)
	}
}
