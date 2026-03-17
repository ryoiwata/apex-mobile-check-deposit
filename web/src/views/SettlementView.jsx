import { useState, useEffect, useCallback, useRef } from 'react'
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

function ReturnModal({ deposit, reasons, onClose, onSuccess }) {
  const [selectedCode, setSelectedCode] = useState(reasons[0]?.code || '')
  const [notes, setNotes] = useState('')
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState(null)
  const [error, setError] = useState(null)
  const overlayRef = useRef(null)

  const selectedReason = reasons.find(r => r.code === selectedCode)

  async function handleConfirm() {
    setLoading(true)
    setError(null)
    try {
      const resp = await api.returnDeposit(deposit.transfer_id || deposit.id, {
        reason_code: selectedCode,
        notes,
      })
      setResult(resp.data)
      onSuccess()
    } catch (err) {
      setError(err?.error || 'Return failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      ref={overlayRef}
      onClick={e => { if (e.target === overlayRef.current) onClose() }}
      style={{
        position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.4)',
        display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50,
      }}
    >
      <div style={{
        background: '#fff', borderRadius: 10, padding: 24, width: 480, maxWidth: '95vw',
        boxShadow: '0 20px 60px rgba(0,0,0,0.2)', display: 'flex', flexDirection: 'column', gap: 16,
      }}>
        {result ? (
          <>
            <h3 style={{ margin: 0, fontSize: 16, color: '#dc2626' }}>✓ Check Returned</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, fontSize: 13 }}>
              <p style={{ margin: 0 }}><strong>Reason:</strong> {result.return_reason?.label}</p>
              <p style={{ margin: 0 }}><strong>Bank Reference:</strong> <code>{result.bank_reference}</code></p>
              <p style={{ margin: 0, color: '#dc2626' }}>
                <strong>Reversal:</strong> −{fmtCents(result.reversal?.original_amount_cents)}
              </p>
              <p style={{ margin: 0, color: '#dc2626' }}>
                <strong>Return Fee:</strong> −{fmtCents(result.reversal?.fee_cents)}
              </p>
              <p style={{ margin: 0, fontWeight: 600 }}>
                <strong>Total Debited:</strong> −{fmtCents(result.reversal?.total_debited_cents)}
              </p>
              <p style={{ margin: 0, color: '#6b7280' }}>
                Investor Notified: {result.investor_notified ? 'Yes' : 'No'}
              </p>
            </div>
            <p style={{ margin: 0, fontSize: 12, color: '#6b7280' }}>
              Switch to the Investor View to see the return notification and updated ledger.
            </p>
            <button
              onClick={onClose}
              style={{ alignSelf: 'flex-end', padding: '7px 16px', borderRadius: 6, border: '1px solid #d1d5db', background: '#fff', fontSize: 13, cursor: 'pointer' }}
            >
              Close
            </button>
          </>
        ) : (
          <>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700 }}>↩ Simulate Bank Return</h3>
              <button onClick={onClose} style={{ background: 'none', border: 'none', fontSize: 18, cursor: 'pointer', color: '#9ca3af' }}>✕</button>
            </div>

            {/* Deposit summary */}
            <div style={{ background: '#f8fafc', border: '1px solid #e5e7eb', borderRadius: 6, padding: 10, fontSize: 12 }}>
              <p style={{ margin: '0 0 2px', color: '#6b7280' }}>Transfer</p>
              <p style={{ margin: '0 0 4px', fontFamily: 'monospace', fontSize: 11 }}>{deposit.transfer_id || deposit.id}</p>
              <p style={{ margin: 0 }}>
                <strong>{fmtCents(deposit.amount_cents)}</strong> · {deposit.account_id}
              </p>
            </div>

            {/* Return reason */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              <label style={{ fontSize: 12, fontWeight: 600, color: '#374151' }}>Return Reason</label>
              <select
                value={selectedCode}
                onChange={e => setSelectedCode(e.target.value)}
                style={{ border: '1px solid #d1d5db', borderRadius: 4, padding: '7px 10px', fontSize: 13 }}
              >
                {reasons.map(r => (
                  <option key={r.code} value={r.code}>{r.label}</option>
                ))}
              </select>
              {selectedReason && (
                <p style={{ margin: 0, fontSize: 12, color: '#6b7280', fontStyle: 'italic' }}>
                  {selectedReason.description}
                </p>
              )}
            </div>

            {/* Notes */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <label style={{ fontSize: 12, fontWeight: 600, color: '#374151' }}>Notes (optional)</label>
              <input
                type="text"
                value={notes}
                onChange={e => setNotes(e.target.value)}
                placeholder="Internal notes about this return"
                style={{ border: '1px solid #d1d5db', borderRadius: 4, padding: '7px 10px', fontSize: 13 }}
              />
            </div>

            {/* Impact preview */}
            <div style={{ background: '#fef2f2', border: '1px solid #fecaca', borderRadius: 6, padding: 10, fontSize: 12 }}>
              <p style={{ margin: '0 0 6px', fontWeight: 600, color: '#dc2626' }}>This will:</p>
              <ul style={{ margin: 0, paddingLeft: 16, color: '#374151', lineHeight: 1.7 }}>
                <li>Debit {fmtCents(deposit.amount_cents)} from {deposit.account_id} (reversal)</li>
                <li>Debit $30.00 from {deposit.account_id} (return fee)</li>
                <li>Transition deposit from Completed → Returned</li>
                <li>Notify the investor</li>
              </ul>
            </div>

            {error && <p style={{ margin: 0, color: '#dc2626', fontSize: 13 }}>{error}</p>}

            <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
              <button
                onClick={onClose}
                style={{ padding: '7px 16px', borderRadius: 6, border: '1px solid #d1d5db', background: '#fff', fontSize: 13, cursor: 'pointer' }}
              >
                Cancel
              </button>
              <button
                onClick={handleConfirm}
                disabled={loading}
                style={{
                  padding: '7px 16px', borderRadius: 6, border: 'none',
                  backgroundColor: '#dc2626', color: 'white', fontSize: 13, fontWeight: 600,
                  cursor: loading ? 'not-allowed' : 'pointer', opacity: loading ? 0.6 : 1,
                }}
              >
                {loading ? 'Processing…' : 'Confirm Return'}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

function BatchDetailTab({ batchId, onBack }) {
  const [detail, setDetail] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [returnModal, setReturnModal] = useState(null)
  const [returnReasons, setReturnReasons] = useState([])

  useEffect(() => {
    api.getReturnReasons()
      .then(reasons => setReturnReasons(Array.isArray(reasons) ? reasons : []))
      .catch(() => {})
  }, [])

  const fetchDetail = useCallback(() => {
    if (!batchId) return
    setLoading(true)
    api.getBatch(batchId)
      .then(resp => { setDetail(resp.data); setError(null) })
      .catch(err => setError(err?.error || 'Failed to load batch'))
      .finally(() => setLoading(false))
  }, [batchId])

  useEffect(() => {
    fetchDetail()
  }, [fetchDetail])

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
      {returnModal && returnReasons.length > 0 && (
        <ReturnModal
          deposit={returnModal}
          reasons={returnReasons}
          onClose={() => setReturnModal(null)}
          onSuccess={() => { fetchDetail() }}
        />
      )}

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
                  {['Transfer ID', 'Account', 'Amount', 'Status', 'Created', 'Actions'].map(h => (
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
                    <td style={{ padding: '6px 10px' }}>
                      {t.status === 'completed' && (
                        <button
                          onClick={() => setReturnModal(t)}
                          style={{
                            fontSize: 12, padding: '3px 10px',
                            backgroundColor: '#fef2f2', color: '#dc2626',
                            border: '1px solid #fecaca', borderRadius: 4, cursor: 'pointer',
                          }}
                        >
                          ↩ Simulate Return
                        </button>
                      )}
                    </td>
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

function PreviewDepositTable({ deposits, headerColor, checkmark }) {
  if (deposits.length === 0) return null
  return (
    <table style={{ width: '100%', fontSize: 12, borderCollapse: 'collapse', marginTop: 8 }}>
      <thead>
        <tr style={{ color: headerColor }}>
          <th style={{ textAlign: 'left', padding: '3px 6px' }}>Transfer ID</th>
          <th style={{ textAlign: 'left', padding: '3px 6px' }}>Account</th>
          <th style={{ textAlign: 'right', padding: '3px 6px' }}>Amount</th>
          <th style={{ textAlign: 'left', padding: '3px 6px' }}>Submitted</th>
        </tr>
      </thead>
      <tbody>
        {deposits.map(d => (
          <tr key={d.transfer_id} style={{ borderTop: '1px solid rgba(0,0,0,0.06)' }}>
            <td style={{ padding: '4px 6px', fontFamily: 'monospace', fontSize: 11, color: '#6b7280' }}>
              {d.transfer_id.slice(0, 8)}…
            </td>
            <td style={{ padding: '4px 6px' }}>{d.account_id}</td>
            <td style={{ padding: '4px 6px', textAlign: 'right', fontWeight: 500 }}>{fmtCents(d.amount_cents)}</td>
            <td style={{ padding: '4px 6px', color: '#6b7280' }}>
              {new Date(d.created_at).toLocaleTimeString()} {checkmark}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}

function SettlementPreviewModal({ preview, onClose, onConfirm, confirming }) {
  const overlayRef = useRef(null)
  const included = preview.included_deposits ?? []
  const rolled = preview.rolled_deposits ?? []

  return (
    <div
      ref={overlayRef}
      onClick={e => { if (e.target === overlayRef.current) onClose() }}
      style={{
        position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.45)',
        display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50,
      }}
    >
      <div style={{
        background: '#fff', borderRadius: 10, width: 640, maxWidth: '96vw',
        maxHeight: '85vh', display: 'flex', flexDirection: 'column',
        boxShadow: '0 20px 60px rgba(0,0,0,0.2)',
      }}>
        {/* Header */}
        <div style={{ padding: '16px 20px', borderBottom: '1px solid #e5e7eb', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 style={{ margin: 0, fontSize: 16, fontWeight: 700 }}>EOD Settlement Preview</h2>
          <button onClick={onClose} style={{ background: 'none', border: 'none', fontSize: 18, cursor: 'pointer', color: '#9ca3af' }}>✕</button>
        </div>

        {/* Body */}
        <div style={{ padding: '16px 20px', overflowY: 'auto', flex: 1, display: 'flex', flexDirection: 'column', gap: 14 }}>
          <p style={{ margin: 0, fontSize: 12, color: '#6b7280' }}>
            Cutoff: <strong>{new Date(preview.cutoff_time).toLocaleString()}</strong> (6:30 PM CT)
          </p>

          {/* Included */}
          <div style={{ background: '#f0fdf4', border: '1px solid #bbf7d0', borderRadius: 8, padding: '12px 14px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
              <span style={{ fontWeight: 600, color: '#065f46', fontSize: 13 }}>
                ✅ Included in this batch ({included.length})
              </span>
              <span style={{ fontWeight: 700, color: '#065f46', fontSize: 13 }}>{fmtCents(preview.included_total)}</span>
            </div>
            {included.length === 0
              ? <p style={{ margin: '8px 0 0', fontSize: 12, color: '#6b7280', fontStyle: 'italic' }}>No deposits before cutoff</p>
              : <PreviewDepositTable deposits={included} headerColor="#166534" checkmark="✓" />
            }
          </div>

          {/* Rolled */}
          <div style={{ background: '#fff7ed', border: '1px solid #fed7aa', borderRadius: 8, padding: '12px 14px' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
              <span style={{ fontWeight: 600, color: '#9a3412', fontSize: 13 }}>
                ⏳ Rolled to next business day ({rolled.length})
              </span>
              <span style={{ fontWeight: 700, color: '#9a3412', fontSize: 13 }}>{fmtCents(preview.rolled_total)}</span>
            </div>
            {rolled.length === 0
              ? <p style={{ margin: '8px 0 0', fontSize: 12, color: '#6b7280', fontStyle: 'italic' }}>No deposits after cutoff</p>
              : <PreviewDepositTable deposits={rolled} headerColor="#9a3412" checkmark="✗ after cutoff" />
            }
          </div>
        </div>

        {/* Footer */}
        <div style={{ padding: '12px 20px', borderTop: '1px solid #e5e7eb', display: 'flex', justifyContent: 'flex-end', gap: 10 }}>
          <button
            onClick={onClose}
            style={{ padding: '8px 16px', borderRadius: 6, border: '1px solid #d1d5db', background: '#fff', fontSize: 13, cursor: 'pointer' }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            disabled={confirming || included.length === 0}
            style={{
              padding: '8px 16px', borderRadius: 6, border: 'none',
              backgroundColor: ACCENT, color: 'white', fontSize: 13, fontWeight: 600,
              cursor: (confirming || included.length === 0) ? 'not-allowed' : 'pointer',
              opacity: (confirming || included.length === 0) ? 0.55 : 1,
            }}
          >
            {confirming ? 'Running…' : `Confirm Settlement (${included.length} deposit${included.length !== 1 ? 's' : ''})`}
          </button>
        </div>
      </div>
    </div>
  )
}

function EODStatusTab() {
  const [status, setStatus] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [showPreview, setShowPreview] = useState(false)
  const [preview, setPreview] = useState(null)
  const [confirming, setConfirming] = useState(false)
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

  async function handleTriggerClick() {
    setPreviewLoading(true)
    setSettlementResult(null)
    setSettlementError(null)
    try {
      const resp = await api.getSettlementPreview()
      setPreview(resp.data)
      setShowPreview(true)
    } catch (err) {
      setSettlementError(err?.error || 'Failed to load settlement preview')
    } finally {
      setPreviewLoading(false)
    }
  }

  async function handleConfirmSettlement() {
    const today = new Date().toISOString().slice(0, 10)
    setConfirming(true)
    try {
      const resp = await api.triggerSettlement(today)
      setSettlementResult(resp.data)
      setShowPreview(false)
      fetchStatus()
    } catch (err) {
      setSettlementError(err?.error || 'Settlement trigger failed')
      setShowPreview(false)
    } finally {
      setConfirming(false)
    }
  }

  if (loading) return <p style={{ color: '#6b7280', fontSize: 13 }}>Loading EOD status…</p>
  if (error) return <p style={{ color: '#dc2626', fontSize: 13 }}>{error}</p>

  const cutoff = status ? new Date(status.cutoff_time) : null

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {showPreview && preview && (
        <SettlementPreviewModal
          preview={preview}
          onClose={() => setShowPreview(false)}
          onConfirm={handleConfirmSettlement}
          confirming={confirming}
        />
      )}

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
          onClick={handleTriggerClick}
          disabled={previewLoading || confirming}
          style={{
            backgroundColor: ACCENT,
            color: 'white',
            border: 'none',
            borderRadius: 6,
            padding: '10px 20px',
            fontSize: 14,
            fontWeight: 600,
            cursor: (previewLoading || confirming) ? 'not-allowed' : 'pointer',
            opacity: (previewLoading || confirming) ? 0.6 : 1,
          }}
        >
          {previewLoading ? 'Loading preview…' : 'Trigger EOD Settlement'}
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

  function handleTabClick(tabId) {
    // Clicking Batch Detail without a batch selected redirects to Batches
    if (tabId === 'detail' && !selectedBatchId) {
      setActiveTab('batches')
    } else {
      setActiveTab(tabId)
    }
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
            onClick={() => handleTabClick(tab.id)}
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
            {tab.label}{tab.id === 'detail' && selectedBatchId ? ` (${selectedBatchId.slice(0, 8)}…)` : ''}
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
