import { useMemo, useRef, useState } from 'react';
import {
  Accordion, AccordionDetails, AccordionSummary, Alert, Box, Button, Card, CardContent, Chip,
  Dialog, DialogActions, DialogContent, DialogTitle, Divider, FormControlLabel, Grid,
  IconButton, MenuItem, Stack, Switch, TextField, Tooltip, Typography, CircularProgress,
  Paper, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import ArrowDownwardIcon from '@mui/icons-material/ArrowDownward';
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import InventoryIcon from '@mui/icons-material/Inventory2Outlined';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import SendIcon from '@mui/icons-material/Send';
import VisibilityIcon from '@mui/icons-material/VisibilityOutlined';
import { useProducts, useSaveProduct, useDeleteProduct, useSendProduct, useProductOrders, useGenerateProductAI } from '../hooks';
import type { CheckoutStepConfig, CheckoutStepType, Product, ProductButtonAction, ProductButtonConfig, ProductDetailItem, ProductOrder, ProductType } from '../types';
import { swalConfirm, swalToast } from '../services/swal';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';

const buttonActions: { value: ProductButtonAction; label: string }[] = [
  { value: 'checkout', label: 'Mulai checkout' },
  { value: 'ai', label: 'Jawab dengan AI' },
  { value: 'reply', label: 'Jawaban manual' },
  { value: 'handoff', label: 'Teruskan ke CS' },
];

const buttonIcons = ['none', '🛒', '💬', 'ℹ️', '👤', '📋', '📞', '✅', '🎁', '📅'];

function defaultButtonIcon(action: ProductButtonAction) {
  if (action === 'checkout') return '🛒';
  if (action === 'ai') return '💬';
  if (action === 'handoff') return '👤';
  return 'ℹ️';
}

const stepTypes: { value: CheckoutStepType; label: string }[] = [
  { value: 'text', label: 'Teks' },
  { value: 'number', label: 'Angka' },
  { value: 'select', label: 'Pilihan' },
];

const productTypes: { value: ProductType; label: string; helper: string }[] = [
  { value: 'physical', label: 'Produk fisik', helper: 'Barang yang dikirim atau diambil.' },
  { value: 'digital', label: 'Produk digital', helper: 'File, aplikasi, template, atau akses digital.' },
  { value: 'service', label: 'Jasa / layanan', helper: 'Pengerjaan, konsultasi, atau layanan profesional.' },
  { value: 'subscription', label: 'Langganan', helper: 'Membership atau layanan berulang.' },
  { value: 'event', label: 'Acara / kelas', helper: 'Kelas, webinar, workshop, atau acara.' },
  { value: 'donation', label: 'Donasi / penjemputan', helper: 'Donasi barang, pengumpulan, atau penjemputan.' },
  { value: 'other', label: 'Lainnya', helper: 'Gunakan informasi khusus buatan sendiri.' },
];

const productDetailPresets: Record<ProductType, string[]> = {
  physical: ['Varian / model', 'Ukuran', 'Warna', 'Bahan', 'Stok', 'Berat', 'Pengiriman', 'Garansi / retur'],
  digital: ['Format / jenis file', 'Cara akses', 'Cara pengiriman', 'Kompatibilitas', 'Lisensi penggunaan', 'Masa akses', 'Update', 'Dukungan'],
  service: ['Cakupan layanan', 'Hasil yang diterima', 'Durasi pengerjaan', 'Area layanan', 'Jadwal tersedia', 'Syarat pelanggan', 'Yang tidak termasuk', 'Revisi / garansi'],
  subscription: ['Durasi paket', 'Manfaat', 'Batas penggunaan', 'Cara akses', 'Perpanjangan', 'Pembatalan', 'Dukungan'],
  event: ['Tanggal dan waktu', 'Lokasi / platform', 'Durasi', 'Kapasitas', 'Fasilitas', 'Syarat peserta', 'Rekaman', 'Pembatalan'],
  donation: ['Barang yang diterima', 'Kondisi barang', 'Barang yang tidak diterima', 'Area penjemputan', 'Jadwal', 'Syarat penjemputan', 'Biaya'],
  other: ['Informasi penting', 'Pilihan tersedia', 'Cara memperoleh', 'Syarat dan batasan'],
};

const productDetailExamples: Record<ProductType, Record<string, string>> = {
  physical: {
    'Varian / model': 'Contoh: Reguler, Premium, Paket 3 pcs',
    Ukuran: 'Contoh: S, M, L, XL',
    Warna: 'Contoh: Hitam, Putih, Navy',
    Bahan: 'Contoh: Cotton combed 24s',
    Stok: 'Contoh: Tersedia atau dibuat sesuai pesanan',
    Berat: 'Contoh: 250 gram per pcs',
    Pengiriman: 'Contoh: JNE dan J&T ke seluruh Indonesia',
    'Garansi / retur': 'Contoh: Penukaran ukuran maksimal 3 hari',
  },
  digital: {
    'Format / jenis file': 'Contoh: PDF, ZIP, PSD, atau akses web',
    'Cara akses': 'Contoh: Tautan unduhan setelah pembayaran',
    'Cara pengiriman': 'Contoh: Dikirim melalui email atau WhatsApp',
    Kompatibilitas: 'Contoh: Windows 10+, macOS, Android',
    'Lisensi penggunaan': 'Contoh: Lisensi pribadi untuk satu pengguna',
    'Masa akses': 'Contoh: Akses selamanya atau selama 12 bulan',
    Update: 'Contoh: Update gratis selama satu tahun',
    Dukungan: 'Contoh: Bantuan instalasi melalui WhatsApp',
  },
  service: {
    'Cakupan layanan': 'Contoh: Konsultasi dan penyusunan strategi',
    'Hasil yang diterima': 'Contoh: Laporan PDF dan sesi konsultasi',
    'Durasi pengerjaan': 'Contoh: 3–5 hari kerja',
    'Area layanan': 'Contoh: Online seluruh Indonesia atau khusus Yogyakarta',
    'Jadwal tersedia': 'Contoh: Senin–Sabtu, pukul 09.00–17.00',
    'Syarat pelanggan': 'Contoh: Menyiapkan brief dan data pendukung',
    'Yang tidak termasuk': 'Contoh: Biaya iklan dan produksi konten',
    'Revisi / garansi': 'Contoh: Maksimal dua kali revisi',
  },
  subscription: {
    'Durasi paket': 'Contoh: Bulanan atau tahunan',
    Manfaat: 'Contoh: Akses semua materi dan grup komunitas',
    'Batas penggunaan': 'Contoh: Maksimal 5 pengguna',
    'Cara akses': 'Contoh: Login menggunakan email terdaftar',
    Perpanjangan: 'Contoh: Diperpanjang manual setiap bulan',
    Pembatalan: 'Contoh: Dapat dibatalkan sebelum periode berikutnya',
    Dukungan: 'Contoh: Dukungan chat pada jam kerja',
  },
  event: {
    'Tanggal dan waktu': 'Contoh: 20 Juli 2026, pukul 09.00 WIB',
    'Lokasi / platform': 'Contoh: Zoom atau Hotel ABC Yogyakarta',
    Durasi: 'Contoh: 2 jam',
    Kapasitas: 'Contoh: Maksimal 50 peserta',
    Fasilitas: 'Contoh: Materi, sertifikat, dan konsumsi',
    'Syarat peserta': 'Contoh: Membawa laptop pribadi',
    Rekaman: 'Contoh: Rekaman tersedia selama 30 hari',
    Pembatalan: 'Contoh: Tiket dapat dialihkan ke peserta lain',
  },
  donation: {
    'Barang yang diterima': 'Contoh: Sofa, meja, lemari, dan pakaian layak pakai',
    'Kondisi barang': 'Contoh: Bersih dan masih layak digunakan',
    'Barang yang tidak diterima': 'Contoh: Barang rusak berat atau limbah berbahaya',
    'Area penjemputan': 'Contoh: Yogyakarta dan sekitarnya',
    Jadwal: 'Contoh: Senin–Sabtu dengan konfirmasi petugas',
    'Syarat penjemputan': 'Contoh: Foto barang dan alamat lengkap',
    Biaya: 'Contoh: Gratis atau sesuai kesepakatan',
  },
  other: {
    'Informasi penting': 'Tuliskan fakta utama yang perlu diketahui pelanggan',
    'Pilihan tersedia': 'Tuliskan pilihan atau paket yang tersedia',
    'Cara memperoleh': 'Jelaskan bagaimana pelanggan mendapatkannya',
    'Syarat dan batasan': 'Tuliskan syarat atau batasan yang berlaku',
  },
};

function productDetailPlaceholder(type: ProductType, label: string) {
  return productDetailExamples[type][label.trim()] || `Tulis informasi untuk ${label.trim() || 'field ini'}`;
}

function suggestedProductDetails(type: ProductType): ProductDetailItem[] {
  return productDetailPresets[type].map(label => ({ label, value: '' }));
}

function parseProductDetails(raw: string | undefined, type: ProductType): ProductDetailItem[] {
  if (raw) {
    try {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed) && parsed.length) return parsed as ProductDetailItem[];
    } catch { /* tampilkan saran bawaan */ }
  }
  return suggestedProductDetails(type);
}

function defaultButtons(): ProductButtonConfig[] {
  return [
    { key: 'order', label: 'Pesan Sekarang', icon: '🛒', action: 'checkout' },
    { key: 'ask', label: 'Tanya Detail', icon: '💬', action: 'ai' },
  ];
}

function checkoutStepsForType(type: ProductType): CheckoutStepConfig[] {
  const note = { key: 'note', label: 'Ada catatan tambahan? Ketik lewati jika tidak ada.', type: 'text' as const, required: false };
  switch (type) {
    case 'digital': return [
      { key: 'quantity', label: 'Mau pesan berapa lisensi atau akses?', type: 'number', required: true },
      { key: 'customer_name', label: 'Pesanan ini atas nama siapa?', type: 'text', required: true },
      { key: 'delivery_account', label: 'Kirim akses ke email atau akun mana?', type: 'text', required: true }, note,
    ];
    case 'service': return [
      { key: 'customer_name', label: 'Layanan ini atas nama siapa?', type: 'text', required: true },
      { key: 'need', label: 'Ceritakan kebutuhan atau hasil yang diinginkan.', type: 'text', required: true },
      { key: 'schedule', label: 'Kapan waktu layanan yang diinginkan?', type: 'text', required: true },
      { key: 'location', label: 'Layanan dilakukan online atau di lokasi mana?', type: 'text', required: false }, note,
    ];
    case 'subscription': return [
      { key: 'customer_name', label: 'Langganan ini atas nama siapa?', type: 'text', required: true },
      { key: 'package', label: 'Paket atau durasi mana yang dipilih?', type: 'text', required: true },
      { key: 'account', label: 'Akun atau email yang akan memakai layanan?', type: 'text', required: true },
      { key: 'start_date', label: 'Kapan langganan ingin dimulai?', type: 'text', required: false }, note,
    ];
    case 'event': return [
      { key: 'participant_name', label: 'Pendaftaran peserta atas nama siapa?', type: 'text', required: true },
      { key: 'participants', label: 'Berapa peserta yang akan ikut?', type: 'number', required: true },
      { key: 'session', label: 'Jadwal atau sesi mana yang dipilih?', type: 'text', required: true },
      { key: 'contact', label: 'Email atau kontak peserta yang digunakan?', type: 'text', required: false }, note,
    ];
    case 'donation': return [
      { key: 'donor_name', label: 'Boleh dibantu nama lengkapnya?', type: 'text', required: true },
      { key: 'items', label: 'Barang apa saja yang akan didonasikan?', type: 'text', required: true },
      { key: 'condition', label: 'Bagaimana kondisi barangnya?', type: 'text', required: true },
      { key: 'pickup_address', label: 'Di mana alamat lengkap penjemputannya?', type: 'text', required: true },
      { key: 'pickup_schedule', label: 'Kapan waktu penjemputan yang diharapkan?', type: 'text', required: false }, note,
    ];
    case 'other': return [
      { key: 'customer_name', label: 'Permintaan ini atas nama siapa?', type: 'text', required: true },
      { key: 'need', label: 'Pilihan atau kebutuhan apa yang diinginkan?', type: 'text', required: true }, note,
    ];
    default: return [
      { key: 'quantity', label: 'Mau pesan berapa?', type: 'number', required: true },
      { key: 'customer_name', label: 'Pesanan ini atas nama siapa?', type: 'text', required: true },
      { key: 'address', label: 'Kirim ke alamat lengkap mana?', type: 'text', required: true }, note,
    ];
  }
}

function defaultSteps(): CheckoutStepConfig[] {
  return checkoutStepsForType('physical');
}

function legacyPhysicalSteps(): CheckoutStepConfig[] {
  return [
    { key: 'quantity', label: 'Mau pesan berapa?', type: 'number', required: true },
    { key: 'customer_name', label: 'Pesanan ini atas nama siapa?', type: 'text', required: true },
    { key: 'address', label: 'Kirim ke alamat lengkap mana?', type: 'text', required: true },
    { key: 'note', label: 'Ada catatan untuk pesanan? Ketik lewati jika tidak ada.', type: 'text', required: false },
  ];
}

function safeParse<T>(raw: string | undefined, fallback: T): T {
  if (!raw) return fallback;
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) && parsed.length ? parsed as T : fallback;
  } catch {
    return fallback;
  }
}

function cleanKey(value: string, fallback: string) {
  const cleaned = value.toLowerCase().trim().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '').slice(0, 40);
  return cleaned || fallback;
}

function productImageSrc(product: Product) {
  return product.image_url || '';
}

function orderStatusLabel(status: string) {
  if (status === 'confirmed') return 'Terkonfirmasi';
  if (status === 'pending_cs') return 'Menunggu CS';
  if (status === 'cancelled') return 'Dibatalkan';
  return status || 'Baru';
}

function orderStatusColor(status: string): 'success' | 'warning' | 'default' {
  if (status === 'confirmed') return 'success';
  if (status === 'pending_cs') return 'warning';
  return 'default';
}

function formatOrderDate(value: string) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return new Intl.DateTimeFormat('id-ID', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function orderDetailRows(order: ProductOrder) {
  const summary = (order.summary || '').replace(/\*/g, '');
  return summary
    .split('\n')
    .map(line => line.trim())
    .filter(line => line && !line.toLowerCase().startsWith('ringkasan pesanan'))
    .map(line => {
      const separator = line.indexOf(':');
      if (separator === -1) return { label: 'Info', value: line };
      return {
        label: line.slice(0, separator).trim(),
        value: line.slice(separator + 1).trim(),
      };
    })
    .filter(row => row.value && row.label.toLowerCase() !== 'produk');
}

export default function ProductPanel({ agentId }: { agentId: number }) {
  const { data: products, isLoading } = useProducts(agentId);
  const { data: orders } = useProductOrders(agentId);
  const saveMut = useSaveProduct(agentId);
  const deleteMut = useDeleteProduct(agentId);
  const sendMut = useSendProduct(agentId);
  const generateProductAI = useGenerateProductAI(agentId);

  const [open, setOpen] = useState(false);
  const [selectedOrder, setSelectedOrder] = useState<ProductOrder | null>(null);
  const [orderPage, setOrderPage] = useState(0);
  const [orderRowsPerPage, setOrderRowsPerPage] = useState(5);
  const [editId, setEditId] = useState<number | null>(null);
  const [name, setName] = useState('');
  const [productType, setProductType] = useState<ProductType>('physical');
  const [price, setPrice] = useState('');
  const [description, setDescription] = useState('');
  const [productDetails, setProductDetails] = useState<ProductDetailItem[]>(suggestedProductDetails('physical'));
  const [knowledge, setKnowledge] = useState('');
  const [aiSalesGuidance, setAISalesGuidance] = useState('');
  const [image, setImage] = useState<File | null>(null);
  const [imagePreview, setImagePreview] = useState('');
  const [buttons, setButtons] = useState<ProductButtonConfig[]>(defaultButtons);
  const [checkoutSteps, setCheckoutSteps] = useState<CheckoutStepConfig[]>(defaultSteps);
  const [checkoutHandoff, setCheckoutHandoff] = useState(true);
  const [checkoutSuccessMessage, setCheckoutSuccessMessage] = useState('Pesanan *{order_code}* berhasil dicatat. CS kami akan memeriksa dan melanjutkan pesanan ini.');
  const fileRef = useRef<HTMLInputElement>(null);

  const [sendOpen, setSendOpen] = useState(false);
  const [sendPid, setSendPid] = useState<number | null>(null);
  const [sendTo, setSendTo] = useState('');

  const hasCheckout = useMemo(() => buttons.some(b => b.action === 'checkout'), [buttons]);

  const resetForm = () => {
    setName('');
    setProductType('physical');
    setPrice('');
    setDescription('');
    setProductDetails(suggestedProductDetails('physical'));
    setKnowledge('');
    setAISalesGuidance('');
    setImage(null);
    setImagePreview('');
    setButtons(defaultButtons());
    setCheckoutSteps(defaultSteps());
    setCheckoutHandoff(true);
    setCheckoutSuccessMessage('Pesanan *{order_code}* berhasil dicatat. CS kami akan memeriksa dan melanjutkan pesanan ini.');
  };

  const openNew = () => {
    setEditId(null);
    resetForm();
    setOpen(true);
  };

  const openEdit = (p: Product) => {
    setEditId(p.id);
    setName(p.name);
    const nextType = p.product_type || 'physical';
    setProductType(nextType);
    setPrice(p.price);
    setDescription(p.description);
    setProductDetails(parseProductDetails(p.details_json, nextType));
    setKnowledge(p.knowledge || '');
    setAISalesGuidance(p.ai_sales_guidance || '');
    setImage(null);
    setImagePreview(productImageSrc(p));
    setButtons(safeParse<ProductButtonConfig[]>(p.buttons_json, defaultButtons()));
    setCheckoutSteps(safeParse<CheckoutStepConfig[]>(p.checkout_steps_json, checkoutStepsForType(nextType)));
    setCheckoutHandoff(p.checkout_handoff ?? true);
    setCheckoutSuccessMessage(p.checkout_success_message || 'Pesanan *{order_code}* berhasil dicatat. CS kami akan memeriksa dan melanjutkan pesanan ini.');
    setOpen(true);
  };

  const handleImage = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0];
    if (!f) return;
    setImage(f);
    setImagePreview(URL.createObjectURL(f));
  };

  const updateButton = (index: number, patch: Partial<ProductButtonConfig>) => {
    setButtons(prev => prev.map((button, i) => i === index ? { ...button, ...patch } : button));
  };

  const updateProductDetail = (index: number, patch: Partial<ProductDetailItem>) => {
    setProductDetails(prev => prev.map((detail, i) => i === index ? { ...detail, ...patch } : detail));
  };

  const changeProductType = (nextType: ProductType) => {
    const serializedCheckout = JSON.stringify(checkoutSteps);
    const checkoutStillUsesPreset = serializedCheckout === JSON.stringify(checkoutStepsForType(productType))
      || serializedCheckout === JSON.stringify(legacyPhysicalSteps());
    setProductType(nextType);
    if (productDetails.every(detail => !detail.value.trim())) {
      setProductDetails(suggestedProductDetails(nextType));
    }
    if (checkoutStillUsesPreset) {
      setCheckoutSteps(checkoutStepsForType(nextType));
    }
  };

  const addSuggestedDetails = () => {
    const existing = new Set(productDetails.map(detail => detail.label.trim().toLowerCase()));
    const additions = suggestedProductDetails(productType).filter(detail => !existing.has(detail.label.toLowerCase()));
    setProductDetails(prev => [...prev, ...additions]);
  };

  const addButton = () => {
    if (buttons.length >= 3) return;
    const next = buttons.length + 1;
    setButtons(prev => [...prev, { key: `button_${next}`, label: `Tombol ${next}`, icon: 'ℹ️', action: 'reply', response: '' }]);
  };

  const updateStep = (index: number, patch: Partial<CheckoutStepConfig>) => {
    setCheckoutSteps(prev => prev.map((step, i) => i === index ? { ...step, ...patch } : step));
  };

  const moveStep = (index: number, direction: -1 | 1) => {
    const target = index + direction;
    if (target < 0 || target >= checkoutSteps.length) return;
    setCheckoutSteps(prev => {
      const copy = [...prev];
      [copy[index], copy[target]] = [copy[target], copy[index]];
      return copy;
    });
  };

  const addStep = () => {
    const next = checkoutSteps.length + 1;
    setCheckoutSteps(prev => [...prev, { key: `field_${next}`, label: 'Pertanyaan checkout baru', type: 'text', required: true }]);
  };

  const validateForm = () => {
    if (!name.trim()) return 'Nama produk wajib diisi';
    const completedDetails = productDetails.filter(detail => detail.label.trim() && detail.value.trim());
    if (completedDetails.length > 30) return 'Informasi terstruktur maksimal 30 field';
    if (new Set(completedDetails.map(detail => detail.label.trim().toLowerCase())).size !== completedDetails.length) return 'Label informasi produk tidak boleh duplikat';
    if (knowledge.length > 20000) return 'Knowledge produk maksimal 20.000 karakter';
    if (aiSalesGuidance.length > 8000) return 'Arahan AI maksimal 8.000 karakter';
    if (!buttons.length || buttons.length > 3) return 'Produk membutuhkan 1 sampai 3 tombol';
    if (buttons.some(b => !b.label.trim())) return 'Label tombol tidak boleh kosong';
    if (buttons.some(b => b.action === 'reply' && !b.response?.trim())) return 'Tombol jawaban manual membutuhkan teks balasan';
    if (hasCheckout && !checkoutSteps.length) return 'Checkout membutuhkan minimal 1 langkah';
    if (hasCheckout && checkoutSteps.some(s => !s.label.trim())) return 'Pertanyaan checkout tidak boleh kosong';
    if (hasCheckout && checkoutSteps.some(s => s.type === 'select' && (s.options || []).filter(Boolean).length < 2)) return 'Langkah pilihan membutuhkan minimal 2 opsi';
    return '';
  };

  const submit = async () => {
    const error = validateForm();
    if (error) {
      swalToast(error, 'warning');
      return;
    }
    const normalizedButtons = buttons.map((button, index) => ({
      ...button,
      key: cleanKey(button.key, `button_${index + 1}`),
      label: button.label.trim(),
      response: button.response?.trim() || '',
    }));
    const normalizedSteps = checkoutSteps.map((step, index) => ({
      ...step,
      key: cleanKey(step.key, `field_${index + 1}`),
      label: step.label.trim(),
      options: step.type === 'select' ? (step.options || []).map(o => o.trim()).filter(Boolean) : undefined,
    }));
    const fd = new FormData();
    fd.append('name', name.trim());
    fd.append('product_type', productType);
    fd.append('price', price.trim());
    fd.append('description', description.trim());
    fd.append('details_json', JSON.stringify(productDetails.map(detail => ({ label: detail.label.trim(), value: detail.value.trim() })).filter(detail => detail.label && detail.value)));
    fd.append('knowledge', knowledge.trim());
    fd.append('ai_sales_guidance', aiSalesGuidance.trim());
    fd.append('buttons_json', JSON.stringify(normalizedButtons));
    fd.append('checkout_steps_json', JSON.stringify(hasCheckout && normalizedSteps.length ? normalizedSteps : defaultSteps()));
    fd.append('checkout_handoff', String(checkoutHandoff));
    fd.append('checkout_success_message', checkoutSuccessMessage.trim());
    if (image) fd.append('image', image);
    try {
      await saveMut.mutateAsync({ id: editId ?? undefined, fd });
      setOpen(false);
      resetForm();
      swalToast(editId ? 'Produk diperbarui' : 'Produk ditambahkan', 'success');
    } catch (e: any) {
      swalToast(e?.response?.data?.error || 'Gagal menyimpan produk', 'error');
    }
  };

  const generateKnowledgeDraft = async () => {
    const detailFacts = productDetails.filter(detail => detail.label.trim() && detail.value.trim());
    const availableFacts = description.trim().length + detailFacts.reduce((total, detail) => total + detail.label.length + detail.value.length, 0);
    if (!name.trim() || availableFacts < 15) {
      swalToast('Isi nama, deskripsi, atau beberapa informasi produk terlebih dahulu', 'warning');
      return;
    }
    if ((knowledge.trim() || aiSalesGuidance.trim()) && !await swalConfirm('Ganti draft knowledge dan arahan AI yang sekarang dengan hasil baru?')) {
      return;
    }
    try {
      const result = await generateProductAI.mutateAsync({
        name: name.trim(),
        product_type: productType,
        price: price.trim(),
        description: description.trim(),
        details_json: JSON.stringify(productDetails.map(detail => ({ label: detail.label.trim(), value: detail.value.trim() })).filter(detail => detail.label && detail.value)),
        existing_knowledge: knowledge.trim(),
        checkout_enabled: hasCheckout,
      });
      setKnowledge(result.knowledge || '');
      setAISalesGuidance(result.ai_sales_guidance || '');
      swalToast('Draft AI selesai. Periksa hasilnya lalu simpan produk.', 'success');
    } catch (error: any) {
      swalToast(error?.response?.data?.error || 'AI belum bisa membuat draft produk', 'error');
    }
  };

  const remove = async (p: Product) => {
    if (!await swalConfirm(`Hapus "${p.name}"?`)) return;
    try {
      await deleteMut.mutateAsync(p.id);
      swalToast('Produk dihapus', 'success');
    } catch {
      swalToast('Gagal menghapus', 'error');
    }
  };

  const sendProduct = async () => {
    if (!sendPid || !sendTo.trim()) {
      swalToast('Nomor tujuan wajib diisi', 'warning');
      return;
    }
    try {
      await sendMut.mutateAsync({ pid: sendPid, to: sendTo.trim() });
      setSendOpen(false);
      setSendTo('');
      swalToast('Produk terkirim', 'success');
    } catch (e: any) {
      swalToast(e?.response?.data?.error || 'Gagal mengirim', 'error');
    }
  };

  if (isLoading) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;

  return (
    <Box>
      <PageHeader
        title="Katalog Produk"
        subtitle="Kelola produk, tombol WhatsApp, dan alur checkout otomatis dari satu tempat."
        action={<Button variant="contained" startIcon={<AddIcon />} onClick={openNew}>Tambah Produk</Button>}
      />

      {(!products || products.length === 0) ? (
        <EmptyState
          icon={<InventoryIcon sx={{ fontSize: 48 }} />}
          title="Belum ada produk"
          description="Tambahkan produk agar bisa dikirim ke pelanggan saat mereka bertanya."
          actionLabel="Tambah Produk"
          onAction={openNew}
        />
      ) : (
        <Grid container spacing={1.5}>
          {products.map(p => {
            const productButtons = safeParse<ProductButtonConfig[]>(p.buttons_json, defaultButtons());
            return (
              <Grid key={p.id} size={{ xs: 12, sm: 6, md: 4 }}>
                <Card variant="outlined" sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                  {productImageSrc(p) ? (
                    <Box sx={{ height: 160, bgcolor: 'action.hover', display: 'flex', alignItems: 'center', justifyContent: 'center', overflow: 'hidden' }}>
                      <Box component="img"
                        src={productImageSrc(p)}
                        alt={p.name}
                        sx={{ maxWidth: '100%', maxHeight: '100%', objectFit: 'contain' }}
                      />
                    </Box>
                  ) : (
                    <Box sx={{ height: 120, bgcolor: 'action.hover', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                      <InventoryIcon sx={{ fontSize: 48, color: 'text.disabled' }} />
                    </Box>
                  )}
                  <CardContent sx={{ flex: 1, pb: '8px !important' }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>{p.name}</Typography>
                    {p.price && <Chip size="small" label={p.price} color="primary" variant="outlined" sx={{ mt: 0.25, mb: 0.5 }} />}
                    <Stack direction="row" spacing={0.5} sx={{ flexWrap: 'wrap', gap: 0.5, mb: 0.75 }}>
                      {productButtons.map(button => <Chip key={button.key} size="small" label={button.label} variant="outlined" />)}
                    </Stack>
                    {p.description && (
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5, lineHeight: 1.4 }}>
                        {p.description.length > 90 ? p.description.slice(0, 90) + '...' : p.description}
                      </Typography>
                    )}
                  </CardContent>
                  <Stack direction="row" spacing={0.5} sx={{ px: 1, pb: 1, justifyContent: 'flex-end' }}>
                    <IconButton size="small" color="primary" title="Kirim ke pelanggan" onClick={() => { setSendPid(p.id); setSendOpen(true); }}>
                      <SendIcon fontSize="small" />
                    </IconButton>
                    <IconButton size="small" title="Edit" onClick={() => openEdit(p)}><EditIcon fontSize="small" /></IconButton>
                    <IconButton size="small" color="error" title="Hapus" onClick={() => remove(p)}><DeleteIcon fontSize="small" /></IconButton>
                  </Stack>
                </Card>
              </Grid>
            );
          })}
        </Grid>
      )}

      {!!orders?.length && (
        <Paper variant="outlined" sx={{ mt: 2, borderRadius: 1, overflow: 'hidden' }}>
          <Stack direction="row" spacing={1} sx={{ px: 1.5, py: 1.1, justifyContent: 'space-between', alignItems: 'center', borderBottom: '1px solid', borderColor: 'divider' }}>
            <Box sx={{ minWidth: 0 }}>
              <Typography variant="subtitle1" sx={{ fontWeight: 800, lineHeight: 1.2 }}>Pesanan terbaru</Typography>
              <Typography variant="caption" color="text.secondary">Checkout produk yang sudah dikonfirmasi.</Typography>
            </Box>
            <Chip size="small" color="primary" variant="outlined" label={orders.length} />
          </Stack>

          <TableContainer sx={{ display: { xs: 'none', md: 'block' } }}>
            <Table size="small" aria-label="Pesanan terbaru">
              <TableHead>
                <TableRow>
                  <TableCell sx={{ fontWeight: 800 }}>Kode</TableCell>
                  <TableCell sx={{ fontWeight: 800 }}>Produk</TableCell>
                  <TableCell sx={{ fontWeight: 800 }}>Pelanggan</TableCell>
                  <TableCell sx={{ fontWeight: 800 }}>Waktu</TableCell>
                  <TableCell sx={{ fontWeight: 800 }}>Status</TableCell>
                  <TableCell align="right" sx={{ fontWeight: 800, width: 64 }}>Detail</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {orders.slice(orderPage * orderRowsPerPage, orderPage * orderRowsPerPage + orderRowsPerPage).map(order => (
                  <TableRow key={order.id} hover>
                    <TableCell><Typography variant="body2" sx={{ fontWeight: 750 }}>{order.order_code}</Typography></TableCell>
                    <TableCell sx={{ maxWidth: 220 }}>
                      <Typography variant="body2" noWrap title={order.product?.name || 'Produk'}>{order.product?.name || 'Produk'}</Typography>
                    </TableCell>
                    <TableCell><Typography variant="body2">+{order.sender}</Typography></TableCell>
                    <TableCell><Typography variant="caption" color="text.secondary">{formatOrderDate(order.created_at) || '—'}</Typography></TableCell>
                    <TableCell><Chip size="small" color={orderStatusColor(order.status)} label={orderStatusLabel(order.status)} sx={{ height: 22 }} /></TableCell>
                    <TableCell align="right">
                      <Tooltip title="Lihat detail"><IconButton size="small" onClick={() => setSelectedOrder(order)}><VisibilityIcon fontSize="small" /></IconButton></Tooltip>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>

          <Stack divider={<Divider flexItem />} sx={{ display: { xs: 'flex', md: 'none' } }}>
            {orders.slice(orderPage * orderRowsPerPage, orderPage * orderRowsPerPage + orderRowsPerPage).map(order => (
              <Button key={order.id} color="inherit" onClick={() => setSelectedOrder(order)}
                sx={{ px: 1.25, py: 1, justifyContent: 'flex-start', textAlign: 'left', textTransform: 'none', borderRadius: 0 }}>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
                    <Typography variant="body2" sx={{ fontWeight: 800 }}>{order.order_code}</Typography>
                    <Chip size="small" color={orderStatusColor(order.status)} label={orderStatusLabel(order.status)} sx={{ height: 20, fontSize: 10.5 }} />
                  </Stack>
                  <Typography variant="body2" noWrap sx={{ mt: 0.25 }}>{order.product?.name || 'Produk'}</Typography>
                  <Typography variant="caption" color="text.secondary">+{order.sender} · {formatOrderDate(order.created_at) || 'Waktu tidak terbaca'}</Typography>
                </Box>
                <VisibilityIcon sx={{ ml: 1, fontSize: 18, color: 'text.secondary', flexShrink: 0 }} />
              </Button>
            ))}
          </Stack>

          <TablePagination
            component="div"
            count={orders.length}
            page={Math.min(orderPage, Math.max(0, Math.ceil(orders.length / orderRowsPerPage) - 1))}
            onPageChange={(_event, nextPage) => setOrderPage(nextPage)}
            rowsPerPage={orderRowsPerPage}
            onRowsPerPageChange={event => { setOrderRowsPerPage(Number(event.target.value)); setOrderPage(0); }}
            rowsPerPageOptions={[5, 10, 20]}
            labelRowsPerPage="Baris"
            labelDisplayedRows={({ from, to, count }) => `${from}–${to} dari ${count}`}
            sx={{ borderTop: '1px solid', borderColor: 'divider', '& .MuiTablePagination-toolbar': { minHeight: 44 }, '& .MuiTablePagination-selectLabel': { display: { xs: 'none', sm: 'block' } } }}
          />
        </Paper>
      )}

      <Dialog open={!!selectedOrder} onClose={() => setSelectedOrder(null)} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ pb: 1 }}>
          <Stack direction="row" spacing={1} sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
            <Box>
              <Typography variant="subtitle1" sx={{ fontWeight: 850 }}>Detail pesanan</Typography>
              <Typography variant="caption" color="text.secondary">{selectedOrder?.order_code}</Typography>
            </Box>
            {selectedOrder && <Chip size="small" color={orderStatusColor(selectedOrder.status)} label={orderStatusLabel(selectedOrder.status)} />}
          </Stack>
        </DialogTitle>
        <DialogContent dividers>
          {selectedOrder && (
            <Stack spacing={1.25}>
              <Grid container spacing={1}>
                <Grid size={{ xs: 12, sm: 6 }}>
                  <Typography variant="caption" color="text.secondary">Produk</Typography>
                  <Typography variant="body2" sx={{ fontWeight: 700 }}>{selectedOrder.product?.name || 'Produk'}</Typography>
                </Grid>
                <Grid size={{ xs: 12, sm: 6 }}>
                  <Typography variant="caption" color="text.secondary">Pelanggan</Typography>
                  <Typography variant="body2" sx={{ fontWeight: 700 }}>+{selectedOrder.sender}</Typography>
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <Typography variant="caption" color="text.secondary">Dibuat</Typography>
                  <Typography variant="body2">{formatOrderDate(selectedOrder.created_at) || 'Waktu tidak terbaca'}</Typography>
                </Grid>
              </Grid>
              <Divider />
              <Grid container spacing={1}>
                {orderDetailRows(selectedOrder).length ? orderDetailRows(selectedOrder).map((row, index) => (
                  <Grid key={`${row.label}-${index}`} size={{ xs: 12, sm: 6 }}>
                    <Box sx={{ bgcolor: 'action.hover', borderRadius: 1, px: 1, py: 0.75, height: '100%' }}>
                      <Typography variant="caption" color="text.secondary">{row.label}</Typography>
                      <Typography variant="body2" sx={{ mt: 0.2, wordBreak: 'break-word' }}>{row.value}</Typography>
                    </Box>
                  </Grid>
                )) : <Grid size={{ xs: 12 }}><Typography variant="body2" color="text.secondary">Belum ada detail tambahan.</Typography></Grid>}
              </Grid>
            </Stack>
          )}
        </DialogContent>
        <DialogActions><Button onClick={() => setSelectedOrder(null)}>Tutup</Button></DialogActions>
      </Dialog>

      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="md" fullWidth>
        <DialogTitle>{editId ? 'Edit Produk' : 'Tambah Produk'}</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <TextField label="Nama produk *" size="small" fullWidth value={name}
              onChange={e => setName(e.target.value)} placeholder="Kaos Polos Premium" />
            <TextField label="Jenis produk" size="small" select fullWidth value={productType}
              onChange={e => changeProductType(e.target.value as ProductType)}
              helperText={productTypes.find(item => item.value === productType)?.helper}>
              {productTypes.map(item => <MenuItem key={item.value} value={item.value}>{item.label}</MenuItem>)}
            </TextField>
            <TextField label="Harga / biaya (opsional)" size="small" fullWidth value={price}
              onChange={e => setPrice(e.target.value)} placeholder="Rp 75.000" />
            <TextField label="Deskripsi untuk pelanggan" size="small" fullWidth multiline rows={3} value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="Jelaskan produk secara singkat dan menarik. Deskripsi ini dapat tampil di pesan produk." />

            <Accordion variant="outlined" defaultExpanded>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                  <Typography variant="subtitle2">Informasi produk</Typography>
                  <Chip size="small" label={`${productDetails.filter(detail => detail.value.trim()).length} terisi`} />
                </Stack>
              </AccordionSummary>
              <AccordionDetails>
                <Stack spacing={1}>
                  <Alert severity="info">
                    Isi yang memang diketahui saja. Field kosong tidak disimpan. Informasi ini menjadi sumber fakta utama saat AI membuat FAQ dan menjawab pelanggan.
                  </Alert>
                  {productDetails.map((detail, index) => (
                    <Grid container spacing={1} key={`${detail.label}-${index}`} sx={{ alignItems: 'center' }}>
                      <Grid size={{ xs: 12, sm: 4 }}>
                        <TextField size="small" fullWidth label="Nama informasi" value={detail.label}
                          onChange={e => updateProductDetail(index, { label: e.target.value })}
                          placeholder="Contoh: Ukuran" />
                      </Grid>
                      <Grid size={{ xs: 10, sm: 7 }}>
                        <TextField size="small" fullWidth label="Isi" value={detail.value}
                          onChange={e => updateProductDetail(index, { value: e.target.value })}
                          placeholder={productDetailPlaceholder(productType, detail.label)} />
                      </Grid>
                      <Grid size={{ xs: 2, sm: 1 }} sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                        <Tooltip title="Hapus informasi">
                          <IconButton size="small" color="error" onClick={() => setProductDetails(prev => prev.filter((_, i) => i !== index))}>
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </Grid>
                    </Grid>
                  ))}
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={0.75}>
                    <Button size="small" variant="outlined" startIcon={<AddIcon />}
                      disabled={productDetails.length >= 30}
                      onClick={() => setProductDetails(prev => [...prev, { label: '', value: '' }])}>
                      Tambah informasi
                    </Button>
                    <Button size="small" variant="text" onClick={addSuggestedDetails}>
                      Tambahkan saran untuk jenis ini
                    </Button>
                  </Stack>
                </Stack>
              </AccordionDetails>
            </Accordion>
            <Box>
              <Typography variant="caption" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>Gambar produk</Typography>
              <input ref={fileRef} type="file" accept="image/*" onChange={handleImage} style={{ display: 'none' }} />
              <Button size="small" variant="outlined" onClick={() => fileRef.current?.click()}>
                {image || imagePreview ? 'Ganti gambar' : 'Pilih gambar'}
              </Button>
              {imagePreview && (
                <Box sx={{ mt: 1, maxWidth: 200, borderRadius: 1, overflow: 'hidden' }}>
                  <img src={imagePreview} alt="Preview" style={{ width: '100%', display: 'block' }} />
                </Box>
              )}
            </Box>

            <Accordion variant="outlined" defaultExpanded>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center', justifyContent: 'space-between', width: '100%', minWidth: 0, pr: 1 }}>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minWidth: 0 }}>
                    <Typography variant="subtitle2">Pengetahuan & cara menawarkan</Typography>
                    <Chip size="small" color={knowledge.trim() ? 'success' : 'default'} label={knowledge.trim() ? 'Terisi' : 'Belum lengkap'} />
                  </Stack>
                </Stack>
              </AccordionSummary>
              <AccordionDetails>
                <Stack spacing={1.25}>
                  <Alert severity="info">
                    Informasi di sini hanya berlaku untuk produk ini dan diprioritaskan di atas Knowledge umum. Tidak perlu menyalin fakta yang sama ke menu Asisten AI.
                  </Alert>
                  <Button
                    variant="outlined"
                    size="small"
                    startIcon={generateProductAI.isPending ? <CircularProgress size={15} /> : <AutoAwesomeIcon />}
                    disabled={generateProductAI.isPending}
                    onClick={() => void generateKnowledgeDraft()}
                    sx={{ alignSelf: 'flex-start' }}
                  >
                    {generateProductAI.isPending ? 'AI sedang menyusun...' : 'Buat dengan AI'}
                  </Button>
                  <TextField
                    label="Fakta dan FAQ khusus produk"
                    size="small"
                    fullWidth
                    multiline
                    minRows={5}
                    value={knowledge}
                    onChange={e => setKnowledge(e.target.value)}
                    placeholder={'Contoh:\n- Tersedia ukuran S sampai XXL.\n- Bahan cotton combed 24s.\n- Bisa dikirim ke seluruh Indonesia.\n- Penukaran ukuran maksimal 3 hari setelah diterima.'}
                    helperText={`${knowledge.length.toLocaleString('id-ID')}/20.000 karakter · Isi fakta, batas layanan, varian, pengiriman, dan FAQ yang benar-benar khusus produk ini.`}
                  />
                  <TextField
                    label="Arahan AI saat menawarkan produk"
                    size="small"
                    fullWidth
                    multiline
                    minRows={3}
                    value={aiSalesGuidance}
                    onChange={e => setAISalesGuidance(e.target.value)}
                    placeholder={'Contoh: Setelah menjawab, tanyakan satu hal yang paling relevan: ukuran, warna, atau jumlah. Jika pelanggan sudah siap membeli, arahkan ke checkout. Jangan memaksa dan jangan menanyakan data yang sudah diberikan.'}
                    helperText={`${aiSalesGuidance.length.toLocaleString('id-ID')}/8.000 karakter · Atur gaya follow-up dan pertanyaan berikutnya, bukan fakta produk.`}
                  />
                </Stack>
              </AccordionDetails>
            </Accordion>

            <Accordion variant="outlined" defaultExpanded>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                  <Typography variant="subtitle2">Tombol WhatsApp</Typography>
                  <Chip size="small" label={`${buttons.length}/3`} />
                </Stack>
              </AccordionSummary>
              <AccordionDetails>
                <Stack spacing={1.25}>
                  <Alert severity="info" icon={false}>
                    Nama, harga, deskripsi, fakta khusus, dan arahan menawarkan otomatis menjadi knowledge produk ini. Tombol <b>Jawab dengan AI</b> mengutamakan knowledge produk yang dipilih, lalu memakai Knowledge umum hanya jika masih relevan dan tidak bertentangan.
                  </Alert>
                  {buttons.map((button, index) => (
                    <Box key={`${button.key}-${index}`} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1.25 }}>
                      <Grid container spacing={1}>
                        <Grid size={{ xs: 4, md: 2 }}>
                          <TextField label="Ikon" size="small" select fullWidth value={button.icon ?? defaultButtonIcon(button.action)}
                            onChange={e => updateButton(index, { icon: e.target.value })}>
                            {buttonIcons.map(icon => <MenuItem key={icon} value={icon}>{icon === 'none' ? 'Tanpa ikon' : icon}</MenuItem>)}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 8, md: 3 }}>
                          <TextField label="Label tombol" size="small" fullWidth value={button.label}
                            onChange={e => updateButton(index, { label: e.target.value })} />
                        </Grid>
                        <Grid size={{ xs: 12, md: 3 }}>
                          <TextField label="Aksi" size="small" select fullWidth value={button.action}
                            onChange={e => {
                              const nextAction = e.target.value as ProductButtonAction;
                              const currentDefault = defaultButtonIcon(button.action);
                              updateButton(index, { action: nextAction, icon: !button.icon || button.icon === currentDefault ? defaultButtonIcon(nextAction) : button.icon });
                            }}>
                            {buttonActions.map(action => <MenuItem key={action.value} value={action.value}>{action.label}</MenuItem>)}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 10, md: 3 }}>
                          <TextField label="Kode internal" size="small" fullWidth value={button.key}
                            onChange={e => updateButton(index, { key: e.target.value })} />
                        </Grid>
                        <Grid size={{ xs: 2, md: 1 }} sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                          <Tooltip title="Hapus tombol">
                            <span>
                              <IconButton size="small" color="error" disabled={buttons.length <= 1} onClick={() => setButtons(prev => prev.filter((_, i) => i !== index))}>
                                <DeleteIcon fontSize="small" />
                              </IconButton>
                            </span>
                          </Tooltip>
                        </Grid>
                        {(button.action === 'reply' || button.action === 'handoff') && (
                          <Grid size={{ xs: 12 }}>
                            <TextField
                              label={button.action === 'handoff' ? 'Pesan sebelum masuk Butuh CS' : 'Teks balasan manual'}
                              size="small"
                              fullWidth
                              multiline
                              rows={2}
                              value={button.response || ''}
                              onChange={e => updateButton(index, { response: e.target.value })}
                            />
                          </Grid>
                        )}
                        {button.action === 'ai' && (
                          <Grid size={{ xs: 12 }}>
                            <Alert severity="info" icon={false} sx={{ py: 0.75 }}>
                              Sumber jawaban: seluruh knowledge khusus produk ini, katalog yang relevan, lalu Knowledge umum Asisten AI. Pertanyaan checkout dipakai untuk mengumpulkan data, bukan sebagai fakta jawaban.
                            </Alert>
                          </Grid>
                        )}
                      </Grid>
                    </Box>
                  ))}
                  <Button size="small" variant="outlined" startIcon={<AddIcon />} disabled={buttons.length >= 3} onClick={addButton}>
                    Tambah Tombol
                  </Button>
                </Stack>
              </AccordionDetails>
            </Accordion>

            <Accordion variant="outlined" defaultExpanded={hasCheckout}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                  <Typography variant="subtitle2">Alur Checkout</Typography>
                  <Chip size="small" label={hasCheckout ? `${checkoutSteps.length} langkah` : 'Tidak aktif'} />
                </Stack>
              </AccordionSummary>
              <AccordionDetails>
                <Stack spacing={1.25}>
                  {!hasCheckout && <Alert severity="info" icon={false}>Tambahkan tombol dengan aksi Mulai checkout agar alur ini aktif.</Alert>}
                  {hasCheckout && (
                    <Alert severity="success">
                      AI menjawab pertanyaan produk secara natural terlebih dahulu. Checkout baru dibuka saat niat pelanggan sudah jelas; permintaan koreksi membuka pesanan lama agar diperbarui tanpa membuat duplikat.
                    </Alert>
                  )}
                  {checkoutSteps.map((step, index) => (
                    <Box key={`${step.key}-${index}`} sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 1.25, opacity: hasCheckout ? 1 : 0.65 }}>
                      <Grid container spacing={1}>
                        <Grid size={{ xs: 12, md: 5 }}>
                          <TextField label={`Pertanyaan ${index + 1}`} size="small" fullWidth value={step.label}
                            onChange={e => updateStep(index, { label: e.target.value })} />
                        </Grid>
                        <Grid size={{ xs: 6, md: 2 }}>
                          <TextField label="Tipe" size="small" select fullWidth value={step.type}
                            onChange={e => updateStep(index, { type: e.target.value as CheckoutStepType, options: e.target.value === 'select' ? (step.options?.length ? step.options : ['Pilihan 1', 'Pilihan 2']) : undefined })}>
                            {stepTypes.map(type => <MenuItem key={type.value} value={type.value}>{type.label}</MenuItem>)}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 6, md: 2 }}>
                          <TextField label="Kode data" size="small" fullWidth value={step.key}
                            onChange={e => updateStep(index, { key: e.target.value })} />
                        </Grid>
                        <Grid size={{ xs: 12, md: 3 }}>
                          <Stack direction="row" spacing={0.5} sx={{ justifyContent: 'flex-end', alignItems: 'center' }}>
                            <FormControlLabel control={<Switch size="small" checked={step.required} onChange={e => updateStep(index, { required: e.target.checked })} />} label="Wajib" />
                            <Tooltip title="Naik"><span><IconButton size="small" disabled={index === 0} onClick={() => moveStep(index, -1)}><ArrowUpwardIcon fontSize="small" /></IconButton></span></Tooltip>
                            <Tooltip title="Turun"><span><IconButton size="small" disabled={index === checkoutSteps.length - 1} onClick={() => moveStep(index, 1)}><ArrowDownwardIcon fontSize="small" /></IconButton></span></Tooltip>
                            <Tooltip title="Hapus langkah"><span><IconButton size="small" color="error" disabled={checkoutSteps.length <= 1} onClick={() => setCheckoutSteps(prev => prev.filter((_, i) => i !== index))}><DeleteIcon fontSize="small" /></IconButton></span></Tooltip>
                          </Stack>
                        </Grid>
                        {step.type === 'select' && (
                          <Grid size={{ xs: 12 }}>
                            <TextField
                              label="Opsi pilihan"
                              size="small"
                              fullWidth
                              value={(step.options || []).join(', ')}
                              onChange={e => updateStep(index, { options: e.target.value.split(',').map(v => v.trim()).filter(Boolean) })}
                              placeholder="S, M, L, XL"
                            />
                          </Grid>
                        )}
                      </Grid>
                    </Box>
                  ))}
                  <Button size="small" variant="outlined" startIcon={<AddIcon />} onClick={addStep}>Tambah Langkah</Button>
                  <Divider />
                  <FormControlLabel
                    control={<Switch checked={checkoutHandoff} onChange={e => setCheckoutHandoff(e.target.checked)} />}
                    label="Setelah konfirmasi, masukkan percakapan ke Butuh CS"
                  />
                  <TextField
                    label="Pesan setelah order dikonfirmasi"
                    size="small"
                    fullWidth
                    multiline
                    rows={2}
                    value={checkoutSuccessMessage}
                    onChange={e => setCheckoutSuccessMessage(e.target.value)}
                    helperText="Bisa memakai {order_code} dan {product}."
                  />
                  <Alert severity="info" icon={false}>
                    Checkout tidak memakai AI. Jawaban di alur ini mengikuti langkah statis yang diatur di sini, lalu order dicatat saat pelanggan menekan Konfirmasi.
                  </Alert>
                </Stack>
              </AccordionDetails>
            </Accordion>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Batal</Button>
          <Button variant="contained" onClick={submit} disabled={saveMut.isPending}>
            {saveMut.isPending ? 'Menyimpan...' : 'Simpan'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={sendOpen} onClose={() => setSendOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Kirim Produk</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <Alert severity="info" icon={false}>
              Produk akan dikirim dengan tombol yang sudah diatur di katalog.
            </Alert>
            <TextField label="Nomor WhatsApp" size="small" fullWidth value={sendTo}
              onChange={e => setSendTo(e.target.value)} placeholder="08123456789" />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSendOpen(false)}>Batal</Button>
          <Button variant="contained" startIcon={<SendIcon />} onClick={sendProduct} disabled={sendMut.isPending}>
            {sendMut.isPending ? 'Mengirim...' : 'Kirim'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
