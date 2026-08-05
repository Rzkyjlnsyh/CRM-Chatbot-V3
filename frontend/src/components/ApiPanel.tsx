import { useMemo, useState, type ReactNode } from 'react';
import {
  Accordion, AccordionDetails, AccordionSummary, Alert, Box, Button, Chip,
  CircularProgress, Divider, FormControl, IconButton, InputAdornment, InputLabel,
  MenuItem, Paper, Select, Stack, Step, StepLabel, Stepper, Tab, Tabs, TextField,
  ToggleButton, ToggleButtonGroup, Typography,
} from '@mui/material';
import ApiIcon from '@mui/icons-material/ApiOutlined';
import KeyIcon from '@mui/icons-material/VpnKeyOutlined';
import WebhookIcon from '@mui/icons-material/WebhookOutlined';
import CopyIcon from '@mui/icons-material/ContentCopyOutlined';
import RefreshIcon from '@mui/icons-material/RefreshOutlined';
import DeleteIcon from '@mui/icons-material/DeleteOutlined';
import TerminalIcon from '@mui/icons-material/TerminalOutlined';
import CheckIcon from '@mui/icons-material/CheckCircleRounded';
import CircleIcon from '@mui/icons-material/RadioButtonUncheckedOutlined';
import SearchIcon from '@mui/icons-material/SearchOutlined';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import SendIcon from '@mui/icons-material/SendOutlined';
import LinkIcon from '@mui/icons-material/LinkOutlined';
import ShieldIcon from '@mui/icons-material/ShieldOutlined';
import PersonIcon from '@mui/icons-material/PersonOutlineOutlined';
import StorageIcon from '@mui/icons-material/StorageOutlined';
import {
  useApiSettings, useRevokeApiKey, useRotateApiKey, useRotateWebhookSecret,
  useSaveWebhook, useTestApiMessage, useTestWebhook,
} from '../hooks';
import PageHeader from './PageHeader';
import { swalConfirm, swalToast } from '../services/swal';

const API_BASE = `${window.location.origin}/api/v1`;
const LANGS = ['cURL', 'Node.js', 'PHP', 'Python'] as const;
type Language = typeof LANGS[number];
type Method = 'GET' | 'POST' | 'PUT' | 'DELETE';
type Category = 'Pesan' | 'Kontak' | 'Percakapan' | 'Broadcast' | 'OTP' | 'Grup' | 'Sistem';

type Endpoint = {
  id: string;
  method: Method;
  path: string;
  samplePath?: string;
  title: string;
  description: string;
  category: Category;
  query?: string[];
  notes?: string[];
  body?: Record<string, unknown>;
  response: unknown;
  responseType?: 'json' | 'binary';
};

const ENDPOINTS: Endpoint[] = [
  {
    id: 'send-message', method: 'POST', path: '/messages', title: 'Kirim pesan', category: 'Pesan',
    description: 'Kirim teks, gambar, video, dokumen, kartu kontak, atau balasan pesan.',
    notes: [
      'type: text membutuhkan text.',
      'type: image, video, atau document membutuhkan media_url HTTPS; caption dan filename bersifat opsional.',
      'type: contact membutuhkan contact_number; contact_name bersifat opsional.',
      'reply_to diisi message_id bila ingin membalas pesan tertentu.',
    ],
    body: { to: '6281234567890', type: 'text', text: 'Pesanan kamu sudah dikirim.' },
    response: { status: 'sent', to: '6281234567890', type: 'text', message_id: '3EB0...' },
  },
  {
    id: 'message-media', method: 'GET', path: '/messages/:message_id/media', samplePath: '/messages/3EB0ABC123/media',
    title: 'Unduh media pesan', category: 'Pesan', description: 'Ambil media memakai message_id dari event webhook.',
    notes: ['Respons berupa file asli, bukan JSON. Simpan body respons sebagai file atau tampilkan sebagai Blob.'],
    response: 'File biner dengan Content-Type media asli', responseType: 'binary',
  },
  {
    id: 'message-analysis', method: 'GET', path: '/messages/:message_id/analysis', samplePath: '/messages/3EB0ABC123/analysis',
    title: 'Hasil analisis gambar', category: 'Pesan', description: 'Ambil hasil vision, jawaban alur, kecocokan produk, dan kebutuhan validasi CS dari sebuah gambar masuk.',
    response: { data: { message_id: '3EB0ABC123', from: '6281234567890', status: 'completed', analysis: 'Terlihat sofa warna abu-abu dalam kondisi cukup baik.', answer: 'Sofa abu-abu', product_id: 12, confidence: 0.91, needs_human: false, model: 'openai/gpt-4.1-mini' } },
  },
  {
    id: 'check-number', method: 'POST', path: '/check', title: 'Cek nomor WhatsApp', category: 'Pesan',
    description: 'Periksa maksimal 100 nomor dalam satu permintaan.',
    body: { numbers: ['081234567890', '628987654321'] },
    response: { data: [{ number: '6281234567890', registered: true }] },
  },
  {
    id: 'list-contacts', method: 'GET', path: '/contacts', samplePath: '/contacts?page=1&per_page=50',
    title: 'Daftar kontak', category: 'Kontak', description: 'Cari dan filter kontak tersimpan.',
    query: ['q: nama atau nomor', 'tags: filter tag', 'page: halaman', 'per_page: 1-500'],
    response: { data: [{ number: '6281234567890', name: 'Budi', tags: 'pelanggan' }], meta: { page: 1, per_page: 50, total: 1, total_pages: 1 } },
  },
  {
    id: 'save-contact', method: 'POST', path: '/contacts', title: 'Simpan kontak', category: 'Kontak',
    description: 'Buat kontak baru atau perbarui data jika nomornya sudah ada.',
    notes: ['lead_stage opsional: new, cold, warm, hot, customer, atau unqualified. Status yang dikirim lewat API dianggap pilihan manual.'],
    body: { number: '081234567890', name: 'Budi', notes: 'Pelanggan Jakarta', tags: 'pelanggan,vip', lead_stage: 'warm' },
    response: { data: { id: 12, number: '6281234567890', name: 'Budi', tags: 'pelanggan,vip', lead_stage: 'warm', lead_stage_source: 'manual' } },
  },
  {
    id: 'get-contact', method: 'GET', path: '/contacts/:number', samplePath: '/contacts/6281234567890',
    title: 'Detail kontak', category: 'Kontak', description: 'Ambil satu kontak berdasarkan nomor.',
    response: { data: { id: 12, number: '6281234567890', name: 'Budi', notes: 'Pelanggan Jakarta', tags: 'pelanggan,vip', lead_stage: 'warm', lead_stage_source: 'ai', lead_stage_reason: 'Menanyakan harga produk' } },
  },
  {
    id: 'update-contact', method: 'PUT', path: '/contacts/:number', samplePath: '/contacts/6281234567890',
    title: 'Perbarui kontak', category: 'Kontak', description: 'Ubah nama, catatan, tag, atau status CRM tanpa membuat kontak baru.',
    notes: ['Jika lead_stage dikirim, status dikunci sebagai pilihan manual agar tidak langsung ditimpa penilaian AI.'],
    body: { name: 'Budi Santoso', tags: 'pelanggan,vip', lead_stage: 'hot' },
    response: { data: { id: 12, number: '6281234567890', name: 'Budi Santoso', tags: 'pelanggan,vip', lead_stage: 'hot', lead_stage_source: 'manual', lead_stage_locked: true } },
  },
  {
    id: 'delete-contact', method: 'DELETE', path: '/contacts/:number', samplePath: '/contacts/6281234567890',
    title: 'Hapus kontak', category: 'Kontak', description: 'Hapus data kontak tersimpan. Riwayat percakapan tidak ikut dihapus.',
    response: { status: 'deleted', number: '6281234567890' },
  },
  {
    id: 'list-chats', method: 'GET', path: '/chats', samplePath: '/chats?page=1&per_page=50',
    title: 'Daftar percakapan', category: 'Percakapan', description: 'Ambil percakapan terbaru beserta kontak dan status handoff.',
    query: ['q: nomor', 'page: halaman', 'per_page: 1-200'],
    response: { data: [{ number: '6281234567890', name: 'Budi', last_message: 'Apakah tersedia?', needs_human: false }], meta: { page: 1, per_page: 50, total: 1, total_pages: 1 } },
  },
  {
    id: 'chat-messages', method: 'GET', path: '/chats/:number/messages', samplePath: '/chats/6281234567890/messages?page=1&per_page=50',
    title: 'Riwayat pesan', category: 'Percakapan', description: 'Ambil pesan masuk dan balasan terbaru. Pesan gambar juga menyertakan hasil analisis vision bila sudah tersedia.',
    query: ['page: halaman', 'per_page: 1-200'],
    response: { data: [{ id: 81, message: 'Apakah tersedia?', reply: 'Tersedia.', media_type: 'image', image_analysis: 'Terlihat sofa abu-abu.', image_analysis_confidence: 0.91, image_analysis_needs_human: false, created_at: '2026-07-10T08:00:00Z' }], meta: { page: 1, per_page: 50, total: 1, total_pages: 1 } },
  },
  {
    id: 'create-broadcast', method: 'POST', path: '/broadcasts', title: 'Buat broadcast', category: 'Broadcast',
    description: 'Kirim pesan massal asinkron dengan jeda, istirahat, opt-out, dan rotasi nomor.',
    notes: ['Maksimal 1.000 penerima per request.', 'agent_ids opsional dan hanya boleh berisi nomor lain milik akun yang sama serta sedang tersambung.'],
    body: { message: 'Halo {nama}, promo hari ini tersedia.', recipients: [{ number: '6281234567890', name: 'Budi' }], min_delay: 8, max_delay: 15, rest_every: 30, rest_duration: 120 },
    response: { id: 42, total: 1, status: 'pending' },
  },
  {
    id: 'list-broadcasts', method: 'GET', path: '/broadcasts', samplePath: '/broadcasts?page=1&per_page=20',
    title: 'Daftar broadcast', category: 'Broadcast', description: 'Ambil riwayat broadcast dan filter berdasarkan status.',
    query: ['status: pending, running, done, failed, cancelled', 'page: halaman', 'per_page: 1-100'],
    response: { data: [{ id: 42, status: 'running', total: 100, sent: 18, failed: 0 }], meta: { page: 1, per_page: 20, total: 1, total_pages: 1 } },
  },
  {
    id: 'broadcast-status', method: 'GET', path: '/broadcasts/:id', samplePath: '/broadcasts/42',
    title: 'Status broadcast', category: 'Broadcast', description: 'Polling progres dan hasil pengiriman broadcast.',
    response: { id: 42, status: 'running', total: 100, sent: 18, failed: 0, skipped: 0 },
  },
  {
    id: 'broadcast-recipients', method: 'GET', path: '/broadcasts/:id/recipients', samplePath: '/broadcasts/42/recipients?page=1&per_page=100',
    title: 'Hasil per penerima', category: 'Broadcast', description: 'Audit nomor yang terkirim, gagal, dilewati, atau masih menunggu.',
    query: ['status: pending, sent, failed, skipped', 'page: halaman', 'per_page: 1-500'],
    response: { data: [{ number: '6281234567890', status: 'sent', error: '' }], meta: { page: 1, per_page: 100, total: 1, total_pages: 1 } },
  },
  {
    id: 'cancel-broadcast', method: 'POST', path: '/broadcasts/:id/cancel', samplePath: '/broadcasts/42/cancel',
    title: 'Batalkan broadcast', category: 'Broadcast', description: 'Minta worker berhenti dengan aman setelah proses aktif selesai.',
    response: { status: 'cancel_requested' },
  },
  {
    id: 'otp-request', method: 'POST', path: '/otp/request', title: 'Kirim OTP', category: 'OTP',
    description: 'Buat dan kirim kode OTP dengan masa berlaku terbatas.',
    notes: ['length: 4-8 digit, default 6.', 'minutes: 1-30 menit, default 5.', 'Gunakan {code} dan {minutes} pada template message. Satu nomor hanya dapat meminta OTP sekali per 60 detik.'],
    body: { to: '6281234567890', length: 6, minutes: 5, message: 'Kode verifikasi kamu: {code}. Berlaku {minutes} menit.' },
    response: { status: 'sent', to: '6281234567890', expires_in: 300 },
  },
  {
    id: 'otp-verify', method: 'POST', path: '/otp/verify', title: 'Verifikasi OTP', category: 'OTP',
    description: 'Validasi kode OTP terbaru untuk nomor yang sama.',
    notes: ['Respons tetap HTTP 200 untuk kode benar maupun salah. Periksa field verified.', 'OTP hanya dapat dipakai sekali dan dibatalkan setelah 5 percobaan salah.'],
    body: { to: '6281234567890', code: '123456' }, response: { verified: true },
  },
  {
    id: 'list-groups', method: 'GET', path: '/groups', title: 'Daftar grup', category: 'Grup',
    description: 'Ambil grup WhatsApp yang dapat diakses nomor ini.', response: { data: [{ jid: '120363000000@g.us', name: 'Komunitas Pelanggan' }] },
  },
  {
    id: 'group-message', method: 'POST', path: '/groups/:jid/messages', samplePath: '/groups/120363000000@g.us/messages',
    title: 'Kirim ke grup', category: 'Grup', description: 'Kirim teks atau media ke JID grup.',
    body: { type: 'text', text: 'Pengumuman terbaru untuk anggota grup.' },
    response: { status: 'sent', to: '120363000000@g.us', type: 'text', message_id: '3EB0...' },
  },
  {
    id: 'status', method: 'GET', path: '/status', title: 'Status nomor', category: 'Sistem',
    description: 'Periksa identitas agent dan koneksi WhatsApp sebelum mengirim.',
    response: { agent_id: 1, number: '628111111111', name: 'CS Utama', connected: true },
  },
];

const METHOD_COLORS: Record<Method, string> = {
  GET: '#087f8c', POST: '#3859b8', PUT: '#a15c00', DELETE: '#b42318',
};
const CATEGORIES = ['Semua', 'Pesan', 'Kontak', 'Percakapan', 'Broadcast', 'OTP', 'Grup', 'Sistem'] as const;

const HTML_FORM_EXAMPLE = `<!doctype html>
<html lang="id">
<body>
  <form id="wa-form">
    <input id="nomor" placeholder="628123456789" required>
    <textarea id="pesan" placeholder="Tulis pesan" required></textarea>
    <button type="submit">Kirim WhatsApp</button>
  </form>
  <p id="hasil"></p>

  <script>
    document.querySelector('#wa-form').addEventListener('submit', async (event) => {
      event.preventDefault();
      const response = await fetch('/api/kirim-whatsapp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          to: document.querySelector('#nomor').value,
          text: document.querySelector('#pesan').value
        })
      });
      const result = await response.json();
      document.querySelector('#hasil').textContent = response.ok
        ? 'Pesan berhasil dikirim'
        : 'Gagal: ' + (result.error || 'Terjadi kesalahan');
    });
  </script>
</body>
</html>`;

const BACKEND_PROXY_EXAMPLE = `import express from 'express';

const app = express();
app.use(express.json());
app.use(express.static('public'));

app.post('/api/kirim-whatsapp', async (req, res) => {
  const response = await fetch('${API_BASE}/messages', {
    method: 'POST',
    headers: {
      Authorization: 'Bearer ' + process.env.SLALUDISKON_API_KEY,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ to: req.body.to, type: 'text', text: req.body.text })
  });
  res.status(response.status).json(await response.json());
});

app.listen(3000);`;

const WEBHOOK_NODE_EXAMPLE = `import crypto from 'node:crypto';
import express from 'express';

const app = express();
app.post('/webhooks/whatsapp', express.raw({ type: 'application/json' }), (req, res) => {
  const rawBody = req.body;
  const received = req.header('x-signature') || '';
  const expected = 'sha256=' + crypto
    .createHmac('sha256', process.env.SLALUDISKON_WEBHOOK_SECRET)
    .update(rawBody)
    .digest('hex');

  if (received.length !== expected.length || !crypto.timingSafeEqual(Buffer.from(received), Buffer.from(expected))) {
    return res.sendStatus(401);
  }
  const event = JSON.parse(rawBody.toString('utf8'));
  if (event.event === 'message.received') console.log('Pesan baru:', event.from, event.text);
  if (event.event === 'image.analyzed') console.log('Hasil gambar:', event.analysis, event.product_id);
  res.sendStatus(200);
});

app.listen(3000);`;

const WEBHOOK_PHP_EXAMPLE = `<?php
$rawBody = file_get_contents('php://input');
$received = $_SERVER['HTTP_X_SIGNATURE'] ?? '';
$expected = 'sha256=' . hash_hmac('sha256', $rawBody, getenv('SLALUDISKON_WEBHOOK_SECRET'));

if (!hash_equals($expected, $received)) {
  http_response_code(401);
  exit('Signature tidak valid');
}
$event = json_decode($rawBody, true);
if (($event['event'] ?? '') === 'message.received') {
  error_log('Pesan baru: ' . $event['from'] . ' - ' . $event['text']);
}
if (($event['event'] ?? '') === 'image.analyzed') {
  error_log('Hasil gambar: ' . $event['analysis']);
}
http_response_code(200);
echo 'OK';`;

function copy(text: string) {
  navigator.clipboard?.writeText(text).then(() => swalToast('Berhasil disalin.'), () => swalToast('Belum bisa menyalin.', 'error'));
}

function MethodBadge({ method }: { method: Method }) {
  return (
    <Box sx={{ bgcolor: METHOD_COLORS[method], color: '#fff', fontFamily: 'monospace', fontWeight: 800, fontSize: 11, px: 0.9, py: 0.35, borderRadius: 1, minWidth: 54, textAlign: 'center', flexShrink: 0 }}>
      {method}
    </Box>
  );
}

function CodeBlock({ value, label }: { value: string; label?: string }) {
  return (
    <Box>
      {label && <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5, fontWeight: 700 }}>{label}</Typography>}
      <Box sx={{ position: 'relative', bgcolor: '#17211c', color: '#e9f2eb', borderRadius: 1, overflow: 'hidden' }}>
        <Box component="pre" sx={{ m: 0, p: 1.5, pr: 5, overflowX: 'auto', fontSize: 12, lineHeight: 1.65, fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{value}</Box>
        <IconButton onClick={() => copy(value)} aria-label="Salin" sx={{ position: 'absolute', top: 6, right: 6, color: '#e9f2eb', bgcolor: 'rgba(255,255,255,0.08)' }}>
          <CopyIcon fontSize="small" />
        </IconButton>
      </Box>
    </Box>
  );
}

function InlineCode({ value }: { value: string }) {
  return (
    <Stack direction="row" spacing={1} sx={{ alignItems: 'center', bgcolor: 'action.hover', border: '1px solid', borderColor: 'divider', borderRadius: 1, px: 1.25, py: 0.65, minWidth: 0 }}>
      <Box component="code" sx={{ flex: 1, minWidth: 0, overflowX: 'auto', whiteSpace: 'nowrap', fontSize: 12.5 }}>{value}</Box>
      <IconButton onClick={() => copy(value)} aria-label="Salin"><CopyIcon fontSize="small" /></IconButton>
    </Stack>
  );
}

function StatusItem({ ready, label, value }: { ready: boolean; label: string; value: string }) {
  return (
    <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minWidth: 0, flex: 1 }}>
      {ready ? <CheckIcon color="success" fontSize="small" /> : <CircleIcon color="disabled" fontSize="small" />}
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>{label}</Typography>
        <Typography variant="body2" sx={{ fontWeight: 700 }}>{value}</Typography>
      </Box>
    </Stack>
  );
}

function buildSnippet(endpoint: Endpoint, lang: Language) {
  const url = API_BASE + (endpoint.samplePath || endpoint.path);
  const json = endpoint.body ? JSON.stringify(endpoint.body, null, 2) : '';
  if (lang === 'Node.js') {
    const readResult = endpoint.responseType === 'binary'
      ? `const file = await response.blob();\n// Simpan file atau tampilkan dengan URL.createObjectURL(file)`
      : `const result = await response.json();\nif (!response.ok) throw new Error(result.error || 'Request gagal');`;
    return `const response = await fetch('${url}', {\n  method: '${endpoint.method}',\n  headers: {\n    Authorization: 'Bearer <API_KEY>',\n    'Content-Type': 'application/json'\n  }${json ? `,\n  body: JSON.stringify(${json})` : ''}\n});\n\n${readResult}`;
  }
  if (lang === 'PHP') {
    const phpBody = endpoint.body ? JSON.stringify(endpoint.body).replace(/'/g, "\\'") : '';
    return `<?php\n$ch = curl_init('${url}');\ncurl_setopt_array($ch, [\n  CURLOPT_CUSTOMREQUEST => '${endpoint.method}',\n  CURLOPT_HTTPHEADER => [\n    'Authorization: Bearer <API_KEY>',\n    'Content-Type: application/json'\n  ],${phpBody ? `\n  CURLOPT_POSTFIELDS => '${phpBody}',` : ''}\n  CURLOPT_RETURNTRANSFER => true,\n]);\necho curl_exec($ch);`;
  }
  if (lang === 'Python') {
    const method = endpoint.method.toLowerCase();
    const output = endpoint.responseType === 'binary'
      ? `response.raise_for_status()\nwith open('media.bin', 'wb') as file:\n    file.write(response.content)`
      : `result = response.json()\nprint(result)\nresponse.raise_for_status()`;
    return `import requests\n\nresponse = requests.${method}(\n    '${url}',\n    headers={'Authorization': 'Bearer <API_KEY>'}${json ? `,\n    json=${json}` : ''}\n)\n${output}`;
  }
  const headers = [`-H 'Authorization: Bearer <API_KEY>'`];
  if (json) {
    headers.push(`-H 'Content-Type: application/json'`);
    headers.push(`-d '${JSON.stringify(endpoint.body)}'`);
  }
  if (endpoint.responseType === 'binary') headers.push(`--output media.bin`);
  return `curl -X ${endpoint.method} '${url}' \\\n  ${headers.join(' \\\n  ')}`;
}

function LanguageToggle({ value, onChange }: { value: Language; onChange: (value: Language) => void }) {
  return (
    <ToggleButtonGroup size="small" exclusive value={value} onChange={(_, next) => next && onChange(next)} sx={{ flexWrap: 'wrap' }}>
      {LANGS.map(item => <ToggleButton key={item} value={item} sx={{ px: 1.5, py: 0.35 }}>{item}</ToggleButton>)}
    </ToggleButtonGroup>
  );
}

function SecretReveal({ label, value, onClose }: { label: string; value: string; onClose: () => void }) {
  return (
    <Alert severity="success" onClose={onClose} sx={{ mt: 1.5 }}>
      <Typography variant="body2" sx={{ fontWeight: 700, mb: 0.75 }}>{label} hanya ditampilkan sekali. Simpan di secret manager.</Typography>
      <InlineCode value={value} />
    </Alert>
  );
}

function WebhookFlowNode({ icon, title, detail, accent = false }: { icon: ReactNode; title: string; detail: string; accent?: boolean }) {
  return (
    <Box sx={{
      width: { xs: '100%', sm: 150 }, minHeight: { sm: 116 }, p: 1.25,
      border: '1px solid', borderColor: accent ? 'primary.main' : 'divider', borderRadius: 1.25,
      bgcolor: accent ? 'rgba(25, 118, 210, 0.04)' : 'background.paper', textAlign: 'center',
      display: 'flex', flexDirection: { xs: 'row', sm: 'column' }, alignItems: 'center', justifyContent: 'center', gap: 0.75,
      boxShadow: accent ? '0 0 0 3px rgba(25, 118, 210, 0.08)' : 'none',
    }}>
      <Box sx={{
        width: 34, height: 34, borderRadius: '50%', flexShrink: 0, display: 'grid', placeItems: 'center',
        bgcolor: accent ? 'primary.main' : 'action.hover', color: accent ? 'primary.contrastText' : 'primary.main',
        animation: accent ? 'webhookPulse 2s ease-in-out infinite' : 'none',
        '@keyframes webhookPulse': { '0%, 100%': { transform: 'scale(1)' }, '50%': { transform: 'scale(1.08)' } },
        '@media (prefers-reduced-motion: reduce)': { animation: 'none' },
      }}>{icon}</Box>
      <Box sx={{ minWidth: 0, textAlign: { xs: 'left', sm: 'center' } }}>
        <Typography variant="body2" sx={{ fontWeight: 800 }}>{title}</Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.35 }}>{detail}</Typography>
      </Box>
    </Box>
  );
}

function WebhookFlowConnector({ delay }: { delay: number }) {
  return (
    <Box sx={{
      position: 'relative', flexShrink: 0,
      width: { xs: 2, sm: 56 }, height: { xs: 34, sm: 2 }, bgcolor: 'divider', overflow: 'visible',
      '&::after': {
        content: '""', position: 'absolute', width: 8, height: 8, borderRadius: '50%', bgcolor: 'primary.main',
        left: { xs: -3, sm: 0 }, top: { xs: 0, sm: -3 },
        animation: { xs: `webhookFlowY 1.8s ${delay}s linear infinite`, sm: `webhookFlowX 1.8s ${delay}s linear infinite` },
        boxShadow: '0 0 0 4px rgba(25, 118, 210, 0.12)',
      },
      '@keyframes webhookFlowX': { from: { transform: 'translateX(0)' }, to: { transform: 'translateX(56px)' } },
      '@keyframes webhookFlowY': { from: { transform: 'translateY(0)' }, to: { transform: 'translateY(34px)' } },
      '@media (prefers-reduced-motion: reduce)': { '&::after': { animation: 'none', left: { sm: 24 }, top: { xs: 13, sm: -3 } } },
    }} />
  );
}

function WebhookFlowVisual() {
  return (
    <Paper variant="outlined" sx={{ p: { xs: 1.25, sm: 2 }, overflow: 'hidden' }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={0.5} sx={{ mb: 1.5, alignItems: { sm: 'center' }, justifyContent: 'space-between' }}>
        <Box>
          <Typography sx={{ fontWeight: 850 }}>Cara kerja webhook</Typography>
          <Typography variant="caption" color="text.secondary">Data dikirim otomatis saat aktivitas terjadi—sistem Anda tidak perlu mengecek SlaluDiskon berulang kali.</Typography>
        </Box>
        <Chip size="small" color="success" variant="outlined" label="Otomatis & realtime" />
      </Stack>
      <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ alignItems: 'center', justifyContent: 'center' }}>
        <WebhookFlowNode icon={<PersonIcon fontSize="small" />} title="Pelanggan" detail="Mengirim pesan WhatsApp" />
        <WebhookFlowConnector delay={0} />
        <WebhookFlowNode icon={<ApiIcon fontSize="small" />} title="SlaluDiskon" detail="Menerima aktivitas baru" />
        <WebhookFlowConnector delay={0.45} />
        <WebhookFlowNode accent icon={<WebhookIcon fontSize="small" />} title="Webhook" detail="Mengirim event JSON" />
        <WebhookFlowConnector delay={0.9} />
        <WebhookFlowNode icon={<StorageIcon fontSize="small" />} title="Sistem Anda" detail="Menyimpan atau memproses data" />
      </Stack>
      <Stack direction="row" sx={{ mt: 1.5, gap: 0.75, flexWrap: 'wrap', justifyContent: 'center' }}>
        <Chip size="small" label="Pesan baru" />
        <Chip size="small" label="Analisis gambar selesai" />
        <Chip size="small" label="Pesan dibaca" />
      </Stack>
    </Paper>
  );
}

const SETUP_STEPS = [
  { key: 'wa', label: 'Hubungkan WhatsApp' },
  { key: 'key', label: 'Buat API key' },
  { key: 'test', label: 'Uji kirim pesan' },
  { key: 'webhook', label: 'Webhook (opsional)' },
] as const;

const ERROR_EXAMPLES: { code: number; title: string; body: Record<string, string>; note?: string }[] = [
  { code: 400, title: 'Input salah', body: { error: "Field 'text' wajib diisi untuk pesan teks." } },
  { code: 401, title: 'API key salah/hilang', body: { error: "API key tidak ada. Sertakan header 'Authorization: Bearer <token>'." } },
  { code: 409, title: 'WhatsApp belum siap', body: { error: 'Nomor WhatsApp sedang tidak tersambung.' } },
  { code: 429, title: 'Terlalu banyak request', body: { error: 'Terlalu banyak permintaan. Coba lagi sebentar.' }, note: 'Header Retry-After: detik tunggu' },
  { code: 502, title: 'Gagal ke WhatsApp/media', body: { error: 'Gagal mengirim: connection closed' } },
];

export default function ApiPanel({ agentId, onOpenDashboard }: { agentId: number; onOpenDashboard?: () => void }) {
  const { data: settings, isLoading } = useApiSettings(agentId);
  const rotateKey = useRotateApiKey(agentId);
  const revokeKey = useRevokeApiKey(agentId);
  const saveWebhook = useSaveWebhook(agentId);
  const rotateSecret = useRotateWebhookSecret(agentId);
  const testWebhook = useTestWebhook(agentId);
  const testMessage = useTestApiMessage(agentId);
  const [tab, setTab] = useState(0);
  const [language, setLanguage] = useState<Language>('cURL');
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState<(typeof CATEGORIES)[number]>('Semua');
  const [expanded, setExpanded] = useState<string | false>('send-message');
  const [webhookUrl, setWebhookUrl] = useState('');
  const [urlDirty, setUrlDirty] = useState(false);
  const [newKey, setNewKey] = useState('');
  const [newSecret, setNewSecret] = useState('');
  const [webhookExample, setWebhookExample] = useState<'Node.js' | 'PHP'>('Node.js');
  const [testTo, setTestTo] = useState('');
  const [testText, setTestText] = useState('Uji REST API SlaluDiskon — pesan dari dashboard.');
  const [testResult, setTestResult] = useState<string>('');
  const urlValue = urlDirty ? webhookUrl : (settings?.webhook_url ?? '');
  const apiReady = !!settings?.has_key && !!settings?.connected;
  const testSentOk = /"status"\s*:\s*"sent"/.test(testResult);
  const setupDone = [
    !!settings?.connected,
    !!settings?.has_key,
    testSentOk,
    !!settings?.webhook_url,
  ];
  // Index langkah yang belum selesai; bila semua selesai = panjang array (stepper “complete”).
  const activeSetupStep = (() => {
    const idx = setupDone.findIndex(done => !done);
    return idx === -1 ? SETUP_STEPS.length : idx;
  })();

  const filteredEndpoints = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return ENDPOINTS.filter(endpoint =>
      (category === 'Semua' || endpoint.category === category)
      && (!needle || `${endpoint.method} ${endpoint.path} ${endpoint.title} ${endpoint.description}`.toLowerCase().includes(needle)),
    );
  }, [category, search]);

  const createKey = async () => {
    if (settings?.has_key && !await swalConfirm('Putar ulang API key?', 'API key lama langsung berhenti berfungsi.')) return;
    try {
      const result = await rotateKey.mutateAsync();
      setNewKey(result.api_key);
    } catch {
      swalToast('API key belum bisa dibuat.', 'error');
    }
  };

  const removeKey = async () => {
    if (!await swalConfirm('Cabut API key?', 'Semua integrasi yang memakai key ini akan ditolak.')) return;
    try {
      await revokeKey.mutateAsync();
      setNewKey('');
      swalToast('API key dicabut.');
    } catch {
      swalToast('API key belum bisa dicabut.', 'error');
    }
  };

  const storeWebhook = async () => {
    try {
      const result = await saveWebhook.mutateAsync(urlValue.trim());
      setUrlDirty(false);
      if (result.webhook_secret) setNewSecret(result.webhook_secret);
      swalToast(urlValue.trim() ? 'Webhook tersimpan.' : 'Webhook dinonaktifkan.');
    } catch (error) {
      const detail = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(detail || 'Webhook belum bisa disimpan.', 'error');
    }
  };

  const renewSecret = async () => {
    if (!await swalConfirm('Putar ulang secret?', 'Verifikasi signature harus memakai secret baru.')) return;
    try {
      const result = await rotateSecret.mutateAsync();
      setNewSecret(result.webhook_secret);
    } catch {
      swalToast('Secret belum bisa diperbarui.', 'error');
    }
  };

  const sendWebhookTest = async () => {
    try {
      const result = await testWebhook.mutateAsync();
      swalToast(`Event uji diterima endpoint (HTTP ${result.http_status}).`);
    } catch (error) {
      const detail = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(detail || 'Event uji belum berhasil dikirim.', 'error');
    }
  };

  const sendTestMessage = async () => {
    const to = testTo.trim();
    if (!to) {
      swalToast('Isi nomor tujuan dulu (format 08… atau 62…).', 'error');
      return;
    }
    setTestResult('');
    try {
      const result = await testMessage.mutateAsync({ to, text: testText.trim() || undefined });
      setTestResult(JSON.stringify(result, null, 2));
      swalToast(`Pesan uji terkirim ke ${result.to}.`);
    } catch (error) {
      const detail = (error as { response?: { data?: { error?: string }; status?: number } })?.response?.data?.error;
      const status = (error as { response?: { status?: number } })?.response?.status;
      setTestResult(JSON.stringify({ error: detail || 'Gagal mengirim uji', status }, null, 2));
      swalToast(detail || 'Pesan uji belum berhasil dikirim.', 'error');
    }
  };

  if (isLoading) return <Box sx={{ py: 6, textAlign: 'center' }}><CircularProgress /></Box>;
  if (settings && !settings.allowed) {
    return <Box sx={{ maxWidth: 1080 }}><PageHeader title="REST API" /><Alert severity="info">REST API belum tersedia untuk akun ini.</Alert></Box>;
  }

  return (
    <Box sx={{ maxWidth: 1080 }}>
      <PageHeader
        title="REST API"
        subtitle="Hubungkan website atau aplikasi ke WhatsApp. Ikuti langkah di bawah, lalu salin snippet ke sistem Anda."
        action={<Chip icon={apiReady ? <CheckIcon /> : <KeyIcon />} color={apiReady ? 'success' : 'default'} label={apiReady ? 'Siap mengirim' : settings?.has_key ? 'Hubungkan WhatsApp' : 'Buat API key'} />}
      />

      <Paper variant="outlined" sx={{ px: 1.5, mb: 1.5 }}>
        <Tabs value={tab} onChange={(_, value) => setTab(value)} variant="scrollable" scrollButtons="auto" aria-label="Pengaturan REST API">
          <Tab icon={<ApiIcon fontSize="small" />} iconPosition="start" label="Mulai di sini" />
          <Tab icon={<TerminalIcon fontSize="small" />} iconPosition="start" label={`Daftar endpoint (${ENDPOINTS.length})`} />
          <Tab icon={<WebhookIcon fontSize="small" />} iconPosition="start" label="Webhook ke sistem Anda" />
        </Tabs>
      </Paper>

      {tab === 0 && (
        <Stack spacing={1.5}>
          <Alert severity="info">
            <b>REST API</b> = website Anda menyuruh SlaluDiskon mengirim WhatsApp.
            {' '}<b>Webhook</b> = SlaluDiskon memberi tahu website Anda saat ada pesan/status baru.
          </Alert>

          {/* Stepper setup */}
          <Paper variant="outlined" sx={{ p: { xs: 1.5, sm: 2 } }}>
            <Typography sx={{ fontWeight: 700, mb: 1.5 }}>Langkah setup</Typography>
            <Stepper activeStep={activeSetupStep} alternativeLabel sx={{ mb: 2, display: { xs: 'none', sm: 'flex' } }}>
              {SETUP_STEPS.map((step, index) => (
                <Step key={step.key} completed={setupDone[index]}>
                  <StepLabel>{step.label}</StepLabel>
                </Step>
              ))}
            </Stepper>
            <Stack spacing={1} sx={{ display: { xs: 'flex', sm: 'none' }, mb: 1.5 }}>
              {SETUP_STEPS.map((step, index) => (
                <Stack key={step.key} direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                  {setupDone[index]
                    ? <CheckIcon color="success" fontSize="small" />
                    : <CircleIcon color={index === activeSetupStep ? 'primary' : 'disabled'} fontSize="small" />}
                  <Typography variant="body2" sx={{ fontWeight: index === activeSetupStep ? 700 : 500 }}>{index + 1}. {step.label}</Typography>
                </Stack>
              ))}
            </Stack>

            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(2, minmax(0, 1fr))' }, gap: 1.25 }}>
              <Paper variant="outlined" sx={{ p: 1.5, borderColor: settings?.connected ? 'success.main' : 'divider' }}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 0.75 }}>
                  <Chip size="small" color={settings?.connected ? 'success' : 'warning'} label="1" sx={{ minWidth: 28 }} />
                  <Typography variant="body2" sx={{ fontWeight: 700 }}>Hubungkan WhatsApp</Typography>
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                  Nomor CS yang dipilih di sidebar harus Online sebelum API bisa mengirim.
                </Typography>
                {settings?.connected
                  ? <Chip size="small" color="success" icon={<CheckIcon />} label="WhatsApp tersambung" />
                  : (
                    <Button size="small" variant="contained" onClick={() => onOpenDashboard?.()}>
                      Buka Dashboard · tautkan WA
                    </Button>
                  )}
              </Paper>

              <Paper variant="outlined" sx={{ p: 1.5, borderColor: settings?.has_key ? 'success.main' : 'divider' }}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 0.75 }}>
                  <Chip size="small" color={settings?.has_key ? 'success' : 'primary'} label="2" sx={{ minWidth: 28 }} />
                  <Typography variant="body2" sx={{ fontWeight: 700 }}>Buat API key</Typography>
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                  Satu key hanya untuk nomor ini. Header: <code>Authorization: Bearer …</code>
                </Typography>
                {settings?.has_key && <Box sx={{ mb: 1 }}><InlineCode value={settings.key_hint || 'API key aktif'} /></Box>}
                <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap', gap: 1 }}>
                  <Button size="small" variant={settings?.has_key ? 'outlined' : 'contained'} startIcon={settings?.has_key ? <RefreshIcon /> : <KeyIcon />} onClick={createKey} disabled={rotateKey.isPending}>
                    {settings?.has_key ? 'Putar ulang' : 'Buat API key'}
                  </Button>
                  {settings?.has_key && <Button size="small" color="error" variant="outlined" startIcon={<DeleteIcon />} onClick={removeKey} disabled={revokeKey.isPending}>Cabut</Button>}
                </Stack>
                {newKey && <SecretReveal label="API key baru — simpan sekarang" value={newKey} onClose={() => setNewKey('')} />}
              </Paper>

              <Paper variant="outlined" sx={{ p: 1.5, gridColumn: { md: '1 / -1' } }}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 0.75 }}>
                  <Chip size="small" color={testSentOk ? 'success' : 'primary'} label="3" sx={{ minWidth: 28 }} />
                  <Typography variant="body2" sx={{ fontWeight: 700 }}>Uji kirim dari dashboard</Typography>
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.25 }}>
                  Menguji jalur yang sama dengan <code>POST /api/v1/messages</code> tanpa menaruh API key di browser. Kirim ke nomor Anda sendiri dulu.
                </Typography>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ mb: 1 }}>
                  <TextField
                    size="small" fullWidth label="Nomor tujuan" placeholder="6281234567890"
                    value={testTo} onChange={e => setTestTo(e.target.value)}
                    disabled={!settings?.has_key || !settings?.connected || testMessage.isPending}
                  />
                  <TextField
                    size="small" fullWidth label="Pesan uji"
                    value={testText} onChange={e => setTestText(e.target.value)}
                    disabled={!settings?.has_key || !settings?.connected || testMessage.isPending}
                  />
                  <Button
                    variant="contained" startIcon={testMessage.isPending ? <CircularProgress size={16} color="inherit" /> : <SendIcon />}
                    onClick={sendTestMessage}
                    disabled={!settings?.has_key || !settings?.connected || testMessage.isPending}
                    sx={{ flexShrink: 0, minWidth: { sm: 140 }, borderRadius: 999 }}
                  >
                    {testMessage.isPending ? 'Mengirim…' : 'Kirim uji'}
                  </Button>
                </Stack>
                {!settings?.connected && (
                  <Alert severity="warning" sx={{ mb: 1 }}>
                    WhatsApp belum online.{' '}
                    <Button size="small" onClick={() => onOpenDashboard?.()}>Hubungkan di Dashboard</Button>
                  </Alert>
                )}
                {settings?.connected && !settings?.has_key && (
                  <Alert severity="warning" sx={{ mb: 1 }}>Buat API key dulu (langkah 2) sebelum uji kirim.</Alert>
                )}
                {testResult && <CodeBlock label="Respons uji" value={testResult} />}
              </Paper>

              <Paper variant="outlined" sx={{ p: 1.5, gridColumn: { md: '1 / -1' } }}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 0.75 }}>
                  <Chip size="small" color={settings?.webhook_url ? 'success' : 'default'} label="4" sx={{ minWidth: 28 }} />
                  <Typography variant="body2" sx={{ fontWeight: 700 }}>Webhook (opsional)</Typography>
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                  Perlu server HTTPS milik Anda untuk menerima pesan masuk secara realtime.
                </Typography>
                <Button size="small" variant="outlined" startIcon={<WebhookIcon />} onClick={() => setTab(2)}>
                  Atur webhook
                </Button>
              </Paper>
            </Box>
          </Paper>

          <Paper variant="outlined" sx={{ p: 1.5 }}>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} divider={<Divider orientation="vertical" flexItem sx={{ display: { xs: 'none', sm: 'block' } }} />}>
              <StatusItem ready={!!settings?.has_key} label="Autentikasi" value={settings?.has_key ? 'API key aktif' : 'API key belum dibuat'} />
              <StatusItem ready={!!settings?.connected} label="WhatsApp" value={settings?.connected ? 'Nomor tersambung' : 'Nomor belum tersambung'} />
              <StatusItem ready={!!settings?.webhook_url} label="Webhook" value={settings?.webhook_url ? 'Event aktif' : 'Belum dikonfigurasi'} />
            </Stack>
          </Paper>

          <Paper variant="outlined" sx={{ p: 2 }}>
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 1.25 }}>
              <LinkIcon color="primary" />
              <Typography sx={{ fontWeight: 700 }}>Snippet untuk integrasi eksternal</Typography>
            </Stack>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>Base URL</Typography>
            <InlineCode value={API_BASE} />
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'center' }, justifyContent: 'space-between', mt: 1.5, mb: 1 }}>
              <Typography variant="body2" sx={{ fontWeight: 700 }}>POST /messages — kirim teks</Typography>
              <LanguageToggle value={language} onChange={setLanguage} />
            </Stack>
            <CodeBlock value={buildSnippet(ENDPOINTS[0], language)} />
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
              Ganti <code>&lt;API_KEY&gt;</code> dengan key dari langkah 2. Jangan commit key ke git.
            </Typography>
          </Paper>

          <Accordion disableGutters elevation={0} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: '8px !important', '&:before': { display: 'none' }, overflow: 'hidden' }}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Box>
                <Typography variant="body2" sx={{ fontWeight: 700 }}>Contoh website HTML + backend proxy</Typography>
                <Typography variant="caption" color="text.secondary">
                  Path <code>/api/kirim-whatsapp</code> ada di <b>server Anda</b>, bukan di SlaluDiskon. Server itulah yang memanggil SlaluDiskon dengan API key.
                </Typography>
              </Box>
            </AccordionSummary>
            <AccordionDetails sx={{ pt: 0 }}>
              <Alert severity="warning" sx={{ mb: 1.5 }}>
                Jangan menaruh API key di HTML/JavaScript browser. Pengunjung bisa menyalin dan menyalahgunakannya.
              </Alert>
              <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5}>
                <Box sx={{ flex: 1, minWidth: 0 }}><CodeBlock label="1. public/index.html (frontend Anda)" value={HTML_FORM_EXAMPLE} /></Box>
                <Box sx={{ flex: 1, minWidth: 0 }}><CodeBlock label="2. server.js (backend Anda → SlaluDiskon)" value={BACKEND_PROXY_EXAMPLE} /></Box>
              </Stack>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                Simpan key sebagai <code>SLALUDISKON_API_KEY</code>, jalankan server, buka HTML.
              </Typography>
            </AccordionDetails>
          </Accordion>

          <Alert severity="info" icon={<ShieldIcon fontSize="inherit" />}>
            Batas default 60 request/menit. Respons 429 menyertakan header Retry-After. Nomor 08… atau 62… dinormalisasi otomatis.
            Auth juga menerima header <code>X-API-Key</code> (selain Bearer).
          </Alert>

          <Paper variant="outlined" sx={{ p: 1.5 }}>
            <Typography variant="body2" sx={{ fontWeight: 700, mb: 1 }}>Contoh respons error</Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(2, minmax(0, 1fr))' }, gap: 1.25 }}>
              {ERROR_EXAMPLES.map(item => (
                <Box key={item.code}>
                  <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', mb: 0.5 }}>
                    <Chip size="small" color={item.code >= 500 ? 'error' : item.code >= 400 ? 'warning' : 'default'} label={item.code} />
                    <Typography variant="caption" sx={{ fontWeight: 700 }}>{item.title}</Typography>
                  </Stack>
                  <CodeBlock value={JSON.stringify(item.body, null, 2) + (item.note ? `\n// ${item.note}` : '')} />
                </Box>
              ))}
            </Box>
            <Stack direction="row" sx={{ gap: 0.75, flexWrap: 'wrap', mt: 1.25 }}>
              {['2xx · berhasil', '400 · input', '401 · auth', '409 · WA offline', '429 · rate limit', '502 · WA/media'].map(item => (
                <Chip key={item} size="small" variant="outlined" label={item} />
              ))}
            </Stack>
          </Paper>

          <Paper variant="outlined" sx={{ p: 1.5 }}>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'center' }, justifyContent: 'space-between' }}>
              <Box>
                <Typography variant="body2" sx={{ fontWeight: 700 }}>Butuh daftar endpoint lengkap?</Typography>
                <Typography variant="caption" color="text.secondary">Cari per kategori: Pesan, Kontak, Broadcast, OTP, dan lainnya.</Typography>
              </Box>
              <Button variant="outlined" startIcon={<TerminalIcon />} onClick={() => setTab(1)} sx={{ borderRadius: 999 }}>
                Buka daftar endpoint
              </Button>
            </Stack>
          </Paper>
        </Stack>
      )}

      {tab === 1 && (
        <Box>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ mb: 1.5 }}>
            <TextField
              size="small" fullWidth placeholder="Cari endpoint atau fungsi..." value={search} onChange={event => setSearch(event.target.value)}
              slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchIcon fontSize="small" /></InputAdornment> } }}
            />
            <FormControl size="small" sx={{ minWidth: { sm: 170 } }}>
              <InputLabel>Kategori</InputLabel>
              <Select value={category} label="Kategori" onChange={event => setCategory(event.target.value as typeof category)}>
                {CATEGORIES.map(item => <MenuItem key={item} value={item}>{item}</MenuItem>)}
              </Select>
            </FormControl>
          </Stack>

          <Paper variant="outlined" sx={{ overflow: 'hidden' }}>
            {filteredEndpoints.map((endpoint, index) => (
              <Accordion
                key={endpoint.id} disableGutters elevation={0} expanded={expanded === endpoint.id}
                onChange={(_, open) => setExpanded(open ? endpoint.id : false)}
                sx={{ borderBottom: index < filteredEndpoints.length - 1 ? '1px solid' : 0, borderColor: 'divider', '&:before': { display: 'none' } }}
              >
                <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={{ minHeight: 58, '& .MuiAccordionSummary-content': { minWidth: 0, alignItems: 'center', gap: 1.25 } }}>
                  <MethodBadge method={endpoint.method} />
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={{ xs: 0.2, sm: 1 }} sx={{ alignItems: { sm: 'center' } }}>
                      <Box component="code" sx={{ fontSize: 12.5, fontWeight: 800, overflowWrap: 'anywhere' }}>{endpoint.path}</Box>
                      <Typography variant="body2" color="text.secondary">{endpoint.title}</Typography>
                    </Stack>
                  </Box>
                  <Chip label={endpoint.category} size="small" variant="outlined" sx={{ display: { xs: 'none', sm: 'inline-flex' } }} />
                </AccordionSummary>
                <AccordionDetails sx={{ pt: 0, pb: 2 }}>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>{endpoint.description}</Typography>
                  {endpoint.query && (
                    <Box sx={{ mb: 1.5 }}>
                      <Typography variant="caption" sx={{ fontWeight: 800 }}>Query parameter</Typography>
                      <Stack direction="row" spacing={0.75} sx={{ mt: 0.75, flexWrap: 'wrap', gap: 0.75 }}>
                        {endpoint.query.map(item => <Chip key={item} label={item} size="small" variant="outlined" />)}
                      </Stack>
                    </Box>
                  )}
                  {endpoint.notes && (
                    <Alert severity="info" icon={false} sx={{ mb: 1.5 }}>
                      <Box component="ul" sx={{ m: 0, pl: 2 }}>
                        {endpoint.notes.map(note => <li key={note}><Typography variant="caption">{note}</Typography></li>)}
                      </Box>
                    </Alert>
                  )}
                  <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} sx={{ mb: 1.5 }}>
                    {endpoint.body && <Box sx={{ flex: 1, minWidth: 0 }}><CodeBlock label="Request JSON" value={JSON.stringify(endpoint.body, null, 2)} /></Box>}
                    <Box sx={{ flex: 1, minWidth: 0 }}><CodeBlock label={endpoint.responseType === 'binary' ? 'Respons file' : 'Respons berhasil'} value={typeof endpoint.response === 'string' ? endpoint.response : JSON.stringify(endpoint.response, null, 2)} /></Box>
                  </Stack>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'center' }, justifyContent: 'space-between', mb: 0.75 }}>
                    <Typography variant="caption" sx={{ fontWeight: 800 }}>Contoh request</Typography>
                    <LanguageToggle value={language} onChange={setLanguage} />
                  </Stack>
                  <CodeBlock value={buildSnippet(endpoint, language)} />
                </AccordionDetails>
              </Accordion>
            ))}
            {filteredEndpoints.length === 0 && <Box sx={{ p: 4, textAlign: 'center' }}><Typography color="text.secondary">Endpoint tidak ditemukan.</Typography></Box>}
          </Paper>

          <Stack direction="row" spacing={1} sx={{ mt: 1.5, alignItems: 'flex-start' }}>
            <TerminalIcon fontSize="small" color="action" />
            <Typography variant="caption" color="text.secondary">
              Semua endpoint memakai JSON, kecuali endpoint unduh media. Respons list memakai meta page, per_page, total, dan total_pages.
            </Typography>
          </Stack>
        </Box>
      )}

      {tab === 2 && (
        <Stack spacing={1.5}>
          <Alert severity="info">
            <b>Webhook adalah notifikasi otomatis ke server Anda.</b> Saat pesan masuk, gambar selesai dianalisis, atau status berubah, SlaluDiskon mengirim HTTP POST berisi JSON ke URL yang disimpan. Halaman HTML biasa tidak dapat menerima webhook; Anda membutuhkan endpoint backend yang dapat diakses melalui HTTPS.
          </Alert>

          <WebhookFlowVisual />

          <Paper variant="outlined" sx={{ p: 2 }}>
            <Typography sx={{ fontWeight: 800, mb: 1 }}>Cara memasang webhook</Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(3, minmax(0, 1fr))' }, gap: 1 }}>
              {[
                ['1', 'Buat endpoint', 'Siapkan URL HTTPS di backend yang menerima POST JSON.'],
                ['2', 'Simpan URL', 'Masukkan URL tersebut pada kolom Endpoint webhook.'],
                ['3', 'Kirim uji', 'Klik Kirim uji. Jika HTTP 2xx diterima, integrasi siap.'],
              ].map(([number, title, detail]) => (
                <Stack key={number} direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                  <Box sx={{ width: 24, height: 24, flexShrink: 0, borderRadius: '50%', bgcolor: 'primary.main', color: 'primary.contrastText', display: 'grid', placeItems: 'center', fontSize: 12, fontWeight: 800 }}>{number}</Box>
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 700 }}>{title}</Typography>
                    <Typography variant="caption" color="text.secondary">{detail}</Typography>
                  </Box>
                </Stack>
              ))}
            </Box>
          </Paper>

          <Paper variant="outlined" sx={{ p: 2 }}>
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 0.75 }}>
              <WebhookIcon color="primary" />
              <Typography sx={{ fontWeight: 800, flex: 1 }}>Endpoint webhook</Typography>
              <Chip size="small" color={settings?.webhook_url ? 'success' : 'default'} label={settings?.webhook_url ? 'Aktif' : 'Nonaktif'} />
            </Stack>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
              SlaluDiskon mengirim event pesan masuk, hasil analisis gambar, dan status pengiriman ke URL HTTPS milikmu. Kosongkan URL untuk menonaktifkan.
            </Typography>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
              <TextField fullWidth size="small" label="URL webhook" placeholder="https://app.example.com/webhooks/whatsapp" value={urlValue} onChange={event => { setWebhookUrl(event.target.value); setUrlDirty(true); }} />
              <Button variant="contained" onClick={storeWebhook} disabled={saveWebhook.isPending}>Simpan</Button>
              <Button variant="outlined" startIcon={<SendIcon />} onClick={sendWebhookTest} disabled={!settings?.webhook_url || testWebhook.isPending}>Kirim uji</Button>
            </Stack>
            {settings?.has_webhook_secret && (
              <Stack direction="row" spacing={1} sx={{ mt: 1.25, alignItems: 'center', flexWrap: 'wrap', gap: 1 }}>
                <Chip size="small" color="success" variant="outlined" label={`Secret ${settings.webhook_secret_hint}`} />
                <Button startIcon={<RefreshIcon />} onClick={renewSecret} disabled={rotateSecret.isPending}>Putar ulang secret</Button>
              </Stack>
            )}
            {newSecret && <SecretReveal label="Webhook secret baru" value={newSecret} onClose={() => setNewSecret('')} />}
          </Paper>

          <Paper variant="outlined" sx={{ p: 2 }}>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ justifyContent: 'space-between', alignItems: { sm: 'center' }, mb: 1.25 }}>
              <Box>
                <Typography sx={{ fontWeight: 800 }}>Contoh endpoint penerima</Typography>
                <Typography variant="caption" color="text.secondary">Salin sesuai backend Anda, lalu simpan URL publiknya pada kolom webhook.</Typography>
              </Box>
              <ToggleButtonGroup size="small" exclusive value={webhookExample} onChange={(_, next) => next && setWebhookExample(next)}>
                <ToggleButton value="Node.js">Node.js</ToggleButton>
                <ToggleButton value="PHP">PHP</ToggleButton>
              </ToggleButtonGroup>
            </Stack>
            <CodeBlock value={webhookExample === 'Node.js' ? WEBHOOK_NODE_EXAMPLE : WEBHOOK_PHP_EXAMPLE} />
          </Paper>

          <Paper variant="outlined" sx={{ p: 2 }}>
            <Typography sx={{ fontWeight: 800, mb: 1.25 }}>Event yang dikirim</Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'repeat(3, minmax(0, 1fr))' }, gap: 1.5 }}>
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <CodeBlock label="message.received" value={JSON.stringify({ event: 'message.received', agent_id: 1, number: '628111111111', from: '6281234567890', name: 'Budi', type: 'text', text: 'Halo', media_type: '', message_id: '3EB0...', timestamp: 1783645200 }, null, 2)} />
              </Box>
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <CodeBlock label="image.analyzed" value={JSON.stringify({ event: 'image.analyzed', agent_id: 1, number: '628111111111', from: '6281234567890', message_id: '3EB0...', status: 'completed', analysis: 'Terlihat sofa abu-abu.', answer: 'Sofa abu-abu', product_id: 12, confidence: 0.91, needs_human: false, model: 'openai/gpt-4.1-mini', timestamp: 1783645230 }, null, 2)} />
              </Box>
              <Box sx={{ flex: 1, minWidth: 0 }}>
                <CodeBlock label="message.status" value={JSON.stringify({ event: 'message.status', agent_id: 1, number: '628111111111', to: '6281234567890', status: 'read', message_ids: ['3EB0...'], timestamp: 1783645260 }, null, 2)} />
              </Box>
            </Box>
          </Paper>

          <Alert severity="info" icon={<ShieldIcon fontSize="inherit" />}>
            Contoh endpoint di atas sudah memverifikasi header X-Signature. Balas HTTP 2xx setelah event diterima; SlaluDiskon mencoba ulang setelah 2 dan 5 detik bila endpoint gagal.
          </Alert>
        </Stack>
      )}
    </Box>
  );
}
