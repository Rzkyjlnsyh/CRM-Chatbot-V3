package handlers

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────────────────
// Link preview (pola v4) — aman SSRF: hanya http/https, blok IP privat/loopback,
// batas 5 MB, timeout 6 detik. Tanpa dependensi eksternal (fork tidak punya
// paket safehttp — guard dibuat inline di sini).
// ─────────────────────────────────────────────────────────────────────────

var (
	ogTitleRe   = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']*)["']`)
	ogDescRe    = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']*)["']`)
	ogImageRe   = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']*)["']`)
	titleFallRe = regexp.MustCompile(`(?is)<title[^>]*>([^<]*)</title>`)
)

var linkPreviewClient = &http.Client{
	Timeout: 6 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return http.ErrUseLastResponse
		}
		if !isSafeLinkURL(req.URL) {
			return http.ErrUseLastResponse
		}
		return nil
	},
}

// isSafeLinkURL memblokir tujuan non-http(s) dan IP privat/loopback/link-local.
func isSafeLinkURL(u *url.URL) bool {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsGlobalUnicast() && !ip.IsPrivate()
	}
	return true
}

// LinkPreview — GET /agents/:id/link-preview?url=...
func LinkPreview(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("url"))
	if raw == "" {
		c.JSON(400, gin.H{"error": "url wajib"})
		return
	}
	u, err := url.Parse(raw)
	if err != nil || !isSafeLinkURL(u) {
		c.JSON(400, gin.H{"error": "URL tidak aman"})
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, u.String(), nil)
	if err != nil {
		c.JSON(400, gin.H{"error": "URL tidak valid"})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; WontenBot/1.0)")
	resp, err := linkPreviewClient.Do(req)
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal mengambil halaman"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		c.JSON(502, gin.H{"error": "Halaman tidak tersedia"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal membaca halaman"})
		return
	}
	html := string(body)
	title := firstMatch(ogTitleRe, html)
	if title == "" {
		title = firstMatch(titleFallRe, html)
	}
	desc := firstMatch(ogDescRe, html)
	image := firstMatch(ogImageRe, html)
	if image != "" {
		if iu, err := url.Parse(image); err == nil && !iu.IsAbs() {
			image = u.ResolveReference(iu).String()
		}
	}
	if title == "" && desc == "" && image == "" {
		c.JSON(200, gin.H{"data": gin.H{"title": u.Host}})
		return
	}
	c.JSON(200, gin.H{"data": gin.H{
		"title":       strings.TrimSpace(title),
		"description": strings.TrimSpace(desc),
		"image":       image,
		"url":         u.String(),
	}})
}

func firstMatch(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// ServeProfilePicture — GET /agents/:id/profile-picture?sender=...
// Redirect ke URL thumbnail WA (klien <img> tanpa header auth).
func ServeProfilePicture(c *gin.Context) {
	sender := strings.TrimSpace(c.Query("sender"))
	if sender == "" {
		c.JSON(400, gin.H{"error": "sender wajib"})
		return
	}
	agentID := currentAgentID(c)
	if agentID == 0 {
		// <img> tidak bisa mengirim header Authorization → dukung ?token=
		// (JWT login / media token, seperti endpoint media lainnya).
		if tid, ok := tenantFromToken(c.Query("token")); ok {
			var agent models.Agent
			if err := database.DB.Select("id").
				Where("id = ? AND tenant_id = ?", c.Param("id"), tid).
				First(&agent).Error; err == nil {
				agentID = agent.ID
			}
		}
	}
	if agentID == 0 {
		c.JSON(404, gin.H{"error": "Agent tidak ditemukan"})
		return
	}
	// Batasi waktu tunggu WA (jangan sampai request menggantung lama).
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	url, err := services.WA(agentID).ProfilePictureURL(ctx, sender)
	if err != nil || url == "" {
		c.JSON(404, gin.H{"error": "Foto profil tidak tersedia"})
		return
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.Redirect(http.StatusFound, url)
}
