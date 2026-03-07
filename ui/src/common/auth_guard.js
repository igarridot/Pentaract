export function checkAuth(navigate, location) {
  const token = localStorage.getItem('access_token')
  if (!token) {
    localStorage.setItem('redirect', location.pathname)
    navigate('/login')
    return false
  }
  return true
}

export function isAuthenticated() {
  return !!localStorage.getItem('access_token')
}

export function logout(navigate) {
  localStorage.removeItem('access_token')
  navigate('/login')
}

export function getRedirectPath() {
  const path = localStorage.getItem('redirect')
  localStorage.removeItem('redirect')
  return path || '/storages'
}
