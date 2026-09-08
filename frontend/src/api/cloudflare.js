import { get, post, del } from './index.js'
import api from './index.js'

export function listZones() {
  return get('/cloudflare/zones')
}

export function listRecords(zoneId) {
  return get(`/cloudflare/${zoneId}/records`)
}

export function createRecord(zoneId, data) {
  return post(`/cloudflare/${zoneId}/records`, data)
}

export function updateRecord(zoneId, recordId, data) {
  return api.put(`/cloudflare/${zoneId}/records/${recordId}`, data).then(r => r.data)
}

export function deleteRecord(zoneId, recordId) {
  return del(`/cloudflare/${zoneId}/records/${recordId}`)
}

// ── Instance ↔ DNS bindings ─────────────────────────────────────────────
// A binding ties a specific instance's public IP to a named DNS record in a
// Cloudflare zone, so auto-sync / change-IP update exactly that record.

export function listBindings(instanceId) {
  return get('/cloudflare/bindings', instanceId ? { instance_id: instanceId } : {})
}

export function createBinding(data) {
  return post('/cloudflare/bindings', data)
}

export function updateBinding(id, data) {
  return api.put(`/cloudflare/bindings/${id}`, data).then(r => r.data)
}

export function deleteBinding(id) {
  return del(`/cloudflare/bindings/${id}`)
}

export function listCfConfigs() {
  return get('/cloudflare/cfgs')
}
