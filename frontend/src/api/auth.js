import { get, post } from './index.js'

export function login(username, password, totp) {
  return post('/login', {}, {
    headers: {
      'Authorization': 'Basic ' + btoa(username + ':' + password),
      ...(totp ? { 'X-TOTP': totp } : {})
    }
  })
}

export function logout() {
  return post('/logout')
}

export function getConfig() {
  return get('/config')
}

export function saveConfig(key, value) {
  return post('/config', { key, value })
}

export function mfaSetup() {
  return get('/mfa/setup')
}

export function mfaVerify(code) {
  return post('/mfa/verify', { code })
}

export function mfaDisable(code) {
  return post('/mfa/disable', { code })
}
