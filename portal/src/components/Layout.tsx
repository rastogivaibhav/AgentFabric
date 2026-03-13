import { Outlet, NavLink, useLocation } from 'react-router-dom'
import {
  LayoutDashboard, Activity, Radio,
  Bot, DollarSign, Server, Zap, LogOut, User
} from 'lucide-react'
import { useAuth } from '../hooks/auth'

const NAV = [
  { to: '/dashboard',    icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/live',         icon: Radio,           label: 'Live Stream', badge: 'LIVE' },
  { to: '/traces',       icon: Activity,        label: 'Traces' },
  { to: '/agents',       icon: Bot,             label: 'Agents' },
  { to: '/cost',         icon: DollarSign,      label: 'Cost' },
  { to: '/environments', icon: Server,          label: 'Environments' },
]

export default function Layout() {
  const { user, logout } = useAuth()

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
          {NAV.map(({ to, icon: Icon, label, badge }) => (
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

        {/* User info + logout */}
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
