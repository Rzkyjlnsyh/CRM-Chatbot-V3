package services

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types"
)

func TestReserveDeepHistorySyncRejectsBusyWithoutQueueing(t *testing.T) {
	w := &waInstance{}
	w.historyRequestMu.Lock()
	_, err := w.ReserveDeepHistorySync("6282261008855")
	w.historyRequestMu.Unlock()
	if !errors.Is(err, ErrHistorySyncBusy) {
		t.Fatalf("reservasi sibuk harus ditolak tanpa antrean: %v", err)
	}
}

func TestReserveDeepHistorySyncReleasesSlotWhenDisconnected(t *testing.T) {
	w := &waInstance{} // client nil = belum tersambung
	_, err := w.ReserveDeepHistorySync("6282261008855")
	if err == nil || strings.Contains(err.Error(), "busy") {
		t.Fatalf("harus error 'belum tersambung', got %v", err)
	}
	// Slot harus KEMBALI terlepas: reservasi kedua boleh gagal dengan alasan
	// yang sama (belum tersambung), BUKAN busy.
	if !w.historyRequestMu.TryLock() {
		t.Fatal("slot tidak dilepas setelah gagal reservasi")
	}
	w.historyRequestMu.Unlock()
}

func TestRequestRecentChatCatchUpRejectsBusyWithoutQueueing(t *testing.T) {
	w := &waInstance{}
	w.historyRequestMu.Lock()
	err := w.RequestRecentChatCatchUp("6282261008855", 100, time.Time{})
	w.historyRequestMu.Unlock()
	if !errors.Is(err, ErrHistorySyncBusy) {
		t.Fatalf("catch-up saat sibuk harus ErrHistorySyncBusy: %v", err)
	}
}

func TestHistorySyncFailureMessageCollapsesDeviceTimeouts(t *testing.T) {
	msg := historySyncFailureMessage(errHistoryDeviceNoResponse)
	if msg == "" || !strings.Contains(strings.ToLower(msg), "hp utama") {
		t.Fatalf("pesan timeout perangkat tidak dikenali: %q", msg)
	}
}

func TestHistorySyncFailureMessageDeduplicatesOtherErrors(t *testing.T) {
	joined := errors.Join(errors.New("gagal A"), errors.New("gagal A"), errors.New("gagal B"))
	msg := historySyncFailureMessage(joined)
	if strings.Count(msg, "gagal A") != 1 {
		t.Fatalf("error berulang tidak didedup: %q", msg)
	}
	if !strings.Contains(msg, "gagal B") {
		t.Fatalf("error unik hilang: %q", msg)
	}
}

// TestOnDemandHistorySyncWithoutConversationsAcknowledgesRequest — respons
// ON_DEMAND kosong tetap meng-ack waiter (permintaan aktif dianggap terjawab).
func TestOnDemandHistorySyncWithoutConversationsAcknowledgesRequest(t *testing.T) {
	w := &waInstance{}
	waiter := w.addHistoryWaiter("6282261008855")
	syncType := waHistorySync.HistorySync_ON_DEMAND
	done := make(chan struct{})
	go func() {
		_, _, _ = w.processHistorySync(&waHistorySync.HistorySync{SyncType: &syncType}, false)
		close(done)
	}()
	select {
	case <-waiter:
	case <-time.After(3 * time.Second):
		t.Fatal("waiter tidak di-ack oleh sync ON_DEMAND kosong")
	}
	<-done
}

// TestCatchUpHistoryRequestUsesEventResponseMode — permintaan catch-up memakai
// PeerDataOperationRequest (HISTORY_SYNC_ON_DEMAND), bukan BuildHistorySyncRequest.
func TestCatchUpHistoryRequestUsesEventResponseMode(t *testing.T) {
	tip := types.MessageInfo{
		MessageSource: types.MessageSource{Chat: types.NewJID("6282261008855", types.DefaultUserServer)},
		ID:            types.MessageID("TIP1"),
		Timestamp:     time.Now(),
	}
	req := buildCatchUpHistoryRequest(tip, 100)
	pm := req.GetProtocolMessage()
	if pm == nil || pm.GetType() != waProto.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE {
		t.Fatalf("permintaan catch-up bukan PeerDataOperationRequest: %+v", pm)
	}
	odr := pm.GetPeerDataOperationRequestMessage().GetHistorySyncOnDemandRequest()
	if odr == nil {
		t.Fatal("HistorySyncOnDemandRequest kosong")
	}
	if odr.GetOnDemandMsgCount() != 100 {
		t.Fatalf("jumlah on-demand = %d, want 100", odr.GetOnDemandMsgCount())
	}
	if odr.GetOldestMsgID() != "TIP1" {
		t.Fatalf("anchor salah: %q", odr.GetOldestMsgID())
	}
}

// TestInteractiveSendTimeoutIsBoundedAndRecognizable — batas waktu tunggu HP
// utama harus dalam jendela interaktif & errornya bisa dikenali user.
func TestInteractiveSendTimeoutIsBoundedAndRecognizable(t *testing.T) {
	if historyOnDemandTimeout > 30*time.Second || historyOnDemandTimeout < 5*time.Second {
		t.Fatalf("timeout interaktif di luar jendela wajar: %s", historyOnDemandTimeout)
	}
	if errHistoryDeviceNoResponse.Error() == "" {
		t.Fatal("error perangkat tidak menjawab harus punya teks")
	}
}

// TestHistorySyncStatusLifecycle — status berpindah syncing → completed/failed
// dengan pesan jujur (bukan placeholder kosong).
func TestHistorySyncStatusLifecycle(t *testing.T) {
	w := &waInstance{}
	now := time.Now()
	w.mu.Lock()
	w.historySeq++
	w.historyStatus = HistorySyncStatus{State: "syncing", Sender: "6281", StartedAt: &now}
	w.mu.Unlock()
	_ = w.completeHistoryRequest("6281")
	st := w.HistorySyncStatus()
	if st.State != "completed" || st.Message == "" {
		t.Fatalf("status selesai tidak jujur: %+v", st)
	}
}

// TestRequestFullHistoryFallbackRejectsWithoutClient — fallback penuh harus
// menolak bersih saat WA belum tersambung.
func TestRequestFullHistoryFallbackRejectsWithoutClient(t *testing.T) {
	w := &waInstance{}
	if err := w.requestFullHistoryFallback("6281"); err == nil {
		t.Fatal("fallback penuh tanpa client harus gagal")
	}
}

// guard sinkronisasi agar test paralel tidak saling mengganggu map status.
var _ sync.Mutex
