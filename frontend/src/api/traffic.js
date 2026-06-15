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
