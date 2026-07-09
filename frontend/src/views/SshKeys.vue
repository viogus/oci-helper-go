<template>
  <div class="ssh-keys-page">
    <div class="page-header">
      <h3>{{ $t('sshKeys.title') }}</h3>
      <div class="header-actions">
        <el-button type="primary" @click="handleGenerate">
          <el-icon><Plus /></el-icon> {{ $t('sshKeys.generate') }}
        </el-button>
        <el-button @click="triggerUpload">
          {{ $t('sshKeys.upload') }}
        </el-button>
        <input
          ref="pemInput"
          type="file"
          accept=".pem"
          style="display:none"
          @change="onPemFilePicked"
        />
      </div>
    </div>

    <!-- Table -->
    <el-table
      :data="keys"
      stripe
      v-loading="loading"
      :empty-text="$t('sshKeys.notFound')"
    >
      <el-table-column prop="name" :label="$t('sshKeys.name')" min-width="140" />
      <el-table-column :label="$t('sshKeys.tenant')" width="120">
        <template #default="{ row }">
          <span v-if="row.tenantName">{{ row.tenantName }}</span>
          <el-tag v-else type="info" size="small">{{ $t('sshKeys.global') }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="$t('sshKeys.fingerprint')" min-width="200">
        <template #default="{ row }">
          <el-tooltip :content="row.fingerprint" placement="top" :show-after="300">
            <code style="font-family: monospace; font-size: 12px; cursor: default;">
              {{ truncate(row.fingerprint, 24) }}
            </code>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column :label="$t('sshKeys.type')" width="100">
        <template #default="{ row }">
          <el-tag v-if="row.key_type" size="small" :type="row.key_type === 'ed25519' ? 'success' : 'primary'">
            {{ row.key_type.toUpperCase() }}
          </el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column :label="$t('sshKeys.publicKey')" min-width="250">
        <template #default="{ row }">
          <div class="key-cell">
            <code class="key-text">{{ truncate(row.public_key, 60) }}</code>
            <el-button
              v-if="row.public_key"
              type="primary"
              link
              size="small"
              @click="copyKey(row.public_key)"
            >
              Copy
            </el-button>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="$t('sshKeys.actions')" width="100" fixed="right">
        <template #default="{ row }">
          <el-button type="danger" link size="small" @click="handleDelete(row)">
            {{ $t('common.delete') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty
      v-if="!loading && keys.length === 0"
      :description="$t('sshKeys.notFound')"
    />

    <!-- Generate Keypair Dialog -->
    <el-dialog
      v-model="generateVisible"
      :title="$t('sshKeys.generateTitle')"
      width="520px"
      :close-on-click-modal="false"
      @closed="resetGenerateForm"
    >
      <el-form label-position="top">
        <el-form-item :label="$t('sshKeys.name')" required>
          <el-input v-model="genForm.name" :placeholder="$t('sshKeys.name')" />
        </el-form-item>
        <el-form-item :label="$t('sshKeys.type')">
          <el-select v-model="genForm.keyType" style="width: 100%">
            <el-option label="ED25519" value="ed25519" />
            <el-option label="RSA 4096" value="rsa" />
          </el-select>
        </el-form-item>
      </el-form>

      <div v-if="generatedKey" style="margin-top: 16px">
        <el-form-item :label="$t('sshKeys.publicKey')">
          <el-input
            type="textarea"
            :model-value="generatedKey"
            :rows="4"
            readonly
          />
        </el-form-item>
        <div style="display: flex; align-items: center; gap: 8px;">
          <el-button size="small" @click="copyKey(generatedKey)">
            {{ $t('sshKeys.copyKey') }}
          </el-button>
          <el-text type="warning" size="small">
            {{ $t('sshKeys.keyWarning') }}
          </el-text>
        </div>
      </div>

      <template #footer>
        <el-button @click="generateVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button
          v-if="!generatedKey"
          type="primary"
          :loading="generating"
          @click="handleGenerateConfirm"
        >
          {{ $t('sshKeys.generate') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
import { get, post, upload, del } from '../api/index.js'

const keys = ref([])
const loading = ref(false)
const pemInput = ref(null)

// Generate dialog state
const generateVisible = ref(false)
const generating = ref(false)
const generatedKey = ref('')
const genForm = reactive({
  name: '',
  keyType: 'ed25519'
})

// Upload
const generatingInDialog = ref(false)

function truncate(val, max) {
  if (!val) return '-'
  return val.length > max ? val.substring(0, max) + '...' : val
}

onMounted(() => {
  loadKeys()
})

async function loadKeys() {
  loading.value = true
  try {
    const res = await get('/ssh/keys')
    keys.value = res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to load SSH keys')
    keys.value = []
  }
  loading.value = false
}

function resetGenerateForm() {
  genForm.name = ''
  genForm.keyType = 'ed25519'
  generatedKey.value = ''
}

function triggerUpload() {
  pemInput.value?.click()
}

function handleGenerate() {
  resetGenerateForm()
  generateVisible.value = true
}

async function handleGenerateConfirm() {
  if (!genForm.name.trim()) {
    ElMessage.warning('Key name is required')
    return
  }
  generating.value = true
  try {
    const res = await post('/ssh/keys?action=generate', {
      name: genForm.name,
      key_type: genForm.keyType
    })
    generatedKey.value = res.public_key || ''
    ElMessage.success('Keypair generated')
    loadKeys()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to generate keypair')
  }
  generating.value = false
}

async function onPemFilePicked(e) {
  const file = e.target.files[0]
  if (!file) return
  const fd = new FormData()
  fd.append('file', file)
  try {
    await upload('/ssh/keys', fd)
    ElMessage.success('PEM key uploaded')
    loadKeys()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to upload PEM')
  }
  e.target.value = ''
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(
      t('sshKeys.confirmDelete'),
      t('common.delete'),
      {
        confirmButtonText: t('common.delete'),
        cancelButtonText: t('common.cancel'),
        type: 'warning'
      }
    )
    await del(`/ssh/keys/${row.id}`)
    ElMessage.success('SSH key deleted')
    loadKeys()
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error(err.response?.data?.error || 'Delete failed')
    }
  }
}

async function copyKey(text) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('Copied to clipboard')
  } catch {
    ElMessage.error('Copy failed')
  }
}
</script>

<style scoped>
.ssh-keys-page {
  padding: 0;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}

.page-header h3 {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}

.header-actions {
  display: flex;
  gap: 8px;
}

.key-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.key-text {
  font-family: monospace;
  font-size: 12px;
  word-break: break-all;
}
</style>
