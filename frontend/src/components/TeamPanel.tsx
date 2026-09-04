import { useState } from 'react';
import {
  Box, Button, Card, CardContent, Chip, CircularProgress,
  Dialog, DialogActions, DialogContent, DialogTitle,
  FormControl, FormControlLabel, IconButton, InputLabel,
  MenuItem, OutlinedInput, Select, Stack, Switch,
  Table, TableBody, TableCell, TableContainer, TableHead,
  TableRow, TextField, Tooltip, Typography, Paper, Alert,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import PersonOffIcon from '@mui/icons-material/PersonOff';
import HistoryIcon from '@mui/icons-material/History';

import {
  useTeamUsers, useCreateTeamUser, useUpdateTeamUser, useDeleteTeamUser, useCSActivity,
} from '../hooks';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';
import { swalConfirm, swalToast } from '../services/swal';
import type { Agent, TeamUser, CreateTeamUserRequest, UpdateTeamUserRequest } from '../types';

interface Props {
  agents: Agent[];
}

// --- Dialog Buat/Edit CS User ---
interface UserFormDialogProps {
  open: boolean;
  onClose: () => void;
  editing: TeamUser | null;
  agents: Agent[];
}

function UserFormDialog({ open, onClose, editing, agents }: UserFormDialogProps) {
  const createUser = useCreateTeamUser();
  const updateUser = useUpdateTeamUser();

  const [username, setUsername] = useState(editing?.username ?? '');
  const [name, setName] = useState(editing?.name ?? '');
  const [password, setPassword] = useState('');
  const [phone, setPhone] = useState(editing?.phone ?? '');
  const [agentIds, setAgentIds] = useState<number[]>(editing?.agent_ids ?? []);
  const [active, setActive] = useState(editing?.active ?? true);
  const [error, setError] = useState('');

  const isEdit = !!editing;

  const resetForm = () => {
    setUsername('');
    setName('');
    setPassword('');
    setPhone('');
    setAgentIds([]);
    setActive(true);
    setError('');
  };

  const handleClose = () => {
    resetForm();
    onClose();
  };

  const handleSubmit = async () => {
    setError('');
    if (!isEdit && !username.trim()) {
      setError('Username wajib diisi');
      return;
    }
    if (!isEdit && !password.trim()) {
      setError('Password wajib diisi');
      return;
    }
    if (!isEdit && password.length < 8) {
      setError('Password minimal 8 karakter');
      return;
    }
    if (agentIds.length === 0) {
      setError('Pilih minimal satu nomor WA yang di-assign');
      return;
    }

    try {
      if (isEdit) {
        const data: UpdateTeamUserRequest = {
          name: name || undefined,
          phone: phone || undefined,
          active,
          agent_ids: agentIds,
        };
        if (password) data.password = password;
        await updateUser.mutateAsync({ id: editing.id, data });
        swalToast('Akun CS berhasil diperbarui', 'success');
      } else {
        const data: CreateTeamUserRequest = {
          username: username.trim(),
          password,
          name: name.trim(),
          phone: phone.trim(),
          agent_ids: agentIds,
        };
        await createUser.mutateAsync(data);
        swalToast('Akun CS berhasil dibuat', 'success');
      }
      handleClose();
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error;
      setError(msg || 'Terjadi kesalahan');
    }
  };

  const isLoading = createUser.isPending || updateUser.isPending;

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {isEdit ? `Edit Akun CS: ${editing.username}` : 'Buat Akun CS Baru'}
      </DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          {error && <Alert severity="error">{error}</Alert>}
          {!isEdit && (
            <TextField
              label="Username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              fullWidth
              autoComplete="off"
              disabled={isLoading}
            />
          )}
          <TextField
            label="Nama Lengkap"
            value={name}
            onChange={(e) => setName(e.target.value)}
            fullWidth
            disabled={isLoading}
          />
          <TextField
            label={isEdit ? 'Password Baru (kosongkan jika tidak diubah)' : 'Password'}
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            fullWidth
            autoComplete="new-password"
            disabled={isLoading}
          />
          <TextField
            label="Nomor HP (opsional)"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            fullWidth
            disabled={isLoading}
          />
          <FormControl fullWidth>
            <InputLabel>Nomor WA yang Di-assign *</InputLabel>
            <Select
              multiple
              value={agentIds}
              onChange={(e) => setAgentIds(e.target.value as number[])}
              input={<OutlinedInput label="Nomor WA yang Di-assign *" />}
              renderValue={(selected) =>
                (selected as number[])
                  .map((id) => agents.find((a) => a.id === id)?.name || `Agent ${id}`)
                  .join(', ')
              }
              disabled={isLoading}
            >
              {agents.map((a) => (
                <MenuItem key={a.id} value={a.id}>
                  {a.name} {a.number ? `(${a.number})` : ''}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          {isEdit && (
            <FormControlLabel
              control={
                <Switch
                  checked={active}
                  onChange={(e) => setActive(e.target.checked)}
                  disabled={isLoading}
                />
              }
              label="Akun Aktif"
            />
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={isLoading}>Batal</Button>
        <Button
          variant="contained"
          onClick={handleSubmit}
          disabled={isLoading}
          startIcon={isLoading ? <CircularProgress size={16} /> : undefined}
        >
          {isEdit ? 'Simpan Perubahan' : 'Buat Akun'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

// --- Panel Utama ---
export default function TeamPanel({ agents }: Props) {
  const { data: users, isLoading } = useTeamUsers();
  const { data: activityLogs, isLoading: activityLoading } = useCSActivity();
  const deleteUser = useDeleteTeamUser();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<TeamUser | null>(null);
  const [tab, setTab] = useState<'users' | 'activity'>('users');

  const handleEdit = (u: TeamUser) => {
    setEditing(u);
    setDialogOpen(true);
  };

  const handleCreate = () => {
    setEditing(null);
    setDialogOpen(true);
  };

  const handleDelete = async (u: TeamUser) => {
    const ok = await swalConfirm(
      `Hapus akun CS "${u.name || u.username}"?`,
      'Akun dan semua assignment-nya akan dihapus permanen.',
    );
    if (!ok) return;
    try {
      await deleteUser.mutateAsync(u.id);
      swalToast('Akun CS dihapus', 'success');
    } catch {
      swalToast('Gagal menghapus akun', 'error');
    }
  };

  const getAgentNames = (ids: number[]) =>
    ids.map((id) => agents.find((a) => a.id === id)?.name || `#${id}`).join(', ') || '—';

  const actionLabel: Record<string, string> = {
    login: 'Login',
    reply: 'Balas Pesan',
    handoff: 'Handoff',
    read: 'Mark Baca',
    close: 'Tutup Percakapan',
  };

  return (
    <Box>
      <PageHeader
        title="Manajemen Tim CS"
        subtitle="Kelola akun Customer Service dan monitor aktivitas mereka"
        action={
          <Stack direction="row" spacing={1}>
            <Button
              variant={tab === 'users' ? 'contained' : 'outlined'}
              size="small"
              onClick={() => setTab('users')}
            >
              Daftar CS
            </Button>
            <Button
              variant={tab === 'activity' ? 'contained' : 'outlined'}
              size="small"
              startIcon={<HistoryIcon />}
              onClick={() => setTab('activity')}
            >
              Log Aktivitas
            </Button>
            {tab === 'users' && (
              <Button variant="contained" startIcon={<AddIcon />} onClick={handleCreate}>
                Tambah CS
              </Button>
            )}
          </Stack>
        }
      />

      {tab === 'users' && (
        <>
          {isLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 6 }}>
              <CircularProgress />
            </Box>
          ) : !users?.length ? (
            <EmptyState
              icon={<PersonOffIcon sx={{ fontSize: 48 }} />}
              title="Belum ada akun CS"
              description='Buat akun CS pertama dengan tombol "Tambah CS" di atas'
            />
          ) : (
            <TableContainer component={Paper} variant="outlined">
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Username</TableCell>
                    <TableCell>Nama</TableCell>
                    <TableCell>Nomor WA yang Di-assign</TableCell>
                    <TableCell>Status</TableCell>
                    <TableCell align="right">Aksi</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {users.map((u) => (
                    <TableRow key={u.id} hover>
                      <TableCell>
                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                          {u.username}
                        </Typography>
                        {u.phone && (
                          <Typography variant="caption" color="text.secondary">
                            {u.phone}
                          </Typography>
                        )}
                      </TableCell>
                      <TableCell>{u.name || '—'}</TableCell>
                      <TableCell>
                        <Typography variant="body2" noWrap sx={{ maxWidth: 220 }}>
                          {getAgentNames(u.agent_ids)}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Chip
                          size="small"
                          label={u.active ? 'Aktif' : 'Nonaktif'}
                          color={u.active ? 'success' : 'default'}
                        />
                      </TableCell>
                      <TableCell align="right">
                        <Tooltip title="Edit">
                          <IconButton size="small" onClick={() => handleEdit(u)}>
                            <EditIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Hapus">
                          <IconButton
                            size="small"
                            color="error"
                            onClick={() => handleDelete(u)}
                          >
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}

          <Card variant="outlined" sx={{ mt: 3 }}>
            <CardContent>
              <Typography variant="subtitle2" gutterBottom>
                💡 Tentang Akun CS
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Akun CS hanya bisa mengakses nomor WA yang di-assign kepadanya. Mereka bisa
                membalas percakapan di inbox, tetapi tidak bisa mengubah pengaturan sistem, AI,
                atau mengelola akun lain.
              </Typography>
            </CardContent>
          </Card>
        </>
      )}

      {tab === 'activity' && (
        <>
          {activityLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 6 }}>
              <CircularProgress />
            </Box>
          ) : !activityLogs?.length ? (
            <EmptyState
              icon={<HistoryIcon sx={{ fontSize: 48 }} />}
              title="Belum ada log aktivitas"
              description="Log aktivitas CS akan muncul di sini setelah CS mulai bekerja"
            />
          ) : (
            <TableContainer component={Paper} variant="outlined">
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Waktu</TableCell>
                    <TableCell>CS</TableCell>
                    <TableCell>Aksi</TableCell>
                    <TableCell>Kontak</TableCell>
                    <TableCell>Nomor WA</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {activityLogs.map((log) => (
                    <TableRow key={log.id} hover>
                      <TableCell>
                        <Typography variant="caption" color="text.secondary">
                          {new Date(log.created_at).toLocaleString('id-ID', {
                            dateStyle: 'short',
                            timeStyle: 'short',
                          })}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" sx={{ fontWeight: 600 }}>
                          {log.user_name || `User #${log.user_id}`}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Chip
                          size="small"
                          label={actionLabel[log.action] || log.action}
                          variant="outlined"
                        />
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" noWrap>
                          {log.sender || '—'}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Typography variant="caption" color="text.secondary">
                          {agents.find((a) => a.id === log.agent_id)?.name || `#${log.agent_id}`}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </>
      )}

      <UserFormDialog
        open={dialogOpen}
        onClose={() => {
          setDialogOpen(false);
          setEditing(null);
        }}
        editing={editing}
        agents={agents}
      />
    </Box>
  );
}
