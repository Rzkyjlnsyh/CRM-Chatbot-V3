package services

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// history_content — helper konten & identitas pesan riwayat (pola v4
// chatloop-1.6-1.7): baca isi pesan riwayat, deteksi revoke, urutan waktu
// live yang anti-tabrakan detik, dan label pesan grup.

// unwrapHistoryProtoMessage membuka wrapper DeviceSent/Ephemeral/ViewOnce yang
// umum di history/export (pola v4) bila ParseWebMessage tidak sempat unwrap.
func unwrapHistoryProtoMessage(m *waProto.Message) *waProto.Message {
	if m == nil {
		return m
	}
	for i := 0; i < 4; i++ {
		switch {
		case m.GetDeviceSentMessage().GetMessage() != nil:
			m = m.GetDeviceSentMessage().GetMessage()
		case m.GetEphemeralMessage().GetMessage() != nil:
			m = m.GetEphemeralMessage().GetMessage()
		case m.GetViewOnceMessage().GetMessage() != nil:
			m = m.GetViewOnceMessage().GetMessage()
		case m.GetViewOnceMessageV2().GetMessage() != nil:
			m = m.GetViewOnceMessageV2().GetMessage()
		case m.GetViewOnceMessageV2Extension().GetMessage() != nil:
			m = m.GetViewOnceMessageV2Extension().GetMessage()
		case m.GetDocumentWithCaptionMessage().GetMessage() != nil:
			m = m.GetDocumentWithCaptionMessage().GetMessage()
		case m.GetEditedMessage().GetMessage() != nil:
			m = m.GetEditedMessage().GetMessage()
		default:
			return m
		}
	}
	return m
}

// contextReplyPreview merangkum pesan yang di-quote (teks/caption/label media)
// supaya Inbox bisa menampilkan quote intuitif tanpa lookup ID.
func contextReplyPreview(info *waProto.ContextInfo) string {
	if info == nil {
		return ""
	}
	q := info.GetQuotedMessage()
	if q == nil {
		return ""
	}
	if t := strings.TrimSpace(q.GetConversation()); t != "" {
		return truncateRunesBrief(t, 160)
	}
	if ext := q.GetExtendedTextMessage(); ext != nil {
		if t := strings.TrimSpace(ext.GetText()); t != "" {
			return truncateRunesBrief(t, 160)
		}
	}
	if img := q.GetImageMessage(); img != nil {
		if cap := strings.TrimSpace(img.GetCaption()); cap != "" {
			return "📷 " + truncateRunesBrief(cap, 140)
		}
		return "📷 Foto"
	}
	if vid := q.GetVideoMessage(); vid != nil {
		if cap := strings.TrimSpace(vid.GetCaption()); cap != "" {
			return "🎥 " + truncateRunesBrief(cap, 140)
		}
		return "🎥 Video"
	}
	return ""
}

// historicalMessageContent membaca teks/jenis media/reply dari pesan riwayat WA.
func historicalMessageContent(m *waProto.Message) (text, mediaType, fileName, mimetype, replyTo, replyText string) {
	if m == nil {
		return
	}
	m = unwrapHistoryProtoMessage(m)
	if t := m.GetConversation(); t != "" {
		return t, "", "", "", "", ""
	}
	if ext := m.GetExtendedTextMessage(); ext != nil {
		ci := ext.GetContextInfo()
		return ext.GetText(), "", "", "", contextReplyID(ci), contextReplyPreview(ci)
	}
	if img := m.GetImageMessage(); img != nil {
		ci := img.GetContextInfo()
		return img.GetCaption(), "image", "", img.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if doc := m.GetDocumentMessage(); doc != nil {
		ci := doc.GetContextInfo()
		return doc.GetCaption(), "document", doc.GetFileName(), doc.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if vid := m.GetVideoMessage(); vid != nil {
		ci := vid.GetContextInfo()
		return vid.GetCaption(), "video", "", vid.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if aud := m.GetAudioMessage(); aud != nil {
		ci := aud.GetContextInfo()
		return "", "audio", "", aud.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if sticker := m.GetStickerMessage(); sticker != nil {
		ci := sticker.GetContextInfo()
		return "", "sticker", "", sticker.GetMimetype(), contextReplyID(ci), contextReplyPreview(ci)
	}
	if loc := m.GetLocationMessage(); loc != nil {
		label := strings.TrimSpace(loc.GetName())
		if label == "" {
			label = "Lokasi"
		}
		ci := loc.GetContextInfo()
		return fmt.Sprintf("📍 %s\nhttps://maps.google.com/?q=%f,%f", label, loc.GetDegreesLatitude(), loc.GetDegreesLongitude()), "", "", "", contextReplyID(ci), contextReplyPreview(ci)
	}
	if live := m.GetLiveLocationMessage(); live != nil {
		ci := live.GetContextInfo()
		return fmt.Sprintf("📍 Lokasi live\nhttps://maps.google.com/?q=%f,%f", live.GetDegreesLatitude(), live.GetDegreesLongitude()), "", "", "", contextReplyID(ci), contextReplyPreview(ci)
	}
	if text, _, reply, ok := interactiveReplyText(m); ok {
		return text, "", "", "", reply, ""
	}
	return
}

// protocolRevokeTargetID membaca ID pesan target dari ProtocolMessage REVOKE.
func protocolRevokeTargetID(message *waProto.Message) string {
	message = unwrapHistoryProtoMessage(message)
	if message == nil {
		return ""
	}
	protocolMessage := message.GetProtocolMessage()
	if protocolMessage == nil || protocolMessage.GetType() != waProto.ProtocolMessage_REVOKE {
		return ""
	}
	return strings.TrimSpace(protocolMessage.GetKey().GetID())
}

// revokedMessageID = ID pesan yang dihapus dari event live (revoke).
func revokedMessageID(event *events.Message) string {
	if event == nil {
		return ""
	}
	if messageID := protocolRevokeTargetID(event.Message); messageID != "" {
		return messageID
	}
	if messageID := protocolRevokeTargetID(event.RawMessage); messageID != "" {
		return messageID
	}
	if event.Info.Edit == types.EditAttributeSenderRevoke ||
		event.Info.Edit == types.EditAttributeAdminRevoke {
		return strings.TrimSpace(string(event.Info.MsgMetaInfo.TargetID))
	}
	return ""
}

// historyRevokedMessageID = ID pesan yang dihapus dari history sync (stub).
func historyRevokedMessageID(message *waWeb.WebMessageInfo) string {
	if message == nil {
		return ""
	}
	switch message.GetMessageStubType() {
	case waWeb.WebMessageInfo_REVOKE, waWeb.WebMessageInfo_ADMIN_REVOKE:
		return strings.TrimSpace(message.GetKey().GetID())
	default:
		return ""
	}
}

// historicalMediaLabel = label ringkas untuk media riwayat di Inbox.
func historicalMediaLabel(mediaType, fileName string) string {
	switch mediaType {
	case "image":
		return "📷 Foto dari riwayat WhatsApp"
	case "sticker":
		return "🌟 Stiker dari riwayat WhatsApp"
	case "video":
		return "🎥 Video dari riwayat WhatsApp"
	case "audio":
		return "🎵 Audio dari riwayat WhatsApp"
	case "document":
		if fileName != "" {
			return "📄 " + fileName
		}
		return "📄 Dokumen dari riwayat WhatsApp"
	default:
		return "Pesan dari riwayat WhatsApp"
	}
}

// isProtocolSystemNotification hanya memakai metadata protokol WhatsApp:
// UNKNOWN (0) = pesan biasa; stub non-zero = notifikasi/status protokol.
func isProtocolSystemNotification(msg *waWeb.WebMessageInfo) bool {
	return msg != nil && msg.GetMessageStubType() != waWeb.WebMessageInfo_UNKNOWN
}

// newestHistoryMessages memilih N pesan riwayat TERBARU (tie-break msg_order_id).
func newestHistoryMessages(messages []*waHistorySync.HistorySyncMsg, limit int) []*waHistorySync.HistorySyncMsg {
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	selected := append([]*waHistorySync.HistorySyncMsg(nil), messages...)
	sort.SliceStable(selected, func(i, j int) bool {
		left := selected[i].GetMessage().GetMessageTimestamp()
		right := selected[j].GetMessage().GetMessageTimestamp()
		if left == right {
			return selected[i].GetMsgOrderID() > selected[j].GetMsgOrderID()
		}
		return left > right
	})
	return selected[:limit]
}

// orderedLiveMessageTime memberi timestamp unik untuk pesan live yang tiba
// dalam DETIK yang sama (tie-break milidetik GLOBAL antar chat) agar urutan
// chat konsisten di daftar maupun di dalam percakapan.
var liveOrderMu sync.Mutex
var liveOrderLast time.Time

func (w *waInstance) orderedLiveMessageTime(chatKey string, base time.Time) time.Time {
	liveOrderMu.Lock()
	defer liveOrderMu.Unlock()
	if base.After(liveOrderLast) {
		liveOrderLast = base
		return base
	}
	liveOrderLast = liveOrderLast.Add(time.Millisecond)
	return liveOrderLast
}

// FormatGroupInboxText menambah nama pengirim di depan teks pesan grup.
// Nomor dinormalisasi ke format internasional bila nama kosong.
func FormatGroupInboxText(text, name, number string) string {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = "+" + NormalizePhone(number)
	}
	cleanText := strings.TrimSpace(text)
	if cleanText == "" {
		cleanText = "Pesan grup"
	}
	return cleanName + ": " + cleanText
}
