import { useEffect, useMemo, useState } from 'react';
import {
  Box, Typography, Card, CardContent, Button, Stack, TextField, MenuItem,
  IconButton, Switch, FormControlLabel, Alert, Chip, Divider, CircularProgress,
  Accordion, AccordionSummary, AccordionDetails,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/DeleteOutlined';
import AccountTreeIcon from '@mui/icons-material/AccountTreeOutlined';
import SaveIcon from '@mui/icons-material/SaveOutlined';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import VisibilityIcon from '@mui/icons-material/VisibilityOutlined';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesomeOutlined';
import { useFlow, useSaveFlow } from '../hooks';
import PageHeader from './PageHeader';
import { swalConfirm, swalToast } from '../services/swal';
import type { FlowNode, FlowOption } from '../types';

const ROOT = 'start';
const genId = () => 'n' + Math.random().toString(36).slice(2, 7);

const DEFAULT_NODES: Record<string, FlowNode> = {
  [ROOT]: {
    message: 'Halo 👋 Ada yang bisa kami bantu?',
    options: [
      { key: '1', label: 'Lihat produk', action: 'reply', reply: 'Ini katalog produk kami: (isi link/daftar produk)' },
      { key: '2', label: 'Cek ongkir', action: 'reply', reply: 'Silakan kirim kota tujuan, nanti kami bantu cek ongkirnya ya.' },
      { key: '3', label: 'Bicara dengan CS', action: 'handoff', reply: 'Baik, percakapan diteruskan ke CS.' },
    ],
  },
};

const SERVICE_NODES: Record<string, FlowNode> = {
  [ROOT]: {
    message: 'Halo 👋 Selamat datang. Silakan pilih bantuan yang dibutuhkan.',
    options: [
      { key: '1', label: 'Lihat layanan', action: 'goto', target: 'services' },
      { key: '2', label: 'Buat jadwal', action: 'reply', reply: 'Tuliskan layanan, tanggal, dan waktu yang diinginkan.' },
      { key: '3', label: 'Bicara dengan CS', action: 'handoff', reply: 'Baik, percakapan diteruskan ke CS.' },
    ],
  },
  services: {
    message: 'Berikut layanan yang tersedia. Sesuaikan daftar ini dengan bisnis Anda.',
    options: [
      { key: '1', label: 'Layanan utama', action: 'reply', reply: 'Jelaskan layanan utama di sini.' },
      { key: '2', label: 'Tanya harga', action: 'reply', reply: 'Tuliskan layanan yang ingin ditanyakan agar kami bantu cek harganya.' },
    ],
  },
};

const ACTIONS = [
  { value: 'reply', label: 'Kirim jawaban', description: 'Jawaban dikirim. Pelanggan tetap bisa mengetik kode lain selama 30 menit.' },
  { value: 'reply_menu', label: 'Jawab lalu ulangi menu', description: 'Jawaban dikirim, kemudian pilihan ini ditampilkan lagi.' },
  { value: 'goto', label: 'Tampilkan pilihan lanjutan', description: 'Sistem otomatis membuat halaman submenu berikutnya.' },
  { value: 'handoff', label: 'Teruskan ke CS', description: 'Masuk ke Butuh CS dan alur dijeda khusus untuk kontak ini sampai penanganan selesai.' },
];

const DELAY_OPTIONS = [
  { value: '0-0', label: 'Tanpa jeda', detail: 'Langsung membalas' },
  { value: '1-2', label: 'Cepat · 1–2 detik', detail: 'Cocok untuk menu singkat' },
  { value: '2-4', label: 'Natural · 2–4 detik', detail: 'Disarankan' },
  { value: '4-7', label: 'Santai · 4–7 detik', detail: 'Terasa lebih manusiawi' },
  { value: '8-12', label: 'Lambat · 8–12 detik', detail: 'Gunakan seperlunya' },
];

function cloneNodes(nodes: Record<string, FlowNode>) {
  return JSON.parse(JSON.stringify(nodes)) as Record<string, FlowNode>;
}

function previewUsesButtons(mode: 'auto' | 'text' | 'buttons', node: FlowNode) {
  return mode !== 'text' && node.options.length > 0 && node.options.length <= 3
    && node.options.every(option => option.label?.trim() && Array.from(option.label.trim()).length <= 24);
}

function renderedPreview(id: string, node: FlowNode, mode: 'auto' | 'text' | 'buttons') {
  const choices = node.options
    .filter(option => option.label?.trim())
    .map(option => `${option.key}. ${option.label?.trim()}`);
  const hint = id === ROOT ? 'Ketik keluar untuk menutup menu.' : 'Ketik 0 untuk kembali · keluar untuk menutup menu.';
  return [node.message.trim(), previewUsesButtons(mode, node) ? '' : choices.join('\n'), hint].filter(Boolean).join('\n\n');
}

function nodeLabel(id: string, node: FlowNode) {
  if (id === ROOT) return 'Menu Utama';
  const first = (node.message || '').split('\n')[0].trim();
  return first ? (first.length > 28 ? first.slice(0, 28) + '…' : first) : 'Menu tanpa judul';
}

export default function FlowPanel({ agentId }: { agentId: number }) {
  const { data: flow, isLoading } = useFlow(agentId);
  const saveFlow = useSaveFlow(agentId);

  const [enabled, setEnabled] = useState(false);
  const [trigger, setTrigger] = useState('menu');
  const [matchType, setMatchType] = useState('exact');
  const [displayMode, setDisplayMode] = useState<'auto' | 'text' | 'buttons'>('auto');
  const [delayMin, setDelayMin] = useState(2);
  const [delayMax, setDelayMax] = useState(4);
  const [nodes, setNodes] = useState<Record<string, FlowNode>>(() => cloneNodes(DEFAULT_NODES));
  const [loaded, setLoaded] = useState(false);
  const [baseline, setBaseline] = useState('');
  const [previewNode, setPreviewNode] = useState(ROOT);
  const [expandedNode, setExpandedNode] = useState(ROOT);

  const configKey = useMemo(() => JSON.stringify({ enabled, trigger: trigger.trim(), matchType, displayMode, delayMin, delayMax, nodes }), [enabled, trigger, matchType, displayMode, delayMin, delayMax, nodes]);
  const hasChanges = loaded && baseline !== configKey;

  useEffect(() => {
    setLoaded(false);
    setBaseline('');
    setEnabled(false);
    setTrigger('menu');
    setMatchType('exact');
    setDisplayMode('auto');
    setDelayMin(2);
    setDelayMax(4);
    setNodes(cloneNodes(DEFAULT_NODES));
    setPreviewNode(ROOT);
    setExpandedNode(ROOT);
  }, [agentId]);

  // Inisialisasi sekali dari server (jangan menimpa editan user setelahnya).
  useEffect(() => {
    if (loaded || !flow) return;
    const nextEnabled = !!flow.enabled;
    const nextTrigger = flow.trigger || 'menu';
    const nextMatchType = flow.match_type || 'exact';
    const nextDisplayMode = flow.display_mode || 'auto';
    const nextDelayMin = Number.isFinite(flow.delay_min) ? flow.delay_min : 2;
    const nextDelayMax = Number.isFinite(flow.delay_max) ? flow.delay_max : 4;
    let nextNodes = cloneNodes(DEFAULT_NODES);
    if (flow.structure && flow.structure.trim()) {
      try {
        const parsed = JSON.parse(flow.structure);
        if (parsed?.nodes && Object.keys(parsed.nodes).length) nextNodes = parsed.nodes;
      } catch { /* struktur rusak -> pakai default */ }
    }
    const serverKey = JSON.stringify({ enabled: nextEnabled, trigger: nextTrigger.trim(), matchType: nextMatchType, displayMode: nextDisplayMode, delayMin: nextDelayMin, delayMax: nextDelayMax, nodes: nextNodes });
    try {
      const draft = JSON.parse(localStorage.getItem(`wai_flow_draft_${agentId}`) || 'null');
      if (draft?.nodes?.[ROOT]) {
        setEnabled(!!draft.enabled);
        setTrigger(draft.trigger || 'menu');
        setMatchType(draft.matchType || 'exact');
        setDisplayMode(draft.displayMode || 'auto');
        setDelayMin(Number.isFinite(draft.delayMin) ? draft.delayMin : 2);
        setDelayMax(Number.isFinite(draft.delayMax) ? draft.delayMax : 4);
        setNodes(draft.nodes);
        setPreviewNode(draft.nodes[previewNode] ? previewNode : ROOT);
      } else {
        setEnabled(nextEnabled);
        setTrigger(nextTrigger);
        setMatchType(nextMatchType);
        setDisplayMode(nextDisplayMode);
        setDelayMin(nextDelayMin);
        setDelayMax(nextDelayMax);
        setNodes(nextNodes);
      }
    } catch {
      localStorage.removeItem(`wai_flow_draft_${agentId}`);
      setEnabled(nextEnabled);
      setTrigger(nextTrigger);
      setMatchType(nextMatchType);
      setDisplayMode(nextDisplayMode);
      setDelayMin(nextDelayMin);
      setDelayMax(nextDelayMax);
      setNodes(nextNodes);
    }
    setBaseline(serverKey);
    setLoaded(true);
  }, [flow, loaded, agentId, previewNode]);

  useEffect(() => {
    if (!loaded) return;
    const draftKey = `wai_flow_draft_${agentId}`;
    if (hasChanges) {
      localStorage.setItem(draftKey, JSON.stringify({ enabled, trigger, matchType, displayMode, delayMin, delayMax, nodes }));
    } else {
      localStorage.removeItem(draftKey);
    }
  }, [agentId, enabled, trigger, matchType, displayMode, delayMin, delayMax, nodes, loaded, hasChanges]);

  const patchNode = (id: string, patch: Partial<FlowNode>) =>
    setNodes(prev => ({ ...prev, [id]: { ...prev[id], ...patch } }));

  const patchOption = (id: string, idx: number, patch: Partial<FlowOption>) =>
    setNodes(prev => {
      const opts = prev[id].options.map((o, i) => (i === idx ? { ...o, ...patch } : o));
      return { ...prev, [id]: { ...prev[id], options: opts } };
    });

  const addOption = (id: string) =>
    setNodes(prev => ({
      ...prev,
      [id]: {
        ...prev[id],
        options: [...prev[id].options, { key: String(prev[id].options.length + 1), label: '', action: 'reply', reply: '' }],
      },
    }));

  const removeOption = async (id: string, idx: number) => {
    const option = nodes[id]?.options[idx];
    if (!option) return;
    if (option.action === 'goto' && option.target) {
      if (!await swalConfirm('Hapus pilihan dan submenu lanjutannya?', 'Seluruh pilihan di dalam submenu tersebut juga akan dihapus.')) return;
    }
    setNodes(prev => {
      const next = { ...prev, [id]: { ...prev[id], options: prev[id].options.filter((_, i) => i !== idx) } };
      if (option.action === 'goto' && option.target) {
        const stillReferenced = Object.values(next).some(node => node.options.some(item => item.action === 'goto' && item.target === option.target));
        if (!stillReferenced) delete next[option.target];
      }
      return next;
    });
    if (option.target && (previewNode === option.target || expandedNode === option.target)) {
      setPreviewNode(id);
      setExpandedNode(id);
    }
  };

  const createSubmenuForOption = (id: string, idx: number) => {
    const newID = genId();
    setNodes(prev => {
      const option = prev[id].options[idx];
      const optionName = option.label?.trim() || `Pilihan ${option.key}`;
      const options = prev[id].options.map((item, optionIndex) => optionIndex === idx
        ? { ...item, action: 'goto' as const, target: newID, reply: '' }
        : item);
      return {
        ...prev,
        [id]: { ...prev[id], options },
        [newID]: { message: `Anda memilih ${optionName}. Silakan pilih informasi berikutnya.`, options: [] },
      };
    });
    setPreviewNode(newID);
    setExpandedNode(newID);
    window.setTimeout(() => document.getElementById(`flow-node-${newID}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' }), 80);
    swalToast('Submenu dibuat dan langsung terhubung. Lengkapi pilihan di bagian yang terbuka.', 'success');
  };

  const changeOptionAction = async (id: string, idx: number, action: FlowOption['action']) => {
    const previous = nodes[id]?.options[idx];
    if (action === 'goto') {
      createSubmenuForOption(id, idx);
      return;
    }
    if (previous?.action === 'goto' && previous.target) {
      if (!await swalConfirm('Ganti aksi dan hapus submenu lanjutannya?', 'Isi submenu yang sebelumnya terhubung akan dihapus.')) return;
      const oldTarget = previous.target;
      setNodes(prev => {
        const options = prev[id].options.map((item, optionIndex) => optionIndex === idx ? { ...item, action, target: '' } : item);
        const next = { ...prev, [id]: { ...prev[id], options } };
        const stillReferenced = Object.values(next).some(node => node.options.some(item => item.action === 'goto' && item.target === oldTarget));
        if (!stillReferenced) delete next[oldTarget];
        return next;
      });
      if (previewNode === oldTarget) setPreviewNode(id);
      if (expandedNode === oldTarget) setExpandedNode(id);
      return;
    }
    patchOption(id, idx, { action, target: '' });
  };

  const applyTemplate = async (template: Record<string, FlowNode>, label: string) => {
    if (hasChanges && !await swalConfirm(`Gunakan contoh ${label}?`, 'Perubahan yang belum disimpan akan diganti.')) return;
    setNodes(cloneNodes(template));
    setPreviewNode(ROOT);
    setExpandedNode(ROOT);
    swalToast(`Contoh ${label} diterapkan. Sesuaikan isinya lalu simpan.`, 'success');
  };

  const removeNode = async (id: string) => {
    if (id === ROOT) return;
    if (!await swalConfirm('Hapus menu ini?', 'Opsi yang menuju ke menu ini akan berhenti berfungsi.')) return;
    setNodes(prev => {
      const next = { ...prev };
      delete next[id];
      // Bersihkan opsi 'goto' yang menunjuk ke node terhapus.
      for (const nid of Object.keys(next)) {
        next[nid] = { ...next[nid], options: next[nid].options.map(o => (o.action === 'goto' && o.target === id ? { ...o, target: '' } : o)) };
      }
      return next;
    });
    if (previewNode === id) setPreviewNode(ROOT);
    if (expandedNode === id) setExpandedNode(ROOT);
  };

  const cancelChanges = async () => {
    if (!hasChanges) return;
    if (!await swalConfirm('Batalkan semua perubahan?', 'Editor akan kembali ke versi terakhir yang sudah disimpan.')) return;
    try {
      const saved = JSON.parse(baseline) as {
        enabled: boolean;
        trigger: string;
        matchType: string;
        displayMode: 'auto' | 'text' | 'buttons';
        delayMin: number;
        delayMax: number;
        nodes: Record<string, FlowNode>;
      };
      setEnabled(!!saved.enabled);
      setTrigger(saved.trigger || 'menu');
      setMatchType(saved.matchType || 'exact');
      setDisplayMode(saved.displayMode || 'auto');
      setDelayMin(Number.isFinite(saved.delayMin) ? saved.delayMin : 2);
      setDelayMax(Number.isFinite(saved.delayMax) ? saved.delayMax : 4);
      setNodes(cloneNodes(saved.nodes));
      setPreviewNode(ROOT);
      setExpandedNode(ROOT);
      localStorage.removeItem(`wai_flow_draft_${agentId}`);
      swalToast('Perubahan dibatalkan. Versi tersimpan dipulihkan.', 'success');
    } catch {
      swalToast('Versi tersimpan belum bisa dipulihkan. Muat ulang halaman.', 'error');
    }
  };

  const save = async () => {
    // Validasi ringan sebelum kirim.
    if (!trigger.trim()) { swalToast('Kata pemicu wajib diisi, misalnya "menu".', 'error'); return; }
    for (const [id, node] of Object.entries(nodes)) {
      if (!node.message.trim()) { swalToast(`Menu "${nodeLabel(id, node)}" belum ada pesannya.`, 'error'); return; }
      if (id === ROOT && enabled && node.options.length === 0) { swalToast('Menu Utama membutuhkan minimal satu pilihan sebelum diaktifkan.', 'error'); return; }
      if (displayMode === 'buttons' && node.options.length > 3) { swalToast(`Mode Selalu tombol maksimal 3 pilihan di "${nodeLabel(id, node)}".`, 'error'); return; }
      const seenKeys = new Set<string>();
      for (const o of node.options) {
        if (!o.key.trim()) { swalToast(`Ada opsi tanpa kode ketik di "${nodeLabel(id, node)}".`, 'error'); return; }
        if (displayMode === 'buttons' && !o.label?.trim()) { swalToast(`Mode tombol membutuhkan nama pada setiap pilihan di "${nodeLabel(id, node)}".`, 'error'); return; }
        const normalizedKey = o.key.trim().toLowerCase();
        if (seenKeys.has(normalizedKey)) { swalToast(`Kode "${o.key}" dipakai dua kali di "${nodeLabel(id, node)}".`, 'error'); return; }
        seenKeys.add(normalizedKey);
        if (displayMode === 'buttons' && Array.from(o.label?.trim() || '').length > 24) { swalToast(`Nama tombol "${o.label}" maksimal 24 karakter.`, 'error'); return; }
        if (o.action === 'goto' && !o.target) { swalToast(`Opsi "${o.key}" belum memilih menu tujuan.`, 'error'); return; }
        if (o.action === 'goto' && o.target === id) { swalToast(`Opsi "${o.key}" tidak boleh menuju menu yang sama.`, 'error'); return; }
        if (o.action !== 'goto' && !o.reply?.trim()) { swalToast(`Opsi "${o.key}" belum ada teks balasannya.`, 'error'); return; }
      }
    }
    const structure = JSON.stringify({ root: ROOT, nodes });
    try {
      await saveFlow.mutateAsync({ enabled, trigger: trigger.trim() || 'menu', match_type: matchType, display_mode: displayMode, delay_min: delayMin, delay_max: delayMax, structure });
      setBaseline(configKey);
      localStorage.removeItem(`wai_flow_draft_${agentId}`);
      swalToast(enabled ? 'Alur disimpan dan aktif.' : 'Alur disimpan sebagai nonaktif.');
    } catch (e) {
      const detail = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(detail || 'Alur belum bisa disimpan.', 'error');
    }
  };

  if (isLoading && !loaded) {
    return <Box sx={{ py: 6, textAlign: 'center' }}><CircularProgress /></Box>;
  }

  const nodeIds = Object.keys(nodes);
  const optionCount = nodeIds.reduce((total, id) => total + nodes[id].options.length, 0);
  const preview = nodes[previewNode] || nodes[ROOT];

  return (
    <Box>
      <PageHeader title="Alur Percakapan"
        subtitle="Menu statis dari teks yang Anda susun sendiri. Hanya pemicu, kode, dan tombol menu yang diproses; kalimat biasa otomatis kembali ke percakapan AI." />

      <Card variant="outlined" sx={{ mb: 2, bgcolor: 'rgba(25,118,210,0.04)' }}>
        <CardContent sx={{ py: '12px !important' }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 800, mb: 1 }}>Cara membuat alur</Typography>
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: 'repeat(3, minmax(0, 1fr))' }, gap: 1 }}>
            {[
              ['1', 'Tentukan pemicu', 'Contoh: pelanggan mengetik “menu”.'],
              ['2', 'Buat pilihan', 'Isi nama pilihan dan tentukan apa yang terjadi.'],
              ['3', 'Simpan dan uji', 'Aktifkan, simpan, lalu coba dari nomor lain.'],
            ].map(([number, title, detail]) => (
              <Stack key={number} direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                <Box sx={{ width: 24, height: 24, flexShrink: 0, borderRadius: '50%', bgcolor: 'primary.main', color: 'primary.contrastText', display: 'grid', placeItems: 'center', fontSize: '0.72rem', fontWeight: 800 }}>{number}</Box>
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 700 }}>{title}</Typography>
                  <Typography variant="caption" color="text.secondary">{detail}</Typography>
                </Box>
              </Stack>
            ))}
          </Box>
        </CardContent>
      </Card>

      <Card variant="outlined" sx={{ mb: 2 }}>
        <CardContent>
          <Stack spacing={2}>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ justifyContent: 'space-between', alignItems: { xs: 'flex-start', sm: 'center' } }}>
              <Box>
                <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>Status dan pemicu</Typography>
                <Typography variant="caption" color="text.secondary">{nodeIds.length} menu · {optionCount} pilihan</Typography>
              </Box>
              <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                {hasChanges && <Chip size="small" color="warning" variant="outlined" label="Belum disimpan" />}
                <FormControlLabel sx={{ mr: 0 }}
                  control={<Switch checked={enabled} onChange={e => setEnabled(e.target.checked)} />}
                  label={<Typography sx={{ fontWeight: 700 }}>{enabled ? 'Aktif' : 'Nonaktif'}</Typography>}
                />
              </Stack>
            </Stack>
            <Alert severity="info" icon={<AccountTreeIcon fontSize="inherit" />}>
              Pelanggan mengetik <b>{trigger.trim() || 'kata pemicu'}</b> untuk membuka menu. Pilihan dapat ditekan sebagai tombol atau diketik dengan kode selama 30 menit. Ketik <b>0</b> untuk kembali atau <b>keluar</b> untuk menutup menu.
            </Alert>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'repeat(2, minmax(0, 1fr))', xl: 'repeat(4, minmax(0, 1fr))' }, gap: 1.5 }}>
              <TextField label="Kata pemicu" value={trigger} onChange={e => setTrigger(e.target.value)}
                size="small" fullWidth error={!trigger.trim()} helperText="Contoh: menu, bantuan, layanan" />
              <TextField label="Cara cocok" select value={matchType} onChange={e => setMatchType(e.target.value)}
                size="small" fullWidth helperText="Disarankan: persis sama">
                <MenuItem value="exact">Persis sama — paling aman</MenuItem>
                <MenuItem value="prefix">Pesan diawali kata pemicu</MenuItem>
                <MenuItem value="contains">Pesan mengandung kata pemicu</MenuItem>
              </TextField>
              <TextField label="Tampilan pilihan" select value={displayMode}
                onChange={e => setDisplayMode(e.target.value as 'auto' | 'text' | 'buttons')}
                size="small" fullWidth
                helperText={displayMode === 'auto' ? 'Tombol untuk 1–3 pilihan, menu angka untuk 4+.' : displayMode === 'buttons' ? 'Maksimal 3 pilihan per menu.' : 'Selalu tampil sebagai kode bernomor.'}>
                <MenuItem value="auto">Otomatis — disarankan</MenuItem>
                <MenuItem value="text">Selalu menu angka</MenuItem>
                <MenuItem value="buttons">Selalu tombol</MenuItem>
              </TextField>
              <TextField label="Jeda balasan" select value={`${delayMin}-${delayMax}`}
                onChange={event => {
                  const [min, max] = event.target.value.split('-').map(Number);
                  setDelayMin(min); setDelayMax(max);
                }} size="small" fullWidth
                helperText={hasChanges
                  ? 'Perubahan jeda belum berlaku. Klik Simpan & Aktifkan.'
                  : delayMax === 0 ? 'Tersimpan: balasan dikirim tanpa jeda.' : `Tersimpan: setiap balasan alur menunggu acak ${delayMin}–${delayMax} detik.`}>
                {DELAY_OPTIONS.map(option => (
                  <MenuItem key={option.value} value={option.value}>
                    <Box>
                      <Typography variant="body2">{option.label}</Typography>
                      <Typography variant="caption" color="text.secondary">{option.detail}</Typography>
                    </Box>
                  </MenuItem>
                ))}
              </TextField>
            </Box>
            {matchType === 'contains' && (
              <Alert severity="warning">Mode “mengandung” dapat membuka menu dari percakapan biasa. Gunakan hanya jika kata pemicunya cukup unik.</Alert>
            )}
            {displayMode === 'buttons' && Object.values(nodes).some(node => node.options.length > 3) && (
              <Alert severity="warning">Ada menu dengan lebih dari 3 pilihan. Kurangi pilihannya atau gunakan mode Otomatis sebelum menyimpan.</Alert>
            )}
            <Divider />
            <Box>
              <Typography variant="caption" sx={{ fontWeight: 700, display: 'block', mb: 0.75 }}>Mulai cepat dari contoh</Typography>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                <Button size="small" variant="outlined" startIcon={<AutoAwesomeIcon />} onClick={() => applyTemplate(DEFAULT_NODES, 'toko online')}>Toko online</Button>
                <Button size="small" variant="outlined" startIcon={<AutoAwesomeIcon />} onClick={() => applyTemplate(SERVICE_NODES, 'jasa/booking')}>Jasa / booking</Button>
              </Stack>
            </Box>
          </Stack>
        </CardContent>
      </Card>

      <Card variant="outlined" sx={{ mb: 2, bgcolor: 'action.hover' }}>
        <CardContent>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} sx={{ alignItems: { md: 'flex-start' } }}>
            <Box sx={{ flex: 1 }}>
              <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', mb: 0.75 }}>
                <VisibilityIcon fontSize="small" color="action" />
                <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>Pratinjau WhatsApp</Typography>
              </Stack>
              <TextField select size="small" value={previewNode} onChange={event => setPreviewNode(event.target.value)} sx={{ minWidth: 220, mb: 1 }} label="Menu yang dilihat">
                {nodeIds.map(id => <MenuItem key={id} value={id}>{nodeLabel(id, nodes[id])}</MenuItem>)}
              </TextField>
            </Box>
            <Box sx={{ width: { xs: '100%', md: '55%' }, bgcolor: '#e7f4e4', p: 1.25, borderRadius: 1.5 }}>
              <Box sx={{ bgcolor: '#fff', px: 1.25, py: 1, borderRadius: 1, whiteSpace: 'pre-wrap', fontSize: '0.82rem', lineHeight: 1.5, boxShadow: '0 1px 2px rgba(0,0,0,.08)' }}>
                {preview ? renderedPreview(previewNode, preview, displayMode) : 'Belum ada menu.'}
              </Box>
              {preview && previewUsesButtons(displayMode, preview) && (
                <Stack spacing={0.5} sx={{ mt: 0.75 }}>
                  {preview.options.map(option => (
                    <Button key={option.key} variant="outlined" size="small" disabled sx={{ bgcolor: '#fff', justifyContent: 'center', '&.Mui-disabled': { color: 'primary.main', borderColor: 'divider' } }}>
                      {option.label}
                    </Button>
                  ))}
                </Stack>
              )}
            </Box>
          </Stack>
        </CardContent>
      </Card>

      <Stack spacing={1.5}>
        {nodeIds.map(id => {
          const node = nodes[id];
          const parent = Object.entries(nodes).flatMap(([parentID, parentNode]) =>
            parentNode.options.map(option => ({ parentID, parentNode, option })))
            .find(item => item.option.action === 'goto' && item.option.target === id);
          return (
            <Accordion id={`flow-node-${id}`} key={id} expanded={expandedNode === id} onChange={(_, expanded) => setExpandedNode(expanded ? id : '')}
              disableGutters sx={{ border: '1px solid', borderColor: 'divider', '&:before': { display: 'none' } }}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center', width: '100%', pr: 1 }}>
                  <Chip size="small" color={id === ROOT ? 'primary' : 'default'} icon={<AccountTreeIcon fontSize="small" />} label={id === ROOT ? 'Menu Utama' : 'Submenu'} />
                  <Typography variant="body2" sx={{ fontWeight: 700, flex: 1 }}>{nodeLabel(id, node)}</Typography>
                  <Chip size="small" variant="outlined" label={`${node.options.length} pilihan`} />
                  {id !== ROOT && (
                    <IconButton size="small" onClick={event => { event.stopPropagation(); void removeNode(id); }} aria-label="Hapus menu"><DeleteIcon fontSize="small" /></IconButton>
                  )}
                </Stack>
              </AccordionSummary>
              <AccordionDetails sx={{ pt: 0 }}>
                {id !== ROOT && (
                  <Alert severity={parent ? 'info' : 'warning'} sx={{ mb: 1.25, py: 0.25 }}>
                    {parent
                      ? <>Submenu ini terbuka ketika pelanggan memilih <b>{parent.option.key}. {parent.option.label || 'Pilihan tanpa nama'}</b> dari <b>{nodeLabel(parent.parentID, parent.parentNode)}</b>.</>
                      : 'Submenu ini belum terhubung dengan pilihan mana pun. Hapus jika tidak digunakan.'}
                  </Alert>
                )}
                <TextField fullWidth multiline minRows={2} size="small" label="Pesan pembuka"
                  placeholder="Contoh: Halo, ada yang bisa kami bantu?"
                  helperText="Daftar pilihan dibuat otomatis dari nama pilihan di bawah. Tidak perlu mengetik nomor lagi di sini."
                  value={node.message} onChange={e => patchNode(id, { message: e.target.value })} sx={{ mb: 1.5 }} />

                <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600 }}>Pilihan</Typography>
                <Stack spacing={1} sx={{ mt: 0.5 }}>
                  {node.options.map((o, idx) => (
                    <Box key={idx} sx={{ p: { xs: 1.25, md: 1.5 }, border: '1px solid', borderColor: 'divider', borderRadius: 2, bgcolor: 'background.default' }}>
                      <Stack direction="row" spacing={1} sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 1.25 }}>
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minWidth: 0 }}>
                          <Chip size="small" color="primary" variant="outlined" label={`Pilihan ${idx + 1}`} />
                          <Typography variant="body2" sx={{ fontWeight: 700 }} noWrap>
                            {o.label?.trim() || 'Belum diberi nama'}
                          </Typography>
                        </Stack>
                        <IconButton size="small" color="error" onClick={() => { void removeOption(id, idx); }} aria-label={`Hapus pilihan ${idx + 1}`}>
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </Stack>
                      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '96px minmax(240px, 1fr) minmax(300px, 380px)' }, gap: 1.25, alignItems: 'start' }}>
                        <TextField label="Kode" value={o.key} onChange={e => patchOption(id, idx, { key: e.target.value })}
                          size="small" placeholder="1" slotProps={{ htmlInput: { 'aria-label': `Kode pilihan ${idx + 1}` } }} />
                        <TextField label="Nama yang tampil di menu" value={o.label || ''} onChange={e => patchOption(id, idx, { label: e.target.value })}
                          size="small" fullWidth placeholder="Contoh: Lihat katalog" />
                        <TextField label="Setelah pelanggan memilih" select value={o.action}
                          onChange={e => { void changeOptionAction(id, idx, e.target.value as FlowOption['action']); }}
                          size="small" fullWidth helperText={ACTIONS.find(action => action.value === o.action)?.description || ''}
                          slotProps={{ select: { renderValue: (value: unknown) => ACTIONS.find(action => action.value === value)?.label || String(value) } }}>
                          {ACTIONS.map(action => <MenuItem key={action.value} value={action.value}>{action.label}</MenuItem>)}
                        </TextField>
                      </Box>
                      <Box sx={{ mt: 1.25 }}>
                        {o.action === 'goto' ? (
                          <Alert severity={o.target && nodes[o.target] ? 'success' : 'warning'} sx={{ width: '100%', py: 0.25 }}
                            action={o.target && nodes[o.target]
                              ? <Button size="small" onClick={() => { setExpandedNode(o.target || ROOT); setPreviewNode(o.target || ROOT); }}>Edit submenu</Button>
                              : <Button size="small" onClick={() => createSubmenuForOption(id, idx)}>Buat submenu</Button>}>
                            {o.target && nodes[o.target]
                              ? <>Terhubung ke <b>{nodeLabel(o.target, nodes[o.target])}</b>. Submenu sudah dibuat otomatis.</>
                              : 'Pilihan lanjutan belum memiliki submenu. Klik “Buat submenu”.'}
                          </Alert>
                        ) : (
                          <Box sx={{ width: '100%' }}>
                            <TextField label={o.action === 'handoff' ? 'Pesan sebelum diteruskan ke CS' : 'Teks balasan'} value={o.reply || ''}
                              onChange={e => patchOption(id, idx, { reply: e.target.value })} size="small" fullWidth multiline maxRows={4} />
                            {o.action === 'handoff' && (
                              <Alert severity="warning" sx={{ mt: 1, py: 0.25 }}>
                                Alur dijeda untuk kontak ini sampai petugas membuka <b>Butuh CS</b> dan menyelesaikan penanganan.
                              </Alert>
                            )}
                          </Box>
                        )}
                      </Box>
                    </Box>
                  ))}
                  {node.options.length === 0 && <Alert severity="warning">Belum ada pilihan. Tambahkan minimal satu pilihan agar menu dapat digunakan.</Alert>}
                  <Button size="small" startIcon={<AddIcon />} onClick={() => addOption(id)} sx={{ alignSelf: 'flex-start' }}>Tambah pilihan</Button>
                </Stack>
              </AccordionDetails>
            </Accordion>
          );
        })}
      </Stack>

      <Divider sx={{ my: 2 }} />
      <Alert severity="warning" sx={{ mb: 2 }}>
        Saat menguji pilihan <b>Teruskan ke CS</b>, Alur Otomatis dijeda khusus untuk nomor penguji tersebut. Buka <b>Butuh CS</b> lalu klik <b>Selesaikan penanganan</b> sebelum menguji menu kembali.
      </Alert>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'center' }, justifyContent: 'space-between', position: 'sticky', bottom: 8, bgcolor: 'background.paper', p: 1, border: '1px solid', borderColor: 'divider', borderRadius: 1.5, zIndex: 2 }}>
        <Typography variant="caption" color="text.secondary" sx={{ maxWidth: 360 }}>
          Submenu dibuat otomatis saat aksi “Tampilkan pilihan lanjutan” dipilih.
        </Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'center' } }}>
          <Typography variant="caption" color={hasChanges ? 'warning.main' : 'text.secondary'}>
            {hasChanges ? 'Ada perubahan yang belum disimpan' : enabled ? 'Alur aktif dan tersimpan' : 'Alur tersimpan sebagai nonaktif'}
          </Typography>
          {hasChanges && (
            <Button variant="text" color="inherit" onClick={() => { void cancelChanges(); }} disabled={saveFlow.isPending}>
              Batalkan perubahan
            </Button>
          )}
          <Button variant="contained" startIcon={saveFlow.isPending ? <CircularProgress size={16} color="inherit" /> : <SaveIcon />}
            onClick={save} disabled={saveFlow.isPending}>{enabled ? 'Simpan & Aktifkan' : 'Simpan Alur'}</Button>
        </Stack>
      </Stack>
    </Box>
  );
}
