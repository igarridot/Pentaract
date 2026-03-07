import { ListItem, ListItemButton, ListItemIcon, ListItemText } from '@mui/material'
import { Link, useLocation } from 'react-router-dom'

export default function SideBarItem({ to, icon, label, showLabel }) {
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
