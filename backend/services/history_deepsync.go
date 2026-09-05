package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// history_deepsync — mesin deep-sync PENUH (pola v4 chatloop-1.6-1.7):
// reservasi antrean satu-per-satu, catch-up per percakapan, paginasi ke
// belakang, status jujur untuk UI, dan pesan error yang bisa dipahami user.

const (
	// historyOnDemandPageSize = ukuran 1 halaman protokol saat deep sync.
	historyOnDemandPageSize = 300
	// historyOnDemandMaxCount = batas atas jumlah pesan per request on-demand.
	historyOnDemandMaxCount = 500
	// historyDeepMaxPages = pengaman (~12k pesan teoritis per klik sinkron).
	historyDeepMaxPages = 40
	// historyOnDemandTimeout = batas tunggu HP utama merespons (di bawah batas
	// interaktif agar tombol tidak terasa menggantung selamanya).
	historyOnDemandTimeout = 12 * time.Second
)

var (
	// ErrHistorySyncBusy = ada sinkronisasi lain yang sedang berjalan.
	ErrHistorySyncBusy = errors.New("sinkronisasi riwayat lain masih berjalan")
	// errHistoryAnchorUnavailable = belum ada pesan acuan untuk catch-up.
	errHistoryAnchorUnavailable = errors.New("belum ada pesan acuan untuk catch-up")
	// errHistoryDeviceNoResponse = perangkat utama tidak merespons.
	errHistoryDeviceNoResponse = errors.New("perangkat utama WhatsApp tidak merespons sinkronisasi")
)

// ─────────────────────────────────────────────────────────────────────────────
// Status & pesan yang jujur untuk UI
// ─────────────────────────────────────────────────────────────────────────────

// historySyncFailureMessage mengubah error teknis jadi kalimat yang bisa
// dipahami user. Error berulang dari errors.Join didedup agar satu kegagalan
// tidak terlihat seperti banyak masalah.
func historySyncFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, errHistoryDeviceNoResponse) {
		return "Perangkat tertaut tetap online, tetapi HP utama belum mengirim paket riwayat. Buka chat tersebut di HP dan biarkan WhatsApp aktif, lalu coba lagi."
	}
	seen := make(map[string]bool)
	parts := make([]string, 0, 3)
	for _, part := range strings.Split(err.Error(), "\n") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}

// historySyncUserMessage = teks status untuk UI (jujur, bukan pujian palsu).
func historySyncUserMessage(s HistorySyncStatus) string {
	switch s.State {
	case "syncing":
		return "Mengambil riwayat dari HP…"
	case "completed":
		if s.StillStale {
			return "Sinkronisasi selesai, tetapi sebagian riwayat di HP tampaknya masih lebih baru. Coba lagi sebentar lagi."
		}
		return "Sinkronisasi riwayat selesai."
	case "failed":
		if strings.TrimSpace(s.Error) != "" {
			return s.Error
		}
		return "Sinkronisasi riwayat gagal. Coba lagi."
	default:
		return ""
	}
}

// ChatWATipTime = titik waktu terakhir yang diketahui dari WA (last_msg_at).
func ChatWATipTime(agentID uint, sender string) time.Time {
	var state models.InboxReadState
	if database.DB.Select("last_msg_at").
		Where("agent_id = ? AND sender = ?", agentID, sender).
		First(&state).Error != nil {
		return time.Time{}
	}
	if state.LastMsgAt == nil || state.LastMsgAt.IsZero() {
		return time.Time{}
	}
	return *state.LastMsgAt
}

// ChatPreviewStale = true bila pratinjau Inbox lokal tertinggal dari WA
// (pesan di HP lebih baru dari baris lokal) → layak di-catch-up.
func ChatPreviewStale(agentID uint, sender string) bool {
	sender = NormalizeInboxSender(sender)
	if agentID == 0 || sender == "" {
		return false
	}
	var localMax *time.Time
	if err := database.DB.Model(&models.ChatHistory{}).
		Select("MAX(created_at)").
		Where("agent_id = ? AND sender = ?", agentID, sender).
		Scan(&localMax).Error; err != nil {
		return false
	}
	hasLocal := localMax != nil
	tip := ChatWATipTime(agentID, sender)
	if tip.IsZero() {
		return false
	}
	if !hasLocal {
		return true
	}
	return tip.After(localMax.Add(5 * time.Second))
}

// recipientJID membangun JID chat dari nomor/grup yang sudah dinormalisasi.
func recipientJID(sender string) (types.JID, error) {
	sender = NormalizeInboxSender(sender)
	if sender == "" {
		return types.EmptyJID, errors.New("alamat percakapan kosong")
	}
	if IsGroupJID(sender) {
		return types.NewJID(sender, types.GroupServer), nil
	}
	return types.NewJID(sender, types.DefaultUserServer), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Permintaan ke perangkat utama
// ─────────────────────────────────────────────────────────────────────────────

func clampHistoryOnDemandCount(count int) int {
	if count <= 0 || count > historyOnDemandMaxCount {
		return historyOnDemandMaxCount
	}
	return count
}

// buildCatchUpHistoryRequest membangun permintaan catch-up via
// PeerDataOperationRequest (HISTORY_SYNC_ON_DEMAND) — jalur resmi whatsmeow
// yang tidak menunggu waktu interaktif lama.
func buildCatchUpHistoryRequest(lastKnown types.MessageInfo, count int) *waProto.Message {
	ts := lastKnown.Timestamp
	if ts.IsZero() {
		ts = time.Now().Add(2 * time.Minute)
	}
	return &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Type: waProto.ProtocolMessage_PEER_DATA_OPERATION_REQUEST_MESSAGE.Enum(),
			PeerDataOperationRequestMessage: &waProto.PeerDataOperationRequestMessage{
				PeerDataOperationRequestType: waProto.PeerDataOperationRequestType_HISTORY_SYNC_ON_DEMAND.Enum(),
				HistorySyncOnDemandRequest: &waProto.PeerDataOperationRequestMessage_HistorySyncOnDemandRequest{
					ChatJID:              proto.String(lastKnown.Chat.String()),
					OldestMsgID:          proto.String(string(lastKnown.ID)),
					OldestMsgFromMe:      proto.Bool(lastKnown.IsFromMe),
					OnDemandMsgCount:     proto.Int32(int32(count)),
					OldestMsgTimestampMS: proto.Int64(ts.Unix()),
				},
			},
		},
	}
}

// sendHistoryOnDemand mengirim permintaan on-demand & menunggu ack. Status UI
// diperbarui jujur (syncing → completed/failed). historyRequestMu dipastikan
// sudah dipegang oleh entrypoint publik.
func (w *waInstance) sendHistoryOnDemand(anchor types.MessageInfo, count int, sender, mode string, catchUp bool) error {
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	if !connected {
		w.mu.Unlock()
		err := errors.New("WhatsApp belum tersambung")
		w.failHistoryRequest(sender, err)
		return err
	}
	count = clampHistoryOnDemandCount(count)
	now := time.Now()
	w.historySeq++
	prevImported := 0
	if strings.HasPrefix(mode, "deep_older") {
		prevImported = w.historyStatus.Imported
	}
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: mode, Sender: sender, StartedAt: &now,
		Imported: prevImported, Skipped: 0,
		Message: "Mengambil riwayat dari HP…",
	}
	w.mu.Unlock()

	waiter := w.addHistoryWaiter(sender)
	defer w.removeHistoryWaiter(sender, waiter)

	var req *waProto.Message
	if catchUp {
		req = buildCatchUpHistoryRequest(anchor, count)
	} else {
		req = client.BuildHistorySyncRequest(&anchor, count)
	}
	ctx, cancel := context.WithTimeout(context.Background(), historyOnDemandTimeout)
	defer cancel()
	if _, err := client.SendPeerMessage(ctx, req); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = fmt.Errorf("%w (permintaan, batas %s): %v", errHistoryDeviceNoResponse, historyOnDemandTimeout, err)
		}
		w.failHistoryRequest(sender, err)
		return err
	}
	// Ack datang lewat processHistorySync (notifyAllHistoryWaiters) saat
	// chunk ON_DEMAND/FULL selesai ditulis ke DB.
	select {
	case <-waiter:
		w.completeHistoryRequest(sender)
		return nil
	case <-ctx.Done():
		err := fmt.Errorf("%w (menunggu paket riwayat, batas %s)", errHistoryDeviceNoResponse, historyOnDemandTimeout)
		w.failHistoryRequest(sender, err)
		return err
	}
}

func (w *waInstance) failHistoryRequest(sender string, requestErr error) {
	finished := time.Now()
	stale := ChatPreviewStale(w.agentID, sender)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.historyStatus.State = "failed"
	w.historyStatus.Sender = sender
	if requestErr != nil {
		w.historyStatus.Error = historySyncFailureMessage(requestErr)
	}
	w.historyStatus.FinishedAt = &finished
	w.historyStatus.StillStale = stale
	w.historyStatus.Message = historySyncUserMessage(w.historyStatus)
}

func (w *waInstance) completeHistoryRequest(sender string) error {
	finished := time.Now()
	stale := ChatPreviewStale(w.agentID, sender)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.historyStatus.State == "failed" {
		if strings.TrimSpace(w.historyStatus.Error) != "" {
			return errors.New(w.historyStatus.Error)
		}
		return errors.New("gagal mengimpor riwayat WhatsApp")
	}
	w.historyStatus.State = "completed"
	w.historyStatus.Sender = sender
	w.historyStatus.Progress = 100
	w.historyStatus.FinishedAt = &finished
	w.historyStatus.StillStale = stale
	w.historyStatus.Error = ""
	w.historyStatus.Message = historySyncUserMessage(w.historyStatus)
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Entrypoint publik (antrean satu-per-satu)
// ─────────────────────────────────────────────────────────────────────────────

// ReserveDeepHistorySync mengambil slot sinkronisasi; bila slot sibuk,
// langsung menolak (ErrHistorySyncBusy) — tidak ada antrean senyap.
func (w *waInstance) ReserveDeepHistorySync(sender string) (HistorySyncStatus, error) {
	sender = NormalizeInboxSender(strings.TrimSpace(sender))
	if sender == "" {
		return w.HistorySyncStatus(), errors.New("nomor percakapan kosong")
	}
	if !w.historyRequestMu.TryLock() {
		return w.HistorySyncStatus(), ErrHistorySyncBusy
	}
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	if !connected {
		w.mu.Unlock()
		w.historyRequestMu.Unlock()
		return w.HistorySyncStatus(), errors.New("WhatsApp belum tersambung")
	}
	now := time.Now()
	w.historySeq++
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: "deep", Sender: sender, StartedAt: &now,
		Message: "Mengambil riwayat lengkap dari HP…",
	}
	status := w.historyStatus
	w.mu.Unlock()
	return status, nil
}

// RunReservedDeepHistorySync menjalankan job setelah ReserveDeepHistorySync
// sukses (slot dilepas di akhir fungsi).
func (w *waInstance) RunReservedDeepHistorySync(sender string) error {
	defer w.historyRequestMu.Unlock()
	return w.requestDeepHistorySyncLocked(sender)
}

// requestDeepHistorySyncLocked = deep sync multi-pass:
// 1) catch-up celah ujung, 2) paginasi ke belakang sampai HP habis,
// 3) bila belum ada acuan sama sekali → FULL_HISTORY sebagai pemulihan resmi.
func (w *waInstance) requestDeepHistorySyncLocked(sender string) error {
	sender = NormalizeInboxSender(strings.TrimSpace(sender))
	if sender == "" {
		return errors.New("nomor percakapan kosong")
	}
	nowStart := time.Now()
	w.mu.Lock()
	w.historySeq++
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: "deep", Sender: sender, StartedAt: &nowStart,
		Message: "Mengambil riwayat lengkap dari HP…",
	}
	w.mu.Unlock()

	var beforeCount int64
	database.DB.Model(&models.ChatHistory{}).
		Where("agent_id = ? AND sender = ?", w.agentID, sender).
		Count(&beforeCount)

	var syncErr error
	waTip := ChatWATipTime(w.agentID, sender)
	stopRequests := false
	// Pass 1: tutup celah ujung (pesan terbaru yang belum sempat masuk).
	if err := w.requestChatCatchUpLocked(sender, historyOnDemandPageSize, waTip, false); err != nil {
		log.Printf("WA agent %d: deep catch-up %s: %v", w.agentID, sender, err)
		if errors.Is(err, errHistoryAnchorUnavailable) {
			// Tanpa row lokal memang tidak ada acuan — FULL_HISTORY di pass 3.
		} else {
			syncErr = errors.Join(syncErr, err)
			stopRequests = true
		}
	}

	// Pass 2: paginasi ke belakang dari pesan lokal tertua.
	emptyStreak := 0
	for page := 0; !stopRequests && page < historyDeepMaxPages; page++ {
		var oldest models.ChatHistory
		if err := database.DB.
			Where("agent_id = ? AND sender = ? AND wa_msg_id <> '' AND wa_msg_id IS NOT NULL", w.agentID, sender).
			Order("created_at ASC, id ASC").
			First(&oldest).Error; err != nil {
			break
		}
		chat, err := recipientJID(sender)
		if err != nil {
			syncErr = errors.Join(syncErr, err)
			break
		}
		fromMe := strings.TrimSpace(oldest.Message) == "" && strings.TrimSpace(oldest.Reply) != ""
		anchor := types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, IsFromMe: fromMe},
			ID:            types.MessageID(oldest.WAMsgID),
			Timestamp:     oldest.CreatedAt,
		}
		var before int64
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ?", w.agentID, sender).
			Count(&before)
		if err := w.sendHistoryOnDemand(anchor, historyOnDemandPageSize, sender, "deep_older", false); err != nil {
			syncErr = errors.Join(syncErr, err)
			break
		}
		var after int64
		database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ?", w.agentID, sender).
			Count(&after)
		if after <= before {
			emptyStreak++
		} else {
			emptyStreak = 0
		}
		if emptyStreak >= 2 {
			break
		}
	}

	// Pass 3: fallback pemulihan resmi bila tidak ada acuan lokal sama sekali.
	if beforeCount == 0 && !stopRequests {
		if err := w.requestFullHistoryFallback(sender); err != nil {
			syncErr = errors.Join(syncErr, err)
		}
	}

	if syncErr != nil {
		w.failHistoryRequest(sender, syncErr)
		return syncErr
	}
	return w.completeHistoryRequest(sender)
}

// requestFullHistoryFallback meminta sync penuh (FULL) bila tidak ada acuan.
func (w *waInstance) requestFullHistoryFallback(sender string) error {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || !client.IsConnected() || !client.IsLoggedIn() {
		return errors.New("WhatsApp belum tersambung")
	}
	req := client.BuildHistorySyncRequest(nil, 0)
	if req == nil {
		return errors.New("gagal membangun permintaan riwayat penuh")
	}
	waiter := w.addHistoryWaiter(sender)
	defer w.removeHistoryWaiter(sender, waiter)
	ctx, cancel := context.WithTimeout(context.Background(), historyOnDemandTimeout)
	defer cancel()
	if _, err := client.SendMessage(ctx, client.Store.ID.ToNonAD(), req); err != nil {
		return fmt.Errorf("%w (permintaan penuh): %v", errHistoryDeviceNoResponse, err)
	}
	select {
	case <-waiter:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w (menunggu riwayat penuh, batas %s)", errHistoryDeviceNoResponse, historyOnDemandTimeout)
	}
}

// requestChatCatchUpLocked menarik pesan terbaru satu percakapan (ON_DEMAND
// dengan acuan tip sintetis atau stanza ID asli untuk grup).
func (w *waInstance) requestChatCatchUpLocked(sender string, count int, waLastHint time.Time, catchUp bool) error {
	sender = NormalizeInboxSender(sender)
	if sender == "" {
		return errors.New("alamat percakapan kosong")
	}
	count = clampHistoryOnDemandCount(count)
	chat, err := recipientJID(sender)
	if err != nil {
		return err
	}
	if tip := ChatWATipTime(w.agentID, sender); tip.After(waLastHint) {
		waLastHint = tip
	}
	// Grup dengan riwayat lokal: pakai stanza ID asli (lebih andal).
	if IsGroupJID(sender) {
		var newest models.ChatHistory
		if err := database.DB.
			Where("agent_id = ? AND sender = ? AND wa_msg_id <> '' AND wa_msg_id IS NOT NULL", w.agentID, sender).
			Order("created_at DESC, id DESC").
			First(&newest).Error; err == nil {
			fromMe := strings.TrimSpace(newest.Message) == "" && strings.TrimSpace(newest.Reply) != ""
			anchor := types.MessageInfo{
				MessageSource: types.MessageSource{Chat: chat, IsFromMe: fromMe},
				ID:            types.MessageID(newest.WAMsgID),
				Timestamp:     newest.CreatedAt,
			}
			return w.sendHistoryOnDemand(anchor, count, sender, "recent_group", catchUp)
		}
	}
	now := time.Now()
	tipAt := now.Add(2 * time.Minute)
	if !waLastHint.IsZero() &&
		waLastHint.After(now.Add(-24*time.Hour)) &&
		waLastHint.Before(now.Add(5*time.Minute)) {
		tipAt = waLastHint.Add(2 * time.Minute)
	}
	tip := types.MessageInfo{
		MessageSource: types.MessageSource{Chat: chat, IsFromMe: false},
		ID:            types.MessageID(fmt.Sprintf("3EB0AUTO%X", time.Now().UnixNano())),
		Timestamp:     tipAt,
	}
	return w.sendHistoryOnDemand(tip, count, sender, "auto_catch_up", catchUp)
}

// RequestRecentChatCatchUp = catch-up ringan satu percakapan (dipakai saat
// pengguna membuka chat yang pratinjaunya basi).
func (w *waInstance) RequestRecentChatCatchUp(sender string, count int, waLastHint time.Time) error {
	if !w.historyRequestMu.TryLock() {
		return ErrHistorySyncBusy
	}
	defer w.historyRequestMu.Unlock()
	return w.requestChatCatchUpLocked(sender, count, waLastHint, true)
}

// ReserveRecentHistorySync = slot background untuk grup (klik Inbox tidak
// menunggu timeout perangkat utama).
func (w *waInstance) ReserveRecentHistorySync(sender string) (HistorySyncStatus, error) {
	sender = NormalizeInboxSender(sender)
	if sender == "" {
		return w.HistorySyncStatus(), errors.New("alamat percakapan kosong")
	}
	if !w.historyRequestMu.TryLock() {
		return w.HistorySyncStatus(), ErrHistorySyncBusy
	}
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected() && client.IsLoggedIn()
	if !connected {
		w.mu.Unlock()
		w.historyRequestMu.Unlock()
		return w.HistorySyncStatus(), errors.New("WhatsApp belum tersambung")
	}
	now := time.Now()
	w.historySeq++
	w.historyStatus = HistorySyncStatus{
		State: "syncing", Mode: "recent_group", Sender: sender, StartedAt: &now,
		Message: "Mengambil pesan grup terbaru…",
	}
	status := w.historyStatus
	w.mu.Unlock()
	return status, nil
}

// RunReservedRecentHistorySync menjalankan job grup setelah reservasi sukses.
func (w *waInstance) RunReservedRecentHistorySync(sender string) error {
	defer w.historyRequestMu.Unlock()
	if err := w.requestChatCatchUpLocked(sender, 100, time.Time{}, false); err != nil {
		w.failHistoryRequest(sender, err)
		return err
	}
	return w.completeHistoryRequest(sender)
}

// HistorySyncStatus = akses publik status terakhir (untuk handler/UI).
func (w *waInstance) HistorySyncStatus() HistorySyncStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.historyStatus
}
