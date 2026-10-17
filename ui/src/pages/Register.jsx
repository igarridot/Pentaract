import API from '../api'
import AuthForm from '../components/AuthForm'

export default function Register() {
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
