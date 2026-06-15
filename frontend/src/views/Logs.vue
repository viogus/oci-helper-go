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
import { ref, computed, onMounted, nextTick } from 'vue'
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

onMounted(() => {
  refreshLogs()
})
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
