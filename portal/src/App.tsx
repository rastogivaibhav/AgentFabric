import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import TracesPage from './pages/TracesPage'
import TraceDetail from './pages/TraceDetail'
import TraceComparePage from './pages/TraceComparePage'
import LiveStream from './pages/LiveStream'
import AgentsPage from './pages/AgentsPage'
import AgentDetailPage from './pages/AgentDetailPage'
import RunsPage from './pages/RunsPage'
import RunDetailPage from './pages/RunDetailPage'
import Analytics from './pages/Analytics'
import Governance from './pages/Governance'
import ErrorAnalyticsPage from './pages/ErrorAnalyticsPage'
import CostPage from './pages/CostPage'
import EnvironmentsPage from './pages/EnvironmentsPage'
import UsersPage from './pages/UsersPage'
import AuditPage from './pages/AuditPage'
import DecisionsPage from './pages/DecisionsPage'
import ApiKeysPage from './pages/ApiKeysPage'
import PricingRulesPage from './pages/PricingRulesPage'
import PoliciesPage from './pages/PoliciesPage'
import EvalsPage from './pages/EvalsPage'
import RegressionPage from './pages/RegressionPage'
import PromptsPage from './pages/PromptsPage'
import PromptReleasePage from './pages/PromptReleasePage'
import RolloutsPage from './pages/RolloutsPage'
import ReleaseControlPage from './pages/ReleaseControlPage'
import PolicySimulationPage from './pages/PolicySimulationPage'
import RecommendationsPage from './pages/RecommendationsPage'
import MemoryPage from './pages/MemoryPage'
import LoginPage from './pages/LoginPage'
import { useAuth, isAuthEnabled, hasRole } from './hooks/auth'

const qc = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: 2,
      refetchOnWindowFocus: false,
    },
  },
})

// RequireAuth: redirect to /login if not authenticated (and auth is enabled).
function RequireAuth({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth()
  if (!isAuthEnabled()) return <>{children}</>
  if (isLoading) return null // brief flicker guard
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

// RequireRole: renders children only when the current user holds one of roles.
// Returns fallback (default: null) when the check fails — the API enforces 403
// regardless, but hiding the element removes the invitation to attempt the action.
// Usage: <RequireRole roles={['admin']}><DeleteButton /></RequireRole>
export function RequireRole({
  roles,
  children,
  fallback = null,
}: {
  roles: string[]
  children: React.ReactNode
  fallback?: React.ReactNode
}) {
  const { user } = useAuth()
  return hasRole(user, roles) ? <>{children}</> : <>{fallback}</>
}

export default function App() {
  return (
    <QueryClientProvider client={qc}>
      <BrowserRouter>
        <Routes>
          {/* Public routes */}
          <Route path="login" element={<LoginPage />} />

          {/* Protected routes — wrapped in RequireAuth */}
          <Route element={
            <RequireAuth>
              <Layout />
            </RequireAuth>
          }>
            <Route index element={<Navigate to="/dashboard" replace />} />
            <Route path="dashboard" element={<Dashboard />} />
            <Route path="traces" element={<TracesPage />} />
            <Route path="traces/compare" element={<TraceComparePage />} />
            <Route path="traces/:traceId" element={<TraceDetail />} />
            <Route path="live" element={<LiveStream />} />
            <Route path="agents" element={<AgentsPage />} />
            <Route path="agents/:agentId" element={<AgentDetailPage />} />
            <Route path="runs" element={<RunsPage />} />
            <Route path="runs/:runId" element={<RunDetailPage />} />
            <Route path="analytics" element={<Analytics />} />
            <Route path="analytics/errors" element={<ErrorAnalyticsPage />} />
            <Route path="cost" element={<CostPage />} />
            <Route path="environments" element={<EnvironmentsPage />} />
            <Route path="users" element={<UsersPage />} />
            <Route path="audit" element={<AuditPage />} />
            <Route path="governance" element={<Governance />} />
            <Route path="decisions" element={<DecisionsPage />} />
            <Route path="keys" element={<ApiKeysPage />} />
            <Route path="pricing" element={<PricingRulesPage />} />
            <Route path="prompts" element={<PromptsPage />} />
            <Route path="prompts/:promptId" element={<PromptReleasePage />} />
            <Route path="policies" element={<PoliciesPage />} />
            <Route path="evals" element={<EvalsPage />} />
            <Route path="evals/regressions" element={<RegressionPage />} />
            <Route path="release-control" element={<ReleaseControlPage />} />
            <Route path="rollouts" element={<RolloutsPage />} />
            <Route path="policies/simulate" element={<PolicySimulationPage />} />
            <Route path="recommendations" element={<RecommendationsPage />} />
            <Route path="memory" element={<MemoryPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
