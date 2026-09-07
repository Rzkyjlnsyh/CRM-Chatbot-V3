import { useState } from 'react';
import {
  Alert, Box, Button, Chip, Grid, Paper, Stack, Table, TableBody,
  TableCell, TableContainer, TableHead, TableRow, TextField, Typography, useTheme,
} from '@mui/material';
import CampaignIcon from '@mui/icons-material/Campaign';
import PsychologyIcon from '@mui/icons-material/Psychology';
import RefreshIcon from '@mui/icons-material/Refresh';
import SaveIcon from '@mui/icons-material/Save';
import api from '../services/api';

interface Campaign {
  campaign_id: string;
  campaign_name: string;
  objective: string;
  spend: number;
  impressions: number;
  clicks: number;
  ctr: number;
  cpc: number;
  cpm: number;
  reach: number;
  frequency: number;
  results: number;
  cost_per_result: number;
  roas: number;
}

interface Summary {
  spend: number; impressions: number; clicks: number; results: number;
  campaigns: number; ctr: number; cost_per_result: number; roas: number;
  frequency: number; date_from: string; date_to: string; updated_at: string;
}

const fmtIDR = (v: number) => 'Rp' + Math.round(v).toLocaleString('id-ID');
const fmtN = (v: number, d = 2) => (v || 0).toFixed(d);

export default function MetaAdsPanel() {
  const theme = useTheme();
  const dark = theme.palette.mode === 'dark';

  const [config, setConfig] = useState<{ ad_account_id: string; has_token: boolean; target_roas: number } | null>(null);
  const [accId, setAccId] = useState('');
  const [token, setToken] = useState('');
  const [targetRoas, setTargetRoas] = useState('');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [data, setData] = useState<{ campaigns: Campaign[]; summary: Summary | null; history: any[] } | null>(null);
  const [analyzing, setAnalyzing] = useState(false);
  const [analysis, setAnalysis] = useState('');
  const [err, setErr] = useState('');
  const [toast, setToast] = useState('');

  const load = async () => {
    setLoading(true); setErr('');
    try {
      const c = (await api.get('/meta-ads/config')).data.data;
      setConfig(c);
      setAccId(c.ad_account_id || '');
      setTargetRoas(c.target_roas ? String(c.target_roas) : '');
      const d = (await api.get('/meta-ads/data')).data.data;
      setData(d);
    } catch (e: any) {
      setErr(e?.response?.data?.error || 'Gagal memuat data');
    } finally { setLoading(false); }
  };
  if (!config && !loading && !data) { void load(); }

  const save = async () => {
    setSaving(true); setErr(''); setToast('');
    try {
      await api.put('/meta-ads/config', {
        ad_account_id: accId.trim(), access_token: token.trim(),
        target_roas: parseFloat(targetRoas) || 0,
      });
      setToken('');
      setToast('Konfigurasi tersimpan ✓');
      await load();
    } catch (e: any) { setErr(e?.response?.data?.error || 'Gagal menyimpan'); }
    finally { setSaving(false); }
  };

  const refresh = async () => {
    setLoading(true); setErr(''); setToast('');
    try {
      const r = (await api.post('/meta-ads/refresh')).data.data;
      setToast(`Data diambil: ${r.campaigns} kampanye (${r.date_from} s.d. ${r.date_to}) ✓`);
      const d = (await api.get('/meta-ads/data')).data.data;
      setData(d); setAnalysis('');
    } catch (e: any) { setErr(e?.response?.data?.error || 'Gagal mengambil data Meta'); }
    finally { setLoading(false); }
  };

  const analyze = async () => {
    setAnalyzing(true); setErr(''); setAnalysis('');
    try {
      const r = (await api.post('/meta-ads/analyze')).data.data;
      setAnalysis(r.analysis);
    } catch (e: any) { setErr(e?.response?.data?.error || 'AI gagal menganalisis'); }
    finally { setAnalyzing(false); }
  };

  const s = data?.summary;
  const be = parseFloat(targetRoas) || 0;

  const card = (label: string, value: string, hint?: string, tone: 'ok' | 'bad' | 'neutral' = 'neutral') => (
    <Paper variant="outlined" sx={{ p: 1.5, textAlign: 'center' }}>
      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700 }}>{label}</Typography>
      <Typography variant="h6" sx={{ fontWeight: 800, color: tone === 'ok' ? '#2db86a' : tone === 'bad' ? '#ff6b6b' : 'text.primary' }}>
        {value}
      </Typography>
      {hint && <Typography variant="caption" color="text.secondary">{hint}</Typography>}
    </Paper>
  );

  return (
    <Stack spacing={2}>
      <Alert severity="info" sx={{ fontSize: 12.5 }}>
        <b>Hasil Iklan Meta — data nyata dari Marketing API</b> · Hubungkan Ad Account & access token (izin <code>ads_read</code>) ·
        klik <b>Ambil Data</b> untuk snapshot 30 hari. Analisis AI membaca data sebagai <b>advertiser Meta Ads profesional</b>, bukan perwakilan Meta.
      </Alert>

      {/* Konfigurasi */}
      <Paper variant="outlined" sx={{ p: 2 }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 800, mb: 1 }}>Koneksi Akun Iklan</Typography>
        <Grid container spacing={1.5}>
          <Grid size={{ xs: 12, sm: 5 }}>
            <TextField size="small" fullWidth label="Ad Account ID" value={accId} onChange={(e) => setAccId(e.target.value)}
              placeholder="act_1234567890" helperText={config?.has_token ? 'Token tersimpan ✓ (kosongkan untuk tidak mengubah)' : 'Token belum diset'} />
          </Grid>
          <Grid size={{ xs: 12, sm: 4 }}>
            <TextField size="small" fullWidth label="Access Token (ads_read)" value={token} onChange={(e) => setToken(e.target.value)}
              type="password" placeholder="EAAG..." helperText="Long-lived token 60 hari — Graph API Explorer" />
          </Grid>
          <Grid size={{ xs: 12, sm: 3 }}>
            <TextField size="small" fullWidth label="Target ROAS" value={targetRoas} onChange={(e) => setTargetRoas(e.target.value)}
              placeholder="cth: 3" helperText="Break-even bisnis Anda" />
          </Grid>
        </Grid>
        <Button size="small" variant="outlined" startIcon={<SaveIcon />} onClick={save} disabled={saving || !accId.trim()} sx={{ mt: 1 }}>
          {saving ? 'Menyimpan...' : 'Simpan Koneksi'}
        </Button>
        <Button size="small" variant="contained" startIcon={<RefreshIcon />} onClick={refresh} disabled={loading || !config?.has_token} sx={{ mt: 1, ml: 1 }}>
          {loading ? 'Mengambil...' : 'Ambil Data Meta'}
        </Button>
      </Paper>

      {err && <Alert severity="error" sx={{ fontSize: 12.5 }}>{err}</Alert>}
      {toast && <Alert severity="success" sx={{ fontSize: 12.5 }}>{toast}</Alert>}

      {/* Ringkasan */}
      {s && (
        <>
          <Grid container spacing={1.5}>
            <Grid size={{ xs: 6, sm: 3 }}>{card('Total Spend', fmtIDR(s.spend), `${s.date_from} s.d. ${s.date_to}`)}</Grid>
            <Grid size={{ xs: 6, sm: 3 }}>
              {card('ROAS (blended)', fmtN(s.roas) + 'x', be > 0 ? `break-even ${fmtN(be)}x` : 'set target ROAS', be > 0 ? (s.roas >= be ? 'ok' : 'bad') : 'neutral')}
            </Grid>
            <Grid size={{ xs: 6, sm: 2 }}>{card('Hasil', String(s.results || 0), s.cost_per_result ? `Rp${Math.round(s.cost_per_result).toLocaleString('id-ID')}/hasil` : '')}</Grid>
            <Grid size={{ xs: 6, sm: 2 }}>{card('CTR', fmtN(s.ctr) + '%', `${s.clicks} klik`, s.ctr >= 1 ? 'ok' : 'neutral')}</Grid>
            <Grid size={{ xs: 6, sm: 2 }}>{card('Frekuensi', fmtN(s.frequency), `${s.impressions.toLocaleString('id-ID')} tayangan`)}</Grid>
          </Grid>

          {/* Analisis */}
          <Paper variant="outlined" sx={{ p: 2 }}>
            <Stack direction="row" spacing={1.5} sx={{ mb: 1.5, alignItems: 'center' }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 800, flex: 1 }}>
                <PsychologyIcon sx={{ verticalAlign: 'middle', mr: 0.5, color: '#2db86a' }} />
                Analisis Expert Advertiser Meta Ads
              </Typography>
              <Button size="small" variant="contained" startIcon={<PsychologyIcon />} onClick={analyze} disabled={analyzing || !s}>
                {analyzing ? 'Menganalisis...' : 'Analisis Sekarang'}
              </Button>
            </Stack>
            {analysis ? (
              <Box sx={{ whiteSpace: 'pre-wrap', fontSize: 13, lineHeight: 1.7, fontFamily: 'inherit',
                bgcolor: dark ? 'rgba(10,15,14,.6)' : 'rgba(0,0,0,.03)', borderRadius: 1.5, p: 2, maxHeight: 560, overflow: 'auto' }}>
                {analysis}
              </Box>
            ) : (
              <Typography variant="body2" color="text.secondary">
                Data snapshot siap dianalisis. Klik <b>Analisis Sekarang</b> — AI membacanya dengan framework advertiser
                (CPA = CPM ÷ (CTR × CVR × 10), fatigue kreatif, frekuensi, learning phase, konsentrasi spend) dan memberi
                3 tindakan prioritas + diagnosa per kampanye.
              </Typography>
            )}
          </Paper>

          {/* Tabel kampanye */}
          <TableContainer component={Paper} variant="outlined">
            <Table size="small">
              <TableHead>
                <TableRow sx={{ '& th': { fontWeight: 800, fontSize: 12 } }}>
                  <TableCell>Kampanye</TableCell>
                  <TableCell align="right">Spend</TableCell>
                  <TableCell align="right">Hasil</TableCell>
                  <TableCell align="right">Cost/Hasil</TableCell>
                  <TableCell align="right">ROAS</TableCell>
                  <TableCell align="right">CTR</TableCell>
                  <TableCell align="right">CPC</TableCell>
                  <TableCell align="right">Frekuensi</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {data?.campaigns.map((c) => (
                  <TableRow key={c.campaign_id} hover>
                    <TableCell sx={{ maxWidth: 260, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {c.campaign_name || c.campaign_id}
                      <Box><Chip size="small" label={c.objective || '—'} sx={{ fontSize: 10, height: 18 }} /></Box>
                    </TableCell>
                    <TableCell align="right">{fmtIDR(c.spend)}</TableCell>
                    <TableCell align="right">{c.results || 0}</TableCell>
                    <TableCell align="right">{c.cost_per_result ? fmtIDR(c.cost_per_result) : '—'}</TableCell>
                    <TableCell align="right">
                      <Typography sx={{ fontWeight: 700, color: be > 0 ? (c.roas >= be ? '#2db86a' : '#ff6b6b') : 'text.primary' }}>
                        {c.roas > 0 ? fmtN(c.roas) + 'x' : '—'}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">{fmtN(c.ctr)}%</TableCell>
                    <TableCell align="right">{c.cpc ? fmtIDR(c.cpc) : '—'}</TableCell>
                    <TableCell align="right">{fmtN(c.frequency)}</TableCell>
                  </TableRow>
                ))}
                {(!data?.campaigns || data.campaigns.length === 0) && (
                  <TableRow><TableCell colSpan={8} sx={{ textAlign: 'center', color: 'text.secondary', py: 3 }}>
                    Belum ada data — klik "Ambil Data Meta"
                  </TableCell></TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>

          {/* Riwayat snapshot */}
          {data?.history && data.history.length > 1 && (
            <Paper variant="outlined" sx={{ p: 2 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 800, mb: 1 }}>Riwayat Snapshot (trend)</Typography>
              <TableContainer>
                <Table size="small">
                  <TableHead>
                    <TableRow sx={{ '& th': { fontWeight: 800, fontSize: 12 } }}>
                      <TableCell>Periode</TableCell><TableCell align="right">Spend</TableCell>
                      <TableCell align="right">Hasil</TableCell><TableCell align="right">Diambil</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {data.history.map((h) => (
                      <TableRow key={h.id}>
                        <TableCell>{h.date_from} s.d. {h.date_to}</TableCell>
                        <TableCell align="right">{fmtIDR(h.spend)}</TableCell>
                        <TableCell align="right">{h.results || 0}</TableCell>
                        <TableCell align="right">{new Date(h.created_at).toLocaleString('id-ID', { dateStyle: 'short', timeStyle: 'short' })}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            </Paper>
          )}
        </>
      )}
      {!s && !loading && (
        <Paper variant="outlined" sx={{ p: 4, textAlign: 'center', color: 'text.secondary' }}>
          <CampaignIcon sx={{ fontSize: 40, mb: 1 }} />
          <Typography variant="body2">Hubungkan akun iklan & ambil data untuk melihat performa + analisis expert.</Typography>
        </Paper>
      )}
    </Stack>
  );
}
