<template>
  <div class="logs-page">
    <el-card shadow="never">
      <template #header>
        <div class="card-header">
          <span>Server Logs</span>
          <div class="header-actions">
            <el-select
              v-model="tailLines"
              style="width: 120px; margin-right: 12px;"
              @change="refreshLogs"
            >
              <el-option label="50" :value="50" />
              <el-option label="100" :value="100" />
              <el-option label="200" :value="200" />
              <el-option label="500" :value="500" />
            </el-select>
            <el-button type="primary" :loading="loading" @click="refreshLogs">
              Refresh
            </el-button>
            <span v-if="liveActive" class="live-dot" style="color:#67C23A;margin-right:8px">&#x25CF; LIVE</span>
            <el-switch v-model="liveActive" @change="toggleLive" :active-text="$t('logs.live')" size="small" style="margin-left:8px" />
          </div>
        </div>
      </template>

      <el-input
        ref="logAreaRef"
        type="textarea"
        :model-value="logText"
        readonly
        resize="none"
        class="log-textarea"
      />

      <el-alert
        v-if="error"
        :title="error"
        type="error"
        :closable="false"
        show-icon
        style="margin-top: 12px;"
      />

      <el-empty
        v-if="!loading && !error && logLines.length === 0"
        description="No logs available"
        style="margin-top: 24px;"
      />
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { get } from '../api/index.js'

const tailLines = ref(100)
const loading = ref(false)
const logLines = ref([])
const error = ref('')
const logAreaRef = ref(null)

const logText = computed(() => logLines.value.join('\n'))

async function refreshLogs() {
  loading.value = true
  error.value = ''
  try {
    const res = await get('/logs', { tail: tailLines.value })
    logLines.value = res.lines || []
    await nextTick()
    const textarea = logAreaRef.value?.$el?.querySelector('textarea')
    if (textarea) {
      textarea.scrollTop = textarea.scrollHeight
    }
  } catch (e) {
    error.value = e.response?.data?.error || 'Failed to load logs'
    logLines.value = []
  } finally {
    loading.value = false
  }
}

const liveActive = ref(false)
let ws = null

function toggleLive(val) {
  if (val) {
    startLiveWS()
  } else {
    stopLiveWS()
  }
}

function startLiveWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const url = proto + '//' + location.host + '/api/logs/ws?tail=' + (logLines.value?.length || 100)
  try {
    ws = new WebSocket(url)
  } catch (e) {
    ElMessage.error('Live log connection failed')
    liveActive.value = false
    return
  }
  ws.onmessage = (ev) => {
    try {
      const msg = JSON.parse(ev.data)
      if (msg.type === 'init') {
        logLines.value = msg.lines || []
      } else if (msg.type === 'line') {
        logLines.value.push(msg.data)
        if (logLines.value.length > 5000) logLines.value.shift()
      } else if (msg.type === 'reset') {
        logLines.value = ['--- Log file rotated ---']
      }
      nextTick(() => {
        const textarea = logAreaRef.value?.$el?.querySelector('textarea')
        if (textarea) {
          textarea.scrollTop = textarea.scrollHeight
        }
      })
    } catch {}
  }
  ws.onerror = () => {
    ElMessage.error('Live log connection failed')
    liveActive.value = false
  }
  ws.onclose = () => {
    if (liveActive.value) liveActive.value = false
  }
}

function stopLiveWS() {
  if (ws) { ws.close(); ws = null }
}

onMounted(() => {
  refreshLogs()
})

onBeforeUnmount(() => { stopLiveWS() })
</script>

<style scoped>
.logs-page {
  padding: 0;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header > span {
  font-size: 18px;
  font-weight: 600;
}

.header-actions {
  display: flex;
  align-items: center;
}

.log-textarea :deep(.el-textarea__inner) {
  font-family: 'Courier New', Courier, monospace;
  background: #1e1e1e;
  color: #d4d4d4;
  min-height: 500px;
  line-height: 1.5;
  font-size: 13px;
}
</style>
