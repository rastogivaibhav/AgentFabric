// portal/src/pages/LoginPage.tsx
// Login page — username/password (default) with optional SSO when OIDC is configured.

import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Zap, Shield, Lock, AlertCircle } from 'lucide-react'
import { useAuth, isAuthEnabled } from '../hooks/auth'

const BASE = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'
const SSO_ENABLED = import.meta.env.VITE_SSO_ENABLED === 'true'

export default function LoginPage() {
  const { isAuthenticated, isLoading, login } = useAuth()
  const navigate = useNavigate()

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  // Already authenticated → go to dashboard
  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      navigate('/dashboard', { replace: true })
    }
  }, [isAuthenticated, isLoading, navigate])

  // Auth fully disabled (e2e / CI) → skip login entirely
  useEffect(() => {
    if (!isAuthEnabled()) {
      navigate('/dashboard', { replace: true })
    }
  }, [navigate])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!username.trim() || !password.trim()) {
      setError('Username and password are required.')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      const res = await fetch(`${BASE}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include', // required for cross-origin HttpOnly cookie to be set
        body: JSON.stringify({ username: username.trim(), password: password.trim() }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        setError(body.error ?? 'Invalid credentials.')
        return
      }
      // Token is set as an HttpOnly cookie by the server (XSS-safe).
      // We do NOT store it in localStorage — reading an HttpOnly cookie
      // from JS is intentionally impossible.
      await res.json() // consume body; token arrives via Set-Cookie header
      navigate('/dashboard', { replace: true })
    } catch {
      setError('Could not reach the server. Make sure the API gateway is running.')
    } finally {
      setSubmitting(false)
    }
  }

  if (isLoading) {
    return (
      <div style={s.root}>
        <div style={s.spinner} />
      </div>
    )
  }

  return (
    <div style={s.root}>
      <div style={s.card}>
        {/* Logo */}
        <div style={s.logo}>
          <div style={s.logoIcon}>
            <Zap size={28} color="#fff" />
          </div>
          <div>
            <div style={s.logoName}>AgentFabric</div>
            <div style={s.logoSub}>OBSERVABILITY PLATFORM</div>
          </div>
        </div>

        <h1 style={s.heading}>Sign in</h1>

        {/* Username / password form */}
        <form onSubmit={handleSubmit} style={s.form}>
          <div style={s.field}>
            <label style={s.label} htmlFor="af-username">Username</label>
            <input
              id="af-username"
              type="text"
              autoComplete="username"
              value={username}
              onChange={e => setUsername(e.target.value)}
              placeholder="admin"
              style={s.input}
              disabled={submitting}
            />
          </div>
          <div style={s.field}>
            <label style={s.label} htmlFor="af-password">Password</label>
            <input
              id="af-password"
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder="••••••••"
              style={s.input}
              disabled={submitting}
            />
          </div>

          {error && (
            <div style={s.errorBox}>
              <AlertCircle size={13} color="#EF4444" />
              <span>{error}</span>
            </div>
          )}

          <button type="submit" style={s.submitBtn} disabled={submitting}>
            {submitting ? 'Signing in…' : 'Sign in'}
          </button>
        </form>

        {/* SSO — only shown when explicitly enabled */}
        {SSO_ENABLED && (
          <>
            <div style={s.divider}><span style={s.dividerText}>or</span></div>
            <button style={s.ssoButton} onClick={login} type="button">
              <Shield size={16} />
              <span>Continue with SSO</span>
            </button>
          </>
        )}

        {/* Hints — default-creds line is suppressed in production builds.
             Set VITE_SHOW_DEFAULT_CREDS=true in .env.development only. */}
        <div style={s.hints}>
          {import.meta.env.VITE_SHOW_DEFAULT_CREDS === 'true' && (
            <div style={s.hint}>
              <Lock size={11} color="#334155" />
              <span style={s.hintText}>Default credentials: admin / admin</span>
            </div>
          )}
          <div style={s.hint}>
            <Shield size={11} color="#334155" />
            <span style={s.hintText}>Set AF_ADMIN_USER + AF_ADMIN_PASSWORD to change</span>
          </div>
        </div>

        <div style={s.footer}>v1.0.0 · © AgentFabric</div>
      </div>
    </div>
  )
}

const s: Record<string, React.CSSProperties> = {
  root: {
    minHeight: '100vh',
    background: '#080C18',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontFamily: "'JetBrains Mono', monospace",
  },
  card: {
    background: '#060A14',
    border: '1px solid #0F1F35',
    borderRadius: 12,
    padding: '40px 48px',
    width: 420,
    display: 'flex',
    flexDirection: 'column',
    gap: 20,
  },
  logo: { display: 'flex', alignItems: 'center', gap: 14, marginBottom: 4 },
  logoIcon: {
    width: 48, height: 48,
    background: 'linear-gradient(135deg,#3B82F6,#8B5CF6)',
    borderRadius: 10,
    display: 'flex', alignItems: 'center', justifyContent: 'center',
  },
  logoName: { fontSize: 20, fontWeight: 700, color: '#F0F9FF', letterSpacing: '0.05em' },
  logoSub:  { fontSize: 9, color: '#334155', letterSpacing: '0.15em' },
  heading:  { fontSize: 18, fontWeight: 600, color: '#E2E8F0', margin: 0 },
  form:     { display: 'flex', flexDirection: 'column', gap: 14 },
  field:    { display: 'flex', flexDirection: 'column', gap: 6 },
  label:    { fontSize: 11, color: '#64748B', letterSpacing: '0.08em' },
  input: {
    background: '#0D1B2A',
    border: '1px solid #1E3A5F',
    borderRadius: 6,
    padding: '10px 14px',
    color: '#E2E8F0',
    fontSize: 13,
    fontFamily: "'JetBrains Mono', monospace",
    outline: 'none',
  },
  errorBox: {
    display: 'flex', alignItems: 'center', gap: 8,
    background: '#EF444415',
    border: '1px solid #EF444430',
    borderRadius: 6,
    padding: '8px 12px',
    fontSize: 11,
    color: '#EF4444',
  },
  submitBtn: {
    background: 'linear-gradient(90deg,#1E3A5F,#1e40af)',
    border: '1px solid #3B82F6',
    borderRadius: 8,
    color: '#93C5FD',
    fontSize: 13,
    fontFamily: "'JetBrains Mono', monospace",
    fontWeight: 600,
    padding: '12px 24px',
    cursor: 'pointer',
    letterSpacing: '0.05em',
    marginTop: 4,
  },
  divider: {
    display: 'flex', alignItems: 'center',
    borderTop: '1px solid #0F1F35',
    position: 'relative',
    margin: '4px 0',
  },
  dividerText: {
    position: 'absolute', left: '50%', transform: 'translateX(-50%)',
    background: '#060A14', padding: '0 10px',
    fontSize: 10, color: '#334155',
  },
  ssoButton: {
    display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 10,
    background: 'transparent',
    border: '1px solid #1E3A5F',
    borderRadius: 8, color: '#475569',
    fontSize: 12, fontFamily: "'JetBrains Mono', monospace",
    padding: '10px 24px', cursor: 'pointer',
  },
  hints: {
    display: 'flex', flexDirection: 'column', gap: 6,
    paddingTop: 8, borderTop: '1px solid #0F1F35',
  },
  hint:     { display: 'flex', alignItems: 'center', gap: 8 },
  hintText: { fontSize: 10, color: '#334155' },
  footer:   { fontSize: 10, color: '#1E3A5F', textAlign: 'center', paddingTop: 4 },
  spinner: {
    width: 24, height: 24,
    border: '2px solid #0F1F35',
    borderTop: '2px solid #3B82F6',
    borderRadius: '50%',
    animation: 'spin 0.8s linear infinite',
  },
}
