import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { UserPlus, Trash2, Pencil, ShieldAlert, X, Check } from 'lucide-react'
import { useAuth, hasRole, isSelfOrRole } from '../hooks/auth'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

interface UserRecord {
  user_id: string; tenant_id: string; username: string; email: string
  display_name: string; role: 'admin' | 'editor' | 'viewer'; is_active: boolean
  last_login_at: string | null; created_at: string
}

interface CreateUserPayload {
  username: string; email: string; password: string; display_name: string; role: string
}

const ROLE_STYLE: Record<string, { bg: string; color: string }> = {
  admin:  { bg: 'rgba(255,69,58,0.1)',  color: 'var(--protect)' },
  editor: { bg: 'rgba(255,159,10,0.1)', color: 'var(--prove)' },
  viewer: { bg: 'rgba(255,255,255,0.06)', color: 'var(--text-secondary)' },
}

function RoleBadge({ role }: { role: string }) {
  const style = ROLE_STYLE[role] ?? ROLE_STYLE.viewer
  return (
    <span style={{ fontSize: 9, fontWeight: 700, letterSpacing: '0.1em', padding: '2px 8px', borderRadius: 4, background: style.bg, color: style.color, border: `1px solid ${style.color}30` }}>
      {role.toUpperCase()}
    </span>
  )
}

function CreateUserModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const qc = useQueryClient()
  const [form, setForm] = useState<CreateUserPayload>({ username: '', email: '', password: '', display_name: '', role: 'viewer' })
  const [error, setError] = useState('')

  const mutation = useMutation({
    mutationFn: async (payload: CreateUserPayload) => {
      const res = await fetch(`${BASE}/api/v1/users`, { method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'include', body: JSON.stringify(payload) })
      if (!res.ok) { const body = await res.json().catch(() => ({})); throw new Error(body.error ?? `HTTP ${res.status}`) }
      return res.json()
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['users'] }); onCreated() },
    onError: (err: Error) => setError(err.message),
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.username || !form.email || !form.password) { setError('Username, email, and password are required.'); return }
    mutation.mutate(form)
  }

  return (
    <div style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.7)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000, backdropFilter: 'blur(8px)' }} onClick={onClose}>
      <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 14, padding: 32, width: 440 }} onClick={e => e.stopPropagation()}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 24 }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 700, color: 'var(--text-primary)' }}>Create User</h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-tertiary)' }}><X size={16} /></button>
        </div>
        <form onSubmit={handleSubmit} style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
          {(['username', 'email', 'display_name'] as const).map(field => (
            <div key={field} style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
              <label style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.08em', fontWeight: 700 }}>
                {field.replace('_', ' ').toUpperCase()}{field !== 'display_name' ? ' *' : ''}
              </label>
              <input id={`user-${field}`} type={field === 'email' ? 'email' : 'text'} value={form[field]} onChange={e => setForm(f => ({ ...f, [field]: e.target.value }))} style={inputStyle} disabled={mutation.isPending} />
            </div>
          ))}
          <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
            <label style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.08em', fontWeight: 700 }}>PASSWORD *</label>
            <input id="user-password" type="password" value={form.password} onChange={e => setForm(f => ({ ...f, password: e.target.value }))} style={inputStyle} disabled={mutation.isPending} />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
            <label style={{ fontSize: 10, color: 'var(--text-tertiary)', letterSpacing: '0.08em', fontWeight: 700 }}>ROLE</label>
            <select id="user-role" value={form.role} onChange={e => setForm(f => ({ ...f, role: e.target.value }))} style={inputStyle} disabled={mutation.isPending}>
              <option value="viewer">viewer</option>
              <option value="editor">editor</option>
              <option value="admin">admin</option>
            </select>
          </div>
          {error && <div style={{ fontSize: 12, color: 'var(--protect)', padding: '8px 12px', background: 'rgba(255,69,58,0.08)', borderRadius: 8, border: '1px solid rgba(255,69,58,0.2)' }}>{error}</div>}
          <div style={{ display: 'flex', gap: 10, marginTop: 8 }}>
            <button type="button" onClick={onClose} style={ghostBtn} disabled={mutation.isPending}>Cancel</button>
            <button id="user-create-submit" type="submit" style={primaryBtn} disabled={mutation.isPending}>{mutation.isPending ? 'Creating…' : 'Create User'}</button>
          </div>
        </form>
      </div>
    </div>
  )
}

export default function UsersPage() {
  const { user: currentUser } = useAuth()
  const qc = useQueryClient()
  const [showCreate, setShowCreate] = useState(false)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const isAdmin = hasRole(currentUser, ['admin'])

  const { data: users, isLoading, error } = useQuery<UserRecord[]>({
    queryKey: ['users'],
    queryFn: async () => {
      const res = await fetch(`${BASE}/api/v1/users`, { credentials: 'include' })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const body = await res.json()
      return Array.isArray(body) ? body : (body.items ?? [])
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (userId: string) => {
      const res = await fetch(`${BASE}/api/v1/users/${userId}`, { method: 'DELETE', credentials: 'include' })
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
    },
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['users'] }); setDeletingId(null) },
  })

  return (
    <div style={{ padding: '40px 48px', maxWidth: 1200, margin: '0 auto' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', marginBottom: 32 }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
            <span style={{ fontSize: 10, fontWeight: 700, color: 'var(--text-tertiary)', letterSpacing: '0.1em' }}>ADMIN</span>
          </div>
          <h1 style={{ fontSize: 28, fontWeight: 700, color: 'var(--text-primary)', margin: 0, letterSpacing: '-0.02em' }}>Users</h1>
          <p style={{ fontSize: 12, color: 'var(--text-tertiary)', marginTop: 4 }}>Manage user accounts and RBAC roles</p>
        </div>
        {isAdmin && (
          <button id="create-user-btn" onClick={() => setShowCreate(true)}
            style={{ display: 'flex', alignItems: 'center', gap: 8, background: 'var(--control)', border: 'none', borderRadius: 8, color: '#fff', fontSize: 12, fontWeight: 700, padding: '10px 18px', cursor: 'pointer' }}>
            <UserPlus size={14} /> Create User
          </button>
        )}
      </div>

      {!isAdmin && (
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, background: 'rgba(255,159,10,0.08)', border: '1px solid rgba(255,159,10,0.2)', borderRadius: 10, padding: '10px 16px', marginBottom: 24, fontSize: 12, color: 'var(--prove)' }}>
          <ShieldAlert size={14} /> You have read-only access. Contact an admin to modify users.
        </div>
      )}

      <div style={{ background: 'var(--layer-2)', border: '1px solid var(--layer-border)', borderRadius: 12, overflow: 'hidden' }}>
        <div style={{ padding: '14px 24px', borderBottom: '1px solid var(--layer-border)', fontSize: 10, fontWeight: 700, letterSpacing: '0.1em', color: 'var(--text-tertiary)' }}>
          USERS — {isLoading ? '…' : `${users?.length ?? 0} total`}
        </div>

        {isLoading && <div style={{ padding: 40, textAlign: 'center', fontSize: 13, color: 'var(--text-tertiary)' }}>Loading users…</div>}
        {error && <div style={{ padding: 40, textAlign: 'center', fontSize: 13, color: 'var(--protect)' }}>Failed to load users. Make sure the API gateway is reachable.</div>}

        {!isLoading && !error && users && (
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ background: 'var(--layer-1)' }}>
                {['Username', 'Email', 'Display Name', 'Role', 'Status', 'Created', ''].map(h => (
                  <th key={h} style={{ padding: '10px 16px', textAlign: 'left', fontSize: 9, fontWeight: 700, letterSpacing: '0.1em', color: 'var(--text-tertiary)', borderBottom: '1px solid var(--layer-border)' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {users.length === 0 && <tr><td colSpan={7} style={{ padding: 48, textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 13 }}>No users found.</td></tr>}
              {users.map(u => {
                const canEdit = isSelfOrRole(currentUser, ['admin'], u.user_id)
                const canDelete = isAdmin && u.user_id !== currentUser?.sub
                const isDeleting = deletingId === u.user_id
                return (
                  <tr key={u.user_id} style={{ borderBottom: '1px solid var(--layer-border)' }}>
                    <td style={tdStyle}><span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>{u.username}</span></td>
                    <td style={tdStyle}><span style={{ color: 'var(--text-secondary)' }}>{u.email}</span></td>
                    <td style={tdStyle}><span style={{ color: 'var(--text-tertiary)' }}>{u.display_name || '—'}</span></td>
                    <td style={tdStyle}><RoleBadge role={u.role} /></td>
                    <td style={tdStyle}><span style={{ color: u.is_active ? 'var(--spend)' : 'var(--protect)', fontSize: 11 }}>{u.is_active ? '● active' : '○ inactive'}</span></td>
                    <td style={tdStyle}><span style={{ color: 'var(--text-tertiary)', fontSize: 11 }}>{u.created_at ? new Date(u.created_at).toLocaleDateString() : '—'}</span></td>
                    <td style={{ ...tdStyle, textAlign: 'right' }}>
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 6 }}>
                        {canEdit && (
                          <button title="Edit user" style={iconBtn} onClick={() => { window.location.href = `/users/${u.user_id}/edit` }}>
                            <Pencil size={13} />
                          </button>
                        )}
                        {canDelete && !isDeleting && (
                          <button title="Delete user" style={{ ...iconBtn, color: 'var(--protect)' }} onClick={() => setDeletingId(u.user_id)}>
                            <Trash2 size={13} />
                          </button>
                        )}
                        {canDelete && isDeleting && (
                          <div style={{ display: 'flex', gap: 4 }}>
                            <button title="Confirm delete" style={{ ...iconBtn, color: 'var(--protect)', border: '1px solid rgba(255,69,58,0.25)' }} onClick={() => deleteMutation.mutate(u.user_id)} disabled={deleteMutation.isPending}><Check size={13} /></button>
                            <button title="Cancel" style={iconBtn} onClick={() => setDeletingId(null)} disabled={deleteMutation.isPending}><X size={13} /></button>
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

      {showCreate && <CreateUserModal onClose={() => setShowCreate(false)} onCreated={() => setShowCreate(false)} />}
    </div>
  )
}

const tdStyle: React.CSSProperties = { padding: '12px 16px', verticalAlign: 'middle' }
const iconBtn: React.CSSProperties = { background: 'none', border: '1px solid transparent', borderRadius: 6, padding: '4px 7px', cursor: 'pointer', color: 'var(--text-tertiary)', display: 'flex', alignItems: 'center', transition: 'color 0.15s' }
const inputStyle: React.CSSProperties = { background: 'var(--layer-1)', border: '1px solid var(--layer-border)', borderRadius: 8, padding: '9px 12px', color: 'var(--text-primary)', fontSize: 13, outline: 'none', width: '100%', boxSizing: 'border-box' }
const primaryBtn: React.CSSProperties = { flex: 1, background: 'var(--control)', border: 'none', borderRadius: 8, color: '#fff', fontSize: 13, fontWeight: 700, padding: '10px 20px', cursor: 'pointer' }
const ghostBtn: React.CSSProperties = { background: 'none', border: '1px solid var(--layer-border)', borderRadius: 8, color: 'var(--text-secondary)', fontSize: 13, padding: '10px 20px', cursor: 'pointer' }
