package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// flowSessionTTL = berapa lama sesi menu dianggap aktif tanpa balasan. Lewat ini, kontak dianggap
// keluar dari menu (balasannya kembali ke keyword/AI) supaya tidak terjebak di menu selamanya.
const flowSessionTTL = 30 * time.Minute

// ── Struktur alur (disimpan JSON di Flow.Structure) ─────────────────────────
type flowOption struct {
	Key    string `json:"key"`    // input yang diketik kontak, mis "1"
	Label  string `json:"label"`  // keterangan (untuk builder saja)
	Action string `json:"action"` // reply | goto | handoff
	Reply  string `json:"reply"`  // teks balasan (action reply/handoff)
	Target string `json:"target"` // id node tujuan (action goto)
}
type flowNode struct {
	Message string       `json:"message"`
	Options []flowOption `json:"options"`
}
type flowStructure struct {
	Root  string              `json:"root"`
	Nodes map[string]flowNode `json:"nodes"`
}

// flowResult = hasil satu langkah penafsiran alur untuk processMessage.
type flowResult struct {
	handled  bool   // true = alur menangani pesan ini (jangan lanjut ke keyword/AI)
	reply    string // pesan yang harus dikirim (boleh kosong)
	handoff  bool   // true = serahkan ke manusia
	delayMin int    // rentang jeda khusus Alur Otomatis (detik)
	delayMax int
	buttons  []services.ReplyButton
	fallback string
}

var validFlowActions = map[string]bool{
	"reply": true, "reply_menu": true, "goto": true, "handoff": true,
}

func validateFlowStructure(s flowStructure) error {
	if s.Root == "" || len(s.Nodes) == 0 {
		return fmt.Errorf("alur belum memiliki menu utama")
	}
	if len(s.Nodes) > 30 {
		return fmt.Errorf("maksimal 30 menu dalam satu alur")
	}
	if _, ok := s.Nodes[s.Root]; !ok {
		return fmt.Errorf("menu utama tidak ditemukan")
	}
	if len(s.Nodes[s.Root].Options) == 0 {
		return fmt.Errorf("menu utama membutuhkan minimal satu pilihan")
	}
	for nodeID, node := range s.Nodes {
		if strings.TrimSpace(node.Message) == "" {
			return fmt.Errorf("menu %q belum memiliki pesan pembuka", nodeID)
		}
		if len([]rune(node.Message)) > 4000 {
			return fmt.Errorf("pesan menu %q terlalu panjang", nodeID)
		}
		if len(node.Options) > 20 {
			return fmt.Errorf("menu %q memiliki terlalu banyak pilihan", nodeID)
		}
		seenKeys := map[string]bool{}
		for _, option := range node.Options {
			key := strings.ToLower(strings.TrimSpace(option.Key))
			if key == "" {
				return fmt.Errorf("menu %q memiliki pilihan tanpa kode", nodeID)
			}
			if seenKeys[key] {
				return fmt.Errorf("kode pilihan %q dipakai dua kali di menu %q", option.Key, nodeID)
			}
			seenKeys[key] = true
			if len([]rune(option.Key)) > 32 || len([]rune(option.Label)) > 120 {
				return fmt.Errorf("kode atau nama pilihan %q terlalu panjang", option.Key)
			}
			if len([]rune(option.Reply)) > 4000 {
				return fmt.Errorf("teks balasan pilihan %q terlalu panjang", option.Key)
			}
			if !validFlowActions[option.Action] {
				return fmt.Errorf("aksi pilihan %q tidak dikenal", option.Key)
			}
			switch option.Action {
			case "goto":
				if option.Target == "" {
					return fmt.Errorf("pilihan %q belum memiliki menu tujuan", option.Key)
				}
				if option.Target == nodeID {
					return fmt.Errorf("pilihan %q tidak boleh menuju menu yang sama", option.Key)
				}
				if _, ok := s.Nodes[option.Target]; !ok {
					return fmt.Errorf("tujuan pilihan %q tidak ditemukan", option.Key)
				}
			case "reply", "reply_menu", "handoff":
				if strings.TrimSpace(option.Reply) == "" {
					return fmt.Errorf("pilihan %q belum memiliki teks balasan", option.Key)
				}
			}
		}
	}
	return nil
}

func renderFlowNodeMessage(nodeID string, node flowNode, rootID string) string {
	message := strings.TrimSpace(node.Message)
	var choices []string
	for _, option := range node.Options {
		if label := strings.TrimSpace(option.Label); label != "" {
			choices = append(choices, strings.TrimSpace(option.Key)+". "+label)
		}
	}
	if len(choices) > 0 {
		message += "\n\n" + strings.Join(choices, "\n")
	}
	if nodeID == rootID {
		message += "\n\nKetik *keluar* untuk menutup menu."
	} else {
		message += "\n\nKetik *0* untuk kembali · *keluar* untuk menutup menu."
	}
	return message
}

func flowNodeUsesButtons(mode string, node flowNode) bool {
	if mode == "text" || len(node.Options) == 0 || len(node.Options) > 3 {
		return false
	}
	for _, option := range node.Options {
		label := strings.TrimSpace(option.Label)
		if label == "" || len([]rune(label)) > 24 {
			return false
		}
	}
	return mode == "" || mode == "auto" || mode == "buttons"
}

func renderFlowNodeResult(nodeID string, node flowNode, rootID, mode, prefix string) flowResult {
	fallback := renderFlowNodeMessage(nodeID, node, rootID)
	if prefix != "" {
		fallback = strings.TrimSpace(prefix) + "\n\n" + fallback
	}
	if !flowNodeUsesButtons(mode, node) {
		return flowResult{handled: true, reply: fallback}
	}
	body := strings.TrimSpace(node.Message)
	if prefix != "" {
		body = strings.TrimSpace(prefix) + "\n\n" + body
	}
	if nodeID == rootID {
		body += "\n\nKetik *keluar* untuk menutup menu."
	} else {
		body += "\n\nKetik *0* untuk kembali · *keluar* untuk menutup menu."
	}
	buttons := make([]services.ReplyButton, 0, len(node.Options))
	for _, option := range node.Options {
		buttons = append(buttons, services.ReplyButton{ID: "flow:" + strings.TrimSpace(option.Key), Text: strings.TrimSpace(option.Label)})
	}
	return flowResult{handled: true, reply: body, buttons: buttons, fallback: fallback}
}

func parentFlowNode(s flowStructure, childID string) (string, bool) {
	for nodeID, node := range s.Nodes {
		for _, option := range node.Options {
			if option.Action == "goto" && option.Target == childID {
				return nodeID, true
			}
		}
	}
	return "", false
}

func matchTrigger(text, trigger, matchType string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	kw := strings.ToLower(strings.TrimSpace(trigger))
	if t == "" || kw == "" {
		return false
	}
	switch matchType {
	case "contains":
		return containsWholeFlowTrigger(t, kw)
	case "prefix":
		return strings.HasPrefix(t, kw)
	default: // exact
		return t == kw
	}
}

func containsWholeFlowTrigger(text, trigger string) bool {
	textRunes, triggerRunes := []rune(text), []rune(trigger)
	if len(triggerRunes) == 0 || len(triggerRunes) > len(textRunes) {
		return false
	}
	isWord := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsNumber(r)
	}
	for start := 0; start+len(triggerRunes) <= len(textRunes); start++ {
		if string(textRunes[start:start+len(triggerRunes)]) != string(triggerRunes) {
			continue
		}
		beforeOK := start == 0 || !isWord(textRunes[start-1])
		after := start + len(triggerRunes)
		afterOK := after == len(textRunes) || !isWord(textRunes[after])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

// matchOption mencari opsi yang cocok dengan input kontak (cocokkan ke Key, lalu ke Label).
func matchOption(node flowNode, text string) (flowOption, bool) {
	in := strings.ToLower(strings.TrimSpace(text))
	if in == "" {
		return flowOption{}, false
	}
	for _, o := range node.Options {
		if strings.ToLower(strings.TrimSpace(o.Key)) == in {
			return o, true
		}
	}
	for _, o := range node.Options {
		if o.Label != "" && strings.ToLower(strings.TrimSpace(o.Label)) == in {
			return o, true
		}
	}
	return flowOption{}, false
}

func looksLikeFlowCode(text, actionID string) bool {
	if strings.HasPrefix(actionID, "flow:") {
		return true
	}
	value := []rune(strings.ToLower(strings.TrimSpace(text)))
	if len(value) == 0 || len(value) > 3 {
		return false
	}
	allDigits := true
	hasDigit := false
	for _, r := range value {
		if r == ' ' || r == '\n' || r == '\t' {
			return false
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
		} else {
			allDigits = false
		}
	}
	return allDigits || hasDigit || len(value) == 1
}

func loadFlowStructure(f models.Flow) (flowStructure, bool) {
	var s flowStructure
	if strings.TrimSpace(f.Structure) == "" {
		return s, false
	}
	if err := json.Unmarshal([]byte(f.Structure), &s); err != nil {
		return s, false
	}
	if s.Root == "" || len(s.Nodes) == 0 {
		return s, false
	}
	if _, ok := s.Nodes[s.Root]; !ok {
		return s, false
	}
	if validateFlowStructure(s) != nil {
		return s, false
	}
	return s, true
}

func clearFlowSession(agentID uint, sender string) {
	database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).Delete(&models.FlowSession{})
}

func setFlowSession(agentID uint, sender, nodeID string) {
	sess := models.FlowSession{AgentID: agentID, Sender: sender, NodeID: nodeID, UpdatedAt: time.Now()}
	// Upsert manual: coba update, kalau tidak ada baris, buat.
	res := database.DB.Model(&models.FlowSession{}).
		Where("agent_id = ? AND sender = ?", agentID, sender).
		Updates(map[string]any{"node_id": nodeID, "updated_at": time.Now()})
	if res.RowsAffected == 0 {
		database.DB.Create(&sess)
	}
}

// enterFlowAt memulai/berpindah ke node lalu mengembalikan pesannya sebagai hasil.
func enterFlowAt(agentID uint, sender string, s flowStructure, nodeID, displayMode string) flowResult {
	node, ok := s.Nodes[nodeID]
	if !ok {
		clearFlowSession(agentID, sender)
		return flowResult{handled: true}
	}
	setFlowSession(agentID, sender, nodeID)
	return renderFlowNodeResult(nodeID, node, s.Root, displayMode, "")
}

// inFlowContext = true bila pesan ini bagian dari navigasi menu: agent punya alur aktif DAN
// (teksnya memicu menu ATAU kontak sedang di dalam sesi menu yang belum kedaluwarsa). Dipakai
// OnWAMessage untuk MELEWATI debounce — input menu ("1", "2") harus diproses utuh & terpisah,
// bukan digabung antar pesan (kalau digabung jadi "1\n2" tak cocok ke opsi mana pun).
func inFlowContext(agentID uint, sender, text string) bool {
	var f models.Flow
	if database.DB.Where("agent_id = ?", agentID).First(&f).Error != nil || !f.Enabled {
		return false
	}
	if matchTrigger(text, f.Trigger, f.MatchType) {
		return true
	}
	var sess models.FlowSession
	if database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).First(&sess).Error != nil {
		return false
	}
	return time.Since(sess.UpdatedAt) <= flowSessionTTL
}

// handleFlowMessage adalah penafsir alur/menu untuk satu pesan masuk. Mengembalikan handled=false
// bila kontak tidak sedang di menu dan pesannya tidak memicu menu (biar lanjut ke keyword/AI).
func handleFlowMessage(agentID uint, sender, text, actionID string) (result flowResult) {
	var f models.Flow
	if database.DB.Where("agent_id = ?", agentID).First(&f).Error != nil || !f.Enabled {
		return flowResult{}
	}
	defer func() {
		if result.handled {
			result.delayMin = f.DelayMin
			result.delayMax = f.DelayMax
		}
	}()
	s, ok := loadFlowStructure(f)
	if !ok {
		return flowResult{}
	}

	// Kata kunci pemicu selalu (re)start dari akar, termasuk saat sudah di dalam menu.
	if matchTrigger(text, f.Trigger, f.MatchType) {
		return enterFlowAt(agentID, sender, s, s.Root, f.DisplayMode)
	}

	// Sesi aktif? Kalau kedaluwarsa, buang dan anggap tak ada.
	var sess models.FlowSession
	if database.DB.Where("agent_id = ? AND sender = ?", agentID, sender).First(&sess).Error != nil {
		return flowResult{} // tidak di menu & bukan pemicu -> lanjut keyword/AI
	}
	if time.Since(sess.UpdatedAt) > flowSessionTTL {
		clearFlowSession(agentID, sender)
		return flowResult{}
	}

	node, ok := s.Nodes[sess.NodeID]
	if !ok {
		clearFlowSession(agentID, sender)
		return flowResult{}
	}
	input := text
	if strings.HasPrefix(actionID, "flow:") {
		input = strings.TrimPrefix(actionID, "flow:")
	}
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "keluar" || normalized == "batal" || normalized == "exit" {
		clearFlowSession(agentID, sender)
		return flowResult{handled: true, reply: "Baik, menu ditutup. Ketik *" + f.Trigger + "* kapan saja untuk membukanya lagi."}
	}
	if normalized == "0" || normalized == "kembali" || normalized == "back" {
		if parentID, found := parentFlowNode(s, sess.NodeID); found {
			return enterFlowAt(agentID, sender, s, parentID, f.DisplayMode)
		}
		clearFlowSession(agentID, sender)
		return flowResult{handled: true, reply: "Menu ditutup. Ketik *" + f.Trigger + "* untuk membuka kembali."}
	}

	opt, matched := matchOption(node, input)
	if !matched {
		if looksLikeFlowCode(input, actionID) {
			// Kode singkat yang salah tetap mendapat petunjuk menu.
			setFlowSession(agentID, sender, sess.NodeID)
			return renderFlowNodeResult(sess.NodeID, node, s.Root, f.DisplayMode, "Pilihan belum dikenali. Ketik kode atau pilih tombol yang tersedia.")
		}
		// Kalimat biasa berarti pelanggan kembali mengobrol. Tutup sesi menu tanpa
		// mengirim pesan tambahan lalu teruskan pesan yang sama ke Auto Reply/AI.
		clearFlowSession(agentID, sender)
		return flowResult{}
	}

	switch opt.Action {
	case "goto":
		return enterFlowAt(agentID, sender, s, opt.Target, f.DisplayMode)
	case "handoff":
		clearFlowSession(agentID, sender)
		return flowResult{handled: true, reply: opt.Reply, handoff: true}
	case "reply_menu":
		setFlowSession(agentID, sender, sess.NodeID)
		return renderFlowNodeResult(sess.NodeID, node, s.Root, f.DisplayMode, strings.TrimSpace(opt.Reply))
	default: // reply
		// Tetap pertahankan posisi menu. Kontak dapat mengetik kode lain tanpa harus
		// mengulang kata pemicu, tetapi daftar pilihan tidak dikirim ulang agar chat
		// tidak terlalu ramai. Sesi akan selesai lewat "keluar", kedaluwarsa, atau handoff.
		setFlowSession(agentID, sender, sess.NodeID)
		return flowResult{handled: true, reply: opt.Reply}
	}
}

// ── CRUD (satu alur per agent) ──────────────────────────────────────────────

// GetFlow mengembalikan alur agent (objek kosong bila belum ada).
func GetFlow(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var f models.Flow
	if database.DB.Where("agent_id = ?", id).First(&f).Error != nil {
		c.JSON(200, gin.H{"data": gin.H{"agent_id": id, "enabled": false, "trigger": "menu", "match_type": "exact", "display_mode": "auto", "structure": "", "delay_min": 2, "delay_max": 4}})
		return
	}
	c.JSON(200, gin.H{"data": f})
}

// SaveFlow membuat/memperbarui alur agent (upsert). Body: trigger, match_type, enabled, structure(JSON).
func SaveFlow(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		Enabled     bool   `json:"enabled"`
		Trigger     string `json:"trigger"`
		MatchType   string `json:"match_type"`
		DisplayMode string `json:"display_mode"`
		Structure   string `json:"structure"`
		DelayMin    *int   `json:"delay_min"`
		DelayMax    *int   `json:"delay_max"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Format data tidak valid"})
		return
	}
	// Validasi ringan struktur (kalau diisi) supaya JSON rusak tidak tersimpan.
	if strings.TrimSpace(req.Structure) != "" {
		var s flowStructure
		if err := json.Unmarshal([]byte(req.Structure), &s); err != nil {
			c.JSON(400, gin.H{"error": "Struktur alur tidak valid"})
			return
		}
		if err := validateFlowStructure(s); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
	} else if req.Enabled {
		c.JSON(400, gin.H{"error": "Alur masih kosong. Tambahkan menu dulu sebelum mengaktifkan."})
		return
	}
	if strings.TrimSpace(req.MatchType) == "" {
		req.MatchType = "exact"
	}
	if req.DisplayMode == "" {
		req.DisplayMode = "auto"
	}
	if req.DisplayMode != "auto" && req.DisplayMode != "text" && req.DisplayMode != "buttons" {
		c.JSON(400, gin.H{"error": "Mode tampilan menu tidak valid"})
		return
	}
	if req.DisplayMode == "buttons" && strings.TrimSpace(req.Structure) != "" {
		var structure flowStructure
		_ = json.Unmarshal([]byte(req.Structure), &structure)
		for _, node := range structure.Nodes {
			if len(node.Options) > 3 {
				c.JSON(400, gin.H{"error": "Mode selalu tombol hanya mendukung maksimal 3 pilihan per menu"})
				return
			}
			for _, option := range node.Options {
				if strings.TrimSpace(option.Label) == "" {
					c.JSON(400, gin.H{"error": "Mode selalu tombol membutuhkan nama pada setiap pilihan"})
					return
				}
				if len([]rune(strings.TrimSpace(option.Label))) > 24 {
					c.JSON(400, gin.H{"error": "Nama pilihan tombol maksimal 24 karakter"})
					return
				}
			}
		}
	}
	if req.MatchType != "exact" && req.MatchType != "prefix" && req.MatchType != "contains" {
		c.JSON(400, gin.H{"error": "Cara cocok pemicu tidak valid"})
		return
	}
	if strings.TrimSpace(req.Trigger) == "" {
		req.Trigger = "menu"
	}
	req.Trigger = strings.TrimSpace(req.Trigger)
	if len([]rune(req.Trigger)) > 64 {
		c.JSON(400, gin.H{"error": "Kata pemicu maksimal 64 karakter"})
		return
	}

	var f models.Flow
	err := database.DB.Where("agent_id = ?", id).First(&f).Error
	delayMin, delayMax := 2, 4
	if err == nil {
		delayMin, delayMax = f.DelayMin, f.DelayMax
	}
	if req.DelayMin != nil {
		delayMin = *req.DelayMin
	}
	if req.DelayMax != nil {
		delayMax = *req.DelayMax
	}
	if delayMin < 0 || delayMax < 0 || delayMin > 30 || delayMax > 30 || delayMin > delayMax {
		c.JSON(400, gin.H{"error": "Jeda balasan harus 0-30 detik dan minimum tidak boleh melebihi maksimum"})
		return
	}
	f.AgentID = id
	f.Enabled = req.Enabled
	f.Trigger = req.Trigger
	f.MatchType = req.MatchType
	f.DisplayMode = req.DisplayMode
	f.Structure = req.Structure
	f.DelayMin = delayMin
	f.DelayMax = delayMax
	if err != nil {
		if e := database.DB.Create(&f).Error; e != nil {
			c.JSON(500, gin.H{"error": "Gagal menyimpan alur"})
			return
		}
	} else if e := database.DB.Save(&f).Error; e != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan alur"})
		return
	}
	c.JSON(200, gin.H{"data": f})
}
