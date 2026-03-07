import { AppBar, Toolbar, Typography, IconButton, Box } from '@mui/material'
import { Menu as MenuIcon, Logout as LogoutIcon } from '@mui/icons-material'
import { Link, useNavigate } from 'react-router-dom'
import { logout } from '../common/auth_guard'
import AppIcon from './AppIcon'

export default function Header({ onToggleSidebar }) {
  const navigate = useNavigate()

  return (
    <AppBar position="fixed" sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}>
      <Toolbar>
        <IconButton color="inherit" edge="start" onClick={onToggleSidebar} sx={{ mr: 2 }}>
          <MenuIcon />
        </IconButton>
        <Link to="/" style={{ textDecoration: 'none', color: 'inherit', display: 'flex', alignItems: 'center' }}>
          <AppIcon sx={{ mr: 1 }} />
          <Typography variant="h6" noWrap>
            Pentaract
          </Typography>
        </Link>
        <Box sx={{ flexGrow: 1 }} />
        <IconButton color="inherit" onClick={() => logout(navigate)}>
          <LogoutIcon />
        </IconButton>
      </Toolbar>
    </AppBar>
  )
}
