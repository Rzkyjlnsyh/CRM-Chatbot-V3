package handlers

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"gorm.io/gorm"
)

// ── Rotasi nomor (failover cerdas) ──────────────────────────────────────────
//
// Algoritma:
//  1. Sticky hash — satu penerima menempel ke satu nomor selama nomor itu sehat.
//  2. Health-aware pool — hanya nomor tersambung & tidak dalam karantina keras.
//  3. Karantina bertingkat:
//       hard  (463/ban)      → tidak dipakai lagi di run ini kecuali last-resort resume
//       soft  (429/rate)     → cooldown, boleh kembali setelah expired
//       disconnect           → boleh kembali saat tersambung lagi
//  4. Failover load-aware — sisa antrean dipindah ke nomor sehat dengan beban pending
//     terkecil; seri diputus sticky agar stabil.
//  5. Circuit breaker — N gagal sistemik beruntun → soft-quarantine (bukan spam failed).
//  6. Adaptive delay — setelah soft-restrict, jeda agent naik sementara.
//  7. State karantina dipersist (QuarantineJSON) agar resume tidak memukul nomor berisiko dulu.

const (
	quarantineHard       = "hard"
	quarantineSoft       = "soft"
	quarantineDisconnect = "disconnect"

	softQuarantineTTL  = 10 * time.Minute
	disconnectCooldownLimit = 45 * time.Second
	// Gagal sistemik beruntun sebelum soft-quarantine (bukan invalid recipient).
	circuitBreakerThreshold = 4
	// Multiplier jeda setelah soft-restrict (1.0 = normal).
	softRestrictDelayBoost = 1.35
)

// ── Ringkasan rotasi untuk API detail ───────────────────────────────────────

// quarantineReasonLabel = label manusiawi untuk UI.
func quarantineReasonLabel(reason string, code int) string {
	switch reason {
	case quarantineHard:
		if code == 463 {
			return "Dibatasi WhatsApp (anti-spam)"
		}
		if code == 401 || code == 403 {
			return "Akses ditolak WhatsApp"
		}
		if code != 0 {
			return fmt.Sprintf("Dibatasi WhatsApp (kode %d)", code)
		}
		return "Dibatasi / berisiko (karantina keras)"
	case quarantineSoft:
		if code == 429 {
			return "Rate limit sementara"
		}
		return "Tidak stabil sementara (soft)"
	case quarantineDisconnect:
		return "Terputus / offline"
	default:
		if reason == "" {
			return "Tidak diketahui"
		}
		return reason
	}
}

func quarantineAdvice(reason string, code int) string {
	switch reason {
	case quarantineHard:
		if code == 463 {
			return "Istirahatkan nomor 1–3 hari, hangatkan dengan chat dua arah, lalu blast kecil ke kontak yang pernah membalas."
		}
		return "Jangan paksa kirim massal dari nomor ini dulu. Periksa sesi WhatsApp dan kesehatan akun."
	case quarantineSoft:
		return "Tunggu cooldown, naikkan jeda antar pesan, dan kurangi volume harian."
	case quarantineDisconnect:
		return "Pastikan WhatsApp tersambung (scan QR / pairing) lalu lanjutkan Blast."
	default:
		return "Pantau nomor ini; failover akan memakai nomor cadangan yang sehat."
	}
}

func isQuarantineActive(e quarantineEntry, now time.Time) bool {
	if e.Reason == quarantineHard {
		return true
	}
	if e.Until.IsZero() {
		return true
	}
	return now.Before(e.Until) || now.Equal(e.Until)
}

func parseStoredQuarantine(raw string) map[uint]quarantineEntry {
	out := map[uint]quarantineEntry{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return out
	}
	var stored map[string]quarantineEntry
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return out
	}
	for k, e := range stored {
		id, err := strconv.ParseUint(k, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		if e.Reason == "" {
			e.Reason = quarantineHard
		}
		out[uint(id)] = e
	}
	return out
}

// buildBroadcastRotationSummary menyusun data pool + karantina untuk detail Blast.
// Aman dipanggil tanpa multi-rotasi (pool 1 nomor).
func buildBroadcastRotationSummary(b models.Broadcast, recipients []models.BroadcastRecipient) map[string]any {
	pool := parseAgentPool(b)
	now := time.Now()
	qMap := parseStoredQuarantine(b.QuarantineJSON)

	// Jika tidak ada JSON karantina tapi ada sinyal pause WA, sintesis entri untuk nomor utama.
	if len(qMap) == 0 && (b.PauseCode != 0 || b.PauseReason == "wa_restriction" || b.Status == models.BroadcastWARestricted) {
		reason := quarantineHard
		if b.PauseCode == 429 {
			reason = quarantineSoft
		}
		entry := quarantineEntry{Reason: reason, Code: b.PauseCode, At: time.Now()}
		if b.PausedAt != nil {
			entry.At = *b.PausedAt
		}
		if reason == quarantineSoft {
			entry.Until = entry.At.Add(softQuarantineTTL)
		}
		qMap[b.AgentID] = entry
	}

	// Agregat hitungan per agent dari penerima.
	type tallies struct{ pending, sent, failed, skipped int }
	byAgent := map[uint]*tallies{}
	ensure := func(id uint) *tallies {
		if t, ok := byAgent[id]; ok {
			return t
		}
		t := &tallies{}
		byAgent[id] = t
		return t
	}
	for _, r := range recipients {
		aid := r.AgentID
		if aid == 0 {
			aid = b.AgentID
		}
		t := ensure(aid)
		switch r.Status {
		case "sent":
			t.sent++
		case "failed":
			t.failed++
		case "skipped":
			t.skipped++
		default:
			t.pending++
		}
	}

	// Muat metadata agent (nama/nomor).
	agentMeta := map[uint]models.Agent{}
	if len(pool) > 0 {
		var agents []models.Agent
		database.DB.Select("id", "name", "number").Where("id IN ?", pool).Find(&agents)
		for _, a := range agents {
			agentMeta[a.ID] = a
		}
	}

	agentsOut := make([]map[string]any, 0, len(pool))
	quarantineOut := make([]map[string]any, 0)
	activeQuarantine := 0

	for i, aid := range pool {
		meta := agentMeta[aid]
		name := meta.Name
		if name == "" {
			name = fmt.Sprintf("Nomor %d", aid)
		}
		role := "backup"
		if i == 0 {
			role = "primary"
		}
		t := ensure(aid)
		row := map[string]any{
			"id":            aid,
			"name":          name,
			"number":        meta.Number,
			"role":          role,
			"connected":     services.WA(aid).IsConnected(),
			"pending_count": t.pending,
			"sent_count":    t.sent,
			"failed_count":  t.failed,
			"skipped_count": t.skipped,
		}
		if e, ok := qMap[aid]; ok {
			active := isQuarantineActive(e, now)
			if active {
				activeQuarantine++
			}
			var until any
			if !e.Until.IsZero() {
				until = e.Until.UTC().Format(time.RFC3339)
			}
			qView := map[string]any{
				"reason":       e.Reason,
				"reason_label": quarantineReasonLabel(e.Reason, e.Code),
				"code":         e.Code,
				"at":           e.At.UTC().Format(time.RFC3339),
				"until":        until,
				"active":       active,
				"advice":       quarantineAdvice(e.Reason, e.Code),
			}
			row["quarantine"] = qView
			qEntry := map[string]any{
				"agent_id":      aid,
				"name":          name,
				"number":        meta.Number,
				"role":          role,
				"reason":        e.Reason,
				"reason_label":  quarantineReasonLabel(e.Reason, e.Code),
				"code":          e.Code,
				"at":            e.At.UTC().Format(time.RFC3339),
				"until":         until,
				"active":        active,
				"advice":        quarantineAdvice(e.Reason, e.Code),
				"pending_count": t.pending,
				"sent_count":    t.sent,
			}
			quarantineOut = append(quarantineOut, qEntry)
		}
		agentsOut = append(agentsOut, row)
	}

	// Sort quarantine: active hard first, then soft, then inactive.
	sort.SliceStable(quarantineOut, func(i, j int) bool {
		ai, _ := quarantineOut[i]["active"].(bool)
		aj, _ := quarantineOut[j]["active"].(bool)
		if ai != aj {
			return ai && !aj
		}
		ri, _ := quarantineOut[i]["reason"].(string)
		rj, _ := quarantineOut[j]["reason"].(string)
		rank := map[string]int{quarantineHard: 0, quarantineSoft: 1, quarantineDisconnect: 2}
		return rank[ri] < rank[rj]
	})

	return map[string]any{
		"enabled":           len(pool) > 1,
		"pool_size":         len(pool),
		"agents":            agentsOut,
		"quarantine":        quarantineOut,
		"quarantine_active": activeQuarantine,
		"pause_code":        b.PauseCode,
		"pause_reason":      b.PauseReason,
	}
}

// parseAgentPool mengembalikan daftar nomor yang ikut broadcast.
func parseAgentPool(b models.Broadcast) []uint {
	var raw []uint
	if strings.TrimSpace(b.AgentIDs) != "" {
		_ = json.Unmarshal([]byte(b.AgentIDs), &raw)
	}
	seen := map[uint]bool{}
	pool := make([]uint, 0, len(raw))
	for _, a := range raw {
		if a == 0 || seen[a] {
			continue
		}
		seen[a] = true
		pool = append(pool, a)
	}
	if len(pool) == 0 {
		return []uint{b.AgentID}
	}
	return pool
}

// anyAgentConnected = true bila minimal satu nomor pool tersambung.
func anyAgentConnected(pool []uint) bool {
	for _, a := range pool {
		if services.WA(a).IsConnected() {
			return true
		}
	}
	return false
}

// stickyAgent memilih nomor untuk sebuah penerima secara deterministik.
func stickyAgent(number string, pool []uint) uint {
	if len(pool) == 0 {
		return 0
	}
	if len(pool) == 1 {
		return pool[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(number))
	return pool[int(h.Sum32())%len(pool)]
}

// pickFailoverAgent memilih nomor failover: beban pending terkecil, seri → sticky.
// pendingLoad di-update caller setelah assignment agar batch rebalance merata.
func pickFailoverAgent(number string, healthy []uint, pendingLoad map[uint]int) uint {
	if len(healthy) == 0 {
		return 0
	}
	if len(healthy) == 1 {
		return healthy[0]
	}
	minLoad := math.MaxInt
	for _, a := range healthy {
		if pendingLoad[a] < minLoad {
			minLoad = pendingLoad[a]
		}
	}
	// Prefer sticky di antara nomor dengan beban min; biarkan ±0 (strict min) agar
	// beban merata tanpa loncat ke nomor yang sudah jauh lebih penuh.
	candidates := make([]uint, 0, len(healthy))
	for _, a := range healthy {
		if pendingLoad[a] == minLoad {
			candidates = append(candidates, a)
		}
	}
	return stickyAgent(number, candidates)
}

// ── Campaign state ──────────────────────────────────────────────────────────

type quarantineEntry struct {
	Reason string    `json:"reason"` // hard | soft | disconnect
	Code   int       `json:"code,omitempty"`
	At     time.Time `json:"at"`
	Until  time.Time `json:"until"` // zero = selamanya (hard)
}

type broadcastCampaign struct {
	mu            sync.Mutex
	quarantined   map[uint]quarantineEntry
	pauseCode     int
	cancelled     bool
	resumeMode    bool // true saat dilanjutkan user setelah wa_restricted
	delayBoost    map[uint]float64
	persistTarget uint // broadcast id untuk persist
}

func newBroadcastCampaign(broadcastID uint) *broadcastCampaign {
	return &broadcastCampaign{
		quarantined:   map[uint]quarantineEntry{},
		delayBoost:    map[uint]float64{},
		persistTarget: broadcastID,
	}
}

func loadCampaignFromBroadcast(b models.Broadcast) *broadcastCampaign {
	c := newBroadcastCampaign(b.ID)
	if b.Status == models.BroadcastResuming || b.Status == models.BroadcastWARestricted {
		c.resumeMode = true
	}
	// Saat claim runBroadcast sudah set status running; deteksi resume lewat quarantine terisi
	// atau pause_code yang baru dibersihkan — resumeMode di-set dari caller.
	raw := strings.TrimSpace(b.QuarantineJSON)
	if raw == "" {
		return c
	}
	var stored map[string]quarantineEntry
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return c
	}
	now := time.Now()
	for k, e := range stored {
		id, err := strconv.ParseUint(k, 10, 64)
		if err != nil || id == 0 {
			continue
		}
		// Soft/disconnect yang sudah lewat tidak dimuat lagi.
		if e.Reason != quarantineHard && !e.Until.IsZero() && now.After(e.Until) {
			continue
		}
		c.quarantined[uint(id)] = e
		if e.Reason == quarantineSoft || e.Reason == quarantineHard {
			c.delayBoost[uint(id)] = softRestrictDelayBoost
		}
		if e.Code != 0 {
			c.pauseCode = e.Code
		}
	}
	return c
}

func (c *broadcastCampaign) setResumeMode(v bool) {
	c.mu.Lock()
	c.resumeMode = v
	c.mu.Unlock()
}

func (c *broadcastCampaign) quarantine(agentID uint, reason string, code int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	entry := quarantineEntry{Reason: reason, Code: code, At: now}
	switch reason {
	case quarantineHard:
		// Selamanya untuk run ini (Until zero).
		entry.Until = time.Time{}
		c.delayBoost[agentID] = softRestrictDelayBoost
	case quarantineSoft:
		entry.Until = now.Add(softQuarantineTTL)
		c.delayBoost[agentID] = softRestrictDelayBoost
	default:
		entry.Reason = quarantineDisconnect
		entry.Until = now.Add(disconnectCooldownLimit)
	}
	// Hard menang dari soft/disconnect yang sudah ada.
	if prev, ok := c.quarantined[agentID]; ok && prev.Reason == quarantineHard && reason != quarantineHard {
		return
	}
	c.quarantined[agentID] = entry
	if code != 0 {
		c.pauseCode = code
	}
	c.persistLocked()
}

func (c *broadcastCampaign) clearQuarantine(agentID uint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.quarantined, agentID)
	c.persistLocked()
}

func (c *broadcastCampaign) isHardQuarantined(agentID uint) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.quarantined[agentID]
	return ok && e.Reason == quarantineHard
}

func (c *broadcastCampaign) quarantinedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.quarantined)
}

func (c *broadcastCampaign) setPause(code int) {
	c.mu.Lock()
	if code != 0 {
		c.pauseCode = code
	}
	c.mu.Unlock()
}

func (c *broadcastCampaign) getPause() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pauseCode
}

// hasRestrictionSignal true bila ada karantina hard/soft (bukan cuma disconnect).
func (c *broadcastCampaign) hasRestrictionSignal() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pauseCode != 0 {
		return true
	}
	for _, e := range c.quarantined {
		if e.Reason == quarantineHard || e.Reason == quarantineSoft {
			return true
		}
	}
	return false
}

func (c *broadcastCampaign) setCancelled() {
	c.mu.Lock()
	c.cancelled = true
	c.mu.Unlock()
}

func (c *broadcastCampaign) isCancelled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cancelled
}

func (c *broadcastCampaign) agentDelayBoost(agentID uint) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if b := c.delayBoost[agentID]; b > 1 {
		return b
	}
	return 1
}

// selectHealthyAgents memilih nomor yang boleh mengirim di putaran ini.
// Urutan preferensi: sehat murni → soft expired/recovered → (last-resort) hard saat resume.
func (c *broadcastCampaign) selectHealthyAgents(pool []uint) []uint {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()

	var preferred, softReady, hardConnected []uint
	for _, a := range pool {
		connected := services.WA(a).IsConnected()
		e, qOK := c.quarantined[a]

		if !qOK {
			if connected {
				preferred = append(preferred, a)
			} else {
				// Offline: soft-mark disconnect agar putaran berikutnya bisa pulih.
				c.quarantined[a] = quarantineEntry{
					Reason: quarantineDisconnect, At: now, Until: now.Add(disconnectCooldownLimit),
				}
			}
			continue
		}

		switch e.Reason {
		case quarantineDisconnect:
			if connected && (e.Until.IsZero() || now.After(e.Until) || now.After(e.At.Add(disconnectCooldownLimit))) {
				delete(c.quarantined, a)
				preferred = append(preferred, a)
			}
		case quarantineSoft:
			if connected && !e.Until.IsZero() && now.After(e.Until) {
				delete(c.quarantined, a)
				softReady = append(softReady, a)
			}
		case quarantineHard:
			if connected {
				hardConnected = append(hardConnected, a)
			}
		}
	}
	c.persistLocked()

	if len(preferred) > 0 {
		return preferred
	}
	if len(softReady) > 0 {
		return softReady
	}
	// Last-resort: hanya saat user menekan resume, biarkan hard dicoba lagi
	// agar tidak stuck wa_restricted selamanya tanpa opsi.
	if c.resumeMode && len(hardConnected) > 0 {
		for _, a := range hardConnected {
			delete(c.quarantined, a)
			c.delayBoost[a] = softRestrictDelayBoost * 1.15
		}
		c.persistLocked()
		log.Printf("Broadcast %d: last-resort resume memakai %d nomor yang sebelumnya hard-quarantine", c.persistTarget, len(hardConnected))
		// Resume last-resort hanya sekali per campaign run.
		c.resumeMode = false
		return hardConnected
	}
	return nil
}

func (c *broadcastCampaign) persistLocked() {
	if c.persistTarget == 0 {
		return
	}
	out := map[string]quarantineEntry{}
	for id, e := range c.quarantined {
		out[strconv.FormatUint(uint64(id), 10)] = e
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return
	}
	// Non-blocking-ish: update DB; ignore error agar worker tidak mati.
	_ = database.DB.Model(&models.Broadcast{}).Where("id = ?", c.persistTarget).
		Update("quarantine_json", string(raw)).Error
}

// ── Assignment ──────────────────────────────────────────────────────────────

// assignRecipientsToPool memberi agent_id ke penerima pending via sticky assignment.
func assignRecipientsToPool(broadcastID uint, pool []uint) {
	poolSet := map[uint]bool{}
	for _, a := range pool {
		poolSet[a] = true
	}
	var recs []models.BroadcastRecipient
	database.DB.Where("broadcast_id = ? AND status = ?", broadcastID, "pending").Find(&recs)
	for _, r := range recs {
		if r.AgentID == 0 || !poolSet[r.AgentID] {
			database.DB.Model(&models.BroadcastRecipient{}).Where("id = ?", r.ID).
				Update("agent_id", stickyAgent(r.Number, pool))
		}
	}
}

// reassignPendingToHealthy memindahkan penerima pending ke nomor sehat (load-aware + sticky).
func reassignPendingToHealthy(broadcastID uint, healthy []uint) {
	if len(healthy) == 0 {
		return
	}
	healthySet := map[uint]bool{}
	pendingLoad := map[uint]int{}
	for _, a := range healthy {
		healthySet[a] = true
		var n int64
		database.DB.Model(&models.BroadcastRecipient{}).
			Where("broadcast_id = ? AND agent_id = ? AND status = ?", broadcastID, a, "pending").Count(&n)
		pendingLoad[a] = int(n)
	}

	var recs []models.BroadcastRecipient
	database.DB.Where("broadcast_id = ? AND status = ?", broadcastID, "pending").
		Order("id asc").Find(&recs)

	// Proses yang butuh pindah dulu; yang sudah di healthy tetap (sticky).
	type move struct {
		id     uint
		number string
	}
	var toMove []move
	for _, r := range recs {
		if !healthySet[r.AgentID] {
			toMove = append(toMove, move{id: r.ID, number: r.Number})
		}
	}
	// Acak ringan agar hash sticky tidak selalu mengisi nomor yang sama dulu
	// saat beban awal 0 — tetap deterministik per number via pickFailoverAgent.
	// Sort by number hash for stable batch order.
	sort.SliceStable(toMove, func(i, j int) bool {
		return toMove[i].number < toMove[j].number
	})

	for _, m := range toMove {
		dest := pickFailoverAgent(m.number, healthy, pendingLoad)
		if dest == 0 {
			continue
		}
		database.DB.Model(&models.BroadcastRecipient{}).Where("id = ?", m.id).
			Update("agent_id", dest)
		pendingLoad[dest]++
	}
}

// ── Worker per nomor ────────────────────────────────────────────────────────

// runBroadcastAgentWorker mengirim penerima yang ditugaskan ke SATU nomor.
func runBroadcastAgentWorker(broadcastID, agentID uint, b models.Broadcast, minD, maxD int, campaign *broadcastCampaign) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Broadcast %d worker nomor %d PANIC dipulihkan: %v\n%s", broadcastID, agentID, r, debug.Stack())
			campaign.quarantine(agentID, quarantineHard, 0)
		}
	}()
	agentLock := broadcastAgentLock(agentID)
	agentLock.Lock()
	defer agentLock.Unlock()

	// Hard-quarantine bisa terjadi setelah selectHealthy (race); jangan kirim.
	if campaign.isHardQuarantined(agentID) {
		return
	}

	var recipients []models.BroadcastRecipient
	database.DB.Where("broadcast_id = ? AND agent_id = ? AND status = ?", broadcastID, agentID, "pending").
		Order("id asc").Find(&recipients)
	if len(recipients) == 0 {
		return
	}

	isGroupBroadcast := b.TargetType == "group"

	var mediaBytes []byte
	if b.MediaType != "" && b.MediaPath != "" {
		mediaBytes, _ = os.ReadFile(b.MediaPath)
	}
	var prepared *services.PreparedMedia
	if len(mediaBytes) > 0 {
		if pm, err := services.WA(agentID).PrepareMedia(b.MediaType, b.Mimetype, b.FileName, mediaBytes); err != nil {
			log.Printf("Broadcast %d nomor %d: upload media sekali gagal (%v), fallback per penerima", broadcastID, agentID, err)
		} else {
			prepared = pm
			mediaBytes = nil
		}
	}

	restEvery, restDuration := normalizeBroadcastRest(b.RestEvery, b.RestDuration)
	sentSinceRest := 0
	consecutiveSystemic := 0

	recipientNumbers := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		recipientNumbers = append(recipientNumbers, recipient.Number)
	}
	optedOut := optedOutSet(agentID)
	consented := activeConsentSet(agentID, b.ConsentCategory, recipientNumbers)

	boost := campaign.agentDelayBoost(agentID)

	for i, r := range recipients {
		if isBroadcastCancelRequested(broadcastID) {
			campaign.setCancelled()
			return
		}
		if !waitConnected(agentID, 60*time.Second) {
			campaign.quarantine(agentID, quarantineDisconnect, 0)
			log.Printf("Broadcast %d nomor %d terputus — penerimanya dialihkan ke nomor lain bila ada", broadcastID, agentID)
			return
		}
		if i > 0 && i%25 == 0 {
			optedOut = optedOutSet(agentID)
			consented = activeConsentSet(agentID, b.ConsentCategory, recipientNumbers)
		}
		if !isGroupBroadcast {
			if optedOut[r.Number] {
				markRecipient(r.ID, "skipped", "opt-out")
				bumpBroadcastCounter(broadcastID, "skipped")
				continue
			}
			if b.ConsentCategory != "" && !consented[r.Number] {
				markRecipient(r.ID, "skipped", "consent sudah tidak aktif")
				bumpBroadcastCounter(broadcastID, "skipped")
				continue
			}
		}

		msg := personalize(spinText(b.Message), r.Name)
		if isBroadcastCancelRequested(broadcastID) {
			campaign.setCancelled()
			return
		}

		var sendTo string
		if isGroupBroadcast {
			sendTo = r.Number
		} else {
			sendTo = services.NormalizePhone(r.Number)
			if ok, reason := services.ValidatePhoneForWA(sendTo); !ok {
				markRecipient(r.ID, "failed", "nomor tidak valid: "+reason+" (asal: "+r.Number+")")
				bumpBroadcastCounter(broadcastID, "failed")
				// Invalid number bukan systemic — jangan naikkan circuit breaker.
				continue
			}
		}

		if isGroupBroadcast && prepared == nil && len(mediaBytes) > 0 {
			markRecipient(r.ID, "failed", "media gagal disiapkan")
			bumpBroadcastCounter(broadcastID, "failed")
			continue
		}

		var sendErr error
		switch {
		case isGroupBroadcast && prepared != nil:
			sendErr = services.WA(agentID).SendPreparedMediaToJID(sendTo, msg, prepared)
		case isGroupBroadcast:
			sendErr = services.WA(agentID).SendTextToJID(sendTo, msg)
		case prepared != nil:
			sendErr = services.WA(agentID).SendPreparedMedia(sendTo, msg, prepared)
		case b.MediaType == "image" && len(mediaBytes) > 0:
			sendErr = services.WA(agentID).SendImage(sendTo, msg, b.Mimetype, mediaBytes)
		case b.MediaType == "video" && len(mediaBytes) > 0:
			sendErr = services.WA(agentID).SendVideo(sendTo, msg, b.Mimetype, mediaBytes)
		case b.MediaType == "document" && len(mediaBytes) > 0:
			sendErr = services.WA(agentID).SendDocument(sendTo, b.FileName, b.Mimetype, msg, mediaBytes)
		case b.MediaType == "contact":
			sendErr = services.WA(agentID).SendContact(sendTo, msg, b.ContactName, b.ContactNumber)
		default:
			sendErr = services.WA(agentID).SendText(sendTo, msg)
		}
		// Tombol produk interaktif (sama seperti jalur single-agent).
		if sendErr == nil && !isGroupBroadcast && b.ProductID > 0 && strings.TrimSpace(b.ProductButtonsJSON) != "" {
			sendErr = sendBroadcastProductButtons(b, agentID, sendTo)
		}

		if sendErr != nil {
			action, code := classifyBroadcastSendError(sendErr)
			if action == broadcastErrorWARestricted {
				markRecipient(r.ID, "pending", "Pengiriman dijeda oleh WhatsApp")
				campaign.setPause(code)
				reason := quarantineHard
				if code == 429 {
					reason = quarantineSoft
				}
				// Kode 0 dari pattern text: anggap hard agar aman.
				if code == 0 {
					reason = quarantineHard
				}
				campaign.quarantine(agentID, reason, code)
				log.Printf("Broadcast %d nomor %d ditolak WhatsApp (kode %d, %s) — dikarantina, sisa dialihkan", broadcastID, agentID, code, reason)
				return
			}
			if action == broadcastErrorInterrupted {
				campaign.quarantine(agentID, quarantineDisconnect, 0)
				log.Printf("Broadcast %d nomor %d terputus saat kirim ke %s — dialihkan", broadcastID, agentID, r.Number)
				return
			}
			markRecipient(r.ID, "failed", sendErr.Error())
			bumpBroadcastCounter(broadcastID, "failed")
			if isSystemicSendFailure(sendErr) {
				consecutiveSystemic++
				if consecutiveSystemic >= circuitBreakerThreshold {
					campaign.quarantine(agentID, quarantineSoft, 0)
					log.Printf("Broadcast %d nomor %d circuit-breaker: %d gagal sistemik beruntun — soft-quarantine", broadcastID, agentID, consecutiveSystemic)
					return
				}
			} else {
				consecutiveSystemic = 0
			}
		} else {
			now := time.Now()
			database.DB.Model(&models.BroadcastRecipient{}).Where("id = ?", r.ID).
				Updates(map[string]any{"status": "sent", "sent_at": &now, "error": ""})
			bumpBroadcastCounter(broadcastID, "sent")
			sentSinceRest++
			consecutiveSystemic = 0
		}

		if i < len(recipients)-1 {
			d := minD
			if maxD > minD {
				d = minD + rand.Intn(maxD-minD+1)
			}
			if boost > 1 {
				d = int(math.Ceil(float64(d) * boost))
			}
			if !sleepBroadcastDelay(broadcastID, d) {
				campaign.setCancelled()
				return
			}
			if restEvery > 0 && sentSinceRest >= restEvery {
				log.Printf("Broadcast %d nomor %d istirahat %d dtk setelah %d pesan", broadcastID, agentID, restDuration, sentSinceRest)
				sentSinceRest = 0
				rest := restDuration
				if boost > 1 {
					rest = int(math.Ceil(float64(rest) * boost))
				}
				if !sleepBroadcastDelay(broadcastID, rest) {
					campaign.setCancelled()
					return
				}
			}
		}
	}
}

// isSystemicSendFailure = error yang cenderung masalah nomor/kanal, bukan penerima salah.
func isSystemicSendFailure(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := services.WAServerErrorCode(err); ok {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"timeout", "timed out", "temporarily", "try again", "overloaded",
		"internal server", "service unavailable", "eof", "reset by peer",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	// Error penerima umum — jangan hit circuit breaker.
	for _, needle := range []string{
		"not found", "no whatsapp", "not on whatsapp", "invalid", "jid",
	} {
		if strings.Contains(msg, needle) {
			return false
		}
	}
	return false
}

// bumpBroadcastCounter menaikkan satu kolom hitungan broadcast secara atomik.
func bumpBroadcastCounter(broadcastID uint, field string) {
	database.DB.Model(&models.Broadcast{}).Where("id = ?", broadcastID).
		Update(field, gorm.Expr(field+" + 1"))
}

func recomputeBroadcastCounts(broadcastID uint) (sent, failed, skipped, pending int) {
	count := func(status string) int {
		var n int64
		database.DB.Model(&models.BroadcastRecipient{}).
			Where("broadcast_id = ? AND status = ?", broadcastID, status).Count(&n)
		return int(n)
	}
	return count("sent"), count("failed"), count("skipped"), count("pending")
}

// recordRestrictionIfAny menandai bila broadcast tuntas TAPI ada nomor yang sempat kena restriksi.
func recordRestrictionIfAny(broadcastID uint, campaign *broadcastCampaign) {
	code := campaign.getPause()
	if code == 0 {
		return
	}
	database.DB.Model(&models.Broadcast{}).Where("id = ?", broadcastID).
		Updates(map[string]any{"pause_reason": "wa_restriction", "pause_code": code})
	log.Printf("Broadcast %d selesai, TAPI satu nomor kena restriksi WhatsApp (kode %d) — dikarantina & sisanya dialihkan. Perhatikan kesehatan nomor itu.", broadcastID, code)
}

func runBroadcastRotation(broadcastID uint, b models.Broadcast, minD, maxD int, pool []uint) {
	// Muat karantina yang dipersist (mis. resume setelah wa_restricted).
	campaign := loadCampaignFromBroadcast(b)
	// runBroadcast sudah claim ke "running"; set resume jika quarantine hard masih ada.
	if len(campaign.quarantined) > 0 && b.PauseCode != 0 {
		campaign.setResumeMode(true)
	}
	// Deteksi resume: status resuming sempat di-set sebelum claim, atau quarantine terisi.
	// Juga izinkan last-resort jika barusan dari pause_reason.
	if strings.TrimSpace(b.QuarantineJSON) != "" && strings.Contains(b.QuarantineJSON, quarantineHard) {
		campaign.setResumeMode(true)
	}

	const maxRounds = 64
	for round := 0; round < maxRounds; round++ {
		if campaign.isCancelled() || isBroadcastCancelRequested(broadcastID) {
			finalizeCancelledBroadcast(broadcastID)
			return
		}

		_, _, _, pending := recomputeBroadcastCounts(broadcastID)
		if pending == 0 {
			sent, failed, skipped, _ := recomputeBroadcastCounts(broadcastID)
			finalStatus := "done"
			if sent == 0 && failed > 0 {
				finalStatus = "failed"
			}
			finishBroadcast(broadcastID, finalStatus, sent, failed, skipped)
			log.Printf("Broadcast %d %s: %d terkirim, %d gagal, %d dilewati (rotasi %d nomor)", broadcastID, finalStatus, sent, failed, skipped, len(pool))
			recordRestrictionIfAny(broadcastID, campaign)
			return
		}

		healthy := campaign.selectHealthyAgents(pool)
		if len(healthy) == 0 {
			sent, failed, skipped, _ := recomputeBroadcastCounts(broadcastID)
			if code := campaign.getPause(); code != 0 || campaign.hasRestrictionSignal() {
				if code == 0 {
					code = 463
				}
				pauseBroadcastByWhatsApp(broadcastID, code, sent, failed, skipped)
				log.Printf("Broadcast %d dijeda WhatsApp (kode %d): tidak ada nomor sehat untuk failover", broadcastID, code)
			} else {
				finishBroadcast(broadcastID, models.BroadcastInterrupted, sent, failed, skipped)
				log.Printf("Broadcast %d tertunda: semua nomor terputus / tidak siap", broadcastID)
			}
			return
		}

		reassignPendingToHealthy(broadcastID, healthy)

		quarantinedBefore := campaign.quarantinedCount()
		var wg sync.WaitGroup
		workers := 0
		for _, a := range healthy {
			var pc int64
			database.DB.Model(&models.BroadcastRecipient{}).
				Where("broadcast_id = ? AND agent_id = ? AND status = ?", broadcastID, a, "pending").Count(&pc)
			if pc == 0 {
				continue
			}
			wg.Add(1)
			workers++
			ag := a
			services.Go("broadcastAgentWorker", func() {
				defer wg.Done()
				runBroadcastAgentWorker(broadcastID, ag, b, minD, maxD, campaign)
			})
		}
		if workers == 0 {
			// Semua healthy tapi pending menempel di nomor lain yang offline/karantina —
			// reassign harusnya sudah jalan; jika tetap 0 worker, hentikan loop.
			break
		}
		wg.Wait()

		if campaign.isCancelled() || isBroadcastCancelRequested(broadcastID) {
			finalizeCancelledBroadcast(broadcastID)
			return
		}
		_, _, _, pendingAfter := recomputeBroadcastCounts(broadcastID)
		if pendingAfter == pending && campaign.quarantinedCount() == quarantinedBefore {
			break
		}
	}

	if isBroadcastCancelRequested(broadcastID) {
		finalizeCancelledBroadcast(broadcastID)
		return
	}
	sent, failed, skipped, pending := recomputeBroadcastCounts(broadcastID)
	if pending > 0 {
		if code := campaign.getPause(); code != 0 {
			pauseBroadcastByWhatsApp(broadcastID, code, sent, failed, skipped)
			log.Printf("Broadcast %d dijeda WhatsApp (kode %d): %d terkirim, %d masih menunggu", broadcastID, code, sent, pending)
		} else {
			finishBroadcast(broadcastID, models.BroadcastInterrupted, sent, failed, skipped)
			log.Printf("Broadcast %d tertunda: nomor terputus, %d masih menunggu", broadcastID, pending)
		}
		return
	}
	finalStatus := "done"
	if sent == 0 && failed > 0 {
		finalStatus = "failed"
	}
	finishBroadcast(broadcastID, finalStatus, sent, failed, skipped)
	recordRestrictionIfAny(broadcastID, campaign)
}
