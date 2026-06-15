import { get, post, del, upload } from './index.js'

export function listTenants(params = {}) {
  return get('/tenants', params)
}

export function createTenant(data) {
  return post('/tenants', data)
}

export function getTenant(id) {
  return get('/tenants/' + id)
}

export function deleteTenant(id) {
  return del('/tenants/' + id)
}

export function syncTenant(id) {
  return post('/sync/' + id)
}

export function listKeys() {
  return get('/keys')
}

export function uploadKeys(formData) {
  return upload('/keys', formData)
}
