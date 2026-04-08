// portal/src/hooks/auth.ts
// OIDC session management hook.
//
// Security model (Blocker 1 fix):
//   The JWT is stored exclusively in an HttpOnly + Secure + SameSite=Strict
//   cookie set by the server at /auth/login and /auth/callback. JavaScript
//   CANNOT read HttpOnly cookies, so the token is never accessible to scripts
//   — eliminating the localStorage XSS attack surface entirely.
//
//   Authentication state is determined by calling GET /auth/me with
//   credentials:'include'. The browser sends the HttpOnly cookie automatically;
//   the portal never reads or stores the raw token value.
//
//   Silent refresh is scheduled 30 minutes before the token expiry using the
//   `exp` timestamp returned by /auth/me and /auth/refresh. The refresh call
//   also uses credentials:'include'; the server rotates the cookie transparently.

import { useEffect, useState, useCallback, useRef } from 'react'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

export interface AuthUser {
  sub: string
  email: string
  name: string
  role: string // admin | editor | viewer
}

interface AuthState {
  user: AuthUser | null
  isAuthenticated: boolean
  isLoading: boolean
  error: string | null
}

// Schedule a silent token refresh 30 minutes before the server-reported expiry.
// On success the server rotates the HttpOnly cookie and returns a new `exp`;
// we re-arm the timer for the new expiry. On a non-recoverable failure the
// session state is cleared so the user is prompted to log in again.
function useTokenRefreshScheduler() {
  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const setAuthState = useRef<((s: Partial<AuthState>) => void) | null>(null)

  const scheduleRefresh = useCallback((expUnix: number) => {
    if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current)

    // Fire 30 minutes before expiry; floor at 0 so we refresh immediately if
    // the token is already within the refresh window.
    const msUntilRefresh = Math.max(0, (expUnix - 30 * 60) * 1000 - Date.now())

    refreshTimerRef.current = setTimeout(async () => {
      try {
        const res = await fetch(`${BASE}/auth/refresh`, {
          method: 'POST',
          credentials: 'include', // sends af_token HttpOnly cookie
        })
        if (res.ok) {
          const { exp } = await res.json()
          if (exp) scheduleRefresh(exp) // re-arm for the new token's expiry
        } else {
          // Server rejected refresh (token expired or revoked) — force re-login.
          setAuthState.current?.({ user: null, isAuthenticated: false, isLoading: false, error: 'session_expired' })
        }
      } catch {
        // Network error — leave session intact; will be re-evaluated on next
        // page load or when the next refresh fires.
      }
    }, msUntilRefresh)
  }, [])

  const cancelRefresh = useCallback(() => {
    if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current)
  }, [])

  return { scheduleRefresh, cancelRefresh, setAuthState }
}

export function useAuth(): AuthState & {
  login: () => void
  logout: () => void
  refetch: () => void
} {
  const [state, setState] = useState<AuthState>({
    user: null,
    isAuthenticated: false,
    isLoading: true,
    error: null,
  })

  const { scheduleRefresh, cancelRefresh, setAuthState } = useTokenRefreshScheduler()

  // Wire the setState ref so the refresh scheduler can clear auth state on
  // session expiry without creating a circular dependency.
  useEffect(() => {
    setAuthState.current = (partial) => setState((prev) => ({ ...prev, ...partial }))
  })

  // Fetch identity from the server using the HttpOnly cookie.
  // GET /auth/me validates the cookie server-side and returns user claims + exp.
  const fetchUser = useCallback(async () => {
    try {
      const res = await fetch(`${BASE}/auth/me`, {
        credentials: 'include', // browser sends af_token HttpOnly cookie automatically
      })
      if (res.ok) {
        const data = await res.json()
        const user: AuthUser = {
          sub: data.sub,
          email: data.email,
          name: data.name,
          role: data.role,
        }
        setState({ user, isAuthenticated: true, isLoading: false, error: null })
        if (data.exp) scheduleRefresh(data.exp) // arm silent-refresh cycle
      } else {
        // 401 = no valid cookie (not logged in) — not an error state, just unauthenticated.
        setState({ user: null, isAuthenticated: false, isLoading: false, error: null })
      }
    } catch {
      setState({ user: null, isAuthenticated: false, isLoading: false, error: 'network_error' })
    }
  }, [scheduleRefresh])

  useEffect(() => {
    fetchUser()
    return cancelRefresh // cancel timer on unmount
  }, [fetchUser, cancelRefresh])

  const login = useCallback(() => {
    window.location.href = `${BASE}/auth/login`
  }, [])

  const logout = useCallback(() => {
    cancelRefresh()
    // Navigate to the logout endpoint; the server expires the HttpOnly cookie.
    window.location.href = `${BASE}/auth/logout`
  }, [cancelRefresh])

  return { ...state, login, logout, refetch: fetchUser }
}

// hasRole: returns true if user is authenticated AND their role is in allowedRoles.
// Pure function — exported so it can be unit-tested independently of React
// and used for inline element gating (e.g. show/hide buttons in UsersPage).
//
// Role hierarchy (most privileged first): admin > editor > viewer
// Always returns false for unauthenticated users (user === null).
//
// Comparison is case-insensitive on both sides (normalized to lowercase) to match
// the Go RequireRole middleware which uses strings.ToLower. DB roles are always
// stored lowercase, so this is a defensive guard against JWT claim casing drift.
export function hasRole(user: AuthUser | null, allowedRoles: string[]): boolean {
  if (!isAuthEnabled()) return true
  if (!user) return false
  const userRole = user.role.toLowerCase()
  return allowedRoles.map((r) => r.toLowerCase()).includes(userRole)
}

// isSelfOrRole: ABAC helper — returns true if the user holds one of allowedRoles
// OR if subjectId matches user.sub (the user is acting on their own record).
// Mirrors the RequireRoleOrSelf middleware on the api-gateway.
export function isSelfOrRole(
  user: AuthUser | null,
  allowedRoles: string[],
  subjectId: string,
): boolean {
  if (!isAuthEnabled()) return true
  if (!user) return false
  const userRole = user.role.toLowerCase()
  return allowedRoles.map((r) => r.toLowerCase()).includes(userRole) || user.sub === subjectId
}

// isAuthEnabled: returns true unless VITE_AUTH_DISABLED=true (dev-only bypass).
export function isAuthEnabled(): boolean {
  return import.meta.env.VITE_AUTH_DISABLED !== 'true'
}
