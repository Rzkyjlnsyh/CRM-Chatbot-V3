import { Box, Stack, TextField, Typography } from '@mui/material';

// DelayFields = kontrol "Jeda Kirim" yang dipakai bersama oleh Broadcast & Jadwal:
// jeda acak antar pesan + jeda istirahat berkala. Validasi tetap di parent (lewat `error`).
export default function DelayFields({
  minDelay, maxDelay, restEvery, restDuration,
  setMinDelay, setMaxDelay, setRestEvery, setRestDuration,
  error, onEditDelay,
}: {
  minDelay: number;
  maxDelay: number;
  restEvery: number;
  restDuration: number;
  setMinDelay: (n: number) => void;
  setMaxDelay: (n: number) => void;
  setRestEvery: (n: number) => void;
  setRestDuration: (n: number) => void;
  error?: string;
  onEditDelay?: () => void;
}) {
  return (
    <Box>
      <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>Jeda antar pesan</Typography>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
        Tunggu beberapa detik (acak) sebelum mengirim ke nomor berikutnya.
      </Typography>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
        <TextField type="number" size="small" label="Minimal (detik)" value={minDelay}
          onChange={e => { setMinDelay(Number(e.target.value)); onEditDelay?.(); }}
          error={!!error}
          sx={{ width: { xs: '100%', sm: 160 } }} />
        <TextField type="number" size="small" label="Maksimal (detik)" value={maxDelay}
          onChange={e => { setMaxDelay(Number(e.target.value)); onEditDelay?.(); }}
          error={!!error}
          helperText={error || ' '}
          sx={{ width: { xs: '100%', sm: 160 } }} />
      </Stack>

      <Typography variant="subtitle2" sx={{ fontWeight: 700, mt: 1.5 }}>Jeda istirahat</Typography>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
        Berhenti sejenak setelah mengirim sejumlah pesan, lalu lanjut otomatis. Isi 0 untuk mematikan.
      </Typography>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
        <TextField type="number" size="small" label="Berhenti setiap (pesan)" value={restEvery}
          onChange={e => setRestEvery(Math.max(0, Number(e.target.value)))}
          helperText={restEvery <= 0 ? 'Mati' : ' '}
          sx={{ width: { xs: '100%', sm: 190 } }} />
        <TextField type="number" size="small" label="Lama berhenti (detik)" value={restDuration}
          onChange={e => setRestDuration(Math.max(0, Number(e.target.value)))}
          disabled={restEvery <= 0}
          helperText=" "
          sx={{ width: { xs: '100%', sm: 190 } }} />
      </Stack>
    </Box>
  );
}
