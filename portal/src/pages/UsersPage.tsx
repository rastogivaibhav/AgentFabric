// portal/src/pages/UsersPage.tsx
// User management — list, create, and delete users.
//
// RBAC / ABAC rules (mirror api-gateway middleware):
//   List / Read : any authenticated user
//   Create      : admin only
//   Delete      : admin only
//   Edit        : admin OR the user editing their own record (ABAC self-service)
//
// The API returns 403 for unauthorised attempts regardless — hiding the UI
// elements is defence-in-depth that removes the invitation to attempt them.

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { UserPlus, Trash2, Pencil, ShieldAlert, X, Check } from 'lucide-react'
import { useAuth, hasRole, isSelfOrRole } from '../hooks/auth'
import { getToken } from '../hooks/auth'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

// ─── Types ────────────────────────────────────────────────────────────────────

interface UserRecord {
  user_id:       string
  tenant_id:     string
  username:      string
  email:         string
  display_name:  string
  role:          'admin' | 'editor' | 'viewer'
  is_active:     boolean
  last_login_at: string | null
  created_at:    string
}

interface CreateUserPayload {
  username:     string
  email:        string
  password:     string
  display_name: string
  role:         string
}

// ─── Role badge ───────────────────────────────────────────────────────────────

const ROLE_STYLE: Record<string, { bg: string; color: string }> = {
  admin:  { bg: '#EF444420', color: '#EF4444' },
  editor: { bg: '#F59E0B20', color: '#F59E0B' },
  viewer: { bg: '#47556920', color: '#64748B' },
}

function RoleBadge({ role }: { role: string }) {
  const style = ROLE_STYLE[role] ?? ROLE_STYLE.viewer
  return (
    <span style={{
      fontSize: 9, fontWeight: 600, letterSpacing: '0.1em',
      padding: '2px 7px', borderRadius: 3,
      background: style.bg, color: style.color,
      border: `1px solid ${style.color}40`,
    }}>
      {role.toUpperCase()}
    </span>
  )
}

// ─── Create-user modal ────────────────────────────────────────────────────────

interface CreateModalProps {
  onClose: () => void
  onCreated: () => void
}

function CreateUserModal({ onClose, onCreated }: CreateModalProps) {
  const qc = useQueryClient()
  const [form, setForm] = useState<CreateUserPayload>({
    username: '', email: '', password: '', display_name: '', role: 'viewer',
  })
  const [error, setError] = useState('')

  const mutation = useMutation({
    mutationFn: async (payload: CreateUserPayload) => {
      const token = getToken()
      const res = await fetch(`${BASE}/api/v1/users`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify(payload),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error ?? `HTTP ${res.status}`)
      }
      return res.json()
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      onCreated()
    },
    onError: (err: Error) => setError(err.message),
  })

  function handleChange(field: keyof CreateUserPayload, value: string) {
    setForm(f => ({ ...f, [field]: value }))
    setError('')
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.username || !form.email || !form.password) {
      setError('Username, email, and password are required.')
      return
    }
    mutation.mutate(form)
  }

  return (
    // Backdrop
    <div style={{
      position: 'fixed', inset: 0, background: '#00000080',
      display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50,
    }} onClick={onClose}>
      <div
        style={{
          background: '#060A14', border: '1px solid #1E3A5F', borderRadius: 10,
          padding: 32, width: 420, fontFamily: "'JetBrains Mono', monospace",
        }}
        onClick={e => e.stopPropagation()}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
          <h2 style={{ margin: 0, fontSize: 16, fontWeight: 600, color: '#F0F9FF' }}>Create User</h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: '#475569' }}>
            <X size={16} />
          </button>
        </div>

        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {(['username', 'email', 'display_name'] as const).map(field => (
            <div key={field} style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
              <label style={{ fontSize: 10, color: '#64748B', letterSpacing: '0.08em' }}>
                {field.replace('_', ' ').toUpperCase()}{field !== 'display_name' ? ' *' : ''}
              </label>
              <input
                type={field === 'email' ? 'email' : 'text'}
                value={form[field]}
                onChange={e => handleChange(field, e.target.value)}
                style={inputStyle}
                disabled={mutation.isPending}
              />
            </div>
          ))}

          <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
            <label style={{ fontSize: 10, color: '#64748B', letterSpacing: '0.08em' }}>PASSWORD *</label>
            <input
              type="password"
              value={form.password}
              onChange={e => handleChange('password', e.target.value)}
              style={inputStyle}
              disabled={mutation.isPending}
            />
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
            <label style={{ fontSize: 10, color: '#64748B', letterSpacing: '0.08em' }}>ROLE</label>
            <select
              value={form.role}
              onChange={e => handleChange('role', e.target.value)}
              style={{ ...inputStyle, cursor: 'pointer' }}
              disabled={mutation.isPending}
            >
              <option value="viewer">viewer</option>
              <option value="editor">editor</option>
              <option value="admin">admin</option>
            </select>
          </div>

          {error && (
            <div style={{ fontSize: 11, color: '#EF4444', padding: '8px 12px', background: '#EF444415', borderRadius: 6, border: '1px solid #EF444430' }}>
              {error}
            </div>
          )}

          <div style={{ display: 'flex', gap: 10, marginTop: 4 }}>
            <button type="button" onClick={onClose} style={cancelBtnStyle} disabled={mutation.isPending}>
              Cancel
            </button>
            <button type="submit" style={submitBtnStyle} disabled={mutation.isPending}>
              {mutation.isPending ? 'Creating…' : 'Create User'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ─── Main page ────────────────────────────────────────────────────────────────

export default function UsersPage() {
  const { user: currentUser } = useAuth()
  const qc = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)

  const isAdmin = hasRole(currentUser, ['admin'])

  const { data: users, isLoading, error } = useQuery<UserRecord[]>({
    queryKey: ['users'],
    queryFn: async () => {
      const token = getToken()
      const res = await fetch(`${BASE}/api/v1/users`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const body = await res.json()
      // API may return { items: [...] } or a plain array
      return Array.isArray(body) ? body : (body.items ?? [])
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (userId: string) => {
      const token = getToken()
      const res = await fetch(`${BASE}/api/v1/users/${userId}`, {
        method: 'DELETE',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['users'] })
      setDeletingId(null)
    },
  })

  return (
    <div style={{ padding: 32 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 28 }}>
        <div>
          <h1 style={{ fontSize: 22, fontWeight: 700, color: '#F0F9FF', margin: 0 }}>Users</h1>
          <p style={{ fontSize: 12, color: '#475569', marginTop: 4 }}>Manage user accounts and roles</p>
        </div>
        {/* Create button — admin only */}
        {isAdmin && (
          <button
            onClick={() => setShowCreate(true)}
            style={{
              display: 'flex', alignItems: 'center', gap: 8,
              background: 'linear-gradient(90deg,#1E3A5F,#1e40af)',
              border: '1px solid #3B82F6', borderRadius: 8,
              color: '#93C5FD', fontSize: 12,
              fontFamily: "'JetBrains Mono', monospace",
              fontWeight: 600, padding: '10px 18px', cursor: 'pointer',
            }}
          >
            <UserPlus size={14} />
            Create User
          </button>
        )}
      </div>

      {/* Non-admin notice */}
      {!isAdmin && (
        <div style={{
          display: 'flex', alignItems: 'center', gap: 10,
          background: '#F59E0B10', border: '1px solid #F59E0B30',
          borderRadius: 8, padding: '10px 16px', marginBottom: 20,
          fontSize: 11, color: '#F59E0B',
        }}>
          <ShieldAlert size={14} />
          You have read-only access to this page. Contact an admin to modify users.
        </div>
      )}

      {/* Table */}
      <div style={{ background: '#0D1B2A', border: '1px solid #0F1F35', borderRadius: 10, overflow: 'hidden' }}>
        <div style={{ padding: '12px 24px', borderBottom: '1px solid #0F1F35', fontSize: 11, color: '#475569', letterSpacing: '0.1em' }}>
          USERS — {isLoading ? '…' : `${users?.length ?? 0} total`}
        </div>

        {isLoading && (
          <div style={{ padding: 32, textAlign: 'center', fontSize: 12, color: '#334155' }}>Loading users…</div>
        )}

        {error && (
          <div style={{ padding: 32, textAlign: 'center', fontSize: 12, color: '#EF4444' }}>
            Failed to load users. Make sure the API gateway is reachable.
          </div>
        )}

        {!isLoading && !error && users && (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 12 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #0F1F35' }}>
                {['Username', 'Email', 'Display Name', 'Role', 'Status', 'Created', ''].map(h => (
                  <th key={h} style={{ padding: '10px 16px', textAlign: 'left', fontSize: 10, color: '#475569', letterSpacing: '0.1em', fontWeight: 500 }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {users.length === 0 && (
                <tr>
                  <td colSpan={7} style={{ padding: 32, textAlign: 'center', color: '#334155' }}>
                    No users found.
                  </td>
                </tr>
              )}
              {users.map(u => {
                const canEdit   = isSelfOrRole(currentUser, ['admin'], u.user_id)
                const canDelete = isAdmin && u.user_id !== currentUser?.sub // prevent self-delete
                const isDeleting = deletingId === u.user_id

                return (
                  <tr key={u.user_id} style={{ borderBottom: '1px solid #0A1628' }}>
                    <td style={tdStyle}>
                      <span style={{ color: '#E2E8F0', fontWeight: 500 }}>{u.username}</span>
                    </td>
                    <td style={tdStyle}>
                      <span style={{ color: '#94A3B8' }}>{u.email}</span>
                    </td>
                    <td style={tdStyle}>
                      <span style={{ color: '#64748B' }}>{u.display_name || '—'}</span>
                    </td>
                    <td style={tdStyle}>
                      <RoleBadge role={u.role} />
                    </td>
                    <td style={tdStyle}>
                      <span style={{ color: u.is_active ? '#10B981' : '#EF4444', fontSize: 10 }}>
                        {u.is_active ? '● active' : '○ inactive'}
                      </span>
                    </td>
                    <td style={tdStyle}>
                      <span style={{ color: '#334155', fontSize: 10 }}>
                        {u.created_at ? new Date(u.created_at).toLocaleDateString() : '—'}
                      </span>
                    </td>
                    <td style={{ ...tdStyle, textAlign: 'right' }}>
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 6 }}>
                        {/* Edit — admin or own user (ABAC) */}
                        {canEdit && (
                          <button
                            title="Edit user"
                            style={iconBtnStyle}
                            onClick={() => {
                              // Navigate to edit or open inline edit modal (stubbed — API PUT /users/:id)
                              window.location.href = `/users/${u.user_id}/edit`
                            }}
                          >
                            <Pencil size={13} />
                          </button>
                        )}
                        {/* Delete — admin only, cannot self-delete */}
                        {canDelete && !isDeleting && (
                          <button
                            title="Delete user"
                            style={{ ...iconBtnStyle, color: '#EF4444' }}
                            onClick={() => setDeletingId(u.user_id)}
                          >
                            <Trash2 size={13} />
                          </button>
                        )}
                        {/* Confirm-delete micro-UX */}
                        {canDelete && isDeleting && (
                          <div style={{ display: 'flex', gap: 4 }}>
                            <button
                              title="Confirm delete"
                              style={{ ...iconBtnStyle, color: '#EF4444', border: '1px solid #EF444440' }}
                              onClick={() => deleteMutation.mutate(u.user_id)}
                              disabled={deleteMutation.isPending}
                            >
                              <Check size={13} />
                            </button>
                            <button
                              title="Cancel"
                              style={iconBtnStyle}
                              onClick={() => setDeletingId(null)}
                              disabled={deleteMutation.isPending}
                            >
                              <X size={13} />
                            </button>
                          </div>
                        )}
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* Create-user modal */}
      {showCreate && (
        <CreateUserModal
          onClose={() => setShowCreate(false)}
          onCreated={() => setShowCreate(false)}
        />
      )}
    </div>
  )
}

// ─── Shared style objects ────────────────────────────────────────────────────

const tdStyle: React.CSSProperties = {
  padding: '12px 16px',
  verticalAlign: 'middle',
}

const iconBtnStyle: React.CSSProperties = {
  background: 'none',
  border: '1px solid transparent',
  borderRadius: 4,
  padding: '4px 6px',
  cursor: 'pointer',
  color: '#475569',
  display: 'flex',
  alignItems: 'center',
  fontFamily: "'JetBrains Mono', monospace",
}

const inputStyle: React.CSSProperties = {
  background: '#0D1B2A',
  border: '1px solid #1E3A5F',
  borderRadius: 6,
  padding: '9px 12px',
  color: '#E2E8F0',
  fontSize: 12,
  fontFamily: "'JetBrains Mono', monospace",
  outline: 'none',
  width: '100%',
  boxSizing: 'border-box',
}

const submitBtnStyle: React.CSSProperties = {
  flex: 1,
  background: 'linear-gradient(90deg,#1E3A5F,#1e40af)',
  border: '1px solid #3B82F6',
  borderRadius: 8,
  color: '#93C5FD',
  fontSize: 12,
  fontFamily: "'JetBrains Mono', monospace",
  fontWeight: 600,
  padding: '10px 20px',
  cursor: 'pointer',
}

const cancelBtnStyle: React.CSSProperties = {
  background: 'transparent',
  border: '1px solid #1E3A5F',
  borderRadius: 8,
  color: '#475569',
  fontSize: 12,
  fontFamily: "'JetBrains Mono', monospace",
  padding: '10px 20px',
  cursor: 'pointer',
}
