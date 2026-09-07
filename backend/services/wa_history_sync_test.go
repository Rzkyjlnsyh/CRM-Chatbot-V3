package services

import (
	"strings"
	"testing"
	"time"

	waCommon "go.mau.fi/whatsmeow/proto/waCommon"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	waWeb "go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestHistoricalMessageContentText(t *testing.T) {
	text, mediaType, _, _, replyTo, replyText := historicalMessageContent(&waProto.Message{
		ExtendedTextMessage: &waProto.ExtendedTextMessage{
			Text: proto.String("Halo dari history"),
			ContextInfo: &waProto.ContextInfo{
				StanzaID:      proto.String("quoted-id"),
				QuotedMessage: &waProto.Message{Conversation: proto.String("Source code")},
			},
		},
	})
	if text != "Halo dari history" || mediaType != "" || replyTo != "quoted-id" || replyText != "Source code" {
		t.Fatalf("hasil parse tidak sesuai: text=%q media=%q reply=%q preview=%q", text, mediaType, replyTo, replyText)
	}
}

func TestHistoricalMessageContentMediaWithoutDownload(t *testing.T) {
	text, mediaType, fileName, mimetype, _, _ := historicalMessageContent(&waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			Caption:  proto.String("Invoice Juli"),
			FileName: proto.String("invoice.pdf"),
			Mimetype: proto.String("application/pdf"),
		},
	})
	if text != "Invoice Juli" || mediaType != "document" || fileName != "invoice.pdf" || mimetype != "application/pdf" {
		t.Fatalf("metadata media tidak sesuai: %q %q %q %q", text, mediaType, fileName, mimetype)
	}
	if !strings.Contains(historicalMediaLabel(mediaType, fileName), "invoice.pdf") {
		t.Fatal("label dokumen harus menyertakan nama file")
	}
}

func TestProtocolSystemNotificationUsesStubMetadata(t *testing.T) {
	if isProtocolSystemNotification(&waWeb.WebMessageInfo{}) {
		t.Fatal("stub UNKNOWN harus dianggap pesan biasa")
	}
	stub := waWeb.WebMessageInfo_BIZ_NAME_CHANGE
	if !isProtocolSystemNotification(&waWeb.WebMessageInfo{MessageStubType: &stub}) {
		t.Fatal("stub protokol non-UNKNOWN harus dianggap notifikasi sistem")
	}
}

func TestRevokedMessageIDFromProtocolMessage(t *testing.T) {
	revokeType := waProto.ProtocolMessage_REVOKE
	event := &events.Message{Message: &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Type: &revokeType,
			Key:  &waCommon.MessageKey{ID: proto.String("wamid-deleted")},
		},
	}}
	if got := revokedMessageID(event); got != "wamid-deleted" {
		t.Fatalf("target revoke tidak terbaca, got %q", got)
	}
}

func TestHistoryRevokedMessageIDUsesOriginalStanzaID(t *testing.T) {
	stub := waWeb.WebMessageInfo_REVOKE
	message := &waWeb.WebMessageInfo{
		Key:             &waCommon.MessageKey{ID: proto.String("wamid-history-deleted")},
		MessageStubType: &stub,
	}
	if got := historyRevokedMessageID(message); got != "wamid-history-deleted" {
		t.Fatalf("target revoke history tidak terbaca, got %q", got)
	}
}

func TestExtractIncomingDoesNotFilterShortTextMatchingPushName(t *testing.T) {
	w := &waInstance{}
	in, ok := w.extractIncoming(&events.Message{
		Info:    types.MessageInfo{PushName: "ArifKlik"},
		Message: &waProto.Message{Conversation: proto.String("ArifKlik")},
	})
	if !ok || in.Text != "ArifKlik" {
		t.Fatalf("pesan valid terbuang: ok=%v text=%q", ok, in.Text)
	}
}

func TestExtractIncomingRejectsProtocolStub(t *testing.T) {
	stub := waWeb.WebMessageInfo_BIZ_NAME_CHANGE
	w := &waInstance{}
	_, ok := w.extractIncoming(&events.Message{
		Message:      &waProto.Message{Conversation: proto.String("ArifKlik")},
		SourceWebMsg: &waWeb.WebMessageInfo{MessageStubType: &stub},
	})
	if ok {
		t.Fatal("notifikasi stub protokol tidak boleh menjadi bubble chat")
	}
}

func TestExtractIncomingPreservesStickerWhenDownloadUnavailable(t *testing.T) {
	w := &waInstance{} // client nil mensimulasikan unduhan pertama gagal
	in, ok := w.extractIncoming(&events.Message{
		Message: &waProto.Message{StickerMessage: &waProto.StickerMessage{
			Mimetype: proto.String("image/webp"),
		}},
	})
	if !ok {
		t.Fatal("envelope stiker harus tetap diteruskan saat download gagal")
	}
	if in.MediaType != "sticker" || in.Mimetype != "image/webp" {
		t.Fatalf("metadata stiker tidak sesuai: type=%q mime=%q", in.MediaType, in.Mimetype)
	}
	if len(in.Data) != 0 || len(in.MediaMetadata) == 0 {
		t.Fatalf("data=%d metadata=%d; metadata lazy-download wajib tersedia", len(in.Data), len(in.MediaMetadata))
	}
	var decoded waProto.Message
	if err := proto.Unmarshal(in.MediaMetadata, &decoded); err != nil || decoded.GetStickerMessage() == nil {
		t.Fatalf("metadata stiker tidak dapat dipakai ulang: %v", err)
	}
}

func TestOrderedLiveMessageTimeBreaksSameSecondTies(t *testing.T) {
	w := &waInstance{}
	base := time.Date(2026, time.July, 28, 14, 16, 0, 0, time.UTC)

	first := w.orderedLiveMessageTime("6282261008855", base)
	second := w.orderedLiveMessageTime("6282261008855", base)
	otherChat := w.orderedLiveMessageTime("6285608483004", base)
	nextSecond := w.orderedLiveMessageTime("6282261008855", base.Add(time.Second))

	if !first.Equal(base) {
		t.Fatalf("first timestamp = %s, want %s", first, base)
	}
	if !second.Equal(base.Add(time.Millisecond)) {
		t.Fatalf("second timestamp = %s, want one millisecond after first", second)
	}
	if !otherChat.Equal(base.Add(2 * time.Millisecond)) {
		t.Fatalf("other chat must preserve global chat-list order, got %s", otherChat)
	}
	if !nextSecond.Equal(base.Add(time.Second)) {
		t.Fatalf("new WA second must reset tie-break, got %s", nextSecond)
	}
}

func TestNewestHistoryMessagesKeepsNewestWithinBound(t *testing.T) {
	message := func(timestamp, order uint64) *waHistorySync.HistorySyncMsg {
		return &waHistorySync.HistorySyncMsg{
			Message:    &waWeb.WebMessageInfo{MessageTimestamp: proto.Uint64(timestamp)},
			MsgOrderID: proto.Uint64(order),
		}
	}
	messages := []*waHistorySync.HistorySyncMsg{
		message(10, 1), message(30, 1), message(20, 2), message(20, 1),
	}
	got := newestHistoryMessages(messages, 3)
	if len(got) != 3 {
		t.Fatalf("jumlah pesan = %d, mau 3", len(got))
	}
	if got[0].GetMessage().GetMessageTimestamp() != 30 ||
		got[1].GetMsgOrderID() != 2 || got[2].GetMsgOrderID() != 1 {
		t.Fatalf("urutan pesan terbaru tidak sesuai: %+v", got)
	}
	// Helper tidak boleh mengubah urutan slice protobuf milik event asli.
	if messages[0].GetMessage().GetMessageTimestamp() != 10 {
		t.Fatal("slice HistorySync asli berubah")
	}
}

func TestFormatGroupInboxTextIncludesParticipant(t *testing.T) {
	if got := FormatGroupInboxText("Update selesai", "Budi", "628123"); got != "Budi: Update selesai" {
		t.Fatalf("label nama grup = %q", got)
	}
	if got := FormatGroupInboxText("", "", "08123"); got != "+628123: Pesan grup" {
		t.Fatalf("fallback nomor grup = %q", got)
	}
}
