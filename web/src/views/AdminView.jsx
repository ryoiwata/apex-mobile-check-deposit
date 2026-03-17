import { useState, useEffect, useCallback } from 'react'
import { api } from '../api.js'

const ACCENT = '#7c3aed'

function fmtCents(cents) {
  if (cents == null) return '—'
  return `$${(cents / 100).toFixed(2)}`
}

function fmtDate(iso) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

const STATUS_COLORS = {
  requested: '#6b7280',
  validating: '#2563eb',
  analyzing: '#d97706',
  approved: '#059669',
  funds_posted: '#059669',
  completed: '#065f46',
  rejected: '#dc2626',
  returned: '#c2410c',
}

function StatusBadge({ status }) {
  const color = STATUS_COLORS[status] || '#6b7280'
  return (
    <span style={{
      backgroundColor: `${color}18`,
      color,
      fontSize: 11,
      fontWeight: 700,
      padding: '2px 8px',
      borderRadius: 4,
      textTransform: 'uppercase',
    }}>
      {status?.replace(/_/g, ' ')}
    </span>
  )
}

// ─── Deposit Trace Tab ──────────────────────────────────────────────────────

function DepositTraceTab({ initialTransferId }) {
  const [inputId, setInputId] = useState(initialTransferId || '')
  const [trace, setTrace] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  async function handleLookup(e) {
    if (e) e.preventDefault()
    const id = inputId.trim()
    if (!id) return
    setLoading(true)
    setError(null)
    setTrace(null)
    try {
      const resp = await api.getDepositTrace(id)
      setTrace(resp.data)
    } catch (err) {
      setError(err?.error || 'Transfer not found')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (initialTransferId) {
      setInputId(initialTransferId)
      setLoading(true)
      api.getDepositTrace(initialTransferId)
        .then(resp => setTrace(resp.data))
        .catch(err => setError(err?.error || 'Transfer not found'))
        .finally(() => setLoading(false))
    }
  }, [initialTransferId])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      <form onSubmit={handleLookup} style={{ display: 'flex', gap: 8 }}>
        <input
          type="text"
          value={inputId}
          onChange={e => setInputId(e.target.value)}
          placeholder="Enter transfer ID (UUID)"
          style={{
            flex: 1,
            border: '1px solid #d1d5db',
            borderRadius: 6,
            padding: '8px 12px',
            fontSize: 13,
            fontFamily: 'monospace',
          }}
        />
        <button
          type="submit"
          disabled={loading || !inputId.trim()}
          style={{
            backgroundColor: ACCENT,
            color: 'white',
            border: 'none',
            borderRadius: 6,
            padding: '8px 16px',
            fontSize: 13,
            fontWeight: 600,
            cursor: loading ? 'not-allowed' : 'pointer',
            opacity: loading ? 0.6 : 1,
          }}
        >
          {loading ? 'Loading…' : 'Trace'}
        </button>
      </form>

      {error && <p style={{ color: '#dc2626', fontSize: 13 }}>{error}</p>}

      {trace && <TraceDisplay trace={trace} />}
    </div>
  )
}

function TraceDisplay({ trace }) {
  const t = trace.transfer
  if (!t) return null

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* Transfer summary */}
      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 12 }}>
          <StatusBadge status={t.status} />
          {t.flagged && (
            <span style={{ backgroundColor: '#fff7ed', color: '#c2410c', fontSize: 11, fontWeight: 700, padding: '2px 8px', borderRadius: 4 }}>
              FLAGGED: {t.flag_reason}
            </span>
          )}
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '8px 16px', fontSize: 13 }}>
          {[
            ['Transfer ID', <span style={{ fontFamily: 'monospace', fontSize: 11 }}>{t.transfer_id || t.id}</span>],
            ['Account', t.account_id],
            ['Amount', fmtCents(t.amount_cents)],
            ['Declared Amount', fmtCents(t.declared_amount_cents)],
            ['OCR Amount', t.ocr_amount_cents ? fmtCents(t.ocr_amount_cents) : '—'],
            ['Contribution Type', t.contribution_type || '—'],
            ['MICR Routing', t.micr_routing || '—'],
            ['MICR Account', t.micr_account || '—'],
            ['MICR Confidence', t.micr_confidence ? `${(t.micr_confidence * 100).toFixed(0)}%` : '—'],
            ['Settlement Batch', t.settlement_batch_id || '—'],
            ['Created', fmtDate(t.created_at)],
            ['Updated', fmtDate(t.updated_at)],
          ].map(([label, val]) => (
            <div key={label}>
              <p style={{ fontSize: 11, color: '#6b7280', margin: '0 0 2px' }}>{label}</p>
              <p style={{ fontSize: 13, margin: 0, fontWeight: 500 }}>{val}</p>
            </div>
          ))}
        </div>
      </div>

      {/* Timeline */}
      <Section title="State Timeline">
        {trace.state_transitions?.length === 0 ? (
          <p style={{ fontSize: 13, color: '#6b7280' }}>No state transitions recorded.</p>
        ) : (
          <ol style={{ listStyle: 'none', padding: 0, margin: 0, display: 'flex', flexDirection: 'column', gap: 6 }}>
            {trace.state_transitions?.map((st, i) => (
              <li key={i} style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
                <span style={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: ACCENT, flexShrink: 0 }} />
                <code style={{ fontSize: 11 }}>{st.from_state}</code>
                <span style={{ color: '#9ca3af' }}>→</span>
                <code style={{ fontSize: 11, fontWeight: 700 }}>{st.to_state}</code>
                {st.triggered_by && <span style={{ color: '#9ca3af', fontSize: 11 }}>by {st.triggered_by}</span>}
                <span style={{ color: '#9ca3af', fontSize: 11, marginLeft: 'auto' }}>{fmtDate(st.created_at)}</span>
              </li>
            ))}
          </ol>
        )}
      </Section>

      {/* Audit logs */}
      {trace.audit_logs?.length > 0 && (
        <Section title="Operator Actions">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {trace.audit_logs.map((a, i) => (
              <div key={i} style={{ display: 'flex', gap: 12, fontSize: 13, padding: '6px 0', borderBottom: '1px solid #f3f4f6' }}>
                <span style={{ color: '#6b7280', fontSize: 11, whiteSpace: 'nowrap' }}>{fmtDate(a.created_at)}</span>
                <span style={{ fontWeight: 600 }}>{a.operator_id}</span>
                <span style={{
                  backgroundColor: a.action === 'approve' ? '#d1fae5' : a.action === 'reject' ? '#fee2e2' : '#fef3c7',
                  color: a.action === 'approve' ? '#065f46' : a.action === 'reject' ? '#991b1b' : '#92400e',
                  fontSize: 11, fontWeight: 700, padding: '1px 6px', borderRadius: 3,
                }}>
                  {a.action.toUpperCase()}
                </span>
                {a.notes && <span style={{ color: '#374151' }}>{a.notes}</span>}
              </div>
            ))}
          </div>
        </Section>
      )}

      {/* Ledger entries */}
      {trace.ledger_entries?.length > 0 && (
        <Section title="Ledger Entries">
          {trace.ledger_entries.map((e, i) => (
            <div key={i} style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: 13, padding: '4px 0' }}>
              <span style={{
                color: e.sub_type === 'DEPOSIT' ? '#059669' : '#dc2626',
                fontWeight: 700, fontSize: 11,
              }}>
                {e.sub_type}
              </span>
              <span style={{ fontWeight: 600 }}>
                {e.sub_type === 'DEPOSIT' ? '+' : '-'}{fmtCents(e.amount_cents)}
              </span>
              <span style={{ color: '#9ca3af', fontSize: 11 }}>{e.from_account_id} → {e.to_account_id}</span>
              <span style={{ color: '#9ca3af', fontSize: 11, marginLeft: 'auto' }}>{fmtDate(e.created_at)}</span>
            </div>
          ))}
        </Section>
      )}

      {/* Notifications */}
      {trace.notifications?.length > 0 && (
        <Section title="Investor Notifications">
          {trace.notifications.map((n, i) => (
            <div key={i} style={{ padding: '6px 0', borderBottom: '1px solid #f3f4f6', fontSize: 13 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <strong>{n.title}</strong>
                <span style={{ color: '#9ca3af', fontSize: 11 }}>{fmtDate(n.created_at)}</span>
              </div>
              <p style={{ color: '#374151', margin: '2px 0 0', fontSize: 12 }}>{n.message}</p>
            </div>
          ))}
        </Section>
      )}
    </div>
  )
}

function Section({ title, children }) {
  return (
    <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
      <h4 style={{ margin: '0 0 10px', fontSize: 13, fontWeight: 600, color: '#374151', textTransform: 'uppercase', letterSpacing: '0.05em' }}>
        {title}
      </h4>
      {children}
    </div>
  )
}

// ─── All Deposits Tab ───────────────────────────────────────────────────────

function AllDepositsTab({ onTraceTransfer }) {
  const [deposits, setDeposits] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [filterStatus, setFilterStatus] = useState('')
  const [filterAccount, setFilterAccount] = useState('')

  const fetchDeposits = useCallback(async () => {
    const params = { limit: 100 }
    if (filterStatus) params.status = filterStatus
    if (filterAccount) params.account_id = filterAccount
    try {
      const resp = await api.listAllDeposits(params)
      setDeposits(resp.data || [])
      setTotal(resp.pagination?.total ?? 0)
      setError(null)
    } catch (err) {
      setError(err?.error || 'Failed to load deposits')
    } finally {
      setLoading(false)
    }
  }, [filterStatus, filterAccount])

  useEffect(() => {
    fetchDeposits()
    const timer = setInterval(fetchDeposits, 10000)
    return () => clearInterval(timer)
  }, [fetchDeposits])

  return (
    <div>
      <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
        <select
          value={filterStatus}
          onChange={e => setFilterStatus(e.target.value)}
          style={{ fontSize: 13, border: '1px solid #d1d5db', borderRadius: 4, padding: '6px 10px' }}
        >
          <option value="">All statuses</option>
          {['requested','validating','analyzing','approved','funds_posted','completed','rejected','returned'].map(s => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
        <input
          type="text"
          value={filterAccount}
          onChange={e => setFilterAccount(e.target.value)}
          placeholder="Filter by account ID"
          style={{ fontSize: 13, border: '1px solid #d1d5db', borderRadius: 4, padding: '6px 10px', width: 200 }}
        />
        <span style={{ fontSize: 12, color: '#9ca3af', alignSelf: 'center' }}>{total} total · refreshes every 10s</span>
      </div>

      {loading && <p style={{ color: '#6b7280', fontSize: 13 }}>Loading…</p>}
      {error && <p style={{ color: '#dc2626', fontSize: 13 }}>{error}</p>}

      {!loading && deposits.length === 0 && (
        <p style={{ color: '#6b7280', fontSize: 13, textAlign: 'center', padding: '40px 0' }}>No deposits found.</p>
      )}

      {deposits.length > 0 && (
        <div style={{ overflowX: 'auto' }}>
          <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
            <thead>
              <tr style={{ backgroundColor: '#f9fafb', borderBottom: '1px solid #e5e7eb' }}>
                {['Transfer ID', 'Account', 'Amount', 'Status', 'Flagged', 'Created'].map(h => (
                  <th key={h} style={{ padding: '8px 10px', textAlign: 'left', fontSize: 11, color: '#6b7280', textTransform: 'uppercase' }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {deposits.map(d => (
                <tr
                  key={d.transfer_id || d.id}
                  onClick={() => onTraceTransfer(d.transfer_id || d.id)}
                  style={{ borderBottom: '1px solid #f3f4f6', cursor: 'pointer' }}
                  onMouseEnter={e => e.currentTarget.style.backgroundColor = '#f9fafb'}
                  onMouseLeave={e => e.currentTarget.style.backgroundColor = ''}
                >
                  <td style={{ padding: '8px 10px', fontFamily: 'monospace', fontSize: 11, color: '#6b7280' }}>
                    {(d.transfer_id || d.id)?.slice(0, 8)}…
                  </td>
                  <td style={{ padding: '8px 10px' }}>{d.account_id}</td>
                  <td style={{ padding: '8px 10px', fontWeight: 500 }}>{fmtCents(d.amount_cents)}</td>
                  <td style={{ padding: '8px 10px' }}><StatusBadge status={d.status} /></td>
                  <td style={{ padding: '8px 10px' }}>{d.flagged ? <span style={{ color: '#c2410c', fontWeight: 600 }}>⚑</span> : '—'}</td>
                  <td style={{ padding: '8px 10px', color: '#6b7280' }}>{fmtDate(d.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

// ─── Actions Tab ────────────────────────────────────────────────────────────

function ActionsTab() {
  const [batchDate, setBatchDate] = useState(new Date().toISOString().slice(0, 10))
  const [settling, setSettling] = useState(false)
  const [settlementResult, setSettlementResult] = useState(null)
  const [settlementError, setSettlementError] = useState(null)

  const [returnReasons, setReturnReasons] = useState([])
  const [returnId, setReturnId] = useState('')
  const [selectedReasonCode, setSelectedReasonCode] = useState('')
  const [returnNotes, setReturnNotes] = useState('')
  const [returning, setReturning] = useState(false)
  const [returnResult, setReturnResult] = useState(null)
  const [returnError, setReturnError] = useState(null)

  useEffect(() => {
    api.getReturnReasons()
      .then(reasons => {
        const list = Array.isArray(reasons) ? reasons : []
        setReturnReasons(list)
        if (list.length > 0) setSelectedReasonCode(list[0].code)
      })
      .catch(() => {})
  }, [])

  const selectedReason = returnReasons.find(r => r.code === selectedReasonCode)

  async function handleTriggerSettlement(e) {
    e.preventDefault()
    setSettling(true)
    setSettlementResult(null)
    setSettlementError(null)
    try {
      const resp = await api.triggerSettlement(batchDate)
      setSettlementResult(resp.data)
    } catch (err) {
      setSettlementError(err?.error || 'Settlement trigger failed')
    } finally {
      setSettling(false)
    }
  }

  async function handleReturn(e) {
    e.preventDefault()
    if (!returnId.trim() || !selectedReasonCode) return
    setReturning(true)
    setReturnResult(null)
    setReturnError(null)
    try {
      const resp = await api.returnDeposit(returnId.trim(), {
        reason_code: selectedReasonCode,
        notes: returnNotes,
      })
      setReturnResult(resp.data)
    } catch (err) {
      setReturnError(err?.error || 'Return failed — transfer must be in Completed state')
    } finally {
      setReturning(false)
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 24 }}>
      {/* Settlement trigger */}
      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
        <h4 style={{ margin: '0 0 12px', fontSize: 14, fontWeight: 600 }}>Trigger EOD Settlement</h4>
        <form onSubmit={handleTriggerSettlement} style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <input
            type="date"
            value={batchDate}
            onChange={e => setBatchDate(e.target.value)}
            style={{ border: '1px solid #d1d5db', borderRadius: 4, padding: '6px 10px', fontSize: 13 }}
          />
          <button
            type="submit"
            disabled={settling}
            style={{
              backgroundColor: ACCENT,
              color: 'white',
              border: 'none',
              borderRadius: 6,
              padding: '8px 14px',
              fontSize: 13,
              fontWeight: 600,
              cursor: settling ? 'not-allowed' : 'pointer',
              opacity: settling ? 0.6 : 1,
            }}
          >
            {settling ? 'Running…' : 'Trigger Settlement'}
          </button>
        </form>
        {settlementResult && (
          <pre style={{ marginTop: 12, background: '#f8fafc', padding: 10, borderRadius: 4, fontSize: 12, overflow: 'auto' }}>
            {JSON.stringify(settlementResult, null, 2)}
          </pre>
        )}
        {settlementError && <p style={{ color: '#dc2626', fontSize: 13, marginTop: 8 }}>{settlementError}</p>}
      </div>

      {/* Simulate bank return */}
      <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 16 }}>
        <h4 style={{ margin: '0 0 4px', fontSize: 14, fontWeight: 600 }}>↩ Simulate Bank Return</h4>
        <p style={{ margin: '0 0 12px', fontSize: 12, color: '#6b7280' }}>
          Trigger a check return for any deposit in Completed state. This posts reversal ledger entries and notifies the investor.
        </p>

        {returnResult ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ background: '#fef2f2', border: '1px solid #fecaca', borderRadius: 6, padding: 12 }}>
              <p style={{ margin: '0 0 6px', fontWeight: 700, fontSize: 13, color: '#dc2626' }}>✓ Check Returned</p>
              <div style={{ fontSize: 13, display: 'flex', flexDirection: 'column', gap: 3 }}>
                <p style={{ margin: 0 }}><strong>Reason:</strong> {returnResult.return_reason?.label}</p>
                <p style={{ margin: 0 }}><strong>Bank Ref:</strong> <code style={{ fontSize: 12 }}>{returnResult.bank_reference}</code></p>
                <p style={{ margin: 0 }}>
                  Reversal: −${((returnResult.reversal?.original_amount_cents || 0) / 100).toFixed(2)} &nbsp;·&nbsp;
                  Fee: −${((returnResult.reversal?.fee_cents || 0) / 100).toFixed(2)} &nbsp;·&nbsp;
                  <strong>Total: −${((returnResult.reversal?.total_debited_cents || 0) / 100).toFixed(2)}</strong>
                </p>
                <p style={{ margin: 0, color: '#6b7280', fontSize: 12 }}>
                  Investor Notified: {returnResult.investor_notified ? 'Yes' : 'No'}
                </p>
              </div>
            </div>
            <p style={{ margin: 0, fontSize: 12, color: '#6b7280' }}>
              Switch to Investor View to see the return notification and updated ledger.
            </p>
            <button
              onClick={() => { setReturnResult(null); setReturnId(''); setReturnNotes('') }}
              style={{ alignSelf: 'flex-start', fontSize: 12, color: ACCENT, background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}
            >
              Return another deposit
            </button>
          </div>
        ) : (
          <form onSubmit={handleReturn} style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <input
              type="text"
              value={returnId}
              onChange={e => setReturnId(e.target.value)}
              placeholder="Transfer ID (must be in Completed state)"
              style={{ border: '1px solid #d1d5db', borderRadius: 4, padding: '7px 10px', fontSize: 13, fontFamily: 'monospace' }}
            />

            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <select
                value={selectedReasonCode}
                onChange={e => setSelectedReasonCode(e.target.value)}
                style={{ border: '1px solid #d1d5db', borderRadius: 4, padding: '7px 10px', fontSize: 13 }}
              >
                {returnReasons.map(r => (
                  <option key={r.code} value={r.code}>{r.label}</option>
                ))}
              </select>
              {selectedReason && (
                <p style={{ margin: 0, fontSize: 12, color: '#6b7280', fontStyle: 'italic' }}>
                  {selectedReason.description}
                </p>
              )}
            </div>

            <input
              type="text"
              value={returnNotes}
              onChange={e => setReturnNotes(e.target.value)}
              placeholder="Notes (optional)"
              style={{ border: '1px solid #d1d5db', borderRadius: 4, padding: '7px 10px', fontSize: 13 }}
            />

            {/* Impact preview — only shown when a transfer ID is entered */}
            {returnId.trim() && (
              <div style={{ background: '#fef2f2', border: '1px solid #fecaca', borderRadius: 6, padding: 10, fontSize: 12 }}>
                <p style={{ margin: '0 0 4px', fontWeight: 600, color: '#dc2626' }}>This will:</p>
                <ul style={{ margin: 0, paddingLeft: 16, color: '#374151', lineHeight: 1.7 }}>
                  <li>Debit the original deposit amount from the investor account (reversal)</li>
                  <li>Debit $30.00 from the investor account (return fee)</li>
                  <li>Transition deposit from Completed → Returned</li>
                  <li>Notify the investor</li>
                </ul>
              </div>
            )}

            {returnError && <p style={{ margin: 0, color: '#dc2626', fontSize: 13 }}>{returnError}</p>}

            <div>
              <button
                type="submit"
                disabled={returning || !returnId.trim() || !selectedReasonCode}
                style={{
                  backgroundColor: '#dc2626',
                  color: 'white',
                  border: 'none',
                  borderRadius: 6,
                  padding: '8px 16px',
                  fontSize: 13,
                  fontWeight: 600,
                  cursor: (returning || !returnId.trim()) ? 'not-allowed' : 'pointer',
                  opacity: (returning || !returnId.trim()) ? 0.6 : 1,
                }}
              >
                {returning ? 'Processing…' : 'Trigger Return'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  )
}

// ─── Health Tab ─────────────────────────────────────────────────────────────

function HealthTab() {
  const [health, setHealth] = useState(null)
  const [loading, setLoading] = useState(true)

  const fetchHealth = useCallback(async () => {
    try {
      const data = await api.getHealth()
      setHealth(data)
    } catch {
      setHealth({ status: 'error', postgres: 'unknown', redis: 'unknown' })
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchHealth()
    const timer = setInterval(fetchHealth, 15000)
    return () => clearInterval(timer)
  }, [fetchHealth])

  const dot = (ok) => (
    <span style={{ width: 10, height: 10, borderRadius: '50%', backgroundColor: ok ? '#059669' : '#dc2626', display: 'inline-block', marginRight: 6 }} />
  )

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      {loading && <p style={{ color: '#6b7280', fontSize: 13 }}>Checking health…</p>}
      {health && (
        <div style={{ background: '#fff', border: '1px solid #e5e7eb', borderRadius: 8, padding: 20 }}>
          <div style={{ marginBottom: 16 }}>
            <span style={{ fontSize: 16, fontWeight: 700, color: health.status === 'ok' ? '#059669' : '#dc2626' }}>
              {dot(health.status === 'ok')}
              System {health.status === 'ok' ? 'Healthy' : 'Degraded'}
            </span>
          </div>
          {[
            ['PostgreSQL', health.postgres === 'connected'],
            ['Redis', health.redis === 'connected'],
          ].map(([label, ok]) => (
            <div key={label} style={{ display: 'flex', alignItems: 'center', padding: '6px 0', borderBottom: '1px solid #f3f4f6', fontSize: 14 }}>
              {dot(ok)}
              <span style={{ flex: 1 }}>{label}</span>
              <span style={{ color: ok ? '#059669' : '#dc2626', fontWeight: 600 }}>{ok ? 'Connected' : 'Disconnected'}</span>
            </div>
          ))}
          {health.timestamp && (
            <p style={{ fontSize: 12, color: '#9ca3af', marginTop: 12 }}>Last checked: {fmtDate(health.timestamp)}</p>
          )}
          <button
            onClick={fetchHealth}
            style={{ marginTop: 8, fontSize: 12, color: ACCENT, background: 'none', border: 'none', cursor: 'pointer', padding: 0 }}
          >
            Refresh
          </button>
        </div>
      )}
    </div>
  )
}

// ─── AdminView ───────────────────────────────────────────────────────────────

const TABS = [
  { id: 'trace', label: 'Deposit Trace' },
  { id: 'all', label: 'All Deposits' },
  { id: 'actions', label: 'Actions' },
  { id: 'health', label: 'System Health' },
]

export default function AdminView() {
  const [activeTab, setActiveTab] = useState('trace')
  const [traceTransferId, setTraceTransferId] = useState(null)

  function handleTraceTransfer(id) {
    setTraceTransferId(id)
    setActiveTab('trace')
  }

  return (
    <div>
      <div style={{ padding: '12px 0 8px', marginBottom: 8 }}>
        <p style={{ color: '#64748b', fontSize: 13, margin: 0 }}>
          Full system visibility — deposit traces, health checks, and admin actions.
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

      {activeTab === 'trace' && <DepositTraceTab initialTransferId={traceTransferId} />}
      {activeTab === 'all' && <AllDepositsTab onTraceTransfer={handleTraceTransfer} />}
      {activeTab === 'actions' && <ActionsTab />}
      {activeTab === 'health' && <HealthTab />}
    </div>
  )
}
