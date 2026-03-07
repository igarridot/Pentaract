import { ListItem, ListItemButton, ListItemIcon, ListItemText } from '@mui/material'
import { Link, useLocation } from 'react-router-dom'

export default function SideBarItem({ to, icon, label, showLabel }) {
  const location = useLocation()
  const selected = location.pathname.startsWith(to)

  return (
    <ListItem disablePadding>
      <ListItemButton component={Link} to={to} selected={selected}>
        <ListItemIcon>{icon}</ListItemIcon>
        {showLabel && <ListItemText primary={label} />}
      </ListItemButton>
    </ListItem>
  )
}
