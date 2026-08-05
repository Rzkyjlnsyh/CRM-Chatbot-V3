package handlers

import (
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"wa-assistant/backend/config"
	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// ── REST API publik (autentikasi API key per-nomor) ─────────────────────────
//
// Base: /api/v1  (header: Authorization: Bearer <api_key>)
//   POST /messages  kirim pesan teks/media
//   POST /check     cek nomor terdaftar di WhatsApp
//   GET  /status    status koneksi nomor

const apiAgentKey = "api_agent"

// extractAPIKey membaca token dari "Authorization: Bearer", header X-API-Key, atau query ?token=.
func extractAPIKey(c *gin.Context) string {
	if h := strings.TrimSpace(c.GetHeader("Authorization")); h != "" {
		if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
			return strings.TrimSpace(h[7:])
		}
		return h
	}
	if k := strings.TrimSpace(c.GetHeader("X-API-Key")); k != "" {
		return k
	}
	return strings.TrimSpace(c.Query("token"))
}

// APIKeyMiddleware mengautentikasi request lewat API key per-nomor, lalu menaruh agent
// pemilik key di context. Menolak bila key salah atau paket tak mengizinkan.
func APIKeyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := extractAPIKey(c)
		if key == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "API key tidak ada. Sertakan header 'Authorization: Bearer <token>'."})
			return
		}
		var agent models.Agent
		if database.DB.Where("api_key = ?", key).First(&agent).Error != nil || agent.APIKey == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "API key tidak valid."})
			return
		}
		if !tenantPlanAllows(agent.TenantID, featAPI) {
			c.AbortWithStatusJSON(403, gin.H{"error": planFeatureMessage})
			return
		}
		if ok, wait := apiRateLimitAllow(agent.ID); !ok {
			c.Header("Retry-After", strconv.Itoa(wait))
			c.AbortWithStatusJSON(429, gin.H{"error": "Terlalu banyak permintaan. Coba lagi sebentar."})
			return
		}
		c.Set(apiAgentKey, agent)
		c.Next()
	}
}

func apiAgent(c *gin.Context) models.Agent {
	v, _ := c.Get(apiAgentKey)
	a, _ := v.(models.Agent)
	return a
}

// apiMessageReq = body kirim pesan (dipakai /messages & /groups/:jid/messages).
type apiMessageReq struct {
	To            string `json:"to"`
	Type          string `json:"type"` // text | contact | image | video | document
	Text          string `json:"text"`
	Caption       string `json:"caption"`
	MediaURL      string `json:"media_url"`
	Filename      string `json:"filename"`
	ReplyTo       string `json:"reply_to"`       // ID pesan yang dibalas (type=text)
	ContactName   string `json:"contact_name"`   // type=contact
	ContactNumber string `json:"contact_number"` // type=contact
}

// respType = nama tipe untuk balasan JSON (default "text").
func respType(t string) string {
	if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
		return t
	}
	return "text"
}

// deliverAPIMessage mengirim pesan sesuai req ke tujuan `to` (nomor ternormalisasi atau JID grup).
// Return: messageID (bila ada), kode HTTP, dan pesan error ("" = sukses).
func deliverAPIMessage(agentID uint, to string, req apiMessageReq) (string, int, string) {
	if !services.WA(agentID).IsConnected() {
		return "", 409, "Nomor WhatsApp sedang tidak tersambung."
	}
	isGroup := services.IsGroupJID(to)
	switch respType(req.Type) {
	case "text":
		if strings.TrimSpace(req.Text) == "" {
			return "", 400, "Field 'text' wajib diisi untuk pesan teks."
		}
		if strings.TrimSpace(req.ReplyTo) != "" {
			var err error
			if isGroup {
				err = services.WA(agentID).SendReplyToJID(to, req.Text, req.ReplyTo)
			} else {
				err = services.WA(agentID).SendReply(to, req.Text, req.ReplyTo)
			}
			if err != nil {
				return "", 502, "Gagal mengirim: " + err.Error()
			}
			return "", 200, ""
		}
		if isGroup {
			if err := services.WA(agentID).SendTextToJID(to, req.Text); err != nil {
				return "", 502, "Gagal mengirim: " + err.Error()
			}
			return "", 200, ""
		}
		msgID, err := services.WA(agentID).SendTextAndGetID(to, req.Text)
		if err != nil {
			return "", 502, "Gagal mengirim: " + err.Error()
		}
		return msgID, 200, ""
	case "contact":
		if isGroup {
			return "", 400, "Kartu kontak belum didukung untuk tujuan grup."
		}
		if strings.TrimSpace(req.ContactNumber) == "" {
			return "", 400, "Field 'contact_number' wajib diisi untuk kartu kontak."
		}
		name := strings.TrimSpace(req.ContactName)
		if name == "" {
			name = req.ContactNumber
		}
		if err := services.WA(agentID).SendContact(to, req.Text, name, req.ContactNumber); err != nil {
			return "", 502, "Gagal mengirim kartu kontak: " + err.Error()
		}
		return "", 200, ""
	case "image", "video", "document":
		if strings.TrimSpace(req.MediaURL) == "" {
			return "", 400, "Field 'media_url' wajib diisi untuk pesan media."
		}
		if err := isSafeRemoteURL(req.MediaURL); err != nil {
			return "", 400, "URL media tidak diizinkan: " + err.Error()
		}
		data, mimetype, err := fetchRemoteMedia(req.MediaURL)
		if err != nil {
			return "", 400, "Gagal mengambil media: " + err.Error()
		}
		caption := req.Caption
		if caption == "" {
			caption = req.Text
		}
		mediaType := respType(req.Type)
		filename := strings.TrimSpace(req.Filename)
		if filename == "" {
			filename = "file"
		}
		if isGroup {
			prepared, prepareErr := services.WA(agentID).PrepareMedia(mediaType, mimetype, filename, data)
			if prepareErr != nil {
				return "", 502, "Gagal mengunggah media: " + prepareErr.Error()
			}
			err = services.WA(agentID).SendPreparedMediaToJID(to, caption, prepared)
		} else {
			switch mediaType {
			case "image":
				err = services.WA(agentID).SendImage(to, caption, mimetype, data)
			case "video":
				err = services.WA(agentID).SendVideo(to, caption, mimetype, data)
			default: // document
				err = services.WA(agentID).SendDocument(to, filename, mimetype, caption, data)
			}
		}
		if err != nil {
			return "", 502, "Gagal mengirim media: " + err.Error()
		}
		return "", 200, ""
	default:
		return "", 400, "Field 'type' harus salah satu: text, contact, image, video, document."
	}
}

// APISendMessage — POST /api/v1/messages
func APISendMessage(c *gin.Context) {
	agent := apiAgent(c)
	var req apiMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Body JSON tidak valid."})
		return
	}
	to := services.NormalizePhone(req.To)
	if to == "" {
		c.JSON(400, gin.H{"error": "Field 'to' (nomor tujuan) wajib diisi."})
		return
	}
	msgID, code, errMsg := deliverAPIMessage(agent.ID, to, req)
	if errMsg != "" {
		c.JSON(code, gin.H{"error": errMsg})
		return
	}
	messageType := respType(req.Type)
	logText := strings.TrimSpace(req.Text)
	if logText == "" {
		logText = strings.TrimSpace(req.Caption)
	}
	if logText == "" && messageType != "text" {
		logText = "[" + messageType + "]"
	}
	logTurn(agent.ID, to, "", logText, true, req.ReplyTo, "")
	if msgID != "" {
		_ = database.DB.Model(&models.ChatHistory{}).
			Where("agent_id = ? AND sender = ? AND reply = ?", agent.ID, to, logText).
			Order("id desc").Limit(1).Update("wa_msg_id", msgID).Error
	}
	c.JSON(200, gin.H{"status": "sent", "to": to, "type": messageType, "message_id": msgID})
}

// APICheckNumber — POST /api/v1/check  Body: {"numbers":["628...","08..."]}
func APICheckNumber(c *gin.Context) {
	agent := apiAgent(c)
	var req struct {
		Numbers []string `json:"numbers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Numbers) == 0 {
		c.JSON(400, gin.H{"error": "Field 'numbers' (daftar nomor) wajib diisi."})
		return
	}
	if len(req.Numbers) > 100 {
		c.JSON(400, gin.H{"error": "Maksimal 100 nomor per permintaan."})
		return
	}
	if !services.WA(agent.ID).IsConnected() {
		c.JSON(409, gin.H{"error": "Nomor WhatsApp sedang tidak tersambung."})
		return
	}
	res, err := services.WA(agent.ID).CheckNumbers(req.Numbers)
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal cek nomor: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"data": res})
}

// APIStatus — GET /api/v1/status
func APIStatus(c *gin.Context) {
	agent := apiAgent(c)
	c.JSON(200, gin.H{
		"agent_id":  agent.ID,
		"number":    agent.Number,
		"name":      agent.Name,
		"connected": services.WA(agent.ID).IsConnected(),
	})
}

// ── Util unduh media aman ────────────────────────────────────────────────────

// isSafeRemoteURL menolak URL non-http(s) dan host yang menunjuk ke jaringan internal
// (loopback/privat/link-local) — cegah SSRF ke jaringan VPS lewat media_url/webhook_url.
func isSafeRemoteURL(raw string) error {
	u, err := neturl.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return fmt.Errorf("URL tidak valid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("hanya http/https yang didukung")
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("host tidak dapat di-resolve")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("host mengarah ke jaringan internal")
		}
	}
	return nil
}

// fetchRemoteMedia mengunduh media dari URL dengan batas ukuran (WA_MEDIA_MAX_MB, default 16)
// dan timeout, lalu mendeteksi mimetype.
func fetchRemoteMedia(url string) ([]byte, string, error) {
	maxMB := config.EnvInt("WA_MEDIA_MAX_MB", 16)
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return isSafeRemoteURL(req.URL.String())
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("URL membalas status %d", resp.StatusCode)
	}
	limit := int64(maxMB) << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", fmt.Errorf("media melebihi batas %d MB", maxMB)
	}
	mimetype := resp.Header.Get("Content-Type")
	if mimetype == "" {
		mimetype = http.DetectContentType(data)
	}
	if i := strings.IndexByte(mimetype, ';'); i > 0 {
		mimetype = strings.TrimSpace(mimetype[:i])
	}
	return data, mimetype, nil
}
