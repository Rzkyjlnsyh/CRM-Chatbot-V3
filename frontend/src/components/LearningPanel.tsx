import { useState, useEffect, type ReactElement } from 'react';
import {
  Box, Button, Chip, CircularProgress, Dialog, DialogActions,
  DialogContent, DialogTitle, Divider, FormControlLabel, Grid,
  MenuItem, Paper, Select, Slider,
  Stack, Switch, Tab, Tabs, TextField, Typography,
  Tooltip,
} from '@mui/material';
import AutoFixHighIcon from '@mui/icons-material/AutoFixHigh';
import HistoryIcon from '@mui/icons-material/History';
import SettingsIcon from '@mui/icons-material/Settings';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import CancelIcon from '@mui/icons-material/Cancel';
import RestoreIcon from '@mui/icons-material/Restore';
import SaveIcon from '@mui/icons-material/Save';
import LightbulbIcon from '@mui/icons-material/LightbulbOutlined';
import BarChartIcon from '@mui/icons-material/BarChartOutlined';
import EmojiEmotionsIcon from '@mui/icons-material/EmojiEmotionsOutlined';
import ChatIcon from '@mui/icons-material/ChatOutlined';
import StorageIcon from '@mui/icons-material/StorageOutlined';
import TimerIcon from '@mui/icons-material/TimerOutlined';
import {
  useLearningStatus, useLearningScore, useLearningPatterns, useLearningSnapshots, useLearningConfig,
  useStartLearning, useApplyPattern, useRejectPattern, useApplyAllPatterns,
  useCreateSnapshot, useRollbackSnapshot, useSaveLearningConfig, useLabels,
} from '../hooks';
import PageHeader from './PageHeader';
import { swalConfirm, swalToast } from '../services/swal';
import { apiErrorMessage } from '../services/errors';
import type { LearningPattern } from '../types';

type ApplyAllResult = { applied: number; total: number };

const PATTERN_LABELS: Record<string, { label: string; icon: ReactElement; color: string }> = {
  greeting: { label: 'Sapaan', icon: <ChatIcon fontSize="small" />, color: '#4caf50' },
  closing: { label: 'Closing', icon: <CheckCircleIcon fontSize="small" />, color: '#2196f3' },
  objection_handling: { label: 'Atasi Keberatan', icon: <LightbulbIcon fontSize="small" />, color: '#ff9800' },
  upsell: { label: 'Upsell', icon: <BarChartIcon fontSize="small" />, color: '#9c27b0' },
  tone: { label: 'Tone/Gaya', icon: <EmojiEmotionsIcon fontSize="small" />, color: '#e91e63' },
  emoji_style: { label: 'Gaya Emoji', icon: <EmojiEmotionsIcon fontSize="small" />, color: '#ff5722' },
  phrase: { label: 'Frasa', icon: <ChatIcon fontSize="small" />, color: '#607d8b' },
  follow_up: { label: 'Follow-up', icon: <TimerIcon fontSize="small" />, color: '#00bcd4' },
  label_handling: { label: 'Penanganan Label', icon: <LightbulbIcon fontSize="small" />, color: '#26a69a' },
  closing_path: { label: 'Jalur Closing', icon: <CheckCircleIcon fontSize="small" />, color: '#66bb6a' },
};

function PatternCard({ pattern, onApply, onReject, loading }: {
  pattern: LearningPattern;
  onApply: () => void;
  onReject: () => void;
  loading: boolean;
}) {
  const meta = PATTERN_LABELS[pattern.pattern_type] || { label: pattern.pattern_type, icon: <LightbulbIcon fontSize="small" />, color: '#888' };
  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack spacing={1.5}>
        <Stack direction="row" spacing={1} sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
          <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
            <Chip
              icon={meta.icon}
              label={meta.label}
              size="small"
              sx={{ bgcolor: meta.color + '22', color: meta.color, fontWeight: 700, border: `1px solid ${meta.color}44` }}
            />
            {pattern.label_name && (
              <Chip size="small" label={`🏷 ${pattern.label_name}`} variant="outlined" sx={{ color: 'text.secondary' }} />
            )}
          </Stack>
          <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
            <Tooltip title={`Confidence: ${(pattern.confidence * 100).toFixed(0)}%`}>
              <Chip size="small" label={`${(pattern.confidence * 100).toFixed(0)}%`}
                color={pattern.confidence >= 0.7 ? 'success' : pattern.confidence >= 0.5 ? 'warning' : 'default'}
                variant="outlined" />
            </Tooltip>
            {pattern.closing_impact > 0 && (
              <Tooltip title="Dampak closing">
                <Chip size="small" label={`↗${(pattern.closing_impact * 100).toFixed(0)}%`} color="primary" variant="outlined" />
              </Tooltip>
            )}
          </Stack>
        </Stack>
        <Box>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700 }}>Pemicu:</Typography>
          <Typography variant="body2" sx={{ color: 'text.primary', fontStyle: 'italic' }}>{pattern.trigger_context || '(konteks umum)'}</Typography>
        </Box>
        <Box>
          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 700 }}>Template balasan:</Typography>
          <Typography variant="body2" sx={{ color: 'success.main', fontWeight: 500 }}>{pattern.response_template}</Typography>
        </Box>
        {pattern.emoji_signature && (
          <Typography variant="body2" color="text.secondary">
            🎨 Emoji: <Typography component="span" sx={{ fontSize: 18 }}>{pattern.emoji_signature}</Typography>
          </Typography>
        )}
        <Stack direction="row" spacing={1} sx={{ justifyContent: 'flex-end', pt: 0.5 }}>
          <Button size="small" variant="outlined" color="error" startIcon={<CancelIcon />}
            onClick={onReject} disabled={loading}>Tolak</Button>
          <Button size="small" variant="contained" color="success" startIcon={<CheckCircleIcon />}
            onClick={onApply} disabled={loading}>Terapkan</Button>
        </Stack>
      </Stack>
    </Paper>
  );
}

export default function LearningPanel({ agentId }: { agentId: number }) {
  const [tab, setTab] = useState(0);
  const isoDate = (d: Date) => {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    return `${y}-${m}-${day}`;
  };
  const [startDate, setStartDate] = useState(() => {
    const s = new Date();
    s.setDate(s.getDate() - 30);
    return isoDate(s);
  });
  const [endDate, setEndDate] = useState(() => isoDate(new Date()));
  const [applyMinConf, setApplyMinConf] = useState(0.6);
  const [snapshotLabel, setSnapshotLabel] = useState('');
  const [snapshotOpen, setSnapshotOpen] = useState(false);

  const { data: status, isLoading } = useLearningStatus(agentId);
  const { data: score } = useLearningScore(agentId);
  const { data: patterns } = useLearningPatterns(agentId, 'suggested');
  const { data: snapshots } = useLearningSnapshots(agentId);
  const { data: config } = useLearningConfig(agentId);
  const waSync = useLabels(agentId);
  const waLabels = waSync.data;
  const [closingLabels, setClosingLabels] = useState('');

  useEffect(() => {
    if (config?.closing_labels !== undefined) setClosingLabels(config.closing_labels);
  }, [config?.closing_labels]);

  useEffect(() => {
    if (agentId) waSync.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentId]);

  const startLearning = useStartLearning(agentId);
  const applyPattern = useApplyPattern(agentId);
  const rejectPattern = useRejectPattern(agentId);
  const applyAll = useApplyAllPatterns(agentId);
  const createSnapshot = useCreateSnapshot(agentId);
  const rollback = useRollbackSnapshot(agentId);
  const saveConfig = useSaveLearningConfig(agentId);

  const busy = startLearning.isPending || applyPattern.isPending || rejectPattern.isPending
    || applyAll.isPending || createSnapshot.isPending || rollback.isPending;

  const handleRunLearning = () => {
    if (startDate && endDate && startDate > endDate) {
      swalToast('Rentang tanggal terbalik: "Dari" lebih baru daripada "Sampai".', 'error');
      return;
    }
    startLearning.mutate(
      { start_date: startDate || undefined, end_date: endDate || undefined },
      {
        onSuccess: (resp: unknown) => {
          const d = (resp as { data?: { human_chats?: number; contacts?: number; total_chats?: number } })?.data;
          if (d && d.human_chats !== undefined && d.human_chats > 0) {
            swalToast(`Menganalisa ${d.human_chats} chat CS dari ${d.contacts} kontak — pola muncul beberapa saat lagi, lihat kartu status.`);
          } else {
            swalToast(`Tidak ada chat CS manusia dalam rentang ini (${d?.total_chats ?? 0} chat total, ${d?.contacts ?? 0} kontak). Perluas rentang tanggal lalu coba lagi.`, 'warning');
          }
        },
        onError: (err: unknown) => swalToast(apiErrorMessage(err, 'Gagal menjalankan learning'), 'error'),
      }
    );
  };

  const handleApply = (pid: number) => {
    applyPattern.mutate(pid, {
      onSuccess: () => swalToast('Pola diterapkan ke knowledge base!'),
      onError: (err: unknown) => swalToast(apiErrorMessage(err, 'Gagal'), 'error'),
    });
  };

  const handleReject = (pid: number) => {
    rejectPattern.mutate(pid, {
      onSuccess: () => swalToast('Pola ditolak'),
      onError: () => swalToast('Gagal', 'error'),
    });
  };

  const handleApplyAll = () => {
    swalConfirm('Terapkan semua pola?', `Semua pola dengan confidence ≥ ${(applyMinConf * 100).toFixed(0)}% akan diterapkan ke knowledge base.`)
      .then((ok: boolean) => {
        if (!ok) return;
        applyAll.mutate(applyMinConf, {
          onSuccess: (data: ApplyAllResult) => swalToast(`${data.applied}/${data.total} pola diterapkan!`),
          onError: (err: unknown) => swalToast(apiErrorMessage(err, 'Gagal'), 'error'),
        });
      });
  };

  const handleSnapshot = () => {
    createSnapshot.mutate(snapshotLabel, {
      onSuccess: () => { swalToast('Snapshot berhasil dibuat!'); setSnapshotOpen(false); setSnapshotLabel(''); },
      onError: (err: unknown) => swalToast(apiErrorMessage(err, 'Gagal'), 'error'),
    });
  };

  const handleRollback = (sid: number) => {
    swalConfirm('Rollback ke snapshot ini?', 'Persona & knowledge akan dikembalikan ke versi snapshot. Knowledge hasil learning akan dihapus.')
      .then((ok: boolean) => {
        if (!ok) return;
        rollback.mutate(sid, {
          onSuccess: () => swalToast('Rollback berhasil! Persona & knowledge dikembalikan.'),
          onError: (err: unknown) => swalToast(apiErrorMessage(err, 'Gagal'), 'error'),
        });
      });
  };

  const handleToggleEnabled = (enabled: boolean) => {
    saveConfig.mutate({ enabled });
  };

  const handleToggleAutoApply = (autoApply: boolean) => {
    saveConfig.mutate({ auto_apply: autoApply });
  };

  if (isLoading) return <Box sx={{ p: 4, textAlign: 'center' }}><CircularProgress /></Box>;

  return (
    <Box>
      <PageHeader
        title="AI Learning Engine"
        subtitle="AI belajar dari percakapan CS manusia untuk meningkatkan closing rate"
      />

      {/* Status bar */}
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Grid container spacing={2} sx={{ alignItems: 'center' }}>
          <Grid size={{ xs: 12, sm: 3 }}>
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
              <AutoFixHighIcon color={status?.last_run ? 'success' : 'disabled'} />
              <Box>
                <Typography variant="caption" color="text.secondary">Learning terakhir</Typography>
                <Typography variant="body2" sx={{ fontWeight: 700 }}>
                  {status?.last_run ? new Date(status.last_run.created_at).toLocaleDateString('id-ID') : 'Belum pernah'}
                </Typography>
              </Box>
            </Stack>
          </Grid>
          <Grid size={{ xs: 6, sm: 2 }}>
            <Typography variant="caption" color="text.secondary">Pola Ditemukan</Typography>
            <Typography variant="h6">{status?.patterns_suggested || 0}</Typography>
          </Grid>
          <Grid size={{ xs: 6, sm: 2 }}>
            <Typography variant="caption" color="text.secondary">Diterapkan</Typography>
            <Typography variant="h6" color="success.main">{status?.patterns_applied || 0}</Typography>
          </Grid>
          <Grid size={{ xs: 6, sm: 2 }}>
            <Typography variant="caption" color="text.secondary">Snapshot</Typography>
            <Typography variant="h6">{status?.snapshot_count || 0}</Typography>
          </Grid>
          <Grid size={{ xs: 6, sm: 3 }}>
            <FormControlLabel
              control={<Switch checked={config?.enabled || false} onChange={(_, v) => handleToggleEnabled(v)} disabled={saveConfig.isPending} />}
              label={<Typography variant="body2">{config?.enabled ? 'Learning ON' : 'Learning OFF'}</Typography>}
            />
          </Grid>
        </Grid>
      </Paper>

      {status?.last_run && (
        <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
          <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
            {status.last_run.status === 'running' || status.last_run.status === 'pending' ? (
              <>
                <CircularProgress size={16} />
                <Typography variant="body2" sx={{ fontWeight: 700 }}>
                  Learning sedang berjalan… pola muncul otomatis di tab "Pola" bila selesai.
                </Typography>
              </>
            ) : status.last_run.status === 'completed' ? (
              <>
                <Typography variant="body2" sx={{ fontWeight: 700, color: 'success.main' }}>
                  ✅ Selesai — {status.last_run.pattern_count} pola ditemukan
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  dari {status.last_run.human_chats} chat CS · {status.last_run.total_chats} chat total
                </Typography>
              </>
            ) : (
              <>
                <Typography variant="body2" sx={{ fontWeight: 700, color: 'error.main' }}>
                  ❌ Gagal
                </Typography>
                <Typography variant="body2" color="error">
                  {status.last_run.error || 'Terjadi kesalahan. Coba rentang tanggal lain lalu jalankan ulang.'}
                </Typography>
              </>
            )}
          </Stack>
        </Paper>
      )}

      {score && (
        <Paper variant="outlined" sx={{ p: 2, mb: 2, background: 'linear-gradient(135deg, rgba(33,150,243,0.06), rgba(76,175,80,0.06))' }}>
          <Stack direction="row" spacing={2} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
            <Box sx={{ minWidth: 130 }}>
              <Typography variant="caption" color="text.secondary">Skor Agent</Typography>
              <Typography variant="h4" sx={{ fontWeight: 800, color: score.score >= 70 ? 'success.main' : score.score >= 40 ? 'warning.main' : 'error.main' }}>
                {score.score.toFixed(0)}<Typography component="span" variant="h6">/100</Typography>
              </Typography>
            </Box>
            <Divider orientation="vertical" flexItem />
            <Box>
              <Typography variant="caption" color="text.secondary">Close rate 30 hari</Typography>
              <Typography variant="h6">{score.close_rate_pct.toFixed(1)}% <Typography component="span" variant="caption" color="text.secondary">({score.closing_contacts} closing / {score.active_contacts} kontak aktif)</Typography></Typography>
            </Box>
            <Box>
              <Typography variant="caption" color="text.secondary">Dampak pola terpelajar</Typography>
              <Typography variant="h6">{(score.avg_closing_impact * 100).toFixed(0)}% <Typography component="span" variant="caption" color="text.secondary">({score.patterns_applied} diterapkan · {score.patterns_pending} menunggu review)</Typography></Typography>
            </Box>
          </Stack>
        </Paper>
      )}

      <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2 }}>
        <Tab icon={<AutoFixHighIcon />} iconPosition="start" label="Jalankan" />
        <Tab icon={<LightbulbIcon />} iconPosition="start" label={`Pola (${patterns?.length || 0})`} />
        <Tab icon={<HistoryIcon />} iconPosition="start" label={`Versi (${snapshots?.length || 0})`} />
        <Tab icon={<SettingsIcon />} iconPosition="start" label="Konfigurasi" />
      </Tabs>

      {/* Tab 0: Jalankan Learning */}
      {tab === 0 && (
        <Stack spacing={2}>
          <Paper variant="outlined" sx={{ p: 3 }}>
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 800 }}>🔄 Jalankan Pembelajaran AI</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              AI akan menganalisa chat CS manusia (balasan dari WhatsApp terhubung) dan mengekstrak pola gaya bahasa, teknik closing, dan frasa efektif.
              Hasilnya bisa kamu review sebelum diterapkan.
            </Typography>
            <Grid container spacing={2} sx={{ mb: 2 }}>
              <Grid size={{ xs: 6, sm: 3 }}>
                <TextField label="Dari tanggal" type="date" size="small" fullWidth
                  value={startDate} onChange={(e) => setStartDate(e.target.value)}
                  slotProps={{ inputLabel: { shrink: true } }}
                  helperText="Default: 30 hari lalu" />
              </Grid>
              <Grid size={{ xs: 6, sm: 3 }}>
                <TextField label="Sampai tanggal" type="date" size="small" fullWidth
                  value={endDate} onChange={(e) => setEndDate(e.target.value)}
                  slotProps={{ inputLabel: { shrink: true } }}
                  helperText="Termasuk sepanjang hari ini" />
              </Grid>
              <Grid size={{ xs: 12, sm: 6 }} sx={{ display: 'flex', alignItems: 'flex-end' }}>
                <Button variant="contained" size="large" startIcon={startLearning.isPending ? <CircularProgress size={18} /> : <PlayArrowIcon />}
                  onClick={handleRunLearning} disabled={busy}>
                  {startLearning.isPending ? 'Menganalisa...' : 'Mulai Learning'}
                </Button>
              </Grid>
            </Grid>
            <Typography variant="caption" color="text.secondary">
              * Kosongkan tanggal untuk menganalisa semua chat CS manusia yang tersedia.
            </Typography>
          </Paper>

          {status?.last_run?.summary && (
            <Paper variant="outlined" sx={{ p: 2, bgcolor: 'rgba(76,175,80,0.06)', borderColor: 'success.light' }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 800, mb: 0.5 }}>📋 Rekap Learning Terakhir</Typography>
              <Typography variant="body2" color="text.secondary">{status.last_run.summary}</Typography>
            </Paper>
          )}
        </Stack>
      )}

      {/* Tab 1: Patterns */}
      {tab === 1 && (
        <Stack spacing={2}>
          {patterns && patterns.length > 0 && (
            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
              <Typography variant="body2" color="text.secondary">{patterns.length} pola menunggu review</Typography>
              <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                <Typography variant="caption">Min confidence:</Typography>
                <Select size="small" value={applyMinConf} onChange={(e) => setApplyMinConf(e.target.value as number)}
                  sx={{ minWidth: 100 }}>
                  {[0.4, 0.5, 0.6, 0.7, 0.8, 0.9].map(v => (
                    <MenuItem key={v} value={v}>{(v * 100).toFixed(0)}%</MenuItem>
                  ))}
                </Select>
                <Button variant="outlined" size="small" onClick={handleApplyAll} disabled={busy}>
                  Terapkan Semua
                </Button>
              </Stack>
            </Stack>
          )}
          {!patterns || patterns.length === 0 ? (
            <Paper variant="outlined" sx={{ p: 4, textAlign: 'center' }}>
              <LightbulbIcon sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
              <Typography color="text.secondary">Belum ada pola yang ditemukan.</Typography>
              <Typography variant="body2" color="text.disabled">Jalankan learning dulu dari tab "Jalankan".</Typography>
            </Paper>
          ) : (
            patterns.map(p => (
              <PatternCard key={p.id} pattern={p}
                onApply={() => handleApply(p.id)}
                onReject={() => handleReject(p.id)}
                loading={busy} />
            ))
          )}
        </Stack>
      )}

      {/* Tab 2: Snapshots */}
      {tab === 2 && (
        <Stack spacing={2}>
          <Stack direction="row" spacing={1} sx={{ justifyContent: 'space-between', alignItems: 'center' }}>
            <Typography variant="body2" color="text.secondary">
              {snapshots?.length || 0} versi backup tersimpan
            </Typography>
            <Button variant="outlined" size="small" startIcon={<SaveIcon />}
              onClick={() => setSnapshotOpen(true)}>Buat Snapshot</Button>
          </Stack>
          {!snapshots || snapshots.length === 0 ? (
            <Paper variant="outlined" sx={{ p: 4, textAlign: 'center' }}>
              <StorageIcon sx={{ fontSize: 48, color: 'text.disabled', mb: 1 }} />
              <Typography color="text.secondary">Belum ada snapshot.</Typography>
              <Typography variant="body2" color="text.disabled">
                Buat snapshot sebelum menjalankan learning agar bisa rollback jika hasil tidak sesuai.
              </Typography>
            </Paper>
          ) : (
            snapshots.map(s => (
              <Paper key={s.id} variant="outlined" sx={{ p: 2 }}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <HistoryIcon color="action" />
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 700 }}>{s.label}</Typography>
                      <Typography variant="caption" color="text.secondary">
                        {new Date(s.created_at).toLocaleString('id-ID')} · {s.knowledge_count} knowledge · persona tersimpan
                      </Typography>
                    </Box>
                  </Stack>
                  <Button size="small" color="warning" variant="outlined" startIcon={<RestoreIcon />}
                    onClick={() => handleRollback(s.id)} disabled={busy}>
                    Rollback
                  </Button>
                </Stack>
              </Paper>
            ))
          )}
        </Stack>
      )}

      {/* Tab 3: Config */}
      {tab === 3 && config && (
        <Stack spacing={2}>
          <Paper variant="outlined" sx={{ p: 2 }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 800, mb: 2 }}>⚙️ Konfigurasi Learning</Typography>

            <Stack spacing={2}>
              <FormControlLabel
                control={<Switch checked={config.enabled} onChange={(_, v) => handleToggleEnabled(v)} />}
                label="Aktifkan learning engine"
              />
              <FormControlLabel
                control={<Switch checked={config.auto_apply} onChange={(_, v) => handleToggleAutoApply(v)}
                  disabled={!config.enabled} />}
                label="Auto-apply pola (otomatis terapkan tanpa review)"
              />

              <Box>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  Min confidence untuk auto-apply: <strong>{(config.min_confidence * 100).toFixed(0)}%</strong>
                </Typography>
                <Slider value={config.min_confidence} min={0.3} max={0.95} step={0.05}
                  onChange={(_, v) => saveConfig.mutate({ min_confidence: v as number })}
                  disabled={!config.enabled} />
              </Box>

              <Box>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  Min pemakaian oleh CS manusia: <strong>{config.min_usage_count}x</strong>
                </Typography>
                <Slider value={config.min_usage_count} min={1} max={20} step={1}
                  onChange={(_, v) => saveConfig.mutate({ min_usage_count: v as number })}
                  disabled={!config.enabled} />
              </Box>

              <Box>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  Maks pola per sesi: <strong>{config.max_patterns_per_run}</strong>
                </Typography>
                <Slider value={config.max_patterns_per_run} min={3} max={30} step={1}
                  onChange={(_, v) => saveConfig.mutate({ max_patterns_per_run: v as number })}
                  disabled={!config.enabled} />
              </Box>

              <FormControlLabel
                control={<Switch checked={config.preserve_manual_knowledge}
                  onChange={(_, v) => saveConfig.mutate({ preserve_manual_knowledge: v })} />}
                label="Jangan timpa knowledge manual (hanya tambahan)"
              />

              <Divider />

              <FormControlLabel
                control={<Switch checked={config.schedule_enabled}
                  onChange={(_, v) => saveConfig.mutate({ schedule_enabled: v })} />}
                label="Jadwalkan learning otomatis"
              />
              {config.schedule_enabled && (
                <TextField label="Cron schedule" size="small" value={config.schedule_cron}
                  onChange={(e) => saveConfig.mutate({ schedule_cron: e.target.value })}
                  helperText="Contoh: 0 2 * * * (jam 2 pagi setiap hari)" />
              )}

              <Box>
                <Typography variant="body2">
                  Analisa chat dalam <strong>{config.lookback_days} hari</strong> terakhir
                </Typography>
                <Slider value={config.lookback_days} min={7} max={90} step={1}
                  onChange={(_, v) => saveConfig.mutate({ lookback_days: v as number })} />
              </Box>

              <Divider />

              <TextField label="Label closing WhatsApp (dipisah koma)" size="small" fullWidth
                value={config.closing_labels ?? ''}
                onChange={(e) => setClosingLabels(e.target.value)}
                onBlur={() => saveConfig.mutate({ closing_labels: closingLabels })}
                helperText="Kontak yang diberi salah satu label ini oleh CS dihitung closing — dipakai skor agent & materi belajar." />
              {waLabels && waLabels.length > 0 && (
                <Box>
                  <Typography variant="caption" color="text.secondary">
                    Label asli WhatsApp di perangkat ({waLabels.length}):
                  </Typography>
                  <Stack direction="row" spacing={0.5} sx={{ mt: 0.5, flexWrap: 'wrap', rowGap: 0.5 }}>
                    {waLabels.slice(0, 24).map((l) => (
                      <Chip key={l.label_id} size="small" label={`${l.name} (${l.count})`}
                        onClick={() => { setClosingLabels((v) => (v ? v + ', ' : '') + l.name); saveConfig.mutate({ closing_labels: (config.closing_labels ? config.closing_labels + ', ' : '') + l.name }); }}
                        title="Klik untuk menambahkan sebagai label closing" />
                    ))}
                  </Stack>
                </Box>
              )}
            </Stack>
          </Paper>
        </Stack>
      )}

      {/* Snapshot dialog */}
      <Dialog open={snapshotOpen} onClose={() => setSnapshotOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Buat Snapshot Versi</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Snapshot akan menyimpan persona dan semua knowledge saat ini. Berguna untuk rollback jika hasil learning tidak sesuai.
          </Typography>
          <TextField label="Label (opsional)" fullWidth value={snapshotLabel}
            onChange={(e) => setSnapshotLabel(e.target.value)}
            placeholder={`Backup ${new Date().toLocaleDateString('id-ID')}`} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSnapshotOpen(false)}>Batal</Button>
          <Button variant="contained" onClick={handleSnapshot} disabled={createSnapshot.isPending}>
            Simpan Snapshot
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
