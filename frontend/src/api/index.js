import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' }
})

// response interceptor: unwrap data, handle 401
api.interceptors.response.use(
  res => res,
  err => {
    if (err.response?.status === 401) {
      const router = (window.__router)
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

// helper: DELETE
export async function del(path) {
  const res = await api.delete(path)
  return res.data
}

export default api
