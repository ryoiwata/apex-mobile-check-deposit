import { useState } from 'react'
import { api } from '../api.js'

const ACCOUNTS = [
  { id: 'ACC-SOFI-1006', label: 'ACC-SOFI-1006 — Clean Pass (Happy Path)' },
  { id: 'ACC-SOFI-1001', label: 'ACC-SOFI-1001 — IQA Blur (Rejected)' },
  { id: 'ACC-SOFI-1002', label: 'ACC-SOFI-1002 — IQA Glare (Rejected)' },
  { id: 'ACC-SOFI-1003', label: 'ACC-SOFI-1003 — MICR Failure (Operator Review)' },
  { id: 'ACC-SOFI-1004', label: 'ACC-SOFI-1004 — Duplicate (Rejected)' },
  { id: 'ACC-SOFI-1005', label: 'ACC-SOFI-1005 — Amount Mismatch (Operator Review)' },
  { id: 'ACC-SOFI-0000', label: 'ACC-SOFI-0000 — Basic Pass' },
  { id: 'ACC-RETIRE-001', label: 'ACC-RETIRE-001 — Retirement (Contribution Type)' },
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
 * @param {{ onSuccess: (transferId: string) => void }} props
 */
export default function DepositForm({ onSuccess }) {
  const [accountId, setAccountId] = useState('ACC-SOFI-1006')
  const [amountDollars, setAmountDollars] = useState('100.00')
  const [frontFile, setFrontFile] = useState(null)
  const [backFile, setBackFile] = useState(null)
  const [cameraMode, setCameraMode] = useState(false)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState(null)
  const [error, setError] = useState(null)

  async function handleSubmit(e) {
    e.preventDefault()
    setError(null)
    setResult(null)

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
    formData.append('front_image', front)
    formData.append('back_image', back)

    setLoading(true)
    try {
      const resp = await api.submitDeposit(formData)
      setResult(resp.data)
    } catch (err) {
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

  return (
    <div className="max-w-lg">
      <h2 className="text-lg font-semibold text-gray-800 mb-4">Submit Check Deposit</h2>

      <div className="flex items-center gap-2 mb-4">
        <button
          type="button"
          onClick={() => setCameraMode(false)}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded text-sm font-medium border transition-colors ${!cameraMode ? 'bg-blue-700 text-white border-blue-700' : 'bg-white text-gray-600 border-gray-300 hover:bg-gray-50'}`}
        >
          📁 Upload File
        </button>
        <button
          type="button"
          onClick={() => setCameraMode(true)}
          className={`flex items-center gap-1.5 px-3 py-1.5 rounded text-sm font-medium border transition-colors ${cameraMode ? 'bg-blue-700 text-white border-blue-700' : 'bg-white text-gray-600 border-gray-300 hover:bg-gray-50'}`}
        >
          📷 Take Photo
        </button>
        <span className="text-xs text-gray-400">
          {cameraMode ? 'Opens your phone\'s rear camera directly' : 'Select an image file from your device'}
        </span>
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
          <input
            type="number"
            step="0.01"
            min="0.01"
            max="5000.00"
            value={amountDollars}
            onChange={e => setAmountDollars(e.target.value)}
            className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

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
          disabled={loading}
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
