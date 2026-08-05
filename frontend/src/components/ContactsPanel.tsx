import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  Box, Typography, Button, Stack, Chip, IconButton, Checkbox, Card, CardContent, Alert, Divider, Tooltip, Avatar,
  Dialog, DialogTitle, DialogContent, DialogActions, TextField, CircularProgress, InputAdornment, FormControlLabel, Switch,
  Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Paper, Pagination, MenuItem,
} from '@mui/material';
import EmptyState from './common/EmptyState';
import PeopleIcon from '@mui/icons-material/PeopleOutlined';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import SearchIcon from '@mui/icons-material/Search';
import ChatIcon from '@mui/icons-material/ChatBubbleOutlineOutlined';
import CampaignIcon from '@mui/icons-material/CampaignOutlined';
import LocalOfferIcon from '@mui/icons-material/LocalOfferOutlined';
import CloseIcon from '@mui/icons-material/Close';
import UploadFileIcon from '@mui/icons-material/UploadFileOutlined';
import AutoAwesomeIcon from '@mui/icons-material/AutoAwesomeOutlined';
import LockIcon from '@mui/icons-material/LockOutlined';
import { useBroadcastConsentSummary, useCrmContacts, useSaveCrmContact, useDeleteCrmContact, useCrmContactsExport, useBulkDeleteCrmContacts, useBulkStageCrmContacts } from '../hooks';
import type { LeadStage, SavedContact } from '../types';
import api from '../services/api';
import PageHeader from './PageHeader';
import ContactImportDialog from './contacts/ContactImportDialog';
import { swalConfirm, swalToast } from '../services/swal';

const EMPTY: Partial<SavedContact> = { number: '', name: '', notes: '', tags: '', lead_stage: 'new' };

const LEAD_STAGES: { value: LeadStage; label: string; color: string; bg: string }[] = [
  { value: 'new', label: 'Baru', color: '#455a64', bg: '#eceff1' },
  { value: 'cold', label: 'Cold', color: '#1565c0', bg: '#e3f2fd' },
  { value: 'warm', label: 'Warm', color: '#a15c00', bg: '#fff3e0' },
  { value: 'hot', label: 'Hot', color: '#c62828', bg: '#ffebee' },
  { value: 'customer', label: 'Pelanggan', color: '#2e7d32', bg: '#e8f5e9' },
  { value: 'unqualified', label: 'Tidak potensial', color: '#616161', bg: '#eeeeee' },
];

const stageMeta = (stage: LeadStage) => LEAD_STAGES.find(item => item.value === stage) || LEAD_STAGES[0];
const safeLeadStage = (value: unknown): LeadStage =>
  LEAD_STAGES.some(item => item.value === value) ? value as LeadStage : 'new';
const apiStatus = (error: unknown): number | undefined =>
  (error as { response?: { status?: number } } | null)?.response?.status;

const stageSourceLabel = (contact: SavedContact): string => {
  if (contact.lead_stage_locked) return 'Diatur manual';
  if (contact.lead_stage_source === 'ai') return `Dinilai AI${contact.lead_stage_confidence ? ` · ${Math.round(contact.lead_stage_confidence * 100)}%` : ''}`;
  if (contact.lead_stage_source === 'activity') return 'Dari aktivitas';
  if (contact.lead_stage_source === 'manual') return 'AI aktif · menunggu penilaian';
  return 'Otomatis';
};

function StageAssessment({ contact }: { contact: SavedContact }) {
  const manual = contact.lead_stage_locked;
  return (
    <Tooltip title={contact.lead_stage_reason || (manual ? 'AI tidak akan mengubah status ini.' : 'Status diperbarui otomatis.')} arrow>
      <Stack direction="row" spacing={0.4} sx={{ mt: 0.35, alignItems: 'center', color: 'text.secondary', width: 'fit-content' }}>
        {manual ? <LockIcon sx={{ fontSize: 12 }} /> : <AutoAwesomeIcon sx={{ fontSize: 12 }} />}
        <Typography variant="caption" sx={{ fontSize: 10.5 }}>{stageSourceLabel(contact)}</Typography>
      </Stack>
    </Tooltip>
  );
}

function StageSelect({ value, onChange, fullWidth = false, disabled = false }: {
  value: LeadStage | undefined | null;
  onChange: (stage: LeadStage) => void;
  fullWidth?: boolean;
  disabled?: boolean;
}) {
  const safeValue = safeLeadStage(value);
  const meta = stageMeta(safeValue);
  return (
    <TextField select size="small" value={safeValue} onChange={e => onChange(e.target.value as LeadStage)} fullWidth={fullWidth} disabled={disabled}
      aria-label="Status CRM"
      sx={{ minWidth: fullWidth ? undefined : 122, '& .MuiInputBase-root': { height: 30, color: meta.color, bgcolor: meta.bg, fontSize: 12, fontWeight: 750 } }}>
      {LEAD_STAGES.map(item => <MenuItem key={item.value} value={item.value}>{item.label}</MenuItem>)}
    </TextField>
  );
}

function PipelineOverview({ counts, active, onPick, disabled }: {
  counts: Record<LeadStage, number>;
  active: LeadStage | '';
  onPick: (stage: LeadStage | '') => void;
  disabled: boolean;
}) {
  const total = LEAD_STAGES.reduce((sum, item) => sum + (counts[item.value] || 0), 0);
  return (
    <Paper variant="outlined" sx={{ p: 1.25, mb: 1.25, borderRadius: 1.25 }}>
      <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1, mb: 1 }}>
        <Box>
          <Typography variant="body2" sx={{ fontWeight: 800 }}>Pipeline CRM</Typography>
          <Typography variant="caption" color="text.secondary">
            {total} kontak{active ? ` · filter ${stageMeta(active).label}` : ''}
          </Typography>
        </Box>
        {active && <Button size="small" color="inherit" onClick={() => onPick('')}>Tampilkan semua</Button>}
      </Stack>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(3, minmax(0, 1fr))', lg: 'repeat(6, minmax(0, 1fr))' }, gap: 0.75 }}>
        {LEAD_STAGES.map(item => {
          const selected = active === item.value;
          return (
            <Button key={item.value} variant="outlined" disabled={disabled} onClick={() => onPick(item.value)}
              sx={{ px: 1, py: 0.75, minWidth: 0, justifyContent: 'flex-start', textTransform: 'none', borderColor: selected ? item.color : 'divider', bgcolor: selected ? item.bg : 'transparent', color: item.color }}>
              <Box sx={{ textAlign: 'left', minWidth: 0 }}>
                <Typography sx={{ fontSize: 17, lineHeight: 1.1, fontWeight: 900 }}>{counts[item.value] || 0}</Typography>
                <Typography variant="caption" sx={{ color: 'inherit', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis', display: 'block' }}>{item.label}</Typography>
              </Box>
            </Button>
          );
        })}
      </Box>
    </Paper>
  );
}

export default function ContactsPanel({ agentId, onBroadcast, onOpenChat }: {
  agentId: number;
  onBroadcast: (recipients: string) => void;
  onOpenChat: (number: string) => void;
}) {
  const [addOpen, setAddOpen] = useState(false);
  const [edit, setEdit] = useState<SavedContact | null>(null);
  const [open, setOpen] = useState(false);
  const [form, setForm] = useState<Partial<SavedContact>>(EMPTY);
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});
  const [q, setQ] = useState('');
  const [tag, setTag] = useState('');
  const [stage, setStage] = useState<LeadStage | ''>('');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [bulkTag, setBulkTag] = useState('');
  const [bulkApplying, setBulkApplying] = useState(false);
  const [tagModalOpen, setTagModalOpen] = useState(false);
  const [stageModalOpen, setStageModalOpen] = useState(false);
  const [bulkStage, setBulkStage] = useState<LeadStage>('warm');
  const [importOpen, setImportOpen] = useState(false);

  const { data, isLoading } = useCrmContacts(agentId, q, tag, stage, page);
  const { data: consentSummary } = useBroadcastConsentSummary(agentId);
  const saveCrmContact = useSaveCrmContact(agentId);
  const deleteCrmContact = useDeleteCrmContact(agentId);
  const bulkDelete = useBulkDeleteCrmContacts(agentId);
  const bulkStageMutation = useBulkStageCrmContacts(agentId);
  const crmExport = useCrmContactsExport(agentId);
  const queryClient = useQueryClient();

  const contacts = data?.data || [];
  const allTags = data?.all_tags || [];
  const totalContacts = data?.total ?? 0;
  const crmBackendReady = !data || !!data.stage_counts;
  const stageCounts: Record<LeadStage, number> = data?.stage_counts || {
    new: totalContacts, cold: 0, warm: 0, hot: 0, customer: 0, unqualified: 0,
  };
  const totalPages = Math.max(1, Math.ceil(totalContacts / (data?.limit ?? 20)));
  const selectedContacts = contacts.filter(c => selected.has(c.id));
  const hasFilter = !!q.trim() || !!tag || !!stage;

  const openAdd = () => { setForm(EMPTY); setFormErrors({}); setAddOpen(true); };
  const openEdit = (ct: SavedContact) => {
    const normalized = { ...ct, lead_stage: safeLeadStage(ct.lead_stage) };
    setForm(normalized); setFormErrors({}); setEdit(normalized); setOpen(true);
  };
  const closeDialog = () => { setAddOpen(false); setOpen(false); setEdit(null); setFormErrors({}); };

  const validate = (): boolean => {
    const errs: Record<string, string> = {};
    if (!form.number?.trim()) errs.number = 'Nomor WhatsApp wajib diisi';
    setFormErrors(errs);
    return Object.keys(errs).length === 0;
  };

  const save = async () => {
    if (!validate()) return;
    try {
      const payload = { ...form };
      if (edit && payload.lead_stage === edit.lead_stage) delete payload.lead_stage;
      if (edit && payload.lead_stage_locked === edit.lead_stage_locked) delete payload.lead_stage_locked;
      await saveCrmContact.mutateAsync(payload);
      swalToast(addOpen ? 'Kontak ditambahkan' : 'Kontak disimpan');
      closeDialog();
    } catch {
      swalToast('Kontak belum bisa disimpan', 'error');
    }
  };

  const remove = async (ct: SavedContact) => {
    if (!await swalConfirm(`Hapus kontak ${ct.name || ct.number}?`, 'Kontak yang dihapus tidak muncul lagi di daftar CRM.')) return;
    try {
      await deleteCrmContact.mutateAsync(ct.id);
      setSelected(prev => {
        const next = new Set(prev);
        next.delete(ct.id);
        return next;
      });
      swalToast('Kontak dihapus');
    } catch {
      swalToast('Kontak belum bisa dihapus', 'error');
    }
  };

  const pickStage = (next: LeadStage | '') => { setStage(prev => prev === next ? '' : next); setPage(1); setSelected(new Set()); };

  const handleBroadcast = async () => {
    try {
      const list = selectedContacts.length > 0 ? selectedContacts : await crmExport.mutateAsync({ q, tag, stage });
      const lines = list.map(c => `${c.number},${c.name || ''}`);
      onBroadcast(lines.join('\n'));
      swalToast(`${list.length} kontak dikirim ke Blast`);
    } catch {
      swalToast('Kontak belum bisa dikirim ke Blast', 'error');
    }
  };

  const toggleSelect = (id: number) => {
    setSelected(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (selected.size === contacts.length && contacts.length > 0) {
      setSelected(new Set());
    } else {
      setSelected(new Set(contacts.map(c => c.id)));
    }
  };

  const handleBulkTag = async () => {
    if (!bulkTag.trim() || selected.size === 0) return;
    setBulkApplying(true);
    try {
      await api.post(`/agents/${agentId}/crm/contacts/bulk-tag`, {
        ids: Array.from(selected),
        tag: bulkTag.trim(),
      });
      queryClient.invalidateQueries({ queryKey: ['crm-contacts', agentId] });
      setSelected(new Set());
      setBulkTag('');
      swalToast('Tag ditambahkan');
    } catch {
      swalToast('Tag belum bisa ditambahkan', 'error');
    } finally {
      setBulkApplying(false);
    }
  };

  const changeStage = async (ids: number[], leadStage: LeadStage, clearSelection = false): Promise<boolean> => {
    if (ids.length === 0) return false;
    try {
      const res = await bulkStageMutation.mutateAsync({ ids, lead_stage: leadStage });
      if (clearSelection) setSelected(new Set());
      else setSelected(prev => {
        const next = new Set(prev);
        ids.forEach(id => next.delete(id));
        return next;
      });
      swalToast(`${res.updated} kontak dipindahkan ke ${stageMeta(leadStage).label}`);
      return true;
    } catch (error) {
      swalToast(apiStatus(error) === 404
        ? 'Backend belum dimuat ulang. Restart backend agar fitur status CRM aktif.'
        : 'Status CRM belum bisa diubah', 'error');
      return false;
    }
  };

  const handleBulkDeleteSelected = async () => {
    if (selected.size === 0) return;
    if (!await swalConfirm(`Hapus ${selected.size} kontak terpilih?`, 'Kontak yang dihapus tidak muncul lagi di daftar CRM.')) return;
    try {
      const res = await bulkDelete.mutateAsync({ ids: Array.from(selected) });
      setSelected(new Set());
      swalToast(`${res.deleted} kontak dihapus`);
    } catch {
      swalToast('Kontak belum bisa dihapus', 'error');
    }
  };

  const handleDeleteAll = async () => {
    const scope = hasFilter ? 'semua kontak yang cocok filter ini' : 'SEMUA kontak';
    if (!await swalConfirm(`Hapus ${scope}?`, 'Tindakan ini tidak bisa dibatalkan. Kontak akan hilang dari daftar CRM.')) return;
    try {
      const res = await bulkDelete.mutateAsync({ all: true, q, tag, stage });
      setSelected(new Set());
      setPage(1);
      swalToast(`${res.deleted} kontak dihapus`);
    } catch {
      swalToast('Kontak belum bisa dihapus', 'error');
    }
  };

  const contactInitial = (ct: SavedContact) => {
    const base = (ct.name || ct.number || '?').trim();
    return base.slice(0, 1).toUpperCase();
  };

  return (
    <Box>
      <PageHeader
        title="Kontak"
        subtitle="Kelola prospek, tentukan prioritas, lalu tindak lanjuti dari satu tempat."
        action={
          <Stack direction="row" spacing={0.75} sx={{ width: '100%' }}>
            <Button variant="outlined" startIcon={<UploadFileIcon />} onClick={() => setImportOpen(true)} sx={{ flex: { xs: 1, sm: 'initial' } }}>Impor</Button>
            <Button variant="contained" startIcon={<AddIcon />} onClick={openAdd} sx={{ flex: { xs: 1, sm: 'initial' } }}>Tambah Kontak</Button>
          </Stack>
        }
      />

      {!crmBackendReady && (
        <Alert severity="warning" sx={{ mb: 1.25 }}>
          Backend masih memakai versi lama. Restart backend terlebih dahulu agar status CRM dapat disimpan.
        </Alert>
      )}

      {crmBackendReady && (
        <Alert severity="info" icon={<AutoAwesomeIcon fontSize="inherit" />} sx={{ mb: 1.25 }}>
          AI menilai status dari konteks chat saat Asisten AI aktif. Jika kurang tepat, ubah statusnya; pilihan manual akan dikunci agar tidak ditimpa otomatis.
        </Alert>
      )}

      <PipelineOverview counts={stageCounts} active={stage} onPick={pickStage} disabled={isLoading || !crmBackendReady} />

      <Paper variant="outlined" sx={{ mb: 1.25, borderRadius: 1.25, overflow: 'hidden' }}>
        <Box sx={{ p: 1.25 }}>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={0.75} sx={{ alignItems: { xs: 'stretch', md: 'center' } }}>
            <TextField
              fullWidth size="small" placeholder="Cari nama atau nomor"
              value={q} onChange={e => { setQ(e.target.value); setPage(1); setSelected(new Set()); }}
              slotProps={{
                input: {
                  startAdornment: <InputAdornment position="start"><SearchIcon fontSize="small" /></InputAdornment>,
                  endAdornment: q ? (
                    <InputAdornment position="end">
                      <IconButton size="small" onClick={() => { setQ(''); setPage(1); setSelected(new Set()); }}><CloseIcon fontSize="small" /></IconButton>
                    </InputAdornment>
                  ) : undefined,
                },
              }}
            />
            <TextField select size="small" label="Tag" value={tag} disabled={allTags.length === 0}
              onChange={e => { setTag(e.target.value); setPage(1); setSelected(new Set()); }}
              sx={{ minWidth: { md: 170 } }}>
              <MenuItem value="">Semua tag</MenuItem>
              {allTags.map(item => <MenuItem key={item} value={item}>{item}</MenuItem>)}
            </TextField>
            <Button variant="contained" startIcon={<CampaignIcon />} onClick={handleBroadcast}
              disabled={(selectedContacts.length === 0 && totalContacts === 0) || crmExport.isPending}
              sx={{ minWidth: { md: 170 }, whiteSpace: 'nowrap' }}>
              {selected.size ? 'Blast terpilih' : 'Blast hasil filter'}
            </Button>
          </Stack>
        </Box>
        {consentSummary && (
          <>
            <Divider />
            <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ px: 1.25, py: 0.75, gap: 0.75, alignItems: { xs: 'flex-start', sm: 'center' } }}>
              <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>
                Kesiapan promo berdasarkan aktivitas di SlaluDiskon
              </Typography>
              <Stack direction="row" sx={{ gap: 0.5, flexWrap: 'wrap' }}>
                <Chip size="small" color="success" variant="outlined" label={`${consentSummary.marketing_consent} izin promo`} />
                <Chip size="small" variant="outlined" label={`${consentSummary.interacted} pernah chat`} />
                <Chip size="small" color={consentSummary.opted_out ? 'warning' : 'default'} variant="outlined" label={`${consentSummary.opted_out} STOP`} />
              </Stack>
            </Stack>
          </>
        )}
      </Paper>

      {isLoading ? (
        <Paper variant="outlined" sx={{ textAlign: 'center', py: 4 }}>
          <CircularProgress size={24} />
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>Memuat kontak...</Typography>
        </Paper>
      ) : contacts.length === 0 ? (
        <EmptyState
          icon={<PeopleIcon sx={{ fontSize: 48 }} />}
          title={hasFilter ? 'Tidak ada kontak' : 'Belum ada kontak'}
          description={hasFilter
            ? 'Coba ubah filter atau kata kunci.'
            : 'Kontak masuk otomatis saat pelanggan chat. Atau impor manual, dari nomor terkoneksi, maupun file CSV.'}
          actionLabel={hasFilter ? undefined : 'Impor Kontak'}
          onAction={hasFilter ? undefined : () => setImportOpen(true)}
        />
      ) : (
        <>
          {selected.size > 0 && (
            <Paper variant="outlined" sx={{ p: 0.75, mb: 1, borderColor: 'primary.light', bgcolor: 'background.paper', position: 'sticky', top: 8, zIndex: 3, boxShadow: 1 }}>
              <Stack direction="row" sx={{ alignItems: 'center', gap: 0.5, flexWrap: 'wrap' }}>
                <Chip label={`${selected.size} dipilih`} size="small" color="primary" onDelete={() => setSelected(new Set())} />
                <Box sx={{ flex: 1 }} />
                <Button variant="outlined" size="small" onClick={() => setStageModalOpen(true)} disabled={!crmBackendReady}>
                  Ubah status
                </Button>
                <Button variant="outlined" size="small" startIcon={<LocalOfferIcon />} onClick={() => setTagModalOpen(true)}>
                  Tambah Tag
                </Button>
                <Button variant="text" size="small" color="error" startIcon={<DeleteIcon />}
                  onClick={handleBulkDeleteSelected} disabled={bulkDelete.isPending}>
                  Hapus
                </Button>
              </Stack>
            </Paper>
          )}

          <Paper variant="outlined" sx={{ mb: 1, display: { xs: 'none', md: 'block' } }}>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ width: 40, p: 0.5 }}>
                      <Checkbox
                        size="small"
                        checked={contacts.length > 0 && selected.size === contacts.length}
                        indeterminate={selected.size > 0 && selected.size < contacts.length}
                        onChange={toggleSelectAll}
                      />
                    </TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Kontak</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Status CRM</TableCell>
                    <TableCell sx={{ fontWeight: 700, minWidth: 180 }}>Catatan</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Tag</TableCell>
                    <TableCell sx={{ fontWeight: 700 }}>Terakhir Chat</TableCell>
                    <TableCell sx={{ fontWeight: 700, width: 132 }}>Aksi</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {contacts.map(ct => (
                    <TableRow key={ct.id} hover selected={selected.has(ct.id)}>
                      <TableCell sx={{ p: 0.5 }}>
                        <Checkbox size="small" checked={selected.has(ct.id)} onChange={() => toggleSelect(ct.id)} />
                      </TableCell>
                      <TableCell>
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minWidth: 0 }}>
                          <Avatar sx={{ width: 30, height: 30, fontSize: 13, bgcolor: 'primary.main' }}>{contactInitial(ct)}</Avatar>
                          <Box sx={{ minWidth: 0 }}>
                            <Typography sx={{ fontWeight: 700, fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{ct.name || `+${ct.number}`}</Typography>
                            <Typography variant="caption" color="text.secondary">+{ct.number}</Typography>
                          </Box>
                        </Stack>
                      </TableCell>
                      <TableCell>
                        <StageSelect value={ct.lead_stage} onChange={next => changeStage([ct.id], next)} disabled={!crmBackendReady} />
                        <StageAssessment contact={ct} />
                      </TableCell>
                      <TableCell sx={{ maxWidth: 240 }}>
                        {ct.notes ? (
                          <Tooltip title={ct.notes} placement="top-start" arrow>
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', cursor: 'default' }}>
                              {ct.notes}
                            </Typography>
                          </Tooltip>
                        ) : (
                          <Typography variant="caption" color="text.disabled">Belum ada catatan</Typography>
                        )}
                      </TableCell>
                      <TableCell>
                        {ct.tags ? (
                          <Stack direction="row" spacing={0.5} sx={{ flexWrap: 'wrap', gap: 0.5 }}>
                            {ct.tags.split(',').map(t => t.trim()).filter(Boolean).slice(0, 3).map((t, i) => (
                              <Chip key={i} label={t} size="small" variant="outlined" sx={{ height: 20, fontSize: '0.65rem' }} />
                            ))}
                          </Stack>
                        ) : (
                          <Typography variant="caption" color="text.disabled">Belum ada tag</Typography>
                        )}
                      </TableCell>
                      <TableCell>
                        <Typography variant="caption" color="text.secondary">
                          {ct.last_at ? lastChatLabel(ct.last_at) : 'Belum ada riwayat'}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Stack direction="row" spacing={0.25}>
                          <Tooltip title="Buka chat"><IconButton size="small" onClick={() => onOpenChat(ct.number)}><ChatIcon fontSize="small" /></IconButton></Tooltip>
                          <Tooltip title="Edit kontak"><IconButton size="small" onClick={() => openEdit(ct)}><EditIcon fontSize="small" /></IconButton></Tooltip>
                          <Tooltip title="Hapus kontak"><IconButton size="small" color="error" onClick={() => remove(ct)}><DeleteIcon fontSize="small" /></IconButton></Tooltip>
                        </Stack>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          </Paper>

          <Stack spacing={1} sx={{ display: { xs: 'flex', md: 'none' }, mb: 1 }}>
            {contacts.map(ct => (
              <Card key={ct.id} variant="outlined" sx={{ borderColor: selected.has(ct.id) ? 'primary.main' : 'divider' }}>
                <CardContent sx={{ p: 1.25, '&:last-child': { pb: 1.25 } }}>
                  <Stack direction="row" spacing={1} sx={{ alignItems: 'flex-start' }}>
                    <Checkbox size="small" checked={selected.has(ct.id)} onChange={() => toggleSelect(ct.id)} sx={{ p: 0.25 }} />
                    <Avatar sx={{ width: 34, height: 34, fontSize: 14, bgcolor: 'primary.main', flexShrink: 0 }}>{contactInitial(ct)}</Avatar>
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                      <Stack direction="row" sx={{ alignItems: 'flex-start', justifyContent: 'space-between', gap: 0.75 }}>
                        <Box sx={{ minWidth: 0 }}>
                          <Typography sx={{ fontWeight: 800, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{ct.name || `+${ct.number}`}</Typography>
                          <Typography variant="caption" color="text.secondary">+{ct.number}</Typography>
                        </Box>
                        <Box sx={{ width: 126, flexShrink: 0 }}>
                          <StageSelect value={ct.lead_stage} onChange={next => changeStage([ct.id], next)} fullWidth disabled={!crmBackendReady} />
                          <StageAssessment contact={ct} />
                        </Box>
                      </Stack>
                      <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
                        {ct.last_at ? lastChatLabel(ct.last_at) : 'Belum ada riwayat chat'}
                      </Typography>
                      {ct.tags && (
                        <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 0.5, mt: 0.75 }}>
                          {ct.tags.split(',').map(t => t.trim()).filter(Boolean).slice(0, 4).map((t, i) => (
                            <Chip key={i} label={t} size="small" variant="outlined" sx={{ height: 20, fontSize: '0.65rem' }} />
                          ))}
                        </Stack>
                      )}
                      {ct.notes && (
                        <Box sx={{ mt: 0.75, px: 0.75, py: 0.5, bgcolor: 'action.hover', borderRadius: 0.75 }}>
                          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontWeight: 700 }}>Catatan</Typography>
                          <Typography variant="caption" sx={{ display: '-webkit-box', WebkitLineClamp: 3, WebkitBoxOrient: 'vertical', overflow: 'hidden', whiteSpace: 'pre-wrap' }}>
                            {ct.notes}
                          </Typography>
                        </Box>
                      )}
                    </Box>
                  </Stack>
                  <Divider sx={{ my: 1 }} />
                  <Stack direction="row" spacing={0.75} sx={{ justifyContent: 'flex-end' }}>
                    <Button size="small" startIcon={<ChatIcon />} onClick={() => onOpenChat(ct.number)}>Chat</Button>
                    <Button size="small" startIcon={<EditIcon />} onClick={() => openEdit(ct)}>Edit</Button>
                    <Button size="small" color="error" startIcon={<DeleteIcon />} onClick={() => remove(ct)}>Hapus</Button>
                  </Stack>
                </CardContent>
              </Card>
            ))}
          </Stack>

          <Stack direction={{ xs: 'column', sm: 'row' }} sx={{ alignItems: { xs: 'stretch', sm: 'center' }, justifyContent: 'space-between', mb: 1, gap: 1 }}>
            <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
              <Typography variant="body2" color="text.secondary">
                Menampilkan {contacts.length} dari {totalContacts} kontak
              </Typography>
              <Button size="small" color="error" startIcon={<DeleteIcon />} onClick={handleDeleteAll} disabled={bulkDelete.isPending}>
                {hasFilter ? 'Hapus hasil filter' : 'Hapus semua'}
              </Button>
            </Stack>
            <Pagination
              count={totalPages}
              page={page}
              onChange={(_e, p) => { setPage(p); setSelected(new Set()); }}
              size="small"
              siblingCount={0}
              boundaryCount={1}
            />
          </Stack>
        </>
      )}

      <Dialog open={stageModalOpen} onClose={() => setStageModalOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Ubah Status CRM</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <Alert severity="info" icon={false}>
              Status baru akan diterapkan ke {selected.size} kontak terpilih.
            </Alert>
            <TextField select label="Status CRM" size="small" value={bulkStage}
              onChange={e => setBulkStage(e.target.value as LeadStage)}>
              {LEAD_STAGES.map(item => <MenuItem key={item.value} value={item.value}>{item.label}</MenuItem>)}
            </TextField>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setStageModalOpen(false)}>Batal</Button>
          <Button variant="contained" disabled={bulkStageMutation.isPending || selected.size === 0}
            onClick={async () => { if (await changeStage(Array.from(selected), bulkStage, true)) setStageModalOpen(false); }}>
            Terapkan
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={tagModalOpen} onClose={() => { setTagModalOpen(false); setBulkTag(''); }} maxWidth="xs" fullWidth>
        <DialogTitle>Tambah Tag</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <Alert severity="info" icon={false}>
              Tag akan ditambahkan ke {selected.size} kontak yang sedang dipilih.
            </Alert>
            <TextField
              label="Tag"
              size="small"
              value={bulkTag}
              onChange={e => setBulkTag(e.target.value)}
              placeholder="vip, pelanggan tetap"
              autoFocus
            />
            {allTags.length > 0 && (
              <Box>
                <Typography variant="caption" color="text.secondary" sx={{ mb: 0.5, display: 'block' }}>
                  Tag yang sudah ada:
                </Typography>
                <Stack direction="row" sx={{ gap: 0.5, flexWrap: 'wrap' }}>
                  {allTags.map(t => (
                    <Chip key={t} label={t} size="small" variant="outlined" onClick={() => setBulkTag(t)}
                      sx={{ cursor: 'pointer', '&:hover': { opacity: 0.8 } }} />
                  ))}
                </Stack>
              </Box>
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setTagModalOpen(false); setBulkTag(''); }}>Batal</Button>
          <Button variant="contained" onClick={async () => { await handleBulkTag(); setTagModalOpen(false); }} disabled={!bulkTag.trim() || bulkApplying}>
            {bulkApplying ? '...' : 'Terapkan'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={addOpen || open} onClose={closeDialog} maxWidth="sm" fullWidth>
        <DialogTitle>{addOpen ? 'Tambah Kontak' : 'Edit Kontak'}</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ mt: 1 }}>
            <Alert severity="info" icon={false}>
              Kontak dari chat WhatsApp akan masuk otomatis. Form ini dipakai untuk menambah atau merapikan kontak manual.
            </Alert>
            <TextField label="Nama kontak" size="small" value={form.name || ''} onChange={e => setForm({...form, name: e.target.value})}
              placeholder="Budi, Sinta, Toko Maju" />
            <TextField label="Nomor WhatsApp" size="small" value={form.number || ''}
              onChange={e => { setForm({...form, number: e.target.value}); if (formErrors.number) setFormErrors(p => ({ ...p, number: '' })); }}
              disabled={!!edit} error={!!formErrors.number}
              helperText={formErrors.number || (edit ? 'Nomor tidak bisa diubah setelah kontak dibuat.' : 'Boleh pakai format 08xx atau 62xx.')} />
            <TextField select label="Status CRM" size="small" value={safeLeadStage(form.lead_stage)} disabled={!crmBackendReady}
              onChange={e => setForm({ ...form, lead_stage: e.target.value as LeadStage, lead_stage_locked: true })}
              helperText={crmBackendReady ? 'Gunakan satu status utama; tag tetap untuk segmentasi tambahan.' : 'Restart backend untuk mengaktifkan status CRM.'}>
              {LEAD_STAGES.map(item => <MenuItem key={item.value} value={item.value}>{item.label}</MenuItem>)}
            </TextField>
            {!!edit && form.lead_stage_reason && (
              <Alert severity="info" icon={form.lead_stage_locked ? <LockIcon fontSize="inherit" /> : <AutoAwesomeIcon fontSize="inherit" />}>
                <Typography variant="caption" sx={{ display: 'block', fontWeight: 800 }}>{stageSourceLabel(form as SavedContact)}</Typography>
                <Typography variant="body2">{form.lead_stage_reason}</Typography>
              </Alert>
            )}
            {!!edit && (
              <FormControlLabel
                control={<Switch checked={!form.lead_stage_locked} onChange={e => setForm({ ...form, lead_stage_locked: !e.target.checked })} />}
                label={
                  <Box>
                    <Typography variant="body2" sx={{ fontWeight: 700 }}>Izinkan AI memperbarui status</Typography>
                    <Typography variant="caption" color="text.secondary">
                      Nonaktifkan bila status pilihan Anda tidak boleh diubah otomatis.
                    </Typography>
                  </Box>
                }
              />
            )}
            <TextField label="Tag" size="small" value={form.tags || ''} onChange={e => setForm({...form, tags: e.target.value})}
              placeholder="vip, pelanggan tetap" helperText="Pisahkan beberapa tag dengan koma." />
            <TextField label="Catatan" size="small" multiline rows={2} value={form.notes || ''} onChange={e => setForm({...form, notes: e.target.value})}
              placeholder="Contoh: suka produk A, follow up bulan depan." />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={closeDialog}>Batal</Button>
          <Button variant="contained" onClick={save} disabled={saveCrmContact.isPending}>Simpan</Button>
        </DialogActions>
      </Dialog>

      <ContactImportDialog agentId={agentId} open={importOpen} onClose={() => setImportOpen(false)} />
    </Box>
  );
}

function lastChatLabel(d: string | undefined | null): string {
  if (!d) return '';
  const now = Date.now();
  const then = new Date(d).getTime();
  const diff = now - then;
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'Baru saja';
  if (mins < 60) return `${mins} menit lalu`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours} jam lalu`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days} hari lalu`;
  return new Date(d).toLocaleDateString('id-ID');
}
