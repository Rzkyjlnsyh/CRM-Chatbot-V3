package handlers

import (
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"

	"github.com/gin-gonic/gin"
)

// crawlLimitsForTenant — tidak ada batas untuk instalasi internal.
func crawlLimitsForTenant(tenantID uint) (maxChars, maxPages int) {
	return 0, 0 // unlimited
}

func knowledgeCharsUsed(agentID uint) int64 {
	var used int64
	database.DB.Model(&models.Knowledge{}).Where("agent_id = ?", agentID).
		Select("COALESCE(SUM(char_count),0)").Scan(&used)
	return used
}

// StartCrawl memulai crawl website untuk satu agent (nomor) sebagai background job.
func StartCrawl(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		c.JSON(400, gin.H{"error": "URL website wajib diisi"})
		return
	}
	raw := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	if u, err := url.Parse(raw); err != nil || u.Host == "" {
		c.JSON(400, gin.H{"error": "URL tidak valid"})
		return
	}

	// Cegah crawl ganda yang masih berjalan untuk nomor yang sama.
	var running int64
	database.DB.Model(&models.CrawlJob{}).
		Where("agent_id = ? AND status IN ?", aid, []string{"pending", "crawling"}).Count(&running)
	if running > 0 {
		c.JSON(409, gin.H{"error": "Masih ada crawl berjalan untuk nomor ini. Tunggu sampai selesai."})
		return
	}

	_, maxPages := crawlLimitsForTenant(currentTenantID(c))
	job := models.CrawlJob{AgentID: aid, RootURL: raw, Status: "pending"}
	if err := database.DB.Create(&job).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal membuat job crawl"})
		return
	}
	services.Go("RunCrawl", func() { services.RunCrawl(job.ID, maxPages) })
	c.JSON(201, gin.H{"data": job, "max_pages": maxPages})
}

// CrawlStatus mengembalikan satu job + daftar halamannya (untuk polling UI).
func CrawlStatus(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var job models.CrawlJob
	if database.DB.Where("agent_id = ?", aid).First(&job, c.Param("jobId")).Error != nil {
		c.JSON(404, gin.H{"error": "Job tidak ditemukan"})
		return
	}
	c.JSON(200, gin.H{"job": job, "pages": crawlPagesOf(job.ID)})
}

// LatestCrawl mengembalikan job crawl terakhir agent (agar UI bisa lanjut polling setelah refresh).
func LatestCrawl(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var job models.CrawlJob
	if database.DB.Where("agent_id = ?", aid).Order("id desc").First(&job).Error != nil {
		c.JSON(200, gin.H{"job": nil})
		return
	}
	c.JSON(200, gin.H{"job": job, "pages": crawlPagesOf(job.ID)})
}

func crawlPagesOf(jobID uint) []models.CrawlPage {
	var pages []models.CrawlPage
	// Rekomendasi skor tinggi dulu agar UI menampilkan halaman terbaik di atas.
	database.DB.Where("job_id = ?", jobID).
		Order("recommend_score desc, recommended desc, char_count desc, id asc").
		Find(&pages)
	return pages
}

// TrainCrawlPages memulai pelatihan halaman terpilih menjadi FAQ (background job).
// Tiap halaman diubah AI jadi Q&A bersih lalu di-embed. Status job -> "training"; UI polling.
func TrainCrawlPages(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var req struct {
		PageIDs       []uint `json:"page_ids"`
		UpdatePersona *bool  `json:"update_persona"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.PageIDs) == 0 {
		c.JSON(400, gin.H{"error": "Pilih minimal satu halaman"})
		return
	}
	var job models.CrawlJob
	if database.DB.Where("agent_id = ?", aid).First(&job, c.Param("jobId")).Error != nil {
		c.JSON(404, gin.H{"error": "Job tidak ditemukan"})
		return
	}
	if job.Status == "training" {
		c.JSON(409, gin.H{"error": "Pelatihan sedang berjalan, tunggu sampai selesai"})
		return
	}
	var trainable int64
	database.DB.Model(&models.CrawlPage{}).
		Where("agent_id = ? AND job_id = ? AND id IN ? AND char_count > 0 AND status IN ?", aid, job.ID, req.PageIDs, []string{"crawled", "failed"}).
		Count(&trainable)
	if trainable == 0 {
		c.JSON(400, gin.H{"error": "Halaman yang dipilih tidak memiliki konten yang bisa dilatih"})
		return
	}
	maxChars, _ := crawlLimitsForTenant(currentTenantID(c))
	// Website hanya menambah pengetahuan. Persona tidak boleh berubah diam-diam;
	// pembaruan persona dari website tersedia sebagai aksi terpisah dan eksplisit.
	updatePersona := req.UpdatePersona != nil && *req.UpdatePersona
	database.DB.Model(&job).Updates(map[string]any{
		"status": "training", "error": "", "persona_updated": false, "persona_error": "",
	})
	services.Go("runWebTraining", func() { runWebTraining(aid, job.ID, req.PageIDs, maxChars, updatePersona) })
	c.JSON(202, gin.H{"started": true})
}

// runWebTraining (background) mengubah tiap halaman terpilih menjadi FAQ Q&A via AI lalu menyimpannya
// sebagai knowledge. Hormati kuota karakter; halaman tanpa info berguna dilewati (bukan sampah masuk KB).
func runWebTraining(agentID, jobID uint, pageIDs []uint, maxChars int, updatePersona bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[train] PANIC agent %d: %v", agentID, r)
			database.DB.Model(&models.CrawlJob{}).Where("id = ?", jobID).
				Updates(map[string]any{"status": "failed", "error": "Pelatihan terhenti karena kesalahan internal"})
			return
		}
		database.DB.Model(&models.CrawlJob{}).Where("id = ?", jobID).Update("status", "done")
	}()

	used := knowledgeCharsUsed(agentID)
	var pages []models.CrawlPage
	database.DB.Where("agent_id = ? AND job_id = ? AND id IN ?", agentID, jobID, pageIDs).Find(&pages)
	writer := newKnowledgeUpserter(agentID)
	log.Printf("[train] mulai agent %d job %d: %d halaman dipilih", agentID, jobID, len(pages))

	trainedN, skippedN, failedN, totalFAQ := 0, 0, 0, 0
	stopped := false
	for i := range pages {
		// Hormati permintaan Stop dari user: berhenti rapi antar-halaman.
		// Halaman yang sudah jadi FAQ tetap tersimpan; sisanya bisa dilatih lagi nanti.
		if jobStatusIs(jobID, "stopping") {
			log.Printf("[train] dihentikan user pada halaman %d/%d (agent %d)", i, len(pages), agentID)
			stopped = true
			break
		}
		p := pages[i]
		if p.Status == "trained" || strings.TrimSpace(p.Content) == "" {
			continue
		}
		if maxChars > 0 && used >= int64(maxChars) {
			setPageStatus(p.ID, "failed", "kuota knowledge penuh")
			failedN++
			continue
		}
		setPageStatus(p.ID, "training", "")

		faqs, err := services.GenerateWebFAQ(p.Title, p.Content)
		if err != nil {
			errMsg := webTrainingErrorMessage(err)
			setPageStatus(p.ID, "failed", errMsg)
			failedN++
			log.Printf("[train] FAQ gagal page %d (%s): %v", p.ID, p.URL, err)
			continue
		}
		if len(faqs) == 0 {
			setPageStatus(p.ID, "skipped", "tidak ada info berguna untuk pelanggan")
			skippedN++
			log.Printf("[train] page %d (%s) -> dilewati (tak ada info berguna)", p.ID, p.URL)
			continue
		}

		added := 0
		for _, f := range faqs {
			ans := strings.TrimSpace(f.Answer)
			if ans == "" {
				continue
			}
			if maxChars > 0 && used+int64(len([]rune(ans))) > int64(maxChars) {
				break // kuota habis
			}
			beforeChars := 0
			if existing := writer.byQuestion[canonicalKnowledgeQuestion(f.Question)]; existing != nil {
				beforeChars = existing.CharCount
			}
			stored, covered, saveErr := writer.save(f.Question, ans, f.Tags, "web", p.URL)
			if saveErr != nil {
				log.Printf("[train] gagal simpan FAQ page %d: %v", p.ID, saveErr)
				continue
			}
			if !covered {
				continue
			}
			used += int64(stored.CharCount - beforeChars)
			added++
		}
		now := time.Now()
		st := "trained"
		errMsg := ""
		if added == 0 {
			st, errMsg = "failed", "kuota knowledge penuh"
			failedN++
		} else {
			trainedN++
			totalFAQ += added
		}
		log.Printf("[train] page %d (%s) -> %s (%d FAQ)", p.ID, p.URL, st, added)
		database.DB.Model(&models.CrawlPage{}).Where("id = ?", p.ID).
			Updates(map[string]any{"status": st, "error": errMsg, "trained_at": &now})
	}
	services.InvalidateKB(agentID)
	log.Printf("[train] SELESAI agent %d job %d: %d dilatih (%d FAQ), %d dilewati, %d gagal", agentID, jobID, trainedN, totalFAQ, skippedN, failedN)
	// Persona otomatis hanya bila pelatihan tuntas (kalau di-Stop, jangan boros panggil AI lagi).
	if !stopped && trainedN > 0 && updatePersona {
		if err := refreshPersonaFromJob(agentID, jobID); err != nil {
			msg := webTrainingErrorMessage(err)
			database.DB.Model(&models.CrawlJob{}).Where("id = ?", jobID).Update("persona_error", msg)
			log.Printf("[persona] auto-update agent %d gagal: %v", agentID, err)
		} else {
			database.DB.Model(&models.CrawlJob{}).Where("id = ?", jobID).
				Updates(map[string]any{"persona_updated": true, "persona_error": ""})
		}
	}
}

func webTrainingErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "api key") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "401"):
		return "API key AI belum diisi atau tidak valid"
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "quota") || strings.Contains(msg, "credit"):
		return "Kuota/kredit provider AI habis atau sedang dibatasi"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "Provider AI terlalu lama merespons"
	default:
		return "AI gagal mengolah konten halaman. Periksa konfigurasi provider lalu coba lagi"
	}
}

func setPageStatus(pageID uint, status, errMsg string) {
	database.DB.Model(&models.CrawlPage{}).Where("id = ?", pageID).
		Updates(map[string]any{"status": status, "error": errMsg})
}

// jobStatusIs membaca status job terkini dari DB (dipakai untuk mendeteksi permintaan Stop saat training).
func jobStatusIs(jobID uint, status string) bool {
	var s string
	database.DB.Model(&models.CrawlJob{}).Where("id = ?", jobID).Select("status").Scan(&s)
	return s == status
}

// StopTraining menandai job pelatihan agar berhenti rapi pada halaman berikutnya.
func StopTraining(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	var job models.CrawlJob
	if database.DB.Where("agent_id = ?", aid).First(&job, c.Param("jobId")).Error != nil {
		c.JSON(404, gin.H{"error": "Job tidak ditemukan"})
		return
	}
	if job.Status != "training" {
		c.JSON(409, gin.H{"error": "Tidak ada pelatihan yang sedang berjalan"})
		return
	}
	database.DB.Model(&job).Update("status", "stopping")
	c.JSON(200, gin.H{"stopping": true})
}

// KnowledgeUsage menampilkan pemakaian kuota knowledge agent (untuk UI).
func KnowledgeUsage(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	maxChars, maxPages := crawlLimitsForTenant(currentTenantID(c))
	var total int64
	database.DB.Model(&models.Knowledge{}).Where("agent_id = ?", aid).Count(&total)
	var embedded int64
	database.DB.Model(&models.Knowledge{}).
		Where("agent_id = ? AND embedding IS NOT NULL AND embedding <> ''", aid).Count(&embedded)
	c.JSON(200, gin.H{
		"used_chars": knowledgeCharsUsed(aid), "max_chars": maxChars,
		"max_pages": maxPages, "total_knowledge": total,
		"semantic_search": services.EmbeddingEnabled(), "embedded_knowledge": embedded,
	})
}

// refreshPersonaFromJob membuat atau memperbarui persona dari halaman yang berhasil dilatih.
func refreshPersonaFromJob(agentID, jobID uint) error {
	persona := buildPersonaFromJob(agentID, jobID)
	if persona == "" {
		return fmt.Errorf("konten terlatih belum cukup untuk membuat persona")
	}
	if err := database.DB.Model(&models.Agent{}).Where("id = ?", agentID).Update("system_prompt", persona).Error; err != nil {
		return err
	}
	log.Printf("[persona] auto-generate untuk agent %d (%d karakter)", agentID, len([]rune(persona)))
	return nil
}

// RegeneratePersona membuat ulang persona dari job crawl terakhir (dipicu manual oleh user dari UI).
func RegeneratePersona(c *gin.Context) {
	aid, ok := resolveAgent(c)
	if !ok {
		return
	}
	// Generate persona pakai AI — kenakan ke kuota AI bulanan tenant.
	var job models.CrawlJob
	if database.DB.Where("agent_id = ?", aid).Order("id desc").First(&job).Error != nil {
		c.JSON(400, gin.H{"error": "Belum ada data website. Latih dari website dulu."})
		return
	}
	if err := refreshPersonaFromJob(aid, job.ID); err != nil {
		c.JSON(502, gin.H{"error": "Gagal membuat persona dari konten web. Coba lagi."})
		return
	}
	var agent models.Agent
	database.DB.Select("system_prompt").First(&agent, aid)
	persona := agent.SystemPrompt
	c.JSON(200, gin.H{"system_prompt": persona})
}

// buildPersonaFromJob menyusun persona dari halaman terkaya pada satu job (prioritas Home/About).
func buildPersonaFromJob(agentID, jobID uint) string {
	var pages []models.CrawlPage
	database.DB.Where("agent_id = ? AND job_id = ? AND status = ? AND char_count >= ?", agentID, jobID, "trained", 100).
		Order("char_count desc").Find(&pages)
	if len(pages) == 0 {
		return ""
	}
	persona, err := services.GenerateWebPersona(pickPersonaSamples(pages))
	if err != nil {
		log.Printf("[persona] gagal generate agent %d: %v", agentID, err)
		return ""
	}
	return persona
}

// pickPersonaSamples memilih maksimal 3 cuplikan konten paling relevan untuk persona (Home/About dulu).
func pickPersonaSamples(pages []models.CrawlPage) []string {
	var home, about, rest []models.CrawlPage
	for _, p := range pages {
		lu := strings.ToLower(p.URL)
		switch {
		case isHomeURL(p.URL):
			home = append(home, p)
		case strings.Contains(lu, "about") || strings.Contains(lu, "tentang") ||
			strings.Contains(lu, "profil") || strings.Contains(lu, "company") ||
			strings.Contains(lu, "perusahaan"):
			about = append(about, p)
		default:
			rest = append(rest, p)
		}
	}
	ordered := append(append(home, about...), rest...)

	var samples []string
	for _, p := range ordered {
		if len(samples) >= 3 {
			break
		}
		c := p.Content
		if len([]rune(c)) > 1500 {
			c = string([]rune(c)[:1500])
		}
		if strings.TrimSpace(c) != "" {
			samples = append(samples, p.Title+"\n"+c)
		}
	}
	return samples
}

// isHomeURL true bila URL menunjuk ke halaman beranda (tanpa path atau hanya "/").
func isHomeURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Path == "" || u.Path == "/"
}
