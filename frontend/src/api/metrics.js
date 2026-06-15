import { get } from './index.js'

export function getMetrics(params) {
  return get('/metrics', params)
}
