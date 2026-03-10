import { useState } from 'react'
import DepositForm from './components/DepositForm.jsx'
import ReviewQueue from './components/ReviewQueue.jsx'
import TransferStatus from './components/TransferStatus.jsx'
import LedgerView from './components/LedgerView.jsx'

const TABS = [
  { id: 'deposit', label: 'Deposit' },
  { id: 'status', label: 'My Deposits' },
  { id: 'queue', label: 'Operator Queue' },
  { id: 'ledger', label: 'Ledger' },
]

export default function App() {
  const [activeTab, setActiveTab] = useState('deposit')
  const [transferId, setTransferId] = useState(null)

  function handleDepositSuccess(id) {
    setTransferId(id)
    setActiveTab('status')
  }

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
            >
              {tab.label}
            </button>
          ))}
        </div>
      </nav>

      <main className="max-w-5xl mx-auto px-6 py-6">
        {activeTab === 'deposit' && (
          <DepositForm onSuccess={handleDepositSuccess} />
        )}
        {activeTab === 'status' && (
          <TransferStatus initialTransferId={transferId} />
        )}
        {activeTab === 'queue' && <ReviewQueue />}
        {activeTab === 'ledger' && <LedgerView />}
      </main>
    </div>
  )
}
