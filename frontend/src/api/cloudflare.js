import { get, post, del } from './index.js'
import api from './index.js'

// cfgId (optional) selects a named CF config from /cloudflare/cfgs; when
// omitted the server falls back to the global cloudflare_token config.

export function listZones(cfgId) {
  return get('/cloudflare/zones', cfgId ? { cfg_id: cfgId } : {})
}

export function listRecords(zoneId, cfgId) {
  return get(`/cloudflare/${zoneId}/records`, cfgId ? { cfg_id: cfgId } : {})
}

export function createRecord(zoneId, data, cfgId) {
  return post(`/cloudflare/${zoneId}/records`, data, cfgId ? { params: { cfg_id: cfgId } } : {})
}

export function updateRecord(zoneId, recordId, data, cfgId) {
  return api.put(`/cloudflare/${zoneId}/records/${recordId}`, data, cfgId ? { params: { cfg_id: cfgId } } : {}).then(r => r.data)
}

export function deleteRecord(zoneId, recordId, cfgId) {
  return del(`/cloudflare/${zoneId}/records/${recordId}`, cfgId ? { params: { cfg_id: cfgId } } : {})
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
