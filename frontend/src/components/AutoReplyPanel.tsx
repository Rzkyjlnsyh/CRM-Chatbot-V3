import { useState } from 'react';
import {
  Box, Card, CardContent, Typography, Button, Stack, Chip, Switch, IconButton,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, Select, MenuItem, FormControl, InputLabel,
  CircularProgress,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import RuleIcon from '@mui/icons-material/RuleOutlined';
import { useAutoReplies, useSaveAutoReply, useDeleteAutoReply } from '../hooks';
import type { AutoReply } from '../types';
import { swalConfirm } from '../services/swal';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';

const MATCH_LABEL: Record<string, string> = {
  contains: 'Mengandung kata', exact: 'Sama persis', prefix: 'Diawali',
};
const EMPTY: Partial<AutoReply> = { keywords: '', match_type: 'contains', reply: '', enabled: true };

export default function AutoReplyPanel({ agentId }: { agentId: number }) {
  const { data: rules, isLoading } = useAutoReplies(agentId);
  const save = useSaveAutoReply(agentId);
  const del = useDeleteAutoReply(agentId);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<Partial<AutoReply>>(EMPTY);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const openNew = () => { setForm(EMPTY); setErrors({}); setOpen(true); };
  const openEdit = (r: AutoReply) => { setForm(r); setErrors({}); setOpen(true); };
  const validate = () => {
    const e: Record<string, string> = {};
    if (!form.keywords?.trim()) e.keywords = 'Wajib diisi';
    if (!form.reply?.trim()) e.reply = 'Wajib diisi';
    setErrors(e);
    return Object.keys(e).length === 0;
  };
  const submit = async () => {
    if (!validate()) return;
    await save.mutateAsync(form);
    setOpen(false);
  };
  const toggle = (r: AutoReply) => save.mutate({ id: r.id, enabled: !r.enabled });
  const remove = async (r: AutoReply) => { if (await swalConfirm('Hapus aturan ini?')) del.mutate(r.id); };

  if (isLoading) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;

  return (
    <Box>
      <PageHeader title="Auto-Reply"
        subtitle="Balasan instan tanpa AI saat pesan pelanggan cocok kata kunci. Dicek sebelum AI — cepat, hemat biaya, dan jawabannya selalu sama persis."
        action={<Button variant="contained" startIcon={<AddIcon />} onClick={openNew}>Tambah Aturan</Button>} />

      {(!rules || rules.length === 0) ? (
        <EmptyState
          icon={<RuleIcon sx={{ fontSize: 48 }} />}
          title="Belum ada aturan"
          description="Atur balasan instan untuk kata kunci tertentu. Contoh: saat pelanggan tanya harga, langsung balas daftar harga tanpa AI."
          actionLabel="Tambah Aturan"
          onAction={openNew}
        />
      ) : (
        <Stack spacing={1}>
          {rules.map(r => (
            <Card key={r.id} sx={{ opacity: r.enabled ? 1 : 0.6 }}>
              <CardContent>
                <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
                  <Box sx={{ minWidth: 0 }}>
                    <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 0.5, mb: 0.5, alignItems: 'center' }}>
                      {r.keywords.split(',').map(k => k.trim()).filter(Boolean).map((k, i) => (
                        <Chip key={i} size="small" label={k} color="primary" variant="outlined" />
                      ))}
                      <Typography variant="caption" color="text.secondary" sx={{ ml: 0.5 }}>· {MATCH_LABEL[r.match_type] || r.match_type}</Typography>
                    </Stack>
                    <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'pre-wrap' }}>{r.reply}</Typography>
                  </Box>
                  <Stack direction="row" sx={{ alignItems: 'center', flexShrink: 0 }}>
                    <Switch checked={r.enabled} onChange={() => toggle(r)} size="small" />
                    <IconButton size="small" onClick={() => openEdit(r)}><EditIcon fontSize="small" /></IconButton>
                    <IconButton size="small" color="error" onClick={() => remove(r)}><DeleteIcon fontSize="small" /></IconButton>
                  </Stack>
                </Stack>
              </CardContent>
            </Card>
          ))}
        </Stack>
      )}

      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>{form.id ? 'Edit Aturan' : 'Aturan Baru'}</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <TextField label="Kata kunci (pisah dengan koma)" value={form.keywords ?? ''}
              onChange={e => { setForm({ ...form, keywords: e.target.value }); if (errors.keywords) setErrors(p => ({ ...p, keywords: '' })); }} size="small"
              placeholder="harga, price, berapa" error={!!errors.keywords} helperText={errors.keywords || 'Cocok kalau salah satu kata kunci ditemukan.'} />
            <FormControl size="small" fullWidth>
              <InputLabel>Tipe pencocokan</InputLabel>
              <Select label="Tipe pencocokan" value={form.match_type ?? 'contains'}
                onChange={e => setForm({ ...form, match_type: e.target.value })}>
                <MenuItem value="contains">Mengandung kata (paling umum)</MenuItem>
                <MenuItem value="exact">Sama persis</MenuItem>
                <MenuItem value="prefix">Diawali kata</MenuItem>
              </Select>
            </FormControl>
            <TextField label="Balasan" value={form.reply ?? ''} onChange={e => { setForm({ ...form, reply: e.target.value }); if (errors.reply) setErrors(p => ({ ...p, reply: '' })); }}
              size="small" multiline rows={4} placeholder="Halo kak! Berikut daftar harga kami: ..." error={!!errors.reply} helperText={errors.reply} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Batal</Button>
          <Button variant="contained" onClick={submit} disabled={save.isPending}>Simpan</Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
