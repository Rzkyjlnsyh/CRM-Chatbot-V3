import { Box, LinearProgress, Stack, Typography } from '@mui/material';
import type { Broadcast } from '../../types';

const ACTIVE_STATUSES = new Set(['pending', 'running', 'resuming', 'cancel_requested']);

function isBroadcastActive(status: string) {
  return ACTIVE_STATUSES.has(status);
}

function broadcastProgress(broadcast: Pick<Broadcast, 'total' | 'sent' | 'failed' | 'skipped'>) {
  const processed = Math.min(
    broadcast.total,
    broadcast.sent + broadcast.failed + broadcast.skipped,
  );
  const remaining = Math.max(0, broadcast.total - processed);
  const percent = broadcast.total > 0 ? Math.round((processed / broadcast.total) * 100) : 0;
  return { processed, remaining, percent };
}

function progressLabel(status: string, percent: number, processed: number) {
  if (status === 'pending') return 'Menunggu antrean…';
  if (status === 'resuming') return 'Menyiapkan untuk lanjut…';
  if (status === 'cancel_requested') return 'Menghentikan pengiriman…';
  if (status === 'running' && processed === 0) return 'Menyiapkan pengiriman…';
  if (status === 'running') return `${percent}% sedang dikirim`;
  if (status === 'done') return 'Pengiriman selesai';
  if (status === 'cancelled') return 'Pengiriman dibatalkan';
  if (status === 'wa_restricted') return 'Pengiriman dijeda WhatsApp';
  if (status === 'interrupted') return 'Pengiriman tertunda';
  if (status === 'failed') return 'Pengiriman gagal';
  return `${percent}% diproses`;
}

export default function BroadcastProgress({
  broadcast,
  compact = false,
}: {
  broadcast: Pick<Broadcast, 'status' | 'total' | 'sent' | 'failed' | 'skipped'>;
  compact?: boolean;
}) {
  const { processed, remaining, percent } = broadcastProgress(broadcast);
  const active = isBroadcastActive(broadcast.status);
  const preparing = active && processed === 0;

  return (
    <Box sx={{ minWidth: 0 }}>
      <LinearProgress
        aria-label={progressLabel(broadcast.status, percent, processed)}
        className={active ? 'blast-progress blast-progress--active' : 'blast-progress'}
        variant={preparing ? 'indeterminate' : 'determinate'}
        value={preparing ? undefined : percent}
        color={broadcast.status === 'done' ? 'success' : broadcast.status === 'failed' ? 'error' : 'primary'}
        sx={{ height: compact ? 6 : 8, mb: 0.65, bgcolor: 'action.hover' }}
      />
      <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'baseline', gap: 1 }}>
        <Typography variant="caption" sx={{ fontWeight: active ? 700 : 600, color: active ? 'primary.main' : 'text.secondary' }}>
          {progressLabel(broadcast.status, percent, processed)}
        </Typography>
        <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0 }}>
          {processed}/{broadcast.total}
        </Typography>
      </Stack>
      {!compact && (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
          {broadcast.sent} terkirim · {broadcast.failed} gagal · {broadcast.skipped} dilewati · {remaining} menunggu
        </Typography>
      )}
    </Box>
  );
}
