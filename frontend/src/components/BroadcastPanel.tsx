import { useMemo, useState, useRef, type ReactNode } from 'react';
import {
  Box, Typography, Card, CardContent, TextField, Button, Stack, Alert, Chip,
  Table, TableBody, TableCell, TableHead, TableRow, CircularProgress,
  Dialog, DialogTitle, DialogContent, DialogActions, Divider, useMediaQuery,
  Checkbox, Accordion, AccordionSummary, AccordionDetails, MenuItem,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import SendIcon from '@mui/icons-material/Send';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import CloseIcon from '@mui/icons-material/Close';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import EditNoteIcon from '@mui/icons-material/EditNoteOutlined';
import PeopleAltIcon from '@mui/icons-material/PeopleAltOutlined';
import ScheduleIcon from '@mui/icons-material/ScheduleOutlined';
import HistoryIcon from '@mui/icons-material/HistoryOutlined';
import InfoIcon from '@mui/icons-material/InfoOutlined';
import RefreshIcon from '@mui/icons-material/Refresh';
import FactCheckIcon from '@mui/icons-material/FactCheckOutlined';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { useCreateBroadcast, useBroadcasts, useBroadcastDetail, useCancelBroadcast, useResumeBroadcast, useCheckNumbers, useAgents, useAgentStatuses, useTestBroadcastRotation, useProducts, type BroadcastRotationTestResult } from '../hooks';
import SyncAltIcon from '@mui/icons-material/SyncAltOutlined';
import { swalToast, swalConfirm } from '../services/swal';
import RecipientField from './RecipientField';
import WhatsAppEditor from './WhatsAppEditor';
import TemplatePicker from './TemplatePicker';
import PageHeader from './PageHeader';
import DelayFields from './broadcast/DelayFields';
import BroadcastProgress from './broadcast/BroadcastProgress';
import BroadcastQuarantineSummary from './broadcast/BroadcastQuarantineSummary';
import { defaultBroadcastSafetyForm } from '../services/broadcastSafety';
import type { Broadcast, Product } from '../types';

function normalizePhone(s: string): string {
  const d = (s.match(/\d/g) || []).join('');
  if (!d) return '';
  if (d.startsWith('0')) return '62' + d.slice(1);
  if (d.startsWith('8')) return '62' + d;
  return d;
}

// spinPreview meniru parser spin backend: {a|b|c} dipilih acak (dukung bersarang),
// {nama} dan kurung tanpa '|' dibiarkan. Untuk pratinjau contoh hasil di form.
function spinPreview(input: string): string {
  let s = input.replaceAll('{nama}', '\u0000nama\u0001');
  for (let i = 0; i < 200; i++) {
    const open = s.lastIndexOf('{');
    if (open < 0) break;
    const close = s.indexOf('}', open);
    if (close < 0) { s = s.slice(0, open) + '\u0000' + s.slice(open + 1); continue; }
    const body = s.slice(open + 1, close);
    if (!body.includes('|')) { s = s.slice(0, open) + '\u0000' + body + '\u0001' + s.slice(close + 1); continue; }
    const opts = body.split('|');
    s = s.slice(0, open) + opts[Math.floor(Math.random() * opts.length)] + s.slice(close + 1);
  }
  return s.replaceAll('\u0000', '{').replaceAll('\u0001', '}');
}

const HAS_SPIN = /\{[^{}]*\|/;

function productCaption(product: Product) {
  return [
    `*${product.name}*`,
    product.price,
    product.description,
  ].filter(Boolean).join('\n\n');
}

const STATUS_COLOR: Record<string, 'success' | 'warning' | 'error' | 'default'> = {
  done: 'success', running: 'warning', pending: 'default', failed: 'error', interrupted: 'error',
  resuming: 'warning', wa_restricted: 'warning', cancel_requested: 'warning', cancelled: 'default',
};
const STATUS_LABEL: Record<string, string> = {
  done: 'Selesai', running: 'Berjalan', pending: 'Antre', failed: 'Gagal', interrupted: 'Terhenti',
  resuming: 'Mencoba lanjut', wa_restricted: 'Dijeda WhatsApp', cancel_requested: 'Membatalkan', cancelled: 'Dibatalkan',
};
const RCP_COLOR: Record<string, 'success' | 'warning' | 'error' | 'default'> = {
  sent: 'success', failed: 'error', skipped: 'default', pending: 'warning',
};
const RCP_LABEL: Record<string, string> = {
  sent: 'Terkirim', failed: 'Gagal', skipped: 'Dilewati', pending: 'Menunggu',
};
function SectionTitle({ icon, title, subtitle, action }: { icon: ReactNode; title: string; subtitle?: string; action?: ReactNode }) {
  return (
    <Stack direction="row" sx={{ alignItems: 'center', gap: 1, mb: 1 }}>
      <Box sx={{
        width: 30, height: 30, display: 'grid', placeItems: 'center', borderRadius: 1,
        bgcolor: 'action.hover', color: 'primary.main', flexShrink: 0,
      }}>
        {icon}
      </Box>
      <Box sx={{ minWidth: 0, flex: 1 }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>{title}</Typography>
        {subtitle && <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>{subtitle}</Typography>}
      </Box>
      {action}
    </Stack>
  );
}

function ReviewRow({ label, value, good, warning }: { label: string; value: string; good?: boolean; warning?: boolean }) {
  return (
    <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
      <Typography variant="body2" color="text.secondary">{label}</Typography>
      <Chip
        size="small"
        label={value}
        color={warning ? 'warning' : good ? 'success' : 'default'}
        variant={good || warning ? 'filled' : 'outlined'}
      />
    </Stack>
  );
}

export default function BroadcastPanel({ agentId, seed }: { agentId: number; seed?: { value: string; n: number } | null }) {
  const theme = useTheme();
  const mobileDetail = useMediaQuery(theme.breakpoints.down('sm'));
  const [message, setMessage] = useState('');
  // Panel di-mount ulang ketika tab dibuka, jadi seed dari Kontak cukup dijadikan nilai awal.
  const [recipientsText, setRecipientsText] = useState(seed?.value || '');
  const [minDelay, setMinDelay] = useState(10);
  const [maxDelay, setMaxDelay] = useState(30);
  // Istirahat berkala (default pintar): jeda restDuration dtk tiap restEvery pesan. restEvery=0 = mati.
  const [restEvery, setRestEvery] = useState(25);
  const [restDuration, setRestDuration] = useState(90);
  const [file, setFile] = useState<File | null>(null);
  const [selectedProductId, setSelectedProductId] = useState<number | ''>('');
  const { data: products = [] } = useProducts(agentId);
  // Rotasi nomor: nomor tambahan yang ikut mengirim broadcast ini (selain nomor aktif).
  const { data: agents = [] } = useAgents();
  const { data: statusMap = {} } = useAgentStatuses();
  const [rotationIds, setRotationIds] = useState<number[]>([]);
  const [rotationTestResult, setRotationTestResult] = useState<BroadcastRotationTestResult | null>(null);
  const currentAgent = agents.find(a => a.id === agentId);
  const rotationCandidates = agents.filter(a => a.id !== agentId && statusMap[a.id] === 'connected');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [page, setPage] = useState(1);
  const [lastStartedId, setLastStartedId] = useState<number | null>(null);

  const historyRef = useRef<HTMLDivElement | null>(null);

  const [detailId, setDetailId] = useState<number | null>(null);
  const [detailFilter, setDetailFilter] = useState<'all' | 'sent' | 'failed' | 'skipped' | 'pending'>('all');
  const [detailSearch, setDetailSearch] = useState('');
  const closeDetail = () => { setDetailId(null); setDetailFilter('all'); setDetailSearch(''); };

  const createBroadcast = useCreateBroadcast(agentId);
  const cancelBroadcast = useCancelBroadcast(agentId);
  const resumeBroadcast = useResumeBroadcast(agentId);
  const checkNumbersM = useCheckNumbers(agentId);
  const testRotationM = useTestBroadcastRotation(agentId);
  // Pratinjau spin: nonce menaikkan versi agar tombol "Acak lagi" menghasilkan varian baru.
  const [spinNonce, setSpinNonce] = useState(0);
  const [spinHelpOpen, setSpinHelpOpen] = useState(false);
  const { data: bpage } = useBroadcasts(agentId, page);
  const { data: detail } = useBroadcastDetail(agentId, detailId);
  const broadcasts = bpage?.data || [];
  const totalPages = Math.max(1, Math.ceil((bpage?.total || 0) / (bpage?.limit || 10)));

  const parsed = recipientsText.split('\n').map(l => l.trim()).filter(Boolean).map(line => {
    const parts = line.split(/[\t,]/);
    const num = parts.find(p => /\d/.test(p)) || parts[0];
    const name = parts.filter(p => p !== num && p.trim()).join(' ').trim();
    return { number: normalizePhone(num), name };
  }).filter(r => r.number);

  const uniqueParsedCount = new Set(parsed.map(p => p.number)).size;
  const duplicateCount = parsed.length - uniqueParsedCount;
  // eslint-disable-next-line react-hooks/exhaustive-deps -- spinNonce sengaja jadi pemicu acak ulang
  const spinSample = useMemo(() => (HAS_SPIN.test(message) ? spinPreview(message) : ''), [message, spinNonce]);
  const hasLooseSpinText = message.includes('|') && !HAS_SPIN.test(message);
  const delayProblem = minDelay < 1 || maxDelay < 1
    ? 'Jeda harus minimal 1 detik'
    : maxDelay < minDelay
      ? 'Jeda maksimal harus lebih besar atau sama dengan jeda minimal'
      : '';

  const doSend = async () => {
    const e: Record<string, string> = {};
    if (!message.trim()) e.message = 'Pesan tidak boleh kosong';
    if (parsed.length === 0) e.recipients = 'Masukkan minimal satu nomor';
    if (delayProblem) e.delay = delayProblem;
    setErrors(e);
    if (Object.keys(e).length > 0) return;

    const recipients = parsed.map(p => ({ number: p.number, name: p.name }));
    const ok = await swalConfirm(
      `Mulai Blast ke ${uniqueParsedCount} nomor?`,
      'Pesan dikirim langsung ke daftar penerima dengan jeda yang kamu atur. Kontak yang pernah membalas STOP otomatis dilewati.',
    );
    if (!ok) return;
    try {
      const res = await createBroadcast.mutateAsync({
        message, recipients, min_delay: minDelay, max_delay: maxDelay, rest_every: restEvery, rest_duration: restDuration, file,
        agent_ids: rotationIds.length ? [agentId, ...rotationIds] : undefined,
        product_id: selectedProductId || undefined,
        safety: defaultBroadcastSafetyForm(),
      });
      const started = res.data;

      // Bersihkan form agar user tahu Blast sudah masuk proses
      setMessage('');
      setRecipientsText('');
      setFile(null);
      setSelectedProductId('');
      setRotationIds([]);
      setRotationTestResult(null);
      setErrors({});

      // Balik ke halaman 1 — Blast terbaru di paling atas
      setPage(1);
      setLastStartedId(started?.id || null);

      // Scroll ke riwayat
      setTimeout(() => {
        historyRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
      }, 100);

      swalToast(`Blast dimulai untuk ${recipients.length} penerima.`);
    } catch (error) {
      const detail = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(detail || 'Blast belum bisa dimulai. Periksa pesan, penerima, dan koneksi WhatsApp.', 'error');
    }
  };

  const doCheckNumbers = async () => {
    const nums = Array.from(new Set(parsed.map(p => p.number)));
    try {
      const res = await checkNumbersM.mutateAsync(nums);
      const checkedCount = res.total;
      const notRegistered = res.not_registered || [];
      if (checkedCount === 0) {
        swalToast('Tidak ada nomor telepon yang bisa dicek.', 'warning');
        return;
      }
      if (notRegistered.length === 0) {
        swalToast(`Semua ${checkedCount} nomor terdaftar di WhatsApp.`);
        return;
      }
      const ok = await swalConfirm(
        `${notRegistered.length} dari ${checkedCount} nomor tidak terdaftar di WhatsApp`,
        'Hapus nomor tak terdaftar dari daftar penerima? Mengirim ke nomor yang tidak ada membuang kuota dan menambah risiko banned.',
      );
      if (!ok) return;
      const badSet = new Set(notRegistered);
      setRecipientsText(recipientsText.split('\n').filter(line => {
        const t = line.trim();
        if (!t) return false;
        const parts = t.split(/[\t,]/);
        const num = parts.find(p => /\d/.test(p)) || parts[0];
        const norm = normalizePhone(num);
        return !norm || !badSet.has(norm);
      }).join('\n'));
      swalToast(`${notRegistered.length} nomor dihapus dari daftar.`);
    } catch (e: any) {
      const detail = e?.response?.data?.error;
      swalToast(detail || 'Gagal cek nomor. Pastikan WhatsApp tersambung.', 'error');
    }
  };

  const testRotation = async () => {
    try {
      const result = await testRotationM.mutateAsync([agentId, ...rotationIds]);
      setRotationTestResult(result);
      swalToast('Simulasi failover berhasil. Tidak ada pesan yang dikirim.');
    } catch (error) {
      setRotationTestResult(null);
      const detail = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(detail || 'Simulasi failover belum berhasil.', 'error');
    }
  };

  const resume = async (broadcast: Broadcast) => {
    const ok = await swalConfirm(
      'Tetap lanjutkan Blast?',
      'WhatsApp kemungkinan besar masih akan menolak pengiriman ini. Melanjutkan tidak menembus pembatasan WhatsApp, jadi pesan bisa tetap gagal terkirim. Sebaiknya istirahatkan nomor dulu beberapa hari, lalu mulai dari kontak yang pernah membalas Anda.',
    );
    if (!ok) return;
    try {
      await resumeBroadcast.mutateAsync(broadcast.id);
      swalToast('Mencoba melanjutkan Blast.');
    } catch (error) {
      const detail = (error as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(detail || 'Blast belum bisa dilanjutkan.', 'error');
    }
  };

  const formIssueCount = (message.trim() ? 0 : 1) + (parsed.length ? 0 : 1) + (delayProblem ? 1 : 0);

  const cancel = async (broadcast: Broadcast) => {
    if (!await swalConfirm('Batalkan Blast ini?', 'Pesan yang sudah terkirim tidak bisa ditarik.')) return;
    if (broadcast.id === lastStartedId) setLastStartedId(null);
    cancelBroadcast.mutate(broadcast.id);
  };

  const broadcastActions = (broadcast: Broadcast, fullWidth = false) => {
    const remaining = Math.max(0, broadcast.total - broadcast.sent - broadcast.failed - broadcast.skipped);
    const canCancel = ['pending', 'running', 'resuming', 'interrupted', 'wa_restricted', 'cancel_requested'].includes(broadcast.status);
    if (!canCancel) return null;
    return (
      <Stack direction={{ xs: fullWidth ? 'column' : 'row', sm: 'row' }} spacing={0.75} sx={{ justifyContent: 'flex-end' }}>
        {broadcast.status === 'wa_restricted' && (
          <Button size="small" variant="contained" fullWidth={fullWidth}
            disabled={resumeBroadcast.isPending} onClick={() => resume(broadcast)}>
            Coba lanjutkan ({remaining})
          </Button>
        )}
        {canCancel && (
          <Button size="small" color="error" variant="outlined" fullWidth={fullWidth}
            disabled={cancelBroadcast.isPending} onClick={() => cancel(broadcast)}>
            Batalkan
          </Button>
        )}
      </Stack>
    );
  };

  return (
    <Box>
      <PageHeader title="WhatsApp Blast"
        subtitle="Susun pesan, pilih penerima, lalu pantau pengirimannya secara langsung." />

      <Alert
        severity="info"
        icon={<InfoIcon fontSize="small" />}
        sx={{ mb: 2 }}
      >
        <Typography variant="body2">
          Kirim ke kontak yang relevan. Hindari mengirim ke banyak nomor asing sekaligus agar nomor WhatsApp tetap aman. Kontak yang membalas STOP otomatis dilewati.
        </Typography>
      </Alert>

      <Box sx={{
        display: 'grid',
        gridTemplateColumns: { xs: '1fr', lg: 'minmax(0, 1.55fr) minmax(300px, 0.85fr)' },
        gap: 2,
        alignItems: 'start',
      }}>
        <Card>
          <CardContent>
            <Stack spacing={2.25}>
              <Box>
                <SectionTitle
                  icon={<EditNoteIcon fontSize="small" />}
                  title="Pesan"
                  subtitle={`${message.length}/2000 karakter`}
                />
                <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ alignItems: { xs: 'flex-start', sm: 'center' }, justifyContent: 'space-between', gap: 0.5, mb: 0.75 }}>
                  <Typography variant="caption" color="text.secondary">Isi kolom pesan dari pesan yang sudah disimpan.</Typography>
                  <TemplatePicker label="Pakai template pesan" agentId={agentId} onPick={b => { setMessage(m => m ? m + '\n' + b : b); if (errors.message) setErrors(p => ({ ...p, message: '' })); }} />
                </Stack>
                <TextField
                  select
                  fullWidth
                  size="small"
                  label="Broadcast produk"
                  value={selectedProductId}
                  onChange={e => {
                    const value = e.target.value ? Number(e.target.value) : '';
                    setSelectedProductId(value);
                    setFile(null);
                    const product = products.find(p => p.id === value);
                    if (product) setMessage(productCaption(product));
                    if (errors.message) setErrors(p => ({ ...p, message: '' }));
                  }}
                  sx={{ mb: 1 }}
                  helperText="Opsional. Jika dipilih, gambar produk ikut dikirim dan pesan bisa tetap diedit sebelum blast."
                >
                  <MenuItem value="">Tidak pakai produk</MenuItem>
                  {products.map(product => (
                    <MenuItem key={product.id} value={product.id}>
                      {product.name}{product.price ? ` - ${product.price}` : ''}
                    </MenuItem>
                  ))}
                </TextField>
                <WhatsAppEditor value={message} onChange={v => { setMessage(v); if (errors.message) setErrors(p => ({ ...p, message: '' })); }}
                  placeholder="Halo {nama}, ada promo spesial untuk kamu hari ini…" error={!!errors.message} helperText={errors.message} />
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center', mt: 1, flexWrap: 'wrap', gap: 0.75 }}>
                  <Button component="label" size="small" variant="outlined" startIcon={<AttachFileIcon />} disabled={!!selectedProductId}>
                    {file ? 'Ganti lampiran' : 'Lampirkan file'}
                    <input type="file" hidden onChange={e => setFile(e.target.files?.[0] || null)} />
                  </Button>
                  {file && <Chip label={file.name} size="small" onDelete={() => setFile(null)} deleteIcon={<CloseIcon />} />}
                  {selectedProductId && <Chip size="small" color="primary" variant="outlined" label="Gambar + tombol produk ikut dikirim" />}
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                  Lampiran mendukung gambar, video, PDF, dan dokumen. Jika produk dipilih, lampiran memakai gambar produk.
                </Typography>

                <Accordion disableGutters elevation={0} expanded={spinHelpOpen || hasLooseSpinText}
                  onChange={(_, expanded) => setSpinHelpOpen(expanded)}
                  sx={{ mt: 1.25, border: '1px solid', borderColor: hasLooseSpinText ? 'warning.light' : 'divider', borderRadius: '8px !important', '&:before': { display: 'none' }, overflow: 'hidden' }}>
                  <AccordionSummary expandIcon={<ExpandMoreIcon />} sx={{ minHeight: 48, bgcolor: 'action.hover', '& .MuiAccordionSummary-content': { my: 0.75 } }}>
                    <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1, width: '100%', pr: 1 }}>
                      <Box>
                        <Typography variant="body2" sx={{ fontWeight: 700 }}>Variasi pesan (opsional)</Typography>
                        <Typography variant="caption" color="text.secondary">Buat sapaan berbeda tanpa memenuhi editor.</Typography>
                      </Box>
                      <Chip size="small" variant="outlined"
                        color={hasLooseSpinText ? 'warning' : spinSample ? 'success' : 'default'}
                        label={hasLooseSpinText ? 'Perlu diperbaiki' : spinSample ? 'Aktif' : 'Belum digunakan'} />
                    </Stack>
                  </AccordionSummary>
                  <AccordionDetails sx={{ p: 1.25 }}>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                      Gunakan <code>{'{Halo|Hai|Selamat pagi}'}</code> untuk memilih satu variasi secara acak. Gunakan <code>{'{nama}'}</code> untuk nama penerima.
                    </Typography>
                    <Box sx={{ p: 1, bgcolor: 'action.hover', borderRadius: 1, overflowWrap: 'anywhere' }}>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.25 }}>Contoh singkat</Typography>
                      <code>{'{Halo|Hai} {nama}, {ada promo baru|kami punya kabar menarik} untuk kamu.'}</code>
                    </Box>
                    {hasLooseSpinText && (
                      <Alert severity="warning" sx={{ mt: 1 }}>
                        Tanda | ditemukan tetapi belum dibungkus kurung kurawal. Gunakan <code>{'{Halo|Hai|Pagi}'}</code>.
                      </Alert>
                    )}
                    {spinSample && (
                      <Alert severity="info" sx={{ mt: 1 }} action={
                        <Button size="small" startIcon={<RefreshIcon />} onClick={() => setSpinNonce(n => n + 1)}>Acak lagi</Button>
                      }>
                        <Typography variant="caption" sx={{ fontWeight: 700, display: 'block' }}>Pratinjau satu penerima</Typography>
                        <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{spinSample}</Typography>
                      </Alert>
                    )}
                  </AccordionDetails>
                </Accordion>
              </Box>

              <Divider />

              <Box>
                <SectionTitle
                  icon={<PeopleAltIcon fontSize="small" />}
                  title="Penerima"
                  subtitle={`${uniqueParsedCount} nomor unik${duplicateCount > 0 ? `, ${duplicateCount} duplikat terdeteksi` : ''}`}
                  action={
                    <Button size="small" variant="outlined"
                      startIcon={checkNumbersM.isPending ? <CircularProgress size={14} /> : <FactCheckIcon />}
                      disabled={checkNumbersM.isPending || parsed.length === 0}
                      onClick={doCheckNumbers}>
                      {checkNumbersM.isPending ? 'Mengecek…' : 'Cek Nomor'}
                    </Button>
                  }
                />
                <RecipientField agentId={agentId} value={recipientsText} onChange={v => { setRecipientsText(v); if (errors.recipients) setErrors(p => ({ ...p, recipients: '' })); }} error={errors.recipients} />
              </Box>

              <Divider />
              <Box>
                <SectionTitle
                  icon={<SyncAltIcon fontSize="small" />}
                  title="Rotasi & cadangan nomor"
                  subtitle="Bagi beban kirim dan teruskan sisa antrean jika salah satu nomor terputus atau dibatasi WhatsApp."
                />
                <Alert severity="info" sx={{ mb: 1 }}>
                  <b>Failover cerdas.</b> Jika satu nomor dibatasi, rate-limit, putus, atau tidak stabil, antrean yang belum terkirim dialihkan ke nomor sehat dengan beban paling ringan (sticky per kontak tetap dijaga). Nomor berisiko dikarantina sementara agar tidak dipaksa terus. Pesan yang sudah terkirim tidak dikirim ulang.
                </Alert>
                <Box sx={{ p: 1, mb: 1, border: '1px solid', borderColor: 'primary.light', bgcolor: 'rgba(31,138,80,0.04)', borderRadius: 1.5 }}>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>Nomor utama</Typography>
                  <Typography variant="body2" sx={{ fontWeight: 700 }}>
                    {currentAgent?.name || `Nomor ${agentId}`}{currentAgent?.number ? ` · +${currentAgent.number}` : ''}
                  </Typography>
                </Box>
                {rotationCandidates.length > 0 ? (
                  <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(2, minmax(0, 1fr))' }, gap: 0.75 }}>
                    {rotationCandidates.map(a => {
                      const selected = rotationIds.includes(a.id);
                      return (
                        <Box component="label" key={a.id} sx={{ display: 'flex', alignItems: 'center', gap: 0.75, p: 0.75, border: '1px solid', borderColor: selected ? 'primary.main' : 'divider', bgcolor: selected ? 'rgba(31,138,80,0.06)' : 'background.paper', borderRadius: 1.5, cursor: 'pointer' }}>
                          <Checkbox size="small" checked={selected}
                            onChange={e => {
                              setRotationIds(prev => e.target.checked ? [...prev, a.id] : prev.filter(x => x !== a.id));
                              setRotationTestResult(null);
                            }} />
                          <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2" sx={{ fontWeight: 700 }} noWrap>{a.name || `Nomor ${a.id}`}</Typography>
                            <Typography variant="caption" color="text.secondary" noWrap>{a.number ? `+${a.number}` : 'Tersambung'}</Typography>
                          </Box>
                        </Box>
                      );
                    })}
                  </Box>
                ) : (
                  <Alert severity="info">Belum ada nomor tambahan yang tersambung. Hubungkan nomor lain terlebih dahulu untuk memakai failover.</Alert>
                )}
                {rotationIds.length > 0 && (
                  <Stack spacing={1} sx={{ mt: 1 }}>
                    <Alert severity="success">
                      Failover aktif dengan {rotationIds.length + 1} nomor: health-check, karantina ber-cooldown, circuit-breaker, dan load-balancing antrean. Jika semua nomor bermasalah, Blast dijeda WhatsApp agar aman dilanjutkan manual.
                    </Alert>
                    <Button variant="outlined" startIcon={testRotationM.isPending ? <CircularProgress size={15} /> : <FactCheckIcon />}
                      disabled={testRotationM.isPending} onClick={() => { void testRotation(); }} sx={{ alignSelf: 'flex-start' }}>
                      {testRotationM.isPending ? 'Menguji failover…' : 'Uji failover tanpa kirim pesan'}
                    </Button>
                    {rotationTestResult && (
                      <Box sx={{ p: 1.25, border: '1px solid', borderColor: 'success.light', bgcolor: 'rgba(31,138,80,0.04)', borderRadius: 1.5 }}>
                        <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ justifyContent: 'space-between', gap: 0.5, mb: 0.75 }}>
                          <Typography variant="body2" sx={{ fontWeight: 800 }}>Simulasi berhasil</Typography>
                          <Chip size="small" color="success" variant="outlined" label={`${rotationTestResult.messages_sent} pesan dikirim`} />
                        </Stack>
                        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                          Dari {rotationTestResult.sample_size} penerima contoh, {rotationTestResult.reassigned} antrean milik nomor utama berhasil dialihkan ke nomor cadangan.
                        </Typography>
                        <Alert severity="success" icon={false} sx={{ mb: 1 }}>
                          Berhasil: tidak ada antrean yang tertinggal di nomor utama yang gagal. Seluruhnya sudah diambil alih nomor cadangan.
                        </Alert>
                        <Stack spacing={0.75}>
                          {rotationTestResult.agents.map(agent => (
                            <Stack key={agent.id} direction={{ xs: 'column', sm: 'row' }} sx={{ justifyContent: 'space-between', alignItems: { sm: 'center' }, gap: 0.25, p: 0.75, bgcolor: 'background.paper', borderRadius: 1 }}>
                              <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                                <Typography variant="caption" sx={{ fontWeight: 700 }}>{agent.name || `Nomor ${agent.id}`}</Typography>
                                <Chip size="small" color={agent.simulated_failed ? 'error' : 'success'} variant="outlined"
                                  label={agent.simulated_failed ? 'Disimulasikan gagal' : 'Cadangan aktif'} />
                              </Stack>
                              <Typography variant="caption" color="text.secondary">
                                Sebelum gangguan: <b>{agent.initial_count}</b> · Setelah dialihkan: <b>{agent.after_failover_count}</b>
                              </Typography>
                            </Stack>
                          ))}
                        </Stack>
                      </Box>
                    )}
                  </Stack>
                )}
              </Box>

              <Divider />

              <Box>
                <SectionTitle
                  icon={<ScheduleIcon fontSize="small" />}
                  title="Jeda Kirim"
                  subtitle="Atur ritme pengiriman agar lebih aman."
                />

                <DelayFields
                  minDelay={minDelay} maxDelay={maxDelay} restEvery={restEvery} restDuration={restDuration}
                  setMinDelay={setMinDelay} setMaxDelay={setMaxDelay} setRestEvery={setRestEvery} setRestDuration={setRestDuration}
                  error={errors.delay || delayProblem || undefined}
                  onEditDelay={() => { if (errors.delay) setErrors(p => ({ ...p, delay: '' })); }}
                />
              </Box>
            </Stack>
          </CardContent>
        </Card>

        <Card sx={{ position: { lg: 'sticky' }, top: 16 }}>
          <CardContent>
            <SectionTitle
              icon={<CheckCircleIcon fontSize="small" />}
              title="Review"
              subtitle="Ringkasan sebelum mengirim."
            />
            <Stack spacing={1.1}>
              <ReviewRow label="Pesan" value={message.trim() ? 'Siap' : 'Kosong'} good={!!message.trim()} warning={!message.trim()} />
              <ReviewRow label="Penerima" value={parsed.length ? `${uniqueParsedCount} nomor` : 'Belum ada'} good={parsed.length > 0} warning={parsed.length === 0} />
              <ReviewRow label="Produk" value={selectedProductId ? (products.find(p => p.id === selectedProductId)?.name || 'Dipilih') : 'Tidak pakai'} good={!!selectedProductId} />
              <ReviewRow label="Lampiran" value={selectedProductId ? 'Gambar dan tombol produk' : file ? 'Ada' : 'Tidak ada'} good={!!file || !!selectedProductId} />
              {rotationIds.length > 0 && <ReviewRow label="Rotasi nomor" value={`${rotationIds.length + 1} nomor`} good />}
              <ReviewRow label="Jeda antar pesan" value={`${minDelay}-${maxDelay} detik`} good={!delayProblem} warning={!!delayProblem} />
              <ReviewRow label="Jeda istirahat" value={restEvery > 0 ? `berhenti ${restDuration} dtk tiap ${restEvery} pesan` : 'Mati'} good={restEvery > 0} />
              {message.length > 700 && <Alert severity="warning" icon={false}>Pesan cukup panjang. Pertimbangkan dipersingkat agar lebih mudah dibaca.</Alert>}
              {duplicateCount > 0 && <Alert severity="info" icon={false}>{duplicateCount} nomor duplikat terdeteksi. Sistem akan menggabungkan saat daftar diproses.</Alert>}
              {formIssueCount > 0 && (
                <Alert severity="warning" icon={false}>
                  Lengkapi pesan, penerima, dan jeda sebelum mengirim.
                </Alert>
              )}
              <Button fullWidth variant="contained"
                startIcon={createBroadcast.isPending ? <CircularProgress size={16} /> : <SendIcon />}
                onClick={doSend} disabled={createBroadcast.isPending || formIssueCount > 0}>
                {createBroadcast.isPending ? 'Memulai Blast…' : `Mulai Blast (${uniqueParsedCount})`}
              </Button>
              <Typography variant="caption" color="text.secondary">
                Pesan dikirim langsung ke daftar penerima dengan jeda yang kamu atur.
              </Typography>
            </Stack>
          </CardContent>
        </Card>
      </Box>

      <Card ref={historyRef} sx={{ mt: 2 }}>
        <CardContent>
          <SectionTitle
            icon={<HistoryIcon fontSize="small" />}
            title="Riwayat Blast"
            subtitle={broadcasts.length ? 'Progres diperbarui otomatis. Buka item untuk melihat setiap penerima.' : 'Blast yang sudah dimulai akan muncul di sini.'}
          />
          {broadcasts.length === 0 ? (
            <Alert severity="info" icon={false}>Belum ada Blast yang tercatat untuk nomor ini.</Alert>
          ) : (
            <>
              <Box sx={{ display: { xs: 'none', md: 'block' }, overflowX: 'auto' }}>
                <Table size="small" sx={{ minWidth: 820 }}>
                  <TableHead>
                    <TableRow>
                      <TableCell sx={{ width: 145 }}>Waktu</TableCell>
                      <TableCell>Pesan</TableCell>
                      <TableCell align="center" sx={{ width: 130 }}>Status</TableCell>
                      <TableCell sx={{ width: 280 }}>Progres</TableCell>
                      <TableCell align="right" sx={{ width: 230 }}>Aksi</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {broadcasts.map(b => (
                      <TableRow key={b.id} hover sx={{ cursor: 'pointer', bgcolor: b.id === lastStartedId && b.status === 'pending' ? 'action.hover' : 'inherit' }} onClick={() => setDetailId(b.id)}>
                        <TableCell>
                          <Typography variant="caption" sx={{ display: 'block', fontWeight: 700 }}>
                            {new Date(b.created_at).toLocaleDateString('id-ID', { day: '2-digit', month: 'short' })}
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            {new Date(b.created_at).toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' })}
                          </Typography>
                          {b.id === lastStartedId && b.status === 'pending' && <Chip label="Baru dibuat" size="small" color="primary" sx={{ mt: 0.5 }} />}
                        </TableCell>
                        <TableCell sx={{ maxWidth: 260 }}>
                          <Typography variant="body2" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: 600 }}>{b.message}</Typography>
                          <Typography variant="caption" color="text.secondary">{b.total} penerima</Typography>
                        </TableCell>
                        <TableCell align="center">
                          <Chip label={STATUS_LABEL[b.status] ?? b.status} size="small" color={STATUS_COLOR[b.status] ?? 'default'} />
                        </TableCell>
                        <TableCell><BroadcastProgress broadcast={b} /></TableCell>
                        <TableCell align="right" onClick={e => e.stopPropagation()}>{broadcastActions(b)}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Box>

              <Stack spacing={1} sx={{ display: { xs: 'flex', md: 'none' } }}>
                {broadcasts.map(b => (
                  <Box key={b.id} role="button" tabIndex={0} onClick={() => setDetailId(b.id)}
                    onKeyDown={e => { if (e.key === 'Enter' || e.key === ' ') setDetailId(b.id); }}
                    sx={{ p: 1.25, border: '1px solid', borderColor: 'divider', borderRadius: 1, cursor: 'pointer', bgcolor: b.id === lastStartedId && b.status === 'pending' ? 'action.hover' : 'background.paper' }}>
                    <Stack direction="row" sx={{ alignItems: 'flex-start', justifyContent: 'space-between', gap: 1, mb: 0.75 }}>
                      <Box sx={{ minWidth: 0 }}>
                        <Typography variant="caption" color="text.secondary">
                          {new Date(b.created_at).toLocaleString('id-ID', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })}
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 0.2, fontWeight: 700, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{b.message}</Typography>
                      </Box>
                      <Chip label={STATUS_LABEL[b.status] ?? b.status} size="small" color={STATUS_COLOR[b.status] ?? 'default'} sx={{ flexShrink: 0 }} />
                    </Stack>
                    {b.id === lastStartedId && b.status === 'pending' && <Chip label="Baru dibuat" size="small" color="primary" sx={{ mb: 0.75 }} />}
                    <BroadcastProgress broadcast={b} />
                    {broadcastActions(b, true) && (
                      <Box onClick={e => e.stopPropagation()} sx={{ mt: 1 }}>{broadcastActions(b, true)}</Box>
                    )}
                  </Box>
                ))}
              </Stack>

            <Stack direction="row" sx={{ justifyContent: 'flex-end', alignItems: 'center', mt: 1, gap: 1 }}>
              <Button size="small" disabled={page <= 1} onClick={() => setPage(p => p - 1)}>Sebelumnya</Button>
              <Typography variant="caption">Hal {page} / {totalPages}</Typography>
              <Button size="small" disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Berikutnya</Button>
            </Stack>
            </>
          )}
        </CardContent>
      </Card>

      {/* Detail Blast: status per penerima */}
      <Dialog open={!!detailId} onClose={closeDetail} fullWidth maxWidth="md" fullScreen={mobileDetail}>
        <DialogTitle>Detail Blast</DialogTitle>
        <DialogContent dividers>
          {detail ? (() => {
            const recs = detail.recipients;
            const q = detailSearch.replace(/\D/g, '');
            const shown = recs.filter(r =>
              (detailFilter === 'all' || r.status === detailFilter) && (!q || r.number.includes(q)));
            const FILTERS = [
              { k: 'all' as const, label: `Semua ${recs.length}` },
              { k: 'sent' as const, label: `Terkirim ${detail.broadcast.sent}` },
              { k: 'failed' as const, label: `Gagal ${detail.broadcast.failed}` },
              { k: 'skipped' as const, label: `Dilewati ${detail.broadcast.skipped}` },
              { k: 'pending' as const, label: `Menunggu ${Math.max(0, detail.broadcast.total - detail.broadcast.sent - detail.broadcast.failed - detail.broadcast.skipped)}` },
            ];
            return (
              <>
                <Box sx={{ p: 1.25, mb: 1.5, bgcolor: 'action.hover', borderRadius: 1 }}>
                  <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center', gap: 1, mb: 1 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>Progres pengiriman</Typography>
                    <Chip label={STATUS_LABEL[detail.broadcast.status] ?? detail.broadcast.status} size="small" color={STATUS_COLOR[detail.broadcast.status] ?? 'default'} />
                  </Stack>
                  <BroadcastProgress broadcast={detail.broadcast} />
                </Box>
                <BroadcastQuarantineSummary rotation={detail.rotation} />
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'minmax(260px, 0.8fr) minmax(0, 1.2fr)' }, gap: 1.5, alignItems: 'start', mt: 1.5 }}>
                  <Box sx={{ minWidth: 0, p: 1.25, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>Konten Blast</Typography>
                    <Typography variant="caption" color="text.secondary">Pesan yang dikirim ke penerima.</Typography>
                    <Divider sx={{ my: 1 }} />
                    <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{detail.broadcast.message}</Typography>
                    {detail.broadcast.media_type && (
                      <Chip size="small" icon={<AttachFileIcon />} label={detail.broadcast.file_name || detail.broadcast.media_type} sx={{ mt: 1.25 }} />
                    )}
                    {detail.broadcast.risk_level === 'high' && (
                      <Alert severity="warning" icon={false} sx={{ mt: 1.25 }}>
                        <Typography variant="body2" sx={{ fontWeight: 700 }}>Blast ini perlu perhatian</Typography>
                        {detail.broadcast.override_reason && <Typography variant="caption">Alasan pengguna: {detail.broadcast.override_reason}</Typography>}
                      </Alert>
                    )}
                    {detail.broadcast.status === 'wa_restricted' && (() => {
                      const waiting = Math.max(0, detail.broadcast.total - detail.broadcast.sent - detail.broadcast.failed - detail.broadcast.skipped);
                      const at = detail.broadcast.paused_at
                        ? new Date(detail.broadcast.paused_at).toLocaleString('id-ID', { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' })
                        : null;
                      return (
                        <Alert severity="warning" icon={false} sx={{ mt: 1.25 }}>
                          <Typography variant="body2" sx={{ fontWeight: 800, mb: 0.5 }}>Dijeda oleh WhatsApp</Typography>
                          <Typography variant="body2" sx={{ mb: 0.5 }}>
                            Pengiriman Blast ini dihentikan sementara oleh WhatsApp, bukan oleh SlaluDiskon. Saat SlaluDiskon mengirim pesan Anda, WhatsApp menolaknya dan meminta pengiriman dihentikan, lalu SlaluDiskon langsung menjeda Blast agar nomor Anda tetap aman. Nomor Anda tidak terblokir permanen.
                          </Typography>
                          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
                            {at ? `Keputusan ini diberikan langsung oleh WhatsApp pada ${at}. ` : ''}SlaluDiskon tidak memblokir pesan Anda.
                          </Typography>
                          <Typography variant="body2" sx={{ fontWeight: 700 }}>Kenapa ini terjadi?</Typography>
                          <Box component="ul" sx={{ pl: 2.5, m: 0, mb: 1 }}>
                            <li><Typography variant="caption">Mengirim ke nomor yang belum pernah membalas atau menyimpan kontak Anda.</Typography></li>
                            <li><Typography variant="caption">Nomor WhatsApp Anda masih baru atau jarang dipakai mengobrol dua arah.</Typography></li>
                            <li><Typography variant="caption">Terlalu banyak pesan dikirim dalam waktu berdekatan.</Typography></li>
                          </Box>
                          <Typography variant="body2" sx={{ fontWeight: 700 }}>Agar nomor bisa melakukan Blast lagi:</Typography>
                          <Box component="ol" sx={{ pl: 2.5, m: 0, mb: 1 }}>
                            <li><Typography variant="caption">Istirahatkan nomor ini dulu, 1 sampai 3 hari.</Typography></li>
                            <li><Typography variant="caption">Hangatkan nomor: pakai untuk mengobrol normal, minta beberapa orang mengirim pesan ke Anda lalu balas.</Typography></li>
                            <li><Typography variant="caption">Saat mulai lagi, kirim dulu hanya ke kontak yang pernah membalas Anda, dalam jumlah kecil.</Typography></li>
                            <li><Typography variant="caption">Naikkan jumlah penerima sedikit demi sedikit setiap hari.</Typography></li>
                          </Box>
                          <Typography variant="caption" color="text.secondary">{waiting} penerima masih menunggu.</Typography>
                        </Alert>
                      );
                    })()}
                  </Box>

                  <Box sx={{ minWidth: 0, p: 1.25, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>Daftar Penerima</Typography>
                    <Typography variant="caption" color="text.secondary">Cari atau filter berdasarkan hasil pengiriman.</Typography>
                    <Stack direction="row" spacing={0.75} sx={{ my: 1, flexWrap: 'wrap', gap: 0.75 }}>
                      {FILTERS.map(f => (
                        <Chip key={f.k} size="small" label={f.label} onClick={() => setDetailFilter(f.k)}
                          color={detailFilter === f.k ? 'primary' : 'default'} variant={detailFilter === f.k ? 'filled' : 'outlined'} />
                      ))}
                    </Stack>
                    <TextField size="small" fullWidth placeholder="Cari nomor…" value={detailSearch}
                      onChange={e => setDetailSearch(e.target.value)} sx={{ mb: 1 }} />
                    <Box sx={{ display: { xs: 'none', sm: 'block' }, maxHeight: { sm: 420, md: 'min(48vh, 460px)' }, overflowY: 'auto', border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                      <Table size="small" stickyHeader>
                        <TableHead>
                          <TableRow>
                            <TableCell>Nomor</TableCell>
                            <TableCell>Nama</TableCell>
                            {detail.rotation?.enabled && <TableCell>CS</TableCell>}
                            <TableCell align="right">Status</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {shown.map(r => {
                            const cs = detail.rotation?.agents?.find(a => a.id === r.agent_id);
                            return (
                              <TableRow key={r.id}>
                                <TableCell>+{r.number}</TableCell>
                                <TableCell>{r.name || '-'}</TableCell>
                                {detail.rotation?.enabled && (
                                  <TableCell>
                                    <Typography variant="caption" color="text.secondary" noWrap>
                                      {cs?.name || (r.agent_id ? `#${r.agent_id}` : '—')}
                                    </Typography>
                                  </TableCell>
                                )}
                                <TableCell align="right">
                                  <Chip size="small" label={RCP_LABEL[r.status] ?? r.status} color={RCP_COLOR[r.status] ?? 'default'} />
                                  {r.error && <Typography variant="caption" color={r.status === 'pending' ? 'warning.main' : 'error'} sx={{ display: 'block' }}>{r.error}</Typography>}
                                </TableCell>
                              </TableRow>
                            );
                          })}
                        </TableBody>
                      </Table>
                    </Box>
                    <Stack spacing={0.75} sx={{ display: { xs: 'flex', sm: 'none' }, maxHeight: '44svh', overflowY: 'auto', pr: 0.25 }}>
                      {shown.map(r => {
                        const cs = detail.rotation?.agents?.find(a => a.id === r.agent_id);
                        return (
                          <Stack key={r.id} direction="row" sx={{ alignItems: 'flex-start', justifyContent: 'space-between', gap: 1, p: 1, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                            <Box sx={{ minWidth: 0 }}>
                              <Typography variant="body2" sx={{ fontWeight: 700 }}>+{r.number}</Typography>
                              <Typography variant="caption" color="text.secondary">{r.name || 'Tanpa nama'}</Typography>
                              {detail.rotation?.enabled && (
                                <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                                  CS: {cs?.name || (r.agent_id ? `#${r.agent_id}` : '—')}
                                </Typography>
                              )}
                              {r.error && <Typography variant="caption" color={r.status === 'pending' ? 'warning.main' : 'error'} sx={{ display: 'block' }}>{r.error}</Typography>}
                            </Box>
                            <Chip size="small" label={RCP_LABEL[r.status] ?? r.status} color={RCP_COLOR[r.status] ?? 'default'} sx={{ flexShrink: 0 }} />
                          </Stack>
                        );
                      })}
                    </Stack>
                    {shown.length === 0 && <Typography variant="body2" color="text.secondary" sx={{ py: 3, textAlign: 'center' }}>Tidak ada penerima yang cocok.</Typography>}
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                      Menampilkan {shown.length} dari {recs.length} penerima
                    </Typography>
                  </Box>
                </Box>
              </>
            );
          })() : (
            <Box sx={{ textAlign: 'center', py: 3 }}><CircularProgress /></Box>
          )}
        </DialogContent>
        <DialogActions>
          {detail?.broadcast.status === 'wa_restricted' ? (
            <>
              <Button color="inherit" disabled={resumeBroadcast.isPending} onClick={() => resume(detail.broadcast)}>
                Saya mengerti, tetap lanjutkan
              </Button>
              <Button variant="contained" onClick={closeDetail}>Istirahatkan dulu</Button>
            </>
          ) : (
            <Button onClick={closeDetail}>Tutup</Button>
          )}
        </DialogActions>
      </Dialog>
    </Box>
  );
}
