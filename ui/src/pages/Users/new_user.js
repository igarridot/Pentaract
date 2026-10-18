// newUserError checks the Add user form before it is sent, and returns what
// is wrong with it, or null. The server checks again: this only saves a
// round trip on obvious mistakes.
export function newUserError({ email, password, confirm }) {
  if (!email.trim()) return 'Email is required'
  if (!password) return 'Password is required'
  if (password !== confirm) return 'Passwords do not match'
  return null
}
