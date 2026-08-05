import { useState } from 'react';
import { TextField, Button, Stack, Typography, Menu, MenuItem, CircularProgress, Box, IconButton, InputAdornment, Chip } from '@mui/material';
import ForumIcon from '@mui/icons-material/ForumOutlined';
import ContactsIcon from '@mui/icons-material/ContactsOutlined';
import GroupsIcon from '@mui/icons-material/GroupsOutlined';
import LabelIcon from '@mui/icons-material/LabelOutlined';
import CloseIcon from '@mui/icons-material/Close';
import SearchIcon from '@mui/icons-material/Search';
import AutoFixHighIcon from '@mui/icons-material/AutoFixHighOutlined';
import { useChatContacts, useWAContacts, useGroups, useGroupMembers, useLabels, useLabelContacts, useCheckNumbers } from '../hooks';
import { normalizePhone } from '../types';
import type { WAGroup, LabelInfo } from '../types';

type Contact = { number: string; name: string };

const PREVIEW_CAP = 300; // batas baris yang dirender agar daftar besar tetap ringan

function parseRecipientLine(line: string): Contact {
  // Terima format bebas seperti "Jojo 62812...", "62812... Jojo",
  // "Jojo,62812...", serta nomor yang memakai spasi atau tanda hubung.
  const phoneMatch = line.match(/(?:\+?\d[\d\s().-]{6,}\d)/);
  if (phoneMatch?.index !== undefined) {
    const number = normalizePhone(phoneMatch[0]);
    const name = `${line.slice(0, phoneMatch.index)} ${line.slice(phoneMatch.index + phoneMatch[0].length)}`
      .replace(/[\t,;|]+/g, ' ')
      .replace(/\s+/g, ' ')
      .trim();
    if (number) return { number, name };
  }

  const parts = line.split(/[\t,]/).map(part => part.trim()).filter(Boolean);
  const numberIndex = parts.findIndex(part => /\d/.test(part));
  if (numberIndex < 0) return { number: '', name: '' };
  return {
    number: normalizePhone(parts[numberIndex]),
    name: parts.filter((_, index) => index !== numberIndex).join(' ').trim(),
  };
}

export default function RecipientField({ agentId, value, onChange, error }: {
  agentId: number; value: string; onChange: (v: string) => void; error?: string;
}) {
  const chatContacts = useChatContacts(agentId);
  const waContacts = useWAContacts(agentId);
  const groups = useGroups(agentId);
  const groupMembers = useGroupMembers(agentId);
  const labels = useLabels(agentId);
  const labelContacts = useLabelContacts(agentId);
  const checkNumbers = useCheckNumbers(agentId);

  const [note, setNote] = useState('');
  const [noteWarn, setNoteWarn] = useState(false);
  const [unregistered, setUnregistered] = useState<string[]>([]);
  const [groupList, setGroupList] = useState<WAGroup[]>([]);
  const [labelList, setLabelList] = useState<LabelInfo[]>([]);
  const [groupAnchor, setGroupAnchor] = useState<null | HTMLElement>(null);
  const [labelAnchor, setLabelAnchor] = useState<null | HTMLElement>(null);
  const [showPreview, setShowPreview] = useState(true);
  const [filter, setFilter] = useState('');

  // Parse isi kotak jadi daftar penerima (nomor dinormalkan), buang duplikat & baris tak valid.
  const rawLines = value.split('\n').map(l => l.trim()).filter(Boolean);
  const dedup = new Map<string, string>();
  let invalid = 0;
  for (const line of rawLines) {
    const { number, name } = parseRecipientLine(line);
    if (!number) { invalid++; continue; }
    if (!dedup.has(number)) dedup.set(number, name);
  }
  const recipients: Contact[] = Array.from(dedup.entries()).map(([number, name]) => ({ number, name }));
  const dupCount = rawLines.length - invalid - recipients.length;

  const f = filter.trim().toLowerCase();
  const filtered = f ? recipients.filter(r => r.name.toLowerCase().includes(f) || r.number.includes(f)) : recipients;
  const shown = filtered.slice(0, PREVIEW_CAP);
  const capped = filtered.length - shown.length;
  const unregSet = new Set(unregistered);

  const removeNumber = (num: string) => {
    const kept = rawLines.filter(line => parseRecipientLine(line).number !== num);
    onChange(kept.join('\n'));
  };
  const clearAll = () => { onChange(''); setNote(''); setFilter(''); setUnregistered([]); };

  // Cek ke server WhatsApp nomor mana yang benar terdaftar (kurangi gagal-kirim & risiko ban).
  const runCheck = async () => {
    if (recipients.length === 0) return;
    try {
      const res = await checkNumbers.mutateAsync(recipients.map(r => r.number));
      setUnregistered(res.not_registered);
      setNoteWarn(res.not_registered.length > 0);
      setNote(`${res.registered_count}/${res.total} nomor terdaftar di WhatsApp`
        + (res.not_registered.length > 0 ? ` · ${res.not_registered.length} tidak terdaftar` : ' · semua valid'));
    } catch {
      setNoteWarn(true);
      setNote('Gagal cek nomor (WhatsApp tersambung?).');
    }
  };
  const removeUnregistered = () => {
    const set = new Set(unregistered);
    onChange(rawLines.filter(line => !set.has(parseRecipientLine(line).number)).join('\n'));
    setUnregistered([]);
    setNoteWarn(false);
    setNote('Nomor tidak terdaftar dibuang.');
  };

  const formatRecipients = () => {
    if (recipients.length === 0) return;
    onChange(recipients.map(recipient => recipient.name
      ? `${recipient.number},${recipient.name}`
      : recipient.number).join('\n'));
    setNoteWarn(invalid > 0);
    setNote([
      `${recipients.length} nomor dirapikan`,
      dupCount > 0 ? `${dupCount} duplikat digabung` : '',
      invalid > 0 ? `${invalid} baris tidak valid dibuang` : '',
    ].filter(Boolean).join(' · ') + '.');
  };

  const merge = (list: Contact[], label: string, warm = false) => {
    const parsed = value.split('\n').map(l => l.trim()).filter(Boolean)
      .map(parseRecipientLine).filter(r => r.number);
    const map = new Map<string, string>();
    [...parsed, ...list.map(c => ({ number: normalizePhone(c.number), name: c.name || '' }))]
      .forEach(c => { if (c.number && !map.has(c.number)) map.set(c.number, c.name); });
    onChange(Array.from(map.entries()).map(([n, nm]) => (nm ? `${n},${nm}` : n)).join('\n'));
    setNoteWarn(!warm);
    setNote(warm
      ? `${list.length} kontak dari ${label} ditambahkan.`
      : `${list.length} kontak dari ${label} ditambahkan — belum tentu pernah berinteraksi, risiko pembatasan lebih tinggi.`);
  };

  const openGroups = async (e: React.MouseEvent<HTMLElement>) => {
    const target = e.currentTarget;
    try { const g = await groups.mutateAsync(); setGroupList(g); setGroupAnchor(target); }
    catch { setNote('Gagal ambil grup (WhatsApp tersambung?).'); }
  };
  const openLabels = async (e: React.MouseEvent<HTMLElement>) => {
    const target = e.currentTarget;
    try { const l = await labels.mutateAsync(); setLabelList(l); setLabelAnchor(target); }
    catch { setNote('Gagal ambil label.'); }
  };

  return (
    <Box>
      <TextField fullWidth multiline rows={4} value={value} onChange={e => onChange(e.target.value)}
        placeholder={'08123456789,Budi\n08987654321,Sinta'} error={!!error} helperText={error} sx={{ mb: 1 }}
        slotProps={{
          input: {
            endAdornment: (
              <InputAdornment position="end" sx={{ alignSelf: 'flex-start', mt: -0.25, mr: -0.75 }}>
                <Button size="small" variant="outlined" startIcon={<AutoFixHighIcon fontSize="small" />}
                  disabled={recipients.length === 0} onClick={formatRecipients}
                  sx={{
                    whiteSpace: 'nowrap', minHeight: 28, bgcolor: 'grey.100', borderColor: 'grey.300', color: 'text.primary',
                    '&:hover': { bgcolor: 'grey.200', borderColor: 'grey.400' },
                  }}>
                  Format otomatis
                </Button>
              </InputAdornment>
            ),
          },
        }} />
      <Box sx={{ p: 1, mb: 0.5, border: '1px solid', borderColor: 'divider', borderRadius: 1, bgcolor: 'action.hover' }}>
        <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1, mb: 0.75 }}>
          <Typography variant="caption" sx={{ fontWeight: 800, color: 'text.primary' }}>Ambil penerima dari</Typography>
          <Chip size="small" label="Pernah chat disarankan" color="success" variant="outlined" />
        </Stack>
        <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 0.75 }}>
          <Button size="small" variant="contained" color="success" disabled={chatContacts.isPending}
            startIcon={chatContacts.isPending ? <CircularProgress size={14} color="inherit" /> : <ForumIcon />}
            onClick={async () => merge(await chatContacts.mutateAsync(), 'pernah chat', true)} sx={{ minWidth: 0 }}>
            Pernah chat
          </Button>
          <Button size="small" variant="outlined" color="inherit" disabled={waContacts.isPending}
            startIcon={waContacts.isPending ? <CircularProgress size={14} /> : <ContactsIcon />}
            onClick={async () => merge(await waContacts.mutateAsync(), 'kontak WhatsApp')} sx={{ minWidth: 0, bgcolor: 'background.paper' }}>
            Kontak WA
          </Button>
          <Button size="small" variant="outlined" color="inherit" disabled={groups.isPending}
            startIcon={groups.isPending ? <CircularProgress size={14} /> : <GroupsIcon />}
            onClick={openGroups} sx={{ minWidth: 0, bgcolor: 'background.paper' }}>
            Grup WA
          </Button>
          <Button size="small" variant="outlined" color="inherit" disabled={labels.isPending}
            startIcon={labels.isPending ? <CircularProgress size={14} /> : <LabelIcon />}
            onClick={openLabels} sx={{ minWidth: 0, bgcolor: 'background.paper' }}>
            Label WA
          </Button>
        </Box>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.75 }}>
          Sumber lain mungkin berisi kontak yang belum pernah berinteraksi dengan nomor Anda.
        </Typography>
      </Box>
      {note && <Typography variant="caption" color={noteWarn ? 'warning.main' : 'success.main'} sx={{ display: 'block' }}>{note}</Typography>}

      {/* Pratinjau daftar target */}
      {recipients.length > 0 && (
        <Box sx={{ mt: 1 }}>
          <Stack direction="row" sx={{ alignItems: 'center', gap: 1, mb: 0.5, flexWrap: 'wrap' }}>
            <Typography variant="caption" sx={{ fontWeight: 700, color: 'success.main' }}>
              ✓ {recipients.length} nomor valid
            </Typography>
            {dupCount > 0 && <Typography variant="caption" color="text.secondary">· {dupCount} duplikat digabung</Typography>}
            {invalid > 0 && <Typography variant="caption" color="warning.main">· {invalid} baris tak valid</Typography>}
            <Box sx={{ flex: 1 }} />
            {checkNumbers.isPending ? (
              <Stack direction="row" sx={{ alignItems: 'center', gap: 0.5 }}>
                <CircularProgress size={12} /><Typography variant="caption" color="text.secondary">Cek…</Typography>
              </Stack>
            ) : (
              <Typography variant="caption" color="primary" sx={{ cursor: 'pointer', fontWeight: 600 }} onClick={runCheck}>Cek terdaftar WA</Typography>
            )}
            {unregistered.length > 0 && (
              <Typography variant="caption" color="warning.main" sx={{ cursor: 'pointer', fontWeight: 600 }} onClick={removeUnregistered}>
                Buang {unregistered.length} tak terdaftar
              </Typography>
            )}
            <Typography variant="caption" color="error" sx={{ cursor: 'pointer', fontWeight: 600 }} onClick={clearAll}>Kosongkan</Typography>
            <Typography variant="caption" color="primary" sx={{ cursor: 'pointer', fontWeight: 600 }}
              onClick={() => setShowPreview(v => !v)}>
              {showPreview ? 'Sembunyikan' : 'Tampilkan'}
            </Typography>
          </Stack>

          {showPreview && (
            <Box sx={{ border: '1px solid', borderColor: 'divider', borderRadius: 1, p: 0.5 }}>
              {recipients.length > 8 && (
                <TextField size="small" fullWidth placeholder="Cari nama atau nomor…" value={filter}
                  onChange={e => setFilter(e.target.value)} sx={{ mb: 0.5 }}
                  slotProps={{ input: { startAdornment: <InputAdornment position="start"><SearchIcon fontSize="small" /></InputAdornment> } }} />
              )}
              <Box sx={{ maxHeight: 200, overflowY: 'auto' }}>
                {shown.length === 0 ? (
                  <Typography variant="caption" color="text.secondary" sx={{ p: 1, display: 'block' }}>Tidak ada yang cocok.</Typography>
                ) : shown.map(r => (
                  <Stack key={r.number} direction="row" sx={{ alignItems: 'center', gap: 1, px: 1, py: 0.4, borderRadius: 0.5, '&:hover': { bgcolor: 'action.hover' } }}>
                    <Typography variant="body2" color={unregSet.has(r.number) ? 'warning.main' : 'text.primary'}
                      sx={{ flex: 1, minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {r.name ? <>{r.name} <Typography component="span" variant="caption" color="text.secondary">· {r.number}</Typography></> : r.number}
                      {unregSet.has(r.number) && <Typography component="span" variant="caption" color="warning.main"> · tidak terdaftar</Typography>}
                    </Typography>
                    <IconButton size="small" aria-label="Hapus nomor" onClick={() => removeNumber(r.number)} sx={{ p: 0.25 }}>
                      <CloseIcon sx={{ fontSize: 16 }} />
                    </IconButton>
                  </Stack>
                ))}
                {capped > 0 && (
                  <Typography variant="caption" color="text.secondary" sx={{ p: 1, display: 'block' }}>
                    … dan {capped} lainnya. Pakai kotak cari untuk mempersempit.
                  </Typography>
                )}
              </Box>
            </Box>
          )}
        </Box>
      )}

      <Menu anchorEl={groupAnchor} open={!!groupAnchor} onClose={() => setGroupAnchor(null)}>
        {groupList.length === 0 && <MenuItem disabled>Tidak ada grup</MenuItem>}
        {groupList.map(g => (
          <MenuItem key={g.jid} onClick={async () => { setGroupAnchor(null); merge(await groupMembers.mutateAsync(g.jid), `grup ${g.name}`); }}>
            {g.name || g.jid} · {g.participants} anggota
          </MenuItem>
        ))}
      </Menu>
      <Menu anchorEl={labelAnchor} open={!!labelAnchor} onClose={() => setLabelAnchor(null)}>
        {labelList.length === 0 && <MenuItem disabled>Tidak ada label (akun Business?)</MenuItem>}
        {labelList.map(l => (
          <MenuItem key={l.label_id} onClick={async () => { setLabelAnchor(null); merge(await labelContacts.mutateAsync(l.label_id), `label ${l.name}`); }}>
            {l.name} · {l.count} kontak
          </MenuItem>
        ))}
      </Menu>
    </Box>
  );
}
