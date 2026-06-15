<template>
  <div class="backup-page">
    <!-- Export Card -->
    <el-card shadow="never" style="margin-bottom: 20px;">
      <template #header>
        <span>Export Backup</span>
      </template>

      <el-form label-position="top" @submit.prevent>
        <el-form-item label="Encryption Password">
          <el-input
            v-model="exportPassword"
            type="password"
            show-password
            placeholder="Enter password for encryption"
            :disabled="loading"
          />
        </el-form-item>
        <el-button type="primary" :loading="loading" @click="handleExport">
          Export
        </el-button>
      </el-form>

      <div v-if="exportResult" style="margin-top: 16px;">
        <el-alert
          title="Backup data ready — copy or download below"
          type="success"
          :closable="false"
          show-icon
          style="margin-bottom: 12px;"
        />
        <el-input
          type="textarea"
          :model-value="exportResult"
          readonly
          :rows="8"
          style="font-family: 'Courier New', Courier, monospace; font-size: 12px;"
        />
        <div style="margin-top: 8px; display: flex; gap: 8px;">
          <el-button @click="handleDownload">Download as JSON</el-button>
          <el-button @click="copyToClipboard">Copy to Clipboard</el-button>
        </div>
      </div>
    </el-card>

    <!-- Import / Restore Card -->
    <el-card shadow="never">
      <template #header>
        <span>Import / Restore</span>
      </template>

      <el-alert
        title="Warning: This will DELETE all existing data and replace it with the backup."
        type="warning"
        :closable="false"
        show-icon
        style="margin-bottom: 16px;"
      />

      <el-form label-position="top" @submit.prevent>
        <el-form-item label="Encryption Password">
          <el-input
            v-model="importPassword"
            type="password"
            show-password
            placeholder="Enter password used during export"
            :disabled="loading"
          />
        </el-form-item>
        <el-form-item label="Backup Data">
          <el-input
            v-model="backupData"
            type="textarea"
            :rows="8"
            placeholder="Paste the encrypted backup data here..."
            :disabled="loading"
            style="font-family: 'Courier New', Courier, monospace; font-size: 12px;"
          />
        </el-form-item>
        <el-button type="danger" :loading="loading" @click="handleRestore">
          Restore from Backup
        </el-button>
      </el-form>

      <el-empty
        v-if="!loading && !backupData && !exportResult"
        description="No backup data loaded"
        style="margin-top: 16px;"
      />
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { post } from '../api/index.js'

const exportPassword = ref('')
const importPassword = ref('')
const backupData = ref('')
const exportResult = ref('')
const loading = ref(false)

async function handleExport() {
  if (!exportPassword.value.trim()) {
    ElMessage.warning('Password is required')
    return
  }
  loading.value = true
  try {
    const r = await post('/backup', { password: exportPassword.value })
    exportResult.value = r.data
    ElMessage.success('Backup exported successfully')
  } catch (e) {
    const detail = e.response?.data?.error || 'Export failed'
    ElMessage.error(detail)
    exportResult.value = ''
  } finally {
    loading.value = false
  }
}

async function handleRestore() {
  if (!importPassword.value.trim()) {
    ElMessage.warning('Password is required')
    return
  }
  if (!backupData.value.trim()) {
    ElMessage.warning('Backup data is required')
    return
  }
  try {
    await ElMessageBox.confirm(
      'This will DELETE all existing data and restore from backup. This action cannot be undone. Continue?',
      'Warning',
      {
        type: 'warning',
        confirmButtonText: 'Restore',
        cancelButtonText: 'Cancel',
        confirmButtonClass: 'el-button--danger'
      }
    )
  } catch {
    return
  }
  loading.value = true
  try {
    await post('/restore', {
      password: importPassword.value,
      data: backupData.value
    })
    ElMessage.success('Data restored successfully')
    importPassword.value = ''
    backupData.value = ''
    exportResult.value = ''
  } catch (e) {
    const detail = e.response?.data?.error || 'Restore failed'
    ElMessage.error(detail)
  } finally {
    loading.value = false
  }
}

function handleDownload() {
  const blob = new Blob([exportResult.value], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download =
    'oci-helper-backup-' + new Date().toISOString().slice(0, 10) + '.json'
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
}

function copyToClipboard() {
  navigator.clipboard.writeText(exportResult.value).then(
    () => ElMessage.success('Copied to clipboard'),
    () => ElMessage.error('Failed to copy')
  )
}
</script>

<style scoped>
.backup-page {
  padding: 0;
}
</style>
