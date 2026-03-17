import { useState, useEffect, useCallback } from 'react'
import { api } from '../api.js'

const STATUS_STYLES = {
  funds_posted: 'bg-green-100 text-green-800',
  completed:    'bg-green-100 text-green-800',
  rejected:     'bg-red-100 text-red-800',
  analyzing:    'bg-yellow-100 text-yellow-800',
  validating:   'bg-blue-100 text-blue-800',
  approved:     'bg-blue-100 text-blue-800',
  requested:    'bg-gray-100 text-gray-700',
  returned:     'bg-orange-100 text-orange-800',
}

function fmtCents(cents) {
  if (cents == null) return '—'
  return `$${(cents / 100).toFixed(2)}`
}

function fmtDate(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

/**
 * @param {{ accountId: string, selectedTransferId: string|null, onSelect: (id: string) => void }} props
 */
export default function DepositList({ accountId, selectedTransferId, onSelect }) {
  const [deposits, setDeposits] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  const fetchDeposits = useCallback(async () => {
    if (!accountId) return
    try {
      const resp = await api.listDeposits({ account_id: accountId, per_page: 20 })
      setDeposits(resp.data || [])
      setError(null)
    } catch (err) {
      setError(err?.error || 'Failed to load deposits')
    }
  }, [accountId])

  useEffect(() => {
    setLoading(true)
    fetchDeposits().finally(() => setLoading(false))
    const interval = setInterval(fetchDeposits, 5000)
    return () => clearInterval(interval)
  }, [fetchDeposits])

  return (
    <div style={{ marginBottom: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 10 }}>
        <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>Recent Deposits</h3>
        {deposits.length > 0 && (
          <span style={{ fontSize: 12, color: '#9ca3af' }}>
            {deposits.length} deposit{deposits.length !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      {error && (
        <p className="text-sm text-red-600 py-2">{error}</p>
      )}

      {loading && deposits.length === 0 && (
        <p style={{ fontSize: 13, color: '#9ca3af', padding: '16px 0' }}>Loading deposits…</p>
      )}

      {!loading && !error && deposits.length === 0 && (
        <div style={{ textAlign: 'center', padding: '28px 0', color: '#9ca3af' }}>
          <p style={{ margin: '0 0 4px', fontSize: 14 }}>No deposits for this account yet.</p>
          <p style={{ margin: 0, fontSize: 12 }}>Submit a check deposit to get started.</p>
        </div>
      )}

      {deposits.length > 0 && (
        <div style={{ border: '1px solid #e5e7eb', borderRadius: 8, overflow: 'hidden' }}>
          {deposits.map((d, i) => (
            <div
              key={d.transfer_id}
              onClick={() => onSelect?.(d.transfer_id)}
              style={{
                padding: '10px 14px',
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                cursor: 'pointer',
                borderTop: i > 0 ? '1px solid #f3f4f6' : 'none',
                backgroundColor: selectedTransferId === d.transfer_id ? '#eff6ff' : 'white',
              }}
            >
              <span className={`px-2 py-0.5 rounded text-xs font-semibold uppercase shrink-0 ${STATUS_STYLES[d.status] || 'bg-gray-100 text-gray-700'}`}>
                {d.status?.replace('_', ' ')}
              </span>
              <span style={{ fontWeight: 600, fontSize: 13 }}>{fmtCents(d.amount_cents)}</span>
              {d.flagged && (
                <span className="px-1.5 py-0.5 rounded text-xs bg-orange-100 text-orange-700 shrink-0">
                  flagged
                </span>
              )}
              <span style={{ fontSize: 11, color: '#9ca3af', marginLeft: 'auto', whiteSpace: 'nowrap', flexShrink: 0 }}>
                {fmtDate(d.created_at)}
              </span>
              <span style={{ fontSize: 10, fontFamily: 'monospace', color: '#cbd5e1', flexShrink: 0 }}>
                {d.transfer_id?.slice(0, 8)}…
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
