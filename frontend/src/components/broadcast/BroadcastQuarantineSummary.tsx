import { Alert, Box, Chip, Stack, Typography } from '@mui/material';
import ShieldIcon from '@mui/icons-material/HealthAndSafetyOutlined';
import type { BroadcastRotationSummary } from '../../types';

function fmtWhen(iso?: string | null) {
  if (!iso) return null;
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return d.toLocaleString('id-ID', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function reasonColor(reason: string): 'error' | 'warning' | 'default' | 'info' {
  if (reason === 'hard') return 'error';
  if (reason === 'soft') return 'warning';
  if (reason === 'disconnect') return 'info';
  return 'default';
}

/**
 * Ringkasan nomor CS dalam pool rotasi + status karantina (dibatasi / rate-limit / offline).
 * Ditampilkan di detail Blast agar user tahu nomor mana yang bermasalah dan kenapa.
 */
export default function BroadcastQuarantineSummary({
  rotation,
  compact = false,
}: {
  rotation?: BroadcastRotationSummary | null;
  compact?: boolean;
}) {
  if (!rotation || !rotation.agents?.length) return null;

  const hasQuarantine = (rotation.quarantine?.length ?? 0) > 0;
  const multi = rotation.enabled || rotation.pool_size > 1;

  // Single-agent tanpa karantina: tidak perlu panel.
  if (!multi && !hasQuarantine) return null;

  return (
    <Box
      sx={{
        mt: compact ? 1 : 1.25,
        p: 1.25,
        border: '1px solid',
        borderColor: hasQuarantine && rotation.quarantine_active > 0 ? 'warning.light' : 'divider',
        borderRadius: 1,
        bgcolor: hasQuarantine && rotation.quarantine_active > 0 ? 'rgba(194, 65, 12, 0.04)' : 'background.paper',
      }}
    >
      <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', mb: 1, flexWrap: 'wrap', gap: 0.5 }}>
        <ShieldIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
        <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
          {multi ? 'Rotasi & kesehatan nomor' : 'Kesehatan nomor pengirim'}
        </Typography>
        {multi && (
          <Chip size="small" variant="outlined" label={`${rotation.pool_size} nomor`} />
        )}
        {rotation.quarantine_active > 0 && (
          <Chip
            size="small"
            color="warning"
            label={`${rotation.quarantine_active} dikarantina`}
          />
        )}
        {rotation.pause_code ? (
          <Chip size="small" variant="outlined" color="warning" label={`Kode WA ${rotation.pause_code}`} />
        ) : null}
      </Stack>

      {hasQuarantine && rotation.quarantine_active > 0 && (
        <Alert severity="warning" icon={false} sx={{ mb: 1, py: 0.5 }}>
          <Typography variant="caption" sx={{ display: 'block', lineHeight: 1.45 }}>
            Nomor yang dikarantina tidak dipakai mengirim. Antrean menunggu dialihkan ke nomor sehat
            (jika ada). Saat melanjutkan Blast, nomor hard-quarantine hanya dicoba lagi sebagai last-resort.
          </Typography>
        </Alert>
      )}

      <Stack spacing={0.75}>
        {rotation.agents.map((agent) => {
          const q = agent.quarantine;
          const active = Boolean(q?.active);
          return (
            <Box
              key={agent.id}
              sx={{
                p: 1,
                borderRadius: 0.75,
                border: '1px solid',
                borderColor: active ? 'warning.light' : 'divider',
                bgcolor: 'background.paper',
              }}
            >
              <Stack
                direction={{ xs: 'column', sm: 'row' }}
                spacing={0.75}
                sx={{ justifyContent: 'space-between', alignItems: { sm: 'flex-start' }, gap: 0.75 }}
              >
                <Box sx={{ minWidth: 0 }}>
                  <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 0.5, mb: 0.25 }}>
                    <Typography variant="body2" sx={{ fontWeight: 700 }} noWrap>
                      {agent.name}
                    </Typography>
                    <Chip
                      size="small"
                      variant="outlined"
                      label={agent.role === 'primary' ? 'Utama' : 'Cadangan'}
                    />
                    <Chip
                      size="small"
                      color={agent.connected ? 'success' : 'default'}
                      variant="outlined"
                      label={agent.connected ? 'Online' : 'Offline'}
                    />
                    {q && (
                      <Chip
                        size="small"
                        color={reasonColor(q.reason)}
                        variant={active ? 'filled' : 'outlined'}
                        label={active ? q.reason_label : `${q.reason_label} · selesai`}
                      />
                    )}
                  </Stack>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                    {agent.number ? `+${agent.number}` : `ID ${agent.id}`}
                    {' · '}
                    terkirim {agent.sent_count}
                    {' · '}
                    menunggu {agent.pending_count}
                    {agent.failed_count > 0 ? ` · gagal ${agent.failed_count}` : ''}
                  </Typography>
                  {q && (
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.35, lineHeight: 1.4 }}>
                      {q.advice}
                      {q.code ? ` (kode ${q.code})` : ''}
                      {fmtWhen(q.at) ? ` · sejak ${fmtWhen(q.at)}` : ''}
                      {active && q.until && fmtWhen(q.until) ? ` · cooldown s/d ${fmtWhen(q.until)}` : ''}
                    </Typography>
                  )}
                </Box>
              </Stack>
            </Box>
          );
        })}
      </Stack>
    </Box>
  );
}
