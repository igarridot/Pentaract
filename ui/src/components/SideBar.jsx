import { Drawer, List, Toolbar } from '@mui/material'
import { Storage as StorageIcon, SmartToy as WorkerIcon } from '@mui/icons-material'
import SideBarItem from './SideBarItem'

const DRAWER_WIDTH = 240

export default function SideBar({ open }) {
  return (
    <Drawer
      variant="permanent"
      sx={{
        width: open ? DRAWER_WIDTH : 64,
        flexShrink: 0,
        '& .MuiDrawer-paper': {
          width: open ? DRAWER_WIDTH : 64,
          boxSizing: 'border-box',
          overflowX: 'hidden',
          transition: 'width 0.2s',
        },
      }}
    >
      <Toolbar />
      <List>
        <SideBarItem to="/storages" icon={<StorageIcon />} label="Storages" showLabel={open} />
        <SideBarItem to="/storage_workers" icon={<WorkerIcon />} label="Storage Workers" showLabel={open} />
      </List>
    </Drawer>
  )
}
