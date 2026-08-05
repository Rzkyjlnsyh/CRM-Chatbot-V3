import { useState } from 'react';
import {
  Box, Card, CardContent, Typography, Button, Stack, IconButton,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, CircularProgress,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import TemplateIcon from '@mui/icons-material/TextSnippetOutlined';
import DeleteIcon from '@mui/icons-material/Delete';
import { useTemplates, useSaveTemplate, useDeleteTemplate } from '../hooks';
import type { Template } from '../types';
import { swalConfirm } from '../services/swal';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';
import WhatsAppEditor from './WhatsAppEditor';

const EMPTY: Partial<Template> = { title: '', body: '' };

export default function TemplatePanel({ agentId }: { agentId: number }) {
  const { data: templates, isLoading } = useTemplates(agentId);
  const save = useSaveTemplate(agentId);
  const del = useDeleteTemplate(agentId);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<Partial<Template>>(EMPTY);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const openNew = () => { setForm(EMPTY); setErrors({}); setOpen(true); };
  const openEdit = (t: Template) => { setForm(t); setErrors({}); setOpen(true); };
  const validate = () => {
    const e: Record<string, string> = {};
    if (!form.title?.trim()) e.title = 'Wajib diisi';
    if (!form.body?.trim()) e.body = 'Wajib diisi';
    setErrors(e);
    return Object.keys(e).length === 0;
  };
  const submit = async () => {
    if (!validate()) return;
    await save.mutateAsync(form);
    setOpen(false);
  };
  const remove = async (t: Template) => { if (await swalConfirm('Hapus template ini?')) del.mutate(t.id); };

  if (isLoading) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;

  return (
    <Box>
      <PageHeader title="Template Pesan"
        subtitle="Pesan siap-pakai yang bisa dipanggil cepat di Inbox, Blast, dan Jadwal. Pakai {nama} untuk menyapa otomatis dengan nama kontak."
        action={<Button variant="contained" startIcon={<AddIcon />} onClick={openNew}>Tambah Template</Button>} />

      {(!templates || templates.length === 0) ? (
        <EmptyState
          icon={<TemplateIcon sx={{ fontSize: 48 }} />}
          title="Belum ada template"
          description="Simpan pesan yang sering dipakai sebagai template. Pakai {'{nama}'} untuk personalisasi otomatis."
          actionLabel="Tambah Template"
          onAction={openNew}
        />
      ) : (
        <Stack spacing={1}>
          {templates.map(t => (
            <Card key={t.id}>
              <CardContent>
                <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
                  <Box sx={{ minWidth: 0 }}>
                    <Typography sx={{ fontWeight: 600, mb: 0.5 }}>{t.title}</Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ whiteSpace: 'pre-wrap' }}>{t.body}</Typography>
                  </Box>
                  <Stack direction="row" sx={{ alignItems: 'center', flexShrink: 0 }}>
                    <IconButton size="small" onClick={() => openEdit(t)}><EditIcon fontSize="small" /></IconButton>
                    <IconButton size="small" color="error" onClick={() => remove(t)}><DeleteIcon fontSize="small" /></IconButton>
                  </Stack>
                </Stack>
              </CardContent>
            </Card>
          ))}
        </Stack>
      )}

      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="sm">
        <DialogTitle>{form.id ? 'Edit Template' : 'Template Baru'}</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <TextField label="Judul" value={form.title ?? ''}
              onChange={e => { setForm({ ...form, title: e.target.value }); if (errors.title) setErrors(p => ({ ...p, title: '' })); }} size="small"
              placeholder="Sapaan order" error={!!errors.title} helperText={errors.title || 'Nama singkat untuk mengenali template ini.'} />
            <Box>
              <Typography variant="caption" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>Isi pesan</Typography>
              <WhatsAppEditor value={form.body ?? ''} onChange={v => { setForm({ ...form, body: v }); if (errors.body) setErrors(p => ({ ...p, body: '' })); }}
                placeholder="Halo {nama}, terima kasih sudah order 🙏" rows={4} error={!!errors.body} helperText={errors.body || 'Tips: {nama} otomatis diganti nama kontak saat dikirim lewat Blast/Jadwal.'} />
            </Box>
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
