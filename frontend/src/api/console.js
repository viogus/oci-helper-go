import { get, post } from './index.js'

export function listSSHKeys(tenantId) {
  return get('/ssh/keys', { tenant_id: tenantId })
}

export function startVNC(data) {
  return post('/instances/vnc', data)
}

export function stopVNC(data) {
  return post('/instances/vnc/stop', data)
}

export function waitVNC(params) {
  return get('/instances/vnc/wait', params)
}
