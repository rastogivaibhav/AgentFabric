import { useState } from 'react'
import { Archive, ChevronLeft, ChevronRight, Download, History, Lock, RefreshCw } from 'lucide-react'
import { hasRole, useAuth } from '../hooks/auth'
import { useControlAudit, useControlHistory, useCreateEvidenceBundle, useEvidenceBundles, type AdminAuditEntry } from '../hooks/api'

const PAGE_SIZE = 100

function truncate(value: string, len = 8): string {
  if (!value) return '-'
  return value.length <= len ? value : `${value.slice(0, len)}...`
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return iso
  }
}

const OUTCOME_CONFIG = {
  success: { bg: 'bg-emerald-100', text: 'text-emerald-700', border: 'border-emerald-200' },
  error: { bg: 'bg-red-100', text: 'text-red-700', border: 'border-red-200' },
  failed: { bg: 'bg-red-100', text: 'text-red-700', border: 'border-red-200' },
  blocked: { bg: 'bg-amber-100', text: 'text-amber-700', border: 'border-amber-200' },
} as const

function OutcomeBadge({ outcome }: { outcome: AdminAuditEntry['outcome'] }) {
  const cfg = OUTCOME_CONFIG[outcome as keyof typeof OUTCOME_CONFIG] ?? {
    bg: 'bg-slate-100',
    text: 'text-slate-700',
    border: 'border-slate-200',
  }
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-semibold border ${cfg.bg} ${cfg.text} ${cfg.border}`}>
      {outcome || 'unknown'}
    </span>
  )
}

function HashCell({ hash }: { hash: string }) {
  if (!hash) return <span className="text-slate-400 text-xs">-</span>
  return (
    <span title={hash} className="font-mono text-xs text-slate-500 cursor-help">
      {hash.slice(0, 12)}
    </span>
  )
}

function AccessDenied() {
  return (
    <div className="flex flex-col items-center justify-center h-64 gap-4 text-slate-500">
      <Lock size={40} className="text-slate-300" />
      <p className="text-lg font-semibold">Access denied</p>
      <p className="text-sm">The audit log is restricted to administrators.</p>
    </div>
  )
}

export default function AuditPage() {
  const { user } = useAuth()
  const isAdmin = hasRole(user, ['admin'])
  const [offset, setOffset] = useState(0)
  const [bundleForm, setBundleForm] = useState({
    name: 'Rollout incident bundle',
    scope: 'incident',
    release_tag: '',
    prompt_id: '',
    environment: '',
    trace_id: '',
    reason: '',
  })

  const auditQuery = useControlAudit(PAGE_SIZE)
  const historyQuery = useControlHistory(25, offset)
  const bundlesQuery = useEvidenceBundles(12)
  const createBundle = useCreateEvidenceBundle()

  if (!isAdmin) return <AccessDenied />

  const auditEntries: AdminAuditEntry[] = auditQuery.data?.items ?? []
  const historyEntries = historyQuery.data?.items ?? []
  const bundles = bundlesQuery.data?.items ?? []
  const hasPrev = offset > 0
  const hasNext = Boolean(historyQuery.data?.has_more)

  const refreshAll = () => {
    void auditQuery.refetch()
    void historyQuery.refetch()
    void bundlesQuery.refetch()
  }

  const handleBundleSubmit = async () => {
    await createBundle.mutateAsync({
      name: bundleForm.name,
      scope: bundleForm.scope,
      release_tag: bundleForm.release_tag || undefined,
      prompt_id: bundleForm.prompt_id || undefined,
      environment: bundleForm.environment || undefined,
      trace_id: bundleForm.trace_id || undefined,
      reason: bundleForm.reason || undefined,
    })
  }

  const loading = auditQuery.isLoading || historyQuery.isLoading || bundlesQuery.isLoading
  const errored = auditQuery.isError || historyQuery.isError || bundlesQuery.isError

  return (
    <div className="p-6 space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Audit Log</h1>
          <p className="mt-1 text-sm text-slate-500">
            Recent control-plane audit records, append-only control history, and exportable evidence bundles.
          </p>
        </div>
        <button
          onClick={refreshAll}
          disabled={auditQuery.isFetching || historyQuery.isFetching || bundlesQuery.isFetching}
          className="flex items-center gap-2 px-3 py-2 text-sm rounded-lg border border-slate-200 text-slate-600 hover:bg-slate-50 disabled:opacity-50 transition-colors"
        >
          <RefreshCw size={14} className={auditQuery.isFetching || historyQuery.isFetching || bundlesQuery.isFetching ? 'animate-spin' : ''} />
          Refresh
        </button>
      </div>

      {errored && (
        <div className="rounded-lg bg-red-50 border border-red-200 px-4 py-3 text-sm text-red-700">
          Failed to load one or more enterprise memory views. Check your connection and ensure your session is still valid.
        </div>
      )}

      <div className="grid gap-4 md:grid-cols-3">
        <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <p className="text-xs uppercase tracking-wide text-slate-400">Control Audit</p>
          <p className="mt-2 text-2xl font-semibold text-slate-900">{auditEntries.length}</p>
          <p className="mt-1 text-sm text-slate-500">Recent control-plane audit records in the current view.</p>
        </div>
        <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <p className="text-xs uppercase tracking-wide text-slate-400">Control History</p>
          <p className="mt-2 text-2xl font-semibold text-slate-900">{historyQuery.data?.total ?? historyEntries.length}</p>
          <p className="mt-1 text-sm text-slate-500">Append-only record of control-plane changes across prompts, policy, pricing, auth, rollouts, and recommendations.</p>
        </div>
        <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
          <p className="text-xs uppercase tracking-wide text-slate-400">Evidence Bundles</p>
          <p className="mt-2 text-2xl font-semibold text-slate-900">{bundles.length}</p>
          <p className="mt-1 text-sm text-slate-500">Incident-ready exports with linked releases, rollouts, evals, decisions, and recommendations.</p>
        </div>
      </div>

      <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm space-y-4">
        <div className="flex items-center gap-2">
          <Archive size={18} className="text-slate-500" />
          <div>
            <h2 className="text-lg font-semibold text-slate-900">Evidence Bundle Export</h2>
            <p className="text-sm text-slate-500">Capture a rollout or release incident into a tenant-scoped export bundle.</p>
          </div>
        </div>

        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          <label className="space-y-1 text-sm text-slate-600">
            <span>Bundle name</span>
            <input className="w-full rounded-lg border border-slate-200 px-3 py-2" value={bundleForm.name} onChange={e => setBundleForm(cur => ({ ...cur, name: e.target.value }))} />
          </label>
          <label className="space-y-1 text-sm text-slate-600">
            <span>Scope</span>
            <select className="w-full rounded-lg border border-slate-200 px-3 py-2" value={bundleForm.scope} onChange={e => setBundleForm(cur => ({ ...cur, scope: e.target.value }))}>
              <option value="incident">Incident</option>
              <option value="release">Release review</option>
              <option value="audit">Audit package</option>
            </select>
          </label>
          <label className="space-y-1 text-sm text-slate-600">
            <span>Release tag</span>
            <input className="w-full rounded-lg border border-slate-200 px-3 py-2" value={bundleForm.release_tag} onChange={e => setBundleForm(cur => ({ ...cur, release_tag: e.target.value }))} placeholder="candidate-v7" />
          </label>
          <label className="space-y-1 text-sm text-slate-600">
            <span>Prompt ID</span>
            <input className="w-full rounded-lg border border-slate-200 px-3 py-2" value={bundleForm.prompt_id} onChange={e => setBundleForm(cur => ({ ...cur, prompt_id: e.target.value }))} placeholder="support.system" />
          </label>
          <label className="space-y-1 text-sm text-slate-600">
            <span>Environment</span>
            <input className="w-full rounded-lg border border-slate-200 px-3 py-2" value={bundleForm.environment} onChange={e => setBundleForm(cur => ({ ...cur, environment: e.target.value }))} placeholder="staging" />
          </label>
          <label className="space-y-1 text-sm text-slate-600">
            <span>Trace ID</span>
            <input className="w-full rounded-lg border border-slate-200 px-3 py-2" value={bundleForm.trace_id} onChange={e => setBundleForm(cur => ({ ...cur, trace_id: e.target.value }))} placeholder="trace-abc123" />
          </label>
        </div>

        <label className="space-y-1 text-sm text-slate-600 block">
          <span>Reason</span>
          <textarea className="w-full rounded-lg border border-slate-200 px-3 py-2 min-h-24" value={bundleForm.reason} onChange={e => setBundleForm(cur => ({ ...cur, reason: e.target.value }))} placeholder="Rollout auto-paused after error rate breach in staging." />
        </label>

        <div className="flex items-center gap-3">
          <button
            onClick={() => void handleBundleSubmit()}
            disabled={createBundle.isPending}
            className="inline-flex items-center gap-2 rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-50"
          >
            <Archive size={14} />
            {createBundle.isPending ? 'Creating…' : 'Create Evidence Bundle'}
          </button>
          {createBundle.isError && (
            <span className="text-sm text-red-600">Bundle creation failed. Check the filters and try again.</span>
          )}
          {createBundle.isSuccess && (
            <a
              href={`${import.meta.env.VITE_API_URL ?? 'http://localhost:8080'}/api/v1/audit/evidence-bundles/${createBundle.data.id}/export`}
              className="inline-flex items-center gap-2 text-sm text-indigo-600 hover:text-indigo-800 hover:underline"
            >
              <Download size={14} />
              Download latest bundle
            </a>
          )}
        </div>
      </section>

      <section className="rounded-xl border border-slate-200 bg-white shadow-sm overflow-hidden">
        <div className="flex items-center gap-2 border-b border-slate-200 px-5 py-4">
          <History size={18} className="text-slate-500" />
          <div>
            <h2 className="text-lg font-semibold text-slate-900">Control History Timeline</h2>
            <p className="text-sm text-slate-500">Immutable provenance for control-plane changes.</p>
          </div>
        </div>
        <div className="divide-y divide-slate-100">
          {loading
            ? Array.from({ length: 4 }).map((_, index) => (
                <div key={index} className="animate-pulse px-5 py-4 space-y-2">
                  <div className="h-4 w-48 rounded bg-slate-100" />
                  <div className="h-4 w-80 rounded bg-slate-100" />
                </div>
              ))
            : historyEntries.length === 0
              ? <div className="px-5 py-10 text-sm text-slate-400">No control history has been recorded for this tenant yet.</div>
              : historyEntries.map(entry => (
                  <div key={entry.id} className="px-5 py-4 space-y-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="inline-flex rounded-full bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-700">{entry.category}</span>
                      <span className="inline-flex rounded-full bg-slate-50 px-2 py-0.5 text-xs font-medium text-slate-500">{entry.action}</span>
                      <span className="text-sm font-medium text-slate-800">{entry.target_type}:{' '}{entry.target_id || '-'}</span>
                      <span className="text-xs text-slate-400">{formatDate(entry.created_at)}</span>
                    </div>
                    <p className="text-sm text-slate-600">{entry.reason || 'No explicit change reason recorded.'}</p>
                    <div className="flex flex-wrap items-center gap-4 text-xs text-slate-500">
                      <span>Actor: {entry.actor || '-'}</span>
                      <span>Outcome: {entry.outcome}</span>
                      <span>Chain: <HashCell hash={entry.entry_hash || ''} /></span>
                    </div>
                  </div>
                ))}
        </div>
        <div className="flex items-center justify-between border-t border-slate-200 px-5 py-3 text-sm text-slate-500">
          <span>Showing control history rows {offset + 1}-{offset + historyEntries.length}</span>
          <div className="flex gap-2">
            <button
              onClick={() => setOffset(Math.max(0, offset - 25))}
              disabled={!hasPrev || historyQuery.isFetching}
              className="flex items-center gap-1 px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              <ChevronLeft size={14} /> Previous
            </button>
            <button
              onClick={() => setOffset(offset + 25)}
              disabled={!hasNext || historyQuery.isFetching}
              className="flex items-center gap-1 px-3 py-1.5 rounded-lg border border-slate-200 hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Next <ChevronRight size={14} />
            </button>
          </div>
        </div>
      </section>

      <section className="rounded-xl border border-slate-200 bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between border-b border-slate-200 px-5 py-4">
          <div>
            <h2 className="text-lg font-semibold text-slate-900">Policy Audit Chain</h2>
            <p className="text-sm text-slate-500">Recent legacy control-plane audit records captured alongside enterprise memory history.</p>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50">
              <tr>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">#</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Timestamp</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Category</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Action</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Target</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Actor</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600 whitespace-nowrap">Outcome</th>
                <th className="px-4 py-3 text-left font-semibold text-slate-600">Details</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 bg-white">
              {loading
                ? Array.from({ length: 6 }).map((_, index) => (
                    <tr key={index} className="animate-pulse">
                      {Array.from({ length: 8 }).map((__, cell) => (
                        <td key={cell} className="px-4 py-3">
                          <div className="h-4 bg-slate-100 rounded w-20" />
                        </td>
                      ))}
                    </tr>
                  ))
                : auditEntries.length === 0
                  ? (
                    <tr>
                      <td colSpan={8} className="px-4 py-12 text-center text-slate-400">
                        No control audit entries found for this tenant.
                      </td>
                    </tr>
                  )
                  : auditEntries.map(entry => (
                    <tr key={entry.id} className="hover:bg-slate-50 transition-colors">
                      <td className="px-4 py-3 text-slate-400 font-mono text-xs">{entry.id}</td>
                      <td className="px-4 py-3 text-slate-600 whitespace-nowrap">{formatDate(entry.created_at)}</td>
                      <td className="px-4 py-3">
                        <span className="inline-flex rounded-full bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-700">
                          {entry.category}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-slate-700 font-medium">{entry.action}</td>
                      <td className="px-4 py-3">
                        <span
                          title={entry.target_id ? `${entry.target_type}:${entry.target_id}` : entry.target_type}
                          className="font-mono text-xs text-slate-500"
                        >
                          {entry.target_id ? `${entry.target_type}:${truncate(entry.target_id, 16)}` : entry.target_type}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-slate-600">{entry.actor || '-'}</td>
                      <td className="px-4 py-3 text-slate-600 max-w-xs">
                        <OutcomeBadge outcome={entry.outcome} />
                      </td>
                      <td className="px-4 py-3 text-slate-600 max-w-md">
                        <span title={entry.details || ''} className="line-clamp-2 block">
                          {entry.details || '-'}
                        </span>
                      </td>
                    </tr>
                  ))}
            </tbody>
          </table>
        </div>
      </section>

      <section className="rounded-xl border border-slate-200 bg-white p-5 shadow-sm space-y-3">
        <div className="flex items-center gap-2">
          <Archive size={18} className="text-slate-500" />
          <div>
            <h2 className="text-lg font-semibold text-slate-900">Recent Evidence Bundles</h2>
            <p className="text-sm text-slate-500">Tenant-scoped exports for incident review and compliance handoff.</p>
          </div>
        </div>

        {bundles.length === 0 ? (
          <div className="rounded-lg border border-dashed border-slate-200 px-4 py-8 text-sm text-slate-400">
            No evidence bundles have been generated yet.
          </div>
        ) : (
          <div className="grid gap-3 lg:grid-cols-2">
            {bundles.map(bundle => (
              <div key={bundle.id} className="rounded-xl border border-slate-200 p-4">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <h3 className="font-semibold text-slate-900">{bundle.name}</h3>
                    <p className="text-sm text-slate-500">{bundle.scope} · {bundle.item_count} items · {formatDate(bundle.created_at)}</p>
                  </div>
                  <a
                    href={`${import.meta.env.VITE_API_URL ?? 'http://localhost:8080'}/api/v1/audit/evidence-bundles/${bundle.id}/export`}
                    className="inline-flex items-center gap-1 text-sm text-indigo-600 hover:text-indigo-800 hover:underline"
                  >
                    <Download size={14} />
                    Export
                  </a>
                </div>
                {bundle.summary && bundle.summary.length > 0 && (
                  <ul className="mt-3 space-y-1 text-sm text-slate-600">
                    {bundle.summary.slice(0, 3).map(line => (
                      <li key={line}>{line}</li>
                    ))}
                  </ul>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      <p className="text-xs text-slate-400">
        Control-plane records and enterprise memory exports are tenant-scoped.
        Policy entry verification remains available via <code className="font-mono bg-slate-100 px-1 rounded">GET /api/v1/audit/verify</code>.
      </p>
    </div>
  )
}
