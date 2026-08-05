import { useState } from 'react';
import {
  Box, Card, CardContent, Typography, Button, Stack, Chip, Switch, IconButton,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, Select, MenuItem,
  FormControl, InputLabel, FormControlLabel, CircularProgress, Alert, Paper, useMediaQuery,
  ToggleButton, ToggleButtonGroup,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import ScheduleSendIcon from '@mui/icons-material/ScheduleSendOutlined';
import PersonAddIcon from '@mui/icons-material/PersonAddAlt1Outlined';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesome';
import {
  useFollowUps, useSaveFollowUp, useDeleteFollowUp, useEnrollFollowUp,
  useCrmContacts, useCrmContactsExport,
} from '../hooks';
import type { FollowUp, LeadStage } from '../types';
import { normalizePhone } from '../types';
import { swalConfirm, swalAlert } from '../services/swal';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';
import WhatsAppEditor from './WhatsAppEditor';
import TemplatePicker from './TemplatePicker';
import RecipientField from './RecipientField';

type StepForm = { delay_value: number; delay_unit: 'hari' | 'jam'; message: string; ai_generated: boolean; ai_instruction: string };

const CRM_STAGES: { value: LeadStage; label: string }[] = [
  { value: 'new', label: 'Baru' }, { value: 'cold', label: 'Cold' },
  { value: 'warm', label: 'Warm' }, { value: 'hot', label: 'Hot' },
  { value: 'customer', label: 'Pelanggan' }, { value: 'unqualified', label: 'Tidak potensial' },
];

// jam tersimpan -> {nilai, satuan} untuk editor.
function hoursToParts(h: number): { delay_value: number; delay_unit: 'hari' | 'jam' } {
  if (h > 0 && h % 24 === 0) return { delay_value: h / 24, delay_unit: 'hari' };
  return { delay_value: h, delay_unit: 'jam' };
}
function partsToHours(s: StepForm): number {
  const v = Math.max(0, Math.floor(s.delay_value || 0));
  return s.delay_unit === 'hari' ? v * 24 : v;
}
function stepBadge(h: number): string {
  if (h === 0) return 'langsung';
  if (h % 24 === 0) return `H+${h / 24}`;
  return `+${h}j`;
}

const NEW_STEP: StepForm = { delay_value: 1, delay_unit: 'hari', message: '', ai_generated: false, ai_instruction: '' };

export default function FollowUpPanel({ agentId }: { agentId: number }) {
  const theme = useTheme();
  const mobileDialog = useMediaQuery(theme.breakpoints.down('sm'));
  const { data: flows, isLoading } = useFollowUps(agentId);
  const save = useSaveFollowUp(agentId);
  const del = useDeleteFollowUp(agentId);
  const enroll = useEnrollFollowUp(agentId);
  const exportContacts = useCrmContactsExport(agentId);
  const { data: crm } = useCrmContacts(agentId, '', '', '', 1);
  const allTags = crm?.all_tags || [];

  // ---- form urutan ----
  const [open, setOpen] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [name, setName] = useState('');
  const [stopOnReply, setStopOnReply] = useState(true);
  const [steps, setSteps] = useState<StepForm[]>([{ ...NEW_STEP, delay_value: 0, delay_unit: 'jam' }]);

  const openNew = () => {
    setEditId(null); setName(''); setStopOnReply(true);
    setSteps([{ delay_value: 0, delay_unit: 'jam', message: '', ai_generated: false, ai_instruction: '' }]);
    setOpen(true);
  };
  const openEdit = (fu: FollowUp) => {
    setEditId(fu.id); setName(fu.name); setStopOnReply(fu.stop_on_reply);
    setSteps(fu.steps.length ? fu.steps.map(s => ({ ...hoursToParts(s.delay_hours), message: s.message, ai_generated: s.ai_generated || false, ai_instruction: s.ai_instruction || '' })) : [{ delay_value: 0, delay_unit: 'jam', message: '', ai_generated: false, ai_instruction: '' }]);
    setOpen(true);
  };

  const setStep = (i: number, patch: Partial<StepForm>) => setSteps(steps.map((s, j) => j === i ? { ...s, ...patch } : s));
  const addStep = () => {
    const previousHours = steps.length ? partsToHours(steps[steps.length - 1]) : 0;
    const next = hoursToParts(previousHours + 24);
    setSteps([...steps, { ...next, message: '', ai_generated: false, ai_instruction: '' }]);
  };
  const removeStep = (i: number) => setSteps(steps.filter((_, j) => j !== i));

  const submit = async () => {
    if (!name.trim()) { await swalAlert('Nama urutan wajib diisi.', 'warning'); return; }
    const payloadSteps = steps.filter(s => s.message.trim()).map(s => ({ delay_hours: partsToHours(s), message: s.message, ai_generated: s.ai_generated, ai_instruction: s.ai_generated ? s.ai_instruction : '' }));
    if (payloadSteps.length === 0) { await swalAlert('Minimal satu langkah dengan pesan.', 'warning'); return; }
    const invalidOrder = payloadSteps.findIndex((step, index) => index > 0 && step.delay_hours < payloadSteps[index - 1].delay_hours);
    if (invalidOrder >= 0) { await swalAlert(`Waktu langkah ${invalidOrder + 1} tidak boleh lebih awal dari langkah sebelumnya.`, 'warning'); return; }
    await save.mutateAsync({ id: editId ?? undefined, name, stop_on_reply: stopOnReply, steps: payloadSteps } as Partial<FollowUp>);
    setOpen(false);
  };

  const toggle = (fu: FollowUp) => save.mutate({ id: fu.id, enabled: !fu.enabled } as Partial<FollowUp>);
  const remove = async (fu: FollowUp) => { if (await swalConfirm(`Hapus urutan "${fu.name}"?`, 'Kontak yang sedang mengikuti urutan ini juga akan dihapus dari Follow-up.')) del.mutate(fu.id); };

  // ---- dialog daftarkan kontak ----
  const [enrollFu, setEnrollFu] = useState<FollowUp | null>(null);
  const [recipients, setRecipients] = useState('');
  const [enrollTag, setEnrollTag] = useState('');
  const [enrollStage, setEnrollStage] = useState<LeadStage | ''>('');

  const openEnroll = (fu: FollowUp) => { setEnrollFu(fu); setRecipients(''); setEnrollTag(''); setEnrollStage(''); };

  const appendCrmContacts = (list: { number: string; name: string }[]) => {
    const lines = list.map(c => (c.name ? `${c.number},${c.name}` : c.number));
    setRecipients(prev => {
      const have = new Set(prev.split('\n').map(l => normalizePhone(l.split(',')[0])).filter(Boolean));
      const fresh = lines.filter(l => !have.has(normalizePhone(l.split(',')[0])));
      return [prev.trim(), ...fresh].filter(Boolean).join('\n');
    });
  };

  const fillFromTag = async (tag: string) => {
    setEnrollTag(tag);
    if (!tag) return;
    try {
      const list = await exportContacts.mutateAsync({ q: '', tag, stage: '' });
      appendCrmContacts(list);
    } catch { await swalAlert('Gagal mengambil kontak tag.', 'error'); }
  };

  const fillFromStage = async (stage: LeadStage | '') => {
    setEnrollStage(stage);
    if (!stage) return;
    try {
      appendCrmContacts(await exportContacts.mutateAsync({ q: '', tag: '', stage }));
    } catch { await swalAlert('Gagal mengambil kontak dari status CRM.', 'error'); }
  };

  const doEnroll = async () => {
    if (!enrollFu) return;
    const parsed = recipients.split('\n').map(l => l.trim()).filter(Boolean).map(line => {
      const [num, ...rest] = line.split(',');
      return { number: normalizePhone(num), name: rest.join(',').trim() };
    }).filter(r => r.number);
    if (parsed.length === 0) { await swalAlert('Masukkan minimal satu nomor.', 'warning'); return; }
    const res = await enroll.mutateAsync({ fid: enrollFu.id, recipients: parsed });
    setEnrollFu(null);
    await swalAlert(`${res.added} kontak mulai mengikuti Follow-up${res.skipped ? `, ${res.skipped} dilewati karena sudah aktif atau opt-out` : ''}.`, 'success');
  };

  if (isLoading) return <Box sx={{ display: 'flex', justifyContent: 'center', mt: 8 }}><CircularProgress /></Box>;

  return (
    <Box>
      <PageHeader title="Follow-up Otomatis"
        subtitle="Buat urutan pesan, tambahkan kontak, lalu pesan terkirim otomatis sesuai waktu yang ditentukan. Balasan atau STOP dapat menghentikan urutan."
        action={<Button variant="contained" startIcon={<AddIcon />} onClick={openNew}>Buat Urutan</Button>} />

      {(!flows || flows.length === 0) ? (
        <EmptyState
          icon={<ScheduleSendIcon sx={{ fontSize: 48 }} />}
          title="Belum ada urutan follow-up"
          description="Jadwalkan pesan otomatis untuk menjaga hubungan dengan pelanggan. Contoh: H+0 ucapan terima kasih, H+3 tips produk."
          actionLabel="Buat Urutan"
          onAction={openNew}
        />
      ) : (
        <Stack spacing={1}>
          {flows.map(fu => (
            <Card key={fu.id} sx={{ opacity: fu.enabled ? 1 : 0.6 }}>
              <CardContent>
                <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
                  <Box sx={{ minWidth: 0 }}>
                    <Typography sx={{ fontWeight: 600 }}>{fu.name}</Typography>
                    <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 0.5, my: 0.75, alignItems: 'center' }}>
                      {fu.steps.map((s, i) => (
                        <Chip key={i} size="small" label={`${stepBadge(s.delay_hours)}`} color="primary" variant="outlined" />
                      ))}
                      <Typography variant="caption" color="text.secondary">· {fu.steps.length} langkah</Typography>
                    </Stack>
                    <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 0.5, alignItems: 'center' }}>
                      <Chip size="small" label={`Aktif ${fu.counts.active}`} color={fu.counts.active ? 'success' : 'default'} variant="outlined" />
                      <Chip size="small" label={`Selesai ${fu.counts.completed}`} variant="outlined" />
                      <Chip size="small" label={`Stop ${fu.counts.stopped}`} variant="outlined" />
                      {fu.stop_on_reply && <Typography variant="caption" color="text.secondary">· stop bila dibalas</Typography>}
                    </Stack>
                  </Box>
                  <Stack direction="row" sx={{ alignItems: 'center', flexShrink: 0 }}>
                    <Switch checked={fu.enabled} onChange={() => toggle(fu)} size="small" />
                    <IconButton size="small" title="Tambahkan kontak ke urutan" color="primary" onClick={() => openEnroll(fu)}><PersonAddIcon fontSize="small" /></IconButton>
                    <IconButton size="small" title="Edit" onClick={() => openEdit(fu)}><EditIcon fontSize="small" /></IconButton>
                    <IconButton size="small" color="error" title="Hapus" onClick={() => remove(fu)}><DeleteIcon fontSize="small" /></IconButton>
                  </Stack>
                </Stack>
              </CardContent>
            </Card>
          ))}
        </Stack>
      )}

      {/* Buat / edit urutan */}
      <Dialog open={open} onClose={() => setOpen(false)} fullWidth maxWidth="md" fullScreen={mobileDialog}>
        <DialogTitle>{editId ? 'Edit Urutan Follow-up' : 'Buat Urutan Follow-up'}</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={1.5}>
            <Alert severity="info" icon={false}>
              Semua waktu dihitung dari saat kontak mulai mengikuti urutan ini, bukan dari pesan sebelumnya.
            </Alert>

            <Paper variant="outlined" sx={{ p: 1.25 }}>
              <Typography variant="subtitle2" sx={{ fontWeight: 800, mb: 1 }}>Informasi Urutan</Typography>
              <TextField fullWidth label="Nama urutan" value={name} onChange={e => setName(e.target.value)} size="small"
                placeholder="Contoh: Tindak lanjut pembeli baru" helperText="Nama ini hanya terlihat di dashboard." />
              <FormControlLabel sx={{ mt: 0.75, alignItems: 'flex-start' }}
                control={<Switch checked={stopOnReply} onChange={e => setStopOnReply(e.target.checked)} />}
                label={
                  <Box sx={{ pt: 0.65 }}>
                    <Typography variant="body2" sx={{ fontWeight: 700 }}>Hentikan saat kontak membalas</Typography>
                    <Typography variant="caption" color="text.secondary">Pesan berikutnya tidak dikirim setelah kontak membalas.</Typography>
                  </Box>
                } />
            </Paper>

            <Box>
              <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>Urutan Pesan</Typography>
              <Typography variant="caption" color="text.secondary">Susun pesan dari yang dikirim paling awal hingga paling akhir.</Typography>
            </Box>
            {steps.map((s, i) => (
              <Paper key={i} variant="outlined" sx={{ p: 1.25 }}>
                <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1, mb: 1 }}>
                  <Stack direction="row" sx={{ alignItems: 'center', gap: 0.75 }}>
                    <Chip size="small" color="primary" label={`Langkah ${i + 1}`} />
                    <Typography variant="caption" color="text.secondary">{stepBadge(partsToHours(s))}</Typography>
                  </Stack>
                  {steps.length > 1 && <IconButton aria-label={`Hapus langkah ${i + 1}`} size="small" color="error" onClick={() => removeStep(i)}><DeleteIcon fontSize="small" /></IconButton>}
                </Stack>

                <Typography variant="body2" sx={{ fontWeight: 700, mb: 0.75 }}>Waktu kirim</Typography>
                <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start', mb: 0.5 }}>
                  <TextField type="number" size="small" label="Jeda" value={s.delay_value}
                    onChange={e => setStep(i, { delay_value: Math.max(0, Number(e.target.value) || 0) })} sx={{ width: 110 }}
                    slotProps={{ htmlInput: { min: 0 } }} />
                  <FormControl size="small" sx={{ width: 120 }}>
                    <InputLabel>Satuan</InputLabel>
                    <Select label="Satuan" value={s.delay_unit} onChange={e => setStep(i, { delay_unit: e.target.value as 'hari' | 'jam' })}>
                      <MenuItem value="jam">jam</MenuItem>
                      <MenuItem value="hari">hari</MenuItem>
                    </Select>
                  </FormControl>
                </Stack>
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1.25 }}>
                  {partsToHours(s) === 0
                    ? 'Pesan dikirim segera setelah kontak mulai mengikuti Follow-up.'
                    : `Pesan dikirim ${s.delay_value} ${s.delay_unit} setelah kontak mulai mengikuti Follow-up.`}
                </Typography>

                <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ alignItems: { xs: 'flex-start', sm: 'center' }, justifyContent: 'space-between', gap: 0.5, mb: 1 }}>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <Typography variant="body2" sx={{ fontWeight: 700 }}>
                      {s.ai_generated ? 'Instruksi ke AI' : 'Isi pesan'}
                    </Typography>
                    {s.ai_generated && <Chip size="small" icon={<AutoAwesomeIcon />} label="AI" color="success" variant="outlined" sx={{ height: 20, fontSize: '0.6rem' }} />}
                  </Stack>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                    <ToggleButtonGroup
                      size="small"
                      value={s.ai_generated ? 'ai' : 'manual'}
                      exclusive
                      onChange={(_, v) => v && setStep(i, { ai_generated: v === 'ai', ai_instruction: v === 'ai' ? (s.ai_instruction || s.message) : '' })}
                    >
                      <ToggleButton value="manual" sx={{ px: 1.5, py: 0.25, fontSize: '0.7rem' }}>Tulis Manual</ToggleButton>
                      <ToggleButton value="ai" sx={{ px: 1.5, py: 0.25, fontSize: '0.7rem' }}>
                        <AutoAwesomeIcon sx={{ fontSize: '0.85rem', mr: 0.25 }} />AI
                      </ToggleButton>
                    </ToggleButtonGroup>
                    {!s.ai_generated && <TemplatePicker label="Template" agentId={agentId} variant="text" onPick={b => setStep(i, { message: s.message ? s.message + '\n' + b : b })} />}
                  </Stack>
                </Stack>

                {s.ai_generated ? (
                  <>
                    <Alert severity="success" icon={<AutoAwesomeIcon />} sx={{ mb: 1, py: 0.25, '& .MuiAlert-message': { py: 0.25 } }}>
                      <Typography variant="caption">
                        AI akan menulis pesan berbeda untuk <b>setiap kontak</b> — disesuaikan dengan nama, riwayat chat terakhir, dan konteks bisnis. Hasil tidak akan sama antar kontak.
                      </Typography>
                    </Alert>
                    <TextField
                      fullWidth
                      multiline
                      rows={3}
                      size="small"
                      label="Instruksi ke AI"
                      value={s.ai_instruction}
                      onChange={e => setStep(i, { ai_instruction: e.target.value, message: e.target.value })}
                      placeholder="Contoh: Tanyakan apakah barang sudah sampai dengan baik, tawarkan promo repeat order diskon 10% untuk pembelian berikutnya. Jangan terlalu memaksa."
                      helperText="Tulis apa yang harus dilakukan AI. AI akan membaca instruksi + riwayat chat kontak untuk menulis pesan yang personal."
                    />
                  </>
                ) : (
                  <WhatsAppEditor value={s.message} onChange={v => setStep(i, { message: v })}
                    placeholder="Halo {nama}, gimana kabarnya? ..." rows={3} />
                )}
              </Paper>
            ))}
            <Button startIcon={<AddIcon />} variant="outlined" onClick={addStep} size="small" sx={{ alignSelf: 'flex-start' }}>Tambah langkah berikutnya</Button>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Batal</Button>
          <Button variant="contained" onClick={submit} disabled={save.isPending}>
            {save.isPending ? 'Menyimpan...' : 'Simpan Urutan'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Tambahkan kontak agar mulai mengikuti urutan */}
      <Dialog open={!!enrollFu} onClose={() => setEnrollFu(null)} fullWidth maxWidth="sm" fullScreen={mobileDialog}>
        <DialogTitle>Tambahkan Kontak ke Follow-up</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={1.5}>
            <Alert severity="info" icon={false}>
              Kontak yang ditambahkan akan mulai mengikuti urutan <b>{enrollFu?.name}</b>. Waktu setiap langkah dihitung setelah Anda menekan Mulai Follow-up.
            </Alert>
            {allTags.length > 0 && (
              <FormControl size="small" fullWidth>
                <InputLabel>Ambil kontak dari tag</InputLabel>
                <Select label="Ambil kontak dari tag" value={enrollTag} onChange={e => fillFromTag(e.target.value)}>
                  <MenuItem value=""><em>— pilih tag —</em></MenuItem>
                  {allTags.map(t => <MenuItem key={t} value={t}>{t}</MenuItem>)}
                </Select>
              </FormControl>
            )}
            <FormControl size="small" fullWidth>
              <InputLabel>Ambil dari status CRM</InputLabel>
              <Select label="Ambil dari status CRM" value={enrollStage} onChange={e => fillFromStage(e.target.value as LeadStage | '')}>
                <MenuItem value=""><em>— pilih status —</em></MenuItem>
                {CRM_STAGES.map(item => (
                  <MenuItem key={item.value} value={item.value}>
                    {item.label} ({crm?.stage_counts?.[item.value] || 0})
                  </MenuItem>
                ))}
              </Select>
            </FormControl>
            <RecipientField agentId={agentId} value={recipients} onChange={setRecipients} />
            <Typography variant="caption" color="text.secondary">
              Kontak yang sedang mengikuti urutan ini atau sudah memilih STOP otomatis dilewati.
            </Typography>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEnrollFu(null)}>Batal</Button>
          <Button variant="contained" onClick={doEnroll} disabled={enroll.isPending}>
            {enroll.isPending ? 'Memulai...' : 'Mulai Follow-up'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
