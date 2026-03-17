// portal/src/hooks/auth.ts
// OIDC session management hook.
// Reads the af_token from the cookie set by /auth/callback, falls back to
// localStorage for dev environments where cookies aren't set.
// Schedules a silent token refresh 30 minutes before expiry so the session
// never interrupts an active user.

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

// Read af_token from cookie (set by OIDC callback) or localStorage (dev fallback).
export function getToken(): string {
  // Try cookie first (production OIDC flow)
  const cookies = document.cookie.split(';')
  for (const cookie of cookies) {
    const [name, value] = cookie.trim().split('=')
    if (name === 'af_token') return decodeURIComponent(value ?? '')
  }
  // Fall back to localStorage (dev / manual token injection)
  return localStorage.getItem('af_token') ?? ''
}

// Decode the `exp` claim from a JWT payload without signature verification.
// Returns the expiry Unix timestamp in seconds, or null if the token is malformed
// or carries no `exp` claim.  Safe to call on any string.
export function getTokenExpiry(token: string): number | null {
  try {
    const parts = token.split('.')
    if (parts.length !== 3) return null
    // Normalise base64url → standard base64, then pad to a multiple of 4.
    const b64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = b64.padEnd(b64.length + (4 - (b64.length % 4)) % 4, '=')
    const payload = JSON.parse(atob(padded))
    return typeof payload.exp === 'number' ? payload.exp : null
  } catch {
    return null
  }
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

  const refreshTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Schedule a silent token refresh 30 minutes before the token expires.
  // On success the new token is persisted to localStorage and the timer is
  // rescheduled for the new token's expiry.  On a non-recoverable failure the
  // session is cleared so the user is prompted to log in again.
  const scheduleRefresh = useCallback((token: string) => {
    if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current)

    const exp = getTokenExpiry(token)
    if (!exp) return // token has no expiry — nothing to schedule

    const msUntilRefresh = Math.max(0, (exp - 30 * 60) * 1000 - Date.now())

    refreshTimerRef.current = setTimeout(async () => {
      const currentToken = getToken()
      if (!currentToken) return

      try {
        const res = await fetch(`${BASE}/auth/refresh`, {
          method: 'POST',
          headers: { Authorization: `Bearer ${currentToken}` },
        })
        if (res.ok) {
          const { token: newToken } = await res.json()
          localStorage.setItem('af_token', newToken)
          scheduleRefresh(newToken) // arm the timer for the fresh token
        } else {
          // Server rejected refresh — clear session and ask user to re-login
          localStorage.removeItem('af_token')
          setState({ user: null, isAuthenticated: false, isLoading: false, error: 'session_expired' })
        }
      } catch {
        // Network error — don't kill the session; it will be re-evaluated on
        // next page load.  Timer is already cleared so no double-fire.
      }
    }, msUntilRefresh)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const fetchUser = useCallback(async () => {
    const token = getToken()
    if (!token) {
      setState({ user: null, isAuthenticated: false, isLoading: false, error: null })
      return
    }

    try {
      const res = await fetch(`${BASE}/auth/me`, {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (res.ok) {
        const user: AuthUser = await res.json()
        setState({ user, isAuthenticated: true, isLoading: false, error: null })
        scheduleRefresh(token) // start silent-refresh cycle
      } else {
        // Token is invalid or expired — clear it
        localStorage.removeItem('af_token')
        setState({ user: null, isAuthenticated: false, isLoading: false, error: 'session_expired' })
      }
    } catch {
      setState({ user: null, isAuthenticated: false, isLoading: false, error: 'network_error' })
    }
  }, [scheduleRefresh])

  useEffect(() => {
    fetchUser()
    return () => {
      // Cancel any pending refresh timer when the component unmounts
      if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current)
    }
  }, [fetchUser])

  const login = useCallback(() => {
    window.location.href = `${BASE}/auth/login`
  }, [])

  const logout = useCallback(() => {
    if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current)
    localStorage.removeItem('af_token')
    // Clear cookie by navigating to logout endpoint which expires it
    window.location.href = `${BASE}/auth/logout`
  }, [])

  return { ...state, login, logout, refetch: fetchUser }
}

// hasRole: returns true if user is authenticated AND their role is in allowedRoles.
// Pure function — exported so it can be unit-tested independently of React
// and used for inline element gating (e.g. show/hide buttons in UsersPage).
//
// Role hierarchy (most privileged first): admin > editor > viewer
// Always returns false for unauthenticated users (user === null).
export function hasRole(user: AuthUser | null, allowedRoles: string[]): boolean {
  if (!user) return false
  return allowedRoles.includes(user.role)
}

// isSelfOrRole: ABAC helper — returns true if the user holds one of allowedRoles
// OR if subjectId matches user.sub (the user is acting on their own record).
// Mirrors the RequireRoleOrSelf middleware on the api-gateway.
export function isSelfOrRole(
  user: AuthUser | null,
  allowedRoles: string[],
  subjectId: string,
): boolean {
  if (!user) return false
  return allowedRoles.includes(user.role) || user.sub === subjectId
}

// RequireAuth: returns true if the user is authenticated.
// Used by the route guard in App.tsx.
export function isAuthEnabled(): boolean {
  return import.meta.env.VITE_AUTH_DISABLED !== 'true'
}
