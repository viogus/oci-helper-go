import { post } from './index.js'

export function listSecurityRules(data) {
  return post('/security-rules', { ...data, action: 'page' })
}

export function addIngressRule(data) {
  return post('/security-rules', { ...data, action: 'addIngress' })
}

export function addEgressRule(data) {
  return post('/security-rules', { ...data, action: 'addEgress' })
}

export function removeSecurityRules(data) {
  return post('/security-rules', { ...data, action: 'remove' })
}

export function releaseAllPorts(data) {
  return post('/security-rules', { ...data, action: 'release' })
}
