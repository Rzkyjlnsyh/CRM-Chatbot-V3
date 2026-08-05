import { Box, Container, Typography, Stack, Divider, Button } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import heroLogo from '../assets/Logo-slaludiskon-gradients.png';

const SECTIONS: { h: string; p: string[] }[] = [
  {
    h: '1. Penerimaan ketentuan',
    p: [
      'Dengan mendaftar dan menggunakan SlaluDiskon, kamu menyetujui Syarat dan Ketentuan ini. Jika kamu tidak setuju, mohon untuk tidak menggunakan layanan.',
    ],
  },
  {
    h: '2. Tentang layanan',
    p: [
      'SlaluDiskon adalah layanan asisten WhatsApp bertenaga AI yang membantu bisnis membalas pelanggan secara otomatis, mengelola kontak, broadcast, pesan terjadwal, follow up, dan fitur terkait lainnya.',
    ],
  },
  {
    h: '3. Akun dan kelayakan',
    p: [
      'Kamu bertanggung jawab menjaga kerahasiaan akun dan seluruh aktivitas yang terjadi di dalamnya. Pastikan data yang kamu berikan akurat dan terkini.',
      'Layanan ditujukan untuk pelaku usaha dan penggunaan bisnis yang sah.',
    ],
  },
  {
    h: '4. Masa percobaan',
    p: [
      'Kami menyediakan masa percobaan gratis selama jangka waktu tertentu dengan batas pemakaian. Setelah masa percobaan berakhir, kamu perlu berlangganan paket berbayar untuk melanjutkan layanan.',
    ],
  },
  {
    h: '5. Paket dan pembayaran',
    p: [
      'Setiap paket memiliki batas dan fitur masing masing yang ditampilkan pada halaman harga. Akses akan diaktifkan setelah pembayaran kami terima dan verifikasi.',
      'Biaya yang sudah dibayarkan pada umumnya tidak dapat dikembalikan, kecuali ditentukan lain oleh kami.',
    ],
  },
  {
    h: '6. Penggunaan yang dapat diterima',
    p: [
      'Kamu dilarang menggunakan SlaluDiskon untuk mengirim spam, penipuan, konten ilegal, atau melanggar hukum yang berlaku.',
      'Kamu wajib memperoleh persetujuan penerima sebelum mengirim pesan broadcast dan mematuhi kebijakan WhatsApp serta peraturan perlindungan data yang berlaku.',
    ],
  },
  {
    h: '7. Risiko terkait WhatsApp',
    p: [
      'SlaluDiskon terhubung ke WhatsApp melalui fitur perangkat tertaut. Penggunaan otomatisasi, balasan massal, dan broadcast membawa risiko nomor WhatsApp dibatasi atau diblokir oleh pihak WhatsApp.',
      'Risiko ini berada di luar kendali kami. Kamu menggunakan fitur otomatisasi dan broadcast atas tanggung jawab sendiri, dan SlaluDiskon tidak bertanggung jawab atas pemblokiran nomor yang terjadi.',
    ],
  },
  {
    h: '8. Batasan tanggung jawab',
    p: [
      'Layanan disediakan apa adanya. Sejauh diizinkan hukum, SlaluDiskon tidak bertanggung jawab atas kerugian tidak langsung, kehilangan keuntungan, atau kehilangan data yang timbul dari penggunaan layanan.',
    ],
  },
  {
    h: '9. Penghentian',
    p: [
      'Kami dapat menangguhkan atau menghentikan akun yang melanggar ketentuan ini. Kamu juga dapat berhenti berlangganan dan menutup akun kapan saja.',
    ],
  },
  {
    h: '10. Perubahan layanan dan ketentuan',
    p: [
      'Kami dapat memperbarui fitur, harga, maupun ketentuan ini dari waktu ke waktu. Perubahan penting akan kami informasikan melalui aplikasi atau email.',
    ],
  },
  {
    h: '11. Hukum yang berlaku',
    p: [
      'Syarat dan Ketentuan ini tunduk pada hukum yang berlaku di Republik Indonesia.',
    ],
  },
  {
    h: '12. Kontak',
    p: [
      'Untuk pertanyaan tentang ketentuan ini, hubungi kami di halo@slaludiskon.com.',
    ],
  },
];

export default function Terms() {
  const navigate = useNavigate();
  return (
    <Box sx={{ bgcolor: 'background.default', minHeight: '100vh' }}>
      <Box sx={{ borderBottom: '1px solid', borderColor: 'divider', bgcolor: '#fff' }}>
        <Container maxWidth="md" sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', py: 1.5 }}>
          <Box component="img" src={heroLogo} alt="SlaluDiskon" sx={{ height: 34, cursor: 'pointer' }} onClick={() => navigate('/')} />
          <Button startIcon={<ArrowBackIcon />} onClick={() => navigate('/')}>Beranda</Button>
        </Container>
      </Box>
      <Container maxWidth="md" sx={{ py: { xs: 5, md: 8 } }}>
        <Typography variant="h3" sx={{ fontWeight: 900, mb: 1 }}>Syarat dan Ketentuan</Typography>
        <Typography color="text.secondary" sx={{ mb: 4 }}>Terakhir diperbarui: Juni 2026</Typography>
        <Stack spacing={3}>
          {SECTIONS.map((s) => (
            <Box key={s.h}>
              <Typography variant="h6" sx={{ fontWeight: 800, mb: 1 }}>{s.h}</Typography>
              <Stack spacing={1}>
                {s.p.map((para, i) => (
                  <Typography key={i} color="text.secondary" sx={{ lineHeight: 1.7 }}>{para}</Typography>
                ))}
              </Stack>
            </Box>
          ))}
        </Stack>
        <Divider sx={{ my: 4 }} />
        <Typography variant="caption" color="text.secondary">© {new Date().getFullYear()} SlaluDiskon. Seluruh hak cipta dilindungi.</Typography>
      </Container>
    </Box>
  );
}
