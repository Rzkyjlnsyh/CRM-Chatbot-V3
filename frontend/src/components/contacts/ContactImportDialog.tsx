import { useState, useMemo, useRef } from 'react';
import {
  Box, Stack, Typography, Button, Dialog, DialogTitle, DialogContent, DialogActions,
  TextField, ToggleButtonGroup, ToggleButton, Alert, Chip, CircularProgress, MenuItem,
} from '@mui/material';
import UploadFileIcon from '@mui/icons-material/UploadFileOutlined';
import {
  useWAContacts, useGroups, useGroupMembers, useLabels, useLabelContacts, useImportCrmContacts,
} from '../../hooks';
import type { WAGroup, LabelInfo } from '../../types';
import { normalizePhone } from '../../types';
import { swalToast } from '../../services/swal';

type Row = { number: string; name: string };
type Source = 'manual' | 'connected' | 'csv';
type ConnectedSource = 'address' | 'group' | 'label';

// dedupe membersihkan & menyatukan baris: normalisasi nomor, buang yang tak valid & dobel.
function dedupe(rows: Row[]): Row[] {
  const seen = new Set<string>();
  const out: Row[] = [];
  for (const r of rows) {
    const num = normalizePhone(r.number || '');
    if (!num || seen.has(num)) continue;
    seen.add(num);
    out.push({ number: num, name: (r.name || '').trim() });
  }
  return out;
}

// parseLines mengubah teks (manual / CSV) jadi baris kontak. Format per baris:
// "nomor", "nomor,nama", atau dipisah titik koma / tab. Baris header (tanpa angka) terlewati sendiri.
function parseLines(text: string): Row[] {
  const rows: Row[] = [];
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line) continue;
    const parts = line.split(/[,;\t]/).map(s => s.trim());
    rows.push({ number: parts[0] || '', name: parts[1] || '' });
  }
  return dedupe(rows);
}

const CONNECTED_LABELS: Record<ConnectedSource, string> = {
  address: 'Buku alamat WhatsApp',
  group: 'Anggota grup',
  label: 'Kontak berlabel',
};

export default function ContactImportDialog({ agentId, open, onClose }: {
  agentId: number;
  open: boolean;
  onClose: () => void;
}) {
  const [source, setSource] = useState<Source>('manual');
  const [manualText, setManualText] = useState('');
  const [csvRows, setCsvRows] = useState<Row[]>([]);
  const [csvName, setCsvName] = useState('');
  const [connectedRows, setConnectedRows] = useState<Row[]>([]);
  const [tag, setTag] = useState('');
  const [tagEdited, setTagEdited] = useState(false); // true kalau user mengetik tag sendiri

  const [connSource, setConnSource] = useState<ConnectedSource>('address');
  const [groups, setGroups] = useState<WAGroup[]>([]);
  const [groupJid, setGroupJid] = useState('');
  const [labels, setLabels] = useState<LabelInfo[]>([]);
  const [labelId, setLabelId] = useState('');
  const [labelsLoaded, setLabelsLoaded] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const waContacts = useWAContacts(agentId);
  const groupsList = useGroups(agentId);
  const groupMembers = useGroupMembers(agentId);
  const labelsList = useLabels(agentId);
  const labelContacts = useLabelContacts(agentId);
  const importContacts = useImportCrmContacts(agentId);

  const rows = useMemo<Row[]>(() => {
    if (source === 'manual') return parseLines(manualText);
    if (source === 'csv') return csvRows;
    return connectedRows;
  }, [source, manualText, csvRows, connectedRows]);

  const loadingConnected = waContacts.isPending || groupMembers.isPending || labelsList.isPending || labelContacts.isPending;

  const reset = () => {
    setSource('manual'); setManualText(''); setCsvRows([]); setCsvName('');
    setConnectedRows([]); setTag(''); setTagEdited(false); setConnSource('address');
    setGroups([]); setGroupJid(''); setLabels([]); setLabelId(''); setLabelsLoaded(false);
  };

  const close = () => { reset(); onClose(); };

  // autoTag mengisi tag bawaan dari sumber terkoneksi, tapi tidak menimpa tag
  // yang sudah diketik user. Tujuannya: kontak hasil impor dari nomor terkoneksi
  // langsung terlabeli (mis. "grup Reseller", "pernah chat") biar mudah dibedakan.
  const autoTag = (t: string) => { if (!tagEdited) setTag(t); };

  const onCsvFile = async (file?: File) => {
    if (!file) return;
    setCsvName(file.name);
    try {
      const text = await file.text();
      setCsvRows(parseLines(text));
    } catch {
      swalToast('File tidak bisa dibaca', 'error');
    }
  };

  const pickConnSource = async (next: ConnectedSource) => {
    setConnSource(next);
    setConnectedRows([]);
    setGroupJid(''); setLabelId('');
    // Buku alamat sudah punya tag pasti; grup/label menyusul setelah dipilih.
    if (next === 'address') autoTag('buku alamat');
    else autoTag('');
    try {
      if (next === 'address') setConnectedRows(dedupe(await waContacts.mutateAsync()));
      else if (next === 'group') setGroups(await groupsList.mutateAsync());
      else if (next === 'label') {
        setLabels(await labelsList.mutateAsync());
        setLabelsLoaded(true);
      }
    } catch {
      swalToast(next === 'label'
        ? 'Label belum bisa disinkronkan. Pastikan nomor WhatsApp tersambung.'
        : 'Sumber kontak belum bisa dimuat', 'error');
    }
  };

  const loadGroup = async (jid: string) => {
    setGroupJid(jid);
    if (!jid) { setConnectedRows([]); autoTag(''); return; }
    const name = groups.find(g => g.jid === jid)?.name || 'grup';
    autoTag(`grup ${name}`);
    try { setConnectedRows(dedupe(await groupMembers.mutateAsync(jid))); }
    catch { swalToast('Anggota grup belum bisa dimuat', 'error'); }
  };

  const loadLabel = async (lid: string) => {
    setLabelId(lid);
    if (!lid) { setConnectedRows([]); autoTag(''); return; }
    const name = labels.find(l => l.label_id === lid)?.name || 'label';
    autoTag(`label ${name}`);
    try { setConnectedRows(dedupe(await labelContacts.mutateAsync(lid))); }
    catch { swalToast('Kontak berlabel belum bisa dimuat', 'error'); }
  };

  const doImport = async () => {
    if (rows.length === 0) return;
    try {
      const res = await importContacts.mutateAsync({ contacts: rows, tag: tag.trim() || undefined });
      const skipMsg = res.skipped ? `, ${res.skipped} dilewati (sudah ada)` : '';
      swalToast(`${res.imported} kontak diimpor${skipMsg}`);
      close();
    } catch {
      swalToast('Kontak belum bisa diimpor', 'error');
    }
  };

  return (
    <Dialog open={open} onClose={close} maxWidth="sm" fullWidth>
      <DialogTitle>Impor Kontak</DialogTitle>
      <DialogContent>
        <Stack spacing={1.75} sx={{ mt: 1 }}>
          <ToggleButtonGroup
            size="small" exclusive fullWidth value={source}
            onChange={(_e, v) => {
              if (!v) return;
              setSource(v);
              if (v === 'connected') {
                if (connectedRows.length === 0) pickConnSource(connSource);
              } else {
                autoTag(''); // manual/CSV: kosongkan label bawaan (kecuali user sudah isi sendiri)
              }
            }}
          >
            <ToggleButton value="manual">Tempel manual</ToggleButton>
            <ToggleButton value="connected">Nomor terkoneksi</ToggleButton>
            <ToggleButton value="csv">Unggah CSV</ToggleButton>
          </ToggleButtonGroup>

          {source === 'manual' && (
            <TextField
              label="Daftar nomor" multiline minRows={5} size="small" value={manualText}
              onChange={e => setManualText(e.target.value)}
              placeholder={'628123456789, Budi\n6281122334455, Sinta\n0857...'}
              helperText="Satu nomor per baris. Tambahkan nama setelah koma (opsional)."
            />
          )}

          {source === 'csv' && (
            <Box>
              <input
                ref={fileRef} type="file" accept=".csv,text/csv,text/plain" hidden
                onChange={e => onCsvFile(e.target.files?.[0])}
              />
              <Button variant="outlined" startIcon={<UploadFileIcon />} onClick={() => fileRef.current?.click()}>
                {csvName || 'Pilih file CSV'}
              </Button>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.75 }}>
                Kolom: <b>nomor,nama</b> (baris pertama boleh judul kolom, akan dilewati otomatis).
              </Typography>
            </Box>
          )}

          {source === 'connected' && (
            <Stack spacing={1.25}>
              <TextField
                select size="small" label="Sumber" value={connSource}
                onChange={e => pickConnSource(e.target.value as ConnectedSource)}
              >
                {(Object.keys(CONNECTED_LABELS) as ConnectedSource[]).map(k => (
                  <MenuItem key={k} value={k}>{CONNECTED_LABELS[k]}</MenuItem>
                ))}
              </TextField>

              {connSource === 'group' && (
                <TextField
                  select size="small" label="Pilih grup" value={groupJid}
                  onChange={e => loadGroup(e.target.value)}
                  disabled={groupsList.isPending}
                  helperText={groupsList.isPending ? 'Memuat daftar grup...' : ''}
                >
                  <MenuItem value="">— pilih grup —</MenuItem>
                  {groups.map(g => (
                    <MenuItem key={g.jid} value={g.jid}>{g.name} ({g.participants})</MenuItem>
                  ))}
                </TextField>
              )}

              {connSource === 'label' && (
                <Stack spacing={0.25}>
                  <TextField
                    select size="small" label="Pilih label" value={labelId}
                    onChange={e => loadLabel(e.target.value)}
                    disabled={labelsList.isPending}
                    helperText={labelsList.isPending ? 'Menyinkronkan label dari WhatsApp...' : 'Daftar diperbarui langsung dari WhatsApp.'}
                  >
                    <MenuItem value="">— pilih label —</MenuItem>
                    {labels.map(l => (
                      <MenuItem key={l.label_id} value={l.label_id}>{l.name} ({l.count})</MenuItem>
                    ))}
                  </TextField>
                  <Button size="small" variant="text" sx={{ alignSelf: 'flex-start' }} disabled={labelsList.isPending}
                    onClick={() => pickConnSource('label')}>
                    Sinkronkan ulang label
                  </Button>
                </Stack>
              )}

              {connSource === 'label' && labelsLoaded && !labelsList.isPending && labels.length === 0 && (
                <Alert severity="info" icon={false}>
                  Tidak ada label yang ditemukan. Buat label di WhatsApp Business, pasang ke kontak, lalu buka sumber ini kembali.
                </Alert>
              )}

              {connSource === 'address' && (
                <Alert severity="warning" icon={false}>
                  Buku alamat berisi semua kontak tersimpan di HP — banyak yang belum tentu pernah berinteraksi.
                  Lebih aman pakai "Yang pernah chat" untuk broadcast.
                </Alert>
              )}

              {loadingConnected && (
                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                  <CircularProgress size={16} />
                  <Typography variant="caption" color="text.secondary">Memuat kontak...</Typography>
                </Stack>
              )}
            </Stack>
          )}

          <TextField
            label="Tag (opsional)" size="small" value={tag}
            onChange={e => { setTag(e.target.value); setTagEdited(true); }}
            placeholder="impor juni, dari grup reseller"
            helperText={source === 'connected'
              ? 'Diisi otomatis sesuai sumber nomor terkoneksi (mis. "grup ...") biar kontak hasil impor mudah dikenali. Boleh diubah.'
              : 'Tag ini dipasang ke semua kontak baru hasil impor ini.'}
          />

          {rows.length > 0 ? (
            <Alert severity="success" icon={false}>
              <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
                <Chip size="small" color="success" label={`${rows.length} nomor siap diimpor`} />
                <Typography variant="caption" color="text.secondary">
                  Nomor yang sudah ada di kontak akan dilewati otomatis.
                </Typography>
              </Stack>
            </Alert>
          ) : (
            <Typography variant="caption" color="text.secondary">
              Belum ada nomor valid yang terbaca.
            </Typography>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={close}>Batal</Button>
        <Button variant="contained" onClick={doImport} disabled={rows.length === 0 || importContacts.isPending}>
          {importContacts.isPending ? 'Mengimpor...' : `Impor ${rows.length || ''}`.trim()}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
