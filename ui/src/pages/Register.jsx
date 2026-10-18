import { Link } from 'react-router-dom'
import { Typography, Button } from '@mui/material'
import API from '../api'
import AuthForm from '../components/AuthForm'
import AuthLayout from '../components/AuthLayout'
import { useAsyncValue } from '../common/use_async_value'

// Unknown (still loading, or the check failed) shows the form: the server
// refuses the sign-up anyway when registration is closed.
const REGISTRATION_UNKNOWN = { enabled: null }

export default function Register() {
  const registration = useAsyncValue(API.users.registrationStatus, REGISTRATION_UNKNOWN)

  if (registration.enabled === false) {
    return (
      <AuthLayout subtitle="Registration is disabled">
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          Accounts are created by the administrator. Ask them for one.
        </Typography>
        <Button fullWidth variant="contained" size="large" component={Link} to="/login">
          Sign in
        </Button>
      </AuthLayout>
    )
  }

  return (
    <AuthForm
      subtitle="Create your account"
      submitLabel="Create Account"
      footerText="Already have an account?"
      footerLinkTo="/login"
      footerLinkLabel="Sign in"
      onSubmit={async (email, password) => {
        await API.users.register(email, password)
        return API.auth.login(email, password)
      }}
    />
  )
}
