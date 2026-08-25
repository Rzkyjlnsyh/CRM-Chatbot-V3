// Package ui menyediakan tampilan terminal yang rapi (banner/kotak) untuk
// pesan penting ke pengguna, terpisah dari log teknis ber-timestamp.
//
// Warna otomatis dinonaktifkan bila keluaran bukan terminal (mis. dijalankan
// sebagai service/systemd atau di-redirect ke file), atau bila env NO_COLOR di-set,
// sehingga log server tidak dikotori kode escape ANSI.
package ui

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	colorReset = "\x1b[0m"
	colorRed   = "\x1b[1;31m"
	colorGreen = "\x1b[1;32m"
)

// boxMaxWidth membatasi lebar pembungkusan teks isi kotak.
const boxMaxWidth = 56

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// useColor true hanya bila f adalah terminal interaktif dan NO_COLOR tidak di-set.
func useColor(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// wrapWords memecah teks panjang menjadi beberapa baris <= max karakter.
func wrapWords(s string, max int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	for _, w := range words {
		switch {
		case cur == "":
			cur = w
		case runeLen(cur)+1+runeLen(w) > max:
			lines = append(lines, cur)
			cur = w
		default:
			cur += " " + w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// renderBox membangun kotak Unicode berisi judul + baris isi (tanpa warna).
func renderBox(title string, lines []string) string {
	inner := runeLen(title)
	for _, l := range lines {
		if n := runeLen(l); n > inner {
			inner = n
		}
	}
	bar := strings.Repeat("─", inner+4)
	var b strings.Builder
	writeLine := func(s string) {
		b.WriteString("│  " + s + strings.Repeat(" ", inner-runeLen(s)) + "  │\n")
	}
	b.WriteString("╭" + bar + "╮\n")
	writeLine(title)
	b.WriteString("├" + bar + "┤\n")
	for _, l := range lines {
		writeLine(l)
	}
	b.WriteString("╰" + bar + "╯\n")
	return b.String()
}

// colorize membungkus tiap baris dengan kode warna (agar warna konsisten per baris).
func colorize(s, color string) string {
	rows := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, r := range rows {
		rows[i] = color + r + colorReset
	}
	return strings.Join(rows, "\n") + "\n"
}

func printBox(f *os.File, color, title string, lines []string) {
	out := renderBox(title, lines)
	if useColor(f) {
		out = colorize(out, color)
	}
	fmt.Fprint(f, "\n"+out+"\n")
}

// LicenseError menampilkan kotak error lisensi (merah) lalu keluar dengan status 1.
func LicenseError(reason string) {
	if strings.TrimSpace(reason) == "" {
		reason = "Lisensi tidak valid."
	}
	body := []string{"LISENSI BELUM AKTIF", ""}
	body = append(body, wrapWords(reason, boxMaxWidth)...)
	body = append(body,
		"",
		"Cara mengaktifkan",
		" 1) Hubungi admin untuk lisensi",
		" 2) Isi di file .env:",
		"      LICENSE_KEY=xxxx",
		"      LICENSE_API_SECRET=...",
		"      LICENSE_API_URL=...",
		" 3) Jalankan lagi  ->  npm run dev",
		"",
		"Pastikan .env di root project, lalu restart",
		"terminal dev setelah menyimpan.",
		"",
		"Docs  docs/INSTALL-LOCAL.md",
	)
	printBox(os.Stderr, colorRed, "SlaluDiskon", body)
	os.Exit(1)
}

// StartupOK menampilkan banner sukses (hijau) saat server siap menerima koneksi.
func StartupOK(port string) {
	body := []string{
		"SERVER SIAP",
		"",
		"Lisensi aktif. Backend berjalan dan siap",
		"menerima koneksi dari dashboard.",
		"",
		"Endpoint",
		"  •  API        http://127.0.0.1:" + port,
		"  •  Dashboard  http://127.0.0.1:5173",
		"",
		"Hentikan server dengan Ctrl+C di terminal",
		"npm run dev.",
	}
	printBox(os.Stdout, colorGreen, "SlaluDiskon", body)
}
