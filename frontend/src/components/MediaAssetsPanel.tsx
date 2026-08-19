import { useRef, useState } from 'react';
import {
  Box, Button, Card, CardContent, CardMedia, Chip, CircularProgress,
  Dialog, DialogActions, DialogContent, DialogTitle, Grid, IconButton,
  Paper, Stack, TextField, Tooltip, Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import ImageOutlinedIcon from '@mui/icons-material/ImageOutlined';
import VideoLibraryOutlinedIcon from '@mui/icons-material/VideoLibraryOutlined';
import DescriptionOutlinedIcon from '@mui/icons-material/DescriptionOutlined';
import { useMediaAssets, useUploadMediaAsset, useDeleteMediaAsset } from '../hooks';
import { swalConfirm, swalToast } from '../services/swal';
import PageHeader from './PageHeader';
import EmptyState from './common/EmptyState';

// Media assets — file media yang bisa dikirim AI otomatis lewat directive
// [[SEND_MEDIA:label]] di balasan. Label & trigger keys dipakai AI untuk
// memilih media yang tepat sesuai konteks percakapan.
export default function MediaAssetsPanel({ agentId }: { agentId: number }) {
  const { data: assets, isLoading } = useMediaAssets(agentId);
  const upload = useUploadMediaAsset(agentId);
  const remove = useDeleteMediaAsset(agentId);

  const [open, setOpen] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState('');
  const [name, setName] = useState('');
  const [label, setLabel] = useState('');
  const [triggerKeys, setTriggerKeys] = useState('');
  const [caption, setCaption] = useState('');
  const [sortOrder, setSortOrder] = useState(0);
  const fileRef = useRef<HTMLInputElement>(null);

  const resetForm = () => {
    setFile(null);
    setPreview('');
    setName('');
    setLabel('');
    setTriggerKeys('');
    setCaption('');
    setSortOrder(0);
  };

  const handleFile = (f: File | null) => {
    setFile(f);
    if (f && f.type.startsWith('image/')) {
      const reader = new FileReader();
      reader.onload = () => setPreview(String(reader.result));
      reader.readAsDataURL(f);
    } else {
      setPreview('');
    }
  };

  const handleUpload = () => {
    if (!file) {
      swalToast('Pilih file dulu', 'error');
      return;
    }
    if (!label.trim()) {
      swalToast('Label wajib diisi (dipakai AI untuk memilih media)', 'error');
      return;
    }
    const fd = new FormData();
    fd.append('file', file);
    fd.append('name', name.trim());
    fd.append('label', label.trim());
    fd.append('trigger_keys', triggerKeys.trim());
    fd.append('caption', caption.trim());
    fd.append('sort_order', String(sortOrder));
    upload.mutate(fd, {
      onSuccess: () => {
        swalToast('Media berhasil diunggah');
        setOpen(false);
        resetForm();
      },
      onError: (err: any) => swalToast(err?.response?.data?.error || 'Gagal mengunggah media', 'error'),
    });
  };

  const handleDelete = (asset: { id: number; name: string }) => {
    swalConfirm('Hapus media ini?', `"${asset.name || asset.id}" tidak akan bisa dikirim AI lagi.`)
      .then((ok: boolean) => {
        if (!ok) return;
        remove.mutate(asset.id, {
          onSuccess: () => swalToast('Media dihapus'),
          onError: (err: any) => swalToast(err?.response?.data?.error || 'Gagal menghapus', 'error'),
        });
      });
  };

  const mediaIcon = (type: string) => {
    if (type === 'image') return <ImageOutlinedIcon />;
    if (type === 'video') return <VideoLibraryOutlinedIcon />;
    return <DescriptionOutlinedIcon />;
  };

  const fmtSize = (n: number) => {
    if (!n) return '—';
    if (n > 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`;
    return `${Math.max(1, Math.round(n / 1024))} KB`;
  };

  return (
    <Box>
      <PageHeader
        title="Media"
        subtitle="File media yang bisa dikirim AI otomatis lewat directive [[SEND_MEDIA:label]]"
      />

      <Paper variant="outlined" sx={{ p: 2, mb: 2, bgcolor: 'rgba(25,118,210,0.04)' }}>
        <Typography variant="body2" color="text.secondary" sx={{ lineHeight: 1.6 }}>
          <strong>Cara kerja:</strong> unggah media + beri label (mis. <em>katalog dtf</em>).
          Arahkan persona agent untuk memakai directive <code>[[SEND_MEDIA:katalog dtf]]</code> —
          AI akan mengirim media itu beserta teksnya sendiri yang sesuai konteks.
          Trigger keys membantu AI memilih media (mis. <em>katalog, dtf, gambar, contoh</em>).
        </Typography>
      </Paper>

      <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="body2" color="text.secondary">
          {assets?.length || 0} media tersimpan
        </Typography>
        <Button variant="contained" startIcon={<AddIcon />} onClick={() => setOpen(true)}>
          Unggah Media
        </Button>
      </Stack>

      {isLoading ? (
        <Box sx={{ p: 4, textAlign: 'center' }}><CircularProgress /></Box>
      ) : !assets || assets.length === 0 ? (
        <EmptyState
          icon={<ImageOutlinedIcon sx={{ fontSize: 48 }} />}
          title="Belum ada media"
          description="Unggah katalog, foto produk, atau video agar AI bisa mengirimnya ke customer."
        />
      ) : (
        <Grid container spacing={2}>
          {assets.map((a) => (
            <Grid key={a.id} size={{ xs: 12, sm: 6, md: 4 }}>
              <Card variant="outlined" sx={{ height: '100%' }}>
                {a.media_type === 'image' ? (
                  <CardMedia
                    component="img"
                    height={140}
                    image={`/api/agents/${agentId}/media-assets/${a.id}/file?token=${localStorage.getItem('token') || ''}`}
                    alt={a.name || a.file_name}
                    sx={{ objectFit: 'cover', bgcolor: '#f0f2f5' }}
                  />
                ) : (
                  <Box sx={{ height: 140, display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'text.disabled', bgcolor: '#f0f2f5' }}>
                    {mediaIcon(a.media_type)}
                    <Typography variant="caption" sx={{ ml: 1 }}>{a.media_type}</Typography>
                  </Box>
                )}
                <CardContent sx={{ pt: 1.5 }}>
                  <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'flex-start' }}>
                    <Box sx={{ minWidth: 0 }}>
                      <Typography variant="subtitle2" noWrap>{a.name || a.file_name}</Typography>
                      <Typography variant="caption" color="text.secondary">
                        {a.file_name} · {fmtSize(a.file_size)}
                      </Typography>
                    </Box>
                    <Tooltip title="Hapus media">
                      <IconButton size="small" color="error" onClick={() => handleDelete(a)}>
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </Tooltip>
                  </Stack>
                  <Stack direction="row" spacing={1} sx={{ mt: 1, flexWrap: 'wrap', gap: 0.5 }}>
                    {a.label && <Chip size="small" label={`label: ${a.label}`} color="primary" variant="outlined" />}
                    {a.trigger_keys && (
                      <Chip size="small" label={`trigger: ${a.trigger_keys}`} variant="outlined" />
                    )}
                  </Stack>
                  {a.caption && (
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                      {a.caption}
                    </Typography>
                  )}
                </CardContent>
              </Card>
            </Grid>
          ))}
        </Grid>
      )}

      <Dialog open={open} onClose={() => setOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Unggah Media</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <input
              ref={fileRef}
              type="file"
              accept="image/*,video/*,.pdf,.doc,.docx,.xls,.xlsx"
              style={{ display: 'none' }}
              onChange={(e) => handleFile(e.target.files?.[0] || null)}
            />
            <Box
              onClick={() => fileRef.current?.click()}
              sx={{
                border: '1px dashed #b0b8c4', borderRadius: 2, p: 2, textAlign: 'center',
                cursor: 'pointer', bgcolor: '#fafbfc',
              }}
            >
              {preview ? (
                <img src={preview} alt="preview" style={{ maxHeight: 160, maxWidth: '100%', borderRadius: 8 }} />
              ) : (
                <Typography color="text.secondary" variant="body2">
                  {file ? file.name : 'Klik untuk pilih file (gambar, video, atau dokumen)'}
                </Typography>
              )}
            </Box>
            <TextField label="Label (wajib)" size="small" fullWidth
              value={label} onChange={(e) => setLabel(e.target.value)}
              placeholder="contoh: katalog dtf"
              helperText="Label inilah yang dipakai AI di directive [[SEND_MEDIA:...]]" />
            <TextField label="Nama" size="small" fullWidth
              value={name} onChange={(e) => setName(e.target.value)}
              placeholder="contoh: Katalog DTF Premium" />
            <TextField label="Trigger keys (pisah koma)" size="small" fullWidth
              value={triggerKeys} onChange={(e) => setTriggerKeys(e.target.value)}
              placeholder="contoh: katalog, dtf, gambar, contoh" />
            <TextField label="Caption default (opsional)" size="small" fullWidth multiline minRows={2}
              value={caption} onChange={(e) => setCaption(e.target.value)}
              placeholder="Dipakai kalau AI tidak menulis teks sendiri" />
            <TextField label="Urutan (semakin kecil semakin dulu)" type="number" size="small" fullWidth
              value={sortOrder} onChange={(e) => setSortOrder(Number(e.target.value))} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Batal</Button>
          <Button variant="contained" onClick={handleUpload} disabled={upload.isPending}>
            {upload.isPending ? 'Mengunggah...' : 'Unggah'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}
