// portal/src/hooks/auth.ts
// OIDC session management hook.
// Reads the af_token from the cookie set by /auth/callback, falls back to
// localStorage for dev environments where cookies aren't set.

import { useEffect, useState, useCallback } from 'react'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

export interface AuthUser {
  sub: string
  email: string
  name: string
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
      } else {
        // Token is invalid or expired — clear it
        localStorage.removeItem('af_token')
        setState({ user: null, isAuthenticated: false, isLoading: false, error: 'session_expired' })
      }
    } catch {
      setState({ user: null, isAuthenticated: false, isLoading: false, error: 'network_error' })
    }
  }, [])

  useEffect(() => {
    fetchUser()
  }, [fetchUser])

  const login = useCallback(() => {
    window.location.href = `${BASE}/auth/login`
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem('af_token')
    // Clear cookie by navigating to logout endpoint which expires it
    window.location.href = `${BASE}/auth/logout`
  }, [])

  return { ...state, login, logout, refetch: fetchUser }
}

// RequireAuth: returns true if the user is authenticated.
// Used by the route guard in App.tsx.
export function isAuthEnabled(): boolean {
  return import.meta.env.VITE_AUTH_DISABLED !== 'true'
}
