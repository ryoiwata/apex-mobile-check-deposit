import { useState } from 'react'
import { api } from '../api.js'
import { ACCOUNTS } from '../accounts.js'

const TIME_SCENARIOS = [
  { value: '',              label: 'Now (actual current time)',                        description: '' },
  { value: 'before_cutoff', label: 'Before cutoff — today 3:00 PM CT',               description: 'Deposit timestamp set to 3:00 PM CT — included in today\'s settlement batch' },
  { value: 'after_cutoff',  label: 'After cutoff — today 7:15 PM CT',                description: 'Deposit timestamp set to 7:15 PM CT — rolls to next business day\'s batch' },
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
 * @param {{ accountId: string, onSuccess: (transferId: string) => void, onSwitchAccount?: () => void }} props
 */
export default function DepositForm({ accountId, onSuccess, onSwitchAccount }) {
  const [scenario, setScenario] = useState('CLEAN_PASS')
  const [createdAtOverride, setCreatedAtOverride] = useState('')
  const [amountDollars, setAmountDollars] = useState('100.00')
  const [ocrAmountDollars, setOcrAmountDollars] = useState('')
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
  // Account field inline validation (backend errors only — selection is global)
  const [accountError, setAccountError] = useState(null)
  // General violations that don't map to a specific field
  const [generalViolations, setGeneralViolations] = useState([])

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

  const selectedAccount = ACCOUNTS.find(a => a.id === accountId)
  const isAccountIneligible = selectedAccount?.status === 'suspended' || selectedAccount?.status === 'closed'
  const accountWarning = isAccountIneligible ? selectedAccount?.status : null
  const isAmountValid = !validateAmount(amountDollars)
  const canSubmit = isAmountValid && !isAccountIneligible && !loading

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
    setAccountError(null)
    setGeneralViolations([])

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
    if (scenario === 'AMOUNT_MISMATCH' && ocrAmountDollars) {
      const ocrCents = Math.round(parseFloat(ocrAmountDollars) * 100)
      if (!isNaN(ocrCents) && ocrCents > 0) {
        formData.append('simulated_ocr_amount_cents', String(ocrCents))
      }
    }
    if (createdAtOverride) {
      formData.append('created_at_override', createdAtOverride)
    }

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
      // 401 → session expired or missing token
      if (err?.action === 're_authenticate' || err?.action === 'authenticate') {
        setNeedsReauth(true)
        return
      }
      // 422 account_not_found → inline error on account dropdown
      if (err?.error === 'account_not_found') {
        setAccountError(err.message || 'Account does not exist. Please select a valid account.')
        return
      }
      // 422 collect-all violations — map each to its field
      if (err?.violations) {
        setViolations(err.violations)
        const general = []
        err.violations.forEach(v => {
          if (v.code === 'over_limit') {
            setAmountTouched(true)
            setAmountError(v.message || 'Deposits are limited to $5,000.00 per check.')
          } else if (v.code === 'account_ineligible') {
            setAccountError(v.message)
          } else {
            general.push(v)
          }
        })
        setGeneralViolations(general)
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
          onChange={e => { setScenario(e.target.value); setOcrAmountDollars('') }}
          className="w-full border border-amber-300 rounded px-3 py-2 text-sm bg-white focus:outline-none focus:ring-1 focus:ring-amber-500"
        >
          {SCENARIOS.map(s => (
            <option key={s.code} value={s.code}>{s.label} — {s.description}</option>
          ))}
        </select>
        {scenario === 'AMOUNT_MISMATCH' && (
          <div className="mt-3 pt-3 border-t border-amber-200">
            <label className="block text-xs font-semibold text-amber-800 mb-1">
              Simulated OCR Amount ($) <span className="font-normal text-amber-700">— what the stub will "read" from the check</span>
            </label>
            <input
              type="number"
              step="0.01"
              min="0.01"
              placeholder={`e.g. ${amountDollars ? (parseFloat(amountDollars) * 0.8).toFixed(2) : '80.00'} (leave blank for auto 80% of entered)`}
              value={ocrAmountDollars}
              onChange={e => setOcrAmountDollars(e.target.value)}
              className="w-full border border-amber-300 rounded px-3 py-2 text-sm bg-white focus:outline-none focus:ring-1 focus:ring-amber-500"
            />
            {ocrAmountDollars && amountDollars && parseFloat(ocrAmountDollars) === parseFloat(amountDollars) && (
              <p className="mt-1 text-xs text-amber-700">⚠ OCR amount matches entered amount — no mismatch will be created. Enter a different value.</p>
            )}
            <p className="mt-1 text-xs text-amber-600">
              The operator will see this as the OCR-recognized amount vs. {amountDollars ? `$${parseFloat(amountDollars).toFixed(2)}` : 'the entered amount'} investor-entered.
            </p>
          </div>
        )}
      </div>

      {/* Deposit Time Override — demo control for EOD cutoff testing */}
      <div className="mb-5 p-4 bg-orange-50 border border-orange-200 rounded-lg">
        <p className="text-xs font-semibold text-orange-800 uppercase tracking-wide mb-1">
          Deposit Time — Test Scenario
        </p>
        <p className="text-xs text-orange-700 mb-3">
          Override deposit timestamp to test EOD cutoff behavior. Cutoff is 6:30 PM CT.
        </p>
        <select
          value={createdAtOverride}
          onChange={e => setCreatedAtOverride(e.target.value)}
          className="w-full border border-orange-300 rounded px-3 py-2 text-sm bg-white focus:outline-none focus:ring-1 focus:ring-orange-500"
        >
          {TIME_SCENARIOS.map(s => (
            <option key={s.value} value={s.value}>{s.label}</option>
          ))}
        </select>
        {createdAtOverride && TIME_SCENARIOS.find(s => s.value === createdAtOverride)?.description && (
          <p className="mt-2 text-xs text-orange-600">
            {TIME_SCENARIOS.find(s => s.value === createdAtOverride).description}
          </p>
        )}
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <div className="flex items-center justify-between mb-1">
            <label className="text-sm font-medium text-gray-700">Depositing to</label>
            {onSwitchAccount && (
              <button
                type="button"
                onClick={onSwitchAccount}
                className="text-xs text-blue-600 hover:underline"
              >
                Switch account ↑
              </button>
            )}
          </div>
          <div className={`px-3 py-2 rounded border text-sm ${accountWarning ? 'border-amber-300 bg-amber-50' : 'border-gray-200 bg-gray-50'}`}>
            <span className="font-mono font-medium text-gray-800">{accountId}</span>
            {selectedAccount && (
              <span className="ml-2 text-gray-500">— {selectedAccount.label}</span>
            )}
          </div>
          {accountWarning && (
            <div className="mt-1.5 flex items-start gap-2 px-3 py-2.5 bg-amber-50 border border-amber-200 rounded text-xs text-amber-800">
              <span className="shrink-0 text-sm">⚠️</span>
              <div>
                <strong className="block mb-0.5">
                  {accountWarning === 'closed' ? 'Account Closed' : 'Account Suspended'}
                </strong>
                {accountWarning === 'closed'
                  ? 'This account is permanently closed and cannot receive deposits. Use the account switcher above to select a different account.'
                  : 'This account is currently suspended and cannot receive deposits. Use the account switcher above to select a different account.'}
              </div>
            </div>
          )}
          {accountError && (
            <p className="mt-1.5 text-xs text-red-600 flex items-center gap-1">
              <span>⚠</span> {accountError}
            </p>
          )}
        </div>

        <div
          style={{
            opacity: isAccountIneligible ? 0.45 : 1,
            pointerEvents: isAccountIneligible ? 'none' : 'auto',
            transition: 'opacity 0.2s ease',
          }}
          className="space-y-4"
        >
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1">Amount ($)</label>
            <p className="text-xs text-gray-400 mb-1.5">Single deposits are limited to $5,000.00 per check.</p>
            <input
              type="number"
              step="0.01"
              value={amountDollars}
              onChange={handleAmountChange}
              onBlur={handleAmountBlur}
              disabled={isAccountIneligible}
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
              disabled={isAccountIneligible}
              onClick={() => setCameraMode(false)}
              className={`px-3 py-1.5 rounded text-sm font-medium ${!cameraMode ? 'bg-blue-600 text-white' : 'bg-gray-200 text-gray-600'}`}
            >
              📁 Upload File
            </button>
            <button
              type="button"
              disabled={isAccountIneligible}
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
              disabled={isAccountIneligible}
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
              disabled={isAccountIneligible}
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
        </div>
      </form>

      {error && (
        <div className="mt-4 p-3 bg-red-50 border border-red-200 rounded text-sm text-red-700">
          {error}
        </div>
      )}

      {generalViolations.length > 0 && (
        <div className="mt-4 p-4 bg-red-50 border border-red-200 rounded space-y-2">
          <p className="text-sm font-semibold text-red-800">Deposit could not be processed:</p>
          <ul className="space-y-1">
            {generalViolations.map((v, i) => (
              <li key={i} className="flex items-start gap-2 text-sm text-red-700">
                <span className="mt-0.5 text-red-400">⚠</span>
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
