import { useState, useEffect, useCallback } from 'react'
import { api } from '../api.js'

function fmtCents(cents) {
  return `$${(cents / 100).toFixed(2)}`
}

function fmtDate(iso) {
  return new Date(iso).toLocaleString()
}

function StatusBadge({ status }) {
  const styles = {
    analyzing: 'bg-yellow-100 text-yellow-800',
    funds_posted: 'bg-green-100 text-green-800',
    completed: 'bg-green-100 text-green-800',
    rejected: 'bg-red-100 text-red-800',
    returned: 'bg-orange-100 text-orange-800',
  }
  return (
    <span className={`px-2 py-0.5 rounded text-xs font-semibold uppercase ${styles[status] || 'bg-gray-100 text-gray-700'}`}>
      {status?.replace('_', ' ')}
    </span>
  )
}

function CheckCard({ deposit, onAction }) {
  const [showImages, setShowImages] = useState(false)
  const [approving, setApproving] = useState(false)
  const [rejecting, setRejecting] = useState(false)
  const [actionError, setActionError] = useState(null)

  const id = deposit.transfer_id

  async function handleApprove() {
    if (!window.confirm('Approve this deposit? Funds will be provisionally posted.')) return
    setApproving(true)
    setActionError(null)
    try {
      await api.approveDeposit(id, { notes: 'Approved via operator UI' })
      onAction() // fires immediately — no need to wait for next poll interval
    } catch (err) {
      setActionError(err?.error || 'Approve failed')
    } finally {
      setApproving(false)
    }
  }

  async function handleReject() {
    const reason = window.prompt('Enter rejection reason:')
    if (reason === null) return // cancelled
    setRejecting(true)
    setActionError(null)
    try {
      await api.rejectDeposit(id, { reason: reason || 'Rejected by operator', notes: '' })
      onAction() // fires immediately — no need to wait for next poll interval
    } catch (err) {
      setActionError(err?.error || 'Reject failed')
    } finally {
      setRejecting(false)
    }
  }

  return (
    <div className="bg-white border border-gray-200 rounded p-4 space-y-3">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <div className="flex items-center gap-2 flex-wrap">
            <StatusBadge status={deposit.status} />
            {deposit.flagged && (
              <span className="px-2 py-0.5 rounded text-xs font-semibold bg-orange-100 text-orange-700 uppercase">
                Flagged
              </span>
            )}
          </div>
          <p className="text-xs text-gray-500 font-mono">{id}</p>
          <p className="text-sm text-gray-700">
            Account: <strong>{deposit.account_id}</strong>
          </p>
          <p className="text-sm text-gray-700">
            Amount: <strong>{fmtCents(deposit.amount_cents)}</strong>
          </p>
          {deposit.flag_reason && (
            <p className="text-sm text-orange-700">
              Flag: <strong>{deposit.flag_reason}</strong>
            </p>
          )}
          {deposit.micr_routing && (
            <p className="text-xs text-gray-500">
              MICR routing: {deposit.micr_routing} · acct: {deposit.micr_account} · serial: {deposit.micr_serial}
              {deposit.micr_confidence != null && ` · confidence: ${(deposit.micr_confidence * 100).toFixed(0)}%`}
            </p>
          )}
          {deposit.ocr_amount_cents != null && (
            <p className="text-xs text-yellow-700">
              OCR amount: {fmtCents(deposit.ocr_amount_cents)} vs declared: {fmtCents(deposit.declared_amount_cents)}
            </p>
          )}
          <p className="text-xs text-gray-400">{fmtDate(deposit.created_at)}</p>
        </div>

        <div className="flex flex-col gap-2 shrink-0">
          <button
            onClick={handleApprove}
            disabled={approving || rejecting}
            className="px-3 py-1.5 bg-green-600 text-white text-xs font-medium rounded hover:bg-green-700 disabled:opacity-50"
          >
            {approving ? 'Approving…' : 'Approve'}
          </button>
          <button
            onClick={handleReject}
            disabled={approving || rejecting}
            className="px-3 py-1.5 bg-red-600 text-white text-xs font-medium rounded hover:bg-red-700 disabled:opacity-50"
          >
            {rejecting ? 'Rejecting…' : 'Reject'}
          </button>
          <button
            onClick={() => setShowImages(v => !v)}
            className="px-3 py-1.5 bg-gray-100 text-gray-700 text-xs font-medium rounded hover:bg-gray-200"
          >
            {showImages ? 'Hide Images' : 'Show Images'}
          </button>
        </div>
      </div>

      {actionError && (
        <p className="text-xs text-red-600">{actionError}</p>
      )}

      {showImages && (
        <div className="flex gap-3 flex-wrap">
          <div>
            <p className="text-xs text-gray-500 mb-1">Front</p>
            <img
              src={`/api/v1/deposits/${id}/images/front`}
              alt="Check front"
              className="w-48 border border-gray-200 rounded"
              onError={e => { e.target.style.display = 'none' }}
            />
          </div>
          <div>
            <p className="text-xs text-gray-500 mb-1">Back</p>
            <img
              src={`/api/v1/deposits/${id}/images/back`}
              alt="Check back"
              className="w-48 border border-gray-200 rounded"
              onError={e => { e.target.style.display = 'none' }}
            />
          </div>
        </div>
      )}
    </div>
  )
}

export default function ReviewQueue() {
  const [queue, setQueue] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [settlementResult, setSettlementResult] = useState(null)
  const [settlementError, setSettlementError] = useState(null)
  const [settling, setSettling] = useState(false)

  const [actionCount, setActionCount] = useState(0) // incremented after approve/reject to show "all caught up"

  const fetchQueue = useCallback(async () => {
    try {
      const resp = await api.getQueue()
      setQueue(resp.data || [])
      setError(null)
    } catch (err) {
      setError(err?.error || 'Failed to load operator queue')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchQueue()
    const timer = setInterval(fetchQueue, 5000)
    return () => clearInterval(timer)
  }, [fetchQueue])

  async function handleTriggerSettlement() {
    const today = new Date().toISOString().slice(0, 10)
    if (!window.confirm(`Trigger EOD settlement for ${today}?`)) return
    setSettling(true)
    setSettlementResult(null)
    setSettlementError(null)
    try {
      const resp = await api.triggerSettlement(today)
      setSettlementResult(resp.data)
    } catch (err) {
      setSettlementError(err?.error || 'Settlement failed')
    } finally {
      setSettling(false)
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-lg font-semibold text-gray-800">Operator Review Queue</h2>
          {queue.length > 0 && (
            <span className="px-2 py-0.5 bg-yellow-100 text-yellow-800 text-xs font-semibold rounded-full">
              {queue.length} pending
            </span>
          )}
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-gray-400">Refreshes every 5s</span>
          <button
            onClick={handleTriggerSettlement}
            disabled={settling}
            className="px-4 py-2 bg-blue-700 text-white text-sm font-medium rounded hover:bg-blue-800 disabled:opacity-50"
          >
            {settling ? 'Running Settlement…' : 'Trigger EOD Settlement'}
          </button>
        </div>
      </div>

      {settlementResult && (
        <div className={`p-3 border rounded text-sm space-y-1 ${settlementResult.status === 'rolled_to_next_day' ? 'bg-yellow-50 border-yellow-200 text-yellow-800' : 'bg-green-50 border-green-200 text-green-800'}`}>
          {settlementResult.status === 'rolled_to_next_day' ? (
            <>
              <p className="font-medium">After-cutoff: deposits rolled to next business day</p>
              <p>{settlementResult.deposits_rolled_to_next_day} deposit(s) queued for {settlementResult.next_settlement_date}</p>
            </>
          ) : (
            <>
              <p className="font-medium">Settlement complete</p>
              <p>Batch ID: <span className="font-mono">{settlementResult.batch_id}</span></p>
              <p>{settlementResult.deposit_count} deposit(s) · total {settlementResult.total_amount_cents != null ? `$${(settlementResult.total_amount_cents / 100).toFixed(2)}` : '—'}</p>
              {settlementResult.file_path && <p>File: {settlementResult.file_path}</p>}
              {settlementResult.deposits_rolled_to_next_day > 0 && (
                <p className="text-yellow-700">{settlementResult.deposits_rolled_to_next_day} deposit(s) after cutoff rolled to {settlementResult.next_settlement_date}</p>
              )}
            </>
          )}
        </div>
      )}

      {settlementError && (
        <div className="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
          Settlement error: {settlementError}
        </div>
      )}

      {loading && <p className="text-sm text-gray-500">Loading queue…</p>}
      {error && <p className="text-sm text-red-600">{error}</p>}

      {!loading && queue.length === 0 && !error && (
        <div className="py-12 text-center text-gray-500">
          {actionCount > 0
            ? <p className="text-sm font-medium text-green-700">All caught up! No more items in queue.</p>
            : <p className="text-sm">No flagged deposits in queue.</p>
          }
        </div>
      )}

      {queue.map(deposit => (
        <CheckCard
          key={deposit.transfer_id}
          deposit={deposit}
          onAction={() => { setActionCount(n => n + 1); fetchQueue() }}
        />
      ))}
    </div>
  )
}
