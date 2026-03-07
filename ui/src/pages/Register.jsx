import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { Box, Container, TextField, Button, Typography, Stack, Link as MuiLink } from '@mui/material'
import API from '../api'
import { isAuthenticated, getRedirectPath } from '../common/auth_guard'
import AppIcon from '../components/AppIcon'

export default function Register() {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  if (isAuthenticated()) {
    navigate('/storages', { replace: true })
    return null
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    try {
      await API.users.register(email, password)
      const data = await API.auth.login(email, password)
      localStorage.setItem('access_token', data.access_token)
      navigate(getRedirectPath(), { replace: true })
    } catch (err) {
      setError(err.message)
    }
  }

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
            Create your account
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
            <form onSubmit={handleSubmit}>
              <Stack spacing={2.5}>
                <TextField
                  fullWidth label="Email" type="email" value={email}
                  onChange={(e) => setEmail(e.target.value)} required
                />
                <TextField
                  fullWidth label="Password" type="password" value={password}
                  onChange={(e) => setPassword(e.target.value)} required
                />
                {error && (
                  <Typography color="error" variant="body2" sx={{ fontSize: '0.8rem' }}>
                    {error}
                  </Typography>
                )}
                <Button fullWidth variant="contained" type="submit" size="large">
                  Create Account
                </Button>
              </Stack>
            </form>
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 3 }}>
            Already have an account?{' '}
            <MuiLink component={Link} to="/login" sx={{ color: 'primary.main', textDecoration: 'none', fontWeight: 500 }}>
              Sign in
            </MuiLink>
          </Typography>
        </Box>
      </Container>
    </Box>
  )
}
