# Menjalankan SlaluDiskon | SlaluDiskon di Komputer Lokal

Panduan ini berlaku untuk **macOS**, **Windows**, dan **Linux**.

Panduan ini adalah referensi utama untuk setup lokal dan deployment internal.

**Cara yang disarankan:** buka project di **Visual Studio Code**, lalu jalankan semua perintah lewat **Terminal di dalam VS Code**.

## Kebutuhan

| Software | Keterangan |
|----------|------------|
| **Visual Studio Code** | https://code.visualstudio.com/ |
| **Go** | Versi sesuai `go.mod` (lihat file tersebut) |
| **Node.js** | 22+ disarankan (18+ biasanya cukup) + npm |
| **MySQL** | 8.x, service harus berjalan |

### Instal cepat

- **Go:** https://go.dev/dl/
- **Node.js:** https://nodejs.org/ (pilih LTS)
- **MySQL:**
  - macOS: `brew install mysql` lalu `brew services start mysql`
  - Windows: [MySQL Installer](https://dev.mysql.com/downloads/installer/) (pastikan “MySQL Server” terpasang & service Running)
  - Linux: paket distro (`mysql-server` / `mariadb-server`)

Setelah install, **buka ulang terminal** agar `go`, `node`, dan `npm` dikenali.

Cek:

```bash
go version
node -v
npm -v
```

## 1. Ekstrak source & buka di VS Code

1. Ekstrak ZIP ke folder lokal (hindari folder yang di-sync publik).
2. Buka **Visual Studio Code**.
3. **File → Open Folder…** (Windows) / **File → Open…** (macOS).
4. Pilih folder **root** project (ada `package.json`, `go.mod`, folder `frontend` & `backend`).
5. Buka terminal VS Code: **Terminal → New Terminal**  
   (pintasan: `Ctrl+`` di Windows, `Control+`` di macOS).

Pastikan terminal berada di root project. Jangan unggah source atau file `.env` ke repository publik.

## 2. Environment (`.env`)

Dari **Terminal VS Code** di root project:

```bash
# macOS / Linux
cp .env.example .env

# Windows CMD
copy .env.example .env

# atau lintas OS (butuh Node)
npm run setup:env
```

### Edit file `.env` di VS Code

1. Di **sidebar kiri**, klik file `.env`.
2. Jangan hapus nama variabel di kiri tanda `=`. Hanya ganti nilai di **kanan**.
3. Simpan file: `Ctrl+S` (Windows) / `Cmd+S` (macOS).

| Nama di `.env` | Diisi dengan |
|----------------|--------------|
| `DB_HOST` | Alamat MySQL, biasanya `127.0.0.1` |
| `DB_PORT` | Port MySQL, biasanya `3306` |
| `DB_USER` | Username MySQL (contoh `root`) |
| `DB_PASS` | Password MySQL Anda |
| `DB_NAME` | Nama database (contoh `db_wa_blast`) |
| `JWT_SECRET` | Kunci acak **minimal 32 karakter** — lihat cara buat di bawah |
| `SUPERADMIN_USERNAME` | Username login dashboard |
| `SUPERADMIN_PASSWORD` | Password admin (kuat, min. 12 karakter) |
| `LICENSE_KEY` | Isi dari manifest handover jika build yang dipakai mengaktifkan runtime license |
| `LICENSE_API_SECRET` | Secret API lisensi dari manifest handover, jika runtime license dipakai |
| `LICENSE_OWNER` | Nama pemilik lisensi |
| `LICENSE_EMAIL` | Email pemilik lisensi |
| `LICENSE_ORDER_ID` | Nomor invoice/order (jika ada) |
| `LICENSE_API_URL` | URL API lisensi sesuai deployment, jika runtime license dipakai |
| `LICENSE_RESPONSE_SIGNING_PUBLIC_KEY` | Public key resmi dari manifest handover, jika runtime license dipakai |

Backend dapat membuat database/tabel otomatis jika user MySQL punya izin `CREATE`.

### Cara membuat `JWT_SECRET` (wajib acak)

Jalankan **salah satu** perintah di Terminal VS Code, lalu **salin seluruh hasilnya** ke baris `JWT_SECRET=...` di `.env`.

**macOS / Linux:**

```bash
openssl rand -hex 32
```

**Windows PowerShell (Terminal VS Code):**

```powershell
[Convert]::ToHexString((1..32 | ForEach-Object { Get-Random -Maximum 256 }) -as [byte[]]).ToLower()
```

**Semua OS (Node sudah terpasang):**

```bash
node -e "console.log(require('crypto').randomBytes(32).toString('hex'))"
```

Contoh hasil (panjang ~64 karakter hex):

```env
JWT_SECRET=a3f91c8e2b7d0e4f6a1b2c3d4e5f60718293a4b5c6d7e8f90123456789abcdef
```

Jangan pakai kata sederhana seperti `secret123` atau `password`.

## 3. Install dependency (sekali saja)

Masih di **Terminal VS Code**, root project:

```bash
npm run setup
```

Setara dengan:

```bash
go mod download
npm --prefix frontend install
```

## 4. Jalankan (satu perintah — semua OS)

Di **Terminal VS Code**:

```bash
npm run dev
```

Itu saja. Perintah yang sama di Mac, Windows, dan Linux. Biarkan terminal VS Code tetap terbuka.

| Cara lain | Perintah |
|-----------|----------|
| Node langsung | `node scripts/dev.mjs` |
| macOS / Linux | `./scripts/dev.sh` |
| Windows CMD | `scripts\dev.bat` |
| Windows PowerShell | `powershell -ExecutionPolicy Bypass -File scripts\dev.ps1` |

### Yang terjadi

1. Frontend: http://127.0.0.1:5173  
2. Backend API: http://127.0.0.1:3030  
3. Air (hot-reload Go) diinstal otomatis bila belum ada  
4. Port bentrok → script berhenti dengan pesan jelas  

Login: `SUPERADMIN_USERNAME` / `SUPERADMIN_PASSWORD` dari `.env`.

Hentikan: klik Terminal VS Code, lalu **Ctrl+C**.

### Mode manual (dua terminal) — opsional

Jika ingin memisahkan proses:

```bash
# Terminal 1
go run ./backend

# Terminal 2
npm --prefix frontend run dev
```

## 5. Hubungkan WhatsApp

1. Login dashboard  
2. Tambah / pilih agent (nomor)  
3. Hubungkan WhatsApp (QR atau pairing)  
4. Tunggu status **Online / terhubung** sebelum memakai fitur di dashboard

## Windows — tips anti-ribet

1. Install Go + Node + MySQL dengan opsi **Add to PATH**.  
2. Pakai **PowerShell** atau **Windows Terminal**, bukan hanya double-click sembarangan tanpa PATH.  
3. Boleh double-click `scripts\dev.bat` **setelah** `npm run setup` dan `.env` siap (jalankan dari folder project).  
4. Jika PowerShell menolak script:

   ```powershell
   powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1
   ```

5. Firewall Windows: izinkan Node/Go saat diminta (localhost).  
6. Pastikan service **MySQL** status *Running* di Services (`services.msc`).

## macOS — tips

```bash
chmod +x scripts/dev.sh   # sekali saja jika perlu
npm run setup
npm run setup:env         # jika belum ada .env
# edit .env
npm run dev
```

## Masalah umum

| Gejala | Solusi |
|--------|--------|
| `LICENSE_KEY kosong` / banner **LISENSI BELUM AKTIF** | Build yang dipakai mengaktifkan runtime license. Isi `LICENSE_KEY` dan `LICENSE_API_SECRET` dari manifest handover, lalu restart `npm run dev` |
| `Invalid signature` | Public key lisensi harus yang resmi dari rilis |
| Banner **SERVER SIAP** | Lisensi OK — buka dashboard `http://127.0.0.1:5173` |
| MySQL gagal connect | Cek service MySQL + `DB_*` di `.env` |
| Port 3030 / 5173 dipakai | Hentikan dev lama (Ctrl+C) atau tutup proses di port itu |
| `go: command not found` | Install Go, restart terminal |
| `npm: command not found` | Install Node.js LTS, restart terminal |
| Frontend tidak nyambung API | Backend harus jalan di port 3030 |
| Windows: Air tidak ketemu | Script fallback ke `go run`; atau tambahkan `%USERPROFILE%\go\bin` ke PATH |

## Keamanan

Jangan membagikan:

- `.env`
- Database / dump
- Sesi WhatsApp (`data/`, file session)
- Manifest / key lisensi deployment

---

Penggunaan tunduk pada [EULA](EULA.md) & [Disclaimer](DISCLAIMER.md).
