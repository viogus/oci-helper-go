import { get, post } from './index.js'

export function listInstances(params = {}) {
  return get('/instances', params)
}

export function instanceAction(instanceId, action) {
  return post('/instances/' + instanceId, { action })
}

export function batchStart(payload) {
  return post('/instances/batch-start', payload)
}

export function changeShape(data) {
  return post('/instances/change-shape', data)
}

export function changeBootVolume(data) {
  return post('/instances/change-boot-volume', data)
}

export function attachIPv6(data) {
  return post('/instances/attach-ipv6', data)
}

export function updateInstanceName(data) {
  return post('/instances/update-name', data)
}
