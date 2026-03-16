import { useState, useEffect, useCallback } from 'react'
import { api } from '../api.js'

const ACCENT = '#059669'

function fmtCents(cents) {
  if (cents == null) return '—'
  return `$${(cents / 100).toFixed(2)}`
}

function fmtDate(iso) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

function StatusBadge({ status }) {
  const styles = {
    acknowledged: { bg: '#d1fae5', color: '#065f46' },
    submitted: { bg: '#fef9c3', color: '#854d0e' },
    pending: { bg: '#fef3c7', color: '#92400e' },
    retry_pending: { bg: '#fee2e2', color: '#991b1b' },
    escalated: { bg: '#fee2e2', color: '#991b1b' },
  }
  const s = styles[status] || { bg: '#f3f4f6', color: '#374151' }
  return (
    <span style={{
      backgroundColor: s.bg,
      color: s.color,
      fontSize: 11,
      fontWeight: 700,
      padding: '2px 8px',
      borderRadius: 4,
      textTransform: 'uppercase',
    }}>
      {status?.replace('_', ' ')}
    </span>
  )
}

function BatchesTab({ onSelectBatch }) {
  const [batches, setBatches] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  const fetchBatches = useCallback(async () => {
    try {
      const resp = await api.listBatches()
      setBatches(resp.data || [])
      setError(null)
    } catch (err) {
      setError(err?.error || 'Failed to load batches')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchBatches()
    const timer = setInterval(fetchBatches, 10000)
    return () => clearInterval(timer)
  }, [fetchBatches])

  if (loading) return <p style={{ color: '#6b7280', fontSize: 13 }}>Loading batches…</p>
  if (error) return <p style={{ color: '#dc2626', fontSize: 13 }}>{error}</p>

  if (batches.length === 0) {
    return (
      <p style={{ color: '#6b7280', fontSize: 13, textAlign: 'center', padding: '40px 0' }}>
        No settlement batches yet. Trigger EOD settlement to create one.
      </p>
    )
  }

  return (
    <div style={{ overflowX: 'auto' }}>
      <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
        <thead>
          <tr style={{ backgroundColor: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
            {['Batch ID', 'Date', 'Deposits', 'Total Amount', 'Status', 'Created'].map(h => (
              <th key={h} style={{ padding: '8px 12px', textAlign: 'left', fontSize: 11, color: '#6b7280', textTransform: 'uppercase', letterSpacing: '0.05em' }}>{h}</th>
            ))}
          </tr>
        </thead>
        <tbody>
          {batches.map(batch => (
            <tr
              key={batch.batch_id}
              onClick={() => onSelectBatch(batch.batch_id)}
              style={{ borderBottom: '1px solid #f3f4f6', cursor: 'pointer' }}
              onMouseEnter={e => e.currentTarget.style.backgroundColor = '#f9fafb'}
              onMouseLeave={e => e.currentTarget.style.backgroundColor = ''}
            >
              <td style={{ padding: '8px 12px', fontFamily: 'monospace', fontSize: 11, color: '#6b7280' }}>
                {batch.batch_id?.slice(0, 8)}…
              </td>
              <td style={{ padding: '8px 12px' }}>{batch.batch_date?.slice(0, 10)}</td>
              <td style={{ padding: '8px 12px' }}>{batch.deposit_count}</td>
              <td style={{ padding: '8px 12px', fontWeight: 500 }}>{fmtCents(batch.total_amount_cents)}</td>
              <td style={{ padding: '8px 12px' }}><StatusBadge status={batch.status} /></td>
              <td style={{ padding: '8px 12px', color: '#6b7280' }}>{fmtDate(batch.created_at)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function BatchDetailTab({ batchId, onBack }) {
  const [detail, setDetail] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  useEffect(() => {
    if (!batchId) return
    setLoading(true)
    api.getBatch(batchId)
      .then(resp => { setDetail(resp.data); setError(null) })
      .catch(err => setError(err?.error || 'Failed to load batch'))
      .finally(() => setLoading(false))
  }, [batchId])

  if (!batchId) return (
    <p style={{ color: '#6b7280', fontSize: 13, textAlign: 'center', padding: '40px 0' }}>
      Click a batch in the Batches tab to view its details.
    </p>
  )
  if (loading) return <p style={{ color: '#6b7280', fontSize: 13 }}>Loading batch…</p>
  if (error) return <p style={{ color: '#dc2626', fontSize: 13 }}>{error}</p>
  if (!detail) return null

  const b = detail

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <button onClick={onBack} style={{ fontSize: 13, color: ACCENT, background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}>← Back to list</button>
        <StatusBadge status={b.status} />
      </div>

      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16, display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16 }}>
        {[
          ['Batch ID', <span style={{ fontFamily: 'monospace', fontSize: 11 }}>{b.batch_id}</span>],
          ['Date', b.batch_date?.slice(0, 10)],
          ['Deposits', b.deposit_count],
          ['Total Amount', fmtCents(b.total_amount_cents)],
          ['Status', <StatusBadge status={b.status} />],
          ['File Path', b.file_path || '—'],
          ['Bank Reference', b.bank_reference || '—'],
          ['Retry Count', b.retry_count ?? 0],
          ['Created', fmtDate(b.created_at)],
        ].map(([label, val]) => (
          <div key={label}>
            <p style={{ fontSize: 11, color: '#6b7280', margin: '0 0 2px' }}>{label}</p>
            <p style={{ fontSize: 13, fontWeight: 500, margin: 0 }}>{val}</p>
          </div>
        ))}
      </div>

      <div>
        <h4 style={{ fontSize: 13, fontWeight: 600, marginBottom: 8 }}>Deposits in this batch ({b.deposits?.length ?? 0})</h4>
        {(!b.deposits || b.deposits.length === 0) ? (
          <p style={{ fontSize: 13, color: '#6b7280' }}>No deposits in this batch.</p>
        ) : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
              <thead>
                <tr style={{ backgroundColor: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
                  {['Transfer ID', 'Account', 'Amount', 'Status', 'Created'].map(h => (
                    <th key={h} style={{ padding: '6px 10px', textAlign: 'left', fontSize: 11, color: '#6b7280', textTransform: 'uppercase' }}>{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {b.deposits.map(t => (
                  <tr key={t.transfer_id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                    <td style={{ padding: '6px 10px', fontFamily: 'monospace', fontSize: 11, color: '#6b7280' }}>{t.transfer_id?.slice(0, 8)}…</td>
                    <td style={{ padding: '6px 10px' }}>{t.account_id}</td>
                    <td style={{ padding: '6px 10px', fontWeight: 500 }}>{fmtCents(t.amount_cents)}</td>
                    <td style={{ padding: '6px 10px' }}>{t.status}</td>
                    <td style={{ padding: '6px 10px', color: '#6b7280' }}>{fmtDate(t.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function EODStatusTab() {
  const [status, setStatus] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [settling, setSettling] = useState(false)
  const [settlementResult, setSettlementResult] = useState(null)
  const [settlementError, setSettlementError] = useState(null)

  const fetchStatus = useCallback(async () => {
    try {
      const resp = await api.getEODStatus()
      setStatus(resp.data)
      setError(null)
    } catch (err) {
      setError(err?.error || 'Failed to load EOD status')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchStatus()
    const timer = setInterval(fetchStatus, 15000)
    return () => clearInterval(timer)
  }, [fetchStatus])

  async function handleTrigger() {
    const today = new Date().toISOString().slice(0, 10)
    if (!window.confirm(`Trigger EOD settlement for ${today}?`)) return
    setSettling(true)
    setSettlementResult(null)
    setSettlementError(null)
    try {
      const resp = await api.triggerSettlement(today)
      setSettlementResult(resp.data)
      fetchStatus()
    } catch (err) {
      setSettlementError(err?.error || 'Settlement trigger failed')
    } finally {
      setSettling(false)
    }
  }

  if (loading) return <p style={{ color: '#6b7280', fontSize: 13 }}>Loading EOD status…</p>
  if (error) return <p style={{ color: '#dc2626', fontSize: 13 }}>{error}</p>

  const cutoff = status ? new Date(status.cutoff_time) : null

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* Cutoff status card */}
      <div style={{
        background: status?.past_cutoff ? '#fef2f2' : '#f0fdf4',
        border: `1px solid ${status?.past_cutoff ? '#fecaca' : '#bbf7d0'}`,
        borderRadius: 8,
        padding: 16,
      }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div>
            <p style={{ fontSize: 12, color: '#6b7280', margin: '0 0 4px' }}>EOD Cutoff (6:30 PM CT)</p>
            <p style={{ fontSize: 18, fontWeight: 700, margin: 0, color: status?.past_cutoff ? '#991b1b' : '#065f46' }}>
              {status?.past_cutoff ? 'Past cutoff' : 'Before cutoff'}
            </p>
            <p style={{ fontSize: 12, color: '#6b7280', margin: '4px 0 0' }}>
              Cutoff: {cutoff ? cutoff.toLocaleString() : '—'}
            </p>
          </div>
          <div style={{ textAlign: 'right' }}>
            <p style={{ fontSize: 12, color: '#6b7280', margin: '0 0 4px' }}>Awaiting Settlement</p>
            <p style={{ fontSize: 24, fontWeight: 700, margin: 0, color: ACCENT }}>{status?.pending_deposit_count ?? 0}</p>
            <p style={{ fontSize: 12, color: '#6b7280', margin: '4px 0 0' }}>{fmtCents(status?.pending_amount_cents)} total</p>
          </div>
        </div>
      </div>

      {/* Trigger button */}
      <div>
        <button
          onClick={handleTrigger}
          disabled={settling}
          style={{
            backgroundColor: ACCENT,
            color: 'white',
            border: 'none',
            borderRadius: 6,
            padding: '10px 20px',
            fontSize: 14,
            fontWeight: 600,
            cursor: settling ? 'not-allowed' : 'pointer',
            opacity: settling ? 0.6 : 1,
          }}
        >
          {settling ? 'Running Settlement…' : 'Trigger EOD Settlement'}
        </button>
      </div>

      {settlementResult && (
        <div style={{ background: '#f0fdf4', border: '1px solid #bbf7d0', borderRadius: 6, padding: 12, fontSize: 13, color: '#065f46' }}>
          <p style={{ fontWeight: 600, margin: '0 0 4px' }}>Settlement complete</p>
          {settlementResult.batch_id && <p style={{ margin: '0 0 2px' }}>Batch: <code>{settlementResult.batch_id}</code></p>}
          <p style={{ margin: 0 }}>{settlementResult.deposit_count} deposit(s) · {fmtCents(settlementResult.total_amount_cents)}</p>
        </div>
      )}
      {settlementError && (
        <div style={{ background: '#fef2f2', border: '1px solid #fecaca', borderRadius: 6, padding: 12, fontSize: 13, color: '#991b1b' }}>
          {settlementError}
        </div>
      )}
    </div>
  )
}

const TABS = [
  { id: 'batches', label: 'Batches' },
  { id: 'detail', label: 'Batch Detail' },
  { id: 'eod', label: 'EOD Status' },
]

export default function SettlementView() {
  const [activeTab, setActiveTab] = useState('eod')
  const [selectedBatchId, setSelectedBatchId] = useState(null)

  function handleSelectBatch(batchId) {
    setSelectedBatchId(batchId)
    setActiveTab('detail')
  }

  return (
    <div>
      <div style={{ padding: '12px 0 8px', marginBottom: 8 }}>
        <p style={{ color: '#64748b', fontSize: 13, margin: 0 }}>
          Settlement batches, X9 file generation, and bank acknowledgment tracking.
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
              background: 'none',
              border: 'none',
              borderBottom: activeTab === tab.id ? `2px solid ${ACCENT}` : '2px solid transparent',
              cursor: 'pointer',
            }}
          >
            {tab.label}
          </button>
        ))}
      </nav>

      {activeTab === 'batches' && <BatchesTab onSelectBatch={handleSelectBatch} />}
      {activeTab === 'detail' && (
        <BatchDetailTab
          batchId={selectedBatchId}
          onBack={() => setActiveTab('batches')}
        />
      )}
      {activeTab === 'eod' && <EODStatusTab />}
    </div>
  )
}
