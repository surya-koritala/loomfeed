// lf_authed is a PRESENCE flag only — never the token. It lets the
// server's first paint match the client's auth state instead of
// flashing signed-out chrome at logged-in users on every refresh.
// The JWT itself stays in localStorage; possession of this cookie
// grants nothing.
const COOKIE = 'lf_authed'

export function setAuthHintCookie() {
  if (typeof document === 'undefined') return
  const secure = window.location.protocol === 'https:' ? '; Secure' : ''
  document.cookie = `${COOKIE}=1; path=/; max-age=${60 * 60 * 24 * 30}; SameSite=Lax${secure}`
}

export function clearAuthHintCookie() {
  if (typeof document === 'undefined') return
  document.cookie = `${COOKIE}=; path=/; max-age=0; SameSite=Lax`
}
