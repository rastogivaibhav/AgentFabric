import { Outlet, NavLink } from 'react-router-dom'
import {
  LayoutDashboard, Activity, Radio,
  Bot, DollarSign, Server, Zap, LogOut, User, Users, ClipboardList, KeyRound, SlidersHorizontal
  , Shield
  , FlaskConical
  , FileText
  , GitMerge
  , Lightbulb
  , Brain
  , TestTube
  , ListTree
  , AlertTriangle
} from 'lucide-react'
import { useAuth } from '../hooks/auth'

// Role badge colours — kept subtle to match the dark theme.
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
  adminOnly?: true
}

const NAV: NavItem[] = [
  { to: '/dashboard',    icon: LayoutDashboard, label: 'Dashboard'    },
  { to: '/live',         icon: Radio,           label: 'Live Stream',  badge: 'LIVE' },
  { to: '/traces',       icon: Activity,        label: 'Traces'        },
  { to: '/runs',         icon: ListTree,        label: 'Runs'          },
  { to: '/agents',       icon: Bot,             label: 'Agents'        },
  { to: '/cost',         icon: DollarSign,      label: 'Cost'          },
  { to: '/analytics/errors', icon: AlertTriangle, label: 'Error Analytics' },
  { to: '/environments', icon: Server,          label: 'Environments'  },
  // Admin-only nav items — hidden from editors and viewers.
  { to: '/users',        icon: Users,          label: 'Users',      adminOnly: true },
  { to: '/keys',         icon: KeyRound,       label: 'API Keys',   adminOnly: true },
  { to: '/policies',     icon: Shield,         label: 'Policies',   adminOnly: true },
  { to: '/prompts',      icon: FileText,       label: 'Prompts',    adminOnly: true },
  { to: '/evals',        icon: FlaskConical,   label: 'Evals',      adminOnly: true },
  { to: '/pricing',      icon: SlidersHorizontal, label: 'Pricing', adminOnly: true },
  { to: '/rollouts',        icon: GitMerge,   label: 'Rollouts',            adminOnly: true },
  { to: '/recommendations',  icon: Lightbulb,  label: 'Recommendations',     adminOnly: true },
  { to: '/memory',           icon: Brain,      label: 'Enterprise Memory',   adminOnly: true },
  { to: '/policies/simulate',icon: TestTube,   label: 'Policy Simulation',   adminOnly: true },
  { to: '/decisions',        icon: ClipboardList, label: 'Decisions',        adminOnly: true },
  { to: '/audit',        icon: ClipboardList,  label: 'Audit Log',  adminOnly: true },
]

export default function Layout() {
  const { user, logout } = useAuth()
  const isAdmin = user?.role === 'admin'

  // Filter out admin-only items for non-admins.
  const visibleNav = NAV.filter(item => !item.adminOnly || isAdmin)

  const roleBadge = user?.role ? (ROLE_BADGE[user.role] ?? ROLE_BADGE.viewer) : null

  return (
    <div style={{ display:'flex', height:'100vh', background:'#080C18', color:'#E2E8F0', fontFamily:"'JetBrains Mono',monospace", overflow:'hidden' }}>
      {/* Sidebar */}
      <aside style={{ width:220, background:'#060A14', borderRight:'1px solid #0F1F35', display:'flex', flexDirection:'column', flexShrink:0 }}>
        {/* Logo */}
        <div style={{ padding:'20px 16px', borderBottom:'1px solid #0F1F35' }}>
          <div style={{ display:'flex', alignItems:'center', gap:10 }}>
            <div style={{ width:32, height:32, background:'linear-gradient(135deg,#3B82F6,#8B5CF6)', borderRadius:8, display:'flex', alignItems:'center', justifyContent:'center', fontSize:16 }}>
              <Zap size={16} color="#fff" />
            </div>
            <div>
              <div style={{ fontSize:14, fontWeight:700, color:'#F0F9FF', letterSpacing:'0.05em' }}>AgentFabric</div>
              <div style={{ fontSize:9, color:'#334155', letterSpacing:'0.15em' }}>OBSERVABILITY</div>
            </div>
          </div>
        </div>

        {/* Nav */}
        <nav style={{ flex:1, padding:'16px 8px', overflowY:'auto' }}>
          {visibleNav.map(({ to, icon: Icon, label, badge }) => (
            <NavLink key={to} to={to} style={({ isActive }) => ({
              display:'flex', alignItems:'center', gap:10,
              padding:'9px 12px', borderRadius:6, marginBottom:2,
              textDecoration:'none', fontSize:12,
              background: isActive ? 'linear-gradient(90deg,#1E3A5F,transparent)' : 'transparent',
              color: isActive ? '#60A5FA' : '#475569',
              borderLeft: isActive ? '2px solid #3B82F6' : '2px solid transparent',
              transition:'all 0.15s',
            })}>
              <Icon size={15} />
              <span style={{ flex:1 }}>{label}</span>
              {badge && (
                <span style={{ fontSize:9, padding:'1px 5px', background:'#EF444420', color:'#EF4444', borderRadius:3, letterSpacing:'0.1em' }}>
                  {badge}
                </span>
              )}
            </NavLink>
          ))}
        </nav>

        {/* User info + role badge + logout */}
        <div style={{ padding:'12px 16px', borderTop:'1px solid #0F1F35' }}>
          {user ? (
            <div style={{ display:'flex', alignItems:'center', gap:8 }}>
              <div style={{ width:24, height:24, background:'#1E3A5F', borderRadius:'50%', display:'flex', alignItems:'center', justifyContent:'center', flexShrink:0 }}>
                <User size={12} color="#60A5FA" />
              </div>
              <div style={{ flex:1, minWidth:0 }}>
                <div style={{ fontSize:11, color:'#94A3B8', overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }}>
                  {user.email || user.name || user.sub}
                </div>
                {/* Role badge — shows Admin / Editor / Viewer */}
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
            <div style={{ fontSize:10, color:'#1E3A5F' }}>v1.0.0 · © AgentFabric</div>
          )}
        </div>
      </aside>

      {/* Main */}
      <main style={{ flex:1, overflowY:'auto', display:'flex', flexDirection:'column' }}>
        <Outlet />
      </main>
    </div>
  )
}
