// portal/src/hooks/auth.test.ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { getToken, isAuthEnabled, getTokenExpiry, hasRole, isSelfOrRole, AuthUser } from './auth'

// ─── Fixture helpers ──────────────────────────────────────────────────────────

function makeUser(role: string, sub = 'user-123'): AuthUser {
  return { sub, email: `${role}@test.com`, name: role, role }
}

// ─── getToken ────────────────────────────────────────────────────────────────

describe('getToken', () => {
  beforeEach(() => {
    // Reset state between tests
    localStorage.clear()
    // Reset document.cookie (jsdom allows this)
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: '',
    })
  })

  it('returns empty string when no token exists', () => {
    expect(getToken()).toBe('')
  })

  it('reads af_token from localStorage as fallback', () => {
    localStorage.setItem('af_token', 'test-jwt-from-localstorage')
    expect(getToken()).toBe('test-jwt-from-localstorage')
  })

  it('prefers cookie over localStorage', () => {
    localStorage.setItem('af_token', 'localstorage-token')
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: 'af_token=cookie-token; other_cookie=value',
    })
    expect(getToken()).toBe('cookie-token')
  })

  it('handles multiple cookies and finds the right one', () => {
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: 'session=abc; af_token=correct-token; other=xyz',
    })
    expect(getToken()).toBe('correct-token')
  })

  it('returns empty string when cookie name is similar but not exact', () => {
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: 'af_token_other=wrong-token',
    })
    localStorage.clear()
    // The cookie name doesn't match 'af_token', localStorage also empty
    expect(getToken()).toBe('')
  })
})

// ─── isAuthEnabled ───────────────────────────────────────────────────────────

describe('isAuthEnabled', () => {
  afterEach(() => {
    vi.unstubAllEnvs()
  })

  it('returns true when VITE_AUTH_DISABLED is not set', () => {
    vi.stubEnv('VITE_AUTH_DISABLED', '')
    expect(isAuthEnabled()).toBe(true)
  })

  it('returns false when VITE_AUTH_DISABLED is "true"', () => {
    vi.stubEnv('VITE_AUTH_DISABLED', 'true')
    expect(isAuthEnabled()).toBe(false)
  })

  it('returns true when VITE_AUTH_DISABLED is "false"', () => {
    vi.stubEnv('VITE_AUTH_DISABLED', 'false')
    expect(isAuthEnabled()).toBe(true)
  })
})

// ─── getTokenExpiry ──────────────────────────────────────────────────────────

// Build a minimal fake JWT (header.payload.sig) — signature is not validated here.
function makeToken(payload: Record<string, unknown>): string {
  const encode = (obj: object) =>
    btoa(JSON.stringify(obj)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '')
  return `${encode({ alg: 'HS256', typ: 'JWT' })}.${encode(payload)}.fakesig`
}

describe('getTokenExpiry', () => {
  it('returns null for an empty string', () => {
    expect(getTokenExpiry('')).toBeNull()
  })

  it('returns null for a plain non-JWT string', () => {
    expect(getTokenExpiry('not-a-token')).toBeNull()
  })

  it('returns null for a JWT without an exp claim', () => {
    const token = makeToken({ sub: 'alice', role: 'admin' })
    expect(getTokenExpiry(token)).toBeNull()
  })

  it('returns the exp unix timestamp for a valid JWT payload', () => {
    const exp = Math.floor(Date.now() / 1000) + 3600 // 1 hour from now
    const token = makeToken({ sub: 'alice', exp })
    expect(getTokenExpiry(token)).toBe(exp)
  })

  it('returns exp even when the token is already past its expiry', () => {
    const exp = Math.floor(Date.now() / 1000) - 100 // 100 s in the past
    const token = makeToken({ sub: 'alice', exp })
    expect(getTokenExpiry(token)).toBe(exp)
  })

  it('returns null when the payload segment is invalid base64', () => {
    expect(getTokenExpiry('header.!!!invalid.sig')).toBeNull()
  })

  it('returns null when payload is valid base64 but not JSON', () => {
    const bad = btoa('not json').replace(/=/g, '')
    expect(getTokenExpiry(`header.${bad}.sig`)).toBeNull()
  })
})

// ─── hasRole ─────────────────────────────────────────────────────────────────

describe('hasRole', () => {
  it('returns false for unauthenticated user (null)', () => {
    expect(hasRole(null, ['admin'])).toBe(false)
  })

  it('returns true when user role is in allowedRoles', () => {
    expect(hasRole(makeUser('admin'), ['admin'])).toBe(true)
  })

  it('returns true when role matches one of multiple allowed roles', () => {
    expect(hasRole(makeUser('editor'), ['admin', 'editor'])).toBe(true)
  })

  it('returns false when user role is not in allowedRoles', () => {
    expect(hasRole(makeUser('viewer'), ['admin'])).toBe(false)
  })

  it('returns false when user role is not in any of the allowed roles', () => {
    expect(hasRole(makeUser('viewer'), ['admin', 'editor'])).toBe(false)
  })

  it('returns true when allowedRoles contains the exact role string', () => {
    expect(hasRole(makeUser('admin'), ['admin', 'superuser'])).toBe(true)
  })

  it('returns false for an empty allowedRoles list', () => {
    expect(hasRole(makeUser('admin'), [])).toBe(false)
  })

  it('is case-sensitive — "Admin" !== "admin"', () => {
    expect(hasRole(makeUser('Admin'), ['admin'])).toBe(false)
  })
})

// ─── isSelfOrRole ─────────────────────────────────────────────────────────────

describe('isSelfOrRole', () => {
  const ADMIN_ID   = 'admin-001'
  const VIEWER_ID  = 'viewer-002'
  const OTHER_ID   = 'other-003'

  it('returns false for unauthenticated user (null)', () => {
    expect(isSelfOrRole(null, ['admin'], ADMIN_ID)).toBe(false)
  })

  it('returns true when user holds an allowed role', () => {
    const admin = makeUser('admin', ADMIN_ID)
    expect(isSelfOrRole(admin, ['admin'], OTHER_ID)).toBe(true)
  })

  it('returns true when user.sub matches subjectId (self-service)', () => {
    const viewer = makeUser('viewer', VIEWER_ID)
    expect(isSelfOrRole(viewer, ['admin'], VIEWER_ID)).toBe(true)
  })

  it('returns false when user lacks role AND is not the subject', () => {
    const viewer = makeUser('viewer', VIEWER_ID)
    expect(isSelfOrRole(viewer, ['admin'], OTHER_ID)).toBe(false)
  })

  it('editor can edit their own record even though editor is not in allowedRoles', () => {
    const editor = makeUser('editor', 'editor-id')
    expect(isSelfOrRole(editor, ['admin'], 'editor-id')).toBe(true)
  })

  it('editor cannot edit another user\'s record without the admin role', () => {
    const editor = makeUser('editor', 'editor-id')
    expect(isSelfOrRole(editor, ['admin'], 'other-user-id')).toBe(false)
  })
})

// ─── RequireRole rendering contract (behavioural spec via hasRole) ────────────
// RequireRole wraps hasRole inside a React component.  We test the gating logic
// here as pure-function tests; component render tests live in UsersPage.test.tsx.

describe('RequireRole gating logic (via hasRole)', () => {
  it('admin can see admin-only element', () => {
    expect(hasRole(makeUser('admin'), ['admin'])).toBe(true)
  })

  it('viewer cannot see admin-only element', () => {
    expect(hasRole(makeUser('viewer'), ['admin'])).toBe(false)
  })

  it('editor cannot see admin-only element', () => {
    expect(hasRole(makeUser('editor'), ['admin'])).toBe(false)
  })

  it('editor CAN see editor-or-admin element', () => {
    expect(hasRole(makeUser('editor'), ['admin', 'editor'])).toBe(true)
  })

  it('unauthenticated user sees nothing (null guard)', () => {
    expect(hasRole(null, ['admin', 'editor', 'viewer'])).toBe(false)
  })
})

// ─── Token shape ─────────────────────────────────────────────────────────────

describe('JWT token shape validation', () => {
  it('a valid JWT has three dot-separated parts', () => {
    const token = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyMSIsInRlbmFudF9pZCI6InRlbmFudC1hYmMifQ.SIG'
    const parts = token.split('.')
    expect(parts.length).toBe(3)
  })

  it('getToken returns the full token including all three parts', () => {
    const jwt = 'header.payload.signature'
    localStorage.setItem('af_token', jwt)
    expect(getToken()).toBe(jwt)
    expect(getToken().split('.')).toHaveLength(3)
  })

  it('empty getToken does not produce a valid JWT', () => {
    localStorage.clear()
    const token = getToken()
    expect(token).toBe('')
    expect(token.split('.').length).toBe(1)
  })
})
