package handlers

import (
	"errors"
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow"
)

func TestClassifyBroadcastSendError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantAction broadcastSendErrorAction
		wantCode   int
	}{
		{
			name:       "whatsapp restriction 463",
			err:        fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 463),
			wantAction: broadcastErrorWARestricted,
			wantCode:   463,
		},
		{
			name:       "whatsapp rate limited 429",
			err:        fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 429),
			wantAction: broadcastErrorWARestricted,
			wantCode:   429,
		},
		{
			name:       "whatsapp forbidden 403",
			err:        fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 403),
			wantAction: broadcastErrorWARestricted,
			wantCode:   403,
		},
		{
			name:       "whatsapp unauthorized 401",
			err:        fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 401),
			wantAction: broadcastErrorWARestricted,
			wantCode:   401,
		},
		{
			name:       "connection lost",
			err:        errors.New("client WA tidak terhubung"),
			wantAction: broadcastErrorInterrupted,
		},
		{
			name:       "belum login",
			err:        errors.New("akun WA belum login"),
			wantAction: broadcastErrorInterrupted,
		},
		{
			name:       "banned text",
			err:        errors.New("account banned from sending"),
			wantAction: broadcastErrorWARestricted,
		},
		{
			name:       "rate limit text",
			err:        errors.New("rate limit exceeded"),
			wantAction: broadcastErrorWARestricted,
		},
		{
			name:       "recipient failure",
			err:        errors.New("unknown recipient error"),
			wantAction: broadcastErrorFailed,
		},
		{
			name:       "nil error",
			err:        nil,
			wantAction: broadcastErrorFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, code := classifyBroadcastSendError(tt.err)
			if action != tt.wantAction || code != tt.wantCode {
				t.Fatalf("classifyBroadcastSendError() = (%q, %d), want (%q, %d)", action, code, tt.wantAction, tt.wantCode)
			}
		})
	}
}

func TestIsSystemicSendFailure(t *testing.T) {
	if !isSystemicSendFailure(errors.New("request timeout")) {
		t.Fatal("timeout should be systemic")
	}
	if isSystemicSendFailure(errors.New("user is not on whatsapp")) {
		t.Fatal("not on whatsapp should not trip circuit breaker")
	}
	if isSystemicSendFailure(fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, 500)) {
		// 500 is server error code — WAServerErrorCode returns true → systemic
	} else {
		t.Fatal("server error code should be systemic")
	}
}
