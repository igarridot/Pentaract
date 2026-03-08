import { useState } from 'react'
import { useNavigate, Link } from 'react-router-dom'
import { TextField, Button, Typography, Stack, Link as MuiLink } from '@mui/material'
import API from '../api'
import { isAuthenticated, getRedirectPath } from '../common/auth_guard'
import AuthLayout from '../components/AuthLayout'

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
    <AuthLayout
      subtitle="Create your account"
      footer={
        <Typography variant="body2" color="text.secondary" sx={{ mt: 3 }}>
          Already have an account?{' '}
          <MuiLink component={Link} to="/login" sx={{ color: 'primary.main', textDecoration: 'none', fontWeight: 500 }}>
            Sign in
          </MuiLink>
        </Typography>
      }
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
    </AuthLayout>
  )
}
