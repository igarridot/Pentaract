import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { Box, Container, Paper, TextField, Button, Typography, Stack } from '@mui/material'
import API from '../api'
import { isAuthenticated, getRedirectPath } from '../common/auth_guard'

export default function Login() {
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
      const data = await API.auth.login(email, password)
      localStorage.setItem('access_token', data.access_token)
      navigate(getRedirectPath(), { replace: true })
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <Container maxWidth="xs">
      <Box sx={{ mt: 8, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
        <Typography variant="h4" gutterBottom>Pentaract</Typography>
        <Paper sx={{ p: 3, width: '100%' }}>
          <form onSubmit={handleSubmit}>
            <Stack spacing={2}>
              <TextField
                fullWidth label="Email" type="email" value={email}
                onChange={(e) => setEmail(e.target.value)} required
              />
              <TextField
                fullWidth label="Password" type="password" value={password}
                onChange={(e) => setPassword(e.target.value)} required
              />
              {error && <Typography color="error" variant="body2">{error}</Typography>}
              <Button fullWidth variant="contained" type="submit">Log in</Button>
              <Typography variant="body2" align="center">
                Don't have an account? <Link to="/register">Register</Link>
              </Typography>
            </Stack>
          </form>
        </Paper>
      </Box>
    </Container>
  )
}
