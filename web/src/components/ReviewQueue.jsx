import { useState, useEffect, useCallback } from 'react'
import { api } from '../api.js'

function fmtCents(cents) {
  if (cents == null) return '—'
  return `$${(cents / 100).toFixed(2)}`
}

function timeInQueue(createdAt) {
  const ms = Date.now() - new Date(createdAt).getTime()
  const min = Math.floor(ms / 60000)
  const hr = Math.floor(min / 60)
  if (hr > 0) return `${hr}h ${min % 60}m`
  if (min > 0) return `${min}m`
  return 'just now'
}

export default function ReviewQueue({ onSelectDeposit, onQueueCountChange }) {
  const [queue, setQueue] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  const fetchQueue = useCallback(async () => {
    try {
      const resp = await api.getQueue()
      const data = resp.data || []
      setQueue(data)
      onQueueCountChange?.(data.length)
      setError(null)
    } catch (err) {
      setError(err?.error || 'Failed to load operator queue')
    } finally {
      setLoading(false)
    }
  }, [onQueueCountChange])

  useEffect(() => {
    fetchQueue()
    const timer = setInterval(fetchQueue, 5000)
    return () => clearInterval(timer)
  }, [fetchQueue])

  if (loading) return <p style={{ color: '#6b7280', fontSize: 13 }}>Loading queue…</p>
  if (error) return <p style={{ color: '#dc2626', fontSize: 13 }}>{error}</p>

  if (queue.length === 0) {
    return (
      <p style={{ color: '#6b7280', fontSize: 13, textAlign: 'center', padding: '40px 0' }}>
        No flagged deposits in queue.
      </p>
    )
  }

  return (
    <div>
      <p style={{ fontSize: 12, color: '#9ca3af', marginBottom: 10 }}>
        Refreshes every 5s · Click a row to review
      </p>
      <div style={{ overflowX: 'auto' }}>
        <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
          <thead>
            <tr style={{ backgroundColor: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
              {['Transfer ID', 'Account', 'Amount', 'Flag Reason', 'Submitted', 'Time in Queue'].map(h => (
                <th key={h} style={{ padding: '8px 12px', textAlign: 'left', fontSize: 11, color: '#6b7280', textTransform: 'uppercase', letterSpacing: '0.05em' }}>{h}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {queue.map(deposit => (
              <tr
                key={deposit.transfer_id}
                onClick={() => onSelectDeposit?.(deposit)}
                style={{ borderBottom: '1px solid #f3f4f6', cursor: 'pointer' }}
                onMouseEnter={e => e.currentTarget.style.backgroundColor = '#fffbeb'}
                onMouseLeave={e => e.currentTarget.style.backgroundColor = ''}
              >
                <td style={{ padding: '8px 12px', fontFamily: 'monospace', fontSize: 11, color: '#6b7280' }}>
                  {deposit.transfer_id?.slice(0, 8)}…
                </td>
                <td style={{ padding: '8px 12px' }}>{deposit.account_id}</td>
                <td style={{ padding: '8px 12px', fontWeight: 500 }}>{fmtCents(deposit.amount_cents)}</td>
                <td style={{ padding: '8px 12px' }}>
                  {deposit.flag_reason ? (
                    <span style={{ backgroundColor: '#fff7ed', color: '#c2410c', fontSize: 11, fontWeight: 700, padding: '2px 8px', borderRadius: 4 }}>
                      {deposit.flag_reason}
                    </span>
                  ) : '—'}
                </td>
                <td style={{ padding: '8px 12px', color: '#6b7280', whiteSpace: 'nowrap', fontSize: 12 }}>
                  {new Date(deposit.created_at).toLocaleString()}
                </td>
                <td style={{ padding: '8px 12px', color: '#d97706', fontWeight: 600 }}>
                  {timeInQueue(deposit.created_at)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
