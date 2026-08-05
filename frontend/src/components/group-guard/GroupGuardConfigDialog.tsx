import { useState } from 'react';
import {
  Alert, Avatar, Box, Button, CircularProgress, Dialog, DialogActions, DialogContent,
  DialogTitle, FormControlLabel, IconButton, Paper, Radio, RadioGroup,
  Stack, Switch, TextField, Typography, useMediaQuery,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import AdminIcon from '@mui/icons-material/AdminPanelSettingsOutlined';
import CheckIcon from '@mui/icons-material/TaskAltOutlined';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import LinkIcon from '@mui/icons-material/LinkOutlined';
import PhoneIcon from '@mui/icons-material/PhoneOutlined';
import ShieldIcon from '@mui/icons-material/ShieldOutlined';
import SpeedIcon from '@mui/icons-material/SpeedOutlined';
import { useGroupConfig, useSaveGroupConfig } from '../../hooks';
import { swalConfirm, swalToast } from '../../services/swal';
import type { GroupGuardConfig, WAGroup } from '../../types';
import { LoadingState } from './GroupGuardShared';

type KickMode = 'none' | 'review' | 'auto';

function SettingSwitch({ icon, title, description, checked, disabled = false, onChange }: {
  icon: React.ReactNode;
  title: string;
  description: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center', py: 0.75, opacity: disabled ? 0.6 : 1 }}>
      <Box sx={{ color: checked ? 'primary.main' : 'text.disabled', display: 'flex' }}>{icon}</Box>
      <Box sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="body2" sx={{ fontWeight: 700 }}>{title}</Typography>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', lineHeight: 1.35 }}>{description}</Typography>
      </Box>
      <Switch checked={checked} disabled={disabled} onChange={event => onChange(event.target.checked)} slotProps={{ input: { 'aria-label': title } }} />
    </Stack>
  );
}

function RuleSection({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return (
    <Box>
      <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>{title}</Typography>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.75 }}>{description}</Typography>
      {children}
    </Box>
  );
}

export default function GroupGuardConfigDialog({ agentId, group, onClose }: { agentId: number; group: WAGroup; onClose: () => void }) {
  const theme = useTheme();
  const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
  const { data, isLoading, isError, refetch } = useGroupConfig(agentId, group.jid);

  if (!data) {
    return (
      <Dialog open onClose={onClose} maxWidth="md" fullWidth fullScreen={fullScreen}>
        <DialogTitle sx={{ pr: 6 }}>
          <Typography component="span" variant="h6" sx={{ display: 'block', fontWeight: 800 }}>Aturan Grup</Typography>
          <Typography component="span" variant="body2" color="text.secondary" sx={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {group.name || group.jid}
          </Typography>
          <IconButton aria-label="Tutup" onClick={onClose} sx={{ position: 'absolute', top: 10, right: 10 }}>
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent dividers sx={{ p: { xs: 1.5, sm: 2.5 } }}>
          {isLoading && <LoadingState label="Memuat aturan..." />}
          {isError && (
            <Alert severity="warning" action={<Button color="inherit" size="small" onClick={() => refetch()}>Coba lagi</Button>}>
              Aturan grup belum bisa dimuat.
            </Alert>
          )}
        </DialogContent>
      </Dialog>
    );
  }

  const initialForm = {
    ...data,
    group_jid: group.jid,
    group_name: group.name || data.group_name,
  };

  return (
    <ConfigEditor
      key={group.jid}
      agentId={agentId}
      group={group}
      initialForm={initialForm}
      fullScreen={fullScreen}
      onClose={onClose}
    />
  );
}

function ConfigEditor({ agentId, group, initialForm, fullScreen, onClose }: {
  agentId: number;
  group: WAGroup;
  initialForm: GroupGuardConfig;
  fullScreen: boolean;
  onClose: () => void;
}) {
  const save = useSaveGroupConfig(agentId);
  const [form, setForm] = useState<GroupGuardConfig>(initialForm);

  const set = (patch: Partial<GroupGuardConfig>) => setForm(current => ({ ...current, ...patch }));
  const isDirty = JSON.stringify(form) !== JSON.stringify(initialForm);
  const kickMode: KickMode = form.auto_kick ? 'auto' : form.flag_for_kick ? 'review' : 'none';
  const hasDetectionRule = form.block_links || form.block_phones || !!form.block_words.trim() || form.flood_count > 0;

  const requestClose = async () => {
    if (save.isPending) return;
    if (isDirty && !await swalConfirm('Tutup tanpa menyimpan?', 'Perubahan aturan di grup ini akan hilang.')) return;
    onClose();
  };

  const onSave = async () => {
    if (form.enabled && !hasDetectionRule) {
      swalToast('Pilih minimal satu jenis deteksi sebelum mengaktifkan anti-spam', 'error');
      return;
    }
    if (form.flood_count > 0 && form.flood_window_sec < 1) {
      swalToast('Jangka waktu anti-flood minimal 1 detik', 'error');
      return;
    }
    try {
      await save.mutateAsync(form);
      swalToast(form.enabled ? 'Anti-Spam Grup diaktifkan' : 'Aturan grup disimpan');
      onClose();
    } catch {
      swalToast('Aturan belum bisa disimpan', 'error');
    }
  };

  return (
    <Dialog open onClose={requestClose} maxWidth="md" fullWidth fullScreen={fullScreen}>
      <DialogTitle sx={{ pr: 6 }}>
        <Typography component="span" variant="h6" sx={{ display: 'block', fontWeight: 800 }}>Aturan Grup</Typography>
        <Typography component="span" variant="body2" color="text.secondary" sx={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {group.name || group.jid}
        </Typography>
        <IconButton aria-label="Tutup" onClick={requestClose} disabled={save.isPending} sx={{ position: 'absolute', top: 10, right: 10 }}>
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent dividers sx={{ p: { xs: 1.5, sm: 2.5 } }}>
        <Stack spacing={1.5}>
          <Paper variant="outlined" sx={{ p: 1.25, borderColor: form.enabled ? 'success.light' : 'divider', bgcolor: form.enabled ? 'rgba(31, 138, 80, 0.05)' : 'action.hover' }}>
            <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center' }}>
              <Avatar sx={{ width: 36, height: 36, bgcolor: form.enabled ? 'success.main' : 'action.disabledBackground' }}>
                <ShieldIcon fontSize="small" />
              </Avatar>
              <Box sx={{ flex: 1 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 800 }}>{form.enabled ? 'Anti-spam aktif' : 'Anti-spam belum aktif'}</Typography>
                <Typography variant="caption" color="text.secondary">
                  {form.enabled ? 'Pesan baru akan diperiksa memakai aturan di bawah.' : 'Aktifkan anti-spam untuk membuka dan mengubah aturan di bawah.'}
                </Typography>
              </Box>
              <Switch checked={form.enabled} onChange={event => set({ enabled: event.target.checked })} slotProps={{ input: { 'aria-label': 'Aktifkan anti-spam' } }} />
            </Stack>
          </Paper>

          {!group.bot_is_admin && (
            <Alert severity="warning" icon={<AdminIcon fontSize="small" />}>
              Nomor ini belum menjadi admin. Deteksi tetap berjalan setelah anti-spam aktif, tetapi hapus pesan dan keluarkan anggota belum dapat dijalankan.
            </Alert>
          )}

          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: 'minmax(0, 1.05fr) minmax(0, 0.95fr)' }, gap: 1.5, alignItems: 'start' }}>
            <Paper variant="outlined" aria-disabled={!form.enabled} sx={{ p: 1.25, minWidth: 0, opacity: form.enabled ? 1 : 0.62, bgcolor: form.enabled ? 'background.paper' : 'action.disabledBackground' }}>
              <RuleSection title="1. Pesan yang dianggap spam" description="Aktifkan hanya pemeriksaan yang sesuai dengan aturan grupmu.">
                <SettingSwitch
                  icon={<LinkIcon fontSize="small" />}
                  title="Tautan"
                  description="Tandai pesan yang berisi alamat situs atau link."
                  checked={form.block_links}
                  disabled={!form.enabled}
                  onChange={checked => set({ block_links: checked })}
                />
                <SettingSwitch
                  icon={<PhoneIcon fontSize="small" />}
                  title="Nomor telepon"
                  description="Tandai pesan yang memuat rangkaian nomor telepon."
                  checked={form.block_phones}
                  disabled={!form.enabled}
                  onChange={checked => set({ block_phones: checked })}
                />
                <TextField
                  fullWidth
                  size="small"
                  label="Kata atau frasa terlarang"
                  value={form.block_words}
                  disabled={!form.enabled}
                  multiline
                  minRows={2}
                  onChange={event => set({ block_words: event.target.value })}
                  placeholder={'judi\npinjol\npromo tertentu'}
                  helperText="Pisahkan dengan baris baru atau koma. Kosongkan bila tidak digunakan."
                  sx={{ mt: 0.75 }}
                />
                <Box sx={{ mt: 1.5 }}>
                  <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', mb: 0.75 }}>
                    <SpeedIcon fontSize="small" color={form.enabled && form.flood_count > 0 ? 'primary' : 'disabled'} />
                    <Box>
                      <Typography variant="body2" sx={{ fontWeight: 700 }}>Batasi pesan beruntun</Typography>
                      <Typography variant="caption" color="text.secondary">Deteksi satu anggota yang mengirim terlalu banyak pesan dalam waktu singkat.</Typography>
                    </Box>
                  </Stack>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: { xs: 'stretch', sm: 'flex-start' } }}>
                    <TextField
                      type="number"
                      size="small"
                      label="Batas pesan"
                      value={form.flood_count}
                      disabled={!form.enabled}
                      onChange={event => set({ flood_count: Math.max(0, Number(event.target.value) || 0) })}
                      helperText="0 = tidak diperiksa"
                      slotProps={{ htmlInput: { min: 0 } }}
                      sx={{ flex: 1 }}
                    />
                    <TextField
                      type="number"
                      size="small"
                      label="Dalam waktu (detik)"
                      value={form.flood_window_sec}
                      onChange={event => set({ flood_window_sec: Math.max(1, Number(event.target.value) || 1) })}
                      disabled={!form.enabled || form.flood_count === 0}
                      slotProps={{ htmlInput: { min: 1 } }}
                      sx={{ flex: 1 }}
                    />
                  </Stack>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
                    Contoh: batas 5 pesan dalam 10 detik berarti pesan ke-5 dari anggota yang sama akan dianggap spam.
                  </Typography>
                </Box>
                {form.enabled && !hasDetectionRule && (
                  <Alert severity="warning" sx={{ mt: 1 }}>
                    Pilih minimal satu jenis deteksi agar anti-spam dapat bekerja.
                  </Alert>
                )}
              </RuleSection>
            </Paper>

            <Stack spacing={1.5} sx={{ minWidth: 0 }}>
              <Paper variant="outlined" aria-disabled={!form.enabled} sx={{ p: 1.25, opacity: form.enabled ? 1 : 0.62, bgcolor: form.enabled ? 'background.paper' : 'action.disabledBackground' }}>
                <RuleSection title="2. Tindakan saat spam terdeteksi" description="Tentukan tindakan setelah pesan memenuhi aturan di sebelah kiri.">
                  <SettingSwitch
                    icon={<DeleteIcon fontSize="small" />}
                    title="Hapus pesan spam"
                    description="Pesan dihapus otomatis bila nomor ini memiliki akses admin."
                    checked={form.delete_spam}
                    disabled={!form.enabled}
                    onChange={checked => set({ delete_spam: checked })}
                  />

                  <Typography variant="body2" sx={{ mt: 1, mb: 0.5, fontWeight: 700 }}>Tindakan terhadap anggota</Typography>
                  <RadioGroup value={kickMode} onChange={event => set({ flag_for_kick: event.target.value === 'review', auto_kick: event.target.value === 'auto' })}>
                    <FormControlLabel disabled={!form.enabled} value="none" control={<Radio size="small" />} label={
                      <Box><Typography variant="body2" sx={{ fontWeight: 650 }}>Tidak ada tindakan</Typography><Typography variant="caption" color="text.secondary">Hanya catat dan hapus pesan sesuai aturan.</Typography></Box>
                    } sx={{ alignItems: 'flex-start', py: 0.35 }} />
                    <FormControlLabel disabled={!form.enabled} value="review" control={<Radio size="small" />} label={
                      <Box><Typography variant="body2" sx={{ fontWeight: 650 }}>Minta konfirmasi</Typography><Typography variant="caption" color="text.secondary">Masukkan anggota ke antrean Aktivitas untuk ditinjau dulu.</Typography></Box>
                    } sx={{ alignItems: 'flex-start', py: 0.35 }} />
                    <FormControlLabel disabled={!form.enabled} value="auto" control={<Radio size="small" color="warning" />} label={
                      <Box><Typography variant="body2" sx={{ fontWeight: 650 }}>Keluarkan otomatis</Typography><Typography variant="caption" color="text.secondary">Anggota langsung dikeluarkan tanpa pemeriksaan manual.</Typography></Box>
                    } sx={{ alignItems: 'flex-start', py: 0.35 }} />
                  </RadioGroup>
                  {form.enabled && kickMode === 'auto' && (
                    <Alert severity="warning" sx={{ mt: 0.75 }}>
                      Gunakan hanya jika aturan deteksi sudah diuji. Pesan yang sebenarnya aman tetap bisa salah terdeteksi.
                    </Alert>
                  )}
                </RuleSection>
              </Paper>

              <Paper variant="outlined" aria-disabled={!form.enabled} sx={{ p: 1.25, opacity: form.enabled ? 1 : 0.62, bgcolor: form.enabled ? 'background.paper' : 'action.disabledBackground' }}>
                <RuleSection title="3. Pengecualian" description="Admin grup selalu dikecualikan secara otomatis.">
                  <TextField
                    fullWidth
                    size="small"
                    label="Nomor yang tidak diperiksa"
                    value={form.allow_numbers}
                    disabled={!form.enabled}
                    multiline
                    minRows={2}
                    onChange={event => set({ allow_numbers: event.target.value })}
                    placeholder={'6281234567890\n6289876543210'}
                    helperText="Gunakan format nomor WhatsApp. Pisahkan dengan baris baru atau koma."
                  />
                </RuleSection>
              </Paper>
            </Stack>
          </Box>
        </Stack>
      </DialogContent>

      <DialogActions sx={{ px: { xs: 1.5, sm: 2.5 }, py: 1.25 }}>
        <Button onClick={requestClose} disabled={save.isPending}>Batal</Button>
        <Button variant="contained" onClick={onSave} disabled={!isDirty || save.isPending} startIcon={save.isPending ? <CircularProgress size={16} color="inherit" /> : <CheckIcon />}>
          {save.isPending ? 'Menyimpan...' : 'Simpan aturan'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
