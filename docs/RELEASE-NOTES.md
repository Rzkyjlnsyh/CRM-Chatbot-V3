# Release Notes — Learning Engine, Kontrol Kontak, Meta CAPI, Media, Stabilitas Build

Ringkasan teknis lima branch fitur di atas `e31d073`, ditulis berdasarkan
implementasi aktual di kode.

---

## 1. AI Learning Engine — `feature/learning-engine`

### Deskripsi

Engine yang mengekstrak pola komunikasi dari percakapan CS manusia
(`chat_history.from_human = true`) lalu menyimpannya sebagai knowledge
yang bisa diterapkan ke agent. Jawaban AI sendiri tidak pernah menjadi
bahan belajar.

### Alur satu run

1. `POST /agents/:id/learning/run` membuat baris `learning_runs` (status
   `pending`) dan diproses di goroutine background.
2. `loadHumanCSChats`: mengambil chat dengan `from_human=true` dan
   `reply != ''` dalam rentang tanggal (kosong = `lookback_days` config,
   default 30 hari). Validasi: minimal 3 chat, jika tidak → run `failed`
   dengan pesan yang menjelaskan.
3. `extractStyleProfile` (1 panggilan LLM): profil gaya bahasa — variasi
   sapaan, variasi closing, frasa khas, emoji beserta konteks, tone,
   tempo, cara mengatasi keberatan, teknik upsell, gaya follow-up.
4. `extractPatterns` (1 panggilan LLM): pola global (`greeting`,
   `closing`, `objection_handling`, `upsell`, `follow_up`, `phrase`,
   `emoji_style`) — masing-masing berisi `trigger_context`,
   `response_template`, `emoji_signature`, `confidence`, `closing_impact`.
   Filter awal confidence >= 0.4, maks 10 pola.
5. Per label WhatsApp dengan >= 3 chat: `extractLabelPatterns` — pola
   `label_handling` dan `closing_path` (bagaimana kontak berlabel
   dipindahkan menuju closing). Maks 8 pola per label.
6. Semua pola disimpan di `learning_patterns` dengan status `suggested`.
7. Jika `auto_apply` aktif: snapshot dibuat terlebih dahulu, lalu pola
   dengan `confidence >= min_confidence` diterapkan (maks 10 per run).
8. Run ditutup dengan status `completed` + summary.

### Penerapan pola

`ApplyPattern` menulis baris baru ke `knowledge` (question =
`trigger_context`, answer = `response_template`, tags `learned,<type>`,
source `learning`), lalu memanggil `IndexKnowledge` (embedding langsung
di-generate sehingga pola aktif tanpa restart). Status pola menjadi
`applied`; `reject` mengubah status menjadi `rejected` tanpa efek lain.

### Snapshot dan rollback

`learning_snapshots` menyimpan serialisasi penuh persona (SystemPrompt)
dan seluruh knowledge dalam `data_json`. Dibuat manual atau otomatis
sebelum auto-apply / apply-all. `rollback` mengembalikan persona dan
knowledge ke kondisi snapshot dan menghapus knowledge ber-source
`learning` yang dibuat setelahnya.

### Scheduler

`StartLearningScheduler` berjalan di goroutine saat server start, cek
setiap jam. Kondisi: `config.enabled && config.schedule_enabled`, tidak
ada run pending/running, dan jarak dari run terakhir >= 23 jam. Rentang
data memakai `lookback_days`.

### API (13 endpoint)

```
POST   /agents/:id/learning/run
GET    /agents/:id/learning/status
GET    /agents/:id/learning/runs
GET    /agents/:id/learning/runs/:rid
GET    /agents/:id/learning/patterns
POST   /agents/:id/learning/patterns/:pid/apply
POST   /agents/:id/learning/patterns/:pid/reject
POST   /agents/:id/learning/patterns/apply-all
GET    /agents/:id/learning/snapshots
POST   /agents/:id/learning/snapshots
POST   /agents/:id/learning/snapshots/:sid/rollback
GET    /agents/:id/learning/config
PUT    /agents/:id/learning/config
```

### UI

Tab "AI Learning" di dashboard: status bar (run terakhir, jumlah pola
suggested/applied, jumlah snapshot, saklar on/off), tab Jalankan (rentang
tanggal + tombol mulai + rekap run terakhir), tab Pola (kartu pola:
tipe, label, confidence, closing impact, pemicu, template, emoji —
dengan tombol Terapkan/Tolak dan Terapkan Semua + ambang confidence),
tab Versi (daftar snapshot + rollback), tab Konfigurasi (enabled,
auto_apply, min_confidence, min_usage_count, max_patterns_per_run,
preserve_manual_knowledge, schedule_enabled, schedule_cron,
lookback_days).

### Catatan

- Biaya per run: 4–6 panggilan LLM (StyleProfile, pola global, satu
  panggilan per label dengan >= 3 chat).
- `StyleProfile.UnmarshalJSON` menerima field array yang dikembalikan
  sebagai string tunggal oleh sebagian model (dinormalisasi menjadi
  slice satu elemen) — mencegah run gagal karena bentuk JSON berbeda.
- `usage_count` saat ini diisi konstan 1 dan `min_usage_count` belum
  menjadi filter di auto-apply (auto-apply hanya memakai confidence).

---

## 2. Kontrol AI per Kontak — `feature/cs-control`

### Endpoint baru

```
POST /agents/:id/contacts/:sender/ai-off      # jeda semua balasan otomatis
POST /agents/:id/contacts/:sender/ai-on       # lanjutkan balasan otomatis
GET  /agents/:id/contacts/:sender/ai-status   # status jeda
POST /agents/:id/contacts/:sender/handoff     # pindah ke antrean Butuh CS
```

- `ai-off` menulis `manual_pause_until = 2099-12-31 23:59:59` pada tabel
  `contacts` (memakai kolom yang sudah ada, tanpa perubahan skema).
  Kontak dibuat dengan `FirstOrCreate` bila belum ada.
- `ai-on` mengosongkan `manual_pause_until`.
- `ai-status` membaca `manual_pause_until` dan mengembalikan `paused`.
- `handoff` memanggil `ensureHandoff` dengan pesan terakhir kontak —
  jalur yang sama dengan handoff otomatis.

### Perilaku

- Jeda berlaku untuk seluruh lapisan balasan otomatis: AI, auto-reply,
  alur otomatis, tombol produk, dan form AI (cek `manualPaused` di
  `processMessageLocked`, sebelum `handleProductInteraction`).
- `pauseAIForManualReply` (jeda otomatis 10 menit saat CS membalas dari
  inbox) tidak lagi menimpa jeda yang masih aktif.
- Saat AI dilanjutkan, riwayat percakapan (termasuk balasan CS manusia)
  tetap masuk konteks prompt — pertanyaan yang sudah dibahas tidak
  diulang.

### Perbaikan gate handoff (`shouldAllowHumanHandoff`)

- Kata kerja permintaan ditambah: `butuh`, `mau`, `ingin`, `perlu`.
- Term manusia ditambah: `bantuan`.
- Kata berisiko ditambah: `diblokir`, `di blokir`, `ke blokir`, `banned`,
  `akun dibekukan`.
- Test unit `backend/handlers/handoff_gate_test.go` (19 kasus): frasa
  permintaan eksplisit dan kata berisiko lolos; pesan biasa (tanya harga,
  ongkir, status pesanan) tidak mengeskalasi.

---

## 3. Meta Conversions API — `feature/meta-capi`

### Deskripsi

Mengirim event konversi ke Facebook Ads (server-side) saat label konversi
menempel ke kontak. Tidak memerlukan JavaScript pixel di sisi browser.

### Alur

1. Konfigurasi disimpan pada field `meta_*` di tabel `agents`
   (`MetaPixelID`, `MetaAccessToken`, `MetaTestEventCode`,
   `MetaConvLabels`, `MetaEventName`, `MetaLabelEvents`). Access token
   tidak pernah dikembalikan oleh API (`json:"-"`).
2. `OnLabelAssoc` dan `SyncLabels` memanggil `FireMetaConversion` ketika
   label yang masuk daftar konversi menempel ke kontak.
3. `FireMetaConversion` berjalan async (`Go` + `RecoverGo`):
   - Dedup via `meta_conversions` (unique: agent_id, sender, label_id) —
   satu kontak + satu label hanya dikirim sekali, aman terhadap restart.
   - Payload: `event_name` (mapping label → event, fallback
     `meta_event_name`), `event_time`, `user_data.ph` = SHA-256 dari
     nomor WA (lowercase, sesuai aturan Meta), `custom_data.currency =
     IDR`, `test_event_code` bila diisi.
   - Dikirim ke `https://graph.facebook.com/v19.0/<PIXEL>/events`.

### API

```
GET  /agents/:id/meta         # konfigurasi + 20 event terakhir + daftar label
PUT  /agents/:id/meta         # simpan konfigurasi (token kosong = pertahankan lama)
POST /agents/:id/meta/test    # kirim event uji
GET  /agents/:id/meta/logs    # log pengiriman
```

### UI

Tab "Meta CAPI": form Pixel ID, Access Token, Test Event Code, daftar
label konversi, event default, mapping label → event (Autocomplete),
tabel 20 event terakhir, tombol test event.

### Persiapan di Meta

- Pixel ID dan Access Token dari Events Manager.
- Untuk uji: isi Test Event Code, aktifkan "Test Events" di Events
  Manager, beri label konversi ke satu kontak, event muncul dalam
  beberapa detik, lalu hapus Test Event Code.

---

## 4. Stabilitas Build — `feature/stability-fixes`

### Deskripsi

Perbaikan agar aplikasi dapat di-build dan dijalankan tanpa toolchain C
(GCC). Build `CGO_ENABLED=0` di mesin Windows tanpa gcc sebelumnya gagal
pada koneksi WhatsApp karena stub `mattn/go-sqlite3` merebut nama driver
`sqlite3` di store whatsmeow.

### Perubahan

- `backend/services/wa.go`: driver SQLite store diganti dari
  `modernc.org/sqlite` ke `github.com/glebarez/go-sqlite` (fork pure-Go
  yang sama dengan driver GORM) dan didaftarkan sebagai driver `sqlite3`.
  `sessionDSN` memakai format `file:<path>?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)`.
- `backend/database/database.go`: driver GORM `gorm.io/driver/sqlite`
  (mattn) diganti `github.com/glebarez/sqlite`.
- `go.mod`: menambah `github.com/glebarez/sqlite` dan
  `github.com/glebarez/go-sqlite`. `mattn/go-sqlite3` tersisa hanya
  sebagai dependency indirect dari `go.mau.fi/util/dbutil/litestream`
  (subfolder dengan build tag — tidak ikut ter-compile pada build normal).
- `frontend/src/pages/Dashboard.tsx`: tombol simpan konfigurasi AI tidak
  lagi terkunci saat hanya key DeepSeek yang terisi
  (`disabled={!apiKey && !deepseekKey}`).

### Verifikasi

- `CGO_ENABLED=0 go build ./backend` berhasil.
- Inisialisasi store whatsmeow dengan driver `sqlite3` berhasil tanpa
  error stub: `STORE_OK: devices=0` (jalur QR siap).
- Sesi lama tetap terbaca: `sessionDSN` agent 1 memakai file sesi lama.

---


## 5. Panel Media — `feature/media-assets`

### Deskripsi

UI untuk mengelola media asset (katalog, foto produk, video) yang dikirim
AI lewat directive `[[SEND_MEDIA:label]]`. Backend-nya sudah ada sejak
versi dasar; branch ini menambahkan panel dashboard + endpoint serve file
untuk preview.

### Perubahan

- `frontend/src/components/MediaAssetsPanel.tsx` (baru): daftar asset
  (thumbnail, label, trigger keys, caption, ukuran, tipe), form unggah
  (file + preview, label wajib, trigger keys, caption default, urutan),
  tombol hapus. Dipasang di tab "Media" dashboard.
- `frontend/src/hooks.ts`: `useMediaAssets`, `useUploadMediaAsset`,
  `useDeleteMediaAsset` (FormData multipart).
- `frontend/src/types.ts`: interface `MediaAsset`.
- `backend/handlers/media_assets.go`: handler `ServeMediaAssetFile`
  (publik + token query, pola sama dengan gambar produk); upload membaca
  field `sort_order` dari form.
- `backend/main.go`: route `GET /agents/:id/media-assets/:assetId/file`.
- `backend/handlers/media_directive_test.go` (baru): test unit
  `parseMediaLabels` (6 kasus) dan `buildFinalText` (6 kasus).

### Verifikasi

- Upload via multipart → id, label, tipe, ukuran, urutan tersimpan.
- List asset → 1 baris; serve file → 200 dengan byte PNG valid; delete →
  list kosong.
- `CGO_ENABLED=0` build + `go vet` + test unit + frontend build bersih.

### Catatan

- Label adalah kunci bagi AI: arahkan persona untuk memakai
  `[[SEND_MEDIA:<label>]]` (mis. `katalog dtf`). Trigger keys membantu AI
  memilih media dari konteks percakapan.
- AI sudah diinstruksikan soal directive ini sejak versi dasar (Layer 6
  Media Directive di `buildSystemPrompt`): menulis teks dulu, directive di
  akhir, satu directive per balasan.

---

## Catatan implementasi

- Tabel baru: `learning_runs`, `learning_patterns`, `learning_snapshots`,
  `learning_configs`, `meta_conversions` (+ 6 kolom `meta_*` di `agents`).
- File utama: `backend/services/learning.go`, `backend/handlers/learning.go`,
  `backend/handlers/contact_control.go`, `backend/services/meta.go`,
  `backend/handlers/meta.go`, `backend/services/wa.go`,
  `frontend/src/components/MediaAssetsPanel.tsx`.
- Dependency Go baru: `github.com/glebarez/sqlite`,
  `github.com/glebarez/go-sqlite` (SQLite pure-Go, tanpa CGO). Frontend
  tidak berubah dependency-nya.
- Test unit: `go test ./backend/handlers/ -run TestShouldAllowHumanHandoff`
  (butuh `JWT_SECRET` minimal 32 karakter di environment).
