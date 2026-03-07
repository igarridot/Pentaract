import { useState } from 'react'
import { AppBar, Toolbar, Typography, IconButton, Box, Menu, MenuItem, ListItemIcon, ListItemText } from '@mui/material'
import {
  Menu as MenuIcon,
  Logout as LogoutIcon,
  LightMode as LightIcon,
  DarkMode as DarkIcon,
  SettingsBrightness as AutoIcon,
} from '@mui/icons-material'
import { Link, useNavigate } from 'react-router-dom'
import { logout } from '../common/auth_guard'
import { useThemeMode } from '../common/theme_context'
import AppIcon from './AppIcon'

const modeOptions = [
  { value: 'light', label: 'Light', icon: <LightIcon fontSize="small" /> },
  { value: 'dark', label: 'Dark', icon: <DarkIcon fontSize="small" /> },
  { value: 'auto', label: 'Auto', icon: <AutoIcon fontSize="small" /> },
]

export default function Header({ onToggleSidebar }) {
  const navigate = useNavigate()
  const { setting, setSetting, resolvedMode } = useThemeMode()
  const [anchorEl, setAnchorEl] = useState(null)

  const currentIcon = setting === 'dark' ? <DarkIcon fontSize="small" />
    : setting === 'light' ? <LightIcon fontSize="small" />
    : <AutoIcon fontSize="small" />

  return (
    <AppBar
      position="fixed"
      elevation={0}
      sx={{
        zIndex: (theme) => theme.zIndex.drawer + 1,
        backgroundColor: (theme) => theme.palette.mode === 'dark'
          ? 'rgba(28,28,30,0.72)'
          : 'rgba(255,255,255,0.72)',
        backdropFilter: 'saturate(180%) blur(20px)',
        borderBottom: '1px solid',
        borderColor: 'divider',
        color: 'text.primary',
      }}
    >
      <Toolbar sx={{ minHeight: '52px !important', px: 2 }}>
        <IconButton
          edge="start"
          onClick={onToggleSidebar}
          sx={{ mr: 1.5, color: 'text.secondary' }}
        >
          <MenuIcon fontSize="small" />
        </IconButton>
        <Link to="/" style={{ textDecoration: 'none', color: 'inherit', display: 'flex', alignItems: 'center', gap: 8 }}>
          <AppIcon sx={{ fontSize: 24 }} />
          <Typography variant="body1" sx={{ fontWeight: 600, letterSpacing: '-0.01em' }}>
            Pentaract
          </Typography>
        </Link>
        <Box sx={{ flexGrow: 1 }} />
        <IconButton onClick={(e) => setAnchorEl(e.currentTarget)} sx={{ color: 'text.secondary', mr: 0.5 }}>
          {currentIcon}
        </IconButton>
        <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => setAnchorEl(null)}>
          {modeOptions.map((opt) => (
            <MenuItem
              key={opt.value}
              selected={setting === opt.value}
              onClick={() => { setSetting(opt.value); setAnchorEl(null) }}
            >
              <ListItemIcon>{opt.icon}</ListItemIcon>
              <ListItemText>{opt.label}</ListItemText>
            </MenuItem>
          ))}
        </Menu>
        <IconButton onClick={() => logout(navigate)} sx={{ color: 'text.secondary' }}>
          <LogoutIcon fontSize="small" />
        </IconButton>
      </Toolbar>
    </AppBar>
  )
}
