import { createTheme, alpha } from '@mui/material/styles';

// ────────────────────────────────────────────────────────────
// Design tokens — mirror index.css :root
// Flat, border-first, minimal shadow. Profesional & konsisten.
// ────────────────────────────────────────────────────────────

const TOKENS = {
  color: {
    primary:        '#168A4A',
    primaryLight:   '#2DB86A',
    primaryDark:    '#0C5C30',
    secondary:      '#3F4B58',
    secondaryLight: '#5B6876',
    secondaryDark:  '#2A3340',
    success:        '#148F45',
    warning:        '#C2410C',
    error:          '#D32F2F',
    info:           '#1565C0',
    bg:             '#F6F7F6',
    bgAlt:          '#EEF1EF',
    surface:        '#FFFFFF',
    surfaceMuted:   '#F0F3F1',
    text:           '#111814',
    textSecondary:  '#4A554F',
    border:         '#D7E0D9',
    borderLight:    '#E8EEE9',
  },
  font: {
    sans:  'Inter, Roboto, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    xs:    '12px',
    sm:    '13.5px',
    md:    '15px',
    lg:    '17px',
    xl:    '20px',
    xxl:   '26px',
    xxxl:  '32px',
    hero:  '40px',
  },
  weight: {
    normal:    400,
    medium:   500,
    semibold: 600,
    bold:     700,
  },
  radius: {
    sm: 8,
    md: 10,
    lg: 14,
  },
  space: {
    s1: 4, s2: 8, s3: 12, s4: 16, s5: 20,
    s6: 24, s8: 32, s10: 40, s12: 48,
  },
} as const;

const flat = {
  boxShadow: 'none',
  backgroundImage: 'none',
};

const theme = createTheme({
  palette: {
    mode: 'light',
    primary:    { main: TOKENS.color.primary,    light: TOKENS.color.primaryLight,  dark: TOKENS.color.primaryDark,    contrastText: '#ffffff' },
    secondary:  { main: TOKENS.color.secondary,  light: TOKENS.color.secondaryLight, dark: TOKENS.color.secondaryDark, contrastText: '#ffffff' },
    success:    { main: TOKENS.color.success, light: '#3CB56C', dark: '#0C6B32', contrastText: '#ffffff' },
    warning:    { main: TOKENS.color.warning, light: '#EA580C', dark: '#9A3412', contrastText: '#ffffff' },
    error:      { main: TOKENS.color.error,   light: '#EF5350', dark: '#B71C1C', contrastText: '#ffffff' },
    info:       { main: TOKENS.color.info,    light: '#42A5F5', dark: '#0D47A1', contrastText: '#ffffff' },
    background: { default: TOKENS.color.bg,  paper: TOKENS.color.surface },
    text:       { primary: TOKENS.color.text, secondary: TOKENS.color.textSecondary },
    divider:    TOKENS.color.border,
    action: {
      hover: alpha(TOKENS.color.primary, 0.04),
      selected: alpha(TOKENS.color.primary, 0.08),
      disabledBackground: TOKENS.color.surfaceMuted,
    },
  },
  shape: { borderRadius: TOKENS.radius.sm },
  // Flat UI: nonaktifkan semua elevation shadow MUI.
  shadows: [
    'none', 'none', 'none', 'none', 'none',
    'none', 'none', 'none', 'none', 'none',
    'none', 'none', 'none', 'none', 'none',
    'none', 'none', 'none', 'none', 'none',
    'none', 'none', 'none', 'none', 'none',
  ],
  typography: {
    fontFamily: TOKENS.font.sans,
    h1: { fontWeight: TOKENS.weight.bold, fontSize: TOKENS.font.hero, lineHeight: 1.15, letterSpacing: '-0.02em' },
    h2: { fontWeight: TOKENS.weight.bold, fontSize: TOKENS.font.xxxl, lineHeight: 1.2, letterSpacing: '-0.015em' },
    h3: { fontWeight: TOKENS.weight.bold, fontSize: TOKENS.font.xxl, lineHeight: 1.25, letterSpacing: '-0.01em' },
    h4: { fontWeight: TOKENS.weight.semibold, fontSize: TOKENS.font.xl, lineHeight: 1.3, letterSpacing: '-0.01em' },
    h5: { fontWeight: TOKENS.weight.semibold, fontSize: TOKENS.font.lg, lineHeight: 1.35 },
    h6: { fontWeight: TOKENS.weight.semibold, fontSize: TOKENS.font.md, lineHeight: 1.4 },
    subtitle1: { fontWeight: TOKENS.weight.medium, fontSize: TOKENS.font.sm, lineHeight: 1.45 },
    subtitle2: { fontWeight: TOKENS.weight.medium, fontSize: TOKENS.font.xs, lineHeight: 1.4 },
    body1: { fontSize: TOKENS.font.sm, lineHeight: 1.5 },
    body2: { fontSize: TOKENS.font.xs, lineHeight: 1.5 },
    caption: { fontSize: TOKENS.font.xs, lineHeight: 1.4, color: TOKENS.color.textSecondary },
    button: { textTransform: 'none', fontWeight: TOKENS.weight.semibold, letterSpacing: 0 },
  },
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: {
          backgroundColor: TOKENS.color.bg,
          color: TOKENS.color.text,
        },
        '*, *::before, *::after': {
          boxSizing: 'border-box',
        },
      },
    },
    MuiPaper: {
      defaultProps: { elevation: 0 },
      styleOverrides: {
        root: {
          ...flat,
          backgroundColor: TOKENS.color.surface,
        },
        outlined: {
          borderColor: TOKENS.color.border,
        },
      },
    },
    MuiButtonBase: {
      styleOverrides: { root: { cursor: 'pointer' } },
    },
    MuiButton: {
      defaultProps: { disableElevation: true, size: 'small' },
      styleOverrides: {
        root: {
          borderRadius: TOKENS.radius.sm,
          paddingBlock: 6,
          paddingInline: TOKENS.space.s3,
          minHeight: 34,
          boxShadow: 'none',
          '&:hover': { boxShadow: 'none' },
          '&:active': { boxShadow: 'none' },
        },
        sizeLarge: { minHeight: 40, paddingBlock: 8, paddingInline: TOKENS.space.s4 },
        contained: {
          boxShadow: 'none',
          '&:hover': { boxShadow: 'none' },
        },
        outlined: {
          borderColor: TOKENS.color.border,
          backgroundColor: TOKENS.color.surface,
          '&:hover': {
            borderColor: TOKENS.color.border,
            backgroundColor: TOKENS.color.surfaceMuted,
          },
        },
        text: {
          '&:hover': { backgroundColor: alpha(TOKENS.color.primary, 0.06) },
        },
      },
    },
    MuiCard: {
      defaultProps: { elevation: 0 },
      styleOverrides: {
        root: {
          ...flat,
          borderRadius: TOKENS.radius.md,
          border: `1px solid ${TOKENS.color.border}`,
          backgroundColor: TOKENS.color.surface,
        },
      },
    },
    MuiCardContent: {
      styleOverrides: {
        root: {
          padding: TOKENS.space.s4,
          '&:last-child': { paddingBottom: TOKENS.space.s4 },
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          ...flat,
          borderRadius: TOKENS.radius.lg,
          border: `1px solid ${TOKENS.color.border}`,
        },
      },
    },
    MuiDialogTitle: {
      styleOverrides: {
        root: {
          padding: `${TOKENS.space.s4} ${TOKENS.space.s5}`,
          fontSize: TOKENS.font.md,
          fontWeight: TOKENS.weight.semibold,
          borderBottom: `1px solid ${TOKENS.color.borderLight}`,
        },
      },
    },
    MuiDialogContent: {
      styleOverrides: { root: { padding: `${TOKENS.space.s4} ${TOKENS.space.s5}` } },
    },
    MuiDialogActions: {
      styleOverrides: {
        root: {
          padding: `${TOKENS.space.s3} ${TOKENS.space.s5} ${TOKENS.space.s4}`,
          borderTop: `1px solid ${TOKENS.color.borderLight}`,
        },
      },
    },
    MuiPopover: {
      defaultProps: { elevation: 0 },
      styleOverrides: {
        paper: {
          ...flat,
          border: `1px solid ${TOKENS.color.border}`,
          borderRadius: TOKENS.radius.md,
        },
      },
    },
    MuiMenu: {
      defaultProps: { elevation: 0 },
      styleOverrides: {
        paper: {
          ...flat,
          border: `1px solid ${TOKENS.color.border}`,
          borderRadius: TOKENS.radius.md,
          marginTop: 4,
        },
      },
    },
    MuiAlert: {
      defaultProps: { variant: 'outlined' },
      styleOverrides: {
        root: {
          alignItems: 'flex-start',
          borderRadius: TOKENS.radius.sm,
          padding: `${TOKENS.space.s2} ${TOKENS.space.s3}`,
          boxShadow: 'none',
        },
        icon: {
          alignItems: 'center',
          alignSelf: 'flex-start',
          display: 'flex',
          flexShrink: 0,
          fontSize: 20,
          lineHeight: 1,
          marginRight: TOKENS.space.s2,
          marginTop: 1,
          padding: 0,
        },
        message: {
          lineHeight: 1.45,
          minWidth: 0,
          padding: 0,
        },
        action: {
          alignItems: 'flex-start',
          marginRight: 0,
          padding: `0 0 0 ${TOKENS.space.s2}`,
        },
      },
    },
    MuiIconButton: {
      defaultProps: { size: 'small' },
      styleOverrides: {
        root: {
          borderRadius: TOKENS.radius.sm,
          padding: TOKENS.space.s1,
          '&:hover': { backgroundColor: alpha(TOKENS.color.primary, 0.06) },
        },
      },
    },
    MuiOutlinedInput: {
      styleOverrides: {
        root: {
          borderRadius: TOKENS.radius.sm,
          backgroundColor: TOKENS.color.surface,
          '& .MuiOutlinedInput-notchedOutline': {
            borderColor: TOKENS.color.border,
          },
          '&:hover .MuiOutlinedInput-notchedOutline': {
            borderColor: '#C5CFC8',
          },
          '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
            borderWidth: 1.5,
            borderColor: TOKENS.color.primary,
          },
        },
        // Padding vertikal diselaraskan dengan transform label di MuiInputLabel
        // agar label kosong (Username/Password) benar-benar center, tidak “molorot”.
        input: {
          paddingTop: 12,
          paddingBottom: 12,
          paddingLeft: 14,
          paddingRight: 14,
          height: '1.4375em',
          boxSizing: 'content-box',
        },
        // sizeSmall class di root (bukan di input) pada MUI v9
        sizeSmall: {
          '& .MuiOutlinedInput-input': {
            paddingTop: 8,
            paddingBottom: 8,
            paddingLeft: 12,
            paddingRight: 12,
          },
        },
      },
    },
    MuiInputBase: {
      styleOverrides: { root: { fontSize: TOKENS.font.sm } },
    },
    MuiInputLabel: {
      styleOverrides: {
        root: {
          fontSize: TOKENS.font.sm,
          // Samakan line-height dengan input agar posisi baseline label center
          lineHeight: 1.4375,
          maxWidth: 'calc(100% - 24px)',
          overflow: 'visible',
          // Label kosong (belum shrink): center vertikal di dalam field
          '&.MuiInputLabel-outlined:not(.MuiInputLabel-shrink)': {
            transform: 'translate(14px, 12px) scale(1)',
          },
          '&.MuiInputLabel-sizeSmall:not(.MuiInputLabel-shrink)': {
            transform: 'translate(12px, 8px) scale(1)',
          },
          '&.MuiInputLabel-outlined.MuiInputLabel-shrink': {
            backgroundColor: TOKENS.color.surface,
            borderRadius: 2,
            marginLeft: -4,
            paddingLeft: 4,
            paddingRight: 4,
            zIndex: 1,
            transform: 'translate(14px, -9px) scale(0.75)',
          },
          '&.MuiInputLabel-sizeSmall.MuiInputLabel-shrink': {
            transform: 'translate(12px, -8px) scale(0.75)',
          },
          '&.MuiInputLabel-outlined:not(.MuiInputLabel-shrink) + .MuiOutlinedInput-root .MuiOutlinedInput-notchedOutline legend': {
            maxWidth: '0 !important',
          },
        },
      },
    },
    MuiFormControl: {
      styleOverrides: {
        root: { overflow: 'visible' },
      },
    },
    MuiChip: {
      defaultProps: { size: 'small' },
      styleOverrides: {
        root: {
          borderRadius: 6,
          fontWeight: TOKENS.weight.semibold,
          height: 26,
          // Jangan paksa warna muted di root — biarkan color* / filled menang.
          letterSpacing: 0.01,
        },
        label: {
          paddingLeft: 8,
          paddingRight: 8,
          fontWeight: TOKENS.weight.semibold,
          lineHeight: 1.2,
        },
        sizeSmall: {
          height: 24,
          fontSize: '0.75rem',
          '& .MuiChip-label': { paddingLeft: 8, paddingRight: 8 },
        },
        // Default / tanpa color: netral tapi tetap terbaca
        filled: {
          backgroundColor: '#E8EEEA',
          color: TOKENS.color.text,
          border: `1px solid ${TOKENS.color.border}`,
        },
        outlined: {
          backgroundColor: TOKENS.color.surface,
          borderColor: TOKENS.color.border,
          color: TOKENS.color.textSecondary,
          fontWeight: TOKENS.weight.semibold,
        },
        // Filled + color: solid-ish background, teks kontras kuat
        colorDefault: {
          '&.MuiChip-filled': {
            backgroundColor: '#E2E8E4',
            color: TOKENS.color.text,
            border: `1px solid ${TOKENS.color.border}`,
          },
          '&.MuiChip-outlined': {
            borderColor: '#B8C4BC',
            color: TOKENS.color.text,
            backgroundColor: '#F7FAF8',
          },
        },
        colorPrimary: {
          '&.MuiChip-filled': {
            backgroundColor: TOKENS.color.primary,
            color: '#ffffff',
            border: `1px solid ${TOKENS.color.primaryDark}`,
          },
          '&.MuiChip-outlined': {
            backgroundColor: alpha(TOKENS.color.primary, 0.12),
            borderColor: TOKENS.color.primary,
            color: TOKENS.color.primaryDark,
          },
        },
        colorSecondary: {
          '&.MuiChip-filled': {
            backgroundColor: TOKENS.color.secondary,
            color: '#ffffff',
            border: `1px solid ${TOKENS.color.secondaryDark}`,
          },
          '&.MuiChip-outlined': {
            backgroundColor: alpha(TOKENS.color.secondary, 0.1),
            borderColor: TOKENS.color.secondary,
            color: TOKENS.color.secondaryDark,
          },
        },
        colorSuccess: {
          '&.MuiChip-filled': {
            backgroundColor: TOKENS.color.success,
            color: '#ffffff',
            border: `1px solid ${TOKENS.color.primaryDark}`,
          },
          '&.MuiChip-outlined': {
            backgroundColor: alpha(TOKENS.color.success, 0.14),
            borderColor: TOKENS.color.success,
            color: '#0A5C2C',
          },
        },
        colorWarning: {
          '&.MuiChip-filled': {
            backgroundColor: TOKENS.color.warning,
            color: '#ffffff',
            border: `1px solid #9A3412`,
          },
          '&.MuiChip-outlined': {
            backgroundColor: alpha(TOKENS.color.warning, 0.14),
            borderColor: TOKENS.color.warning,
            color: '#9A3412',
          },
        },
        colorError: {
          '&.MuiChip-filled': {
            backgroundColor: TOKENS.color.error,
            color: '#ffffff',
            border: `1px solid #B71C1C`,
          },
          '&.MuiChip-outlined': {
            backgroundColor: alpha(TOKENS.color.error, 0.12),
            borderColor: TOKENS.color.error,
            color: '#B71C1C',
          },
        },
        colorInfo: {
          '&.MuiChip-filled': {
            backgroundColor: TOKENS.color.info,
            color: '#ffffff',
            border: `1px solid #0D47A1`,
          },
          '&.MuiChip-outlined': {
            backgroundColor: alpha(TOKENS.color.info, 0.12),
            borderColor: TOKENS.color.info,
            color: '#0D47A1',
          },
        },
        deleteIcon: {
          color: 'inherit',
          opacity: 0.75,
          '&:hover': { opacity: 1 },
        },
        icon: {
          color: 'inherit',
        },
      },
    },
    MuiBadge: {
      styleOverrides: {
        badge: {
          boxShadow: 'none',
          fontWeight: TOKENS.weight.bold,
          fontSize: '0.68rem',
          minWidth: 18,
          height: 18,
          padding: '0 5px',
        },
        colorError: {
          backgroundColor: TOKENS.color.error,
          color: '#ffffff',
        },
        colorPrimary: {
          backgroundColor: TOKENS.color.primary,
          color: '#ffffff',
        },
        colorSuccess: {
          backgroundColor: TOKENS.color.success,
          color: '#ffffff',
        },
        colorWarning: {
          backgroundColor: TOKENS.color.warning,
          color: '#ffffff',
        },
        colorInfo: {
          backgroundColor: TOKENS.color.info,
          color: '#ffffff',
        },
      },
    },
    MuiAvatar: {
      styleOverrides: {
        root: {
          fontWeight: TOKENS.weight.semibold,
          fontSize: 13,
        },
      },
    },
    MuiMenuItem: {
      styleOverrides: {
        root: {
          cursor: 'pointer',
          fontSize: TOKENS.font.sm,
          borderRadius: 6,
          marginInline: 4,
          minHeight: 36,
          '&.Mui-selected': {
            backgroundColor: alpha(TOKENS.color.primary, 0.08),
            '&:hover': { backgroundColor: alpha(TOKENS.color.primary, 0.12) },
          },
        },
      },
    },
    MuiListItemButton: {
      styleOverrides: {
        root: {
          cursor: 'pointer',
          minHeight: 40,
          paddingTop: TOKENS.space.s1,
          paddingBottom: TOKENS.space.s1,
          borderRadius: TOKENS.radius.sm,
          '&.Mui-selected': {
            backgroundColor: alpha(TOKENS.color.primary, 0.08),
            '&:hover': { backgroundColor: alpha(TOKENS.color.primary, 0.12) },
          },
        },
      },
    },
    MuiListItemText: {
      styleOverrides: {
        primary: { fontSize: TOKENS.font.sm },
        secondary: { fontSize: TOKENS.font.xs },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderColor: TOKENS.color.borderLight,
          padding: `${TOKENS.space.s2} ${TOKENS.space.s3}`,
          fontSize: TOKENS.font.xs,
        },
        head: {
          fontWeight: TOKENS.weight.semibold,
          color: TOKENS.color.textSecondary,
          backgroundColor: TOKENS.color.surfaceMuted,
        },
      },
    },
    MuiTableContainer: {
      styleOverrides: {
        root: {
          border: `1px solid ${TOKENS.color.border}`,
          borderRadius: TOKENS.radius.md,
          boxShadow: 'none',
        },
      },
    },
    MuiLinearProgress: {
      styleOverrides: {
        root: {
          borderRadius: 9999,
          height: 6,
          backgroundColor: TOKENS.color.surfaceMuted,
        },
        bar: { borderRadius: 9999 },
      },
    },
    MuiTooltip: {
      styleOverrides: {
        tooltip: {
          backgroundColor: '#1F2933',
          fontSize: 11.5,
          fontWeight: TOKENS.weight.medium,
          borderRadius: 6,
          padding: '6px 10px',
          boxShadow: 'none',
        },
      },
    },
    MuiDivider: {
      styleOverrides: {
        root: { borderColor: TOKENS.color.borderLight },
      },
    },
    MuiAccordion: {
      defaultProps: { elevation: 0, disableGutters: true },
      styleOverrides: {
        root: {
          ...flat,
          border: `1px solid ${TOKENS.color.border}`,
          borderRadius: `${TOKENS.radius.md}px !important`,
          '&:before': { display: 'none' },
          '&.Mui-expanded': { margin: 0 },
        },
      },
    },
    MuiToggleButton: {
      styleOverrides: {
        root: {
          borderColor: TOKENS.color.border,
          textTransform: 'none',
          fontWeight: TOKENS.weight.semibold,
          color: TOKENS.color.textSecondary,
          '&.Mui-selected': {
            backgroundColor: alpha(TOKENS.color.primary, 0.16),
            color: TOKENS.color.primaryDark,
            borderColor: TOKENS.color.primary,
            fontWeight: TOKENS.weight.bold,
            '&:hover': { backgroundColor: alpha(TOKENS.color.primary, 0.22) },
          },
        },
      },
    },
    MuiSwitch: {
      styleOverrides: {
        root: { padding: 8 },
        track: { borderRadius: 12, opacity: 0.25 },
      },
    },
  },
});

export default theme;
