import React from 'react'
import ReactDOM from 'react-dom/client'
import { ThemeProvider, createTheme, CssBaseline } from '@mui/material'
import App from './App'

const theme = createTheme({
  palette: {
    mode: 'light',
    primary: { main: '#0071e3', light: '#2997ff', dark: '#0058b0' },
    secondary: { main: '#86868b' },
    error: { main: '#ff3b30' },
    warning: { main: '#ff9f0a' },
    success: { main: '#30d158' },
    background: {
      default: '#f5f5f7',
      paper: '#ffffff',
    },
    text: {
      primary: '#1d1d1f',
      secondary: '#86868b',
    },
    divider: 'rgba(0,0,0,0.06)',
  },
  typography: {
    fontFamily: '"Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif',
    h4: { fontWeight: 700, letterSpacing: '-0.02em' },
    h5: { fontWeight: 600, letterSpacing: '-0.01em' },
    h6: { fontWeight: 600, letterSpacing: '-0.01em' },
    body1: { fontWeight: 400 },
    body2: { fontWeight: 400 },
    button: { textTransform: 'none', fontWeight: 500 },
  },
  shape: { borderRadius: 12 },
  shadows: [
    'none',
    '0 1px 3px rgba(0,0,0,0.04)',
    '0 2px 8px rgba(0,0,0,0.06)',
    '0 4px 16px rgba(0,0,0,0.08)',
    '0 8px 32px rgba(0,0,0,0.10)',
    ...Array(20).fill('0 8px 32px rgba(0,0,0,0.10)'),
  ],
  components: {
    MuiCssBaseline: {
      styleOverrides: {
        body: {
          backgroundColor: '#f5f5f7',
          WebkitFontSmoothing: 'antialiased',
          MozOsxFontSmoothing: 'grayscale',
        },
        '*::-webkit-scrollbar': { width: 6 },
        '*::-webkit-scrollbar-track': { background: 'transparent' },
        '*::-webkit-scrollbar-thumb': { background: 'rgba(0,0,0,0.15)', borderRadius: 3 },
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
          borderColor: 'rgba(0,0,0,0.12)',
          '&:hover': { borderColor: 'rgba(0,0,0,0.24)', backgroundColor: 'rgba(0,0,0,0.02)' },
        },
      },
    },
    MuiFab: {
      styleOverrides: {
        root: {
          borderRadius: 980,
          boxShadow: '0 2px 12px rgba(0,0,0,0.12)',
          '&:hover': { boxShadow: '0 4px 20px rgba(0,0,0,0.16)' },
        },
      },
    },
    MuiPaper: {
      defaultProps: { elevation: 0 },
      styleOverrides: {
        root: {
          borderRadius: 16,
          border: '1px solid rgba(0,0,0,0.06)',
        },
      },
    },
    MuiDialog: {
      styleOverrides: {
        paper: {
          borderRadius: 20,
          border: 'none',
          boxShadow: '0 24px 80px rgba(0,0,0,0.14)',
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
            backgroundColor: '#f5f5f7',
            '& fieldset': { borderColor: 'transparent' },
            '&:hover fieldset': { borderColor: 'rgba(0,0,0,0.12)' },
            '&.Mui-focused fieldset': { borderColor: '#0071e3', borderWidth: 1.5 },
          },
        },
      },
    },
    MuiSelect: {
      styleOverrides: {
        root: {
          borderRadius: 10,
          backgroundColor: '#f5f5f7',
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
          '&:hover': { backgroundColor: 'rgba(0,0,0,0.03)' },
        },
      },
    },
    MuiAlert: {
      styleOverrides: {
        root: {
          borderRadius: 14,
          border: 'none',
          boxShadow: '0 4px 24px rgba(0,0,0,0.10)',
          backdropFilter: 'blur(20px)',
        },
        standardSuccess: { backgroundColor: 'rgba(48,209,88,0.12)', color: '#1d1d1f' },
        standardError: { backgroundColor: 'rgba(255,59,48,0.12)', color: '#1d1d1f' },
        standardInfo: { backgroundColor: 'rgba(0,113,227,0.12)', color: '#1d1d1f' },
        standardWarning: { backgroundColor: 'rgba(255,159,10,0.12)', color: '#1d1d1f' },
      },
    },
    MuiChip: {
      styleOverrides: {
        root: { borderRadius: 8, fontWeight: 500 },
      },
    },
    MuiLinearProgress: {
      styleOverrides: {
        root: { borderRadius: 4, height: 4, backgroundColor: 'rgba(0,0,0,0.06)' },
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
        root: { borderColor: 'rgba(0,0,0,0.05)' },
      },
    },
    MuiIconButton: {
      styleOverrides: {
        root: {
          borderRadius: 10,
          '&:hover': { backgroundColor: 'rgba(0,0,0,0.04)' },
        },
      },
    },
    MuiMenu: {
      styleOverrides: {
        paper: {
          borderRadius: 12,
          boxShadow: '0 8px 32px rgba(0,0,0,0.12)',
          border: '1px solid rgba(0,0,0,0.06)',
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
          '&:hover': { backgroundColor: 'rgba(0,0,0,0.04)' },
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: { borderColor: 'rgba(0,0,0,0.05)' },
        head: { fontWeight: 600, color: '#86868b', fontSize: '0.75rem', textTransform: 'uppercase', letterSpacing: '0.04em' },
      },
    },
  },
})

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <App />
    </ThemeProvider>
  </React.StrictMode>
)
