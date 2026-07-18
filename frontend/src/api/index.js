import axios from 'axios'
import { getRouter } from '../router/instance.js'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' }
})

// Retry configuration
const MAX_RETRIES = 3
const RETRY_DELAY = 1000 // 1 second base

// response interceptor: unwrap data, handle 401, retry on failure
api.interceptors.response.use(
  res => res,
  async err => {
    const config = err.config

    // Retry GET requests on network errors or 5xx (up to 3 times) with exponential backoff
    if (config.method === 'get' && (!err.response || err.response.status >= 500)) {
      config._retryCount = config._retryCount || 0
      if (config._retryCount < MAX_RETRIES) {
        config._retryCount++
        const delay = RETRY_DELAY * Math.pow(2, config._retryCount - 1)
        await new Promise(resolve => setTimeout(resolve, delay))
        return api(config)
      }
    }

    // Handle 401 redirect
    if (err.response?.status === 401) {
      const router = getRouter()
      if (router && router.currentRoute?.value?.path !== '/login') {
        router.push('/login')
      }
    }
    return Promise.reject(err)
  }
)

// helper: GET JSON
export async function get(path, params = {}) {
  const res = await api.get(path, { params })
  return res.data
}

// helper: POST JSON
export async function post(path, data = {}, config = {}) {
  const res = await api.post(path, data, config)
  return res.data
}

// helper: POST FormData
export async function upload(path, formData) {
  const res = await api.post(path, formData, {
    headers: { 'Content-Type': 'multipart/form-data' }
  })
  return res.data
}

// helper: PUT
export async function put(path, data = {}) {
  const res = await api.put(path, data)
  return res.data
}

// helper: DELETE
export async function del(path, config = {}) {
  const res = await api.delete(path, config)
  return res.data
}

export default api
