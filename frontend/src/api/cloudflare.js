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
