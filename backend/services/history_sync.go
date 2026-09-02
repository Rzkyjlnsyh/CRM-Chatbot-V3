package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ─────────────────────────────────────────────────────────────────────────
// History Sync — import riwayat WhatsApp dari perangkat utama.
// Pola: v4 (chatloop-1.6-1.7); diport ke fork klien agar chat lama tidak hilang.
// API whatsmeow 30-Jul: events.HistorySync{Data *waHistorySync.HistorySync},
// pesan dibungkus HistorySyncMsg{Message *waWeb.WebMessageInfo}.
// ─────────────────────────────────────────────────────────────────────────

// HistorySyncHandler dipanggil dengan batch pesan riwayat yang berhasil diparse.
type HistorySyncHandler func(agentID uint, messages []HistoricalMessage) (imported int, skipped int, err error)

var onHistorySync HistorySyncHandler

// SetHistorySyncHandler dipasang sekali saat startup worker.
func SetHistorySyncHandler(handler HistorySyncHandler) { onHistorySync = handler }

// MessageRevokeHandler = pesan dihapus (revoke) dari perangkat lain.
type MessageRevokeHandler func(agentID uint, waMsgID string, timestamp time.Time)

var onMessageRevoke MessageRevokeHandler

// SetMessageRevokeHandler dipasang sekali saat startup worker.
func SetMessageRevokeHandler(handler MessageRevokeHandler) { onMessageRevoke = handler }

// HistorySyncStatus = kondisi sinkronisasi terakhir per agent (in-memory).
type HistorySyncStatus struct {
	AgentID    uint
	Started    time.Time
	Finished   time.Time
	Imported   int
	Skipped    int
	InProgress bool
	Processed  int
	BatchCount int
}

var historySyncStatuses = map[uint]*HistorySyncStatus{}
var historySyncStatusMu sync.Mutex

func HistorySyncStatusFor(agentID uint) *HistorySyncStatus {
	historySyncStatusMu.Lock()
	defer historySyncStatusMu.Unlock()
	st, ok := historySyncStatuses[agentID]
	if !ok {
		st = &HistorySyncStatus{AgentID: agentID}
		historySyncStatuses[agentID] = st
	}
	return st
}

// HistoricalMessage = satu pesan dari riwayat WhatsApp (hasil parsing web proto).
type HistoricalMessage struct {
	Sender        string
	Text          string
	FromMe        bool
	MediaType     string
	FileName      string
	Mimetype      string
	WAMsgID       string
	ReplyTo       string
	ReplyText     string
	PushName      string
	Timestamp     time.Time
	Unread        bool   // pesan terakhir percakapan belum dibaca
	MediaMetadata []byte // protobuf pesan asli (untuk unduh media on-demand)
}

const historySyncChunkSize = 500

// processHistorySync mengubah payload HistorySync menjadi batch pesan.
func (w *waInstance) processHistorySync(payload *waHistorySync.HistorySync, deep bool) (int, int, error) {
	st := HistorySyncStatusFor(w.agentID)
	st.InProgress = true
	st.Started = time.Now()
	defer func() {
		st.InProgress = false
		st.Finished = time.Now()
	}()

	imported, skipped := 0, 0
	batch := make([]HistoricalMessage, 0, historySyncChunkSize)
	flush := func() bool {
		if onHistorySync == nil || len(batch) == 0 {
			return true
		}
		imp, skip, err := onHistorySync(w.agentID, batch)
		if err != nil {
			log.Printf("WA agent %d history sync error: %v", w.agentID, err)
		}
		imported += imp
		skipped += skip
		st.Imported += imp
		st.Skipped += skip
		st.Processed += len(batch)
		st.BatchCount++
		batch = batch[:0]
		return true
	}

	for _, conv := range payload.GetConversations() {
		lastTs := time.Unix(int64(conv.GetLastMsgTimestamp()), 0)
		for _, msgWrap := range conv.GetMessages() {
			msgEvt := msgWrap.GetMessage()
			if msgEvt == nil {
				continue
			}
			parsed, ok := unwrapHistoryMessage(w, conv, msgEvt, lastTs)
			if !ok {
				continue
			}
			if parsed.MediaType != "" {
				// Simpan protobuf asli untuk unduh media on-demand saat dibuka.
				if raw, err := proto.Marshal(msgEvt.GetMessage()); err == nil {
					parsed.MediaMetadata = raw
				}
			}
			batch = append(batch, parsed)
			if len(batch) >= historySyncChunkSize && !flush() {
				return imported, skipped, errors.New("history sync dibatalkan handler")
			}
		}
	}
	flush()
	return imported, skipped, nil
}

// unwrapHistoryMessage memparse satu pesan riwayat menjadi HistoricalMessage.
func unwrapHistoryMessage(w *waInstance, conv *waHistorySync.Conversation, msgEvt *waWeb.WebMessageInfo, lastTs time.Time) (HistoricalMessage, bool) {
	key := msgEvt.GetKey()
	if key == nil || key.FromMe == nil || key.ID == nil || *key.ID == "" {
		return HistoricalMessage{}, false
	}
	remote := key.GetRemoteJID()
	if strings.Contains(strings.ToLower(remote), "@g.us") ||
		strings.Contains(strings.ToLower(remote), "@broadcast") ||
		strings.Contains(strings.ToLower(remote), "@newsletter") {
		return HistoricalMessage{}, false
	}
	sender := NormalizePhone(remote)
	msg := msgEvt.GetMessage()
	if msg == nil {
		return HistoricalMessage{}, false
	}
	// Revoke di riwayat: pesan terhapus → jangan impor isinya.
	if pm := msg.GetProtocolMessage(); pm != nil && pm.GetType() == waProto.ProtocolMessage_REVOKE {
		return HistoricalMessage{}, false
	}
	text, mediaType, fileName, mimetype := textForHistoryMessage(msg)
	replyTo, replyText := replyInfoFromHistory(msg)
	if text == "" && mediaType == "" {
		return HistoricalMessage{}, false
	}
	ts := time.Unix(int64(msgEvt.GetMessageTimestamp()), 0)
	return HistoricalMessage{
		Sender:    sender,
		Text:      text,
		FromMe:    key.GetFromMe(),
		MediaType: mediaType,
		FileName:  fileName,
		Mimetype:  mimetype,
		WAMsgID:   key.GetID(),
		ReplyTo:   replyTo,
		ReplyText: replyText,
		PushName:  msgEvt.GetPushName(),
		Timestamp: ts,
		Unread:    !ts.IsZero() && lastTs.Unix() == ts.Unix() && conv.GetUnreadCount() > 0,
	}, true
}

func replyInfoFromHistory(msg *waProto.Message) (string, string) {
	ci := msg.GetExtendedTextMessage().GetContextInfo()
	if ci == nil {
		return "", ""
	}
	return ci.GetStanzaID(), ""
}

func textForHistoryMessage(msg *waProto.Message) (text, mediaType, fileName, mimetype string) {
	switch {
	case msg.GetConversation() != "":
		return msg.GetConversation(), "", "", ""
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetText(), "", "", ""
	case msg.GetImageMessage() != nil:
		m := msg.GetImageMessage()
		return m.GetCaption(), "image", "", m.GetMimetype()
	case msg.GetVideoMessage() != nil:
		m := msg.GetVideoMessage()
		return m.GetCaption(), "video", "", m.GetMimetype()
	case msg.GetAudioMessage() != nil:
		m := msg.GetAudioMessage()
		return "", "audio", "", m.GetMimetype()
	case msg.GetDocumentMessage() != nil:
		m := msg.GetDocumentMessage()
		return m.GetCaption(), "document", m.GetFileName(), m.GetMimetype()
	case msg.GetStickerMessage() != nil:
		return "", "sticker", "", "image/webp"
	case msg.GetLocationMessage() != nil:
		return "", "location", "", ""
	default:
		return "", "", "", ""
	}
}

// BuildHistorySyncRequestMessage: permintaan riwayat tambahan dari perangkat
// primer (pakai API resmi whatsmeow 30-Jul).
func (w *waInstance) BuildHistorySyncRequestMessage(lastKnown *types.MessageInfo, count int) *waProto.Message {
	w.mu.Lock()
	client := w.client
	w.mu.Unlock()
	if client == nil || lastKnown == nil {
		return nil
	}
	return client.BuildHistorySyncRequest(lastKnown, count)
}

// DownloadHistoryMedia mengunduh lampiran riwayat WhatsApp saat dibuka di
// Inbox (on-demand). Metadata = protobuf pesan asli yang disimpan saat
// HistorySync. Return (bytes, mimetype, err).
func (w *waInstance) DownloadHistoryMedia(ctx context.Context, metadata []byte) ([]byte, string, error) {
	if len(metadata) == 0 {
		return nil, "", errors.New("metadata media tidak tersedia")
	}
	w.mu.Lock()
	client := w.client
	connected := client != nil && client.IsConnected()
	w.mu.Unlock()
	if !connected {
		return nil, "", errors.New("WhatsApp belum terhubung")
	}
	var msg waProto.Message
	if err := proto.Unmarshal(metadata, &msg); err != nil {
		return nil, "", fmt.Errorf("metadata media rusak: %w", err)
	}
	mime := ""
	switch {
	case msg.GetImageMessage() != nil:
		mime = msg.GetImageMessage().GetMimetype()
		data, err := client.Download(ctx, msg.GetImageMessage())
		return data, mime, err
	case msg.GetDocumentMessage() != nil:
		mime = msg.GetDocumentMessage().GetMimetype()
		data, err := client.Download(ctx, msg.GetDocumentMessage())
		return data, mime, err
	case msg.GetVideoMessage() != nil:
		mime = msg.GetVideoMessage().GetMimetype()
		data, err := client.Download(ctx, msg.GetVideoMessage())
		return data, mime, err
	case msg.GetAudioMessage() != nil:
		mime = msg.GetAudioMessage().GetMimetype()
		data, err := client.Download(ctx, msg.GetAudioMessage())
		return data, mime, err
	case msg.GetStickerMessage() != nil:
		mime = msg.GetStickerMessage().GetMimetype()
		data, err := client.Download(ctx, msg.GetStickerMessage())
		return data, mime, err
	default:
		return nil, "", errors.New("jenis media tidak didukung")
	}
}
