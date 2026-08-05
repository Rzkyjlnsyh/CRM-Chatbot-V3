import { useEffect, useMemo, useState, type ReactElement, type ReactNode } from 'react';
import {
  alpha, Box, Typography, Card, CardContent, Button, Stack, Chip, TextField, Alert,
  CircularProgress, IconButton, Tooltip, Divider, Skeleton,
} from '@mui/material';
import AttachFileIcon from '@mui/icons-material/AttachFile';
import CloseIcon from '@mui/icons-material/Close';
import DeleteOutlinedIcon from '@mui/icons-material/DeleteOutlined';
import SendIcon from '@mui/icons-material/Send';
import ScheduleIcon from '@mui/icons-material/ScheduleOutlined';
import AutoStoriesIcon from '@mui/icons-material/AutoStoriesOutlined';
import ImageOutlinedIcon from '@mui/icons-material/ImageOutlined';
import BoltOutlinedIcon from '@mui/icons-material/BoltOutlined';
import EventAvailableOutlinedIcon from '@mui/icons-material/EventAvailableOutlined';
import CheckCircleOutlinedIcon from '@mui/icons-material/CheckCircleOutlined';
import ErrorOutlinedIcon from '@mui/icons-material/ErrorOutlined';
import HourglassEmptyIcon from '@mui/icons-material/HourglassEmpty';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import VisibilityOutlinedIcon from '@mui/icons-material/VisibilityOutlined';
import { useStatuses, useCreateStatus, useCancelStatus } from '../hooks';
import WhatsAppEditor from './WhatsAppEditor';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';
import { swalConfirm, swalToast } from '../services/swal';
import type { ScheduledStatus } from '../types';

/** Batas nyaman untuk teks Status WhatsApp (UI helper, bukan hard-block server). */
const TEXT_SOFT_LIMIT = 700;

type HistoryFilter = 'all' | 'scheduled' | 'done' | 'failed' | 'other';

const STATUS_COLOR: Record<string, 'success' | 'warning' | 'error' | 'default' | 'info'> = {
  scheduled: 'warning',
  running: 'info',
  done: 'success',
  failed: 'error',
  cancelled: 'default',
  interrupted: 'error',
};

const STATUS_LABEL: Record<string, string> = {
  scheduled: 'Terjadwal',
  running: 'Sedang diposting',
  done: 'Terposting',
  failed: 'Gagal',
  cancelled: 'Dibatalkan',
  interrupted: 'Tertunda',
};

const STATUS_ICON: Record<string, ReactNode> = {
  scheduled: <ScheduleIcon sx={{ fontSize: 16 }} />,
  running: <HourglassEmptyIcon sx={{ fontSize: 16 }} />,
  done: <CheckCircleOutlinedIcon sx={{ fontSize: 16 }} />,
  failed: <ErrorOutlinedIcon sx={{ fontSize: 16 }} />,
  cancelled: <CloseIcon sx={{ fontSize: 16 }} />,
  interrupted: <ErrorOutlinedIcon sx={{ fontSize: 16 }} />,
};

const pad = (n: number) => String(n).padStart(2, '0');

function defaultScheduleValue() {
  const d = new Date(Date.now() + 60 * 60 * 1000);
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fmtRunAt(iso: string) {
  const d = new Date(iso);
  return d.toLocaleString('id-ID', {
    weekday: 'short',
    day: '2-digit',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function relativeLabel(iso: string) {
  const t = new Date(iso).getTime();
  const diff = t - Date.now();
  const abs = Math.abs(diff);
  const mins = Math.round(abs / 60000);
  const hours = Math.round(abs / 3600000);
  const days = Math.round(abs / 86400000);

  if (diff > 0) {
    if (mins < 60) return `dalam ${mins || 1} mnt`;
    if (hours < 48) return `dalam ${hours} jam`;
    return `dalam ${days} hari`;
  }
  if (mins < 1) return 'baru saja';
  if (mins < 60) return `${mins} mnt lalu`;
  if (hours < 48) return `${hours} jam lalu`;
  return `${days} hari lalu`;
}

function accentColor(status: string) {
  if (status === 'done') return 'success.main';
  if (status === 'failed' || status === 'interrupted') return 'error.main';
  if (status === 'running') return 'info.main';
  if (status === 'cancelled') return 'text.disabled';
  return 'warning.main';
}

function SectionHeader({ icon, title, subtitle }: { icon: ReactNode; title: string; subtitle?: string }) {
  return (
    <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start', mb: 1.25 }}>
      <Box
        sx={{
          width: 28,
          height: 28,
          display: 'grid',
          placeItems: 'center',
          borderRadius: 0.75,
          bgcolor: 'action.hover',
          color: 'primary.main',
          flexShrink: 0,
          mt: 0.1,
        }}
      >
        {icon}
      </Box>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 600, lineHeight: 1.3 }}>
          {title}
        </Typography>
        {subtitle && (
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.4 }}>
            {subtitle}
          </Typography>
        )}
      </Box>
    </Stack>
  );
}

function StatPill({
  label,
  value,
  tone = 'default',
  active,
  onClick,
}: {
  label: string;
  value: number;
  tone?: 'default' | 'success' | 'warning' | 'error' | 'info';
  active?: boolean;
  onClick?: () => void;
}) {
  const colorMap = {
    default: 'text.secondary',
    success: 'success.main',
    warning: 'warning.main',
    error: 'error.main',
    info: 'info.main',
  } as const;

  return (
    <Box
      component="button"
      type="button"
      onClick={onClick}
      sx={{
        appearance: 'none',
        border: '1px solid',
        borderColor: active ? 'primary.main' : 'divider',
        bgcolor: active ? (theme) => alpha(theme.palette.primary.main, 0.06) : 'background.paper',
        borderRadius: 0.75,
        px: 1.25,
        py: 0.75,
        minWidth: 76,
        textAlign: 'left',
        cursor: onClick ? 'pointer' : 'default',
        '&:hover': onClick
          ? { borderColor: 'primary.light', bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04) }
          : undefined,
      }}
    >
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.2 }}>
        {label}
      </Typography>
      <Typography sx={{ fontWeight: 600, fontSize: 16, color: colorMap[tone], lineHeight: 1.2, mt: 0.15 }}>
        {value}
      </Typography>
    </Box>
  );
}

function ModeCard({
  selected,
  icon,
  title,
  description,
  onClick,
}: {
  selected: boolean;
  icon: ReactNode;
  title: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <Box
      component="button"
      type="button"
      onClick={onClick}
      sx={{
        appearance: 'none',
        flex: 1,
        minWidth: 0,
        textAlign: 'left',
        border: '1px solid',
        borderColor: selected ? 'primary.main' : 'divider',
        bgcolor: selected ? (theme) => alpha(theme.palette.primary.main, 0.06) : 'background.paper',
        borderRadius: 0.75,
        p: 1.25,
        cursor: 'pointer',
        '&:hover': {
          borderColor: selected ? 'primary.main' : 'primary.light',
          bgcolor: (theme) => alpha(theme.palette.primary.main, selected ? 0.08 : 0.03),
        },
      }}
    >
      <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
        <Box
          sx={{
            width: 24,
            height: 24,
            borderRadius: 0.5,
            display: 'grid',
            placeItems: 'center',
            bgcolor: selected ? 'primary.main' : 'action.hover',
            color: selected ? 'primary.contrastText' : 'text.secondary',
            flexShrink: 0,
          }}
        >
          {icon}
        </Box>
        <Box sx={{ minWidth: 0 }}>
          <Typography variant="body2" sx={{ fontWeight: 600, color: selected ? 'primary.dark' : 'text.primary' }}>
            {title}
          </Typography>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.4, mt: 0.15 }}>
            {description}
          </Typography>
        </Box>
      </Stack>
    </Box>
  );
}

function StatusPreview({
  text,
  imageUrl,
  mode,
  runAt,
}: {
  text: string;
  imageUrl: string | null;
  mode: 'now' | 'schedule';
  runAt: string;
}) {
  const hasContent = Boolean(text.trim() || imageUrl);
  const displayText = text.trim() || (imageUrl ? '' : 'Teks status akan tampil di sini…');

  return (
    <Box
      sx={{
        borderRadius: 1,
        border: '1px solid',
        borderColor: 'divider',
        bgcolor: '#0B141A',
        overflow: 'hidden',
        position: 'relative',
        minHeight: 280,
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      {/* Header pratinjau */}
      <Stack direction="row" spacing={1} sx={{ alignItems: 'center', px: 1.25, pt: 1.25, pb: 0.75, zIndex: 2 }}>
        <Box
          sx={{
            width: 28,
            height: 28,
            borderRadius: 0.75,
            bgcolor: '#1F2C34',
            border: '1px solid rgba(255,255,255,0.12)',
            display: 'grid',
            placeItems: 'center',
            color: '#E9EDEF',
            fontSize: 11,
            fontWeight: 600,
          }}
        >
          WA
        </Box>
        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Typography sx={{ color: '#E9EDEF', fontSize: 12.5, fontWeight: 600, lineHeight: 1.2 }}>
            Status bisnis Anda
          </Typography>
          <Typography sx={{ color: 'rgba(233,237,239,0.65)', fontSize: 11, lineHeight: 1.2, mt: 0.15 }}>
            {mode === 'now' ? 'Tampil sekarang · 24 jam' : `Terjadwal · ${runAt ? fmtRunAt(new Date(runAt).toISOString()) : '—'}`}
          </Typography>
        </Box>
        <VisibilityOutlinedIcon sx={{ color: 'rgba(233,237,239,0.55)', fontSize: 16 }} />
      </Stack>

      <Box sx={{ mx: 1.25, mb: 0.75, height: 2, bgcolor: 'rgba(255,255,255,0.75)', zIndex: 2 }} />

      <Box
        sx={{
          flex: 1,
          position: 'relative',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          px: 1.5,
          py: 2.5,
          bgcolor: imageUrl ? undefined : '#128C7E',
        }}
      >
        {imageUrl && (
          <Box
            component="img"
            src={imageUrl}
            alt="Pratinjau status"
            sx={{
              position: 'absolute',
              inset: 0,
              width: '100%',
              height: '100%',
              objectFit: 'cover',
            }}
          />
        )}
        {imageUrl && (
          <Box
            sx={{
              position: 'absolute',
              inset: 0,
              background: 'linear-gradient(180deg, rgba(0,0,0,0.25) 0%, rgba(0,0,0,0.15) 40%, rgba(0,0,0,0.55) 100%)',
            }}
          />
        )}
        <Typography
          sx={{
            position: 'relative',
            zIndex: 1,
            color: '#fff',
            textAlign: 'center',
            fontSize: hasContent && text.trim() ? 16 : 14,
            fontWeight: text.trim() ? 600 : 400,
            lineHeight: 1.45,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            maxWidth: '100%',
            opacity: text.trim() ? 1 : 0.72,
            textShadow: imageUrl ? '0 1px 8px rgba(0,0,0,0.55)' : 'none',
            px: 0.5,
          }}
        >
          {displayText}
        </Typography>
      </Box>

      <Box sx={{ px: 1.25, py: 0.85, bgcolor: 'rgba(0,0,0,0.3)', borderTop: '1px solid rgba(255,255,255,0.06)' }}>
        <Typography sx={{ color: 'rgba(233,237,239,0.65)', fontSize: 11, textAlign: 'center' }}>
          Pratinjau · tampilan di HP bisa berbeda
        </Typography>
      </Box>
    </Box>
  );
}

export default function StatusPanel({ agentId }: { agentId: number }) {
  const { data: statuses = [], isLoading, isFetching } = useStatuses(agentId);
  const createStatus = useCreateStatus(agentId);
  const cancelStatus = useCancelStatus(agentId);

  const [text, setText] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [mode, setMode] = useState<'now' | 'schedule'>('now');
  const [runAt, setRunAt] = useState(defaultScheduleValue());
  const [error, setError] = useState('');
  const [filter, setFilter] = useState<HistoryFilter>('all');

  useEffect(() => {
    if (!file) {
      setPreviewUrl(null);
      return;
    }
    const url = URL.createObjectURL(file);
    setPreviewUrl(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  const counts = useMemo(() => {
    const scheduled = statuses.filter((s) => s.status === 'scheduled' || s.status === 'running').length;
    const done = statuses.filter((s) => s.status === 'done').length;
    const failed = statuses.filter((s) => s.status === 'failed' || s.status === 'interrupted').length;
    return { all: statuses.length, scheduled, done, failed };
  }, [statuses]);

  const filtered = useMemo(() => {
    if (filter === 'all') return statuses;
    if (filter === 'scheduled') return statuses.filter((s) => s.status === 'scheduled' || s.status === 'running');
    if (filter === 'done') return statuses.filter((s) => s.status === 'done');
    if (filter === 'failed') return statuses.filter((s) => s.status === 'failed' || s.status === 'interrupted');
    return statuses.filter((s) => s.status === 'cancelled');
  }, [statuses, filter]);

  const charCount = text.length;
  const overSoftLimit = charCount > TEXT_SOFT_LIMIT;
  const canSubmit = Boolean(text.trim() || file) && !createStatus.isPending;

  const pickFile = (f: File | null) => {
    if (!f) {
      setFile(null);
      return;
    }
    if (!f.type.startsWith('image/')) {
      setError('Hanya file gambar yang didukung untuk Status.');
      return;
    }
    // ~5MB soft guard agar UX lebih ramah sebelum upload
    if (f.size > 5 * 1024 * 1024) {
      setError('Ukuran gambar terlalu besar. Gunakan file di bawah 5 MB.');
      return;
    }
    setFile(f);
    if (error) setError('');
  };

  const submit = async () => {
    setError('');
    if (!text.trim() && !file) {
      setError('Isi teks atau lampirkan gambar dulu.');
      return;
    }
    if (mode === 'schedule') {
      if (!runAt) {
        setError('Pilih waktu jadwal.');
        return;
      }
      if (new Date(runAt).getTime() < Date.now() - 60000) {
        setError('Waktu jadwal sudah lewat. Pilih waktu di masa depan.');
        return;
      }
    }
    const when =
      mode === 'now'
        ? 'Posting status sekarang?'
        : `Jadwalkan status untuk ${fmtRunAt(new Date(runAt).toISOString())}?`;
    const ok = await swalConfirm(
      when,
      'Status tampil 24 jam dan dilihat kontak yang menyimpan nomormu. Tidak ada pesan japri yang dikirim.',
    );
    if (!ok) return;

    const fd = new FormData();
    fd.append('text', text);
    if (file) fd.append('file', file);
    if (mode === 'schedule') fd.append('run_at', new Date(runAt).toISOString());
    try {
      await createStatus.mutateAsync(fd);
      setText('');
      setFile(null);
      swalToast(mode === 'now' ? 'Status berhasil diposting.' : 'Status dijadwalkan.');
    } catch (e) {
      const detail = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setError(detail || 'Status belum bisa diproses. Periksa koneksi WhatsApp.');
    }
  };

  const doCancel = async (s: ScheduledStatus) => {
    if (!(await swalConfirm('Batalkan status terjadwal ini?', 'Status tidak akan diposting.'))) return;
    try {
      await cancelStatus.mutateAsync(s.id);
      swalToast('Jadwal status dibatalkan.');
    } catch {
      swalToast('Gagal membatalkan status.', 'error');
    }
  };

  if (isLoading) {
    return (
      <Box>
        <PageHeader
          title="Status / Story"
          subtitle="Bagikan promo atau update bisnis ke kontak yang menyimpan nomor WhatsApp Anda."
        />
        <Stack spacing={2}>
          <Skeleton variant="rounded" height={220} />
          <Skeleton variant="rounded" height={160} />
        </Stack>
      </Box>
    );
  }

  return (
    <Box>
      <PageHeader
        title="Status / Story"
        subtitle="Bagikan promo atau update bisnis ke kontak yang menyimpan nomor WhatsApp Anda — tanpa kirim pesan japri."
      />

      <Alert
        severity="info"
        icon={<InfoOutlinedIcon fontSize="inherit" />}
        sx={{ mb: 2, borderRadius: 1 }}
      >
        Status hanya terlihat oleh orang yang menyimpan nomor Anda, tampil 24 jam, dan tidak mengirim chat masuk.
        Makin banyak pelanggan simpan nomor, makin luas jangkauannya.
      </Alert>

      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: { xs: '1fr', lg: 'minmax(0, 1.35fr) minmax(280px, 0.85fr)' },
          gap: 2,
          mb: 2.5,
          alignItems: 'start',
        }}
      >
        {/* Composer */}
        <Card variant="outlined" sx={{ borderRadius: 1 }}>
          <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
            <SectionHeader
              icon={<AutoStoriesIcon fontSize="small" />}
              title="Buat Status baru"
              subtitle="Tulis teks, tambah gambar opsional, lalu posting sekarang atau jadwalkan."
            />

            <Stack spacing={2}>
              <Box>
                <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'baseline', mb: 0.75 }}>
                  <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                    Isi Status
                  </Typography>
                  <Typography
                    variant="caption"
                    sx={{
                      fontWeight: 600,
                      color: overSoftLimit ? 'warning.main' : 'text.secondary',
                    }}
                  >
                    {charCount}/{TEXT_SOFT_LIMIT}
                  </Typography>
                </Stack>
                <WhatsAppEditor
                  value={text}
                  onChange={(v) => {
                    setText(v);
                    if (error) setError('');
                  }}
                  placeholder="Contoh: Promo hari ini — diskon 20% untuk semua produk 🎉&#10;Berlaku sampai jam 21.00"
                  rows={5}
                />
                {overSoftLimit && (
                  <Typography variant="caption" color="warning.main" sx={{ display: 'block', mt: 0.75 }}>
                    Teks agak panjang. Status WhatsApp paling nyaman dibaca di bawah {TEXT_SOFT_LIMIT} karakter.
                  </Typography>
                )}
              </Box>

              {/* Media drop / preview */}
              <Box>
                <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.75 }}>
                  Gambar (opsional)
                </Typography>
                {previewUrl ? (
                  <Box
                    sx={{
                      position: 'relative',
                      borderRadius: 1,
                      border: '1px solid',
                      borderColor: 'divider',
                      overflow: 'hidden',
                      bgcolor: 'action.hover',
                    }}
                  >
                    <Box
                      component="img"
                      src={previewUrl}
                      alt={file?.name || 'Pratinjau gambar'}
                      sx={{
                        display: 'block',
                        width: '100%',
                        maxHeight: 180,
                        objectFit: 'cover',
                      }}
                    />
                    <Stack
                      direction="row"
                      spacing={0.75}
                      sx={{ position: 'absolute', top: 6, right: 6 }}
                    >
                      <Button
                        component="label"
                        size="small"
                        variant="contained"
                        color="inherit"
                        sx={{
                          bgcolor: 'rgba(255,255,255,0.92)',
                          color: 'text.primary',
                          minHeight: 28,
                          '&:hover': { bgcolor: '#fff' },
                        }}
                      >
                        Ganti
                        <input
                          type="file"
                          hidden
                          accept="image/*"
                          onChange={(e) => pickFile(e.target.files?.[0] || null)}
                        />
                      </Button>
                      <IconButton
                        size="small"
                        onClick={() => pickFile(null)}
                        aria-label="Hapus gambar"
                        sx={{
                          bgcolor: 'rgba(0,0,0,0.5)',
                          color: '#fff',
                          borderRadius: 0.75,
                          '&:hover': { bgcolor: 'rgba(0,0,0,0.65)' },
                        }}
                      >
                        <CloseIcon fontSize="small" />
                      </IconButton>
                    </Stack>
                    {file && (
                      <Box
                        sx={{
                          position: 'absolute',
                          left: 0,
                          right: 0,
                          bottom: 0,
                          px: 1,
                          py: 0.5,
                          bgcolor: 'rgba(0,0,0,0.5)',
                        }}
                      >
                        <Typography variant="caption" noWrap sx={{ color: '#fff', display: 'block' }}>
                          {file.name} · {(file.size / 1024).toFixed(0)} KB
                        </Typography>
                      </Box>
                    )}
                  </Box>
                ) : (
                  <Box
                    component="label"
                    sx={{
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'center',
                      justifyContent: 'center',
                      gap: 0.5,
                      minHeight: 88,
                      px: 1.5,
                      py: 1.75,
                      borderRadius: 1,
                      border: '1px dashed',
                      borderColor: 'divider',
                      bgcolor: 'background.paper',
                      cursor: 'pointer',
                      '&:hover': {
                        borderColor: 'primary.main',
                        bgcolor: (theme) => alpha(theme.palette.primary.main, 0.03),
                      },
                    }}
                  >
                    <ImageOutlinedIcon sx={{ fontSize: 22, color: 'text.secondary' }} />
                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                      Klik untuk unggah gambar
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                      JPG, PNG, WebP · maks. 5 MB
                    </Typography>
                    <input
                      type="file"
                      hidden
                      accept="image/*"
                      onChange={(e) => pickFile(e.target.files?.[0] || null)}
                    />
                  </Box>
                )}
              </Box>

              <Box>
                <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.75 }}>
                  Kapan diposting?
                </Typography>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                  <ModeCard
                    selected={mode === 'now'}
                    icon={<BoltOutlinedIcon sx={{ fontSize: 15 }} />}
                    title="Sekarang"
                    description="Langsung tampil di Status WhatsApp"
                    onClick={() => {
                      setMode('now');
                      if (error) setError('');
                    }}
                  />
                  <ModeCard
                    selected={mode === 'schedule'}
                    icon={<EventAvailableOutlinedIcon sx={{ fontSize: 15 }} />}
                    title="Jadwalkan"
                    description="Pilih tanggal & jam posting otomatis"
                    onClick={() => {
                      setMode('schedule');
                      if (error) setError('');
                    }}
                  />
                </Stack>

                {mode === 'schedule' && (
                  <Box
                    sx={{
                      mt: 1.25,
                      p: 1.25,
                      borderRadius: 1,
                      border: '1px solid',
                      borderColor: 'divider',
                      bgcolor: 'action.hover',
                    }}
                  >
                    <TextField
                      type="datetime-local"
                      size="small"
                      label="Waktu posting"
                      value={runAt}
                      onChange={(e) => {
                        setRunAt(e.target.value);
                        if (error) setError('');
                      }}
                      slotProps={{ inputLabel: { shrink: true } }}
                      fullWidth
                      sx={{ maxWidth: 320, bgcolor: 'background.paper' }}
                    />
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.75 }}>
                      {runAt
                        ? `Akan diposting ${fmtRunAt(new Date(runAt).toISOString())} (${relativeLabel(new Date(runAt).toISOString())}).`
                        : 'Pilih waktu di masa depan.'}
                    </Typography>
                  </Box>
                )}
              </Box>

              {error && (
                <Alert severity="error" onClose={() => setError('')}>
                  {error}
                </Alert>
              )}

              <Divider />

              <Stack
                direction={{ xs: 'column', sm: 'row' }}
                spacing={1.25}
                sx={{ alignItems: { sm: 'center' }, justifyContent: 'space-between' }}
              >
                <Typography variant="caption" color="text.secondary" sx={{ lineHeight: 1.45 }}>
                  {mode === 'now'
                    ? 'Butuh WhatsApp tersambung untuk posting langsung.'
                    : 'Jadwal tetap diproses saat agent online pada waktunya.'}
                </Typography>
                <Button
                  variant="contained"
                  size="medium"
                  startIcon={
                    createStatus.isPending ? (
                      <CircularProgress size={16} color="inherit" />
                    ) : mode === 'now' ? (
                      <SendIcon />
                    ) : (
                      <ScheduleIcon />
                    )
                  }
                  onClick={submit}
                  disabled={!canSubmit}
                  sx={{ minWidth: { sm: 180 }, alignSelf: { xs: 'stretch', sm: 'auto' } }}
                >
                  {createStatus.isPending
                    ? 'Memproses…'
                    : mode === 'now'
                      ? 'Posting Status'
                      : 'Jadwalkan Status'}
                </Button>
              </Stack>
            </Stack>
          </CardContent>
        </Card>

        {/* Live preview */}
        <Box sx={{ position: { lg: 'sticky' }, top: { lg: 16 } }}>
          <SectionHeader
            icon={<VisibilityOutlinedIcon fontSize="small" />}
            title="Pratinjau"
            subtitle="Perkiraan tampilan Status di WhatsApp."
          />
          <StatusPreview text={text} imageUrl={previewUrl} mode={mode} runAt={runAt} />
          <Stack direction="row" spacing={0.75} sx={{ mt: 1.25, flexWrap: 'wrap', gap: 0.5 }}>
            <Chip size="small" icon={<AutoStoriesIcon />} label="Terlihat 24 jam" variant="outlined" />
            <Chip size="small" icon={<AttachFileIcon />} label={file ? 'Dengan gambar' : 'Teks saja'} variant="outlined" />
          </Stack>
        </Box>
      </Box>

      {/* History */}
      <Card variant="outlined" sx={{ borderRadius: 1 }}>
        <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
          <Stack
            direction={{ xs: 'column', sm: 'row' }}
            spacing={1.25}
            sx={{ justifyContent: 'space-between', alignItems: { sm: 'flex-start' }, mb: 1.5 }}
          >
            <SectionHeader
              icon={<ScheduleIcon fontSize="small" />}
              title="Riwayat & Jadwal"
              subtitle={
                isFetching && !isLoading
                  ? 'Memperbarui daftar…'
                  : 'Status terjadwal, terposting, dan yang gagal.'
              }
            />
            <Stack direction="row" spacing={1} sx={{ flexWrap: 'wrap', gap: 1 }}>
              <StatPill
                label="Semua"
                value={counts.all}
                active={filter === 'all'}
                onClick={() => setFilter('all')}
              />
              <StatPill
                label="Antrian"
                value={counts.scheduled}
                tone="warning"
                active={filter === 'scheduled'}
                onClick={() => setFilter('scheduled')}
              />
              <StatPill
                label="Terposting"
                value={counts.done}
                tone="success"
                active={filter === 'done'}
                onClick={() => setFilter('done')}
              />
              <StatPill
                label="Gagal"
                value={counts.failed}
                tone="error"
                active={filter === 'failed'}
                onClick={() => setFilter('failed')}
              />
            </Stack>
          </Stack>

          {statuses.length === 0 ? (
            <EmptyState
              icon={<AutoStoriesIcon sx={{ fontSize: 40 }} />}
              title="Belum ada status"
              description="Buat Status pertama di form di atas. Cocok untuk promo harian, jam buka, atau update produk."
            />
          ) : filtered.length === 0 ? (
            <EmptyState
              icon={<AutoStoriesIcon sx={{ fontSize: 40 }} />}
              title="Tidak ada di filter ini"
              description="Coba pilih filter lain, atau buat Status baru."
              actionLabel="Lihat semua"
              onAction={() => setFilter('all')}
            />
          ) : (
            <Stack spacing={1}>
              {filtered.map((s) => (
                <Box
                  key={s.id}
                  sx={{
                    display: 'flex',
                    border: '1px solid',
                    borderColor: 'divider',
                    borderRadius: 0.75,
                    overflow: 'hidden',
                    bgcolor: 'background.paper',
                    '&:hover': { borderColor: 'primary.light' },
                  }}
                >
                  <Box sx={{ width: 3, flexShrink: 0, bgcolor: accentColor(s.status) }} />
                  <Box sx={{ flex: 1, minWidth: 0, p: 1.25 }}>
                    <Stack
                      direction={{ xs: 'column', sm: 'row' }}
                      spacing={1}
                      sx={{ justifyContent: 'space-between', alignItems: { sm: 'flex-start' } }}
                    >
                      <Box sx={{ minWidth: 0, flex: 1 }}>
                        <Stack
                          direction="row"
                          spacing={0.5}
                          sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 0.5, mb: 0.5 }}
                        >
                          <Chip
                            size="small"
                            color={STATUS_COLOR[s.status] || 'default'}
                            icon={STATUS_ICON[s.status] as ReactElement | undefined}
                            label={STATUS_LABEL[s.status] || s.status}
                            variant={s.status === 'done' || s.status === 'failed' ? 'filled' : 'outlined'}
                          />
                          {s.media_type === 'image' && (
                            <Chip
                              size="small"
                              variant="outlined"
                              icon={<ImageOutlinedIcon />}
                              label="Gambar"
                            />
                          )}
                          <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 500 }}>
                            {fmtRunAt(s.run_at)}
                            {s.status === 'scheduled' || s.status === 'running'
                              ? ` · ${relativeLabel(s.run_at)}`
                              : s.status === 'done'
                                ? ` · ${relativeLabel(s.run_at)}`
                                : ''}
                          </Typography>
                        </Stack>

                        <Typography
                          variant="body2"
                          sx={{
                            whiteSpace: 'pre-wrap',
                            wordBreak: 'break-word',
                            color: s.text ? 'text.primary' : 'text.secondary',
                            fontStyle: s.text ? 'normal' : 'italic',
                            lineHeight: 1.5,
                          }}
                        >
                          {s.text || (s.media_type === 'image' ? 'Status gambar tanpa teks' : '—')}
                        </Typography>

                        {s.status === 'failed' && s.error && (
                          <Alert severity="error" sx={{ mt: 1, py: 0.25 }}>
                            <Typography variant="caption" sx={{ lineHeight: 1.4 }}>
                              {s.error}
                            </Typography>
                          </Alert>
                        )}
                      </Box>

                      {s.status === 'scheduled' && (
                        <Tooltip title="Batalkan jadwal">
                          <Button
                            size="small"
                            color="error"
                            variant="outlined"
                            startIcon={
                              cancelStatus.isPending ? (
                                <CircularProgress size={14} color="inherit" />
                              ) : (
                                <DeleteOutlinedIcon />
                              )
                            }
                            onClick={() => doCancel(s)}
                            disabled={cancelStatus.isPending}
                            sx={{ flexShrink: 0, alignSelf: { xs: 'stretch', sm: 'flex-start' } }}
                          >
                            Batalkan
                          </Button>
                        </Tooltip>
                      )}
                    </Stack>
                  </Box>
                </Box>
              ))}
            </Stack>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}
