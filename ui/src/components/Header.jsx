import { AppBar, Toolbar, Typography, IconButton, Box } from '@mui/material'
import { Menu as MenuIcon, Logout as LogoutIcon } from '@mui/icons-material'
import { Link, useNavigate } from 'react-router-dom'
import { logout } from '../common/auth_guard'
import AppIcon from './AppIcon'

export default function Header({ onToggleSidebar }) {
  const navigate = useNavigate()

  return (
    <AppBar
      position="fixed"
      elevation={0}
      sx={{
        zIndex: (theme) => theme.zIndex.drawer + 1,
        backgroundColor: 'rgba(255,255,255,0.72)',
        backdropFilter: 'saturate(180%) blur(20px)',
        borderBottom: '1px solid rgba(0,0,0,0.06)',
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
        <IconButton onClick={() => logout(navigate)} sx={{ color: 'text.secondary' }}>
          <LogoutIcon fontSize="small" />
        </IconButton>
      </Toolbar>
    </AppBar>
  )
}
