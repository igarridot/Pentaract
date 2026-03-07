import { Box, Typography, Button } from '@mui/material'
import { Link } from 'react-router-dom'

export default function NotFound() {
  return (
    <Box sx={{ textAlign: 'center', mt: 8 }}>
      <Typography variant="h2" gutterBottom>404</Typography>
      <Typography variant="h6" gutterBottom>Page not found</Typography>
      <Button component={Link} to="/storages" variant="contained">
        Go to Storages
      </Button>
    </Box>
  )
}
