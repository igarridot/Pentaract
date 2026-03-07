import { Box, Typography, Button } from '@mui/material'
import { Link } from 'react-router-dom'

export default function NotFound() {
  return (
    <Box sx={{ textAlign: 'center', mt: 12 }}>
      <Typography sx={{ fontSize: '5rem', fontWeight: 700, color: 'text.secondary', opacity: 0.3, lineHeight: 1 }}>
        404
      </Typography>
      <Typography variant="h6" color="text.secondary" sx={{ mt: 1, mb: 3 }}>
        This page doesn't exist
      </Typography>
      <Button component={Link} to="/storages" variant="contained">
        Go to Storages
      </Button>
    </Box>
  )
}
