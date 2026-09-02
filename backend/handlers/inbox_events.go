package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────────────────
// Inbox realtime (pola v4) — hub event lokal + SSE. Fork klien single-binary,
// jadi tidak perlu proxy api→worker: semua di satu proses.
// Event ringan: {revision, kind, sender, message_id}.
// ─────────────────────────────────────────────────────────────────────────

type InboxEvent struct {
	Revision  int64  `json:"revision"`
	Kind      string `json:"kind"` // new_message | read_state | typing
	Sender    string `json:"sender,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

type inboxSubscriber struct {
	ch chan InboxEvent
}

type inboxHub struct {
	mu          sync.Mutex
	revisions   map[uint]int64
	subscribers map[uint][]inboxSubscriber
}

var hub = &inboxHub{
	revisions:   map[uint]int64{},
	subscribers: map[uint][]inboxSubscriber{},
}

const inboxHubBuffer = 64

// PublishInboxEvent menyiarkan event ke semua browser yang membuka agent ini.
func PublishInboxEvent(agentID uint, kind, sender, messageID string) {
	hub.mu.Lock()
	hub.revisions[agentID]++
	ev := InboxEvent{Revision: hub.revisions[agentID], Kind: kind, Sender: sender, MessageID: messageID}
	for _, sub := range hub.subscribers[agentID] {
		select {
		case sub.ch <- ev:
		default: // browser lambat — lewati frame (SSE replay menutup gap)
		}
	}
	hub.mu.Unlock()
}

func (h *inboxHub) subscribe(agentID uint) (inboxSubscriber, int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	sub := inboxSubscriber{ch: make(chan InboxEvent, inboxHubBuffer)}
	h.subscribers[agentID] = append(h.subscribers[agentID], sub)
	return sub, h.revisions[agentID]
}

func (h *inboxHub) unsubscribe(agentID uint, sub inboxSubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	list := h.subscribers[agentID]
	for i, s := range list {
		if s.ch == sub.ch {
			h.subscribers[agentID] = append(list[:i], list[i+1:]...)
			break
		}
	}
}

// InboxEvents — GET /agents/:id/inbox/events (SSE)
// Frame pertama: "ready" dengan revision saat ini; selanjutnya event live.
func InboxEvents(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	sub, currentRev := hub.subscribe(id)
	defer hub.unsubscribe(id, sub)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Writer.Flush()

	writeFrame := func(event string, data []byte) bool {
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, data); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}

	ready, _ := json.Marshal(gin.H{"revision": currentRev})
	if !writeFrame("ready", ready) {
		return
	}

	// Heartbeat tiap 25 detik agar koneksi tidak diputus proxy/VPS.
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case ev := <-sub.ch:
			data, _ := json.Marshal(ev)
			if !writeFrame("event", data) {
				return
			}
		case <-heartbeat.C:
			if !writeFrame("ping", []byte(`{"t":`+strconv.FormatInt(time.Now().Unix(), 10)+`}`)) {
				return
			}
		case <-c.Request.Context().Done():
			return
		}
	}
}

// InboxClientDebug — POST /agents/:id/inbox/client-debug (klien melaporkan
// kondisi realtime-nya; tersimpan ringkas untuk diagnosa).
var clientDebugLog = map[uint][]gin.H{}
var clientDebugMu sync.Mutex

func InboxClientDebug(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var payload map[string]any
	_ = c.ShouldBindJSON(&payload)
	payload["agent_id"] = id
	payload["at"] = time.Now().Format(time.RFC3339)
	clientDebugMu.Lock()
	clientDebugLog[id] = append([]gin.H{payload}, clientDebugLog[id]...)
	if len(clientDebugLog[id]) > 50 {
		clientDebugLog[id] = clientDebugLog[id][:50]
	}
	clientDebugMu.Unlock()
	log.Printf("[inbox-debug] agent=%d: %v", id, payload)
	c.JSON(200, gin.H{"message": "logged"})
}

func InboxClientDebugDump(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	clientDebugMu.Lock()
	entries := clientDebugLog[id]
	clientDebugMu.Unlock()
	c.JSON(200, gin.H{"entries": entries})
}
