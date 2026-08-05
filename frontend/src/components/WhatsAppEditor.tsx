import { useState, useRef, useCallback } from 'react';
import { Box, ToggleButton, ToggleButtonGroup, TextField, Typography } from '@mui/material';
import FormatBoldIcon from '@mui/icons-material/FormatBold';
import FormatItalicIcon from '@mui/icons-material/FormatItalic';
import StrikethroughSIcon from '@mui/icons-material/StrikethroughS';
import CodeIcon from '@mui/icons-material/Code';

const FORMATS = [
  { key: 'bold', icon: <FormatBoldIcon fontSize="small" />, label: 'Bold', wrapper: '*' },
  { key: 'italic', icon: <FormatItalicIcon fontSize="small" />, label: 'Italic', wrapper: '_' },
  { key: 'strike', icon: <StrikethroughSIcon fontSize="small" />, label: 'Coret', wrapper: '~' },
  { key: 'mono', icon: <CodeIcon fontSize="small" />, label: 'Monospace', wrapper: '```' },
];

interface Props {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  rows?: number;
  error?: boolean;
  helperText?: string;
}

export default function WhatsAppEditor({ value, onChange, placeholder, rows = 4, error, helperText }: Props) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [preview, setPreview] = useState(false);

  const updateCursor = (el: HTMLTextAreaElement, selStart: number, selEnd: number) => {
    // Pastikan fokus dulu, lalu set selection range.
    // Pakai requestAnimationFrame hanya untuk memastikan browser sudah selesai layout.
    el.focus();
    requestAnimationFrame(() => el.setSelectionRange(selStart, selEnd));
  };

  const insertFormat = useCallback((wrapper: string) => {
    const el = textareaRef.current;
    if (!el) return;
    const start = el.selectionStart;
    const end = el.selectionEnd;
    const wlen = wrapper.length;
    const hasSelection = start !== end;

    // Cek apakah teks yang dipilih / sekitar kursor sudah dibungkus wrapper.
    // Kalau iya → UNTOGGLE (hapus wrapper), kalau tidak → tambahkan.
    const before = value.substring(start - wlen, start);
    const after = value.substring(end, end + wlen);
    const selected = value.substring(start, end);
    const alreadyWrapped = before === wrapper && after === wrapper;

    let newText: string;
    let newStart: number;
    let newEnd: number;

    if (alreadyWrapped) {
      // Hapus wrapper di kiri & kanan
      newText = value.substring(0, start - wlen) + selected + value.substring(end + wlen);
      newStart = start - wlen;
      newEnd = end - wlen;
    } else {
      // Tambahkan wrapper
      const inner = hasSelection ? selected : 'teks';
      newText = value.substring(0, start) + wrapper + inner + wrapper + value.substring(end);
      newStart = start + wlen;
      newEnd = newStart + inner.length;
    }

    // 1. Update DOM langsung (imperative)
    el.value = newText;

    // 2. Set selection
    updateCursor(el, newStart, newEnd);

    // 3. Sync ke React
    onChange(newText);
  }, [value, onChange]);

  const renderPreview = (text: string) => {
    let html = text
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/\n/g, '<br>');
    html = html.replace(/```(.+?)```/g, '<code style="background:#edf2ed;padding:1px 4px;border-radius:3px;font-family:monospace">$1</code>');
    html = html.replace(/\*(.+?)\*/g, '<b>$1</b>');
    html = html.replace(/_(.+?)_/g, '<i>$1</i>');
    html = html.replace(/~(.+?)~/g, '<s>$1</s>');
    return html;
  };

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
        <ToggleButtonGroup size="small" exclusive={false}>
          {FORMATS.map(f => (
            <ToggleButton
              key={f.key}
              value={f.key}
              aria-label={f.label}
              onMouseDown={e => e.preventDefault()} // cegah textarea kehilangan fokus
              onClick={() => insertFormat(f.wrapper)}
              sx={{ px: 1, minWidth: 36 }}
            >
              {f.icon}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
        <Typography
          variant="caption"
          color="primary"
          onClick={() => setPreview(!preview)}
          sx={{ cursor: 'pointer', userSelect: 'none', fontWeight: 600 }}
        >
          {preview ? 'Edit' : 'Pratinjau'}
        </Typography>
      </Box>
      {preview ? (
        <Box
          sx={{
            minHeight: rows * 24 + 16,
            maxHeight: 360,
            overflowY: 'auto',
            p: 1.5,
            border: '1px solid',
            borderColor: error ? 'error.main' : 'divider',
            borderRadius: 1,
            fontSize: '0.9rem',
            lineHeight: 1.6,
            whiteSpace: 'pre-wrap',
          }}
          dangerouslySetInnerHTML={{ __html: renderPreview(value) || '<span style="color:#aaa">Ketik pesan…</span>' }}
        />
      ) : (
        <TextField
          fullWidth
          multiline
          rows={rows}
          size="small"
          value={value}
          onChange={e => onChange(e.target.value)}
          placeholder={placeholder || 'Tulis pesan broadcast…'}
          error={error}
          helperText={helperText}
          inputRef={textareaRef}
        />
      )}
    </Box>
  );
}
