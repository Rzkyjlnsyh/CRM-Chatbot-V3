import { useState } from 'react';
import {
  Avatar, Box, Button, Chip, CircularProgress, Divider, IconButton, Paper,
  Stack, ToggleButton, ToggleButtonGroup, Tooltip, Typography,
} from '@mui/material';
import CheckIcon from '@mui/icons-material/TaskAltOutlined';
import HistoryIcon from '@mui/icons-material/HistoryOutlined';
import PendingIcon from '@mui/icons-material/PendingActionsOutlined';
import PersonRemoveIcon from '@mui/icons-material/PersonRemoveOutlined';
import RefreshIcon from '@mui/icons-material/Refresh';
import ShieldIcon from '@mui/icons-material/ShieldOutlined';
import EmptyState from '../common/EmptyState';
import { useConfirmKick, useDismissModeration, useGroupModeration } from '../../hooks';
import { swalConfirm, swalToast } from '../../services/swal';
import type { GroupModerationLog } from '../../types';
import { LoadingState, SummaryGrid } from './GroupGuardShared';

type ActivityFilter = 'pending' | 'all' | 'finished';

function activityState(row: GroupModerationLog): { label: string; color: 'default' | 'success' | 'error' | 'warning' } {
  if (row.status === 'pending') return { label: 'Perlu keputusan', color: 'warning' };
  if (row.status === 'dismissed') return { label: 'Diabaikan', color: 'default' };
  const states: Record<string, { label: string; color: 'default' | 'success' | 'error' | 'warning' }> = {
    deleted: { label: 'Pesan dihapus', color: 'success' },
    kicked: { label: 'Anggota dikeluarkan', color: 'error' },
    flagged: { label: 'Terdeteksi', color: 'warning' },
    warned: { label: 'Diperingatkan', color: 'warning' },
  };
  return states[row.action] || { label: row.action, color: 'default' };
}

function reasonLabel(reason: string) {
  if (reason === 'tautan/link') return 'Mengirim tautan';
  if (reason === 'nomor telepon') return 'Mengirim nomor telepon';
  if (reason === 'flood (pesan beruntun)') return 'Mengirim pesan beruntun';
  if (reason.startsWith('kata terlarang:')) return `Memuat kata terlarang: ${reason.slice('kata terlarang:'.length).trim()}`;
  return reason;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('id-ID', {
    day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit',
  }).format(new Date(value));
}

export default function GroupModerationFeed({ agentId }: { agentId: number }) {
  const { data: rows = [], isLoading, isError, refetch, isFetching } = useGroupModeration(agentId);
  const confirmKick = useConfirmKick(agentId);
  const dismiss = useDismissModeration(agentId);
  const [filter, setFilter] = useState<ActivityFilter>('pending');

  const pendingCount = rows.filter(row => row.status === 'pending').length;
  const finishedCount = rows.length - pendingCount;
  const visibleRows = rows.filter(row => filter === 'all' || (filter === 'pending' ? row.status === 'pending' : row.status !== 'pending'));

  if (isError) {
    return (
      <EmptyState
        icon={<HistoryIcon sx={{ fontSize: 48 }} />}
        title="Belum ada aktivitas moderasi"
        description="Aktivitas deteksi spam dan tindakan Anti-Spam Grup akan tercatat di sini setelah WhatsApp tersambung."
        actionLabel="Coba Lagi"
        onAction={() => refetch()}
      />
    );
  }

  const handleKick = async (row: GroupModerationLog) => {
    const sender = row.sender_name || row.sender;
    if (!await swalConfirm(`Keluarkan ${sender}?`, `Anggota akan dikeluarkan dari ${row.group_name || 'grup ini'}.`)) return;
    try {
      await confirmKick.mutateAsync(row.id);
      swalToast(`${sender} dikeluarkan dari grup`);
    } catch {
      swalToast('Anggota belum bisa dikeluarkan. Pastikan nomor ini masih menjadi admin.', 'error');
    }
  };

  const handleDismiss = async (row: GroupModerationLog) => {
    try {
      await dismiss.mutateAsync(row.id);
      swalToast('Deteksi ditandai aman');
    } catch {
      swalToast('Aktivitas belum bisa diperbarui', 'error');
    }
  };

  return (
    <Box>
      <SummaryGrid items={[
        { label: 'Perlu keputusan', value: pendingCount, tone: pendingCount ? 'warning.main' : 'text.secondary' },
        { label: 'Selesai ditangani', value: finishedCount, tone: finishedCount ? 'success.main' : 'text.secondary' },
        { label: 'Aktivitas terbaru', value: rows.length },
      ]} />

      <Paper variant="outlined" sx={{ p: 1, mb: 1.5 }}>
        <Stack direction="row" spacing={1} sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
          <ToggleButtonGroup
            size="small"
            exclusive
            value={filter}
            onChange={(_, value: ActivityFilter | null) => value && setFilter(value)}
            aria-label="Filter aktivitas"
            sx={{ '& .MuiToggleButton-root': { px: { xs: 1, sm: 1.5 } } }}
          >
            <ToggleButton value="pending">Perlu ditinjau {pendingCount > 0 ? `(${pendingCount})` : ''}</ToggleButton>
            <ToggleButton value="all">Semua</ToggleButton>
            <ToggleButton value="finished">Selesai</ToggleButton>
          </ToggleButtonGroup>
          <Tooltip title="Segarkan aktivitas">
            <span>
              <IconButton aria-label="Segarkan aktivitas" onClick={() => refetch()} disabled={isFetching}>
                {isFetching ? <CircularProgress size={20} /> : <RefreshIcon />}
              </IconButton>
            </span>
          </Tooltip>
        </Stack>
      </Paper>

      {isLoading && <LoadingState label="Memuat aktivitas..." />}
      {!isLoading && visibleRows.length === 0 && (
        <EmptyState
          icon={filter === 'pending' ? <CheckIcon sx={{ fontSize: 44 }} /> : <HistoryIcon sx={{ fontSize: 44 }} />}
          title={filter === 'pending' ? 'Tidak ada yang perlu ditinjau' : 'Belum ada aktivitas'}
          description={filter === 'pending' ? 'Semua deteksi sudah ditangani. Aktivitas baru akan muncul otomatis.' : 'Deteksi spam dan tindakan anti-spam akan tercatat di sini.'}
        />
      )}

      {!isLoading && visibleRows.length > 0 && (
        <Paper variant="outlined" sx={{ overflow: 'hidden' }}>
          {visibleRows.map((row, index) => {
            const state = activityState(row);
            const kickPending = confirmKick.isPending && confirmKick.variables === row.id;
            const dismissPending = dismiss.isPending && dismiss.variables === row.id;
            return (
              <Box key={row.id}>
                {index > 0 && <Divider />}
                <Box sx={{ px: { xs: 1.25, sm: 1.5 }, py: 1.25, bgcolor: row.status === 'pending' ? 'rgba(237, 108, 2, 0.035)' : 'background.paper' }}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { xs: 'stretch', sm: 'flex-start' } }}>
                    <Avatar sx={{ width: 34, height: 34, bgcolor: row.status === 'pending' ? 'warning.light' : 'action.selected', color: row.status === 'pending' ? 'warning.dark' : 'text.secondary' }}>
                      {row.status === 'pending' ? <PendingIcon fontSize="small" /> : <ShieldIcon fontSize="small" />}
                    </Avatar>
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                      <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 0.5 }}>
                        <Typography variant="body2" sx={{ fontWeight: 800 }}>{row.sender_name || row.sender}</Typography>
                        <Chip size="small" color={state.color} variant="outlined" label={state.label} />
                        <Typography variant="caption" color="text.secondary">{formatDate(row.created_at)}</Typography>
                      </Stack>
                      <Typography variant="body2" color="text.secondary" sx={{ mt: 0.35 }}>
                        {reasonLabel(row.reason)} di <b>{row.group_name || row.group_jid}</b>
                      </Typography>
                      {row.excerpt && (
                        <Box sx={{ mt: 0.75, px: 1, py: 0.65, borderLeft: '3px solid', borderColor: 'divider', bgcolor: 'action.hover' }}>
                          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', wordBreak: 'break-word', whiteSpace: 'pre-wrap' }}>
                            {row.excerpt}
                          </Typography>
                        </Box>
                      )}
                    </Box>
                    {row.status === 'pending' && (
                      <Stack direction="row" spacing={0.75} sx={{ alignSelf: { xs: 'stretch', sm: 'center' } }}>
                        <Button size="small" variant="outlined" onClick={() => handleDismiss(row)} disabled={kickPending || dismissPending} sx={{ flex: { xs: 1, sm: 'initial' } }}>
                          {dismissPending ? 'Menyimpan...' : 'Tandai aman'}
                        </Button>
                        <Button size="small" color="error" variant="contained" startIcon={<PersonRemoveIcon />} onClick={() => handleKick(row)} disabled={kickPending || dismissPending} sx={{ flex: { xs: 1, sm: 'initial' } }}>
                          {kickPending ? 'Memproses...' : 'Keluarkan'}
                        </Button>
                      </Stack>
                    )}
                  </Stack>
                </Box>
              </Box>
            );
          })}
        </Paper>
      )}
    </Box>
  );
}
