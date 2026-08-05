import { useMemo, useState } from 'react';
import {
  Alert, Avatar, Box, Button, Chip, CircularProgress, Divider, IconButton,
  InputAdornment, Paper, Stack, Tab, Tabs, TextField, ToggleButton,
  ToggleButtonGroup, Tooltip, Typography,
} from '@mui/material';
import AdminIcon from '@mui/icons-material/AdminPanelSettingsOutlined';
import CloseIcon from '@mui/icons-material/Close';
import GroupsIcon from '@mui/icons-material/GroupsOutlined';
import HistoryIcon from '@mui/icons-material/HistoryOutlined';
import RefreshIcon from '@mui/icons-material/Refresh';
import SearchIcon from '@mui/icons-material/Search';
import ShieldIcon from '@mui/icons-material/ShieldOutlined';
import TuneIcon from '@mui/icons-material/Tune';
import EmptyState from './common/EmptyState';
import GroupGuardConfigDialog from './group-guard/GroupGuardConfigDialog';
import GroupModerationFeed from './group-guard/GroupModerationFeed';
import { LoadingState, SummaryGrid } from './group-guard/GroupGuardShared';
import PageHeader from './PageHeader';
import { useManagedGroups } from '../hooks';
import type { WAGroup } from '../types';

type GroupFilter = 'all' | 'active' | 'needs-admin';

export default function GroupGuardPanel({ agentId }: { agentId: number }) {
  const [tab, setTab] = useState(0);

  return (
    <Box>
      <PageHeader
        title="Anti-Spam Grup"
        subtitle="Atur perlindungan anti-spam dan tinjau tindakan untuk grup WhatsApp yang dikelola nomor ini."
      />

      <Tabs
        value={tab}
        onChange={(_, value) => setTab(value)}
        variant="scrollable"
        scrollButtons="auto"
        sx={{ mb: 2, borderBottom: '1px solid', borderColor: 'divider' }}
      >
        <Tab icon={<GroupsIcon fontSize="small" />} iconPosition="start" label="Grup" />
        <Tab icon={<HistoryIcon fontSize="small" />} iconPosition="start" label="Aktivitas" />
      </Tabs>

      {tab === 0 ? <GroupList agentId={agentId} /> : <GroupModerationFeed agentId={agentId} />}
    </Box>
  );
}

function GroupList({ agentId }: { agentId: number }) {
  const { data: groups = [], isLoading, isError, refetch, isFetching } = useManagedGroups(agentId);
  const [editing, setEditing] = useState<WAGroup | null>(null);
  const [query, setQuery] = useState('');
  const [filter, setFilter] = useState<GroupFilter>('all');

  const totals = useMemo(() => ({
    total: groups.length,
    active: groups.filter(group => group.guard_enabled).length,
    admin: groups.filter(group => group.bot_is_admin).length,
    needsAdmin: groups.filter(group => !group.bot_is_admin).length,
  }), [groups]);

  const visibleGroups = useMemo(() => {
    const keyword = query.trim().toLowerCase();
    return groups
      .filter(group => !keyword || group.name.toLowerCase().includes(keyword) || group.jid.toLowerCase().includes(keyword))
      .filter(group => filter === 'all' || (filter === 'active' ? group.guard_enabled : !group.bot_is_admin))
      .sort((a, b) => Number(b.guard_enabled) - Number(a.guard_enabled) || a.name.localeCompare(b.name, 'id'));
  }, [filter, groups, query]);

  if (isError) {
    return (
      <EmptyState
        icon={<GroupsIcon sx={{ fontSize: 48 }} />}
        title="Belum ada grup yang dapat dijaga"
        description="Sambungkan WhatsApp untuk memuat daftar grup, lalu aktifkan aturan anti-spam sesuai kebutuhan."
        actionLabel="Coba Lagi"
        onAction={() => refetch()}
      />
    );
  }

  return (
    <Box>
      <SummaryGrid items={[
        { label: 'Total grup', value: totals.total },
        { label: 'Anti-spam aktif', value: totals.active, tone: totals.active ? 'success.main' : 'text.secondary' },
        { label: 'Nomor ini admin', value: totals.admin, tone: totals.admin ? 'primary.main' : 'text.secondary' },
        { label: 'Perlu akses admin', value: totals.needsAdmin, tone: totals.needsAdmin ? 'warning.main' : 'text.secondary' },
      ]} />

      {totals.needsAdmin > 0 && (
        <Alert severity="warning" icon={<AdminIcon fontSize="small" />} sx={{ mb: 1.5 }}>
          <Typography variant="body2">
            <b>{totals.needsAdmin} grup belum memberi nomor ini akses admin.</b> Aturan bisa disiapkan, tetapi nomor ini belum dapat menghapus pesan atau mengeluarkan anggota di grup tersebut.
          </Typography>
        </Alert>
      )}

      <Paper variant="outlined" sx={{ mb: 1.5, p: 1 }}>
        <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} sx={{ alignItems: { xs: 'stretch', md: 'center' } }}>
          <TextField
            size="small"
            value={query}
            onChange={event => setQuery(event.target.value)}
            placeholder="Cari nama grup..."
            sx={{ flex: 1 }}
            slotProps={{
              input: {
                startAdornment: <InputAdornment position="start"><SearchIcon fontSize="small" /></InputAdornment>,
                endAdornment: query ? (
                  <InputAdornment position="end">
                    <IconButton size="small" aria-label="Hapus pencarian" onClick={() => setQuery('')}>
                      <CloseIcon fontSize="small" />
                    </IconButton>
                  </InputAdornment>
                ) : undefined,
              },
            }}
          />
          <ToggleButtonGroup
            size="small"
            exclusive
            value={filter}
            onChange={(_, value: GroupFilter | null) => value && setFilter(value)}
            aria-label="Filter grup"
            sx={{ alignSelf: { xs: 'stretch', md: 'center' }, '& .MuiToggleButton-root': { flex: { xs: 1, md: 'initial' } } }}
          >
            <ToggleButton value="all">Semua</ToggleButton>
            <ToggleButton value="active">Aktif</ToggleButton>
            <ToggleButton value="needs-admin">Perlu admin</ToggleButton>
          </ToggleButtonGroup>
          <Tooltip title="Segarkan daftar grup">
            <span>
              <IconButton aria-label="Segarkan daftar grup" onClick={() => refetch()} disabled={isFetching}>
                {isFetching ? <CircularProgress size={20} /> : <RefreshIcon />}
              </IconButton>
            </span>
          </Tooltip>
        </Stack>
      </Paper>

      {isLoading && <LoadingState label="Memuat grup..." />}
      {!isLoading && groups.length === 0 && (
        <EmptyState
          icon={<GroupsIcon sx={{ fontSize: 44 }} />}
          title="Belum ada grup"
          description="Grup WhatsApp yang diikuti nomor ini akan muncul otomatis di sini."
          actionLabel="Segarkan"
          onAction={() => refetch()}
        />
      )}
      {!isLoading && groups.length > 0 && visibleGroups.length === 0 && (
        <EmptyState
          icon={<SearchIcon sx={{ fontSize: 42 }} />}
          title="Grup tidak ditemukan"
          description="Coba ubah kata pencarian atau pilih filter lain."
          actionLabel="Reset filter"
          onAction={() => { setQuery(''); setFilter('all'); }}
        />
      )}

      {!isLoading && visibleGroups.length > 0 && (
        <Paper variant="outlined" sx={{ overflow: 'hidden' }}>
          {visibleGroups.map((group, index) => (
            <Box key={group.jid}>
              {index > 0 && <Divider />}
              <Stack
                direction="row"
                spacing={1.25}
                sx={{ alignItems: 'center', px: { xs: 1, sm: 1.5 }, py: 1.15, '&:hover': { bgcolor: 'action.hover' } }}
              >
                <Avatar sx={{ width: 34, height: 34, bgcolor: group.guard_enabled ? 'success.light' : 'action.selected', color: group.guard_enabled ? 'success.dark' : 'text.secondary' }}>
                  {group.guard_enabled ? <ShieldIcon fontSize="small" /> : <GroupsIcon fontSize="small" />}
                </Avatar>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center', minWidth: 0 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 750, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {group.name || group.jid}
                    </Typography>
                    <Chip
                      size="small"
                      color={group.guard_enabled ? 'success' : 'default'}
                      variant={group.guard_enabled ? 'filled' : 'outlined'}
                      label={group.guard_enabled ? 'Aktif' : 'Nonaktif'}
                    />
                  </Stack>
                  <Typography variant="caption" color={group.bot_is_admin ? 'text.secondary' : 'warning.main'} sx={{ display: 'block', mt: 0.15 }}>
                    {group.participants} anggota · {group.bot_is_admin ? 'Nomor ini admin' : 'Nomor ini belum admin'}
                  </Typography>
                </Box>
                <Button size="small" variant={group.guard_enabled ? 'outlined' : 'contained'} startIcon={<TuneIcon />} onClick={() => setEditing(group)}>
                  {group.guard_enabled ? 'Atur' : 'Siapkan'}
                </Button>
              </Stack>
            </Box>
          ))}
        </Paper>
      )}

      {editing && <GroupGuardConfigDialog agentId={agentId} group={editing} onClose={() => setEditing(null)} />}
    </Box>
  );
}
