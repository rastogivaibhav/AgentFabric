// portal/src/pages/LoginPage.tsx
// Enterprise SSO login page — redirects to /auth/login (OIDC Authorization Code + PKCE)

import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Zap, Shield, Lock } from 'lucide-react'
import { useAuth, isAuthEnabled } from '../hooks/auth'

export default function LoginPage() {
  const { isAuthenticated, isLoading, login } = useAuth()
  const navigate = useNavigate()

  // If already authenticated, bounce to dashboard
  useEffect(() => {
    if (!isLoading && isAuthenticated) {
      navigate('/dashboard', { replace: true })
    }
  }, [isAuthenticated, isLoading, navigate])

  // If auth is disabled in dev, redirect immediately
  useEffect(() => {
    if (!isAuthEnabled()) {
      navigate('/dashboard', { replace: true })
    }
  }, [navigate])

  if (isLoading) {
    return (
      <div style={styles.root}>
        <div style={styles.spinner} />
      </div>
    )
  }

  return (
    <div style={styles.root}>
      <div style={styles.card}>
        {/* Logo */}
        <div style={styles.logo}>
          <div style={styles.logoIcon}>
            <Zap size={28} color="#fff" />
          </div>
          <div>
            <div style={styles.logoName}>AgentFabric</div>
            <div style={styles.logoSub}>OBSERVABILITY PLATFORM</div>
          </div>
        </div>

        {/* Heading */}
        <h1 style={styles.heading}>Sign in to your workspace</h1>
        <p style={styles.subheading}>
          Use your organization's SSO provider to access the AgentFabric portal.
        </p>

        {/* SSO Button */}
        <button style={styles.ssoButton} onClick={login}>
          <Shield size={18} />
          <span>Continue with SSO</span>
        </button>

        {/* Feature list */}
        <div style={styles.features}>
          {[
            { icon: Shield,  text: 'Enterprise OIDC (Okta, Azure AD, Auth0)' },
            { icon: Lock,    text: 'PKCE — no tokens in URLs' },
            { icon: Zap,     text: 'Automatic session renewal' },
          ].map(({ icon: Icon, text }) => (
            <div key={text} style={styles.featureItem}>
              <Icon size={13} color="#3B82F6" />
              <span style={styles.featureText}>{text}</span>
            </div>
          ))}
        </div>

        <div style={styles.footer}>
          v1.0.0 · © AgentFabric
        </div>
      </div>
    </div>
  )
}

const styles: Record<string, React.CSSProperties> = {
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
  logo: {
    display: 'flex',
    alignItems: 'center',
    gap: 14,
    marginBottom: 4,
  },
  logoIcon: {
    width: 48,
    height: 48,
    background: 'linear-gradient(135deg,#3B82F6,#8B5CF6)',
    borderRadius: 10,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  logoName: {
    fontSize: 20,
    fontWeight: 700,
    color: '#F0F9FF',
    letterSpacing: '0.05em',
  },
  logoSub: {
    fontSize: 9,
    color: '#334155',
    letterSpacing: '0.15em',
  },
  heading: {
    fontSize: 18,
    fontWeight: 600,
    color: '#E2E8F0',
    margin: 0,
  },
  subheading: {
    fontSize: 12,
    color: '#475569',
    margin: 0,
    lineHeight: 1.6,
  },
  ssoButton: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 10,
    background: 'linear-gradient(90deg,#1E3A5F,#1e40af)',
    border: '1px solid #3B82F6',
    borderRadius: 8,
    color: '#93C5FD',
    fontSize: 13,
    fontFamily: "'JetBrains Mono', monospace",
    fontWeight: 600,
    padding: '12px 24px',
    cursor: 'pointer',
    transition: 'all 0.15s',
    letterSpacing: '0.05em',
  },
  features: {
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    paddingTop: 8,
    borderTop: '1px solid #0F1F35',
  },
  featureItem: {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
  },
  featureText: {
    fontSize: 11,
    color: '#475569',
  },
  footer: {
    fontSize: 10,
    color: '#1E3A5F',
    textAlign: 'center',
    paddingTop: 8,
  },
  spinner: {
    width: 24,
    height: 24,
    border: '2px solid #0F1F35',
    borderTop: '2px solid #3B82F6',
    borderRadius: '50%',
    animation: 'spin 0.8s linear infinite',
  },
}
