import { useEffect, useState } from 'react';
import {
  Alert, Autocomplete, Box, Button, Card, CardContent, Chip, CircularProgress,
  Stack, Table, TableBody, TableCell, TableContainer, TableHead,
  TableRow, TextField, Typography,
} from '@mui/material';
import { useMetaConfig, useSaveMetaConfig, useTestMetaEvent } from '../hooks';
import PageHeader from './PageHeader';
import { swalToast } from '../services/swal';

// Meta CAPI — kirim konversi nyata ke Meta Ads saat label konversi menempel.
export default function MetaCapiPanel({ agentId }: { agentId: number }) {
  const { data: cfg, isLoading } = useMetaConfig(agentId);
  const saveMeta = useSaveMetaConfig(agentId);
  const testMeta = useTestMetaEvent(agentId);

  const [pixelId, setPixelId] = useState('');
  const [accessToken, setAccessToken] = useState('');
  const [testCode, setTestCode] = useState('');
  const [convLabels, setConvLabels] = useState('');
  const [eventName, setEventName] = useState('Purchase');
  const [labelEvents, setLabelEvents] = useState<Record<string, string>>({});
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    if (!cfg || loaded) return;
    setPixelId(cfg.pixel_id || '');
    setTestCode(cfg.test_event_code || '');
    setConvLabels(cfg.conv_labels || '');
    setEventName(cfg.event_name || 'Purchase');
    setLabelEvents(cfg.label_events || {});
    setLoaded(true);
  }, [cfg, loaded]);

  const save = () => {
    if (!pixelId.trim()) {
      swalToast('Pixel ID wajib diisi', 'error');
      return;
    }
    saveMeta.mutate({
      pixel_id: pixelId.trim(),
      access_token: accessToken, // kosong = pertahankan token lama
      test_event_code: testCode.trim(),
      conv_labels: convLabels.trim(),
      event_name: eventName.trim() || 'Purchase',
      label_events: labelEvents,
    }, {
      onSuccess: () => { swalToast('Konfigurasi Meta CAPI tersimpan!'); setAccessToken(''); },
      onError: (err: any) => swalToast(err?.response?.data?.error || 'Gagal menyimpan', 'error'),
    });
  };

  const labels = cfg?.available_labels || [];
  const convLabelIds = convLabels.split(',').map(s => s.trim()).filter(Boolean);
  const standardEvents = cfg?.standard_events || ['Purchase', 'Lead', 'Contact', 'CompleteRegistration', 'Schedule', 'SubmitApplication'];

  return (
    <Box>
      <PageHeader title="Meta CAPI" subtitle="Deteksi konversi nyata dari Meta Ads otomatis via label WhatsApp" />

      <Card variant="outlined" sx={{ mb: 2 }}>
        <CardContent>
          <Alert severity="info" sx={{ mb: 2 }}>
            Cara kerja: isi Pixel ID + Access Token dari Meta Events Manager, lalu pilih label konversi.
            Setiap kontak yang diberi label tersebut otomatis dikirim sebagai konversi ke Meta Ads.
          </Alert>

          <Stack spacing={2}>
            <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
              <TextField label="Pixel ID (Meta)" value={pixelId} size="small" sx={{ flex: 1, minWidth: 220 }}
                onChange={e => setPixelId(e.target.value)}
                helperText="Dari Events Manager → Data Sources" />
              <TextField label="Event Name" value={eventName} size="small" sx={{ flex: 1, minWidth: 180 }}
                onChange={e => setEventName(e.target.value)}
                helperText="cth: Purchase, Lead, CompleteRegistration" />
            </Box>
            <TextField label="Access Token (API Key Meta)" type="password" value={accessToken} size="small"
              onChange={e => setAccessToken(e.target.value)}
              helperText={cfg?.configured ? '✓ Token sudah tersimpan — isi hanya untuk mengganti.' : 'Dari Events Manager → Settings → Generate Access Token'} />
            <TextField label="Test Event Code (opsional, mode test)" value={testCode} size="small"
              onChange={e => setTestCode(e.target.value)} />
            <TextField label="Label Konversi (dipisah koma)" value={convLabels} size="small"
              onChange={e => setConvLabels(e.target.value)}
              helperText="Masukkan ID label (atau klik label di bawah). Contoh: 1,2,3" />

            {labels.length > 0 && (
              <Box>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                  Klik untuk menambah label:
                </Typography>
                <Stack direction="row" spacing={0.5} sx={{ flexWrap: 'wrap', gap: 0.5 }}>
                  {labels.map((l: any) => (
                    <Chip key={l.label_id} size="small" label={`${l.name} (${l.label_id})`}
                      onClick={() => {
                        const cur = convLabels.split(',').map(s => s.trim()).filter(Boolean);
                        if (!cur.includes(String(l.label_id))) {
                          setConvLabels([...cur, String(l.label_id)].join(', '));
                        }
                      }} />
                  ))}
                </Stack>
              </Box>
            )}

            {convLabelIds.length > 0 && (
              <Box>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.5 }}>
                  Skema pelabelan — pilih event CAPI standar, atau ketik nama event kamu sendiri (dikirim sebagai indikator ke Meta):
                </Typography>
                <Stack spacing={1}>
                  {labels.filter((l: any) => convLabelIds.includes(String(l.label_id))).map((l: any) => (
                    <Box key={l.label_id} sx={{ display: 'flex', alignItems: 'center', gap: 1.5, flexWrap: 'wrap' }}>
                      <Typography variant="body2" sx={{ minWidth: 140, fontWeight: 500 }}>{l.name}</Typography>
                      <Autocomplete
                        size="small"
                        freeSolo
                        options={standardEvents}
                        value={labelEvents[String(l.label_id)] || eventName}
                        onInputChange={(_, v) => setLabelEvents(prev => ({ ...prev, [String(l.label_id)]: v || '' }))}
                        renderInput={(params) => <TextField {...params} label="Event CAPI" />}
                        sx={{ minWidth: 220 }}
                      />
                    </Box>
                  ))}
                </Stack>
              </Box>
            )}

            <Stack direction="row" spacing={1}>
              <Button variant="contained" onClick={save} disabled={saveMeta.isPending || isLoading}>
                {saveMeta.isPending ? <CircularProgress size={16} /> : 'Simpan'}
              </Button>
              {cfg?.configured && (
                <Button variant="outlined" onClick={() => testMeta.mutate()} disabled={testMeta.isPending}>
                  Tes Kirim Event
                </Button>
              )}
            </Stack>
          </Stack>
        </CardContent>
      </Card>

      <Card variant="outlined">
        <CardContent>
          <Typography variant="subtitle1" sx={{ fontWeight: 800, mb: 1 }}>Log Event (20 terakhir)</Typography>
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell>Waktu</TableCell>
                  <TableCell>Kontak</TableCell>
                  <TableCell>Label</TableCell>
                  <TableCell>Event</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Respons</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {(cfg?.recent_events || []).map((e: any) => (
                  <TableRow key={e.id}>
                    <TableCell>{e.sent_at ? new Date(e.sent_at).toLocaleString('id-ID') : '—'}</TableCell>
                    <TableCell>{e.sender}</TableCell>
                    <TableCell>{e.label_id}</TableCell>
                    <TableCell>{e.event_name}</TableCell>
                    <TableCell>
                      <Chip size="small" color={e.status === 'sent' ? 'success' : 'error'}
                        label={e.status === 'sent' ? 'terkirim' : 'gagal'} />
                    </TableCell>
                    <TableCell sx={{ maxWidth: 220, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {e.response}
                    </TableCell>
                  </TableRow>
                ))}
                {(!cfg?.recent_events || cfg.recent_events.length === 0) && (
                  <TableRow><TableCell colSpan={6} sx={{ color: 'text.secondary' }}>Belum ada event.</TableCell></TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </CardContent>
      </Card>
    </Box>
  );
}
