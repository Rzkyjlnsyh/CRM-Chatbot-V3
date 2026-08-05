import { useEffect, useState } from 'react';
import { Box, TextField, Button, Typography, Alert, CircularProgress, Link } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import api from '../services/api';
import logo from '../assets/logo-slaludiskon-login.png';

function responseStatus(error: unknown) {
  if (typeof error === 'object' && error && 'response' in error) {
    return (error as { response?: { status?: number; headers?: Record<string, string> } }).response;
  }
  return undefined;
}

function Illustration() {
  return (
    <Box
      sx={{
        display: { xs: 'none', md: 'flex' },
        flex: 1,
        // Sedikit naik dari tengah murni agar blok ilustrasi+teks tidak terasa “jatuh”.
        alignItems: 'center',
        justifyContent: 'center',
        p: { md: 5 },
        pt: { md: 4 },
        pb: { md: 6 },
        background: 'linear-gradient(160deg, #0D3420 0%, #14522F 30%, #1A6B3C 60%, #1F8A50 100%)',
        position: 'relative',
        overflow: 'hidden',
        minHeight: { md: '100vh' },
      }}
    >
      {/* Decorative circles */}
      <Box
        sx={{
          position: 'absolute',
          width: 500,
          height: 500,
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(255,255,255,0.06) 0%, transparent 70%)',
          top: '-10%',
          right: '-15%',
          pointerEvents: 'none',
        }}
      />
      <Box
        sx={{
          position: 'absolute',
          width: 350,
          height: 350,
          borderRadius: '50%',
          background: 'radial-gradient(circle, rgba(37,211,102,0.1) 0%, transparent 70%)',
          bottom: '5%',
          left: '-10%',
          pointerEvents: 'none',
        }}
      />

      {/* Satu blok konten: ilustrasi di atas, copy di bawah — left-aligned, tidak absolute di bottom viewport */}
      <Box
        sx={{
          position: 'relative',
          zIndex: 1,
          width: '100%',
          maxWidth: 400,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'stretch',
          // Geser blok sedikit ke atas relative ke center viewport
          transform: 'translateY(-4%)',
        }}
      >
        <Box sx={{ width: '100%', mb: 2.5 }}>
          <svg viewBox="0 0 420 340" fill="none" xmlns="http://www.w3.org/2000/svg" style={{ width: '100%', height: 'auto', display: 'block' }} aria-hidden>
            {/* AI Sparkles - top left */}
            <g opacity="0.9">
              <path d="M48 32l4 8 8 4-8 4-4 8-4-8-8-4 8-4 4-8z" fill="#5DBA7D" />
              <path d="M380 18l3 7 7 3-7 3-3 7-3-7-7-3 7-3 3-7z" fill="#7ED6A0" />
              <path d="M28 90l2 5 5 2-5 2-2 5-2-5-5-2 5-2 2-5z" fill="#7ED6A0" opacity="0.6" />
              <path d="M395 85l2 4 4 2-4 2-2 4-2-4-4-2 4-2 2-4z" fill="#5DBA7D" opacity="0.5" />
            </g>

            {/* Broadcast waves - top right */}
            <g opacity="0.35">
              <path d="M298 20c-5-5-13-5-18 0" stroke="#fff" strokeWidth="2" strokeLinecap="round" />
              <path d="M290 27c-5-9-15-9-20 0" stroke="#fff" strokeWidth="2" strokeLinecap="round" />
              <path d="M283 35c-6-14-17-14-23 0" stroke="#fff" strokeWidth="2" strokeLinecap="round" />
            </g>

            {/* Main phone / chat bubble */}
            <g>
              <rect x="115" y="72" width="200" height="240" rx="20" fill="#fff" opacity="0.12" />
              <rect x="123" y="80" width="184" height="224" rx="14" fill="#0D2818" />

              <rect x="123" y="80" width="184" height="44" rx="14" fill="#1A5E38" />
              <rect x="123" y="108" width="184" height="16" fill="#1A5E38" />
              <circle cx="143" cy="102" r="12" fill="#5DBA7D" opacity="0.4" />
              <rect x="163" y="96" width="80" height="6" rx="3" fill="#fff" opacity="0.25" />
              <rect x="163" y="106" width="50" height="4" rx="2" fill="#fff" opacity="0.15" />

              <rect x="138" y="136" width="140" height="36" rx="10" fill="#1E4A30" />
              <rect x="148" y="144" width="80" height="4" rx="2" fill="#5DBA7D" opacity="0.5" />
              <rect x="148" y="152" width="110" height="4" rx="2" fill="#5DBA7D" opacity="0.35" />
              <rect x="148" y="160" width="60" height="4" rx="2" fill="#5DBA7D" opacity="0.35" />

              <rect x="198" y="184" width="96" height="36" rx="10" fill="#1F8A50" />
              <rect x="208" y="192" width="70" height="4" rx="2" fill="#fff" opacity="0.6" />
              <rect x="208" y="200" width="50" height="4" rx="2" fill="#fff" opacity="0.4" />
              <rect x="208" y="208" width="30" height="4" rx="2" fill="#fff" opacity="0.4" />

              <rect x="138" y="232" width="130" height="50" rx="10" fill="#1E4A30" />
              <rect x="148" y="240" width="90" height="4" rx="2" fill="#5DBA7D" opacity="0.5" />
              <rect x="148" y="248" width="100" height="4" rx="2" fill="#5DBA7D" opacity="0.35" />
              <rect x="148" y="256" width="70" height="4" rx="2" fill="#5DBA7D" opacity="0.35" />
              <rect x="148" y="266" width="32" height="12" rx="6" fill="#1F8A50" />
              <text x="164" y="275" textAnchor="middle" fill="#fff" fontSize="8" fontWeight="700" fontFamily="Inter, sans-serif">AI</text>

              <path d="M130 238l2.5 5 5 2.5-5 2.5-2.5 5-2.5-5-5-2.5 5-2.5 2.5-5z" fill="#25D366" opacity="0.9" />

              <circle cx="148" cy="298" r="3" fill="#5DBA7D" opacity="0.7">
                <animate attributeName="opacity" values="0.7;0.2;0.7" dur="1.4s" repeatCount="indefinite" />
              </circle>
              <circle cx="160" cy="298" r="3" fill="#5DBA7D" opacity="0.4">
                <animate attributeName="opacity" values="0.4;0.7;0.4" dur="1.4s" repeatCount="indefinite" begin="0.3s" />
              </circle>
              <circle cx="172" cy="298" r="3" fill="#5DBA7D" opacity="0.2">
                <animate attributeName="opacity" values="0.2;0.7;0.2" dur="1.4s" repeatCount="indefinite" begin="0.6s" />
              </circle>
            </g>

            <g transform="translate(68, 110)" opacity="0.25">
              <circle cx="18" cy="18" r="16" fill="#25D366" />
            </g>

            <g transform="translate(310, 62)" opacity="0.2">
              <path d="M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2zm6-6v-5c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z" fill="#fff" />
            </g>

            <g opacity="0.15">
              <circle cx="60" cy="300" r="2" fill="#fff" />
              <circle cx="75" cy="300" r="2" fill="#fff" />
              <circle cx="90" cy="300" r="2" fill="#fff" />
              <circle cx="60" cy="312" r="2" fill="#fff" />
              <circle cx="75" cy="312" r="2" fill="#fff" />
            </g>
          </svg>
        </Box>

        <Box sx={{ textAlign: 'left', px: 0.5 }}>
          <Typography
            component="h1"
            sx={{
              color: '#fff',
              fontWeight: 600,
              fontSize: { md: '1.35rem', lg: '1.5rem' },
              lineHeight: 1.25,
              letterSpacing: '-0.01em',
              mb: 0.75,
            }}
          >
            WhatsApp AI Assistant
          </Typography>
          <Typography
            sx={{
              color: 'rgba(255,255,255,0.72)',
              fontSize: '0.875rem',
              lineHeight: 1.55,
              maxWidth: 360,
            }}
          >
            Automasi balasan AI, broadcast massal, dan CRM WhatsApp — semua dalam satu dashboard.
          </Typography>
        </Box>
      </Box>
    </Box>
  );
}

export default function Login() {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [cooldown, setCooldown] = useState(0);
  const [needVerify, setNeedVerify] = useState(false);
  const [turnstileToken] = useState('');
  const navigate = useNavigate();

  // Turnstile — disembunyikan dulu
  // useEffect(() => {
  //   const render = () => {
  //     if (window.turnstile) {
  //       window.turnstile.render('#turnstile-login', {
  //         sitekey: '0x4AAAAAADrLaq7r2pyIGOYs',
  //         callback: (token: string) => { setTurnstile(token); },
  //       });
  //     } else {
  //       setTimeout(render, 200);
  //     }
  //   };
  //   render();
  // }, []);

  useEffect(() => {
    if (cooldown <= 0) return;
    const timer = window.setInterval(() => setCooldown((v) => Math.max(0, v - 1)), 1000);
    return () => window.clearInterval(timer);
  }, [cooldown]);

  const handleLogin = async () => {
    if (loading || cooldown > 0) return;
    const cleanUsername = username.trim();
    const e: Record<string, string> = {};
    if (!cleanUsername) e.username = 'Wajib diisi';
    if (!password) e.password = 'Wajib diisi';
    setErrors(e);
    if (Object.keys(e).length > 0) return;
    setError('');
    setNeedVerify(false);
    setLoading(true);
    try {
      const res = await api.post('/login', { username: cleanUsername, password, turnstile: turnstileToken });
      localStorage.setItem('token', res.data.token);
      localStorage.setItem('user', JSON.stringify(res.data.user));
      navigate('/app');
    } catch (e) {
      const response = responseStatus(e);
      if (response?.status === 429) {
        const retryAfter = Number(response.headers?.['retry-after'] || 60);
        setCooldown(Number.isFinite(retryAfter) ? Math.min(Math.max(retryAfter, 30), 300) : 60);
        setError('Terlalu banyak percobaan. Tunggu sebentar lalu coba lagi.');
      } else if (response?.status === 403) {
        setError('Email kamu belum diverifikasi. Cek inbox atau folder spam untuk link aktivasi.');
        setNeedVerify(true);
      } else if (!response || (response.status ?? 0) >= 500) {
        setError('Server belum siap. Coba lagi sebentar lagi.');
      } else {
        setError('Login belum berhasil. Periksa kembali data yang kamu masukkan.');
      }
      setLoading(false);
      // if (window.turnstile) window.turnstile.reset();
    }
  };

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: { xs: 'column', md: 'row' },
        minHeight: '100vh',
        bgcolor: 'background.default',
      }}
    >
      {/* Left: Illustration */}
      <Illustration />

      {/* Right: Login Form */}
      <Box
        sx={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          p: { xs: 2, sm: 4, md: 6 },
          minHeight: { xs: 'auto', md: '100vh' },
          bgcolor: 'background.paper',
          borderLeft: { md: '1px solid' },
          borderColor: { md: 'divider' },
        }}
      >
        <Box sx={{ width: '100%', maxWidth: 420 }}>
          {/* Logo */}
          <Box sx={{ textAlign: 'center', mb: 0.5 }}>
            <img
              src={logo}
              alt="SlaluDiskon"
                            style={{
                width: '42%',
                maxWidth: 180,
                height: 'auto',
                display: 'block',
                margin: '0 auto',
              }}
            />
          </Box>

          <Typography
            variant="body2"
            sx={{
              textAlign: 'center',
              color: 'text.secondary',
              mb: 3,
              fontSize: '0.85rem',
            }}
          >
            Masuk ke dashboard untuk mengelola WhatsApp AI Assistant kamu.
          </Typography>

          {error && (
            <Alert severity={cooldown > 0 ? 'warning' : 'error'} sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}

          {needVerify && (
            <Typography variant="body2" sx={{ mb: 2, textAlign: 'center' }}>
              <Link
                component="button"
                type="button"
                underline="hover"
                onClick={() => navigate('/cek-email', { state: { email: username.trim() } })}
              >
                Kirim ulang link verifikasi
              </Link>
            </Typography>
          )}

          <TextField
            fullWidth
            label="Username"
            value={username}
            disabled={loading || cooldown > 0}
            autoComplete="username"
            onChange={(e) => {
              setUsername(e.target.value);
              if (errors.username) setErrors((p) => ({ ...p, username: '' }));
            }}
            error={!!errors.username}
            helperText={errors.username}
            sx={{ mb: 2 }}
            onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
          />

          <TextField
            fullWidth
            label="Password"
            type="password"
            value={password}
            disabled={loading || cooldown > 0}
            autoComplete="current-password"
            onChange={(e) => {
              setPassword(e.target.value);
              if (errors.password) setErrors((p) => ({ ...p, password: '' }));
            }}
            error={!!errors.password}
            helperText={errors.password}
            sx={{ mb: 2.5 }}
            onKeyDown={(e) => e.key === 'Enter' && handleLogin()}
          />

          {/* Turnstile — disembunyikan dulu */}
          {/* <Box id="turnstile-login" sx={{ mb: 2, display: 'flex', justifyContent: 'center' }} /> */}

          <Button
            fullWidth
            variant="contained"
            size="large"
            onClick={handleLogin}
            disabled={loading || cooldown > 0}
            startIcon={loading ? <CircularProgress size={18} color="inherit" /> : null}
            sx={{
              py: 1.35,
              fontWeight: 600,
              fontSize: '0.95rem',
              // Pill penuh — sama seperti nav aktif, bukan radius “nanggung”
              borderRadius: 999,
              textTransform: 'none',
              boxShadow: 'none',
              '&:hover': { boxShadow: 'none' },
            }}
          >
            {loading ? 'Masuk…' : cooldown > 0 ? `Coba lagi ${cooldown}d` : 'Masuk'}
          </Button>
        </Box>
      </Box>
    </Box>
  );
}
