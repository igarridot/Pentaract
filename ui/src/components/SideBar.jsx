import { Drawer, List, Toolbar, Box } from '@mui/material'
import { Storage as StorageIcon, SmartToy as WorkerIcon } from '@mui/icons-material'
import SideBarItem from './SideBarItem'

const DRAWER_WIDTH = 220

export default function SideBar({ open }) {
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
          backgroundColor: 'rgba(255,255,255,0.6)',
          backdropFilter: 'saturate(180%) blur(20px)',
          borderRight: '1px solid rgba(0,0,0,0.06)',
        },
      }}
    >
      <Toolbar sx={{ minHeight: '52px !important' }} />
      <Box sx={{ px: 0.5, pt: 1 }}>
        <List disablePadding>
          <SideBarItem to="/storages" icon={<StorageIcon />} label="Storages" showLabel={open} />
          <SideBarItem to="/storage_workers" icon={<WorkerIcon />} label="Workers" showLabel={open} />
        </List>
      </Box>
    </Drawer>
  )
}
