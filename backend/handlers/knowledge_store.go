package handlers

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"wa-assistant/backend/database"
	"wa-assistant/backend/models"
	"wa-assistant/backend/services"
)

type knowledgeUpserter struct {
	agentID    uint
	byQuestion map[string]*models.Knowledge
	rows       []*models.Knowledge
}

func newKnowledgeUpserter(agentID uint) *knowledgeUpserter {
	var rows []models.Knowledge
	database.DB.Where("agent_id = ?", agentID).Find(&rows)
	items := make(map[string]*models.Knowledge, len(rows))
	for i := range rows {
		row := &rows[i]
		items[canonicalKnowledgeQuestion(row.Question)] = row
	}
	pointers := make([]*models.Knowledge, 0, len(rows))
	for i := range rows {
		pointers = append(pointers, &rows[i])
	}
	return &knowledgeUpserter{agentID: agentID, byQuestion: items, rows: pointers}
}

// save menyimpan satu FAQ secara idempotent. FAQ manual tidak pernah ditimpa generator.
// Return covered=true bila informasi sudah tersedia atau berhasil disimpan.
func (u *knowledgeUpserter) save(question, answer, tags, source, sourceURL string) (models.Knowledge, bool, error) {
	question = strings.TrimSpace(question)
	answer = strings.TrimSpace(answer)
	if question == "" || answer == "" {
		return models.Knowledge{}, false, nil
	}
	if len([]rune(question)) > 500 {
		question = string([]rune(question)[:500])
	}
	if len([]rune(answer)) > 12000 {
		answer = string([]rune(answer)[:12000])
	}
	if source == "" {
		source = "manual"
	}
	tags = normalizeKnowledgeTags(tags, source)
	key := canonicalKnowledgeQuestion(question)
	existing := u.findDuplicate(question, answer)
	if existing != nil {
		// Pertanyaan manual adalah sumber paling disengaja/presisi. Hasil AI cukup menganggap
		// topiknya sudah tercakup dan tidak mengganti jawaban user.
		if (existing.Source == "" || existing.Source == "manual") && source != "manual" {
			return *existing, true, nil
		}
		// Generator dengan prioritas lebih rendah tidak boleh mengganti fakta dari sumber
		// yang lebih disengaja. Urutan: manual > Tulis Info > import > Setup Cepat > web.
		if knowledgeSourcePriority(source) < knowledgeSourcePriority(existing.Source) {
			return *existing, true, nil
		}
		existing.Question = question
		existing.Answer = answer
		existing.Tags = tags
		existing.Source = source
		existing.SourceURL = sourceURL
		if err := database.DB.Save(existing).Error; err != nil {
			return models.Knowledge{}, false, err
		}
		services.IndexKnowledge(existing)
		u.byQuestion[key] = existing
		return *existing, true, nil
	}

	item := models.Knowledge{
		AgentID: u.agentID, Question: question, Answer: answer, Tags: tags,
		Source: source, SourceURL: sourceURL,
	}
	if err := database.DB.Create(&item).Error; err != nil {
		return models.Knowledge{}, false, fmt.Errorf("simpan knowledge: %w", err)
	}
	u.byQuestion[key] = &item
	u.rows = append(u.rows, &item)
	services.IndexKnowledge(&item)
	return item, true, nil
}

func (u *knowledgeUpserter) findDuplicate(question, answer string) *models.Knowledge {
	if exact := u.byQuestion[canonicalKnowledgeQuestion(question)]; exact != nil {
		return exact
	}
	for _, row := range u.rows {
		if knowledgeLooksDuplicate(row.Question, row.Answer, question, answer) {
			return row
		}
	}
	return nil
}

func knowledgeSourcePriority(source string) int {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "manual", "":
		return 100
	case "text":
		return 80
	case "import":
		return 75
	case "wizard":
		return 70
	case "web":
		return 60
	default:
		return 50
	}
}

var knowledgeQuestionStopwords = map[string]bool{
	"apa": true, "apakah": true, "berapa": true, "bagaimana": true, "gimana": true,
	"yang": true, "ini": true, "itu": true, "saja": true, "aja": true, "ya": true,
	"dong": true, "kak": true, "min": true, "mohon": true, "tolong": true,
	"melakukan": true, "untuk": true, "dengan": true, "dari": true, "adalah": true,
	"ke": true, "di": true,
}

var knowledgeQuestionAliases = map[string]string{
	"biaya": "harga", "tarif": "harga",
	"order": "pesan", "pemesanan": "pesan", "beli": "pesan", "pembelian": "pesan",
	"ongkir": "pengiriman", "kirim": "pengiriman", "dikirim": "pengiriman",
	"alamat": "lokasi", "tempat": "lokasi",
	"dijual": "tersedia", "menjual": "tersedia",
}

func knowledgeQuestionTokens(value string) []string {
	value = strings.NewReplacer(
		"biaya kirim", "ongkir",
		"ongkos kirim", "ongkir",
	).Replace(strings.ToLower(value))
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(parts))
	for _, token := range parts {
		for _, suffix := range []string{"nya", "kah", "lah"} {
			if strings.HasSuffix(token, suffix) && len([]rune(token)) > len([]rune(suffix))+3 {
				token = strings.TrimSuffix(token, suffix)
				break
			}
		}
		if alias := knowledgeQuestionAliases[token]; alias != "" {
			token = alias
		}
		if token == "" || knowledgeQuestionStopwords[token] || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	sort.Strings(out)
	return out
}

func normalizedKnowledgeAnswer(value string) string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	return strings.Join(parts, " ")
}

func knowledgeLooksDuplicate(leftQuestion, leftAnswer, rightQuestion, rightAnswer string) bool {
	leftTokens := knowledgeQuestionTokens(leftQuestion)
	rightTokens := knowledgeQuestionTokens(rightQuestion)
	if len(leftTokens) > 0 && strings.Join(leftTokens, " ") == strings.Join(rightTokens, " ") {
		return true
	}
	leftAnswerKey := normalizedKnowledgeAnswer(leftAnswer)
	rightAnswerKey := normalizedKnowledgeAnswer(rightAnswer)
	if leftAnswerKey != "" && leftAnswerKey == rightAnswerKey {
		return true
	}
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return false
	}
	leftSet := map[string]bool{}
	for _, token := range leftTokens {
		leftSet[token] = true
	}
	intersection := 0
	for _, token := range rightTokens {
		if leftSet[token] {
			intersection++
		}
	}
	union := len(leftTokens) + len(rightTokens) - intersection
	// Izinkan variasi yang hanya menambahkan kata generik, mis. "cara pesan"
	// dibanding "cara pesan produk", tetapi jangan gabungkan nama produk berbeda.
	if intersection >= 2 && intersection == minKnowledgeInt(len(leftTokens), len(rightTokens)) {
		generic := map[string]bool{"produk": true, "layanan": true, "barang": true, "bisnis": true}
		allExtrasGeneric := true
		for _, token := range append(append([]string{}, leftTokens...), rightTokens...) {
			if !leftSet[token] || !containsKnowledgeToken(rightTokens, token) {
				if !generic[token] {
					allExtrasGeneric = false
					break
				}
			}
		}
		if allExtrasGeneric {
			return true
		}
	}
	return len(leftTokens) >= 3 && len(rightTokens) >= 3 && intersection >= 3 && union > 0 && float64(intersection)/float64(union) >= 0.8
}

func containsKnowledgeToken(tokens []string, wanted string) bool {
	for _, token := range tokens {
		if token == wanted {
			return true
		}
	}
	return false
}

func minKnowledgeInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ConsolidateAllKnowledge merapikan duplikat lama secara deterministik saat startup.
// Sumber paling tepercaya dipertahankan; record lain dihapus dan tag-nya digabung.
func ConsolidateAllKnowledge() {
	var agentIDs []uint
	database.DB.Model(&models.Knowledge{}).Distinct("agent_id").Pluck("agent_id", &agentIDs)
	for _, agentID := range agentIDs {
		consolidateKnowledgeForAgent(agentID)
	}
}

func consolidateKnowledgeForAgent(agentID uint) {
	var rows []models.Knowledge
	database.DB.Where("agent_id = ?", agentID).Find(&rows)
	sort.SliceStable(rows, func(i, j int) bool {
		pi, pj := knowledgeSourcePriority(rows[i].Source), knowledgeSourcePriority(rows[j].Source)
		if pi != pj {
			return pi > pj
		}
		return rows[i].ID > rows[j].ID
	})
	kept := make([]*models.Knowledge, 0, len(rows))
	removed := 0
	for i := range rows {
		row := &rows[i]
		var duplicate *models.Knowledge
		for _, candidate := range kept {
			if knowledgeLooksDuplicate(candidate.Question, candidate.Answer, row.Question, row.Answer) {
				duplicate = candidate
				break
			}
		}
		if duplicate == nil {
			kept = append(kept, row)
			continue
		}
		mergedTags := normalizeKnowledgeTags(duplicate.Tags+","+row.Tags, duplicate.Source)
		if mergedTags != duplicate.Tags {
			database.DB.Model(duplicate).Updates(map[string]any{"tags": mergedTags, "embedding": "", "embedding_model": ""})
			duplicate.Tags = mergedTags
		}
		if database.DB.Delete(row).Error == nil {
			removed++
		}
	}
	if removed > 0 {
		services.InvalidateKB(agentID)
	}
}

func canonicalKnowledgeQuestion(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	// Semua tanda baca diperlakukan sebagai pemisah supaya variasi seperti
	// "harga/produk", "harga - produk", dan "harga produk?" tidak tersimpan dobel.
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsPunct(r) || unicode.IsSpace(r)
	})
	return strings.Join(parts, " ")
}

func normalizeKnowledgeTags(raw, source string) string {
	seen := map[string]bool{}
	out := make([]string, 0, 6)
	for _, tag := range append(strings.Split(raw, ","), source) {
		tag = strings.ToLower(strings.TrimSpace(tag))
		tag = strings.Join(strings.Fields(tag), " ")
		if tag == "" || seen[tag] {
			continue
		}
		if len([]rune(tag)) > 40 {
			tag = string([]rune(tag)[:40])
		}
		seen[tag] = true
		out = append(out, tag)
		if len(out) == 8 {
			break
		}
	}
	return strings.Join(out, ",")
}
