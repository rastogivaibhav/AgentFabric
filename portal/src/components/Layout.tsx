import { useEffect, useState } from 'react'
import { Outlet, NavLink, useLocation } from 'react-router-dom'
import {
  LayoutDashboard, Activity, Radio,
  Bot, DollarSign, Server, Zap, LogOut, User, Users, ClipboardList, KeyRound, SlidersHorizontal,
  Shield, FlaskConical, FileText, GitMerge, Lightbulb, Brain, TestTube, ListTree, AlertTriangle, ChevronRight, Settings, BarChart3, ClipboardCheck
} from 'lucide-react'
import { useAuth, isAuthEnabled } from '../hooks/auth'

const ROLE_BADGE: Record<string, { bg: string; color: string; label: string }> = {
  admin:  { bg: '#EF444420', color: '#EF4444', label: 'Admin'  },
  editor: { bg: '#F59E0B20', color: '#F59E0B', label: 'Editor' },
  viewer: { bg: '#47556920', color: '#64748B', label: 'Viewer' },
}

interface NavItem {
  to: string
  icon: React.ElementType
  label: string
  badge?: string
}

interface NavGroup {
  id: string
  label: string
  color: string
  adminOnly?: boolean
  items: NavItem[]
}

const NAV_GROUPS: NavGroup[] = [
  {
    id: 'overview',
    label: 'OVERVIEW',
    color: 'transparent',
    items: [
      { to: '/dashboard', icon: LayoutDashboard, label: 'Dashboard' },
      { to: '/live', icon: Radio, label: 'Live Stream', badge: 'LIVE' },
    ]
  },
  {
    id: 'protect',
    label: 'PROTECT',
    color: 'var(--protect, #FF453A)',
    items: [
      { to: '/governance', icon: AlertTriangle, label: 'Governance' },
      { to: '/policies', icon: Shield, label: 'Policies' },
      { to: '/decisions', icon: ClipboardList, label: 'Decisions' },
      { to: '/policies/simulate', icon: TestTube, label: 'Policy Simulation' },
    ]
  },
  {
    id: 'control',
    label: 'CONTROL',
    color: 'var(--control, #0A84FF)',
    items: [
      { to: '/rollouts', icon: GitMerge, label: 'Rollouts' },
      { to: '/environments', icon: Server, label: 'Environments' },
    ]
  },
  {
    id: 'spend',
    label: 'SPEND',
    color: 'var(--spend, #30D158)',
    items: [
      { to: '/cost', icon: DollarSign, label: 'Cost & Budget' },
      { to: '/pricing', icon: SlidersHorizontal, label: 'Pricing' },
      { to: '/recommendations', icon: Lightbulb, label: 'Recommendations' },
    ]
  },
  {
    id: 'observe',
    label: 'OBSERVE',
    color: 'var(--observe, #FFD60A)',
    items: [
      { to: '/traces', icon: Activity, label: 'Traces' },
      { to: '/runs', icon: ListTree, label: 'Runs' },
      { to: '/agents', icon: Bot, label: 'Agents' },
      { to: '/analytics', icon: BarChart3, label: 'Analytics' },
      { to: '/analytics/errors', icon: AlertTriangle, label: 'Error Analytics' },
    ]
  },
  {
    id: 'ship',
    label: 'SHIP',
    color: 'var(--ship, #BF5AF2)',
    items: [
      { to: '/release-control', icon: ClipboardCheck, label: 'Release Control' },
      { to: '/prompts', icon: FileText, label: 'Prompts' },
      { to: '/evals', icon: FlaskConical, label: 'Evals' },
    ]
  },
  {
    id: 'prove',
    label: 'PROVE',
    color: 'var(--prove, #FF9F0A)',
    adminOnly: true,
    items: [
      { to: '/memory', icon: Brain, label: 'Enterprise Memory' },
      { to: '/audit', icon: ClipboardList, label: 'Audit Log' },
    ]
  }
]

const ADMIN_ITEMS: NavItem[] = [
  { to: '/keys', icon: KeyRound, label: 'API Keys' },
  { to: '/users', icon: Users, label: 'Users' },
]

function NavGroupSection({ 
  group, 
  isAdmin,
  isExpanded,
  onToggle
}: { 
  group: NavGroup, 
  isAdmin: boolean,
  isExpanded: boolean,
  onToggle: () => void 
}) {
  const location = useLocation()
  
  if (group.adminOnly && !isAdmin) return null
  
  // Also filter items within groups that might be admin only if we added them, 
  // but currently we handled adminOnly at the group level or pinned section except for protect, control, spend which might have mixed. 
  // Actually, wait, Policies, Pricing, Rollouts, Decisions are mostly admin only. The original code hid them block-wise.
  // Original admin only items: users, keys, policies, prompts, evals, pricing, rollouts, recommendations, memory, policies/simulate, decisions, audit.
  // So PROTECT, CONTROL, SPEND, SHIP, PROVE are heavily admin.
  // We need to filter items in the group based on whether they were admin only.
  const adminOnlyPaths = ['/users', '/keys', '/policies', '/prompts', '/evals', '/release-control', '/pricing', '/rollouts', '/recommendations', '/memory', '/policies/simulate', '/decisions', '/audit', '/governance']
  
  const visibleItems = group.items.filter(item => {
    if (!isAdmin && adminOnlyPaths.includes(item.to)) return false
    return true
  })

  if (visibleItems.length === 0) return null

  const isActiveGroup = visibleItems.some(item => location.pathname.startsWith(item.to))

  return (
    <div style={{ marginBottom: 12 }}>
      <button 
        onClick={onToggle}
        style={{
          width: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          background: 'transparent',
          border: 'none',
          padding: '6px 16px',
          cursor: 'pointer',
          color: isActiveGroup ? '#E2E8F0' : '#475569',
          fontSize: 10,
          fontWeight: 700,
          letterSpacing: '0.1em'
        }}
      >
        <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {group.color !== 'transparent' && (
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: group.color, display: 'inline-block' }} />
          )}
          {group.label}
        </span>
        <ChevronRight 
          size={12} 
          style={{ 
            transform: isExpanded ? 'rotate(90deg)' : 'rotate(0deg)',
            transition: 'transform 0.2s',
            color: '#475569'
          }} 
        />
      </button>

      <div style={{ 
        overflow: 'hidden', 
        height: isExpanded ? 'auto' : 0,
        opacity: isExpanded ? 1 : 0,
        transition: 'all 0.2s ease-in-out'
      }}>
        <div style={{ padding: '4px 8px' }}>
          {visibleItems.map(({ to, icon: Icon, label, badge }) => (
            <NavLink key={to} to={to} style={({ isActive }) => ({
              display:'flex', alignItems:'center', gap:10,
              padding:'8px 12px', borderRadius:6, marginBottom:2,
              textDecoration:'none', fontSize:12,
              background: isActive ? 'var(--layer-2)' : 'transparent',
              color: isActive ? '#F0F9FF' : '#94A3B8',
              borderLeft: isActive ? `2px solid ${group.color === 'transparent' ? '#3B82F6' : group.color}` : '2px solid transparent',
              transition:'all 0.15s',
            })}>
              {({ isActive }) => (
                <>
                  <Icon size={15} style={{ color: isActive ? (group.color === 'transparent' ? '#60A5FA' : group.color) : '#475569' }} />
                  <span style={{ flex:1 }}>{label}</span>
                  {badge && (
                    <span style={{ fontSize:9, padding:'1px 5px', background:'#EF444420', color:'#EF4444', borderRadius:3, letterSpacing:'0.1em' }}>
                      {badge}
                    </span>
                  )}
                </>
              )}
            </NavLink>
          ))}
        </div>
      </div>
    </div>
  )
}

export default function Layout() {
  const { user, logout } = useAuth()
  const isAdmin = user?.role === 'admin' || !isAuthEnabled()
  const location = useLocation()
  const [viewportWidth, setViewportWidth] = useState(() => typeof window === 'undefined' ? 1024 : window.innerWidth)
  const hideSidebar = viewportWidth < 760

  useEffect(() => {
    const onResize = () => setViewportWidth(window.innerWidth)
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])
  
  // Default to expanded for Overview, and whatever group is currently active
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>(() => {
    const initialState: Record<string, boolean> = { overview: true }
    NAV_GROUPS.forEach(group => {
      if (group.items.some(item => location.pathname.startsWith(item.to))) {
        initialState[group.id] = true
      }
    })
    return initialState
  })

  const toggleGroup = (id: string) => {
    setExpandedGroups(prev => ({ ...prev, [id]: !prev[id] }))
  }

  const roleBadge = user?.role ? (ROLE_BADGE[user.role] ?? ROLE_BADGE.viewer) : null

  return (
    <div style={{ display:'flex', height:'100vh', background:'var(--layer-1)', color:'var(--text-primary)', overflow:'hidden' }}>
      {/* Sidebar with frosted glass look */}
      <aside style={{ 
        width: 220, 
        background: 'rgba(10, 10, 10, 0.8)', 
        backdropFilter: 'blur(20px)',
        borderRight: '1px solid var(--layer-border)', 
        display: hideSidebar ? 'none' : 'flex', 
        flexDirection: 'column', 
        flexShrink: 0 
      }}>
        {/* Logo */}
        <div style={{ padding:'20px 16px', borderBottom:'1px solid var(--layer-border)' }}>
          <div style={{ display:'flex', alignItems:'center', gap:10 }}>
            <div style={{ width:32, height:32, background:'linear-gradient(135deg,#3B82F6,#8B5CF6)', borderRadius:8, display:'flex', alignItems:'center', justifyContent:'center', fontSize:16, color: '#fff' }}>
              <Zap size={16} />
            </div>
            <div>
              <div style={{ fontSize:14, fontWeight:700, color:'#F0F9FF', letterSpacing:'0.05em' }}>Govagn</div>
              <div style={{ fontSize:9, color:'var(--text-secondary)', letterSpacing:'0.15em' }}>GATEWAY</div>
            </div>
          </div>
        </div>

        {/* Nav */}
        <nav style={{ flex:1, padding:'16px 0', overflowY:'auto' }}>
          {NAV_GROUPS.map((group) => (
            <NavGroupSection 
              key={group.id} 
              group={group} 
              isAdmin={isAdmin}
              isExpanded={!!expandedGroups[group.id]}
              onToggle={() => toggleGroup(group.id)}
            />
          ))}
          
          {/* Pinned Admin Section */}
          {isAdmin && (
            <div style={{ marginTop: 16 }}>
              <div style={{ padding: '0 16px', marginBottom: 8, display: 'flex', alignItems: 'center', gap: 6, color: '#475569', fontSize: 10, fontWeight: 700, letterSpacing: '0.1em' }}>
                <Settings size={12} />
                ADMIN
              </div>
              <div style={{ padding: '0 8px' }}>
                {ADMIN_ITEMS.map(({ to, icon: Icon, label }) => (
                  <NavLink key={to} to={to} style={({ isActive }) => ({
                    display:'flex', alignItems:'center', gap:10,
                    padding:'8px 12px', borderRadius:6, marginBottom:2,
                    textDecoration:'none', fontSize:12,
                    background: isActive ? 'var(--layer-2)' : 'transparent',
                    color: isActive ? '#F0F9FF' : '#94A3B8',
                    borderLeft: isActive ? '2px solid #64748B' : '2px solid transparent',
                    transition:'all 0.15s',
                  })}>
                    {({ isActive }) => (
                      <>
                        <Icon size={15} style={{ color: isActive ? '#E2E8F0' : '#475569' }} />
                        <span style={{ flex:1 }}>{label}</span>
                      </>
                    )}
                  </NavLink>
                ))}
              </div>
            </div>
          )}
        </nav>

        {/* User info + role badge + logout */}
        <div style={{ padding:'12px 16px', borderTop:'1px solid var(--layer-border)', background: 'var(--layer-2)' }}>
          {user ? (
            <div style={{ display:'flex', alignItems:'center', gap:8 }}>
              <div style={{ width:24, height:24, background:'#1E3A5F', borderRadius:'50%', display:'flex', alignItems:'center', justifyContent:'center', flexShrink:0 }}>
                <User size={12} color="#60A5FA" />
              </div>
              <div style={{ flex:1, minWidth:0 }}>
                <div style={{ fontSize:11, color:'#94A3B8', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }}>
                  {user.email || user.name || user.sub}
                </div>
                {roleBadge && (
                  <div style={{
                    display: 'inline-block',
                    marginTop: 3,
                    fontSize: 9,
                    fontWeight: 600,
                    letterSpacing: '0.1em',
                    padding: '1px 6px',
                    borderRadius: 3,
                    background: roleBadge.bg,
                    color: roleBadge.color,
                    border: `1px solid ${roleBadge.color}40`,
                  }}>
                    {roleBadge.label.toUpperCase()}
                  </div>
                )}
              </div>
              <button
                onClick={logout}
                title="Sign out"
                style={{ background:'none', border:'none', cursor:'pointer', padding:4, display:'flex', color:'#475569', flexShrink:0 }}
              >
                <LogOut size={13} />
              </button>
            </div>
          ) : (
            <div style={{ fontSize:10, color:'#475569' }}>v1.0.0 · © Govagn</div>
          )}
        </div>
      </aside>

      {/* Main Content Area */}
      <main style={{ flex:1, overflowY:'auto', display:'flex', flexDirection:'column', position: 'relative' }}>
        <Outlet />
      </main>
    </div>
  )
}
