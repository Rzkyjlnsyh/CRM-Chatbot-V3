// LincahPanel — Integrasi Pengiriman Lincah (OpenAPI v1.1.6).
// Satu panel lengkap: konfigurasi kredensial, uji koneksi, daftar gudang,
// daftar kurir, cek ongkir, pesanan lokal + lacak + cetak resi.
import { useCallback, useEffect, useState } from 'react';
import {
  Box, Button, Card, CardContent, Chip, CircularProgress, Dialog, DialogActions,
  DialogContent, DialogTitle, Divider, Grid,
  MenuItem, Select, TextField, Typography,
  Table, TableBody, TableCell, TableHead, TableRow,
  Alert, List, ListItem, ListItemText,
} from '@mui/material';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import api from '../services/api';

const DEV_BASE = 'https://dev-api.lincah.id/openapi';
const PROD_BASE = 'https://api.lincah.id/openapi';

interface LincahConfig {
  partner_id: string;
  base_url: string;
  token_set: boolean;
}

interface Warehouse {
  _id: string;
  name?: string;
  address?: string;
  origin_id?: string;
  zipcode?: string;
  geoloc?: { lat: number; long: number };
}

interface CostItem { type: string; code: string; cost: number; costReal: number; etc?: string }
interface CostRow { code: string; name: string; costs: CostItem[] }
interface LincahOrder {
  id: number; sender?: string; lincah_order_id: string; no_order: string; resi: string;
  status: string; courier: string; courier_service: string; name: string; phone: string;
  address: string; destination: string; product_name: string; product_price: number;
  weight: number; fee: number; created_at: string;
}

function rupiah(n: number) {
  return 'Rp ' + Number(n || 0).toLocaleString('id-ID');
}

export default function LincahPanel({ agentId }: { agentId: number }) {
  const qc = useQueryClient();
  const [cfg, setCfg] = useState<LincahConfig>({ partner_id: '', base_url: DEV_BASE, token_set: false });
  const [token, setToken] = useState('');
  const [testResult, setTestResult] = useState<string | null>(null);
  const [testError, setTestError] = useState<string | null>(null);
  const [testing, setTesting] = useState(false);

  // Form cek ongkir
  const [originId, setOriginId] = useState('');
  const [destCode, setDestCode] = useState('');
  const [destText, setDestText] = useState('');
  const [weight, setWeight] = useState('1');
  const [dims, setDims] = useState('10x10x10');
  const [costs, setCosts] = useState<CostRow[] | null>(null);
  const [ongkirLoading, setOngkirLoading] = useState(false);
  // Quote cepat (chat-quote): tujuan + berat → teks siap kirim ke pelanggan.
  const [quickDest, setQuickDest] = useState('');
  const [quickWeight, setQuickWeight] = useState('1');
  const [quickText, setQuickText] = useState('');
  const [quickLoading, setQuickLoading] = useState(false);
  const [track, setTrack] = useState<any>(null);

  const { data: cfgData, refetch: refetchCfg } = useQuery({
    queryKey: ['lincah-config', agentId],
    queryFn: async () => (await api.get(`/agents/${agentId}/lincah/config`)).data as LincahConfig,
  });
  useEffect(() => {
    if (cfgData) setCfg(cfgData);
  }, [cfgData]);

  const { data: warehouses = [] as Warehouse[], refetch: refetchWh } = useQuery({
    queryKey: ['lincah-addresses', agentId],
    queryFn: async () => ((await api.get(`/agents/${agentId}/lincah/addresses`)).data?.data ?? []) as Warehouse[],
    enabled: !!cfgData?.token_set,
  });

  const { data: orders = [] as LincahOrder[], refetch: refetchOrders } = useQuery({
    queryKey: ['lincah-orders', agentId],
    queryFn: async () => ((await api.get(`/agents/${agentId}/lincah/orders`)).data?.data ?? []) as LincahOrder[],
    enabled: !!cfgData?.token_set,
  });

  const save = useCallback(async () => {
    await api.put(`/agents/${agentId}/lincah/config`, { ...cfg, token });
    setToken('');
    await refetchCfg();
    qc.invalidateQueries({ queryKey: ['lincah-addresses', agentId] });
  }, [agentId, cfg, token, qc, refetchCfg]);

  const doTest = useCallback(async () => {
    setTesting(true);
    setTestError(null);
    setTestResult(null);
    try {
      const r = (await api.post(`/agents/${agentId}/lincah/test`)).data;
      setTestResult(`✅ ${r.name} — ${r.email}${typeof r.balance === 'number' ? ` · Saldo: ${rupiah(r.balance)}` : ''}`);
    } catch (e: any) {
      setTestError(e?.response?.data?.error || String(e));
    } finally {
      setTesting(false);
    }
  }, [agentId]);

  const checkOngkir = useCallback(async () => {
    if (!originId || !destCode || !destText) {
      setTestError('Pilih gudang asal dan isi kode tujuan (mis. 34.02.01) + nama tujuan.');
      return;
    }
    setOngkirLoading(true);
    setCosts(null);
    setTestError(null);
    try {
      const [h, w, d] = dims.split('x').map((x) => parseInt(x || '0', 10) || 0);
      const r = (await api.post(`/agents/${agentId}/lincah/ongkir`, {
        isPickup: true, isCod: false,
        dimensions: [h || 10, w || 10, d || 10],
        weight: Math.max(1, Math.round(parseFloat(weight || '1') * 1000)),
        origin: originId, destination: destCode,
      })).data;
      setCosts(r?.data ?? []);
    } catch (e: any) {
      setTestError(e?.response?.data?.error || String(e));
    } finally {
      setOngkirLoading(false);
    }
  }, [agentId, originId, destCode, destText, weight, dims]);

  const createOrder = useCallback(async (row: CostRow, item: CostItem) => {
    const body = {
      sender_type: 'picked up',
      address_ref: originId,
      name: cfg?.partner_id === '' ? '' : destText,
      phone: '',
      address: destText,
      destination: destCode,
      type: 'regular',
      courier: row.code,
      courier_service: item.code,
      weight: Math.max(1, Math.round(parseFloat(weight || '1'))),
      quantity: 1,
      volume: dims,
      product_name: destText,
      product_price: 0,
    };
    try {
      const r = (await api.post(`/agents/${agentId}/lincah/orders`, body)).data;
      alert(`✅ Resi dibuat!\nNo. Order: ${r.data?.no_order}\nResi: ${r.data?.resi || '(menunggu generate)'}`);
      refetchOrders();
      qc.invalidateQueries({ queryKey: ['lincah-orders', agentId] });
    } catch (e: any) {
      alert('❌ Gagal buat resi: ' + (e?.response?.data?.error || String(e)));
    }
  }, [agentId, originId, destCode, destText, weight, dims, warehouses, qc, refetchOrders]);

  const quickQuote = useCallback(async () => {
    if (quickDest.trim().length < 3) {
      setTestError('Ketik tujuan minimal 3 huruf.');
      return;
    }
    setQuickLoading(true);
    setQuickText('');
    setTestError(null);
    try {
      const r = (await api.post(`/agents/${agentId}/lincah/chat-quote`, {
        dest: quickDest.trim(),
        weight_kg: parseFloat(quickWeight || '1') || 1,
        dimensions: [10, 10, 10],
      })).data;
      setQuickText(r?.data?.text ?? '');
    } catch (e: any) {
      setTestError(e?.response?.data?.error || String(e));
    } finally {
      setQuickLoading(false);
    }
  }, [agentId, quickDest, quickWeight]);

  const doTrack = useCallback(async (id: string) => {
    try {
      const r = (await api.get(`/agents/${agentId}/lincah/orders/${id}/track`)).data;
      setTrack(r?.data ?? null);
    } catch (e: any) {
      alert('Gagal lacak: ' + (e?.response?.data?.error || String(e)));
    }
  }, [agentId]);

  const doPrint = useCallback(async (id: string) => {
    try {
      const r = (await api.get(`/agents/${agentId}/lincah/orders/${id}/pdf`)).data;
      if (r?.pdf_url) window.open(r.pdf_url, '_blank');
      else alert('PDF belum tersedia dari Lincah.');
    } catch (e: any) {
      alert('Gagal cetak: ' + (e?.response?.data?.error || String(e)));
    }
  }, [agentId]);

  return (
    <Box>
      <Card sx={{ mb: 2 }}>
        <CardContent>
          <Typography variant="h6">🔗 Integrasi Lincah</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
            Token & partner-id dari dashboard partner Lincah (Profile → Open API). Mode dev default.
          </Typography>
          <Grid container spacing={1.5}>
            <Grid size={{ xs: 12, sm: 5 }}>
              <TextField label="Partner ID" size="small" fullWidth value={cfg.partner_id}
                onChange={(e) => setCfg({ ...cfg, partner_id: e.target.value })} />
            </Grid>
            <Grid size={{ xs: 12, sm: 7 }}>
              <TextField label="Token (disimpan di server)" size="small" fullWidth type="password"
                placeholder={cfg.token_set ? 'token sudah terpasang ✓ — ketik baru untuk ganti' : 'ketik token API'}
                value={token} onChange={(e) => setToken(e.target.value)} />
            </Grid>
            <Grid size={{ xs: 12, sm: 7 }}>
              <Select size="small" fullWidth value={cfg.base_url || DEV_BASE}
                onChange={(e) => setCfg({ ...cfg, base_url: e.target.value as string })}>
                <MenuItem value={DEV_BASE}>Dev — https://dev-api.lincah.id/openapi</MenuItem>
                <MenuItem value={PROD_BASE}>Prod — https://api.lincah.id/openapi</MenuItem>
              </Select>
            </Grid>
            <Grid size={{ xs: 12, sm: 5 }}>
              <Box sx={{ display: 'flex', gap: 1 }}>
                <Button size="small" variant="contained" onClick={save}>Simpan</Button>
                <Button size="small" variant="outlined" onClick={doTest} disabled={testing}>
                  {testing ? <CircularProgress size={16} /> : 'Tes Koneksi'}
                </Button>
              </Box>
            </Grid>
          </Grid>
          {testResult && <Alert severity="success" sx={{ mt: 1.5 }}>{testResult}</Alert>}
          {testError && <Alert severity="error" sx={{ mt: 1.5 }}>{testError}</Alert>}
        </CardContent>
      </Card>

      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 5 }}>
          <Card sx={{ mb: 2, height: '100%' }}>
            <CardContent>
              <Typography variant="subtitle1">📦 Gudang / Alamat Pengirim</Typography>
              {warehouses.length === 0 && <Typography variant="body2" color="text.secondary">Belum ada (token belum terpasang?).</Typography>}
              <List dense>
                {(warehouses || []).map((w) => (
                  <ListItem key={w._id} secondaryAction={
                    <Chip label={w._id} size="small" variant="outlined" />
                  }>
                    <ListItemText primary={w.name || w.address || w._id}
                      secondary={`${w.address || ''} ${w.origin_id ? `· Kode: ${w.origin_id}` : ''} ${w.zipcode ? `· ${w.zipcode}` : ''}`} />
                  </ListItem>
                ))}
              </List>
              <Divider sx={{ my: 1 }} />
              <Button size="small" onClick={() => refetchWh()}>Muat ulang gudang</Button>
            </CardContent>
          </Card>

          <Card>
            <CardContent>
              <Typography variant="subtitle1">🧾 Pesanan dari Dashboard</Typography>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Resi</TableCell><TableCell>Status</TableCell><TableCell>Kurir</TableCell><TableCell>Biaya</TableCell><TableCell></TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {(orders || []).slice(0, 8).map((o) => (
                    <TableRow key={o.id}>
                      <TableCell>{o.resi || o.no_order}</TableCell>
                      <TableCell><Chip size="small" label={o.status} /></TableCell>
                      <TableCell>{o.courier}{o.courier_service ? ` · ${o.courier_service}` : ''}</TableCell>
                      <TableCell>{rupiah(o.fee)}</TableCell>
                      <TableCell>
                        <Button size="small" onClick={() => doTrack(o.resi || o.lincah_order_id)}>Lacak</Button>
                        <Button size="small" onClick={() => doPrint(o.lincah_order_id)}>Cetak</Button>
                      </TableCell>
                    </TableRow>
                  ))}
                  {(orders || []).length === 0 && (
                    <TableRow><TableCell colSpan={5}>Belum ada pesanan.</TableCell></TableRow>
                  )}
                </TableBody>
              </Table>
            </CardContent>
          </Card>

          <Card sx={{ mt: 2 }}>
            <CardContent>
              <Typography variant="subtitle1">⚡ Quote Cepat (siap salin ke chat)</Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                Ketik tujuan saja (mis. "Bantul") + berat — dapat teks ongkir termurah siap dikirim ke pelanggan.
              </Typography>
              <Grid container spacing={1.5}>
                <Grid size={{ xs: 12, sm: 6 }}>
                  <TextField size="small" fullWidth label="Tujuan (mis. Bantul)" value={quickDest}
                    onChange={(e) => setQuickDest(e.target.value)} />
                </Grid>
                <Grid size={{ xs: 6, sm: 3 }}>
                  <TextField size="small" fullWidth label="Berat (kg)" value={quickWeight}
                    onChange={(e) => setQuickWeight(e.target.value)} />
                </Grid>
                <Grid size={{ xs: 6, sm: 3 }}>
                  <Button variant="contained" size="small" onClick={quickQuote} disabled={quickLoading}>
                    {quickLoading ? <CircularProgress size={16} /> : 'Dapatkan Ongkir'}
                  </Button>
                </Grid>
              </Grid>
              {quickText && (
                <Alert severity="info" sx={{ mt: 1.5, whiteSpace: 'pre-wrap' }}>
                  {quickText}
                  <Box sx={{ mt: 0.5 }}>
                    <Button size="small" onClick={() => navigator.clipboard?.writeText(quickText)}>Salin</Button>
                  </Box>
                </Alert>
              )}
            </CardContent>
          </Card>
        </Grid>

        <Grid size={{ xs: 12, md: 7 }}>
          <Card>
            <CardContent>
              <Typography variant="subtitle1">🛵 Cek Ongkir (Lincah)</Typography>
              <Grid container spacing={1.5} sx={{ mt: 0.5 }}>
                <Grid size={{ xs: 12, sm: 6 }}>
                  <Select size="small" fullWidth value={originId} displayEmpty
                    onChange={(e) => setOriginId(e.target.value as string)}>
                    <MenuItem value="">Asal — pilih gudang…</MenuItem>
                    {(warehouses || []).map((w) => (
                      <MenuItem key={w._id} value={w._id}>{w.name || w.address || w._id}</MenuItem>
                    ))}
                  </Select>
                </Grid>
                <Grid size={{ xs: 12, sm: 6 }}>
                  <TextField size="small" fullWidth label="Kode tujuan (mis. 34.02.01)" value={destCode}
                    onChange={(e) => setDestCode(e.target.value)} />
                </Grid>
                <Grid size={{ xs: 12, sm: 6 }}>
                  <TextField size="small" fullWidth label="Nama tujuan" value={destText}
                    onChange={(e) => setDestText(e.target.value)} />
                </Grid>
                <Grid size={{ xs: 6, sm: 3 }}>
                  <TextField size="small" fullWidth label="Berat (kg)" value={weight}
                    onChange={(e) => setWeight(e.target.value)} />
                </Grid>
                <Grid size={{ xs: 6, sm: 3 }}>
                  <TextField size="small" fullWidth label="Volume PxLxT" value={dims}
                    onChange={(e) => setDims(e.target.value)} />
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <Button variant="contained" size="small" onClick={checkOngkir} disabled={ongkirLoading}>
                    {ongkirLoading ? <CircularProgress size={16} /> : 'Hitung Ongkir'}
                  </Button>
                </Grid>
              </Grid>
              {costs && (
                <Table size="small" sx={{ mt: 1.5 }}>
                  <TableHead>
                    <TableRow><TableCell>Kurir</TableCell><TableCell>Layanan</TableCell><TableCell>Tarif</TableCell><TableCell>Estimasi</TableCell><TableCell></TableCell></TableRow>
                  </TableHead>
                  <TableBody>
                    {costs.map((row) => row.costs.map((ci) => (
                      <TableRow key={`${row.code}-${ci.code}`}>
                        <TableCell>{row.name}</TableCell>
                        <TableCell>{ci.type}</TableCell>
                        <TableCell>{rupiah(ci.cost)}</TableCell>
                        <TableCell>{ci.etc || '-'}</TableCell>
                        <TableCell>
                          <Button size="small" variant="outlined" onClick={() => createOrder(row, ci)}>Buat Resi</Button>
                        </TableCell>
                      </TableRow>
                    )))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      <Dialog open={!!track} onClose={() => setTrack(null)} maxWidth="sm" fullWidth>
        <DialogTitle>📍 Lacak {track?.order?.resi || track?.order?.no_order || ''}</DialogTitle>
        <DialogContent>
          <List dense>
            {(track?.data || []).map((ev: any, i: number) => (
              <ListItem key={i}>
                <ListItemText
                  primary={`${ev.status}${ev.message ? ' — ' + ev.message : ''}`}
                  secondary={ev.time ? new Date(ev.time).toLocaleString('id-ID') : ''}
                />
              </ListItem>
            ))}
            {(track?.data || []).length === 0 && <Typography variant="body2">Belum ada riwayat status.</Typography>}
          </List>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setTrack(null)}>Tutup</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
