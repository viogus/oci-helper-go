import { get, post } from './index.js'

export function getTrafficData(data) {
  return post('/traffic', data)
}

export function getInstances(tenantId) {
  return get('/instances', { tenant_id: tenantId, size: 100 })
}

export function getLimits(data) {
  return post('/limits', data)
}

// Account-wide traffic aggregation fans out across every subscribed region, so
// it needs a longer budget than the default 30s client timeout.
export function getAccountStats(data) {
  return post('/traffic/accountStats', data, { timeout: 90000 })
}
