import { createContext, useContext, useState, useMemo, useEffect } from 'react'
import { createTheme, useMediaQuery } from '@mui/material'

const ThemeModeContext = createContext()

const STORAGE_KEY = 'theme_mode'

const baseTypography = {
  fontFamily: '"Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif',
  h4: { fontWeight: 700, letterSpacing: '-0.02em' },
  h5: { fontWeight: 600, letterSpacing: '-0.01em' },
  h6: { fontWeight: 600, letterSpacing: '-0.01em' },
  body1: { fontWeight: 400 },
  body2: { fontWeight: 400 },
  button: { textTransform: 'none', fontWeight: 500 },
}

const sharedComponents = (mode) => {
  const isDark = mode === 'dark'
  const alpha = (opacity) => isDark ? `rgba(255,255,255,${opacity})` : `rgba(0,0,0,${opacity})`
  const bg = isDark ? '#1c1c1e' : '#f5f5f7'
  const inputBg = isDark ? '#2c2c2e' : '#f5f5f7'

  return {
    MuiCssBaseline: {
      styleOverrides: {
        body: {
          backgroundColor: bg,
          WebkitFontSmoothing: 'antialiased',
          MozOsxFontSmoothing: 'grayscale',
        },
        '*::-webkit-scrollbar': { width: 6 },
        '*::-webkit-scrollbar-track': { background: 'transparent' },
        '*::-webkit-scrollbar-thumb': { background: alpha(0.15), borderRadius: 3 },
      },
    },
    MuiButton: {
      defaultProps: { disableElevation: true },
      styleOverrides: {
        root: {
          borderRadius: 980,
          padding: '8px 20px',
          fontSize: '0.875rem',
          fontWeight: 500,
        },
        contained: {
          '&:hover': { opacity: 0.88 },
        },
        outlined: {
          borderColor: alpha(0.12),
          '&:hover': { borderColor: alpha(0.24), backgroundColor: alpha(0.02) },
        },
      },
    },
    MuiFab: {
      styleOverrides: {
        root: {
          borderRadius: 980,
          boxShadow: isDark ? '0 2px 12px rgba(0,0,0,0.4)' : '0 2px 12px rgba(0,0,0,0.12)',
          '&:hover': { boxShadow: isDark ? '0 4px 20px rgba(0,0,0,0.5)' : '0 4px 20px rgba(0,0,0,0.16)' },
        },
      },
    },
    MuiPaper: {
      defaultProps: { elevation: 0 },
      styleOverrides: {
        root: {
          borderRadius: 16,
          border: `1px solid ${alpha(0.06)}`,
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          borderRadius: 20,
          border: 'none',
          boxShadow: isDark ? '0 24px 80px rgba(0,0,0,0.5)' : '0 24px 80px rgba(0,0,0,0.14)',
        },
      },
    },
    MuiDialogTitle: {
      styleOverrides: {
        root: { fontWeight: 600, fontSize: '1.125rem', padding: '24px 24px 8px' },
      },
    },
    MuiDialogContent: {
      styleOverrides: { root: { padding: '16px 24px' } },
    },
    MuiDialogActions: {
      styleOverrides: { root: { padding: '12px 24px 20px' } },
    },
    MuiTextField: {
      defaultProps: { variant: 'outlined', size: 'small' },
      styleOverrides: {
        root: {
          '& .MuiOutlinedInput-root': {
            borderRadius: 10,
            backgroundColor: inputBg,
            '& fieldset': { borderColor: 'transparent' },
            '&:hover fieldset': { borderColor: alpha(0.12) },
            '&.Mui-focused fieldset': { borderColor: '#0071e3', borderWidth: 1.5 },
          },
        },
      },
    },
    MuiSelect: {
      styleOverrides: {
        root: {
          borderRadius: 10,
          backgroundColor: inputBg,
        },
      },
    },
    MuiListItemButton: {
      styleOverrides: {
        root: {
          borderRadius: 10,
          margin: '2px 8px',
          '&.Mui-selected': {
            backgroundColor: 'rgba(0,113,227,0.08)',
            '&:hover': { backgroundColor: 'rgba(0,113,227,0.12)' },
          },
          '&:hover': { backgroundColor: alpha(0.03) },
        },
      },
    },
    MuiAlert: {
      styleOverrides: {
        root: {
          borderRadius: 14,
          border: 'none',
          boxShadow: isDark ? '0 4px 24px rgba(0,0,0,0.3)' : '0 4px 24px rgba(0,0,0,0.10)',
          backdropFilter: 'blur(20px)',
        },
        standardSuccess: { backgroundColor: 'rgba(48,209,88,0.12)', color: isDark ? '#f5f5f7' : '#1d1d1f' },
        standardError: { backgroundColor: 'rgba(255,59,48,0.12)', color: isDark ? '#f5f5f7' : '#1d1d1f' },
        standardInfo: { backgroundColor: 'rgba(0,113,227,0.12)', color: isDark ? '#f5f5f7' : '#1d1d1f' },
        standardWarning: { backgroundColor: 'rgba(255,159,10,0.12)', color: isDark ? '#f5f5f7' : '#1d1d1f' },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: { borderRadius: 8, fontWeight: 500 },
      },
    },
    MuiLinearProgress: {
      styleOverrides: {
        root: { borderRadius: 4, height: 4, backgroundColor: alpha(0.06) },
        bar: { borderRadius: 4 },
      },
    },
    MuiBreadcrumbs: {
      styleOverrides: {
        root: { fontSize: '0.875rem' },
      },
    },
    MuiDivider: {
      styleOverrides: {
        root: { borderColor: alpha(0.05) },
      },
    },
    MuiIconButton: {
      styleOverrides: {
        root: {
          borderRadius: 10,
          '&:hover': { backgroundColor: alpha(0.04) },
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          borderRadius: 12,
          boxShadow: isDark ? '0 8px 32px rgba(0,0,0,0.4)' : '0 8px 32px rgba(0,0,0,0.12)',
          border: `1px solid ${alpha(0.06)}`,
          minWidth: 160,
        },
      },
    },
    MuiMenuItem: {
      styleOverrides: {
        root: {
          borderRadius: 8,
          margin: '2px 6px',
          fontSize: '0.875rem',
          '&:hover': { backgroundColor: alpha(0.04) },
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: { borderColor: alpha(0.05) },
        head: { fontWeight: 600, color: isDark ? '#98989d' : '#86868b', fontSize: '0.75rem', textTransform: 'uppercase', letterSpacing: '0.04em' },
      },
    },
  }
}

function buildTheme(mode) {
  const isDark = mode === 'dark'
  return createTheme({
    palette: {
      mode,
      primary: { main: '#0071e3', light: '#2997ff', dark: '#0058b0' },
      secondary: { main: isDark ? '#98989d' : '#86868b' },
      error: { main: '#ff3b30' },
      warning: { main: '#ff9f0a' },
      success: { main: '#30d158' },
      background: {
        default: isDark ? '#1c1c1e' : '#f5f5f7',
        paper: isDark ? '#2c2c2e' : '#ffffff',
      },
      text: {
        primary: isDark ? '#f5f5f7' : '#1d1d1f',
        secondary: isDark ? '#98989d' : '#86868b',
      },
      divider: isDark ? 'rgba(255,255,255,0.06)' : 'rgba(0,0,0,0.06)',
    },
    typography: baseTypography,
    shape: { borderRadius: 12 },
    shadows: [
      'none',
      isDark ? '0 1px 3px rgba(0,0,0,0.2)' : '0 1px 3px rgba(0,0,0,0.04)',
      isDark ? '0 2px 8px rgba(0,0,0,0.25)' : '0 2px 8px rgba(0,0,0,0.06)',
      isDark ? '0 4px 16px rgba(0,0,0,0.3)' : '0 4px 16px rgba(0,0,0,0.08)',
      isDark ? '0 8px 32px rgba(0,0,0,0.35)' : '0 8px 32px rgba(0,0,0,0.10)',
      ...Array(20).fill(isDark ? '0 8px 32px rgba(0,0,0,0.35)' : '0 8px 32px rgba(0,0,0,0.10)'),
    ],
    components: sharedComponents(mode),
  })
}

export function ThemeModeProvider({ children }) {
  const prefersDark = useMediaQuery('(prefers-color-scheme: dark)')
  const [setting, setSetting] = useState(() => localStorage.getItem(STORAGE_KEY) || 'auto')

  const resolvedMode = setting === 'auto' ? (prefersDark ? 'dark' : 'light') : setting

  const theme = useMemo(() => buildTheme(resolvedMode), [resolvedMode])

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, setting)
  }, [setting])

  const value = useMemo(() => ({ setting, setSetting, resolvedMode }), [setting, resolvedMode])

  return (
    <ThemeModeContext.Provider value={value}>
      {children(theme)}
    </ThemeModeContext.Provider>
  )
}

export function useThemeMode() {
  return useContext(ThemeModeContext)
}
