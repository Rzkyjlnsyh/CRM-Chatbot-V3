import { useMemo, useRef, useState } from 'react';
import {
  Box, Typography, Card, CardContent, Button, Stack, TextField, Alert,
  MenuItem, IconButton, CircularProgress,
} from '@mui/material';
import { QRCodeCanvas } from 'qrcode.react';
import WhatsAppIcon from '@mui/icons-material/WhatsApp';
import CopyIcon from '@mui/icons-material/ContentCopyOutlined';
import OpenInNewIcon from '@mui/icons-material/OpenInNewOutlined';
import LinkIcon from '@mui/icons-material/AddLinkOutlined';
import WidgetsIcon from '@mui/icons-material/WidgetsOutlined';
import ApiIcon from '@mui/icons-material/ApiOutlined';
import { useAgents } from '../hooks';
import PageHeader from './PageHeader';
import { swalToast } from '../services/swal';
import { normalizePhone } from '../types';

function copy(text: string) {
  navigator.clipboard?.writeText(text).then(() => swalToast('Disalin ke clipboard.'), () => {});
}

// Ikon WhatsApp (path FontAwesome) untuk snippet HTML mentah yang ditempel di situs tenant.
const waSvg = (fill: string) =>
  `<svg viewBox="0 0 448 512" width="34" height="34" fill="${fill}" xmlns="http://www.w3.org/2000/svg"><path d="M380.9 97.1C339 55.1 283.2 32 223.9 32c-122.4 0-222 99.6-222 222 0 39.1 10.2 77.3 29.6 111L0 480l117.7-30.9c32.4 17.7 68.9 27 106.1 27h.1c122.3 0 224.1-99.6 224.1-222 0-59.3-25.2-115-67.1-157zM223.9 438.6c-33.2 0-65.7-8.9-94-25.7l-6.7-4-69.8 18.3L72 359.2l-4.4-7c-18.5-29.4-28.2-63.3-28.2-98.2 0-101.7 82.8-184.5 184.6-184.5 49.3 0 95.6 19.2 130.4 54.1 34.8 34.9 56.2 81.2 56.1 130.5 0 101.8-84.9 184.6-186.6 184.6zm101.2-138.2c-5.5-2.8-32.8-16.2-37.9-18-5.1-1.9-8.8-2.8-12.5 2.8-3.7 5.6-14.3 18-17.6 21.8-3.2 3.7-6.5 4.2-12 1.4-32.6-16.3-54-29.1-75.5-66-5.7-9.8 5.7-9.1 16.3-30.3 1.8-3.7.9-6.9-.5-9.7-1.4-2.8-12.5-30.1-17.1-41.2-4.5-10.8-9.1-9.3-12.5-9.5-3.2-.2-6.9-.2-10.6-.2-3.7 0-9.7 1.4-14.8 6.9-5.1 5.6-19.4 19-19.4 46.3 0 27.3 19.9 53.7 22.6 57.4 2.8 3.7 39.1 59.7 94.8 83.7 35.2 15.2 49 16.5 66.6 13.9 10.7-1.6 32.8-13.4 37.4-26.4 4.6-13 4.6-24.1 3.2-26.4-1.3-2.3-5-3.7-10.5-6.5z"/></svg>`;

// Copyable = teks satu baris (link) dengan tombol salin.
function Copyable({ text }: { text: string }) {
  return (
    <Stack direction="row" spacing={1} sx={{ alignItems: 'center', bgcolor: 'action.hover', borderRadius: 1.5, px: 1.5, py: 0.75, minWidth: 0 }}>
      <Box component="code" sx={{ fontFamily: 'monospace', fontSize: 12.5, flex: 1, overflowX: 'auto', whiteSpace: 'nowrap' }}>{text}</Box>
      <IconButton size="small" onClick={() => copy(text)} aria-label="Salin"><CopyIcon fontSize="small" /></IconButton>
    </Stack>
  );
}

export default function WidgetPanel({ agentId }: { agentId: number }) {
  const { data: agents, isLoading } = useAgents();
  const number = normalizePhone(agents?.find(a => a.id === agentId)?.number || '');

  const [msg, setMsg] = useState('Halo, saya mau tanya produknya 😊');
  const [greeting, setGreeting] = useState('Halo! 👋 Ada yang bisa kami bantu?');
  const [pos, setPos] = useState<'right' | 'left'>('right');
  const [color, setColor] = useState('#25D366');
  const qrRef = useRef<HTMLCanvasElement>(null);

  const link = useMemo(() => {
    if (!number) return '';
    const base = `https://wa.me/${number}`;
    return msg.trim() ? `${base}?text=${encodeURIComponent(msg.trim())}` : base;
  }, [number, msg]);

  const buttonLink = useMemo(() => {
    if (!number) return '';
    const base = `https://wa.me/${number}`;
    return greeting.trim() ? `${base}?text=${encodeURIComponent(greeting.trim())}` : base;
  }, [number, greeting]);

  const snippet = useMemo(() => {
    if (!buttonLink) return '';
    return `<!-- Tombol WhatsApp by SlaluDiskon -->
<a href="${buttonLink}" target="_blank" rel="noopener" aria-label="Chat WhatsApp"
  style="position:fixed;${pos}:20px;bottom:20px;z-index:9999;width:60px;height:60px;border-radius:50%;background:${color};box-shadow:0 4px 14px rgba(0,0,0,.2);display:flex;align-items:center;justify-content:center;">
  ${waSvg('#fff')}
</a>`;
  }, [buttonLink, pos, color]);

  const downloadQR = () => {
    const url = qrRef.current?.toDataURL('image/png');
    if (!url) return;
    const a = document.createElement('a');
    a.href = url;
    a.download = 'qr-whatsapp.png';
    a.click();
  };

  if (isLoading) return <Box sx={{ py: 6, textAlign: 'center' }}><CircularProgress /></Box>;

  if (!number) {
    return (
      <Box sx={{ maxWidth: 840 }}>
        <PageHeader title="Chat Widget & Link" subtitle="Buat link & tombol WhatsApp untuk ditempel di website atau toko online kamu." />
        <Alert severity="info" icon={<WidgetsIcon fontSize="inherit" />}>
          Sambungkan nomor WhatsApp dulu untuk membuat link & widget.
        </Alert>
      </Box>
    );
  }

  return (
    <Box sx={{ maxWidth: 840 }}>
      <PageHeader title="Chat Widget & Link"
        subtitle="Datangkan lebih banyak chat: buat link wa.me + QR, dan tombol WhatsApp mengambang yang tinggal tempel di website kamu." />

      {/* ── Link chat + QR ── */}
      <Card variant="outlined" sx={{ mb: 2 }}>
        <CardContent>
          <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 1.5 }}>
            <LinkIcon color="primary" />
            <Typography sx={{ fontWeight: 700 }}>Link Chat (wa.me)</Typography>
          </Stack>
          <TextField fullWidth size="small" label="Pesan otomatis (opsional)" value={msg}
            onChange={e => setMsg(e.target.value)} sx={{ mb: 1.5 }}
            helperText="Teks yang otomatis terisi di kolom chat saat pelanggan membuka link." />
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} sx={{ alignItems: { md: 'center' } }}>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Copyable text={link} />
              <Button size="small" variant="outlined" startIcon={<OpenInNewIcon />} href={link} target="_blank" rel="noopener" sx={{ mt: 1 }}>
                Coba buka
              </Button>
            </Box>
            <Box sx={{ textAlign: 'center' }}>
              <Box sx={{ p: 1, bgcolor: '#fff', borderRadius: 1.5, display: 'inline-block', lineHeight: 0 }}>
                <QRCodeCanvas value={link} size={132} ref={qrRef} />
              </Box>
              <Button size="small" onClick={downloadQR} sx={{ display: 'block', mx: 'auto', mt: 0.5 }}>Unduh QR</Button>
            </Box>
          </Stack>
        </CardContent>
      </Card>

      {/* ── Tombol untuk website ── */}
      <Card variant="outlined">
        <CardContent>
          <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mb: 1.5 }}>
            <WhatsAppIcon sx={{ color: '#25D366' }} />
            <Typography sx={{ fontWeight: 700 }}>Tombol WhatsApp untuk Website</Typography>
          </Stack>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
            <Stack spacing={1.5} sx={{ flex: 1 }}>
              <TextField fullWidth size="small" label="Pesan otomatis" value={greeting} onChange={e => setGreeting(e.target.value)} />
              <Stack direction="row" spacing={1.5}>
                <TextField select size="small" label="Posisi" value={pos} onChange={e => setPos(e.target.value as 'right' | 'left')} sx={{ width: 140 }}>
                  <MenuItem value="right">Kanan bawah</MenuItem>
                  <MenuItem value="left">Kiri bawah</MenuItem>
                </TextField>
                <TextField type="color" size="small" label="Warna" value={color} onChange={e => setColor(e.target.value)} sx={{ width: 90 }} />
              </Stack>
            </Stack>
            {/* Pratinjau langsung */}
            <Box sx={{ position: 'relative', flex: 1, minHeight: 130, bgcolor: 'action.hover', borderRadius: 1.5, overflow: 'hidden' }}>
              <Typography variant="caption" color="text.secondary" sx={{ position: 'absolute', top: 8, left: 10 }}>Pratinjau</Typography>
              <Box component="a" href={buttonLink} target="_blank" rel="noopener" aria-label="Pratinjau tombol"
                sx={{ position: 'absolute', [pos]: 16, bottom: 16, width: 56, height: 56, borderRadius: '50%', bgcolor: color, display: 'flex', alignItems: 'center', justifyContent: 'center', boxShadow: 3 }}>
                <WhatsAppIcon sx={{ color: '#fff', fontSize: 30 }} />
              </Box>
            </Box>
          </Stack>

          <Typography variant="body2" sx={{ mt: 2, mb: 0.75 }}>
            Salin kode ini dan tempel sebelum <code>&lt;/body&gt;</code> di website kamu:
          </Typography>
          <Box sx={{ position: 'relative' }}>
            <Box component="pre" sx={{ m: 0, p: 1.5, pr: 5, bgcolor: 'action.hover', borderRadius: 1.5, overflowX: 'auto', fontSize: 12, lineHeight: 1.6 }}>
              {snippet}
            </Box>
            <IconButton size="small" onClick={() => copy(snippet)} aria-label="Salin kode" sx={{ position: 'absolute', top: 6, right: 6 }}>
              <CopyIcon fontSize="small" />
            </IconButton>
          </Box>
        </CardContent>
      </Card>
      {/* ── Integrasi lebih lanjut ── */}
      <Card variant="outlined" sx={{ mt: 2, borderColor: 'primary.light', bgcolor: 'rgba(25,118,210,0.04)' }}>
        <CardContent>
          <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
            <ApiIcon color="primary" fontSize="small" sx={{ mt: 0.2 }} />
            <Box>
              <Typography variant="subtitle2" sx={{ fontWeight: 700, mb: 0.25 }}>
                Butuh integrasi lebih dari sekadar link?
              </Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.6 }}>
                Buka menu <b>REST API</b> di sidebar untuk integrasi penuh: kirim/terima pesan, kelola kontak, cek nomor, OTP, broadcast, dan webhook realtime — bisa dipanggil dari website, backend, atau automation kamu. Tersedia contoh kode cURL, Node.js, PHP, dan Python.
              </Typography>
            </Box>
          </Stack>
        </CardContent>
      </Card>
    </Box>
  );
}
