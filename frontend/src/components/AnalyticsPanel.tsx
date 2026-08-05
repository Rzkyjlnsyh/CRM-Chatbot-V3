import { Box, Typography, Card, CardContent, Grid, CircularProgress, Stack, LinearProgress } from '@mui/material';
import { useAgentAnalytics } from '../hooks';
import PageHeader from './PageHeader';

function Stat({ label, value, color }: { label: string; value: string | number; color?: string }) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Typography variant="h6" sx={{ color }}>{value}</Typography>
        <Typography variant="caption" color="text.secondary">{label}</Typography>
      </CardContent>
    </Card>
  );
}

export default function AnalyticsPanel({ agentId }: { agentId: number }) {
  const { data, isLoading } = useAgentAnalytics(agentId);
  if (isLoading || !data) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;

  const maxDay = Math.max(1, ...data.trend.map(t => t.count));

  return (
    <Box>
      <PageHeader title="Analitik" subtitle="Ringkasan performa CS ini: pesan masuk, porsi yang ditangani AI, dan tren mingguan." />
      <Grid container spacing={1.25} sx={{ mb: 1.5 }}>
        <Grid size={{ xs: 6, md: 3 }}><Stat label="Pesan masuk" value={data.total_incoming} /></Grid>
        <Grid size={{ xs: 6, md: 3 }}><Stat label="Dijawab AI" value={data.ai_replies} color="#1F8A50" /></Grid>
        <Grid size={{ xs: 6, md: 3 }}><Stat label="Dijawab manusia" value={data.human_replies} color="#1565c0" /></Grid>
        <Grid size={{ xs: 6, md: 3 }}><Stat label="Total kontak" value={data.contacts} /></Grid>
      </Grid>

      <Card sx={{ mb: 1.5 }}>
        <CardContent>
          <Typography variant="subtitle2" sx={{ fontWeight: 700, mb: 1 }}>Ditangani AI</Typography>
          <Stack direction="row" sx={{ alignItems: 'center' }} spacing={2}>
            <Box sx={{ flex: 1 }}>
              <LinearProgress variant="determinate" value={data.ai_handled_pct} sx={{ height: 8 }} />
            </Box>
            <Typography variant="subtitle2">{data.ai_handled_pct}%</Typography>
          </Stack>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
            {data.open_handoffs} kontak di antrian Butuh CS. Soft mode: asisten masih boleh bantu FAQ; tanpa balasan CS dalam 2 jam, otomatisasi full kembali.
          </Typography>
        </CardContent>
      </Card>

      <Card>
        <CardContent>
          <Typography variant="subtitle2" sx={{ fontWeight: 700, mb: 1.5 }}>Pesan masuk 7 hari terakhir</Typography>
          <Stack spacing={1}>
            {data.trend.map(t => (
              <Stack key={t.day} direction="row" sx={{ alignItems: 'center' }} spacing={1}>
                <Typography variant="caption" sx={{ width: 70, flexShrink: 0, color: 'text.secondary' }}>
                  {new Date(t.day).toLocaleDateString('id-ID', { day: '2-digit', month: 'short' })}
                </Typography>
                <Box sx={{ flex: 1, bgcolor: '#eceff1', borderRadius: 1, height: 18, position: 'relative' }}>
                  <Box sx={{ width: `${(t.count / maxDay) * 100}%`, bgcolor: 'primary.main', height: '100%', borderRadius: 1, minWidth: t.count ? 4 : 0 }} />
                </Box>
                <Typography variant="caption" sx={{ width: 28, textAlign: 'right', fontWeight: 700 }}>{t.count}</Typography>
              </Stack>
            ))}
          </Stack>
        </CardContent>
      </Card>
    </Box>
  );
}
