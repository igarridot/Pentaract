import { useEffect, useState } from 'react'
import { Drawer, List, Toolbar, Box, ListItem, ListItemButton, ListItemIcon, ListItemText } from '@mui/material'
import { Storage as StorageIcon, SmartToy as WorkerIcon, ManageAccounts as UsersIcon, DriveFolderUpload as LocalUploadIcon } from '@mui/icons-material'
import { Link, useLocation } from 'react-router-dom'
import API from '../api'

const DRAWER_WIDTH = 220

function SideBarItem({ to, icon, label, showLabel }) {
  const location = useLocation()
  const selected = location.pathname.startsWith(to)

  return (
    <ListItem disablePadding sx={{ mb: 0.5 }}>
      <ListItemButton
        component={Link}
        to={to}
        selected={selected}
        sx={{
          minHeight: 40,
          justifyContent: showLabel ? 'initial' : 'center',
          px: showLabel ? 2 : 1.5,
        }}
      >
        <ListItemIcon
          sx={{
            minWidth: showLabel ? 36 : 'auto',
            color: selected ? 'primary.main' : 'text.secondary',
            fontSize: 20,
            '& .MuiSvgIcon-root': { fontSize: 20 },
          }}
        >
          {icon}
        </ListItemIcon>
        {showLabel && (
          <ListItemText
            primary={label}
            primaryTypographyProps={{
              fontSize: '0.875rem',
              fontWeight: selected ? 600 : 400,
              color: selected ? 'primary.main' : 'text.primary',
            }}
          />
        )}
      </ListItemButton>
    </ListItem>
  )
}

export default function SideBar({ open }) {
  const [isAdmin, setIsAdmin] = useState(false)

  useEffect(() => {
    let cancelled = false
    API.users.adminStatus()
      .then((data) => {
        if (!cancelled) setIsAdmin(!!data?.is_admin)
      })
      .catch(() => {
        if (!cancelled) setIsAdmin(false)
      })
    return () => { cancelled = true }
  }, [])

  return (
    <Drawer
      variant="permanent"
      sx={{
        width: open ? DRAWER_WIDTH : 60,
        flexShrink: 0,
        '& .MuiDrawer-paper': {
          width: open ? DRAWER_WIDTH : 60,
          boxSizing: 'border-box',
          overflowX: 'hidden',
          transition: 'width 0.25s cubic-bezier(0.4, 0, 0.2, 1)',
          backgroundColor: (theme) => theme.palette.mode === 'dark'
            ? 'rgba(28,28,30,0.6)'
            : 'rgba(255,255,255,0.6)',
          backdropFilter: 'saturate(180%) blur(20px)',
          borderRight: '1px solid',
          borderColor: 'divider',
        },
      }}
    >
      <Toolbar sx={{ minHeight: '52px !important' }} />
      <Box sx={{ px: 0.5, pt: 1 }}>
        <List disablePadding>
          <SideBarItem to="/storages" icon={<StorageIcon />} label="Storages" showLabel={open} />
          <SideBarItem to="/storage_workers" icon={<WorkerIcon />} label="Workers" showLabel={open} />
          <SideBarItem to="/local-upload" icon={<LocalUploadIcon />} label="Local Upload" showLabel={open} />
          {isAdmin && <SideBarItem to="/users" icon={<UsersIcon />} label="Users" showLabel={open} />}
        </List>
      </Box>
    </Drawer>
  )
}
