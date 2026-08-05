package services

import (
	"errors"
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestWAServerErrorCode(t *testing.T) {
	err := fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 463)
	code, ok := WAServerErrorCode(err)
	if !ok || code != 463 {
		t.Fatalf("WAServerErrorCode() = (%d, %v), want (463, true)", code, ok)
	}
}

func TestWAServerErrorCodeRejectsUnrelatedError(t *testing.T) {
	if code, ok := WAServerErrorCode(errors.New("server returned error 463")); ok || code != 0 {
		t.Fatalf("WAServerErrorCode() = (%d, %v), want (0, false)", code, ok)
	}
}
