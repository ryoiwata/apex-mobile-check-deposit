import { useState } from 'react'
import { api } from '../api.js'

const ACCOUNTS = [
  { id: 'ACC-SOFI-1006', label: 'ACC-SOFI-1006 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-1001', label: 'ACC-SOFI-1001 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-1002', label: 'ACC-SOFI-1002 — SoFi Joint Brokerage' },
  { id: 'ACC-SOFI-1003', label: 'ACC-SOFI-1003 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-1004', label: 'ACC-SOFI-1004 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-1005', label: 'ACC-SOFI-1005 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-0000', label: 'ACC-SOFI-0000 — SoFi Demo Account' },
  { id: 'ACC-RETIRE-001', label: 'ACC-RETIRE-001 — SoFi Traditional IRA' },
]

const SCENARIOS = [
  { code: 'CLEAN_PASS',         label: 'Clean Pass',           description: 'All checks pass, MICR data extracted (Happy Path)' },
  { code: 'IQA_FAIL_BLUR',      label: 'IQA Fail — Blur',      description: 'Image too blurry, prompt retake' },
  { code: 'IQA_FAIL_GLARE',     label: 'IQA Fail — Glare',     description: 'Glare detected, prompt retake' },
  { code: 'MICR_READ_FAILURE',  label: 'MICR Read Failure',    description: 'Cannot read MICR line, flags for operator review' },
  { code: 'DUPLICATE_DETECTED', label: 'Duplicate Detected',   description: 'Check previously deposited, reject' },
  { code: 'AMOUNT_MISMATCH',    label: 'Amount Mismatch',      description: 'OCR amount differs from entered amount, flags for review' },
  { code: 'IQA_PASS',           label: 'IQA Pass (basic)',     description: 'Image quality acceptable, proceed normally' },
]

const STATUS_STYLES = {
  funds_posted: 'bg-green-100 text-green-800',
  completed: 'bg-green-100 text-green-800',
  rejected: 'bg-red-100 text-red-800',
  analyzing: 'bg-yellow-100 text-yellow-800',
  approved: 'bg-blue-100 text-blue-800',
  returned: 'bg-orange-100 text-orange-800',
}

const STATUS_MESSAGES = {
  funds_posted: 'Deposit approved — funds provisionally credited to your account.',
  completed: 'Deposit completed and settled.',
  rejected: 'Deposit was rejected. Please check the error details above.',
  analyzing: 'Deposit flagged for manual operator review. Check back soon.',
  approved: 'Deposit approved, posting funds.',
  returned: 'Deposit was returned.',
}

/**
 * @param {{ onSuccess: (transferId: string) => void, initialAccountId?: string }} props
 */
export default function DepositForm({ onSuccess, initialAccountId }) {
  const [accountId, setAccountId] = useState(initialAccountId || 'ACC-SOFI-1006')
  const [scenario, setScenario] = useState('CLEAN_PASS')
  const [amountDollars, setAmountDollars] = useState('100.00')
  const [frontFile, setFrontFile] = useState(null)
  const [backFile, setBackFile] = useState(null)
  const [cameraMode, setCameraMode] = useState(false)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState(null)
  const [error, setError] = useState(null)
  // IQA-specific retake state — preserves account/amount across retakes
  const [iqaError, setIqaError] = useState(null) // { guidance, code }
  // Business rule violations from collect-all evaluation
  const [violations, setViolations] = useState(null)
  // Re-auth prompt
  const [needsReauth, setNeedsReauth] = useState(false)
  // Amount field inline validation
  const [amountError, setAmountError] = useState(null)
  const [amountTouched, setAmountTouched] = useState(false)

  const DEPOSIT_LIMIT = 5000

  function validateAmount(value) {
    const num = parseFloat(value)
    if (!value || isNaN(num) || num <= 0) return 'Please enter a valid deposit amount.'
    if (num > DEPOSIT_LIMIT) return `Deposits are limited to $${DEPOSIT_LIMIT.toLocaleString('en-US', { minimumFractionDigits: 2 })} per check.`
    return null
  }

  function handleAmountChange(e) {
    setAmountDollars(e.target.value)
    if (amountTouched) setAmountError(validateAmount(e.target.value))
  }

  function handleAmountBlur() {
    setAmountTouched(true)
    setAmountError(validateAmount(amountDollars))
  }

  const isAmountValid = !validateAmount(amountDollars)
  const canSubmit = isAmountValid && !loading

  function resetImageState() {
    setFrontFile(null)
    setBackFile(null)
  }

  function handleRetake() {
    setIqaError(null)
    setError(null)
    resetImageState()
    // amount and accountId are preserved intentionally
  }

  async function handleSubmit(e) {
    e.preventDefault()
    setError(null)
    setResult(null)
    setIqaError(null)
    setViolations(null)
    setNeedsReauth(false)
    setAmountError(null)

    const amountCents = Math.round(parseFloat(amountDollars) * 100)
    if (isNaN(amountCents) || amountCents <= 0) {
      setError('Amount must be a positive number.')
      return
    }
    if (amountCents > 500000) {
      setError('Amount cannot exceed $5,000.00.')
      return
    }

    // If no images provided, use placeholder bytes so the server doesn't reject
    const front = frontFile || createPlaceholderFile('front.png')
    const back = backFile || createPlaceholderFile('back.png')

    const formData = new FormData()
    formData.append('account_id', accountId)
    formData.append('amount_cents', String(amountCents))
    formData.append('vendor_scenario', scenario)
    formData.append('front_image', front)
    formData.append('back_image', back)

    setLoading(true)
    try {
      const resp = await api.submitDeposit(formData)
      const transfer = resp.data
      // IQA failure — show retake guidance
      if (transfer?.status === 'rejected' && transfer?.retake_guidance) {
        setIqaError({
          guidance: transfer.retake_guidance,
          code: transfer.vendor_error_code || 'IQA_FAIL',
        })
        return
      }
      setResult(transfer)
    } catch (err) {
      // 401 → session expired
      if (err?.error === 'session_expired' || (typeof err === 'object' && err?.action === 're_authenticate')) {
        setNeedsReauth(true)
        return
      }
      // 422 collect-all violations — map field-level codes to inline errors
      if (err?.violations) {
        setViolations(err.violations)
        err.violations.forEach(v => {
          if (v.code === 'over_limit') {
            setAmountTouched(true)
            setAmountError(v.message || `Deposits are limited to $5,000.00 per check.`)
          }
        })
        return
      }
      setError(err?.error || 'Submission failed. Is the backend running?')
    } finally {
      setLoading(false)
    }
  }

  function createPlaceholderFile(name) {
    // 1x1 white PNG as placeholder
    const bytes = atob(
      'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwADhQGAWjR9awAAAABJRU5ErkJggg=='
    )
    const arr = new Uint8Array(bytes.length)
    for (let i = 0; i < bytes.length; i++) arr[i] = bytes.charCodeAt(i)
    return new File([arr], name, { type: 'image/png' })
  }

  const status = result?.status

  // Re-auth prompt
  if (needsReauth) {
    return (
      <div className="max-w-lg">
        <div className="p-4 bg-yellow-50 border border-yellow-200 rounded-lg space-y-3">
          <p className="text-sm font-semibold text-yellow-800">Session Expired</p>
          <p className="text-sm text-yellow-700">Your session has expired. Please re-authenticate to continue.</p>
          <button
            onClick={() => setNeedsReauth(false)}
            className="px-4 py-2 bg-yellow-600 text-white text-sm font-medium rounded hover:bg-yellow-700"
          >
            Re-authenticate
          </button>
        </div>
      </div>
    )
  }

  // IQA failure retake prompt
  if (iqaError) {
    return (
      <div className="max-w-lg space-y-4">
        <div className="p-4 bg-orange-50 border border-orange-200 rounded-lg space-y-3">
          <div className="flex items-start gap-2">
            <span className="text-orange-500 text-lg">⚠️</span>
            <div>
              <p className="text-sm font-semibold text-orange-800">Image Quality Issue</p>
              <p className="text-sm text-orange-700 mt-1">{iqaError.guidance}</p>
            </div>
          </div>
          <div className="flex gap-2 pt-1">
            <button
              onClick={handleRetake}
              className="px-4 py-2 bg-blue-700 text-white text-sm font-medium rounded hover:bg-blue-800"
            >
              Retake Photo
            </button>
            <button
              onClick={() => { setIqaError(null); setResult(null) }}
              className="px-4 py-2 bg-gray-100 text-gray-700 text-sm font-medium rounded hover:bg-gray-200"
            >
              Cancel
            </button>
          </div>
          <p className="text-xs text-orange-600">
            Your account (<strong>{accountId}</strong>) and amount (<strong>${amountDollars}</strong>) will be preserved.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="max-w-lg">
      <h2 className="text-lg font-semibold text-gray-800 mb-4">Submit Check Deposit</h2>

      {/* Test Configuration Panel — Vendor Service stub control, not investor-facing */}
      <div className="mb-5 p-4 bg-amber-50 border border-amber-200 rounded-lg">
        <p className="text-xs font-semibold text-amber-800 uppercase tracking-wide mb-1">
          Vendor Service Stub — Test Scenario
        </p>
        <p className="text-xs text-amber-700 mb-3">
          Controls how the stub responds. Independent of the investor account selected below.
        </p>
        <select
          value={scenario}
          onChange={e => setScenario(e.target.value)}
          className="w-full border border-amber-300 rounded px-3 py-2 text-sm bg-white focus:outline-none focus:ring-1 focus:ring-amber-500"
        >
          {SCENARIOS.map(s => (
            <option key={s.code} value={s.code}>{s.label} — {s.description}</option>
          ))}
        </select>
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Account</label>
          <select
            value={accountId}
            onChange={e => setAccountId(e.target.value)}
            className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            {ACCOUNTS.map(a => (
              <option key={a.id} value={a.id}>{a.label}</option>
            ))}
          </select>
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">Amount ($)</label>
          <p className="text-xs text-gray-400 mb-1.5">Single deposits are limited to $5,000.00 per check.</p>
          <input
            type="number"
            step="0.01"
            value={amountDollars}
            onChange={handleAmountChange}
            onBlur={handleAmountBlur}
            className={`w-full border rounded px-3 py-2 text-sm focus:outline-none focus:ring-1 ${amountError ? 'border-red-400 focus:ring-red-400' : 'border-gray-300 focus:ring-blue-500'}`}
          />
          {amountError && (
            <p className="mt-1.5 text-xs text-red-600 flex items-center gap-1" style={{ transition: 'opacity 0.2s' }}>
              <span>⚠</span> {amountError}
            </p>
          )}
          {!amountError && amountTouched && isAmountValid && (
            <p className="mt-1.5 text-xs text-green-600 flex items-center gap-1">
              <span>✓</span> Amount within deposit limits
            </p>
          )}
        </div>

        <div className="flex items-center gap-3 mb-4 p-3 bg-gray-50 rounded-lg">
          <span className="text-sm font-medium text-gray-700">Image source:</span>
          <button
            type="button"
            onClick={() => setCameraMode(false)}
            className={`px-3 py-1.5 rounded text-sm font-medium ${!cameraMode ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-600'}`}
          >
            📁 Upload File
          </button>
          <button
            type="button"
            onClick={() => setCameraMode(true)}
            className={`px-3 py-1.5 rounded text-sm font-medium ${cameraMode ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-600'}`}
          >
            📷 Take Photo
          </button>
        </div>
        <p className="text-xs text-gray-500 -mt-3">
          {cameraMode ? "Opens your phone's rear camera directly" : 'Select an image file from your device'}
        </p>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            Front of Check <span className="text-gray-400 font-normal">(optional — placeholder used if omitted)</span>
          </label>
          <input
            type="file"
            accept="image/*"
            {...(cameraMode ? { capture: 'environment' } : {})}
            onChange={e => setFrontFile(e.target.files[0] || null)}
            className="w-full text-sm text-gray-500 file:mr-3 file:py-1 file:px-3 file:rounded file:border-0 file:text-sm file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100"
          />
        </div>

        <div>
          <label className="block text-sm font-medium text-gray-700 mb-1">
            Back of Check <span className="text-gray-400 font-normal">(optional)</span>
          </label>
          <input
            type="file"
            accept="image/*"
            {...(cameraMode ? { capture: 'environment' } : {})}
            onChange={e => setBackFile(e.target.files[0] || null)}
            className="w-full text-sm text-gray-500 file:mr-3 file:py-1 file:px-3 file:rounded file:border-0 file:text-sm file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100"
          />
        </div>

        <button
          type="submit"
          disabled={!canSubmit}
          className="w-full bg-blue-700 text-white py-2 px-4 rounded text-sm font-medium hover:bg-blue-800 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {loading ? 'Submitting…' : 'Submit Deposit'}
        </button>
      </form>

      {error && (
        <div className="mt-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
          {error}
        </div>
      )}

      {violations && (
        <div className="mt-4 p-4 bg-red-50 border border-red-200 rounded space-y-2">
          <p className="text-sm font-semibold text-red-800">Deposit could not be processed — please fix all issues:</p>
          <ul className="space-y-1">
            {violations.map((v, i) => (
              <li key={i} className="flex items-start gap-2 text-sm text-red-700">
                <span className="mt-0.5 text-red-400">•</span>
                <span><strong>{v.rule}:</strong> {v.message}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {result && (
        <div className="mt-4 p-4 bg-white border border-gray-200 rounded shadow-sm space-y-2">
          <div className="flex items-center gap-2">
            <span className={`px-2 py-0.5 rounded text-xs font-semibold uppercase ${STATUS_STYLES[status] || 'bg-gray-100 text-gray-700'}`}>
              {status?.replace('_', ' ')}
            </span>
            <span className="text-xs text-gray-500">{result.transfer_id}</span>
          </div>
          <p className="text-sm text-gray-700">{STATUS_MESSAGES[status] || `Status: ${status}`}</p>
          {result.flag_reason && (
            <p className="text-sm text-yellow-700">Flag reason: <strong>{result.flag_reason}</strong></p>
          )}
          <button
            onClick={() => onSuccess(result.transfer_id)}
            className="text-sm text-blue-600 hover:underline"
          >
            View transfer details →
          </button>
        </div>
      )}
    </div>
  )
}
