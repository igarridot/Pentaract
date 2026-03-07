import { useState } from 'react'
import { Fab, Menu as MuiMenu } from '@mui/material'
import { Add as AddIcon } from '@mui/icons-material'

export default function FloatingMenu({ children }) {
  const [anchorEl, setAnchorEl] = useState(null)

  return (
    <>
      <Fab
        color="secondary"
        onClick={(e) => setAnchorEl(e.currentTarget)}
        sx={{ position: 'fixed', bottom: 24, right: 24 }}
      >
        <AddIcon />
      </Fab>
      <MuiMenu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={() => setAnchorEl(null)}>
        {children(() => setAnchorEl(null))}
      </MuiMenu>
    </>
  )
}
