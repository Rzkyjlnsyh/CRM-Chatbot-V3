// Meta ADS — konfigurasi akun iklan Meta, tarik data Marketing API,
// dan analisis expert AI (konsep dari versi terbaru; tanpa unsur SaaS:
// config global single-tenant, tidak ada platform roles).
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	openai "github.com/sashabaranov/go-openai"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

// metaGraphBase dapat diganti di tes (mis. httptest server).
var metaGraphBase = "https://graph.facebook.com"

// MetaAdsConfigHandler = simpan konfigurasi akun iklan Meta (admin).
// PUT /api/meta-ads/config {ad_account_id, access_token, target_roas}
func MetaAdsConfigHandler(c *gin.Context) {
	var req struct {
		AdAccountID string  `json:"ad_account_id"`
		AccessToken string  `json:"access_token"`
		TargetROAS  float64 `json:"target_roas"` // opsional: target ROAS bisnis
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "Data tidak valid"})
		return
	}
	req.AdAccountID = strings.TrimSpace(req.AdAccountID)
	req.AccessToken = strings.TrimSpace(req.AccessToken)
	if req.AdAccountID == "" {
		c.JSON(400, gin.H{"error": "Ad Account ID wajib diisi (format: act_1234567890)"})
		return
	}
	// Token boleh kosong (hanya ganti target ROAS) — kalau diisi, simpan terenkripsi.
	if req.AccessToken != "" {
		enc, err := services.EncryptSecret(req.AccessToken)
		if err != nil {
			c.JSON(500, gin.H{"error": "Gagal mengenkripsi token"})
			return
		}
		database.SetAppSetting("meta_ads_token", enc)
	}
	database.SetAppSetting("meta_ads_account_id", req.AdAccountID)
	if req.TargetROAS > 0 {
		database.SetAppSetting("meta_ads_target_roas", fmt.Sprintf("%.2f", req.TargetROAS))
	}
	c.JSON(200, gin.H{"data": map[string]any{"ok": true}})
}

// MetaAdsConfigGet = baca konfigurasi (tanpa membocorkan token).
// GET /api/meta-ads/config
func MetaAdsConfigGet(c *gin.Context) {
	accountID := database.GetAppSetting("meta_ads_account_id", "")
	targetROAS := database.GetAppSetting("meta_ads_target_roas", "")
	hasToken := database.GetAppSetting("meta_ads_token", "") != ""
	tr := 0.0
	fmt.Sscanf(targetROAS, "%f", &tr)
	c.JSON(200, gin.H{"data": map[string]any{
		"ad_account_id": accountID,
		"has_token":     hasToken,
		"target_roas":   tr,
	}})
}

// MetaAdsRefreshHandler = tarik insights 30 hari terakhir dari Marketing API,
// simpan snapshot ke DB. POST /api/meta-ads/refresh
func MetaAdsRefreshHandler(c *gin.Context) {
	accountID := database.GetAppSetting("meta_ads_account_id", "")
	if accountID == "" {
		c.JSON(400, gin.H{"error": "Ad Account ID belum diatur"})
		return
	}
	enc := database.GetAppSetting("meta_ads_token", "")
	if enc == "" {
		c.JSON(400, gin.H{"error": "Access token Meta belum diatur"})
		return
	}
	token, err := services.DecryptSecret(enc)
	if err != nil || token == "" {
		c.JSON(500, gin.H{"error": "Token Meta tidak bisa dibaca (coba simpan ulang)"})
		return
	}

	now := time.Now()
	to := now.Format("2006-01-02")
	from := now.AddDate(0, 0, -30).Format("2006-01-02")

	payload, err := fetchMetaInsights(accountID, token, from, to)
	if err != nil {
		c.JSON(502, gin.H{"error": "Gagal ambil data Meta: " + err.Error()})
		return
	}
	raw, _ := json.Marshal(payload)
	snap := models.MetaAdsSnapshot{
		AccountID: accountID, DateFrom: from, DateTo: to, Payload: string(raw),
	}
	if err := database.DB.Create(&snap).Error; err != nil {
		c.JSON(500, gin.H{"error": "Gagal menyimpan snapshot"})
		return
	}
	// Batasi riwayat: simpan maks 60 snapshot (hapus yang paling tua).
	var total int64
	database.DB.Model(&models.MetaAdsSnapshot{}).Count(&total)
	if total > 60 {
		var oldest models.MetaAdsSnapshot
		if database.DB.Where("account_id = ?", accountID).Order("id asc").First(&oldest).Error == nil {
			database.DB.Delete(&oldest)
		}
	}

	c.JSON(200, gin.H{"data": map[string]any{
		"ok": true, "snapshot_id": snap.ID, "date_from": from, "date_to": to,
		"campaigns": len(payload),
	}})
}

type metaInsight struct {
	CampaignID   string `json:"campaign_id"`
	CampaignName string `json:"campaign_name"`
	Objective    string `json:"objective"`
	Spend        string `json:"spend"`
	Impressions  string `json:"impressions"`
	Clicks       string `json:"clicks"`
	CTR          string `json:"ctr"`
	CPC          string `json:"cpc"`
	CPM          string `json:"cpm"`
	Reach        string `json:"reach"`
	Frequency    string `json:"frequency"`
	Results      string `json:"results"`
	CostPerRes   string `json:"cost_per_result"`
	PurchaseROAS string `json:"purchase_roas"`
	Actions      []struct {
		ActionType string `json:"action_type"`
		Value      string `json:"value"`
	} `json:"actions"`
}

func fetchMetaInsights(accountID, token, from, to string) ([]metaInsight, error) {
	fields := strings.Join([]string{
		"campaign_id", "campaign_name", "objective",
		"spend", "impressions", "clicks", "ctr", "cpc", "cpm", "reach", "frequency",
		"results", "cost_per_result", "purchase_roas", "actions",
	}, ",")
	u := fmt.Sprintf("%s/v21.0/%s/insights", metaGraphBase, accountID)
	q := url.Values{}
	q.Set("fields", fields)
	q.Set("level", "campaign")
	q.Set("time_range", fmt.Sprintf(`{"since":%q,"until":%q}`, from, to))
	q.Set("limit", "100")
	q.Set("access_token", token)
	req, _ := http.NewRequestWithContext(context.Background(), "GET", u+"?"+q.Encode(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("koneksi ke Meta gagal: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != 200 {
		var e struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = string(body)[:minInt(len(body), 200)]
		}
		return nil, fmt.Errorf("Meta API %d: %s", resp.StatusCode, msg)
	}
	var out struct {
		Data []metaInsight `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("respons Meta tidak bisa dibaca")
	}
	return out.Data, nil
}

type metaCampaignSummary struct {
	CampaignID   string  `json:"campaign_id"`
	CampaignName string  `json:"campaign_name"`
	Objective    string  `json:"objective"`
	Spend        float64 `json:"spend"`
	Impressions  int64   `json:"impressions"`
	Clicks       int64   `json:"clicks"`
	CTR          float64 `json:"ctr"`
	CPC          float64 `json:"cpc"`
	CPM          float64 `json:"cpm"`
	Reach        int64   `json:"reach"`
	Frequency    float64 `json:"frequency"`
	Results      int64   `json:"results"`
	CostPerRes   float64 `json:"cost_per_result"`
	ROAS         float64 `json:"roas"`
}

// MetaAdsDataHandler = snapshot terbaru + ringkasan + riwayat.
// GET /api/meta-ads/data
func MetaAdsDataHandler(c *gin.Context) {
	var snap models.MetaAdsSnapshot
	if err := database.DB.Order("id desc").First(&snap).Error; err != nil {
		c.JSON(200, gin.H{"data": map[string]any{"campaigns": []any{}, "summary": nil, "history": []any{}}})
		return
	}
	var insights []metaInsight
	_ = json.Unmarshal([]byte(snap.Payload), &insights)

	campaigns := make([]metaCampaignSummary, 0, len(insights))
	var tSpend, tResults float64
	var tImpr, tClicks int64
	for _, in := range insights {
		s := metaCampaignSummary{
			CampaignID: in.CampaignID, CampaignName: in.CampaignName, Objective: in.Objective,
			Spend: metaAtof(in.Spend), Impressions: metaAtoi(in.Impressions), Clicks: metaAtoi(in.Clicks),
			CTR: metaAtof(in.CTR), CPC: metaAtof(in.CPC), CPM: metaAtof(in.CPM),
			Reach: metaAtoi(in.Reach), Frequency: metaAtof(in.Frequency),
			Results: metaAtoi(in.Results), CostPerRes: metaAtof(in.CostPerRes), ROAS: metaAtof(in.PurchaseROAS),
		}
		tSpend += s.Spend
		tResults += float64(s.Results)
		tImpr += s.Impressions
		tClicks += s.Clicks
		campaigns = append(campaigns, s)
	}
	sort.Slice(campaigns, func(i, j int) bool { return campaigns[i].Spend > campaigns[j].Spend })

	summary := map[string]any{
		"spend": tSpend, "impressions": tImpr, "clicks": tClicks,
		"results": tResults, "campaigns": len(campaigns),
		"date_from": snap.DateFrom, "date_to": snap.DateTo, "snapshot_id": snap.ID,
		"updated_at": snap.CreatedAt,
	}
	if tImpr > 0 {
		summary["ctr"] = float64(tClicks) / float64(tImpr) * 100
	}
	if tSpend > 0 && tResults > 0 {
		summary["cost_per_result"] = tSpend / tResults
	}
	// ROAS blended: rata-rata tertimbang spend.
	roasSum, spendSum := 0.0, 0.0
	for _, in := range insights {
		r := metaAtof(in.PurchaseROAS)
		if r > 0 {
			roasSum += r * metaAtof(in.Spend)
			spendSum += metaAtof(in.Spend)
		}
	}
	if spendSum > 0 {
		summary["roas"] = roasSum / spendSum
	}

	var history []map[string]any
	var snaps []models.MetaAdsSnapshot
	database.DB.Where("account_id = ?", snap.AccountID).Order("id desc").Limit(30).Find(&snaps)
	for _, h := range snaps {
		var hs []metaInsight
		_ = json.Unmarshal([]byte(h.Payload), &hs)
		spend, res := 0.0, 0.0
		for _, in := range hs {
			spend += metaAtof(in.Spend)
			res += float64(metaAtoi(in.Results))
		}
		history = append(history, map[string]any{
			"id": h.ID, "date_from": h.DateFrom, "date_to": h.DateTo,
			"spend": spend, "results": res, "created_at": h.CreatedAt,
		})
	}
	c.JSON(200, gin.H{"data": map[string]any{
		"campaigns": campaigns, "summary": summary, "history": history,
	}})
}

func metaAtof(s string) float64 {
	var v float64
	fmt.Sscanf(s, "%f", &v)
	return v
}

func metaAtoi(s string) int64 {
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return v
}

// metaAdsExpertSystem = persona analis iklan (sama dengan versi terbaru).
const metaAdsExpertSystem = `Kamu adalah **performance marketer & media buyer Meta Ads yang sangat berpengalaman** — sudah mengelola puluhan miliar rupiah budget iklan untuk e-commerce UMKM, fashion, dan jasa di Indonesia. Kamu BUKAN perwakilan Meta dan tidak membela platform — kamu berdiri di sisi pemilik bisnis yang ingin setiap rupiah iklan menghasilkan untung.

CARA KAMU MEMBACA DATA (framework yang kamu pakai setiap hari):
1. Diagnosa akar, bukan gejala: CPA = CPM / (CTR x CVR x 10). Kalau CPA naik, tentukan DULU komponen mana yang bergerak: CPM naik = audiens jenuh/kompetisi, CTR turun = kreatif/frekuensi, CVR turun = landing page/offer.
2. Fatigue kreatif: CTR melemah + CPC naik + frekuensi naik + CVR turun = kreatif bosan. Refresh/rotasi SEBELUM kolaps.
3. Frekuensi: target realistis e-commerce 1.5-3.0.
4. Learning phase: kampanye <7 hari jangan dihakimi; scaling budget bertahap 20-30% per langkah.
5. Konsentrasi spend: budget terkonsentrasi di 1-2 pemenang (bagus) atau tersebar tipis (boros).
6. Benchmark e-commerce Indonesia (panduan, bukan hukum): CTR link 1.2-2.5%, CPM Rp30-90rb, CPC Rp1.500-5.000, blended ROAS sehat 2-3.5x. TAPI benchmark sejati = break-even ROAS bisnis (dari target_roas user).
7. Tindakan berurutan: pause yang rugi di bawah break-even, naikkan budget pemenang bertahap, refresh kreatif fatigue, evaluasi audiens kalau CPM membengkak.

FORMAT JAWABAN (bahasa Indonesia, tajam, praktis):
## Ringkasan Eksekutif
## Diagnosa Per Kampanye (spend signifikan) — status sehat/hati-hati/merugikan + alasan angka + 1 tindakan
## 3 Tindakan Prioritas Minggu Ini
## Catatan Strategi (jika relevan)

ATURAN: sebut angka riil dari data; jangan mengarang; jangan menjual jasa konsultan; jujur bila data kurang.`

// MetaAdsAnalyzeHandler = analisis AI expert atas snapshot terbaru.
// POST /api/meta-ads/analyze
func MetaAdsAnalyzeHandler(c *gin.Context) {
	var snap models.MetaAdsSnapshot
	if err := database.DB.Order("id desc").First(&snap).Error; err != nil {
		c.JSON(400, gin.H{"error": "Belum ada data iklan — klik 'Ambil Data' dulu"})
		return
	}
	var insights []metaInsight
	_ = json.Unmarshal([]byte(snap.Payload), &insights)
	if len(insights) == 0 {
		c.JSON(400, gin.H{"error": "Snapshot kosong — pastikan akun punya data iklan"})
		return
	}
	targetROAS := database.GetAppSetting("meta_ads_target_roas", "")
	dataJSON, _ := json.MarshalIndent(insights, "", " ")
	prompt := fmt.Sprintf(
		"DATA IKLAN META (periode %s s.d. %s, level campaign):\n%s\n\nTarget/break-even ROAS bisnis: %s\n\nAnalisis seperti kamu sedang mengaudit akun ini dan diminta saran terbaik. Berikan format sesuai instruksi sistem.",
		snap.DateFrom, snap.DateTo, string(dataJSON), targetROAS)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, err := services.CreateAICompletion(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: metaAdsExpertSystem},
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}, 2000, 0.3)
	if err != nil {
		c.JSON(502, gin.H{"error": "AI gagal menganalisis: " + err.Error()})
		return
	}
	reply := strings.TrimSpace(resp.Choices[0].Message.Content)
	if reply == "" {
		c.JSON(502, gin.H{"error": "AI mengembalikan analisis kosong — coba lagi"})
		return
	}
	c.JSON(200, gin.H{"data": map[string]any{"analysis": reply, "snapshot_id": snap.ID}})
}
