import { useState, useEffect } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { Box, Toolbar } from '@mui/material'
import Header from '../components/Header'
import SideBar from '../components/SideBar'
import { AlertProvider } from '../components/AlertStack'
import { checkAuth } from '../common/auth_guard'

export default function BasicLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const [sidebarOpen, setSidebarOpen] = useState(window.innerWidth > 840)

  useEffect(() => {
    checkAuth(navigate, location)
  }, [])

  return (
    <AlertProvider>
      <Box sx={{ display: 'flex', minHeight: '100vh', bgcolor: 'background.default' }}>
        <Header onToggleSidebar={() => setSidebarOpen(!sidebarOpen)} />
        <SideBar open={sidebarOpen} />
        <Box
          component="main"
          sx={{
            flexGrow: 1,
            px: { xs: 2, sm: 3, md: 4 },
            py: 3,
            maxWidth: 1200,
          }}
        >
          <Toolbar sx={{ minHeight: '52px !important' }} />
          <Outlet />
        </Box>
      </Box>
    </AlertProvider>
  )
}
