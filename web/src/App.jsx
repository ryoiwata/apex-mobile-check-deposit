import { useState } from 'react'
import InvestorView from './views/InvestorView.jsx'
import OperatorView from './views/OperatorView.jsx'
import SettlementView from './views/SettlementView.jsx'
import AdminView from './views/AdminView.jsx'

const ROLES = [
  { id: 'investor',   label: 'Investor',      icon: '👤', color: '#2563eb', desc: 'Investor View' },
  { id: 'operator',   label: 'Operator',       icon: '🛡️', color: '#d97706', desc: 'Operator View' },
  { id: 'settlement', label: 'Settlement',     icon: '🏦', color: '#059669', desc: 'Settlement View' },
  { id: 'admin',      label: 'System / Admin', icon: '⚙️', color: '#7c3aed', desc: 'System / Admin View' },
]

export default function App() {
  const [activeRole, setActiveRole] = useState('investor')
  const role = ROLES.find(r => r.id === activeRole)

  return (
    <div className="min-h-screen bg-gray-50">
      {/* Role selector */}
      <div style={{
        backgroundColor: '#f8fafc',
        borderBottom: '1px solid #e2e8f0',
        padding: '6px 16px',
        display: 'flex',
        gap: 4,
        alignItems: 'center',
      }}>
        <span style={{ fontSize: 11, color: '#94a3b8', marginRight: 8, fontWeight: 600, letterSpacing: '0.05em', textTransform: 'uppercase' }}>
          View as:
        </span>
        {ROLES.map(r => (
          <button
            key={r.id}
            onClick={() => setActiveRole(r.id)}
            style={{
              padding: '5px 14px',
              borderRadius: 6,
              border: activeRole === r.id ? `2px solid ${r.color}` : '2px solid transparent',
              backgroundColor: activeRole === r.id ? `${r.color}18` : 'transparent',
              color: activeRole === r.id ? r.color : '#64748b',
              fontWeight: activeRole === r.id ? 700 : 400,
              fontSize: 13,
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: 5,
              transition: 'all 0.1s',
            }}
          >
            <span style={{ fontSize: 14 }}>{r.icon}</span>
            {r.label}
          </button>
        ))}
      </div>

      {/* Header */}
      <header style={{
        backgroundColor: role.color,
        color: 'white',
        padding: '14px 24px',
        boxShadow: '0 1px 3px rgba(0,0,0,0.15)',
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <span style={{ fontSize: 20 }}>{role.icon}</span>
          <div>
            <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>
              Mobile Check Deposit — {role.desc}
            </h1>
            <p style={{ margin: 0, fontSize: 12, opacity: 0.8 }}>
              Apex Fintech Services · Week 4
            </p>
          </div>
        </div>
      </header>

      <main style={{ maxWidth: 1000, margin: '0 auto', padding: '20px 24px' }}>
        {activeRole === 'investor'   && <InvestorView />}
        {activeRole === 'operator'   && <OperatorView />}
        {activeRole === 'settlement' && <SettlementView />}
        {activeRole === 'admin'      && <AdminView />}
      </main>
    </div>
  )
}
