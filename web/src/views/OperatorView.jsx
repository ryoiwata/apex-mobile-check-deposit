import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from '../api.js'
import ReviewQueue from '../components/ReviewQueue.jsx'

const ACCENT = '#d97706'

function fmtCents(cents) {
  if (cents == null) return '—'
  return `$${(cents / 100).toFixed(2)}`
}

function fmtDate(iso) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

const CONTRIBUTION_TYPES = ['INDIVIDUAL', 'ROLLOVER', 'TRANSFER', 'ROTH_CONVERSION']

// ─── Deposit Detail Tab ───────────────────────────────────────────────────────

function DepositDetailTab({ deposit, onBack, onAction }) {
  const [approving, setApproving] = useState(false)
  const [rejecting, setRejecting] = useState(false)
  const [showRejectForm, setShowRejectForm] = useState(false)
  const [rejectReason, setRejectReason] = useState('')
  const [actionError, setActionError] = useState(null)
  const [lightbox, setLightbox] = useState(null)
  const [view, setView] = useState({ zoom: 1, x: 0, y: 0 })
  const containerRef = useRef(null)
  const [ctOverride, setCtOverride] = useState('')
  const [ctLoading, setCtLoading] = useState(false)
  const [ctError, setCtError] = useState(null)
  const [ctSuccess, setCtSuccess] = useState(false)

  // Reset state when deposit changes
  useEffect(() => {
    setActionError(null)
    setShowRejectForm(false)
    setRejectReason('')
    setCtOverride(deposit?.contribution_type || '')
    setCtSuccess(false)
    setCtError(null)
  }, [deposit?.transfer_id])

  if (!deposit) {
    return (
      <p style={{ color: '#6b7280', fontSize: 13, textAlign: 'center', padding: '40px 0' }}>
        Click a deposit in the Review Queue tab to open its detail view.
      </p>
    )
  }

  const id = deposit.transfer_id
  const hasMICR = !!(deposit.micr_routing || deposit.micr_account)
  const amountMismatch = deposit.ocr_amount_cents != null &&
    deposit.ocr_amount_cents !== deposit.declared_amount_cents

  async function handleApprove() {
    if (!window.confirm('Approve this deposit? Funds will be provisionally posted to the investor account.')) return
    setApproving(true)
    setActionError(null)
    try {
      await api.approveDeposit(id, { notes: 'Approved via operator review' })
      onAction?.()
    } catch (err) {
      setActionError(err?.error || 'Approve failed')
    } finally {
      setApproving(false)
    }
  }

  async function handleReject(e) {
    e.preventDefault()
    if (!rejectReason.trim()) return
    if (!window.confirm(`Reject this deposit?\nReason: "${rejectReason}"`)) return
    setRejecting(true)
    setActionError(null)
    try {
      await api.rejectDeposit(id, { reason: rejectReason.trim(), notes: '' })
      onAction?.()
    } catch (err) {
      setActionError(err?.error || 'Reject failed')
    } finally {
      setRejecting(false)
    }
  }

  async function handleCtOverride() {
    if (!ctOverride) return
    setCtLoading(true)
    setCtError(null)
    setCtSuccess(false)
    try {
      await api.overrideContributionType(id, ctOverride)
      setCtSuccess(true)
    } catch (err) {
      setCtError(err?.error || 'Override failed')
    } finally {
      setCtLoading(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
        <button
          onClick={onBack}
          style={{ fontSize: 13, color: ACCENT, background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}
        >
          ← Back to queue
        </button>
        <span style={{ backgroundColor: '#fff7ed', color: '#c2410c', fontSize: 11, fontWeight: 700, padding: '2px 8px', borderRadius: 4 }}>
          FLAGGED: {deposit.flag_reason || 'unknown'}
        </span>
        <span style={{ fontFamily: 'monospace', fontSize: 11, color: '#9ca3af' }}>{id}</span>
      </div>

      {/* Check images */}
      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
        <h4 style={{ margin: '0 0 12px', fontSize: 12, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: '#6b7280' }}>
          Check Images
        </h4>
        <div style={{ display: 'flex', gap: 20, flexWrap: 'wrap' }}>
          {['front', 'back'].map(side => (
            <div key={side}>
              <p style={{ fontSize: 11, color: '#6b7280', marginBottom: 6, textTransform: 'capitalize' }}>{side}</p>
              <img
                src={`/api/v1/deposits/${id}/images/${side}`}
                alt={`Check ${side}`}
                style={{ width: 220, height: 'auto', border: '1px solid #e5e7eb', borderRadius: 4, cursor: 'zoom-in', display: 'block' }}
                onClick={() => { setLightbox(`/api/v1/deposits/${id}/images/${side}`); setView({ zoom: 1, x: 0, y: 0 }) }}
                onError={e => {
                  e.target.style.cssText = 'width:220px;height:110px;background:#f3f4f6;display:flex;align-items:center;justify-content:center;border:1px dashed #d1d5db;border-radius:4px'
                  e.target.alt = 'Image unavailable'
                }}
              />
            </div>
          ))}
        </div>
      </div>

      {/* Lightbox */}
      {lightbox && (
        <div
          ref={containerRef}
          style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.85)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 50, cursor: view.zoom > 1 ? 'grab' : 'zoom-in' }}
          onClick={() => setLightbox(null)}
          onWheel={e => {
            e.preventDefault()
            const rect = containerRef.current.getBoundingClientRect()
            const cx = e.clientX - rect.left - rect.width / 2
            const cy = e.clientY - rect.top - rect.height / 2
            const factor = Math.exp(-e.deltaY * 0.0012)
            setView(v => {
              const newZoom = Math.min(10, Math.max(0.5, v.zoom * factor))
              const scale = newZoom / v.zoom
              return { zoom: newZoom, x: cx - scale * (cx - v.x), y: cy - scale * (cy - v.y) }
            })
          }}
        >
          <img
            src={lightbox}
            alt="Check enlarged"
            style={{ transform: `translate(${view.x}px, ${view.y}px) scale(${view.zoom})`, transformOrigin: 'center', maxWidth: '85vw', maxHeight: '85vh', objectFit: 'contain', userSelect: 'none' }}
            onClick={e => e.stopPropagation()}
            draggable={false}
          />
          <div style={{ position: 'fixed', top: 16, right: 24, display: 'flex', gap: 12, alignItems: 'center' }}>
            <span style={{ color: 'white', fontSize: 11, backgroundColor: 'rgba(0,0,0,0.5)', padding: '4px 8px', borderRadius: 4 }}>
              scroll to zoom · {Math.round(view.zoom * 100)}%
            </span>
            <button
              style={{ color: 'white', fontSize: 28, fontWeight: 700, background: 'none', border: 'none', cursor: 'pointer', lineHeight: 1 }}
              onClick={() => setLightbox(null)}
            >
              ×
            </button>
          </div>
        </div>
      )}

      {/* MICR data */}
      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
        <h4 style={{ margin: '0 0 12px', fontSize: 12, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: '#6b7280' }}>
          MICR Data
        </h4>
        {!hasMICR ? (
          <p style={{ color: '#c2410c', fontWeight: 700, fontSize: 14, margin: 0 }}>
            ⚠ MICR READ FAILURE — Routing/account data not extracted from check
          </p>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: '8px 16px' }}>
            {[
              ['Routing', deposit.micr_routing],
              ['Account', deposit.micr_account],
              ['Serial', deposit.micr_serial],
              ['Confidence', deposit.micr_confidence != null ? `${(deposit.micr_confidence * 100).toFixed(0)}%` : '—'],
            ].map(([label, val]) => (
              <div key={label}>
                <p style={{ fontSize: 11, color: '#6b7280', margin: '0 0 2px' }}>{label}</p>
                <p style={{ fontFamily: 'monospace', fontSize: 12, margin: 0, fontWeight: 500 }}>{val || '—'}</p>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Amount comparison */}
      <div style={{ background: amountMismatch ? '#fffbeb' : '#fff', border: `1px solid ${amountMismatch ? '#fde68a' : '#e5e7eb'}`, borderRadius: 8, padding: 16 }}>
        <h4 style={{ margin: '0 0 12px', fontSize: 12, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: '#6b7280' }}>
          Amount Comparison{amountMismatch && <span style={{ color: '#c2410c', marginLeft: 8 }}>⚠ MISMATCH</span>}
        </h4>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '8px 16px' }}>
          {[
            ['Investor Entered', fmtCents(deposit.declared_amount_cents)],
            ['OCR Recognized', deposit.ocr_amount_cents != null ? fmtCents(deposit.ocr_amount_cents) : 'Not extracted'],
            ['Difference', deposit.ocr_amount_cents != null ? fmtCents(Math.abs(deposit.ocr_amount_cents - deposit.declared_amount_cents)) : '—'],
          ].map(([label, val]) => (
            <div key={label}>
              <p style={{ fontSize: 11, color: '#6b7280', margin: '0 0 2px' }}>{label}</p>
              <p style={{ fontSize: 15, fontWeight: 600, margin: 0, color: label === 'Difference' && amountMismatch ? '#c2410c' : '#374151' }}>{val}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Account context + contribution type override */}
      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
        <h4 style={{ margin: '0 0 12px', fontSize: 12, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: '#6b7280' }}>
          Account & Contribution Type
        </h4>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16, fontSize: 13 }}>
          <div>
            <p style={{ fontSize: 11, color: '#6b7280', margin: '0 0 2px' }}>Account ID</p>
            <p style={{ fontFamily: 'monospace', fontSize: 12, fontWeight: 600, margin: '0 0 8px' }}>{deposit.account_id}</p>
            <p style={{ fontSize: 11, color: '#6b7280', margin: '0 0 2px' }}>Submitted</p>
            <p style={{ fontSize: 12, margin: 0 }}>{fmtDate(deposit.created_at)}</p>
          </div>
          <div>
            <p style={{ fontSize: 11, color: '#6b7280', margin: '0 0 6px' }}>
              Contribution Type
              {deposit.contribution_type && <span style={{ marginLeft: 6, color: '#374151', fontWeight: 600 }}>(current: {deposit.contribution_type})</span>}
            </p>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
              <select
                value={ctOverride}
                onChange={e => setCtOverride(e.target.value)}
                style={{ border: '1px solid #d1d5db', borderRadius: 4, padding: '5px 8px', fontSize: 13 }}
              >
                <option value="">— system default —</option>
                {CONTRIBUTION_TYPES.map(ct => (
                  <option key={ct} value={ct}>{ct}</option>
                ))}
              </select>
              <button
                onClick={handleCtOverride}
                disabled={ctLoading || !ctOverride}
                style={{ backgroundColor: ACCENT, color: 'white', border: 'none', borderRadius: 4, padding: '5px 12px', fontSize: 12, fontWeight: 600, cursor: ctLoading || !ctOverride ? 'not-allowed' : 'pointer', opacity: ctLoading || !ctOverride ? 0.6 : 1 }}
              >
                {ctLoading ? 'Saving…' : 'Override'}
              </button>
            </div>
            {ctSuccess && <p style={{ fontSize: 11, color: '#059669', margin: '4px 0 0' }}>Override saved.</p>}
            {ctError && <p style={{ fontSize: 11, color: '#dc2626', margin: '4px 0 0' }}>{ctError}</p>}
          </div>
        </div>
      </div>

      {/* Operator decision */}
      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
        <h4 style={{ margin: '0 0 14px', fontSize: 12, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.05em', color: '#6b7280' }}>
          Operator Decision
        </h4>
        {actionError && (
          <p style={{ color: '#dc2626', fontSize: 13, marginBottom: 12 }}>{actionError}</p>
        )}
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap' }}>
          <button
            onClick={handleApprove}
            disabled={approving || rejecting || showRejectForm}
            style={{ backgroundColor: '#059669', color: 'white', border: 'none', borderRadius: 6, padding: '10px 22px', fontSize: 14, fontWeight: 600, cursor: approving ? 'not-allowed' : 'pointer', opacity: approving || showRejectForm ? 0.5 : 1 }}
          >
            {approving ? 'Approving…' : '✓ Approve Deposit'}
          </button>
          <button
            onClick={() => setShowRejectForm(v => !v)}
            disabled={approving}
            style={{ backgroundColor: showRejectForm ? '#fee2e2' : 'white', color: '#dc2626', border: '2px solid #dc2626', borderRadius: 6, padding: '10px 22px', fontSize: 14, fontWeight: 600, cursor: 'pointer', opacity: approving ? 0.5 : 1 }}
          >
            {showRejectForm ? 'Cancel Rejection' : '✕ Reject Deposit'}
          </button>
        </div>

        {showRejectForm && (
          <form onSubmit={handleReject} style={{ marginTop: 16, display: 'flex', flexDirection: 'column', gap: 8, maxWidth: 480 }}>
            <label style={{ fontSize: 13, fontWeight: 500, color: '#374151' }}>
              Rejection reason (required):
            </label>
            <input
              type="text"
              value={rejectReason}
              onChange={e => setRejectReason(e.target.value)}
              placeholder="e.g. Check image appears altered, MICR data inconsistent"
              required
              autoFocus
              style={{ border: '1px solid #d1d5db', borderRadius: 4, padding: '8px 12px', fontSize: 13 }}
            />
            <div style={{ display: 'flex', gap: 8 }}>
              <button
                type="submit"
                disabled={rejecting || !rejectReason.trim()}
                style={{ backgroundColor: '#dc2626', color: 'white', border: 'none', borderRadius: 6, padding: '8px 16px', fontSize: 13, fontWeight: 600, cursor: rejecting || !rejectReason.trim() ? 'not-allowed' : 'pointer', opacity: rejecting || !rejectReason.trim() ? 0.6 : 1 }}
              >
                {rejecting ? 'Rejecting…' : 'Confirm Reject'}
              </button>
              <button
                type="button"
                onClick={() => { setShowRejectForm(false); setRejectReason('') }}
                style={{ background: 'none', border: '1px solid #d1d5db', borderRadius: 6, padding: '8px 16px', fontSize: 13, cursor: 'pointer', color: '#6b7280' }}
              >
                Cancel
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  )
}

// ─── Audit Log Tab ────────────────────────────────────────────────────────────

function AuditLogTab() {
  const [entries, setEntries] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [filterAction, setFilterAction] = useState('')

  const fetchLog = useCallback(async () => {
    try {
      const resp = await api.getAuditLog()
      setEntries(resp.data || [])
      setError(null)
    } catch (err) {
      setError(err?.error || 'Failed to load audit log')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchLog()
    const timer = setInterval(fetchLog, 10000)
    return () => clearInterval(timer)
  }, [fetchLog])

  const filtered = filterAction
    ? entries.filter(e => e.action === filterAction)
    : entries

  const actionColor = { approve: '#059669', reject: '#dc2626', override: '#d97706' }

  return (
    <div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16 }}>
        <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>Operator Audit Log</h3>
        <select
          value={filterAction}
          onChange={e => setFilterAction(e.target.value)}
          style={{ fontSize: 13, border: '1px solid #d1d5db', borderRadius: 4, padding: '4px 8px' }}
        >
          <option value="">All actions</option>
          <option value="approve">Approve</option>
          <option value="reject">Reject</option>
          <option value="override">Override</option>
        </select>
        <span style={{ fontSize: 12, color: '#9ca3af', marginLeft: 'auto' }}>Refreshes every 10s</span>
      </div>

      {loading && <p style={{ color: '#6b7280', fontSize: 13 }}>Loading audit log…</p>}
      {error && <p style={{ color: '#dc2626', fontSize: 13 }}>{error}</p>}

      {!loading && filtered.length === 0 && (
        <p style={{ color: '#6b7280', fontSize: 13, textAlign: 'center', padding: '40px 0' }}>
          No audit entries found.
        </p>
      )}

      {filtered.length > 0 && (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ backgroundColor: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
                {['Timestamp', 'Operator', 'Action', 'Transfer ID', 'Notes'].map(h => (
                  <th key={h} style={{ padding: '8px 12px', textAlign: 'left', fontSize: 11, color: '#6b7280', textTransform: 'uppercase', letterSpacing: '0.05em' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.map(entry => (
                <tr key={entry.id} style={{ borderBottom: '1px solid #f3f4f6' }}>
                  <td style={{ padding: '8px 12px', color: '#6b7280', whiteSpace: 'nowrap' }}>{fmtDate(entry.created_at)}</td>
                  <td style={{ padding: '8px 12px', fontWeight: 500 }}>{entry.operator_id}</td>
                  <td style={{ padding: '8px 12px' }}>
                    <span style={{
                      backgroundColor: `${actionColor[entry.action] || '#6b7280'}22`,
                      color: actionColor[entry.action] || '#6b7280',
                      padding: '2px 8px',
                      borderRadius: 4,
                      fontSize: 11,
                      fontWeight: 700,
                      textTransform: 'uppercase',
                    }}>
                      {entry.action}
                    </span>
                  </td>
                  <td style={{ padding: '8px 12px', fontFamily: 'monospace', fontSize: 11, color: '#6b7280' }}>
                    {entry.transfer_id?.slice(0, 8)}…
                  </td>
                  <td style={{ padding: '8px 12px', color: '#374151' }}>{entry.notes || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

// ─── OperatorView ─────────────────────────────────────────────────────────────

const TABS = [
  { id: 'queue', label: 'Review Queue' },
  { id: 'detail', label: 'Deposit Detail' },
  { id: 'audit', label: 'Audit Log' },
]

export default function OperatorView() {
  const [activeTab, setActiveTab] = useState('queue')
  const [selectedDeposit, setSelectedDeposit] = useState(null)
  const [queueCount, setQueueCount] = useState(0)

  function handleSelectDeposit(deposit) {
    setSelectedDeposit(deposit)
    setActiveTab('detail')
  }

  function handleActionComplete() {
    setSelectedDeposit(null)
    setActiveTab('queue')
  }

  return (
    <div>
      <div style={{ padding: '12px 0 8px', marginBottom: 8 }}>
        <p style={{ color: '#64748b', fontSize: 13, margin: 0 }}>
          Review flagged deposits, approve or reject, and view audit history.
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
              display: 'flex',
              alignItems: 'center',
              gap: 6,
            }}
          >
            {tab.label}
            {tab.id === 'queue' && queueCount > 0 && (
              <span style={{ backgroundColor: ACCENT, color: 'white', fontSize: 10, fontWeight: 700, borderRadius: 10, padding: '1px 6px', lineHeight: 1.5 }}>
                {queueCount > 9 ? '9+' : queueCount}
              </span>
            )}
          </button>
        ))}
      </nav>

      {activeTab === 'queue' && (
        <ReviewQueue
          onSelectDeposit={handleSelectDeposit}
          onQueueCountChange={setQueueCount}
        />
      )}
      {activeTab === 'detail' && (
        <DepositDetailTab
          deposit={selectedDeposit}
          onBack={() => setActiveTab('queue')}
          onAction={handleActionComplete}
        />
      )}
      {activeTab === 'audit' && <AuditLogTab />}
    </div>
  )
}
