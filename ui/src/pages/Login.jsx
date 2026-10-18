import API from '../api'
import AuthForm from '../components/AuthForm'
import { useAsyncValue } from '../common/use_async_value'

// Until the server says otherwise, sign-up is not offered.
const REGISTRATION_CLOSED = { enabled: false }

export default function Login() {
  const registration = useAsyncValue(API.users.registrationStatus, REGISTRATION_CLOSED)

  return (
    <AuthForm
      subtitle="Sign in to your account"
      submitLabel="Sign In"
      footerText="Don't have an account?"
      footerLinkTo={registration.enabled ? '/register' : null}
      footerLinkLabel="Create one"
      onSubmit={(email, password) => API.auth.login(email, password)}
    />
  )
}
