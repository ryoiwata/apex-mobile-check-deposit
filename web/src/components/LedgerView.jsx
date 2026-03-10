import { useState, useCallback } from 'react'
import { api } from '../api.js'

const ACCOUNTS = [
  { id: 'ACC-SOFI-1006', label: 'ACC-SOFI-1006 — Clean Pass' },
  { id: 'ACC-SOFI-1001', label: 'ACC-SOFI-1001 — IQA Blur' },
  { id: 'ACC-SOFI-1002', label: 'ACC-SOFI-1002 — IQA Glare' },
  { id: 'ACC-SOFI-1003', label: 'ACC-SOFI-1003 — MICR Failure' },
  { id: 'ACC-SOFI-1004', label: 'ACC-SOFI-1004 — Duplicate' },
  { id: 'ACC-SOFI-1005', label: 'ACC-SOFI-1005 — Amount Mismatch' },
  { id: 'ACC-SOFI-0000', label: 'ACC-SOFI-0000 — Basic Pass' },
  { id: 'ACC-RETIRE-001', label: 'ACC-RETIRE-001 — Retirement' },
]

function fmtCents(cents) {
  return `$${(cents / 100).toFixed(2)}`
}

function fmtDate(iso) {
  return new Date(iso).toLocaleString()
}

const SUB_TYPE_STYLES = {
  DEPOSIT: 'text-green-700',
  REVERSAL: 'text-red-700',
  RETURN_FEE: 'text-red-700',
}

export default function LedgerView() {
  const [accountId, setAccountId] = useState('ACC-SOFI-1006')
  const [ledger, setLedger] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  const fetchLedger = useCallback(async (id) => {
    setLoading(true)
    setError(null)
    try {
      const resp = await api.getLedger(id)
      setLedger(resp.data)
    } catch (err) {
      setError(err?.error || 'Failed to load ledger')
      setLedger(null)
    } finally {
      setLoading(false)
    }
  }, [])

  function handleLoad(e) {
    e.preventDefault()
    fetchLedger(accountId)
  }

  const entries = ledger?.entries || []
  const balance = entries.reduce((sum, e) => {
    if (e.sub_type === 'DEPOSIT') return sum + e.amount_cents
    if (e.sub_type === 'REVERSAL' || e.sub_type === 'RETURN_FEE') return sum - e.amount_cents
    return sum
  }, 0)

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-gray-800">Ledger View</h2>

      <form onSubmit={handleLoad} className="flex gap-2">
        <select
          value={accountId}
          onChange={e => setAccountId(e.target.value)}
          className="flex-1 border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
        >
          {ACCOUNTS.map(a => (
            <option key={a.id} value={a.id}>{a.label}</option>
          ))}
        </select>
        <button
          type="submit"
          disabled={loading}
          className="px-4 py-2 bg-blue-700 text-white text-sm font-medium rounded hover:bg-blue-800 disabled:opacity-50"
        >
          {loading ? 'Loading…' : 'Load Ledger'}
        </button>
      </form>

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
          {error}
        </div>
      )}

      {ledger && (
        <>
          <div className="bg-white border border-gray-200 rounded p-4 flex items-center gap-6">
            <div>
              <p className="text-xs text-gray-500">Account</p>
              <p className="text-sm font-medium">{ledger.account_id}</p>
            </div>
            <div>
              <p className="text-xs text-gray-500">Net Balance (calculated)</p>
              <p className={`text-lg font-bold ${balance >= 0 ? 'text-green-700' : 'text-red-700'}`}>
                {fmtCents(Math.abs(balance))}
                {balance < 0 && ' (debit)'}
              </p>
            </div>
            <div>
              <p className="text-xs text-gray-500">Entries</p>
              <p className="text-sm font-medium">{entries.length}</p>
            </div>
          </div>

          {entries.length === 0 ? (
            <p className="text-sm text-gray-500 py-8 text-center">No ledger entries for this account.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm border border-gray-200 rounded">
                <thead className="bg-gray-50 text-xs text-gray-500 uppercase">
                  <tr>
                    <th className="px-3 py-2 text-left">Date</th>
                    <th className="px-3 py-2 text-left">Sub Type</th>
                    <th className="px-3 py-2 text-left">Transfer Type</th>
                    <th className="px-3 py-2 text-left">From</th>
                    <th className="px-3 py-2 text-left">To</th>
                    <th className="px-3 py-2 text-right">Amount</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-100">
                  {entries.map(entry => (
                    <tr key={entry.id} className="hover:bg-gray-50">
                      <td className="px-3 py-2 text-gray-500 whitespace-nowrap">{fmtDate(entry.created_at)}</td>
                      <td className={`px-3 py-2 font-medium ${SUB_TYPE_STYLES[entry.sub_type] || 'text-gray-700'}`}>
                        {entry.sub_type}
                      </td>
                      <td className="px-3 py-2 text-gray-600">{entry.transfer_type}</td>
                      <td className="px-3 py-2 text-gray-500 font-mono text-xs">{entry.from_account_id}</td>
                      <td className="px-3 py-2 text-gray-500 font-mono text-xs">{entry.to_account_id}</td>
                      <td className={`px-3 py-2 text-right font-medium ${SUB_TYPE_STYLES[entry.sub_type] || 'text-gray-700'}`}>
                        {entry.sub_type === 'DEPOSIT' ? '+' : '-'}{fmtCents(entry.amount_cents)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}
    </div>
  )
}
