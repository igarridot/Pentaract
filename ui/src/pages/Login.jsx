import API from '../api'
import AuthForm from '../components/AuthForm'

export default function Login() {
  return (
    <AuthForm
      subtitle="Sign in to your account"
      submitLabel="Sign In"
      footerText="Don't have an account?"
      footerLinkTo="/register"
      footerLinkLabel="Create one"
      onSubmit={(email, password) => API.auth.login(email, password)}
    />
  )
}
