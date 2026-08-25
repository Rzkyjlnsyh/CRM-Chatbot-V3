import { useState } from 'react';
import {
  Alert, Box, Button, CircularProgress, Paper, Stack,
  Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography,
} from '@mui/material';
import { useMetaConfig, useSaveMetaConfig, useTestMetaEvent } from '../hooks';
import PageHeader from './PageHeader';
import { swalToast } from '../services/swal';
import { apiErrorMessage } from '../services/errors';

// Meta CAPI (single-tenant) — konversi nyata dari label WhatsApp dilaporkan
// server-side ke Pixel Meta, lengkap dengan nilai transaksi (value-based ads).
export default function MetaCapiPanel({ agentId }: { agentId: number }) {
  const { data: cfg, isLoading, isError } = useMetaConfig(agentId);
  const saveMeta = useSaveMetaConfig(agentId);
  const testMeta = useTestMetaEvent(agentId);

  const [pixelId, setPixelId] = useState('');
  const [accessToken, setAccessToken] = useState('');
  const [testCode, setTestCode] = useState('');
  const [convLabels, setConvLabels] = useState('');
  const [eventName, setEventName] = useState('Purchase');
  const [labelEvents, setLabelEvents] = useState('');
  const [seeded, setSeeded] = useState(false);

  if (cfg && !seeded) {
    setPixelId(cfg.pixel_id || '');
    setTestCode(cfg.test_event_code || '');
    setConvLabels(cfg.conv_labels || '');
    setEventName(cfg.event_name || 'Purchase');
    const lines = Object.entries(cfg.label_events || {})
      .map(([k, v]) => `${k}=${v}`).join('\n');
    setLabelEvents(lines);
    setSeeded(true);
  }

  const parseLabelEvents = (raw: string): Record<string, string> => {
    const out: Record<string, string> = {};
    for (const line of raw.split('\n')) {
      const idx = line.indexOf('=');
      if (idx > 0) {
        const k = line.slice(0, idx).trim();
        const v = line.slice(idx + 1).trim();
        if (k && v) out[k] = v;
      }
    }
    return out;
  };

  const save = () => {
    if (!pixelId.trim()) {
      swalToast('Pixel ID wajib diisi', 'error');
      return;
    }
    saveMeta.mutate({
      pixel_id: pixelId.trim(),
      access_token: accessToken,
      test_event_code: testCode.trim(),
      conv_labels: convLabels,
      event_name: eventName.trim() || 'Purchase',
      label_events: parseLabelEvents(labelEvents),
    } as never, {
      onSuccess: () => {
        setAccessToken('');
        swalToast('Pengaturan Meta CAPI disimpan');
      },
      onError: (e) => swalToast(apiErrorMessage(e, 'Gagal menyimpan'), 'error'),
    });
  };

  const test = () => {
    testMeta.mutate(undefined, {
      onSuccess: () => swalToast('Event uji terkirim ke Meta'),
      onError: (e) => swalToast(apiErrorMessage(e, 'Event uji gagal'), 'error'),
    });
  };

  if (isLoading) return <CircularProgress />;
  if (isError) return <Alert severity="error">Gagal memuat konfigurasi Meta CAPI.</Alert>;

  const stats = cfg?.stats;
  return (
    <Box>
      <PageHeader
        title="Meta CAPI"
        subtitle="Laporkan konversi nyata ke Facebook Ads setiap label konversi menempel — server-side, lengkap dengan nilai transaksi."
      />
      <Stack spacing={2}>
        <Paper variant="outlined" sx={{ p: 3 }}>
          <Typography variant="h6" sx={{ mb: 2, fontWeight: 800 }}>⚙️ Konfigurasi</Typography>
          <Stack spacing={2}>
            <Alert severity="info">
              Cara mendapatkannya: Facebook Events Manager → Data Sources → pilih Pixel →
              buka tab <b>Settings</b> → salin <b>Pixel ID</b> dan <b>Access Token</b>.
              Untuk uji coba, isi <b>Test Event Code</b> agar event tidak mengganggu data iklan asli.
            </Alert>
            <TextField label="Pixel ID (Meta)" value={pixelId} onChange={(e) => setPixelId(e.target.value)} fullWidth />
            <TextField label="Access Token (API Key Meta)" type="password" value={accessToken}
              onChange={(e) => setAccessToken(e.target.value)} fullWidth
              placeholder={cfg?.configured ? '••• tersimpan — kosongkan untuk pakai yang lama' : ''} />
            <TextField label="Test Event Code (opsional, mode test)" value={testCode} onChange={(e) => setTestCode(e.target.value)} fullWidth />
            <TextField label="Label Konversi (dipisah koma)" value={convLabels}
              onChange={(e) => setConvLabels(e.target.value)} fullWidth
              helperText="Label WhatsApp yang dianggap konversi, misal: Transfer, Closing" />
            <TextField label="Event CAPI" value={eventName} onChange={(e) => setEventName(e.target.value)} fullWidth
              helperText="cth: Purchase, Lead, CompleteRegistration" />
            <TextField label="Pemetaan per label (label=Event, satu per baris)" value={labelEvents}
              onChange={(e) => setLabelEvents(e.target.value)} fullWidth multiline minRows={2}
              helperText="Opsional. Kosongkan untuk memakai Event CAPI default untuk semua label." />
            <Stack direction="row" spacing={2}>
              <Button variant="contained" onClick={save} disabled={saveMeta.isPending}>Simpan</Button>
              <Button variant="outlined" onClick={test} disabled={testMeta.isPending}>Tes Kirim</Button>
            </Stack>
          </Stack>
        </Paper>

        <Paper variant="outlined" sx={{ p: 3 }}>
          <Typography variant="h6" sx={{ mb: 2, fontWeight: 800 }}>📊 Statistik Pengiriman</Typography>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell>Menunggu</TableCell>
                <TableCell>Sukses</TableCell>
                <TableCell>Gagal</TableCell>
                <TableCell>Event terakhir</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              <TableRow>
                <TableCell>{stats?.pending ?? 0}</TableCell>
                <TableCell>{stats?.sent ?? 0}</TableCell>
                <TableCell>{stats?.failed ?? 0}</TableCell>
                <TableCell>{stats?.last_event ? `${stats.last_event.event_name} · ${stats.last_event.status}` : '—'}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
          <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
            Token disimpan terenkripsi. Nomor pelanggan di-hash sebelum dikirim ke Meta (SHA-256).
          </Typography>
        </Paper>
      </Stack>
    </Box>
  );
}
