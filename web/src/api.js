const INVESTOR_TOKEN = 'tok_investor_test'
const OPERATOR_ID = 'OP-001'

function investorHeaders(extra = {}) {
  return {
    'Authorization': `Bearer ${INVESTOR_TOKEN}`,
    ...extra,
  }
}

function operatorHeaders(extra = {}) {
  return {
    'X-Operator-ID': OPERATOR_ID,
    ...extra,
  }
}

function handleResponse(resp) {
  if (!resp.ok) return resp.json().then(err => Promise.reject(err))
  return resp.json()
}

export const api = {
  // Investor endpoints
  submitDeposit: (formData) =>
    fetch('/api/v1/deposits', {
      method: 'POST',
      headers: investorHeaders(),
      body: formData,
    }).then(handleResponse),

  getDeposit: (id) =>
    fetch(`/api/v1/deposits/${id}`, {
      headers: investorHeaders(),
    }).then(handleResponse),

  listDeposits: (params = {}) => {
    const qs = new URLSearchParams(params).toString()
    return fetch(`/api/v1/deposits${qs ? '?' + qs : ''}`, {
      headers: investorHeaders(),
    }).then(handleResponse)
  },

  getLedger: (accountId) =>
    fetch(`/api/v1/ledger/${accountId}`, {
      headers: investorHeaders(),
    }).then(handleResponse),

  // Operator endpoints
  getQueue: () =>
    fetch('/api/v1/operator/queue', {
      headers: operatorHeaders(),
    }).then(handleResponse),

  approveDeposit: (id, body) =>
    fetch(`/api/v1/operator/deposits/${id}/approve`, {
      method: 'POST',
      headers: operatorHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    }).then(handleResponse),

  rejectDeposit: (id, body) =>
    fetch(`/api/v1/operator/deposits/${id}/reject`, {
      method: 'POST',
      headers: operatorHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    }).then(handleResponse),

  triggerSettlement: (batchDate) =>
    fetch('/api/v1/operator/settlement/trigger', {
      method: 'POST',
      headers: operatorHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ batch_date: batchDate }),
    }).then(handleResponse),

  overrideContributionType: (id, contributionType) =>
    fetch(`/api/v1/operator/deposits/${id}/contribution-type`, {
      method: 'PATCH',
      headers: operatorHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ contribution_type: contributionType }),
    }).then(handleResponse),

  returnDeposit: (id, body) =>
    fetch(`/api/v1/operator/deposits/${id}/return`, {
      method: 'POST',
      headers: operatorHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify(body),
    }).then(handleResponse),

  getAuditLog: (transferId) => {
    const qs = transferId ? `?transfer_id=${transferId}` : ''
    return fetch(`/api/v1/operator/audit${qs}`, {
      headers: operatorHeaders(),
    }).then(handleResponse)
  },
}
