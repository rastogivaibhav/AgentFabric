import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Key, Plus, Trash2, Copy, Check, RefreshCw } from 'lucide-react'
import { apiFetch, apiMutate, SUPPORTED_KEY_PROVIDERS } from '../hooks/api'

// ─── Types ────────────────────────────────────────────────────────────────────

interface VirtualKey {
  virtual_key: string
  display_name: string
  provider: string
  key_id: string
  team_id?: string
  last_used_at?: string
  revoked: boolean
  created_at: string
}

interface RegisterKeyForm {
  provider: string
  real_key: string
  display_name: string
  team_id: string
}

// ─── API helpers ──────────────────────────────────────────────────────────────

const fetchKeys = (): Promise<{ items: VirtualKey[]; count: number }> =>
  apiFetch('/keys')

const registerKey = (form: RegisterKeyForm) =>
  apiMutate<{ virtual_key: string; provider: string }>('/keys', 'POST', form)

const revokeKey = (virtualKey: string) =>
  apiMutate<void>(`/keys/${virtualKey}`, 'DELETE')

// ─── Component ────────────────────────────────────────────────────────────────

const PROVIDER_LABELS: Record<string, string> = Object.fromEntries(
  SUPPORTED_KEY_PROVIDERS.map((provider) => [provider.value, provider.label]),
)

const PROVIDER_COLORS: Record<string, string> = {
  openai: '#10a37f',
  anthropic: '#c96a3a',
  google: '#4285f4',
}

const providerHintFor = (provider: string) =>
  SUPPORTED_KEY_PROVIDERS.find((option) => option.value === provider) ?? SUPPORTED_KEY_PROVIDERS[0]

export default function ApiKeysPage() {
  const qc = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [newVirtualKey, setNewVirtualKey] = useState<string | null>(null)
  const [newVirtualKeyProvider, setNewVirtualKeyProvider] = useState('openai')
  const [copied, setCopied] = useState(false)
  const [form, setForm] = useState<RegisterKeyForm>({
    provider: 'openai',
    real_key: '',
    display_name: '',
    team_id: '',
  })
  const [revoking, setRevoking] = useState<string | null>(null)

  const { data, isLoading, error } = useQuery({
    queryKey: ['virtual-keys'],
    queryFn: fetchKeys,
    refetchInterval: 30_000,
  })

  const registerMut = useMutation({
    mutationFn: registerKey,
    onSuccess: (res) => {
      setNewVirtualKey(res.virtual_key)
      setNewVirtualKeyProvider(res.provider)
      setShowForm(false)
      setForm({ provider: 'openai', real_key: '', display_name: '', team_id: '' })
      qc.invalidateQueries({ queryKey: ['virtual-keys'] })
    },
  })

  const revokeMut = useMutation({
    mutationFn: revokeKey,
    onSuccess: () => {
      setRevoking(null)
      qc.invalidateQueries({ queryKey: ['virtual-keys'] })
    },
  })

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  const keys = data?.items ?? []
  const selectedProviderHint = providerHintFor(form.provider)
  const newKeyProviderHint = providerHintFor(newVirtualKeyProvider)

  return (
    <div style={{ padding: '24px', maxWidth: 900 }}>
      {/* Header */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 24 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 20, fontWeight: 600, display: 'flex', alignItems: 'center', gap: 8 }}>
            <Key size={20} /> Virtual Keys
          </h1>
          <p style={{ margin: '4px 0 0', fontSize: 13, color: '#888' }}>
            Register real LLM API keys and get virtual keys (<code style={{ fontSize: 12 }}>af-vk-*</code>) to use in your agents.
            Real keys are encrypted and never returned after registration.
          </p>
        </div>
        <button
          onClick={() => { setShowForm(true); setNewVirtualKey(null) }}
          style={{
            display: 'flex', alignItems: 'center', gap: 6,
            padding: '8px 16px', background: '#6366f1', color: '#fff',
            border: 'none', borderRadius: 6, cursor: 'pointer', fontSize: 13, fontWeight: 500,
          }}
        >
          <Plus size={14} /> Add Key
        </button>
      </div>

      {/* New virtual key reveal — shown once after registration */}
      {newVirtualKey && (
        <div style={{
          background: '#0f2a1a', border: '1px solid #166534', borderRadius: 8,
          padding: '16px 20px', marginBottom: 24,
        }}>
          <p style={{ margin: '0 0 8px', fontSize: 13, color: '#4ade80', fontWeight: 500 }}>
            ✓ Key registered. Copy your virtual key now — it will not be shown again.
          </p>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <code style={{
              flex: 1, background: '#1a3a2a', padding: '8px 12px', borderRadius: 4,
              fontSize: 13, color: '#86efac', letterSpacing: '0.02em',
            }}>
              {newVirtualKey}
            </code>
            <button
              onClick={() => copyToClipboard(newVirtualKey)}
              style={{
                display: 'flex', alignItems: 'center', gap: 4,
                padding: '6px 12px', background: copied ? '#166534' : '#374151',
                color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12,
              }}
            >
              {copied ? <Check size={12} /> : <Copy size={12} />}
              {copied ? 'Copied' : 'Copy'}
            </button>
          </div>
          <p style={{ margin: '10px 0 0', fontSize: 12, color: '#6b7280' }}>
            Use it against <code style={{ fontSize: 11 }}>http://localhost:8080{newKeyProviderHint.route_hint}</code> with{' '}
            <code style={{ fontSize: 11 }}>{newVirtualKey}</code> as your API key.
          </p>
        </div>
      )}

      {/* Registration form */}
      {showForm && (
        <div style={{
          background: '#111827', border: '1px solid #374151', borderRadius: 8,
          padding: '20px', marginBottom: 24,
        }}>
          <h3 style={{ margin: '0 0 16px', fontSize: 14, fontWeight: 600 }}>Register new key</h3>
          <div style={{ display: 'grid', gap: 12 }}>
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
              <label style={{ fontSize: 12, color: '#9ca3af' }}>
                Provider
                <select
                  value={form.provider}
                  onChange={e => setForm(f => ({ ...f, provider: e.target.value }))}
                  style={{
                    display: 'block', width: '100%', marginTop: 4,
                    background: '#1f2937', border: '1px solid #374151', borderRadius: 4,
                    color: '#f9fafb', padding: '7px 10px', fontSize: 13,
                  }}
                >
                  {SUPPORTED_KEY_PROVIDERS.map((provider) => (
                    <option key={provider.value} value={provider.value}>
                      {provider.label}
                    </option>
                  ))}
                </select>
              </label>
              <label style={{ fontSize: 12, color: '#9ca3af' }}>
                Display name
                <input
                  value={form.display_name}
                  onChange={e => setForm(f => ({ ...f, display_name: e.target.value }))}
                  placeholder="e.g. Prod OpenAI"
                  style={{
                    display: 'block', width: '100%', marginTop: 4, boxSizing: 'border-box',
                    background: '#1f2937', border: '1px solid #374151', borderRadius: 4,
                    color: '#f9fafb', padding: '7px 10px', fontSize: 13,
                  }}
                />
              </label>
            </div>
            <label style={{ fontSize: 12, color: '#9ca3af' }}>
              Real API key
              <input
                type="password"
                value={form.real_key}
                onChange={e => setForm(f => ({ ...f, real_key: e.target.value }))}
                placeholder={selectedProviderHint.key_placeholder}
                style={{
                  display: 'block', width: '100%', marginTop: 4, boxSizing: 'border-box',
                  background: '#1f2937', border: '1px solid #374151', borderRadius: 4,
                  color: '#f9fafb', padding: '7px 10px', fontSize: 13, fontFamily: 'monospace',
                }}
              />
            </label>
            {registerMut.isError && (
              <p style={{ margin: 0, fontSize: 12, color: '#f87171' }}>
                Registration failed. Check the key and try again.
              </p>
            )}
          </div>
          <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
            <button
              onClick={() => registerMut.mutate(form)}
              disabled={registerMut.isPending || !form.real_key || !form.display_name}
              style={{
                padding: '8px 20px', background: '#6366f1', color: '#fff',
                border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 13,
                opacity: registerMut.isPending ? 0.6 : 1,
              }}
            >
              {registerMut.isPending ? 'Registering…' : 'Register'}
            </button>
            <button
              onClick={() => setShowForm(false)}
              style={{
                padding: '8px 16px', background: '#374151', color: '#d1d5db',
                border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 13,
              }}
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Keys table */}
      {isLoading ? (
        <div style={{ textAlign: 'center', padding: 40, color: '#6b7280' }}>
          <RefreshCw size={20} style={{ animation: 'spin 1s linear infinite' }} />
        </div>
      ) : error ? (
        <div style={{ color: '#f87171', padding: 20 }}>Failed to load keys.</div>
      ) : keys.length === 0 ? (
        <div style={{
          textAlign: 'center', padding: '48px 24px',
          border: '1px dashed #374151', borderRadius: 8, color: '#6b7280',
        }}>
          <Key size={32} style={{ opacity: 0.3, marginBottom: 12 }} />
          <p style={{ margin: 0, fontSize: 14 }}>No virtual keys yet.</p>
          <p style={{ margin: '4px 0 0', fontSize: 12 }}>Add a key to start proxying LLM requests through AgentFabric.</p>
        </div>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead>
            <tr style={{ borderBottom: '1px solid #374151' }}>
              {['Provider', 'Display Name', 'Virtual Key', 'Real Key Prefix', 'Last Used', 'Status'].map(h => (
                <th key={h} style={{ padding: '8px 12px', textAlign: 'left', color: '#6b7280', fontWeight: 500, fontSize: 12 }}>{h}</th>
              ))}
              <th />
            </tr>
          </thead>
          <tbody>
            {keys.map(k => (
              <tr key={k.virtual_key} style={{ borderBottom: '1px solid #1f2937' }}>
                <td style={{ padding: '10px 12px' }}>
                  <span style={{
                    padding: '2px 8px', borderRadius: 12, fontSize: 11, fontWeight: 600,
                    background: PROVIDER_COLORS[k.provider] + '22',
                    color: PROVIDER_COLORS[k.provider] ?? '#9ca3af',
                  }}>
                    {PROVIDER_LABELS[k.provider] ?? k.provider}
                  </span>
                </td>
                <td style={{ padding: '10px 12px', color: '#f3f4f6' }}>{k.display_name}</td>
                <td style={{ padding: '10px 12px' }}>
                  <code style={{ fontSize: 12, color: '#a5b4fc' }}>
                    {k.virtual_key.slice(0, 14)}…
                  </code>
                </td>
                <td style={{ padding: '10px 12px' }}>
                  <code style={{ fontSize: 12, color: '#9ca3af' }}>{k.key_id}…</code>
                </td>
                <td style={{ padding: '10px 12px', color: '#6b7280', fontSize: 12 }}>
                  {k.last_used_at ? new Date(k.last_used_at).toLocaleString() : '—'}
                </td>
                <td style={{ padding: '10px 12px' }}>
                  <span style={{
                    padding: '2px 8px', borderRadius: 12, fontSize: 11,
                    background: k.revoked ? '#451a1a' : '#14532d',
                    color: k.revoked ? '#f87171' : '#4ade80',
                  }}>
                    {k.revoked ? 'Revoked' : 'Active'}
                  </span>
                </td>
                <td style={{ padding: '10px 12px' }}>
                  {!k.revoked && (
                    revoking === k.virtual_key ? (
                      <div style={{ display: 'flex', gap: 6 }}>
                        <button
                          onClick={() => revokeMut.mutate(k.virtual_key)}
                          style={{
                            padding: '4px 10px', background: '#dc2626', color: '#fff',
                            border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 11,
                          }}
                        >
                          Confirm
                        </button>
                        <button
                          onClick={() => setRevoking(null)}
                          style={{
                            padding: '4px 10px', background: '#374151', color: '#d1d5db',
                            border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 11,
                          }}
                        >
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => setRevoking(k.virtual_key)}
                        style={{
                          display: 'flex', alignItems: 'center', gap: 4,
                          padding: '4px 10px', background: 'transparent',
                          border: '1px solid #374151', borderRadius: 4,
                          color: '#9ca3af', cursor: 'pointer', fontSize: 11,
                        }}
                      >
                        <Trash2 size={11} /> Revoke
                      </button>
                    )
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {/* Usage hint */}
      {keys.length > 0 && (
        <div style={{
          marginTop: 24, padding: '12px 16px',
          background: '#0f172a', border: '1px solid #1e3a5f', borderRadius: 6,
          fontSize: 12, color: '#93c5fd',
        }}>
          <strong>How to use:</strong> Set{' '}
          <code>http://localhost:8080/proxy/&lt;provider&gt;/...</code> as your upstream base and use your{' '}
          <code>af-vk-*</code> key in place of the provider API key. Supported providers: {SUPPORTED_KEY_PROVIDERS.map((provider) => provider.label).join(', ')}.
        </div>
      )}
    </div>
  )
}
