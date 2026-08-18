# Panduan Integrasi Branch Fitur

Tiga branch fitur dibuat di atas commit `e31d073` (base yang sama dengan
kode yang sedang dikembangkan). Branch memakai file baru sebanyak mungkin
dan perubahan minimal pada file bersama, sehingga integrasi bersifat
terarah dan konflik yang mungkin muncul kecil.

## Ringkasan branch

| Branch | Isi | File baru | File diubah |
|---|---|---|---|
| `feature/learning-engine` | AI Learning Engine | 5 | 5 |
| `feature/meta-capi` | Meta Conversions API | 4 | 7 |
| `feature/cs-control` | Kontrol AI per kontak + perbaikan gate handoff | 2 | 4 |

## Urutan integrasi

Merge dari cabang utama. Urutan tidak mengikat (setiap branch menyentuh
file yang berbeda), tetapi disarankan:

```
1. feature/learning-engine
2. feature/cs-control
3. feature/meta-capi
```

## Perintah

```bash
git fetch origin
git merge origin/feature/learning-engine
git merge origin/feature/cs-control
git merge origin/feature/meta-capi
```

## Potensi konflik dan penyelesaiannya

| File | Kemungkinan konflik | Penyelesaian |
|---|---|---|
| `backend/main.go` | Baris route baru (blok `learning/*`, `meta/*`, `contacts/:sender/*`) | Pertahankan kedua sisi; urutan route tidak berpengaruh |
| `backend/models/models.go` | Enam field `Meta*` di struct `Agent` (ditempatkan di akhir struct, setelah `WebhookSecret`) | Pertahankan kedua sisi |
| `backend/database/database.go` | Baris `AutoMigrate` (4 model learning + `MetaConversion`) | Gabungkan daftar model |
| `backend/handlers/agents.go` | `shouldAllowHumanHandoff` dan `pauseAIForManualReply` | Ambil versi branch (berisi perbaikan) |
| `frontend/src/pages/Dashboard.tsx` | Tab baru (`learning`, `meta`) + import komponen | Pertahankan kedua sisi |
| `frontend/src/hooks.ts` | Blok hooks baru di akhir file + import type | Pertahankan kedua sisi |
| `frontend/src/components/InboxPanel.tsx` | Tombol Jeda AI / Lanjutkan AI / Ke CS; penghapusan variabel `oldestId` yang tidak terpakai | Ambil versi branch |

Catatan: ketiga branch menyertakan perbaikan yang sama pada
`InboxPanel.tsx` (menghapus variabel `oldestId` yang tidak terpakai —
penyebab gagalnya build TypeScript). Setelah merge pertama, dua merge
berikutnya otomatis mengabaikan perubahan yang sudah masuk.

## Setelah integrasi

1. Backend: `go build ./backend`, lalu jalankan. `AutoMigrate` membuat
   tabel baru (`learning_runs`, `learning_patterns`, `learning_snapshots`,
   `learning_configs`, `meta_conversions`) dan kolom `meta_*` pada tabel
   `agents` secara otomatis — tidak diperlukan migrasi manual.
2. Frontend: `npm install && npm run build`. Tidak ada dependency baru.
3. Jalankan dan login.

## Cakupan

- Tidak ada kode multi-tenant, subscription, landing page, atau pembayaran.
- Tidak ada perubahan pada `go.mod` (tidak ada dependency baru).
- Tidak ada perubahan skema yang memerlukan migrasi manual.
