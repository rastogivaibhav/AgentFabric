// portal/src/pages/AuditPage.tsx
//
// Admin-only view of the immutable policy audit log.
//
// Access control:
//   This page is gated to the "admin" role both here (UX) and at the API
//   gateway (returns 403 for non-admins).  Editors and viewers see an
//   "Access denied" message rather than a blank page or an unhandled error.
//
// Data shape (GET /api/v1/audit):
//   AuditEntry[] — id, decision_id, trace_id, span_id, policy_name, result,
//                  reason, tenant_id, evaluated_at, previous_hash, entry_hash

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ShieldCheck, ShieldX, AlertTriangle, Eraser, ChevronLeft, ChevronRight, RefreshCw, Lock } from 'lucide-react'
import { useAuth, hasRole } from '../hooks/auth'

const BASE = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'
const PAGE_SIZE = 100

// ─── Types ────────────────────────────────────────────────────────────────────

interface AuditEntry {
  id:            number
  decision_id:   string
  trace_id:      string
  span_id:       string
  policy_name:   string
  result:        'allow' | 'deny' | 'warn' | 'sanitize'
  reason:        string
  tenant_id:     string
  evaluated_at:  string
  previous_hash: string
  entry_hash:    string
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

async function fetchAudit(offset: number): Promise<AuditEntry[]> {
  const url = `${BASE}/api/v1/audit?limit=${PAGE_SIZE}&offset=${offset}`
  const res = await fetch(url, {
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include', // af_token HttpOnly cookie sent automatically
  })
  if (!res.ok) throw new Error(`API ${res.status}: /audit`)
  return res.json()
}

function truncate(s: string, len = 8): string {
  if (!s) return '—'
  return s.length <= len ? s : s.slice(0, len) + '…'
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      year:   'numeric',
      month:  'short',
      day:    'numeric',
      hour:   '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return iso
  }
}

// ─── Result badge ─────────────────────────────────────────────────────────────

const RESULT_CONFIG = {
  allow:    { label: 'allow',    icon: ShieldCheck,   bg: 'bg-emerald-100',  text: 'text-emerald-700', border: 'border-emerald-200' },
  deny:     { label: 'deny',     icon: ShieldX,       bg: 'bg-red-100',      text: 'text-red-700',     border: 'border-red-200'     },
  warn:     { label: 'warn',     icon: AlertTriangle, bg: 'bg-amber-100',    text: 'text-amber-700',   border: 'border-amber-200'   },
  sanitize: { label: 'sanitize', icon: Eraser,        bg: 'bg-blue-100',     text: 'text-blue-700',    border: 'border-blue-200'    },
} as const

function ResultBadge({ result }: { result: string }) {
  const cfg = RESULT_CONFIG[result as keyof typeof RESULT_CONFIG] ?? {
    label: result, icon: ShieldCheck,
    bg: 'bg-slate-100', text: 'text-slate-700', border: 'border-slate-200',
  }
  const Icon = cfg.icon
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold border ${cfg.bg} ${cfg.text} ${cfg.border}`}>
      <Icon size={11} />
      {cfg.label}
    </span>
  )
}

// ─── Hash cell (truncated with full value on hover) ──────────────────────────

function HashCell({ hash }: { hash: string }) {
  if (!hash) return <span className="text-slate-400 text-xs">—</span>
  return (
    <span
      title={hash}
      className="font-mono text-xs text-slate-500 cursor-help"
    >
      {hash.slice(0, 12)}
    </span>
  )
}

// ─── Access denied ────────────────────────────────────────────────────────────

function AccessDenied() {
  return (
    <div className="flex flex-col items-center justify-center h-64 gap-4 text-slate-500">
      <Lock size={40} className="text-slate-300" />
      <p className="text-lg font-semibold">Access denied</p>
      <p className="text-sm">The audit log is restricted to administrators.</p>
    </div>
  )
}

// ─── Main page ───────────────────────────────────────────────────────────────

export default function AuditPage() {
  const { user } = useAuth()
  const [offset, setOffset] = useState(0)

  const { data: entries = [], isLoading, isError, refetch, isFetching } = useQuery<AuditEntry[]>({
    queryKey: ['audit', offset],
    queryFn:  () => fetchAudit(offset),
    staleTime: 30_000,
    enabled:   hasRole(user, ['admin']),
  })

  // Non-admin: show access denied immediately without firing a network request.
  if (!hasRole(user, ['admin'])) return <AccessDenied />

  const hasPrev = offset > 0
  const hasNext = entries.length === PAGE_SIZE

  return (
    <div className="p-6 space-y-6">
      {/* ── Header ─────────────────────────────────────────────────────── */}
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Audit Log</h1>
          <p className="mt-1 text-sm text-slate-500">
            Immutable, hash-chained record of every policy decision.
            Each entry covers one span evaluation against one policy rule.
          </p>
        </div>
        <button
          onClick={() => refetch()}
          disabled={isFetching}
          className="flex items-center gap-2 px-3 py-2 text-sm rounded-lg border border-slate-200 text-slate-600 hover:bg-slate-50 disabled:opacity-50 transition-colors"
        >
          <RefreshCw size={14} className={isFetching ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {/* ── Error ──────────────────────────────────────────────────────── */}
      {isError && (
        <div className="rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
          Failed to load audit log. Check your connection and ensure your session is still valid.
        </div>
      )}

      {/* ── Table ──────────────────────────────────────────────────────── */}
      <div className="rounded-xl border border-slate-200 overflow-hidden shadow-sm">
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50">
              <tr>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">#</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Timestamp</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Policy</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Result</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Trace</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Span</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600">Reason</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Entry hash</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 bg-white">
              {isLoading
                ? Array.from({ length: 10 }).map((_, i) => (
                    <tr key={i} className="animate-pulse">
                      {Array.from({ length: 8 }).map((_, j) => (
                        <td key={j} className="px-4 py-3">
                          <div className="h-4 bg-slate-100 rounded w-20" />
                        </td>
                      ))}
                    </tr>
                  ))
                : entries.length === 0
                  ? (
                    <tr>
                      <td colSpan={8} className="px-4 py-12 text-center text-slate-400">
                        No audit entries found for this tenant.
                      </td>
                    </tr>
                  )
                  : entries.map((e) => (
                    <tr key={e.id} className="hover:bg-slate-50 transition-colors">
                      {/* Row # */}
                      <td className="px-4 py-3 text-slate-400 font-mono text-xs">
                        {e.id}
                      </td>

                      {/* Timestamp */}
                      <td className="px-4 py-3 text-slate-600 whitespace-nowrap">
                        {formatDate(e.evaluated_at)}
                      </td>

                      {/* Policy name */}
                      <td className="px-4 py-3">
                        <span
                          title={e.policy_name}
                          className="font-medium text-slate-800 max-w-[160px] truncate block"
                        >
                          {e.policy_name || '—'}
                        </span>
                      </td>

                      {/* Result badge */}
                      <td className="px-4 py-3">
                        <ResultBadge result={e.result} />
                      </td>

                      {/* Trace ID — links to TraceDetail */}
                      <td className="px-4 py-3">
                        {e.trace_id
                          ? (
                            <Link
                              to={`/traces/${e.trace_id}`}
                              title={e.trace_id}
                              className="font-mono text-xs text-indigo-600 hover:text-indigo-800 hover:underline"
                            >
                              {truncate(e.trace_id, 12)}
                            </Link>
                          )
                          : <span className="text-slate-400 text-xs">—</span>
                        }
                      </td>

                      {/* Span ID */}
                      <td className="px-4 py-3">
                        <span title={e.span_id} className="font-mono text-xs text-slate-500">
                          {truncate(e.span_id, 12)}
                        </span>
                      </td>

                      {/* Reason */}
                      <td className="px-4 py-3 text-slate-600 max-w-xs">
                        <span title={e.reason} className="line-clamp-2 block">
                          {e.reason || '—'}
                        </span>
                      </td>

                      {/* Entry hash (chain integrity) */}
                      <td className="px-4 py-3">
                        <HashCell hash={e.entry_hash} />
                      </td>
                    </tr>
                  ))
              }
            </tbody>
          </table>
        </div>
      </div>

      {/* ── Pagination ─────────────────────────────────────────────────── */}
      <div className="flex items-center justify-between text-sm text-slate-500">
        <span>
          Showing rows {offset + 1}–{offset + entries.length}
        </span>
        <div className="flex gap-2">
          <button
            onClick={() => setOffset(Math.max(0, offset - PAGE_SIZE))}
            disabled={!hasPrev || isFetching}
            className="flex items-center gap-1 px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            <ChevronLeft size={14} /> Previous
          </button>
          <button
            onClick={() => setOffset(offset + PAGE_SIZE)}
            disabled={!hasNext || isFetching}
            className="flex items-center gap-1 px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            Next <ChevronRight size={14} />
          </button>
        </div>
      </div>

      {/* ── Chain integrity note ───────────────────────────────────────── */}
      <p className="text-xs text-slate-400">
        Each entry hash is SHA-256(previous_hash + decision fields).  Run{' '}
        <code className="font-mono bg-slate-100 px-1 rounded">GET /api/v1/audit/verify</code>{' '}
        to replay and validate the full chain server-side.
      </p>
    </div>
  )
}
