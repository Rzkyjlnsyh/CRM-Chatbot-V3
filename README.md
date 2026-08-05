# SlaluDiskon | SlaluDiskon

SlaluDiskon — platform WhatsApp untuk bisnis: auto-reply AI berbasis knowledge base,
inbox multi-agent, blast/broadcast dengan pengaman anti-blokir, CRM, jadwal, status, dan lainnya.

## Auto-reply AI
- Balas pesan WhatsApp otomatis dengan AI.
- **Satu pintu OpenRouter**: satu API key untuk chat, persona, ekstraksi closing, dan embedding.
- **Knowledge base**: isi tanya-jawab manual, generate otomatis dengan AI, atau impor.
- **Web crawler**: latih AI dari isi website (crawl → pilih halaman → train).
- **Semantic search** via embedding; model dipilih dari katalog OpenRouter di dashboard dan dapat diganti tanpa edit `.env`.
- Pilihan **tone**: ramah, formal, santai, persuasif.
- **Handoff**: alihkan percakapan ke manusia bila perlu.

## Blast / Broadcast
- Kirim pesan massal ke daftar nomor dengan **teks + lampiran** (gambar/video/dokumen).
- **Jadwal blast** ke tanggal & jam tertentu.
- **Blast ke grup**: post satu pesan ke banyak grup sekaligus (terjadwal).
- **Personalisasi** `{nama}` per penerima.
- Ambil penerima dari: pernah chat, kontak WA, anggota grup, atau label.
- **Cek nomor terdaftar WhatsApp** sebelum kirim, untuk membuang nomor tidak aktif.

### Pengaman anti-blokir
- **Jeda acak** antar pesan + **istirahat berkala** agar pola kirim tidak seperti bot.
- **Humanized typing** (indikator "mengetik…").
- **Opt-out otomatis**: kontak yang balas STOP/BERHENTI dilewati.
- **Consent tracking** per kategori pesan + **risk level** sebelum blast.
- Lanjut otomatis bila server restart; jeda otomatis bila WhatsApp membatasi.

## Manajemen grup (Anti-Spam)
- Moderasi grup: deteksi link/nomor/kata terlarang & flood.
- Aksi: hapus pesan, tandai untuk dikeluarkan, atau auto-kick (butuh bot admin).
- Log audit tiap tindakan moderasi.

## CRM & kontak
- Simpan kontak, sinkronkan label WhatsApp, beri tag, dan impor massal.
- Pipeline CRM: Baru, Cold, Warm, Hot, Pelanggan, dan Tidak potensial.
- Riwayat chat & analitik percakapan.
- Follow-up otomatis bertahap (multi-step).
- Formulir closing & pencatatan order; cek ongkir (opsional).

## Integrasi & tracking
- **Meta Conversions API (CAPI)**: kirim event konversi ke Meta (rahasia dienkripsi at-rest).
- Google Sheets (opsional).

## Multi-agent
- Kelola beberapa nomor/agent WhatsApp dalam satu dashboard.

## Development lokal

Panduan instalasi untuk pembeli:
- PDF singkat: [`docs/PANDUAN-INSTALASI.pdf`](docs/PANDUAN-INSTALASI.pdf)
- Detail teks: [`docs/INSTALL-LOCAL.md`](docs/INSTALL-LOCAL.md)

### Satu perintah (macOS, Windows, Linux)

Setelah dependency terpasang (`npm run setup`) dan file `.env` siap:

```bash
npm run dev
```

Perintah yang sama di semua OS. Alternatif:

| OS | Perintah |
|----|----------|
| Semua | `npm run dev` atau `node scripts/dev.mjs` |
| macOS / Linux | `./scripts/dev.sh` |
| Windows (CMD) | `scripts\dev.bat` |
| Windows (PowerShell) | `powershell -File scripts\dev.ps1` |

Launcher akan:

- Menjalankan **frontend** Vite di `http://127.0.0.1:5173` (hot reload)
- Menjalankan **backend** dengan Air di `http://127.0.0.1:3030` (auto rebuild Go)
- Menginstal Air otomatis bila belum ada
- Menolak start bila port `3030` / `5173` masih dipakai

Tekan `Ctrl+C` untuk menghentikan keduanya.

## Keamanan & operasional
- Login admin dengan password ter-hash (bcrypt) + throttle/lockout.
- JWT untuk sesi; rahasia sensitif dienkripsi di database.
- Sistem lisensi (aktivasi + heartbeat + grace offline).

---
Produk **SlaluDiskon | SlaluDiskon**. Penggunaan tunduk pada [EULA](docs/EULA.md) & [Disclaimer](docs/DISCLAIMER.md). **Wajib dibaca.**
