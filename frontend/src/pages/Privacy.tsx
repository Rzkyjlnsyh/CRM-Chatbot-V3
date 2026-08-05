import { Box, Container, Typography, Stack, Divider, Button } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import heroLogo from '../assets/Logo-slaludiskon-gradients.png';

const SECTIONS: { h: string; p: string[] }[] = [
  {
    h: '1. Pengantar',
    p: [
      'Kebijakan Privasi ini menjelaskan bagaimana SlaluDiskon mengumpulkan, menggunakan, dan melindungi data kamu saat memakai layanan asisten WhatsApp bertenaga AI kami. Dengan mendaftar dan menggunakan SlaluDiskon, kamu menyetujui praktik yang dijelaskan di halaman ini.',
    ],
  },
  {
    h: '2. Data yang kami kumpulkan',
    p: [
      'Data akun: nama, email, nomor telepon, dan kata sandi (tersimpan dalam bentuk terenkripsi).',
      'Data bisnis: profil usaha, knowledge base, persona, serta pengaturan yang kamu buat agar AI dapat menjawab pelanggan.',
      'Data percakapan WhatsApp: pesan masuk dan kontak yang diproses agar AI dapat membalas dan menjalankan fitur seperti broadcast, follow up, dan deteksi order.',
      'Data pembayaran: informasi transaksi saat kamu berlangganan paket berbayar.',
      'Data teknis: alamat IP, jenis perangkat, dan catatan aktivitas untuk keamanan dan perbaikan layanan.',
      'Data atribusi iklan: halaman yang dikunjungi, kejadian seperti pendaftaran atau pembayaran, serta identifier cookie Meta jika pengukuran iklan sedang diaktifkan.',
    ],
  },
  {
    h: '3. Cara kami menggunakan data',
    p: [
      'Menjalankan layanan inti, yaitu membalas pelanggan secara otomatis dan menjalankan fitur yang kamu aktifkan.',
      'Mengelola akun, langganan, dan pembayaran kamu.',
      'Meningkatkan kualitas layanan, keamanan, dan dukungan pelanggan.',
      'Mengirim pemberitahuan penting terkait akun, tagihan, atau perubahan layanan.',
      'Mengukur hasil kampanye dan memperbaiki relevansi iklan tanpa menjual data pribadi kamu.',
    ],
  },
  {
    h: '4. Pihak ketiga',
    p: [
      'Untuk menjalankan layanan, kami menggunakan penyedia pihak ketiga, antara lain penyedia model AI untuk memproses dan menyusun jawaban, penyedia pengiriman email, gerbang pembayaran, serta layanan infrastruktur dan keamanan jaringan.',
      'Jika pengukuran iklan diaktifkan, kami memakai Meta Pixel dan Conversions API. Email, nomor telepon, dan ID akun di-hash sebelum dikirim melalui Conversions API; data atribusi seperti IP, perangkat, _fbp, dan _fbc dapat ikut diproses oleh Meta.',
      'Pihak ketiga hanya menerima data seperlunya untuk menjalankan fungsinya dan tidak kami izinkan memakai datamu untuk tujuan lain.',
    ],
  },
  {
    h: '5. Penyimpanan dan keamanan',
    p: [
      'Kami menyimpan data pada server yang dilindungi dan menerapkan langkah keamanan yang wajar untuk mencegah akses tidak sah. Kata sandi disimpan terenkripsi dan akses ke data dibatasi.',
      'Tidak ada sistem yang sepenuhnya bebas risiko, namun kami berupaya menjaga keamanan data semaksimal mungkin.',
    ],
  },
  {
    h: '6. Data WhatsApp dan retensi',
    p: [
      'Pesan dan kontak diproses untuk menjalankan fitur yang kamu pakai. Kamu dapat menghapus knowledge base, kontak, dan riwayat kapan saja melalui dashboard.',
      'Jika kamu menghapus akun, data terkait akun akan dihapus atau dianonimkan dalam jangka waktu yang wajar, kecuali yang wajib kami simpan oleh hukum.',
    ],
  },
  {
    h: '7. Hak kamu',
    p: [
      'Kamu berhak mengakses, memperbarui, dan meminta penghapusan data pribadimu. Kamu juga dapat berhenti berlangganan dan menghapus akun kapan saja.',
      'Untuk permintaan terkait data, hubungi kami lewat kontak di bawah.',
    ],
  },
  {
    h: '8. Cookie',
    p: [
      'Kami menggunakan cookie dan penyimpanan lokal agar kamu tetap masuk, layanan berjalan dengan baik, dan hasil iklan dapat diukur saat Meta Pixel diaktifkan. Cookie atribusi dapat mencakup _fbp dan _fbc. Kami tidak menjual data kamu kepada pihak lain.',
    ],
  },
  {
    h: '9. Perubahan kebijakan',
    p: [
      'Kebijakan ini dapat kami perbarui dari waktu ke waktu. Perubahan penting akan kami informasikan melalui aplikasi atau email.',
    ],
  },
  {
    h: '10. Kontak',
    p: [
      'Jika ada pertanyaan tentang Kebijakan Privasi ini, hubungi kami di halo@slaludiskon.com.',
    ],
  },
];

export default function Privacy() {
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
        <Typography variant="h3" sx={{ fontWeight: 900, mb: 1 }}>Kebijakan Privasi</Typography>
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
