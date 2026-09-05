import { Box } from '@mui/material'

// Bordered surface used for lists and forms across the app.
export default function Panel({ children, sx }) {
  return (
    <Box sx={{
      bgcolor: 'background.paper',
      borderRadius: 3,
      border: '1px solid',
      borderColor: 'divider',
      overflow: 'hidden',
      ...sx,
    }}>
      {children}
    </Box>
  )
}
