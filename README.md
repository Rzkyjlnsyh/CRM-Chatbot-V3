# CRM Chatbot — WhatsApp AI Assistant

Platform WhatsApp untuk bisnis: auto-reply AI, inbox multi-agent, broadcast, CRM, ongkir realtime, dan integrasi REST API.

---

## Tech Stack

| Layer | Teknologi |
|-------|-----------|
| Backend | Go 1.25 (Gin, GORM) |
| Frontend | React 18 + TypeScript (Vite) |
| Database | MySQL 8 (produksi) / SQLite (development) |
| WhatsApp | whatsmeow (Multi-Device, Go-native) |
| AI Chat | DeepSeek / OpenRouter |
| AI Vision | OpenRouter |
| Shipping | Mengantar API (JNE, JT) |

---

## Quick Start

### Prerequisites

- Go 1.25+
- Node.js 20+
- MySQL 8 (opsional; SQLite otomatis dipakai jika MySQL tidak tersedia)

### Setup

```bash
# 1. Clone
git clone https://github.com/Rzkyjlnsyh/CRM-Chatbot-V3.git
cd CRM-Chatbot-V3

# 2. Install dependencies
npm run setup

# 3. Konfigurasi environment
cp .env.example .env
# Edit .env — minimal isi:
#   DEEPSEEK_API_KEY=sk-...          (untuk AI chat)
#   MENGANTAR_API_KEY=API-...        (untuk cek ongkir)
#   JWT_SECRET=string_random_32_char
#   SUPERADMIN_USERNAME=admin
#   SUPERADMIN_PASSWORD=min_12_char

# 4. Jalankan
npm run dev
```

Dashboard: **http://localhost:5173**  
API: **http://localhost:3030**

---

## Production Deployment

### Build

```bash
# Backend binary
go build -ldflags "-X wa-assistant/backend/license.DevMode=false" -o slaludiskon ./backend

# Frontend static
cd frontend && npm run build
```

### Systemd Service

```ini
[Unit]
Description=SlaluDiskon
After=network.target mysql.service

[Service]
Type=simple
WorkingDirectory=/opt/slaludiskon
ExecStart=/opt/slaludiskon/slaludiskon
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### Nginx Reverse Proxy

```nginx
server {
    listen 443 ssl;
    server_name slaludiskon.com;

    location / {
        root /var/www/html;
        try_files $uri /index.html;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:3030;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

---

## Environment Variables

### Wajib

| Variable | Keterangan |
|----------|-----------|
| `JWT_SECRET` | Secret key JWT (min 32 karakter random) |
| `SUPERADMIN_USERNAME` | Username admin pertama |
| `SUPERADMIN_PASSWORD` | Password admin pertama (min 12 karakter) |

### AI

| Variable | Keterangan |
|----------|-----------|
| `DEEPSEEK_API_KEY` | API key DeepSeek untuk chat AI |
| `OPENROUTER_API_KEY` | API key OpenRouter (fallback + vision) |

### Shipping

| Variable | Keterangan |
|----------|-----------|
| `MENGANTAR_API_KEY` | API key Mengantar untuk cek ongkir |
| `MENGANTAR_ORIGIN_AUTOFILL_ID` | ID alamat asal di Mengantar |
| `SHIPPING_TRANSFER_DISCOUNT` | Diskon ongkir untuk transfer (default: 3000) |

### Database

| Variable | Default | Keterangan |
|----------|---------|-----------|
| `DB_HOST` | `localhost` | Host MySQL |
| `DB_PORT` | `3306` | Port MySQL |
| `DB_USER` | `root` | User MySQL |
| `DB_PASS` | _(kosong)_ | Password MySQL |
| `DB_NAME` | `wa_assistant` | Nama database |
| `DB_PATH` | `./wa-assistant.db` | Path SQLite (fallback) |

---

## Arsitektur

```
WhatsApp Cloud
     │
     ▼
┌──────────┐     ┌─────────────┐     ┌──────────┐
│ whatsmeow │────▶│  Agent      │────▶│ DeepSeek  │
│ (WebSocket)│    │  Handler    │     │   API     │
└──────────┘     └──────┬──────┘     └──────────┘
                        │
              ┌─────────┼─────────┐
              ▼         ▼         ▼
        ┌─────────┐ ┌───────┐ ┌──────────┐
        │ Persona │ │  RAG  │ │ Shipping │
        │ (Prompt)│ │ (FAQ) │ │ (Ongkir) │
        └─────────┘ └───────┘ └──────────┘
                        │
                        ▼
              ┌─────────────────┐
              │   System Prompt │
              │   + Directives  │
              └────────┬────────┘
                       ▼
              ┌─────────────────┐
              │  Response Parse │
              │  (Directives)   │
              └────────┬────────┘
                       ▼
              ┌─────────────────┐
              │  WhatsApp Send  │
              └─────────────────┘
```

### Directive System

AI berkomunikasi dengan sistem melalui token yang di-parse backend:

| Directive | Aksi |
|-----------|------|
| `[[SEND_MEDIA:label]]` | Kirim media (gambar/video katalog) |
| `[[LABEL:nama]]` | Label kontak WhatsApp |
| `[[START_PRODUCT:ID]]` | Buka form checkout produk |
| `[[ESCALATE]]` | Handoff ke CS manusia |

---

## Fitur

### AI Chat
- Persona-based prompting (satu agent = satu persona)
- Knowledge base dengan semantic search (RAG)
- Anti-hallucination grounding
- Multi-model fallback chain

### Inbox
- Real-time chat monitoring
- Balas manual dari dashboard
- AI auto-reply dengan toggle on/off
- Cursor pagination untuk chat panjang

### Broadcast
- Kirim massal dengan jeda acak (anti-blokir)
- Rotasi multi-nomor
- Personalisasi `{nama}`
- Jadwal broadcast
- Opt-out otomatis

### CRM
- Pipeline lead: New → Cold → Warm → Hot → Customer
- Label WhatsApp sync (dua arah)
- Google Sheets export
- Follow-up bertahap

### Shipping
- Cek ongkir realtime (Mengantar API)
- JNE prioritas, JT fallback
- Auto city detection dari chat
- Diskon ongkir transfer

### REST API
- 30+ endpoint dengan API key per-agent
- Webhook realtime (message received, image analyzed)
- Dokumentasi interaktif di dashboard

---

## Development

### Struktur Projekt

```
backend/
  cmd/          Entry point alternatif
  config/       Env loader
  database/     DB init + migrasi
  handlers/     HTTP handler (50+ endpoint)
  license/      Verifikasi lisensi
  models/       GORM models
  services/     AI, WA, shipping, embedding

frontend/
  src/
    components/  UI components
    pages/       Halaman (Dashboard, Login)
    services/    API client
    hooks.ts     React Query hooks
    types.ts     TypeScript types

docs/           Dokumentasi tambahan
```

### Menambah Agent Baru

1. Dashboard → Agents → Create
2. Isi nama + persona
3. Scan QR untuk konek WhatsApp
4. AI otomatis aktif dengan persona tersebut

### Menambah Produk

1. Dashboard → Products → Create
2. Isi nama, harga, deskripsi, gambar
3. AI akan otomatis menyebutkan produk saat relevan

### Custom Persona

Persona ditulis dalam bahasa natural. Contoh struktur:

```
Kamu adalah [nama], CS dari [bisnis].
Produk: [produk] — Rp [harga]/paket.
Alur: 1) Opening, 2) Gali data, 3) Rekap.
Aturan: [daftar aturan].
```

---

## Lisensi

Produk berlisensi. Penggunaan tunduk pada EULA dan Disclaimer di folder `docs/`.

---

## Kontributor

- **Backend**: Go, whatsmeow, GORM, DeepSeek integration
- **Frontend**: React, TypeScript, MUI, Vite
- **Shipping**: Mengantar API integration
- **AI Pipeline**: Persona injection, RAG, grounding, directives
