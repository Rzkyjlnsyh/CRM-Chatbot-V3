import { Box, CircularProgress, Paper, Typography } from '@mui/material';

export function SummaryGrid({ items }: { items: Array<{ label: string; value: number; tone?: string }> }) {
  return (
    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', md: `repeat(${items.length}, minmax(0, 1fr))` }, gap: 1, mb: 1.5 }}>
      {items.map(item => (
        <Paper key={item.label} variant="outlined" sx={{ p: 1 }}>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.2 }}>
            {item.label}
          </Typography>
          <Typography sx={{ mt: 0.25, fontWeight: 600, lineHeight: 1.2, color: item.tone || 'text.primary' }}>
            {item.value}
          </Typography>
        </Paper>
      ))}
    </Box>
  );
}

export function LoadingState({ label }: { label: string }) {
  return (
    <Paper variant="outlined" sx={{ py: 5, textAlign: 'center' }}>
      <CircularProgress size={26} />
      <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>{label}</Typography>
    </Paper>
  );
}
