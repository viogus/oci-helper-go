import { get, post } from './index.js'

export function batchCreate(data) {
  return post('/instances/batch-create', data)
}

export function listCreateTasks(params = {}) {
  return get('/create-tasks', params)
}

export function stopTasks(taskIds) {
  return post('/create-tasks', { action: 'stop', task_ids: taskIds })
}

export function pauseTasks(taskIds) {
  return post('/create-tasks', { action: 'pause', task_ids: taskIds })
}

export function resumeTasks(taskIds) {
  return post('/create-tasks', { action: 'resume', task_ids: taskIds })
}

export function deleteTasks(taskIds) {
  return post('/create-tasks', { action: 'delete', task_ids: taskIds })
}

export function updateTask(taskId, payload) {
  return post('/create-tasks', { action: 'update', task_id: taskId, payload: JSON.stringify(payload) })
}
