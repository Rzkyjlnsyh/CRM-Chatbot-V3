import { useEffect, useMemo, useState } from 'react';
import {
  Box, Typography, Button, Stack, Chip, Paper, Alert, Divider, TextField, Switch, FormControlLabel,
  Dialog, DialogTitle, DialogContent, DialogActions, IconButton, Tooltip, Slider, CircularProgress,
  MenuItem, Autocomplete,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import ArrowDownwardIcon from '@mui/icons-material/ArrowDownward';
import LockIcon from '@mui/icons-material/LockOutlined';
import ScienceIcon from '@mui/icons-material/ScienceOutlined';
import PageHeader from './PageHeader';
import { useCrmPipeline, useSavePipelineStages, useSavePipelineConfig, useSaveLabelRule, useDeleteLabelRule, useTestLabelRules, useLabels } from '../hooks';
import type { LabelRule, LeadStageDef } from '../types';
import { swalConfirm, swalToast } from '../services/swal';

const STAGE_COLORS = ['#90A4AE', '#42A5F5', '#FFB300', '#EF6C00', '#2E7D32', '#9E9E9E', '#AB47BC', '#26A69A', '#EC407A', '#5C6BC0'];

const STAGE_EXAMPLES: Record<string, string> = {
  new: 'Percakapan baru, belum bisa dinilai minatnya.',
  cold: 'Relevan tapi belum ada kebutuhan jelas; menunda atau cari info.',
  warm: 'Tanya produk/harga/stok — ada minat nyata.',
  hot: 'Mau beli/booking/daftar — niat memproses jelas.',
  customer: 'Deal selesai. HANYA dari aktivitas/manual — AI dilarang menetapkan.',
  unqualified: 'Salah sasaran, spam, atau tegas tidak butuh layanan.',
};

function hexToBg(hex: string): string {
  const clean = (hex || '#90A4AE').replace('#', '');
  return `#${clean}1c`;
}

export default function PipelinePanel({ agentId }: { agentId: number }) {
  const { data, isLoading } = useCrmPipeline(agentId);
  const saveStages = useSavePipelineStages(agentId);
  const saveConfig = useSavePipelineConfig(agentId);
  const saveRule = useSaveLabelRule(agentId);
  const deleteRule = useDeleteLabelRule(agentId);
  const testRules = useTestLabelRules(agentId);
  const waSync = useLabels(agentId);
  const waLabels = waSync.data;

  useEffect(() => {
    if (agentId) waSync.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId]);

  const [stages, setStages] = useState<LeadStageDef[]>([]);
  const [dirtyStages, setDirtyStages] = useState(false);
  const [config, setConfig] = useState({ smart_labels_enabled: true, closing_definition: '' });
  const [dirtyConfig, setDirtyConfig] = useState(false);
  const [ruleDialog, setRuleDialog] = useState<Partial<LabelRule> | null>(null);
  const [testText, setTestText] = useState('');
  const [testResult, setTestResult] = useState<{ matched: { id: number; name: string; action_stage: string; action_wa_label: string }[]; count: number } | null>(null);

  useEffect(() => {
    if (!data) return;
    setStages([...data.stages].sort((a, b) => a.rank - b.rank));
    setConfig({ smart_labels_enabled: data.config.smart_labels_enabled, closing_definition: data.config.closing_definition });
    setDirtyStages(false);
    setDirtyConfig(false);
  }, [data]);

  const rules = useMemo(() => [...(data?.rules || [])].sort((a, b) => a.priority - b.priority || a.id - b.id), [data]);
  const stageName = (key: string) => stages.find(s => s.key === key)?.name || key;

  const patchStage = (idx: number, patch: Partial<LeadStageDef>) => {
    setStages(prev => prev.map((s, i) => (i === idx ? { ...s, ...patch } : s)));
    setDirtyStages(true);
  };
  const moveStage = (idx: number, dir: -1 | 1) => {
    setStages(prev => {
      const next = [...prev];
      const target = idx + dir;
      if (target < 0 || target >= next.length) return prev;
      [next[idx], next[target]] = [next[target], next[idx]];
      return next;
    });
    setDirtyStages(true);
  };
  const addStage = () => {
    const key = `tahap_${stages.length + 1}`;
    setStages(prev => [...prev, {
      id: 0, agent_id: agentId, key, name: 'Tahap Baru', color: '#26A69A', rank: prev.length,
      description: '', is_closing: false, min_confidence: 0.72, is_default: false,
    }]);
    setDirtyStages(true);
  };
  const removeStage = (idx: number) => {
    const s = stages[idx];
    if (s.key === 'customer') return;
    setStages(prev => prev.filter((_, i) => i !== idx));
    setDirtyStages(true);
  };

  const handleSaveStages = async () => {
    const renamed = stages.map((s, i) => ({ ...s, rank: i }));
    try {
      await saveStages.mutateAsync(renamed);
      setDirtyStages(false);
      swalToast('Pipeline tahap tersimpan');
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(msg || 'Gagal menyimpan tahap', 'error');
    }
  };

  const handleSaveConfig = async () => {
    try {
      await saveConfig.mutateAsync(config);
      setDirtyConfig(false);
      swalToast('Pengaturan pelabelan tersimpan');
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(msg || 'Gagal menyimpan pengaturan', 'error');
    }
  };

  const handleSaveRule = async () => {
    if (!ruleDialog) return;
    const kw = (ruleDialog.trigger_keywords || '').split(',').map(k => k.trim()).filter(Boolean);
    try {
      await saveRule.mutateAsync({ ...ruleDialog, trigger_keywords: JSON.stringify(kw) });
      setRuleDialog(null);
      swalToast('Aturan tersimpan');
    } catch (e) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      swalToast(msg || 'Gagal menyimpan aturan', 'error');
    }
  };

  const handleDeleteRule = async (rid: number) => {
    if (!(await swalConfirm('Hapus aturan ini?'))) return;
    try {
      await deleteRule.mutateAsync(rid);
      swalToast('Aturan dihapus');
    } catch {
      swalToast('Gagal menghapus aturan', 'error');
    }
  };

  const handleTestRules = async () => {
    if (!testText.trim()) return;
    try {
      const res = await testRules.mutateAsync(testText);
      setTestResult(res);
    } catch {
      swalToast('Gagal menguji aturan', 'error');
    }
  };

  if (isLoading) {
    return <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}><CircularProgress /></Box>;
  }

  return (
    <Box>
      <PageHeader
        title="Pipeline & Label"
        subtitle="Tahap CRM, definisi closing, dan aturan pelabelan otomatis — semuanya bisa disesuaikan dengan bisnis Anda."
      />

      {/* ===== Label WhatsApp asli dari perangkat ===== */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2, borderRadius: 1.5 }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 800, mb: 1 }}>
          🏷️ Label WhatsApp asli di perangkat
        </Typography>
        {waLabels && waLabels.length > 0 ? (
          <>
            <Stack direction="row" spacing={0.5} sx={{ flexWrap: 'wrap', rowGap: 0.5, mb: 1 }}>
              {waLabels.slice(0, 30).map((l) => (
                <Chip key={l.label_id} size="small" label={`${l.name} (${l.count} kontak)`} />
              ))}
            </Stack>
            <Typography variant="caption" color="text.secondary">
              Ini label yang benar-benar ada di WhatsApp bisnis Anda (hasil sinkron perangkat). Gunakan nama yang sama persis saat membuat aturan pelabelan otomatis di bawah, supaya label menempel langsung ke WhatsApp.
            </Typography>
          </>
        ) : (
          <Typography variant="body2" color="text.secondary">
            Belum ada label terdeteksi — pastikan WhatsApp terhubung lalu buka halaman ini lagi (sinkron label otomatis).
          </Typography>
        )}
      </Paper>

      {/* ===== Tahap Pipeline ===== */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2, borderRadius: 1.5 }}>
        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 0.5 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 800 }}>Tahap Pipeline CRM</Typography>
          <Button size="small" startIcon={<AddIcon />} onClick={addStage}>Tambah tahap</Button>
        </Stack>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.5 }}>
          Nama, warna, urutan, dan ambang keyakinan AI per tahap. AI hanya boleh memakai tahap di daftar ini — tidak pernah menciptakan tahap sendiri. Tahap bertanda closing hanya diisi dari aktivitas/manual.
        </Typography>
        {stages.map((s, idx) => (
          <Stack key={s.key} direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'center' }, py: 1, borderBottom: idx < stages.length - 1 ? '1px solid' : 'none', borderColor: 'divider' }}>
            <Stack direction="row" spacing={0.5}>
              <IconButton size="small" onClick={() => moveStage(idx, -1)} disabled={idx === 0} aria-label="Naik"><ArrowUpwardIcon fontSize="small" /></IconButton>
              <IconButton size="small" onClick={() => moveStage(idx, 1)} disabled={idx === stages.length - 1} aria-label="Turun"><ArrowDownwardIcon fontSize="small" /></IconButton>
            </Stack>
            <Chip
              size="small"
              label={s.name}
              sx={{ minWidth: 110, fontWeight: 800, color: s.color, bgcolor: hexToBg(s.color), border: `1px solid ${s.color}` }}
            />
            <TextField size="small" label="Nama" value={s.name} sx={{ flex: 1, minWidth: 140 }}
              onChange={e => patchStage(idx, { name: e.target.value })} />
            <TextField size="small" label="Key (huruf kecil_angka)" value={s.key} sx={{ width: 170 }}
              disabled={s.key === 'customer'} onChange={e => patchStage(idx, { key: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '_') })} />
            <Autocomplete
              size="small" disableClearable options={STAGE_COLORS} value={s.color}
              sx={{ width: 130 }}
              onChange={(_, v) => patchStage(idx, { color: v })}
              renderInput={params => <TextField {...params} label="Warna" />}
              renderOption={(props, opt) => (
                <Box component="li" {...props} sx={{ '&.MuiAutocomplete-option': { p: 0.5 } }}>
                  <Box sx={{ width: '100%', height: 22, borderRadius: 0.5, bgcolor: opt }} />
                </Box>
              )}
            />
            <Box sx={{ width: 130 }}>
              <Typography variant="caption" color="text.secondary">Ambang AI: {Math.round(s.min_confidence * 100)}%</Typography>
              <Slider size="small" min={0.5} max={0.98} step={0.01} value={s.min_confidence}
                onChange={(_, v) => patchStage(idx, { min_confidence: v as number })} />
            </Box>
            <Tooltip title={s.key === 'customer' ? 'Tahap closing wajib ada dan tidak bisa dihapus' : 'AI dilarang menetapkan tahap closing'}>
              <FormControlLabel
                control={<Switch size="small" checked={s.is_closing} disabled={s.key === 'customer'}
                  onChange={e => patchStage(idx, { is_closing: e.target.checked })} />}
                label={<Typography variant="caption">{s.is_closing ? <LockIcon sx={{ fontSize: 13, verticalAlign: 'middle' }} /> : null} closing</Typography>}
                sx={{ m: 0 }}
              />
            </Tooltip>
            <IconButton size="small" color="error" disabled={s.key === 'customer'} onClick={() => removeStage(idx)} aria-label="Hapus tahap">
              <DeleteIcon fontSize="small" />
            </IconButton>
            <TextField size="small" label="Deskripsi (dipakai AI)" value={s.description} multiline minRows={1} maxRows={3}
              placeholder={STAGE_EXAMPLES[s.key] || 'Jelaskan kapan pelanggan masuk tahap ini'}
              sx={{ flex: 2, minWidth: 220 }}
              onChange={e => patchStage(idx, { description: e.target.value })} />
          </Stack>
        ))}
        <Box sx={{ mt: 1.5 }}>
          <Button variant="contained" disabled={!dirtyStages} onClick={handleSaveStages}>Simpan Tahap</Button>
        </Box>
      </Paper>

      {/* ===== Pelabelan Pintar ===== */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2, borderRadius: 1.5 }}>
        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 0.5 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 800 }}>Pelabelan Pintar (AI)</Typography>
          <FormControlLabel
            control={<Switch checked={config.smart_labels_enabled} onChange={e => { setConfig(c => ({ ...c, smart_labels_enabled: e.target.checked })); setDirtyConfig(true); }} />}
            label="Aktif"
          />
        </Stack>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.5 }}>
          AI menilai tahap minat pelanggan dari percakapan. Anti-ngawur bawaan: hanya tahap di daftar yang boleh dipakai, keyakinan harus melewati ambang per-tahap, AI tidak pernah menurunkan tahap manual/aktivitas, dan tidak pernah menetapkan closing.
        </Typography>
        <TextField
          label="Definisi closing bisnis Anda"
          placeholder={'Contoh: "Closing = customer sudah transfer DP dan kirim bukti." atau "Closing = customer konfirmasi booking jadwal."'}
          multiline minRows={3} maxRows={6} fullWidth value={config.closing_definition}
          onChange={e => { setConfig(c => ({ ...c, closing_definition: e.target.value })); setDirtyConfig(true); }}
          helperText="Tulis dengan kalimat Anda sendiri kapan pelanggan dianggap 'jadi'. Ini yang membedakan indikator closing tiap bisnis."
        />
        <Box sx={{ mt: 1.5 }}>
          <Button variant="contained" disabled={!dirtyConfig} onClick={handleSaveConfig}>Simpan Pengaturan</Button>
        </Box>
      </Paper>

      {/* ===== Aturan Otomatis ===== */}
      <Paper variant="outlined" sx={{ p: 2, borderRadius: 1.5 }}>
        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 0.5 }}>
          <Typography variant="subtitle1" sx={{ fontWeight: 800 }}>Aturan Pelabelan Otomatis (tanpa AI)</Typography>
          <Button size="small" startIcon={<AddIcon />} onClick={() => setRuleDialog({
            name: '', enabled: true, priority: rules.length + 1, trigger_keywords: '', trigger_stage: '', action_stage: '', action_wa_label: '',
          })}>Tambah aturan</Button>
        </Stack>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.5 }}>
          Deterministik 100%: jika pesan pelanggan mengandung kata kunci (dan sedang di tahap pemicu bila diisi), langsung terapkan aksi. Kontak yang dikunci manual atau sudah closing tidak pernah disentuh.
        </Typography>
        {rules.length === 0 && <Alert severity="info" sx={{ mb: 1 }}>Belum ada aturan. Contoh: kata kunci "bukti transfer" → tahap "hot" + label WhatsApp "Transfer".</Alert>}
        {rules.map(rule => (
          <Stack key={rule.id} direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'center' }, py: 1, borderBottom: '1px solid', borderColor: 'divider' }}>
            <Switch size="small" checked={rule.enabled} onChange={e => saveRule.mutateAsync({ ...rule, enabled: e.target.checked, trigger_keywords: rule.trigger_keywords })} />
            <Typography variant="body2" sx={{ fontWeight: 700, minWidth: 140 }}>{rule.name}</Typography>
            <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>
              Keyword: {(rule.trigger_keywords ? (() => { try { return (JSON.parse(rule.trigger_keywords) as string[]).join(', '); } catch { return rule.trigger_keywords; } })() : '')}
              {rule.trigger_stage ? ` · dari tahap ${stageName(rule.trigger_stage)}` : ''}
            </Typography>
            {rule.action_stage && <Chip size="small" label={`→ ${stageName(rule.action_stage)}`} color="primary" variant="outlined" />}
            {rule.action_wa_label && <Chip size="small" label={`🏷 ${rule.action_wa_label}`} variant="outlined" />}
            <Stack direction="row" spacing={0.5}>
              <Button size="small" onClick={() => setRuleDialog({ ...rule })}>Edit</Button>
              <IconButton size="small" color="error" onClick={() => handleDeleteRule(rule.id)} aria-label="Hapus aturan"><DeleteIcon fontSize="small" /></IconButton>
            </Stack>
          </Stack>
        ))}

        <Divider sx={{ my: 2 }} />
        <Typography variant="body2" sx={{ fontWeight: 700, mb: 1 }}>
          <ScienceIcon sx={{ fontSize: 16, verticalAlign: 'middle', mr: 0.5 }} />
          Uji aturan (dry-run — tidak mengubah data apa pun)
        </Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { sm: 'flex-start' } }}>
          <TextField size="small" fullWidth placeholder={'Contoh pesan pelanggan: "saya sudah transfer, ini buktinya"'}
            value={testText} onChange={e => setTestText(e.target.value)} />
          <Button variant="outlined" onClick={handleTestRules} disabled={!testText.trim() || testRules.isPending}>
            Uji
          </Button>
        </Stack>
        {testResult && (
          <Box sx={{ mt: 1 }}>
            {testResult.count === 0
              ? <Alert severity="info">Tidak ada aturan yang cocok dengan teks itu.</Alert>
              : testResult.matched.map(m => (
                <Alert key={m.id} severity="success" sx={{ mb: 0.5 }}>
                  Aturan <b>{m.name}</b> cocok{m.action_stage ? ` → pindah ke tahap ${stageName(m.action_stage)}` : ''}{m.action_wa_label ? ` → beri label WhatsApp "${m.action_wa_label}"` : ''}
                </Alert>
              ))}
          </Box>
        )}
      </Paper>

      {/* Dialog aturan */}
      <Dialog open={!!ruleDialog} onClose={() => setRuleDialog(null)} maxWidth="sm" fullWidth>
        <DialogTitle>{ruleDialog?.id ? 'Edit Aturan' : 'Aturan Baru'}</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <TextField size="small" label="Nama aturan" value={ruleDialog?.name || ''} fullWidth
              onChange={e => setRuleDialog(d => ({ ...d, name: e.target.value }))} />
            <TextField size="small" label="Kata kunci pemicu (pisahkan dengan koma)" value={ruleDialog?.trigger_keywords || ''} fullWidth
              helperText="Dicocokkan pada isi pesan pelanggan. Kata < 3 huruf diabaikan."
              onChange={e => setRuleDialog(d => ({ ...d, trigger_keywords: e.target.value }))} />
            <TextField select size="small" label="Hanya jika tahap sekarang (opsional)" value={ruleDialog?.trigger_stage || ''} fullWidth
              onChange={e => setRuleDialog(d => ({ ...d, trigger_stage: e.target.value }))}>
              <MenuItem value="">Semua tahap</MenuItem>
              {stages.map(s => <MenuItem key={s.key} value={s.key}>{s.name}</MenuItem>)}
            </TextField>
            <TextField select size="small" label="Aksi: pindah ke tahap (opsional)" value={ruleDialog?.action_stage || ''} fullWidth
              onChange={e => setRuleDialog(d => ({ ...d, action_stage: e.target.value }))}>
              <MenuItem value="">Tidak pindah tahap</MenuItem>
              {stages.filter(s => !s.is_closing).map(s => <MenuItem key={s.key} value={s.key}>{s.name}</MenuItem>)}
            </TextField>
            <TextField size="small" label="Aksi: label WhatsApp (nama label, opsional)" value={ruleDialog?.action_wa_label || ''} fullWidth
              helperText="Label harus sudah ada di WhatsApp, lalu sinkron dari menu Kontak."
              onChange={e => setRuleDialog(d => ({ ...d, action_wa_label: e.target.value }))} />
            <TextField type="number" size="small" label="Prioritas (kecil = didahulukan)" value={ruleDialog?.priority ?? 1} fullWidth
              onChange={e => setRuleDialog(d => ({ ...d, priority: Number(e.target.value) || 0 }))} />
            {!ruleDialog?.action_stage && !ruleDialog?.action_wa_label && (
              <Alert severity="warning">Pilih minimal satu aksi (ubah tahap dan/atau label WhatsApp).</Alert>
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRuleDialog(null)}>Batal</Button>
          <Button variant="contained" onClick={handleSaveRule} disabled={!ruleDialog?.name?.trim()}>
            {ruleDialog?.id ? 'Simpan' : 'Buat Aturan'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
