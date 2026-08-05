import { useState } from 'react';
import { Box, Card, CardContent, Typography, Button, Alert, Link } from '@mui/material';
import { useNavigate, useLocation } from 'react-router-dom';
import MarkEmailReadOutlinedIcon from '@mui/icons-material/MarkEmailReadOutlined';
import api from '../services/api';
import logo from '../assets/logo-slaludiskon-login.png';

export default function CheckEmail() {
  const navigate = useNavigate();
  const location = useLocation();
  const stateEmail = (location.state as { email?: string } | null)?.email;
  const email = stateEmail || new URLSearchParams(location.search).get('email') || '';

  const [sent, setSent] = useState(false);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState('');

  const resend = async () => {
    if (!email) { setErr('Email tidak diketahui. Silakan daftar atau login ulang.'); return; }
    setLoading(true); setErr('');
    try {
      await api.post('/resend-verification', { email });
      setSent(true);
    } catch {
      setErr('Gagal mengirim ulang. Coba lagi sebentar.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Box sx={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', bgcolor: 'background.default', p: 2 }}>
      <Card sx={{ width: '100%', maxWidth: 440 }}>
        <Box sx={{ textAlign: 'center', pt: 3, px: 4 }}>
          <img src={logo} alt="SlaluDiskon" style={{ width: '50%', maxWidth: 180, height: 'auto', display: 'block', margin: '0 auto' }} />
        </Box>
        <CardContent sx={{ px: { xs: 3, sm: 4 }, pb: 4, textAlign: 'center' }}>
          <MarkEmailReadOutlinedIcon sx={{ fontSize: 56, color: 'primary.main', mt: 1 }} />
          <Typography variant="h5" sx={{ fontWeight: 800, mt: 1, mb: 1 }}>Cek email kamu</Typography>
          <Typography color="text.secondary" sx={{ mb: 1.5 }}>
            Kami sudah mengirim link verifikasi {email ? <>ke <b>{email}</b></> : 'ke email kamu'}. Klik link itu untuk mengaktifkan akun, lalu login.
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Tidak ada di inbox? Cek folder Spam atau Promosi.
          </Typography>
          {err && <Alert severity="error" sx={{ mb: 2 }}>{err}</Alert>}
          {sent && <Alert severity="success" sx={{ mb: 2 }}>Link verifikasi sudah dikirim ulang.</Alert>}
          <Button fullWidth variant="outlined" onClick={resend} disabled={loading || sent} sx={{ mb: 1.5 }}>
            {loading ? 'Mengirim…' : sent ? 'Terkirim' : 'Kirim ulang email'}
          </Button>
          <Typography variant="body2">
            Sudah verifikasi? <Link component="button" type="button" underline="hover" onClick={() => navigate('/login')}>Login</Link>
          </Typography>
        </CardContent>
      </Card>
    </Box>
  );
}
