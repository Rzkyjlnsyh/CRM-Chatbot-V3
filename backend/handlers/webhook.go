package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"go.mau.fi/whatsmeow/types"
)

// webhookPayload = bentuk JSON yang dikirim ke URL webhook tenant saat ada pesan masuk.
type webhookPayload struct {
	Event     string `json:"event"` // "message.received"
	AgentID   uint   `json:"agent_id"`
	Number    string `json:"number"`     // nomor kita (agent/CS)
	From      string `json:"from"`       // nomor pengirim
	Name      string `json:"name"`       // nama profil pengirim
	Type      string `json:"type"`       // text | image | document | audio | video | sticker
	Text      string `json:"text"`       // teks / caption
	MediaType string `json:"media_type"` // "" untuk teks
	MessageID string `json:"message_id"` // ID pesan WhatsApp
	Timestamp int64  `json:"timestamp"`  // unix detik
}

type imageAnalysisWebhookPayload struct {
	Event      string  `json:"event"` // "image.analyzed"
	AgentID    uint    `json:"agent_id"`
	Number     string  `json:"number"`
	From       string  `json:"from"`
	Name       string  `json:"name"`
	MessageID  string  `json:"message_id"`
	Status     string  `json:"status"`
	Analysis   string  `json:"analysis"`
	Answer     string  `json:"answer,omitempty"`
	ProductID  uint    `json:"product_id,omitempty"`
	Confidence float64 `json:"confidence"`
	NeedsHuman bool    `json:"needs_human"`
	Model      string  `json:"model"`
	Timestamp  int64   `json:"timestamp"`
}

var webhookClient = &http.Client{Timeout: 10 * time.Second}

// dispatchIncomingWebhook mengirim notifikasi pesan masuk ke webhook agent (bila diset),
// asinkron & dengan retry ringan — tidak memblokir alur balasan AI/menu. Seluruh kerja
// (termasuk lookup DB) dilakukan di goroutine; hanya field skalar yang ditangkap supaya
// byte media (in.Data) tidak tertahan hidup selama proses webhook.
func dispatchIncomingWebhook(agentID uint, sender types.JID, in services.IncomingMessage) {
	from, name, text, mtype, msgID := sender.User, in.PushName, in.Text, in.MediaType, in.WAMsgID
	services.Go("webhook", func() {
		// Media disimpan oleh processMessage pada goroutine utama. Tunggu sampai barisnya siap
		// agar integrasi bisa langsung memanggil /messages/:message_id/media dari event ini.
		if mtype != "" && msgID != "" {
			for attempt := 0; attempt < 20; attempt++ {
				var count int64
				database.DB.Model(&models.ChatHistory{}).
					Where("agent_id = ? AND wa_msg_id = ? AND media_path <> ''", agentID, msgID).Count(&count)
				if count > 0 {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
		var a models.Agent
		if database.DB.Select("id", "number", "webhook_url", "webhook_secret").First(&a, agentID).Error != nil || a.WebhookURL == "" {
			return
		}
		typ := "text"
		if mtype != "" {
			typ = mtype
		}
		body, err := json.Marshal(webhookPayload{
			Event:     "message.received",
			AgentID:   a.ID,
			Number:    a.Number,
			From:      from,
			Name:      name,
			Type:      typ,
			Text:      text,
			MediaType: mtype,
			MessageID: msgID,
			Timestamp: time.Now().Unix(),
		})
		if err != nil {
			return
		}
		postWebhookWithRetry(a.WebhookURL, a.WebhookSecret, body)
	})
}

func dispatchImageAnalysisWebhook(agentID uint, sender types.JID, in services.IncomingMessage, analysis, status, model string, confidence float64, needsHuman bool, answer string, productID uint) {
	from, name, messageID := sender.User, in.PushName, in.WAMsgID
	services.Go("webhook-image-analysis", func() {
		var agent models.Agent
		if database.DB.Select("id", "number", "webhook_url", "webhook_secret").First(&agent, agentID).Error != nil || agent.WebhookURL == "" {
			return
		}
		body, err := json.Marshal(imageAnalysisWebhookPayload{
			Event: "image.analyzed", AgentID: agent.ID, Number: agent.Number,
			From: from, Name: name, MessageID: messageID, Status: status,
			Analysis: analysis, Answer: answer, ProductID: productID,
			Confidence: confidence, NeedsHuman: needsHuman, Model: model, Timestamp: time.Now().Unix(),
		})
		if err == nil {
			postWebhookWithRetry(agent.WebhookURL, agent.WebhookSecret, body)
		}
	})
}

func dispatchStoredImageAnalysisWebhook(agentID uint, row models.ChatHistory, name, status string, result services.VisionAnalysisResult, needsHuman bool) {
	in := services.IncomingMessage{PushName: name, WAMsgID: row.WAMsgID}
	dispatchImageAnalysisWebhook(agentID, types.NewJID(row.Sender, types.DefaultUserServer), in,
		result.Analysis, status, result.Model, result.Confidence, needsHuman, result.Answer, result.ProductID)
}

// webhookStatusPayload = event "message.status" (delivered/read/played) untuk pesan keluar.
type webhookStatusPayload struct {
	Event      string   `json:"event"` // "message.status"
	AgentID    uint     `json:"agent_id"`
	Number     string   `json:"number"`      // nomor kita (agent/CS)
	To         string   `json:"to"`          // nomor penerima yang menerima/membaca
	Status     string   `json:"status"`      // delivered | read | played
	MessageIDs []string `json:"message_ids"` // ID pesan yang berubah status
	Timestamp  int64    `json:"timestamp"`
}

// OnWAReceipt dipanggil services WA saat ada receipt; kirim webhook "message.status" bila diset.
func OnWAReceipt(agentID uint, m services.ReceiptMeta) {
	services.Go("webhook-status", func() {
		var a models.Agent
		if database.DB.Select("id", "number", "webhook_url", "webhook_secret").First(&a, agentID).Error != nil || a.WebhookURL == "" {
			return
		}
		body, err := json.Marshal(webhookStatusPayload{
			Event:      "message.status",
			AgentID:    a.ID,
			Number:     a.Number,
			To:         m.Recipient,
			Status:     m.Status,
			MessageIDs: m.MessageIDs,
			Timestamp:  m.Timestamp,
		})
		if err != nil {
			return
		}
		postWebhookWithRetry(a.WebhookURL, a.WebhookSecret, body)
	})
}

// postWebhookWithRetry mengirim POST bertanda tangan HMAC-SHA256 (header X-Signature),
// mengulang beberapa kali dengan jeda bila endpoint tenant gagal/nonaktif sesaat.
func postWebhookWithRetry(url, secret string, body []byte) {
	delays := []time.Duration{0, 2 * time.Second, 5 * time.Second}
	for attempt, d := range delays {
		if d > 0 {
			time.Sleep(d)
		}
		req, err := newSignedWebhookRequest(url, secret, bytes.NewReader(body))
		if err != nil {
			return
		}
		resp, err := webhookClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return
			}
		}
		if attempt == len(delays)-1 {
			log.Printf("Webhook gagal terkirim ke %s setelah %d percobaan", url, len(delays))
		}
	}
}

func newSignedWebhookRequest(url, secret string, body io.Reader) (*http.Request, error) {
	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SlaluDiskon-Webhook/1")
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payload)
		req.Header.Set("X-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	return req, nil
}
