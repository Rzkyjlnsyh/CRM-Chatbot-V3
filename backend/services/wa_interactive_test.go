package services

import (
	"testing"

	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestInteractiveReplyTextNativeFlowDisplayText(t *testing.T) {
	msg := &waProto.Message{
		InteractiveResponseMessage: &waProto.InteractiveResponseMessage{
			InteractiveResponseMessage: &waProto.InteractiveResponseMessage_NativeFlowResponseMessage_{
				NativeFlowResponseMessage: &waProto.InteractiveResponseMessage_NativeFlowResponseMessage{
					ParamsJSON: proto.String(`{"id":"product_order","display_text":"Pesan Sekarang"}`),
				},
			},
		},
	}

	text, actionID, _, ok := interactiveReplyText(msg)
	if !ok || text != "Pesan Sekarang" {
		t.Fatalf("respons tombol tidak terbaca: ok=%v text=%q", ok, text)
	}
	if actionID != "product_order" {
		t.Fatalf("action ID tombol tidak terbaca: %q", actionID)
	}
}

func TestInteractiveReplyTextNativeFlowIDFallback(t *testing.T) {
	msg := &waProto.Message{
		InteractiveResponseMessage: &waProto.InteractiveResponseMessage{
			InteractiveResponseMessage: &waProto.InteractiveResponseMessage_NativeFlowResponseMessage_{
				NativeFlowResponseMessage: &waProto.InteractiveResponseMessage_NativeFlowResponseMessage{
					ParamsJSON: proto.String(`{"id":"product_question"}`),
				},
			},
		},
	}

	text, actionID, _, ok := interactiveReplyText(msg)
	if !ok || text != "Tanya Detail" {
		t.Fatalf("ID tombol tidak dipetakan: ok=%v text=%q", ok, text)
	}
	if actionID != "product_question" {
		t.Fatalf("action ID tombol tidak terbaca: %q", actionID)
	}
}

func TestInteractiveReplyTextLegacyButton(t *testing.T) {
	msg := &waProto.Message{
		ButtonsResponseMessage: &waProto.ButtonsResponseMessage{
			Response: &waProto.ButtonsResponseMessage_SelectedDisplayText{SelectedDisplayText: "Pesan Sekarang"},
		},
	}

	text, _, _, ok := interactiveReplyText(msg)
	if !ok || text != "Pesan Sekarang" {
		t.Fatalf("respons tombol lama tidak terbaca: ok=%v text=%q", ok, text)
	}
}

func TestNativeFlowBizNodes(t *testing.T) {
	nodes := nativeFlowBizNodes()
	if len(nodes) != 1 || nodes[0].Tag != "biz" {
		t.Fatalf("node bisnis native flow tidak valid: %+v", nodes)
	}
	interactive := nodes[0].GetChildrenByTag("interactive")
	if len(interactive) != 1 || interactive[0].Attrs["type"] != "native_flow" || interactive[0].Attrs["v"] != "1" {
		t.Fatalf("metadata interactive tidak valid: %+v", interactive)
	}
	nativeFlow := interactive[0].GetChildrenByTag("native_flow")
	if len(nativeFlow) != 1 || nativeFlow[0].Attrs["name"] != "mixed" || nativeFlow[0].Attrs["v"] != "9" {
		t.Fatalf("metadata native flow tidak valid: %+v", nativeFlow)
	}
}
