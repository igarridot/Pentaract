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
      <Box sx={{ display: 'flex' }}>
        <Header onToggleSidebar={() => setSidebarOpen(!sidebarOpen)} />
        <SideBar open={sidebarOpen} />
        <Box component="main" sx={{ flexGrow: 1, p: 3 }}>
          <Toolbar />
          <Outlet />
        </Box>
      </Box>
    </AlertProvider>
  )
}
