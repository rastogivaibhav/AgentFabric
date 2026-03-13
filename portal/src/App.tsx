import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import TracesPage from './pages/TracesPage'
import TraceDetail from './pages/TraceDetail'
import LiveStream from './pages/LiveStream'
import AgentsPage from './pages/AgentsPage'
import CostPage from './pages/CostPage'
import EnvironmentsPage from './pages/EnvironmentsPage'
import LoginPage from './pages/LoginPage'
import { useAuth, isAuthEnabled } from './hooks/auth'

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
            <Route path="traces/:traceId" element={<TraceDetail />} />
            <Route path="live" element={<LiveStream />} />
            <Route path="agents" element={<AgentsPage />} />
            <Route path="cost" element={<CostPage />} />
            <Route path="environments" element={<EnvironmentsPage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  )
}
