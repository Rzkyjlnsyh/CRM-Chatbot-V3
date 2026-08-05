import { useState } from 'react';
import { Box, Card, CardContent, TextField, Button, Typography, Alert, Link } from '@mui/material';
import api from '../services/api';

export default function ForgotPassword() {
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const [sent, setSent] = useState(false);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async () => {
    if (!email.trim()) { setError('Email wajib diisi'); return; }
    setError('');
    setLoading(true);
    try {
      await api.post('/forgot-password', { email: email.trim() });
      setSent(true);
    } catch { setError('Gagal mengirim, coba lagi nanti.'); }
    setLoading(false);
  };

  return (
    <Box sx={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', bgcolor: 'background.default', p: 2 }}>
      <Card sx={{ width: '100%', maxWidth: 400 }}>
        <CardContent sx={{ pt: 3, px: { xs: 3, sm: 4 }, pb: 3 }}>
          {sent ? (
            <Box sx={{ textAlign: 'center' }}>
              <Typography sx={{ fontSize: 48, mb: 1 }}>📧</Typography>
              <Typography variant="h6" sx={{ mb: 1 }}>Cek Email Kamu</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Kalau email terdaftar, kami sudah kirim tautan reset password.
              </Typography>
              <Button variant="outlined" href="/login">Kembali ke Login</Button>
            </Box>
          ) : (
            <>
              <Typography variant="h6" sx={{ mb: 1 }}>Lupa Password</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Masukkan email kamu, kami akan kirim tautan untuk reset password.
              </Typography>
              {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
              <TextField fullWidth label="Email" type="email" value={email} onChange={e => setEmail(e.target.value)}
                disabled={loading} sx={{ mb: 2 }}
                onKeyDown={e => e.key === 'Enter' && handleSubmit()} />
              <Button fullWidth variant="contained" onClick={handleSubmit} disabled={loading} sx={{ py: 1.5 }}>
                {loading ? 'Mengirim…' : 'Kirim Tautan Reset'}
              </Button>
              <Typography variant="body2" sx={{ mt: 2, textAlign: 'center' }}>
                <Link href="/login" underline="hover">Kembali ke Login</Link>
              </Typography>
            </>
          )}
        </CardContent>
      </Card>
    </Box>
  );
}
