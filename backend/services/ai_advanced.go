package services

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"wa-assistant/backend/models"
)

// Prompt layering v2 — prioritas fakta & anti-konflik persona/knowledge.
const (
	personaMaxRunes     = 1600
	knowledgeSelectTopK = 5 // ambil kandidat, lalu resolve konflik → max topK final
	// Bobot hybrid final (jumlah ~1). Semantic menangkap parafrase; keyword nama/kode.
	advWSemantic  = 0.48
	advWKeyword   = 0.30
	advWSource    = 0.12
	advWFreshness = 0.10
	// Tanpa embedding: keyword + prioritas sumber.
	advWKeywordOnly = 0.72
	advWSourceOnly  = 0.16
	advWFreshOnly   = 0.12
	// Ambang skor hybrid final (0–1-ish).
	advSelectMin   = 0.26
	advSelectFloor = 0.18
)

type scoredKnowledgeAdv struct {
	k        models.Knowledge
	score    float64
	sim      float32
	kw       float64
	sourceB  float64
	freshB   float64
	modeHint string
}

// trimPersonaForPrompt memotong persona panjang di batas kalimat agar tidak
// menekan knowledge di context window, sambil menjaga potongan terbaca.
func trimPersonaForPrompt(persona string) string {
	persona = strings.TrimSpace(persona)
	if persona == "" {
		return ""
	}
	// Buang spasi berlebih / baris kosong beruntun.
	lines := strings.Split(persona, "\n")
	var cleaned []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			if len(cleaned) > 0 && cleaned[len(cleaned)-1] != "" {
				cleaned = append(cleaned, "")
			}
			continue
		}
		cleaned = append(cleaned, ln)
	}
	persona = strings.TrimSpace(strings.Join(cleaned, "\n"))
	r := []rune(persona)
	if len(r) <= personaMaxRunes {
		return persona
	}
	cut := r[:personaMaxRunes]
	// Mundur ke akhir kalimat / baris agar tidak putus di tengah kata.
	best := -1
	for i := len(cut) - 1; i >= personaMaxRunes*2/3; i-- {
		switch cut[i] {
		case '.', '!', '?', '\n':
			best = i + 1
			goto done
		}
	}
	// Fallback: spasi terakhir.
	for i := len(cut) - 1; i >= personaMaxRunes*2/3; i-- {
		if unicode.IsSpace(cut[i]) {
			best = i
			break
		}
	}
done:
	if best > 0 {
		cut = cut[:best]
	}
	out := strings.TrimSpace(string(cut))
	if out == "" {
		return string(r[:personaMaxRunes])
	}
	return out + "\n\n[Catatan internal: persona dipotong agar fakta knowledge/produk tetap prioritas.]"
}

// factPriorityInstruction = lapisan eksplisit saat persona + knowledge hidup bersama.
func factPriorityInstruction() string {
	return `

PRIORITAS FAKTA (wajib, tidak bisa diganti persona):
1) BASIS PENGETAHUAN PRODUK AKTIF (jika ada) menang atas semua sumber lain untuk produk itu.
2) BASIS PENGETAHUAN TERPILIH menang atas persona untuk angka, harga, jam, syarat, stok, alamat, kebijakan.
3) Persona hanya untuk identitas, batasan layanan, dan cara melayani — BUKAN sumber angka/harga/jam.
4) Tone hanya mengatur GAYA BAHASA, bukan fakta.
5) Jika dua sumber knowledge saling bertentangan, ikuti yang berlabel prioritas lebih tinggi / lebih baru; jika masih ragu, katakan bagian itu belum bisa dipastikan (jangan tebak).`
}

// selectKnowledgeAdvanced = retrieval hybrid multi-sinyal + dedupe + resolusi konflik angka.
// Mode: none | keyword | semantic | hybrid | hybrid_conflict_resolved
func selectKnowledgeAdvanced(msg string, items []KBItem) ([]models.Knowledge, string, float64) {
	if len(items) == 0 {
		return nil, "none", 0
	}
	qTokens := tokenizeQuery(msg)
	hasEmbed := EmbeddingEnabled()

	var qVec []float32
	if hasEmbed {
		if vec, err := Embed(msg); err == nil {
			qVec = vec
		} else {
			log.Printf("Embedding: query gagal di advanced select, lanjut keyword: %v", err)
		}
	}

	ranked := make([]scoredKnowledgeAdv, 0, len(items))
	for _, it := range items {
		kw := keywordScoreRaw(qTokens, it.K)
		sim := float32(0)
		if len(qVec) > 0 && len(it.Vec) == len(qVec) {
			sim = cosineSim(qVec, it.Vec)
		}
		srcB := sourcePriorityBoost(it.K.Source)
		frB := freshnessBoost(it.K.CreatedAt)
		score := hybridKnowledgeScore(float64(sim), kw, srcB, frB, len(qVec) > 0)
		// Tanpa sinyal sama sekali → skip.
		if score <= 0 && kw <= 0 && sim < simFloor {
			continue
		}
		// Harus ada minimal jejak relevansi.
		if kw <= 0 && sim < simFloor {
			continue
		}
		ranked = append(ranked, scoredKnowledgeAdv{
			k: it.K, score: score, sim: sim, kw: kw, sourceB: srcB, freshB: frB,
		})
	}
	if len(ranked) == 0 {
		// Fallback murni keywordSearch lama agar tidak regressed.
		kwItems := keywordSearch(msg, items)
		if len(kwItems) == 0 {
			return nil, "none", 0
		}
		return kwItems, "keyword", 0
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			if ranked[i].sim == ranked[j].sim {
				return ranked[i].k.CreatedAt.After(ranked[j].k.CreatedAt)
			}
			return ranked[i].sim > ranked[j].sim
		}
		return ranked[i].score > ranked[j].score
	})

	// Relative floor vs best.
	best := ranked[0].score
	minKeep := math.Max(advSelectMin, best-0.22)
	if best < advSelectMin {
		minKeep = advSelectFloor
	}

	candidates := make([]scoredKnowledgeAdv, 0, knowledgeSelectTopK)
	for _, r := range ranked {
		if len(candidates) >= knowledgeSelectTopK {
			break
		}
		if r.score < minKeep && len(candidates) > 0 {
			break
		}
		if r.score < advSelectFloor {
			break
		}
		candidates = append(candidates, r)
	}
	if len(candidates) == 0 && ranked[0].score >= advSelectFloor {
		candidates = append(candidates, ranked[0])
	}

	// Dedupe jawaban mirip sebelum conflict resolve.
	candidates = dedupeScoredKnowledge(candidates)

	// Resolusi konflik angka antar FAQ topik sama: simpan yang skor lebih tinggi.
	resolved, dropped := resolveKnowledgeConflicts(candidates)
	if len(resolved) > topK {
		resolved = resolved[:topK]
	}

	mode := "keyword"
	topSim := 0.0
	anySem, anyKw := false, false
	for _, r := range resolved {
		if float64(r.sim) > topSim {
			topSim = float64(r.sim)
		}
		if r.sim >= simFloor {
			anySem = true
		}
		if r.kw > 0 {
			anyKw = true
		}
	}
	switch {
	case anySem && anyKw:
		mode = "hybrid"
	case anySem:
		mode = "semantic"
	case anyKw:
		mode = "keyword"
	}
	if dropped > 0 {
		mode = mode + "_conflict_resolved"
	}

	out := make([]models.Knowledge, 0, len(resolved))
	for _, r := range resolved {
		out = append(out, r.k)
	}
	return out, mode, topSim
}

func hybridKnowledgeScore(sim, kw, sourceB, freshB float64, hasSemantic bool) float64 {
	kwNorm := kw / 12.0
	if kwNorm > 1 {
		kwNorm = 1
	}
	if kwNorm < 0 {
		kwNorm = 0
	}
	simN := sim
	if simN < 0 {
		simN = 0
	}
	if simN > 1 {
		simN = 1
	}
	if hasSemantic {
		// Semantic lemah tapi keyword kuat tetap hidup.
		if simN < simFloor && kwNorm < 0.15 {
			return 0
		}
		return advWSemantic*simN + advWKeyword*kwNorm + advWSource*sourceB + advWFreshness*freshB
	}
	if kwNorm <= 0 {
		return 0
	}
	return advWKeywordOnly*kwNorm + advWSourceOnly*sourceB + advWFreshOnly*freshB
}

func keywordScoreRaw(qTokens []string, k models.Knowledge) float64 {
	if len(qTokens) == 0 {
		return 0
	}
	questionSet := map[string]bool{}
	answerSet := map[string]bool{}
	tagSet := map[string]bool{}
	for _, t := range tokenizeQuery(k.Question) {
		questionSet[t] = true
	}
	for _, t := range tokenizeQuery(k.Answer) {
		answerSet[t] = true
	}
	for _, t := range tokenizeQuery(k.Tags) {
		tagSet[t] = true
	}
	score := 0.0
	matched := 0
	for _, qt := range qTokens {
		switch {
		case tagSet[qt]:
			score += 4
			matched++
		case questionSet[qt]:
			score += 3
			matched++
		case answerSet[qt]:
			score++
			matched++
		}
	}
	if score <= 0 {
		return 0
	}
	score += (float64(matched) / float64(len(qTokens))) * 2
	return score
}

func sourcePriorityBoost(source string) float64 {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "manual", "product", "produk":
		return 1.0
	case "wizard":
		return 0.95
	case "dokumen", "document", "pdf":
		return 0.9
	case "web", "crawl", "website":
		return 0.82
	case "":
		return 0.88
	default:
		return 0.85
	}
}

func freshnessBoost(created time.Time) float64 {
	if created.IsZero() {
		return 0.55
	}
	days := time.Since(created).Hours() / 24
	switch {
	case days < 7:
		return 1.0
	case days < 30:
		return 0.9
	case days < 90:
		return 0.78
	case days < 365:
		return 0.62
	default:
		return 0.48
	}
}

func dedupeScoredKnowledge(in []scoredKnowledgeAdv) []scoredKnowledgeAdv {
	out := make([]scoredKnowledgeAdv, 0, len(in))
	seenID := map[uint]bool{}
	seenAns := map[string]bool{}
	for _, r := range in {
		if seenID[r.k.ID] {
			continue
		}
		key := strings.Join(tokenizeQuery(r.k.Answer), " ")
		if key != "" && seenAns[key] {
			continue
		}
		// Near-duplicate: overlap token jawaban sangat tinggi dengan yang sudah dipilih.
		if key != "" && nearDuplicateAnswer(key, out) {
			continue
		}
		seenID[r.k.ID] = true
		if key != "" {
			seenAns[key] = true
		}
		out = append(out, r)
	}
	return out
}

func nearDuplicateAnswer(ansKey string, existing []scoredKnowledgeAdv) bool {
	aToks := strings.Fields(ansKey)
	if len(aToks) < 4 {
		return false
	}
	aSet := map[string]bool{}
	for _, t := range aToks {
		aSet[t] = true
	}
	for _, e := range existing {
		eKey := strings.Join(tokenizeQuery(e.k.Answer), " ")
		eToks := strings.Fields(eKey)
		if len(eToks) == 0 {
			continue
		}
		hit := 0
		for _, t := range eToks {
			if aSet[t] {
				hit++
			}
		}
		// Jaccard kasar
		union := len(aSet)
		for _, t := range eToks {
			if !aSet[t] {
				union++
			}
		}
		if union > 0 && float64(hit)/float64(union) >= 0.72 {
			return true
		}
	}
	return false
}

// resolveKnowledgeConflicts membuang FAQ skor lebih rendah yang topiknya mirip
// tetapi angka di jawaban saling bertentangan (harga beda, jam beda, dll.).
func resolveKnowledgeConflicts(in []scoredKnowledgeAdv) ([]scoredKnowledgeAdv, int) {
	if len(in) <= 1 {
		return in, 0
	}
	keep := make([]bool, len(in))
	for i := range keep {
		keep[i] = true
	}
	dropped := 0
	for i := 0; i < len(in); i++ {
		if !keep[i] {
			continue
		}
		for j := i + 1; j < len(in); j++ {
			if !keep[j] {
				continue
			}
			if !knowledgePairConflicts(in[i].k, in[j].k) {
				continue
			}
			// i sudah diurut skor lebih tinggi → buang j
			keep[j] = false
			dropped++
			log.Printf("Knowledge conflict: keep id=%d drop id=%d (angka saling bertentangan, topik mirip)", in[i].k.ID, in[j].k.ID)
		}
	}
	out := make([]scoredKnowledgeAdv, 0, len(in))
	for i, r := range in {
		if keep[i] {
			out = append(out, r)
		}
	}
	return out, dropped
}

func knowledgePairConflicts(a, b models.Knowledge) bool {
	// Topik mirip?
	qa := append(tokenizeQuery(a.Question), tokenizeQuery(a.Tags)...)
	qb := append(tokenizeQuery(b.Question), tokenizeQuery(b.Tags)...)
	if tokenOverlapFraction(qa, qb) < 0.28 {
		// Coba overlap di jawaban pendek (FAQ sejenis tanpa Q mirip)
		aa := tokenizeQuery(a.Answer)
		ab := tokenizeQuery(b.Answer)
		if tokenOverlapFraction(aa, ab) < 0.22 {
			return false
		}
	}
	numsA := normalizedFactNumbers(a.Answer)
	numsB := normalizedFactNumbers(b.Answer)
	if len(numsA) == 0 || len(numsB) == 0 {
		return false
	}
	onlyA, onlyB, shared := 0, 0, 0
	for n := range numsA {
		if numsB[n] {
			shared++
		} else {
			onlyA++
		}
	}
	for n := range numsB {
		if !numsA[n] {
			onlyB++
		}
	}
	// Konflik: topik sama, masing-masing punya angka unik (klaim berbeda).
	// Shared tinggi + only kecil = pelengkap, bukan konflik.
	if onlyA == 0 || onlyB == 0 {
		return false
	}
	// Minimal satu angka "signifikan" (panjang ≥3 digit) yang bertentangan.
	sigConflict := false
	for n := range numsA {
		if len(n) >= 3 && !numsB[n] {
			sigConflict = true
			break
		}
	}
	for n := range numsB {
		if len(n) >= 3 && !numsA[n] {
			sigConflict = true
			break
		}
	}
	if !sigConflict && shared > 0 {
		return false
	}
	return onlyA > 0 && onlyB > 0
}

func tokenOverlapFraction(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	set := map[string]bool{}
	for _, t := range a {
		set[t] = true
	}
	hit := 0
	for _, t := range b {
		if set[t] {
			hit++
		}
	}
	// precision vs b
	return float64(hit) / float64(len(b))
}

// formatKnowledgeBlock menyusun blok prompt dengan prioritas & peringatan konflik sisa.
func formatKnowledgeBlock(relevant []models.Knowledge) string {
	if len(relevant) == 0 {
		return ""
	}
	var kb strings.Builder
	kb.WriteString("\n\nBASIS PENGETAHUAN TERPILIH (sudah di-rank multi-sinyal; urutan = prioritas):\n")
	kb.WriteString("Gunakan hanya fakta yang tertulis eksplisit di sumber berikut. Jangan melengkapi detail dengan asumsi. ")
	kb.WriteString("Jika sumber umum bertentangan dengan BASIS PENGETAHUAN PRODUK AKTIF, sumber produk yang sedang dibahas selalu menang. ")
	kb.WriteString("Jika masih ada ketidakpastian antar sumber, jelaskan singkat bahwa informasinya belum pasti; jangan menebak dan jangan otomatis eskalasi.\n\n")

	// Deteksi sisa konflik (seharusnya jarang setelah resolve) untuk instruksi eksplisit.
	if notes := remainingConflictNotes(relevant); len(notes) > 0 {
		kb.WriteString("PERINGATAN KONFLIK INTERNAL:\n")
		for _, n := range notes {
			kb.WriteString("- " + n + "\n")
		}
		kb.WriteString("Utamakan sumber nomor lebih kecil (prioritas lebih tinggi). Jangan campur angka dari dua sumber yang konflik.\n\n")
	}

	for i, k := range relevant {
		prio := "normal"
		switch {
		case i == 0:
			prio = "utama"
		case i == 1:
			prio = "tinggi"
		}
		kb.WriteString(fmt.Sprintf("[Sumber %d · prioritas %s", i+1, prio))
		if src := strings.TrimSpace(k.Source); src != "" {
			kb.WriteString(" · " + src)
		}
		if !k.CreatedAt.IsZero() {
			kb.WriteString(" · " + k.CreatedAt.Format("2006-01-02"))
		}
		kb.WriteString("]\n")
		kb.WriteString("Q: " + k.Question + "\n")
		kb.WriteString("A: " + k.Answer + "\n\n")
	}
	return kb.String()
}

func remainingConflictNotes(items []models.Knowledge) []string {
	var notes []string
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if knowledgePairConflicts(items[i], items[j]) {
				notes = append(notes, fmt.Sprintf(
					"Sumber %d dan %d punya angka berbeda untuk topik mirip — pakai Sumber %d",
					i+1, j+1, i+1,
				))
			}
		}
	}
	if len(notes) > 3 {
		notes = notes[:3]
	}
	return notes
}

// replyHasUngroundedNumbers: angka di jawaban yang tidak ada di knowledge/produk terpilih.
// Qty kecil dan total = harga_satuan × qty diizinkan (bukan halusinasi harga baru).
func replyHasUngroundedNumbers(reply string, relevant []models.Knowledge, productCtx string) bool {
	var src strings.Builder
	for _, k := range relevant {
		src.WriteString(k.Question)
		src.WriteByte('\n')
		src.WriteString(k.Answer)
		src.WriteByte('\n')
	}
	if productCtx != "" {
		src.WriteString(productCtx)
	}
	srcNums := normalizedFactNumbers(src.String())
	if len(srcNums) == 0 {
		// Knowledge tanpa angka: jawaban yang menyisipkan angka spesifik dicurigai.
		replyNums := normalizedFactNumbers(reply)
		for n := range replyNums {
			if len(n) >= 3 && !isBenignOrderQuantityDigits(n) { // 75_000, jam 08, dll.
				return true
			}
		}
		return false
	}
	for n := range normalizedFactNumbers(reply) {
		if srcNums[n] || isBenignOrderQuantityDigits(n) || isPlausiblePriceTimesQty(n, srcNums) {
			continue
		}
		return true
	}
	return false
}

// isBenignOrderQuantityDigits = qty/ukuran-numerik kecil (1–999), bukan harga.
func isBenignOrderQuantityDigits(digits string) bool {
	if len(digits) == 0 || len(digits) > 3 {
		return false
	}
	n, err := strconv.Atoi(digits)
	return err == nil && n >= 1 && n <= 999
}

// isPlausiblePriceTimesQty: total order = salah satu harga di sumber × qty 2..99.
// Mencegah grounding membunuh "2 × Rp68.250 = Rp136.500".
func isPlausiblePriceTimesQty(digits string, srcNums map[string]bool) bool {
	total, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || total < 1000 {
		return false
	}
	for src := range srcNums {
		if len(src) < 4 {
			continue // lewati % diskon / angka pendek
		}
		unit, err := strconv.ParseInt(src, 10, 64)
		if err != nil || unit < 1000 {
			continue
		}
		if total%unit != 0 {
			continue
		}
		qty := total / unit
		if qty >= 2 && qty <= 99 {
			return true
		}
	}
	return false
}

// answerGroundingOK menggabungkan overlap token + validasi angka.
// productCtx ikut jadi sumber angka (harga produk) agar tidak false-positive.
func answerGroundingOK(reply string, relevant []models.Knowledge, productCtx string) (overlap float64, ok bool, reason string) {
	overlap = answerKnowledgeOverlap(reply, relevant)
	if productCtx != "" {
		if po := tokenOverlapRatio(reply, contentTokenSet(productCtx)); po > overlap {
			overlap = po
		}
	}
	// Angka di jawaban harus ada di knowledge dan/atau blok produk.
	if replyHasUngroundedNumbers(reply, relevant, productCtx) {
		return overlap, false, "ungrounded_numbers"
	}
	// Tanpa knowledge terpilih: cukup lolos validasi angka (+ overlap produk opsional).
	if len(relevant) == 0 {
		return overlap, true, ""
	}
	if looksLikeInventedSpecifics(reply, relevant) && replyHasUngroundedNumbers(reply, relevant, productCtx) {
		return overlap, false, "invented_specifics"
	}
	if overlap < knowledgeOverlapMin {
		return overlap, false, "low_overlap"
	}
	return overlap, true, ""
}
