import { useState, useEffect, useRef } from 'react'
import { api } from '../api.js'

const TERMINAL_STATES = new Set(['funds_posted', 'completed', 'rejected', 'returned'])

function fmtCents(cents) {
  if (cents == null) return '—'
  return `$${(cents / 100).toFixed(2)}`
}

function fmtDate(iso) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

const STATUS_STYLES = {
  funds_posted: 'bg-green-100 text-green-800',
  completed: 'bg-green-100 text-green-800',
  rejected: 'bg-red-100 text-red-800',
  analyzing: 'bg-yellow-100 text-yellow-800',
  validating: 'bg-blue-100 text-blue-800',
  approved: 'bg-blue-100 text-blue-800',
  requested: 'bg-gray-100 text-gray-700',
  returned: 'bg-orange-100 text-orange-800',
}

/**
 * @param {{ initialTransferId: string | null, onStartNewDeposit?: (accountId: string) => void }} props
 */
export default function TransferStatus({ initialTransferId, onStartNewDeposit }) {
  const [inputId, setInputId] = useState(initialTransferId || '')
  const [transfer, setTransfer] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const pollRef = useRef(null)

  function stopPolling() {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }

  async function fetchTransfer(id) {
    try {
      const resp = await api.getDeposit(id)
      setTransfer(resp.data)
      setError(null)
      if (TERMINAL_STATES.has(resp.data?.status)) {
        stopPolling()
      }
    } catch (err) {
      setError(err?.error || 'Failed to fetch transfer')
      stopPolling()
    }
  }

  async function handleLookup(e) {
    if (e) e.preventDefault()
    const id = inputId.trim()
    if (!id) return
    setLoading(true)
    setError(null)
    setTransfer(null)
    stopPolling()
    await fetchTransfer(id)
    setLoading(false)
    if (!TERMINAL_STATES.has(transfer?.status)) {
      pollRef.current = setInterval(() => fetchTransfer(id), 2000)
    }
  }

  // Auto-load if we got an initial ID from DepositForm
  useEffect(() => {
    if (initialTransferId) {
      setInputId(initialTransferId)
      setLoading(true)
      fetchTransfer(initialTransferId).then(() => setLoading(false))
    }
    return stopPolling
  }, [initialTransferId])

  // Start polling after transfer loads if non-terminal
  useEffect(() => {
    if (transfer && !TERMINAL_STATES.has(transfer.status) && inputId) {
      stopPolling()
      pollRef.current = setInterval(() => fetchTransfer(inputId), 2000)
    }
    return () => {}
  }, [transfer?.status])

  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-gray-800">Transfer Status</h2>

      <form onSubmit={handleLookup} className="flex gap-2">
        <input
          type="text"
          value={inputId}
          onChange={e => setInputId(e.target.value)}
          placeholder="Enter transfer ID (UUID)"
          className="flex-1 border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono"
        />
        <button
          type="submit"
          disabled={loading || !inputId.trim()}
          className="px-4 py-2 bg-blue-700 text-white text-sm font-medium rounded hover:bg-blue-800 disabled:opacity-50"
        >
          {loading ? 'Loading…' : 'Look Up'}
        </button>
      </form>

      {error && (
        <div className="p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
          {error}
        </div>
      )}

      {transfer && (
        <div className="bg-white border border-gray-200 rounded shadow-sm divide-y divide-gray-100">
          {/* Header */}
          <div className="p-4 flex items-center gap-3">
            <span className={`px-2 py-0.5 rounded text-xs font-semibold uppercase ${STATUS_STYLES[transfer.status] || 'bg-gray-100 text-gray-700'}`}>
              {transfer.status?.replace('_', ' ')}
            </span>
            {transfer.flagged && (
              <span className="px-2 py-0.5 rounded text-xs font-semibold bg-orange-100 text-orange-700 uppercase">
                Flagged{transfer.flag_reason ? `: ${transfer.flag_reason}` : ''}
              </span>
            )}
            {!TERMINAL_STATES.has(transfer.status) && (
              <span className="text-xs text-gray-400 animate-pulse">polling…</span>
            )}
          </div>

          {/* Fields */}
          <div className="p-4 grid grid-cols-2 gap-x-6 gap-y-2 text-sm">
            <FieldRow label="Transfer ID" value={<span className="font-mono text-xs">{transfer.transfer_id}</span>} />
            <FieldRow label="Account ID" value={transfer.account_id} />
            <FieldRow label="Amount" value={fmtCents(transfer.amount_cents)} />
            <FieldRow label="Declared Amount" value={fmtCents(transfer.declared_amount_cents)} />
            {transfer.contribution_type && (
              <FieldRow label="Contribution Type" value={transfer.contribution_type} />
            )}
            {transfer.micr_routing && (
              <FieldRow label="MICR Routing" value={transfer.micr_routing} />
            )}
            {transfer.micr_account && (
              <FieldRow label="MICR Account" value={transfer.micr_account} />
            )}
            {transfer.micr_confidence != null && (
              <FieldRow label="MICR Confidence" value={`${(transfer.micr_confidence * 100).toFixed(0)}%`} />
            )}
            {transfer.ocr_amount_cents != null && (
              <FieldRow label="OCR Amount" value={fmtCents(transfer.ocr_amount_cents)} />
            )}
            {transfer.return_reason && (
              <FieldRow label="Return Reason" value={transfer.return_reason} />
            )}
            <FieldRow label="Created" value={fmtDate(transfer.created_at)} />
            <FieldRow label="Updated" value={fmtDate(transfer.updated_at)} />
          </div>

          {/* Return notification */}
          {transfer.status === 'returned' && (
            <div className="p-4 bg-orange-50 border-t border-orange-200 space-y-2">
              <p className="text-sm font-semibold text-orange-800">Check Returned</p>
              {transfer.return_reason && (
                <p className="text-sm text-orange-700">Reason: <strong>{transfer.return_reason}</strong></p>
              )}
              <p className="text-sm text-orange-700">
                A $30.00 return fee has been deducted from your account.
              </p>
              <p className="text-sm text-orange-600">You may submit a new deposit with a different check.</p>
              {onStartNewDeposit && (
                <button
                  onClick={() => onStartNewDeposit(transfer.account_id)}
                  className="mt-1 px-4 py-2 bg-blue-700 text-white text-sm font-medium rounded hover:bg-blue-800"
                >
                  Start New Deposit
                </button>
              )}
            </div>
          )}

          {/* State history */}
          {transfer.state_history?.length > 0 && (
            <div className="p-4">
              <h3 className="text-xs font-semibold text-gray-500 uppercase tracking-wide mb-2">State History</h3>
              <ol className="space-y-1">
                {transfer.state_history.map((h, i) => (
                  <li key={i} className="flex items-center gap-2 text-xs text-gray-600">
                    <span className="w-1.5 h-1.5 rounded-full bg-blue-400 shrink-0" />
                    <span className="font-mono">{h.from}</span>
                    <span className="text-gray-400">→</span>
                    <span className="font-mono font-medium">{h.to}</span>
                    <span className="text-gray-400 ml-auto">{fmtDate(h.at)}</span>
                    {h.triggered_by && <span className="text-gray-400">by {h.triggered_by}</span>}
                  </li>
                ))}
              </ol>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function FieldRow({ label, value }) {
  return (
    <>
      <span className="text-gray-500">{label}</span>
      <span className="text-gray-800">{value}</span>
    </>
  )
}
