import { Box, Container, Typography } from '@mui/material'
import AppIcon from './AppIcon'

export default function AuthLayout({ subtitle, children, footer }) {
  return (
    <Box sx={{
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      bgcolor: 'background.default',
    }}>
      <Container maxWidth="xs">
        <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <AppIcon sx={{ fontSize: 56, color: 'primary.main', mb: 1 }} />
          <Typography variant="h4" sx={{ mb: 0.5 }}>Pentaract</Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 4 }}>
            {subtitle}
          </Typography>
          <Box
            sx={{
              width: '100%',
              bgcolor: 'background.paper',
              borderRadius: 4,
              p: 4,
              boxShadow: 2,
              border: '1px solid',
              borderColor: 'divider',
            }}
          >
            {children}
          </Box>
          {footer}
        </Box>
      </Container>
    </Box>
  )
}
