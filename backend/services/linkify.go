package services

import (
	"regexp"
	"strings"
)

// waNumberRe menangkap kandidat nomor HP Indonesia (mobile: diawali 08 / 62 / +62 lalu 8),
// memperbolehkan pemisah spasi/titik/strip antar-grup.
var waNumberRe = regexp.MustCompile(`(?:\+?62[ .\-]?|0)8[0-9 .\-]{6,13}[0-9]`)

// LinkifyWhatsApp mengubah nomor WhatsApp pada teks jadi tautan https://wa.me/<intl> yang bisa diklik.
// Nomor milik bot sendiri (ownNumber) dilewati—pelanggan toh sudah terhubung di chat ini.
func LinkifyWhatsApp(text, ownNumber string) string {
	own := normalizeWANumber(ownNumber)
	locs := waNumberRe.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return text
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		// Lewati bila tepat setelah "/" (kemungkinan sudah bagian URL, mis. wa.me/62...).
		if start > 0 && text[start-1] == '/' {
			continue
		}
		intl := normalizeWANumber(text[start:end])
		if intl == "" || (own != "" && intl == own) {
			continue // bukan nomor valid atau nomor sendiri -> biarkan apa adanya
		}
		b.WriteString(text[last:start])
		b.WriteString("https://wa.me/" + intl)
		last = end
	}
	b.WriteString(text[last:])
	return b.String()
}

// normalizeWANumber: ambil digit saja, ubah 0xxxx -> 62xxxx, validasi panjang wajar (10-15 digit).
// Mengembalikan "" bila bukan nomor HP yang masuk akal.
func normalizeWANumber(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	d := b.String()
	if strings.HasPrefix(d, "0") {
		d = "62" + d[1:]
	}
	if !strings.HasPrefix(d, "62") || len(d) < 10 || len(d) > 15 {
		return ""
	}
	return d
}
