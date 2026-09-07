package services

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestNotifyHistoryChatStateReleasesMatchingWaiters(t *testing.T) {
	w := &waInstance{historyWaiters: make(map[string][]chan struct{})}
	first := make(chan struct{})
	second := make(chan struct{})
	other := make(chan struct{})
	w.historyWaiters["628111"] = []chan struct{}{first, second}
	w.historyWaiters["628222"] = []chan struct{}{other}

	w.notifyHistoryChatState("628111")

	for i, waiter := range []chan struct{}{first, second} {
		select {
		case <-waiter:
		case <-time.After(time.Second):
			t.Fatalf("waiter %d tidak dilepas", i)
		}
	}
	if _, exists := w.historyWaiters["628111"]; exists {
		t.Fatal("waiter yang sudah selesai masih tersimpan")
	}
	select {
	case <-other:
		t.Fatal("waiter percakapan lain ikut dilepas")
	default:
	}
}

func TestRemoveHistoryWaiterOnlyRemovesTarget(t *testing.T) {
	w := &waInstance{historyWaiters: make(map[string][]chan struct{})}
	first := make(chan struct{})
	second := make(chan struct{})
	w.historyWaiters["628111"] = []chan struct{}{first, second}

	w.removeHistoryWaiter("628111", first)

	waiters := w.historyWaiters["628111"]
	if len(waiters) != 1 || waiters[0] != second {
		t.Fatalf("waiter tersisa tidak sesuai: %#v", waiters)
	}
}

func TestIsOwnerReadReceipt(t *testing.T) {
	tests := []struct {
		name string
		evt  *events.Receipt
		want bool
	}{
		{"read from own device", &events.Receipt{MessageSource: types.MessageSource{IsFromMe: true}, Type: types.ReceiptTypeRead}, true},
		{"private read from own device", &events.Receipt{Type: types.ReceiptTypeReadSelf}, true},
		{"customer read outgoing message", &events.Receipt{Type: types.ReceiptTypeRead}, false},
		{"group owner read", &events.Receipt{MessageSource: types.MessageSource{IsFromMe: true, IsGroup: true}, Type: types.ReceiptTypeRead}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isOwnerReadReceipt(test.evt); got != test.want {
				t.Fatalf("isOwnerReadReceipt() = %v, ingin %v", got, test.want)
			}
		})
	}
}
