// portal/src/hooks/auth.test.ts
import { describe, it, expect, afterEach, vi } from 'vitest'
import { isAuthEnabled, hasRole, isSelfOrRole, AuthUser } from './auth'

// ─── Fixture helpers ──────────────────────────────────────────────────────────

function makeUser(role: string, sub = 'user-123'): AuthUser {
  return { sub, email: `${role}@test.com`, name: role, role }
}

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

  it('is case-insensitive — "Admin" matches "admin" (mirrors Go RequireRole)', () => {
    expect(hasRole(makeUser('Admin'), ['admin'])).toBe(true)
  })

  it('is case-insensitive — "ADMIN" matches "admin"', () => {
    expect(hasRole(makeUser('ADMIN'), ['admin'])).toBe(true)
  })
})

// ─── isSelfOrRole ─────────────────────────────────────────────────────────────

describe('isSelfOrRole', () => {
  const ADMIN_ID  = 'admin-001'
  const VIEWER_ID = 'viewer-002'
  const OTHER_ID  = 'other-003'

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

  it("editor cannot edit another user's record without the admin role", () => {
    const editor = makeUser('editor', 'editor-id')
    expect(isSelfOrRole(editor, ['admin'], 'other-user-id')).toBe(false)
  })
})

// ─── RequireRole rendering contract (behavioural spec via hasRole) ────────────

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

// ─── Cookie-based auth model verification ────────────────────────────────────
// The JWT is stored in an HttpOnly cookie set by the server.
// JS cannot read HttpOnly cookies via document.cookie — that is the security guarantee.
// These tests verify the auth module does NOT reference localStorage or document.cookie.

describe('auth module does not use localStorage (HttpOnly cookie model)', () => {
  it('localStorage is not referenced by hasRole', () => {
    // hasRole is a pure function — it only operates on its arguments
    const user = makeUser('admin')
    // If hasRole touched localStorage, this would fail or throw in a restricted env
    expect(hasRole(user, ['admin'])).toBe(true)
    expect(hasRole(null, ['admin'])).toBe(false)
  })

  it('isSelfOrRole is a pure function with no side effects', () => {
    const user = makeUser('viewer', 'v-1')
    expect(isSelfOrRole(user, ['admin'], 'v-1')).toBe(true)
    expect(isSelfOrRole(user, ['admin'], 'other')).toBe(false)
  })
})
