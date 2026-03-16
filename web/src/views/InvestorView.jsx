import { useState, useCallback } from 'react'
import DepositForm from '../components/DepositForm.jsx'
import TransferStatus from '../components/TransferStatus.jsx'
import LedgerView from '../components/LedgerView.jsx'
import NotificationPanel from '../components/NotificationPanel.jsx'

const TABS = [
  { id: 'deposit', label: 'Deposit' },
  { id: 'deposits', label: 'My Deposits' },
  { id: 'account', label: 'Account' },
]

const ACCENT = '#2563eb'

export default function InvestorView() {
  const [activeTab, setActiveTab] = useState('deposit')
  const [transferId, setTransferId] = useState(null)
  const [accountId, setAccountId] = useState(null)
  const [returnAccountId, setReturnAccountId] = useState(null)
  const [unreadCount, setUnreadCount] = useState(0)

  function handleDepositSuccess(id, acctId) {
    setTransferId(id)
    if (acctId) setAccountId(acctId)
    setActiveTab('deposits')
  }

  function handleStartNewDeposit(acctId) {
    setReturnAccountId(acctId)
    setTransferId(null)
    setActiveTab('deposit')
  }

  const handleUnreadChange = useCallback((count) => setUnreadCount(count), [])

  return (
    <div>
      <div style={{ padding: '12px 0 8px', marginBottom: 8 }}>
        <p style={{ color: '#64748b', fontSize: 13, margin: 0 }}>
          Deposit checks, track status, and view your account.
        </p>
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
              borderBottom: activeTab === tab.id ? `2px solid ${ACCENT}` : '2px solid transparent',
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
          onSuccess={handleDepositSuccess}
          initialAccountId={returnAccountId}
        />
      )}

      {activeTab === 'deposits' && (
        <>
          <NotificationPanel
            accountId={accountId}
            onSelectTransfer={(tid) => setTransferId(tid)}
            onUnreadChange={handleUnreadChange}
          />
          <TransferStatus
            initialTransferId={transferId}
            onStartNewDeposit={handleStartNewDeposit}
          />
        </>
      )}

      {activeTab === 'account' && <LedgerView />}
    </div>
  )
}
