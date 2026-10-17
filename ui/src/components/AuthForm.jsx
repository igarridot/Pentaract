import { useState } from 'react'
import { useNavigate, Link, Navigate } from 'react-router-dom'
import { TextField, Button, Typography, Stack, Link as MuiLink } from '@mui/material'
import { isAuthenticated, getRedirectPath } from '../common/auth_guard'
import AuthLayout from './AuthLayout'

// Email + password form shared by the login and register pages. onSubmit
// receives the credentials and resolves with the login response, whose
// access token is stored before redirecting into the app.
export default function AuthForm({ subtitle, submitLabel, footerText, footerLinkTo, footerLinkLabel, onSubmit }) {
  const navigate = useNavigate()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')

  if (isAuthenticated()) {
    return <Navigate to="/storages" replace />
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    try {
      const data = await onSubmit(email, password)
      localStorage.setItem('access_token', data.access_token)
      navigate(getRedirectPath(), { replace: true })
    } catch (err) {
      setError(err.message)
    }
  }

  return (
    <AuthLayout
      subtitle={subtitle}
      footer={
        <Typography variant="body2" color="text.secondary" sx={{ mt: 3 }}>
          {footerText}{' '}
          <MuiLink component={Link} to={footerLinkTo} sx={{ color: 'primary.main', textDecoration: 'none', fontWeight: 500 }}>
            {footerLinkLabel}
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
            {submitLabel}
          </Button>
        </Stack>
      </form>
    </AuthLayout>
  )
}
