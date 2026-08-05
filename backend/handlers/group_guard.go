package handlers

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// InitGroupGuard mendaftarkan handler moderasi pesan grup (dipanggil sekali dari main).
func InitGroupGuard() {
	services.SetGroupMessageHandler(handleGroupMessage)
}

var (
	groupLinkRe  = regexp.MustCompile(`(?i)(https?://|www\.|\b[a-z0-9][a-z0-9-]*\.(com|net|org|id|co|io|xyz|info|biz|link|shop|store|online|site|me|ru|cn|top|click)\b)`)
	groupPhoneRe = regexp.MustCompile(`\+?\d[\d \-]{8,}\d`)
)

// ---- cache admin grup (hindari GetGroupInfo per pesan) ----

type groupModEntry struct {
	admins     map[string]bool
	botIsAdmin bool
	fetchedAt  time.Time
}

var (
	groupModCache = map[string]groupModEntry{}
	groupModMu    sync.Mutex
)

func groupModInfoCached(agentID uint, groupJID string) (map[string]bool, bool) {
	key := fmt.Sprintf("%d|%s", agentID, groupJID)
	groupModMu.Lock()
	e, ok := groupModCache[key]
	groupModMu.Unlock()
	if ok && time.Since(e.fetchedAt) < 3*time.Minute {
		return e.admins, e.botIsAdmin
	}
	admins, botIsAdmin, err := services.WA(agentID).GroupModerationInfo(groupJID)
	if err != nil {
		if ok {
			return e.admins, e.botIsAdmin // pakai cache lama kalau ada
		}
		return map[string]bool{}, false
	}
	groupModMu.Lock()
	groupModCache[key] = groupModEntry{admins: admins, botIsAdmin: botIsAdmin, fetchedAt: time.Now()}
	groupModMu.Unlock()
	return admins, botIsAdmin
}

func invalidateGroupModCache(agentID uint, groupJID string) {
	groupModMu.Lock()
	delete(groupModCache, fmt.Sprintf("%d|%s", agentID, groupJID))
	groupModMu.Unlock()
}

// ---- pelacak flood (per agent+grup+pengirim) ----

var (
	floodMu      sync.Mutex
	floodTracker = map[string][]time.Time{}
)

func floodHit(agentID uint, group, sender string, count, windowSec int) bool {
	if count <= 0 {
		return false
	}
	if windowSec <= 0 {
		windowSec = 10
	}
	key := fmt.Sprintf("%d|%s|%s", agentID, group, sender)
	now := time.Now()
	win := time.Duration(windowSec) * time.Second
	floodMu.Lock()
	defer floodMu.Unlock()
	kept := floodTracker[key][:0]
	for _, t := range floodTracker[key] {
		if now.Sub(t) <= win {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	floodTracker[key] = kept
	return len(kept) >= count
}

// ---- inti moderasi ----

func handleGroupMessage(agentID uint, m services.GroupMessageMeta) {
	var cfg models.GroupGuardConfig
	if err := database.DB.Where("agent_id = ? AND group_jid = ? AND enabled = ?", agentID, m.GroupJID, true).
		First(&cfg).Error; err != nil {
		return // grup tidak dimonitor
	}
	if numInList(cfg.AllowNumbers, m.SenderPN) {
		return
	}
	admins, botIsAdmin := groupModInfoCached(agentID, m.GroupJID)
	if admins[m.SenderPN] || admins[jidUser(m.SenderJID)] {
		return // jangan moderasi admin grup
	}

	reason := detectGroupSpam(cfg, agentID, m)
	if reason == "" {
		return
	}

	deleted := false
	if cfg.DeleteSpam && botIsAdmin && m.MessageID != "" {
		if err := services.WA(agentID).DeleteGroupMessage(m.GroupJID, m.SenderJID, m.MessageID); err == nil {
			deleted = true
		} else {
			log.Printf("[grup] hapus pesan gagal (agent %d grup %s): %v", agentID, m.GroupJID, err)
		}
	}

	action := "flagged"
	if deleted {
		action = "deleted"
	}
	status := "done"
	if cfg.AutoKick && botIsAdmin {
		if err := services.WA(agentID).KickFromGroup(m.GroupJID, m.SenderJID); err == nil {
			action = "kicked"
		} else {
			log.Printf("[grup] auto-kick gagal (agent %d grup %s): %v", agentID, m.GroupJID, err)
		}
	} else if cfg.FlagForKick && botIsAdmin {
		status = "pending" // tunggu konfirmasi manusia
	}

	excerpt := m.Text
	if len([]rune(excerpt)) > 280 {
		excerpt = string([]rune(excerpt)[:280])
	}
	_ = database.DB.Create(&models.GroupModerationLog{
		AgentID: agentID, GroupJID: m.GroupJID, GroupName: cfg.GroupName,
		Sender: m.SenderPN, SenderJID: m.SenderJID, SenderName: m.SenderName,
		WAMsgID: m.MessageID, Action: action, Reason: reason, Excerpt: excerpt,
		Status: status, ActedBy: "auto",
	}).Error
	log.Printf("[grup] agent %d grup %s: spam '%s' dari %s -> %s/%s", agentID, m.GroupJID, reason, m.SenderPN, action, status)
}

func detectGroupSpam(cfg models.GroupGuardConfig, agentID uint, m services.GroupMessageMeta) string {
	text := strings.TrimSpace(m.Text)
	low := strings.ToLower(text)
	if cfg.BlockLinks && text != "" && groupLinkRe.MatchString(text) {
		return "tautan/link"
	}
	if cfg.BlockPhones && groupPhoneRe.MatchString(text) {
		return "nomor telepon"
	}
	for _, w := range splitList(cfg.BlockWords) {
		if w != "" && strings.Contains(low, strings.ToLower(w)) {
			return "kata terlarang: " + w
		}
	}
	if floodHit(agentID, m.GroupJID, m.SenderPN, cfg.FloodCount, cfg.FloodWindowSec) {
		return "flood (pesan beruntun)"
	}
	return ""
}

// splitList memecah daftar yang dipisah baris/koma jadi item bersih.
func splitList(s string) []string {
	raw := strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' || r == ';' })
	out := make([]string, 0, len(raw))
	for _, x := range raw {
		if t := strings.TrimSpace(x); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// numInList cek apakah nomor (berdasarkan digit) ada di daftar allowlist.
func numInList(list, number string) bool {
	target := onlyDigits(number)
	if target == "" {
		return false
	}
	for _, x := range splitList(list) {
		if onlyDigits(x) == target {
			return true
		}
	}
	return false
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func jidUser(jid string) string {
	if i := strings.IndexByte(jid, '@'); i >= 0 {
		return jid[:i]
	}
	return jid
}

// ---- endpoint ----

type groupConfigBody struct {
	GroupJID       string `json:"group_jid"`
	GroupName      string `json:"group_name"`
	Enabled        bool   `json:"enabled"`
	DeleteSpam     bool   `json:"delete_spam"`
	FlagForKick    bool   `json:"flag_for_kick"`
	AutoKick       bool   `json:"auto_kick"`
	BlockLinks     bool   `json:"block_links"`
	BlockPhones    bool   `json:"block_phones"`
	BlockWords     string `json:"block_words"`
	FloodCount     int    `json:"flood_count"`
	FloodWindowSec int    `json:"flood_window_sec"`
	AllowNumbers   string `json:"allow_numbers"`
}

// GroupConfig mengembalikan setelan moderasi satu grup (default kalau belum ada).
func GroupConfig(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	gjid := c.Query("gjid")
	if gjid == "" {
		c.JSON(400, gin.H{"error": "gjid wajib"})
		return
	}
	var cfg models.GroupGuardConfig
	if err := database.DB.Where("agent_id = ? AND group_jid = ?", id, gjid).First(&cfg).Error; err != nil {
		// default belum tersimpan
		c.JSON(200, gin.H{"data": models.GroupGuardConfig{
			AgentID: id, GroupJID: gjid, DeleteSpam: true, FlagForKick: true,
			BlockLinks: true, FloodWindowSec: 10,
		}})
		return
	}
	c.JSON(200, gin.H{"data": cfg})
}

// SaveGroupConfig membuat/memperbarui setelan moderasi satu grup.
func SaveGroupConfig(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var b groupConfigBody
	if err := c.ShouldBindJSON(&b); err != nil || strings.TrimSpace(b.GroupJID) == "" {
		c.JSON(400, gin.H{"error": "Data setelan grup tidak valid"})
		return
	}

	var cfg models.GroupGuardConfig
	database.DB.Where("agent_id = ? AND group_jid = ?", id, b.GroupJID).First(&cfg)
	cfg.AgentID = id
	cfg.TenantID = currentTenantID(c)
	cfg.GroupJID = b.GroupJID
	cfg.GroupName = b.GroupName
	cfg.Enabled = b.Enabled
	cfg.DeleteSpam = b.DeleteSpam
	cfg.FlagForKick = b.FlagForKick
	cfg.AutoKick = b.AutoKick
	cfg.BlockLinks = b.BlockLinks
	cfg.BlockPhones = b.BlockPhones
	cfg.BlockWords = b.BlockWords
	cfg.FloodCount = b.FloodCount
	cfg.FloodWindowSec = b.FloodWindowSec
	cfg.AllowNumbers = b.AllowNumbers
	if err := database.DB.Save(&cfg).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan setelan"})
		return
	}
	invalidateGroupModCache(id, b.GroupJID)
	c.JSON(200, gin.H{"data": cfg})
}

// GroupModeration mengembalikan log/feed moderasi (terbaru dulu).
func GroupModeration(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	q := database.DB.Where("agent_id = ?", id)
	if status := c.Query("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	if gjid := c.Query("gjid"); gjid != "" {
		q = q.Where("group_jid = ?", gjid)
	}
	var rows []models.GroupModerationLog
	q.Order("created_at desc").Limit(100).Find(&rows)
	c.JSON(200, gin.H{"data": rows})
}

// ConfirmKick mengeluarkan pengirim dari grup berdasarkan satu entri moderasi yang pending.
func ConfirmKick(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	var entry models.GroupModerationLog
	if database.DB.Where("id = ? AND agent_id = ?", c.Param("logid"), id).First(&entry).Error != nil {
		c.JSON(404, gin.H{"error": "Entri tidak ditemukan"})
		return
	}
	if entry.SenderJID == "" {
		c.JSON(400, gin.H{"error": "Data pengirim tidak lengkap untuk dikeluarkan"})
		return
	}
	if err := services.WA(id).KickFromGroup(entry.GroupJID, entry.SenderJID); err != nil {
		c.JSON(502, gin.H{"error": "Gagal mengeluarkan (pastikan Wai admin & tersambung): " + err.Error()})
		return
	}
	database.DB.Model(&entry).Updates(map[string]any{
		"action": "kicked", "status": "done", "acted_by": fmt.Sprintf("%d", c.GetUint("user_id")),
	})
	c.JSON(200, gin.H{"message": "Anggota dikeluarkan"})
}

// DismissModeration menandai entri moderasi sebagai diabaikan (false positive).
func DismissModeration(c *gin.Context) {
	id, ok := resolveAgent(c)
	if !ok {
		return
	}
	res := database.DB.Model(&models.GroupModerationLog{}).
		Where("id = ? AND agent_id = ?", c.Param("logid"), id).
		Updates(map[string]any{"status": "dismissed", "acted_by": fmt.Sprintf("%d", c.GetUint("user_id"))})
	if res.RowsAffected == 0 {
		c.JSON(404, gin.H{"error": "Entri tidak ditemukan"})
		return
	}
	c.JSON(200, gin.H{"message": "Diabaikan"})
}
