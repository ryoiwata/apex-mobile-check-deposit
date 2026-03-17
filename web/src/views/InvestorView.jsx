import { useState, useCallback } from 'react'
import DepositForm from '../components/DepositForm.jsx'
import TransferStatus from '../components/TransferStatus.jsx'
import LedgerView from '../components/LedgerView.jsx'
import NotificationPanel from '../components/NotificationPanel.jsx'
import { ACCOUNTS } from '../accounts.js'

const TABS = [
  { id: 'deposit', label: 'Deposit' },
  { id: 'deposits', label: 'My Deposits' },
  { id: 'account', label: 'Account' },
]

const ACCENT = '#2563eb'

export default function InvestorView() {
  const [activeTab, setActiveTab] = useState('deposit')
  const [transferId, setTransferId] = useState(null)
  const [accountId, setAccountId] = useState('ACC-SOFI-1006')
  const [unreadCount, setUnreadCount] = useState(0)

  const selectedAccount = ACCOUNTS.find(a => a.id === accountId)

  function handleDepositSuccess(id) {
    setTransferId(id)
    setActiveTab('deposits')
  }

  function handleStartNewDeposit(acctId) {
    if (acctId) setAccountId(acctId)
    setTransferId(null)
    setActiveTab('deposit')
  }

  const handleUnreadChange = useCallback((count) => setUnreadCount(count), [])

  return (
    <div>
      {/* Global account selector */}
      <div style={{
        padding: '12px 0 14px',
        display: 'flex',
        alignItems: 'center',
        gap: 10,
      }}>
        <span style={{ fontSize: 12, color: '#64748b', whiteSpace: 'nowrap' }}>Account:</span>
        <select
          value={accountId}
          onChange={e => setAccountId(e.target.value)}
          style={{
            border: '1px solid #d1d5db',
            borderRadius: 6,
            padding: '5px 10px',
            fontSize: 13,
            fontWeight: 500,
            color: '#1e293b',
            background: '#f8fafc',
            cursor: 'pointer',
            maxWidth: 360,
          }}
        >
          {ACCOUNTS.map(a => (
            <option
              key={a.id}
              value={a.id}
              style={a.status !== 'active' ? { color: '#9ca3af', fontStyle: 'italic' } : undefined}
            >
              {a.id} — {a.label}{a.status !== 'active' ? ` (${a.status})` : ''}
            </option>
          ))}
        </select>
        {selectedAccount?.status !== 'active' && (
          <span style={{ fontSize: 12, color: '#dc2626', fontWeight: 500 }}>
            {selectedAccount?.status === 'closed' ? 'Closed' : 'Suspended'}
          </span>
        )}
      </div>

      <nav style={{ borderBottom: '1px solid #e5e7eb', marginBottom: 20, display: 'flex' }}>
        {TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            style={{
              padding: '8px 20px',
              fontSize: 13,
              fontWeight: activeTab === tab.id ? 600 : 400,
              color: activeTab === tab.id ? ACCENT : '#64748b',
              borderBottom: activeTab === tab.id ? `2px solid ${ACCENT}` : '2px solid transparent',
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              display: 'flex',
              alignItems: 'center',
              gap: 6,
            }}
          >
            {tab.label}
            {tab.id === 'deposits' && unreadCount > 0 && (
              <span style={{
                backgroundColor: '#dc2626',
                color: 'white',
                fontSize: 10,
                fontWeight: 700,
                borderRadius: 10,
                padding: '1px 6px',
                lineHeight: 1.5,
              }}>
                {unreadCount > 9 ? '9+' : unreadCount}
              </span>
            )}
          </button>
        ))}
      </nav>

      {activeTab === 'deposit' && (
        <DepositForm
          accountId={accountId}
          onSwitchAccount={() => {
            // Focus the global selector — scroll to top and highlight it
            window.scrollTo({ top: 0, behavior: 'smooth' })
          }}
          onSuccess={handleDepositSuccess}
        />
      )}

      {activeTab === 'deposits' && (
        <>
          <NotificationPanel
            accountId={accountId}
            onSelectTransfer={(tid) => setTransferId(tid)}
            onUnreadChange={handleUnreadChange}
            onStartNewDeposit={handleStartNewDeposit}
          />
          <TransferStatus
            initialTransferId={transferId}
            onStartNewDeposit={handleStartNewDeposit}
          />
        </>
      )}

      {activeTab === 'account' && <LedgerView accountId={accountId} />}
    </div>
  )
}
