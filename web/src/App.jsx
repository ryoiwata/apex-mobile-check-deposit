import { useState, useCallback } from 'react'
import DepositForm from './components/DepositForm.jsx'
import ReviewQueue from './components/ReviewQueue.jsx'
import TransferStatus from './components/TransferStatus.jsx'
import LedgerView from './components/LedgerView.jsx'
import NotificationPanel from './components/NotificationPanel.jsx'

const TABS = [
  { id: 'deposit', label: 'Deposit' },
  { id: 'status', label: 'My Deposits' },
  { id: 'queue', label: 'Operator Queue' },
  { id: 'ledger', label: 'Ledger' },
]

export default function App() {
  const [activeTab, setActiveTab] = useState('deposit')
  const [transferId, setTransferId] = useState(null)
  const [accountId, setAccountId] = useState(null)
  const [unreadCount, setUnreadCount] = useState(0)
  // Preserved account ID so "Start New Deposit" after a return can pre-select the account
  const [returnAccountId, setReturnAccountId] = useState(null)

  function handleDepositSuccess(id, acctId) {
    setTransferId(id)
    if (acctId) setAccountId(acctId)
    setActiveTab('status')
  }

  function handleStartNewDeposit(acctId) {
    setReturnAccountId(acctId)
    setTransferId(null)
    setActiveTab('deposit')
  }

  const handleUnreadChange = useCallback((count) => {
    setUnreadCount(count)
  }, [])

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-blue-800 text-white px-6 py-4 shadow">
        <h1 className="text-xl font-bold">Mobile Check Deposit System</h1>
        <p className="text-blue-200 text-sm">Apex Fintech Services — Week 4</p>
      </header>

      <nav className="bg-white border-b border-gray-200 px-6">
        <div className="flex space-x-0">
          {TABS.map(tab => (
            <button
              key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={`px-5 py-3 text-sm font-medium border-b-2 transition-colors ${
                activeTab === tab.id
                  ? 'border-blue-700 text-blue-700'
                  : 'border-transparent text-gray-500 hover:text-gray-700'
              }`}
              style={{ display: 'flex', alignItems: 'center', gap: 6 }}
            >
              {tab.label}
              {tab.id === 'status' && unreadCount > 0 && (
                <span style={{
                  backgroundColor: '#dc2626',
                  color: 'white',
                  fontSize: 11,
                  fontWeight: 700,
                  borderRadius: 10,
                  minWidth: 18,
                  height: 18,
                  display: 'inline-flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  padding: '0 5px',
                  lineHeight: 1,
                }}>
                  {unreadCount > 9 ? '9+' : unreadCount}
                </span>
              )}
            </button>
          ))}
        </div>
      </nav>

      <main className="max-w-5xl mx-auto px-6 py-6">
        {activeTab === 'deposit' && (
          <DepositForm
            onSuccess={handleDepositSuccess}
            initialAccountId={returnAccountId}
          />
        )}
        {activeTab === 'status' && (
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
        {activeTab === 'queue' && <ReviewQueue />}
        {activeTab === 'ledger' && <LedgerView />}
      </main>
    </div>
  )
}
