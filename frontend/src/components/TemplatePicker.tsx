import { useState } from 'react';
import { Button, Menu, MenuItem, ListItemText, Typography } from '@mui/material';
import TextSnippetIcon from '@mui/icons-material/TextSnippetOutlined';
import { useTemplates } from '../hooks';

interface Props {
  agentId: number;
  onPick: (body: string) => void;
  size?: 'small' | 'medium';
  variant?: 'text' | 'outlined' | 'contained';
  label?: string;
}

// TemplatePicker = tombol kecil untuk menyisipkan isi template ke composer pesan.
// Dipakai di Inbox, Broadcast, dan Jadwal.
export default function TemplatePicker({ agentId, onPick, size = 'small', variant = 'outlined', label = 'Template' }: Props) {
  const { data: templates } = useTemplates(agentId);
  const [anchor, setAnchor] = useState<null | HTMLElement>(null);

  const pick = (body: string) => { onPick(body); setAnchor(null); };

  return (
    <>
      <Button size={size} variant={variant} startIcon={<TextSnippetIcon fontSize="small" />}
        onClick={e => setAnchor(e.currentTarget)} sx={{ flexShrink: 0 }}>
        {label}
      </Button>
      <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)}
        slotProps={{ paper: { sx: { maxWidth: 360, maxHeight: 360 } } }}>
        {(!templates || templates.length === 0) ? (
          <MenuItem disabled>
            <Typography variant="body2" color="text.secondary">Belum ada template. Buat di menu Template.</Typography>
          </MenuItem>
        ) : templates.map(t => (
          <MenuItem key={t.id} onClick={() => pick(t.body)} sx={{ display: 'block', py: 1 }}>
            <ListItemText
              primary={<Typography sx={{ fontWeight: 600 }} noWrap>{t.title}</Typography>}
              secondary={<Typography variant="caption" color="text.secondary" sx={{ display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}>{t.body}</Typography>}
            />
          </MenuItem>
        ))}
      </Menu>
    </>
  );
}
