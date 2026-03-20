import { useEffect, useState } from 'react'
import { Drawer, List, Toolbar, Box } from '@mui/material'
import {
  Storage as StorageIcon,
  SmartToy as WorkerIcon,
  ManageAccounts as UsersIcon,
  DriveFolderUpload as LocalFilesIcon,
} from '@mui/icons-material'
import SideBarItem from './SideBarItem'
import API from '../api'

const DRAWER_WIDTH = 220

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
          <SideBarItem to="/local-files" icon={<LocalFilesIcon />} label="Local Files" showLabel={open} />
          <SideBarItem to="/storage_workers" icon={<WorkerIcon />} label="Workers" showLabel={open} />
          {isAdmin && <SideBarItem to="/users" icon={<UsersIcon />} label="Users" showLabel={open} />}
        </List>
      </Box>
    </Drawer>
  )
}
