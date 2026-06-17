<template>
  <div class="tenants-page">
    <div class="page-header">
      <h3>{{ $t('tenant.title') }}</h3>
      <el-button type="primary" @click="handleAdd">
        <el-icon><Plus /></el-icon> {{ $t('tenant.add') }}
      </el-button>
    </div>

    <div class="search-bar">
      <el-input
        v-model="keyword"
        :placeholder="$t('tenant.searchPlaceholder')"
        clearable
        @input="handleSearch"
        @clear="handleSearch"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
    </div>

    <el-table :data="tenants" stripe v-loading="loading" :empty-text="$t('tenant.notFound')">
      <el-table-column prop="id" :label="$t('tenant.id')" width="70" />
      <el-table-column prop="name" :label="$t('tenant.name')" min-width="160" />
      <el-table-column prop="region" :label="$t('tenant.region')" width="160" />
      <el-table-column :label="$t('tenant.status')" width="110">
        <template #default="{ row }">
          <el-tag
            :type="row.status === 'active' ? 'success' : row.status === 'error' ? 'danger' : 'info'"
            size="small"
          >
            {{ row.status || 'unknown' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="$t('tenant.actions')" width="180" fixed="right">
        <template #default="{ row }">
          <el-button
            type="primary"
            link
            size="small"
            :loading="syncingId === row.id"
            :disabled="syncingId !== null"
            @click="handleSync(row.id)"
          >
            {{ $t('common.sync') }}
          </el-button>
          <el-button type="danger" link size="small" @click="handleDelete(row.id)">
            {{ $t('common.delete') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <div v-if="total > 0" class="pagination-wrapper">
      <el-pagination
        :total="total"
        :page-size="size"
        :current-page="page"
        layout="total, prev, pager, next"
        @current-change="onPageChange"
      />
    </div>

    <!-- Add Tenant Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="$t('tenant.addTitle')"
      width="650px"
      :close-on-click-modal="false"
      @closed="onDialogClosed"
    >
      <el-form label-position="top">
        <el-form-item :label="$t('tenant.displayName')">
          <el-input v-model="form.name" :placeholder="$t('tenant.displayName')" />
        </el-form-item>

        <el-form-item :label="$t('tenant.ociConfig')">
          <el-input
            type="textarea"
            v-model="configPaste"
            :rows="6"
            placeholder="[DEFAULT]
user=ocid1.user.oc1..xxx
fingerprint=xx:xx:xx:...
tenancy=ocid1.tenancy.oc1..xxx
region=us-ashburn-1
key_file=~/.oci/key.pem"
            @input="parseConfig"
          />
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item :label="$t('tenant.tenancyOcid')">
              <el-input v-model="form.tenancyOcid" placeholder="ocid1.tenancy.oc1.." />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="$t('tenant.userOcid')">
              <el-input v-model="form.userOcid" placeholder="ocid1.user.oc1.." />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item :label="$t('tenant.region')">
              <el-input v-model="form.region" placeholder="us-ashburn-1" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="$t('tenant.fingerprint')">
              <el-input v-model="form.fingerprint" placeholder="xx:xx:xx:..." />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item :label="$t('tenant.keyFile')">
          <div class="key-section">
            <el-select v-model="form.keyFile" :placeholder="$t('tenant.selectKey')" @change="onKeySelect">
              <el-option :label="$t('tenant.selectKey')" value="" />
              <el-option
                v-for="k in keys"
                :key="k.name"
                :label="`${k.name} (${(k.size / 1024).toFixed(1)}KB, ${k.time})`"
                :value="k.name"
              />
            </el-select>
            <input
              ref="keyFileInput"
              type="file"
              accept=".pem"
              style="display:none"
              @change="onKeyFilePicked"
            />
            <el-button size="default" type="primary" @click="$refs.keyFileInput.click()">
              Upload
            </el-button>
            <input
              ref="batchInput"
              type="file"
              accept=".pem"
              multiple
              style="display:none"
              @change="onBatchKeysPicked"
            />
            <el-button size="default" @click="$refs.batchInput.click()">
              Batch
            </el-button>
          </div>
          <div
            class="upload-zone"
            :class="{ dragover: dragOver }"
            @dragover.prevent="dragOver = true"
            @dragleave="dragOver = false"
            @drop.prevent="handleDrop"
          >
            {{ $t('tenant.dragHere') }}
          </div>
          <div v-if="keyInfo" class="key-info">{{ keyInfo }}</div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('tenant.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">{{ $t('tenant.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Search } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
import {
  listTenants,
  createTenant,
  deleteTenant,
  syncTenant,
  listKeys,
  uploadKeys
} from '../api/tenants.js'


// --- state ---
const tenants = ref([])
const total = ref(0)
const page = ref(1)
const size = ref(20)
const keyword = ref('')
const loading = ref(false)
const syncingId = ref(null)
const saving = ref(false)

// --- dialog state ---
const dialogVisible = ref(false)
const form = reactive({
  name: '',
  tenancyOcid: '',
  userOcid: '',
  region: '',
  fingerprint: '',
  keyFile: ''
})
const configPaste = ref('')
const keys = ref([])
const keyInfo = ref('')
const dragOver = ref(false)

// --- search debounce ---
let searchTimer = null

// --- lifecycle ---
onMounted(() => {
  loadTenants()
})

// --- data loading ---
async function loadTenants() {
  loading.value = true
  try {
    const res = await listTenants({
      keyword: keyword.value || undefined,
      page: page.value,
      size: size.value
    })
    tenants.value = res.data || []
    total.value = res.total || 0
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to load tenants')
    tenants.value = []
    total.value = 0
  }
  loading.value = false
}

async function loadKeys() {
  try {
    const res = await listKeys()
    keys.value = Array.isArray(res) ? res : []
  } catch (e) {
    keys.value = []
  }
}

// --- search ---
function handleSearch() {
  page.value = 1
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    loadTenants()
  }, 300)
}

// --- pagination ---
function onPageChange(newPage) {
  page.value = newPage
  loadTenants()
}

// --- add dialog ---
function handleAdd() {
  form.name = ''
  form.tenancyOcid = ''
  form.userOcid = ''
  form.region = ''
  form.fingerprint = ''
  form.keyFile = ''
  configPaste.value = ''
  keyInfo.value = ''
  dragOver.value = false
  loadKeys()
  dialogVisible.value = true
}

function onDialogClosed() {
  // reset state on close
}

// --- OCI config parsing ---
function parseConfig() {
  const text = configPaste.value
  if (!text.trim()) return
  const m = {}
  text.split('\n').forEach(line => {
    line = line.trim()
    if (!line || line.startsWith('[')) return
    const eq = line.indexOf('=')
    if (eq < 0) return
    const k = line.substring(0, eq).trim().toLowerCase()
    const v = line.substring(eq + 1).trim()
    m[k] = v
  })
  if (m.tenancy) form.tenancyOcid = m.tenancy
  if (m.user) form.userOcid = m.user
  if (m.region) form.region = m.region
  if (m.fingerprint) form.fingerprint = m.fingerprint
  if (m.key_file) {
    const bn = m.key_file.replace(/\\/g, '/').split('/').pop()
    form.keyFile = ''
    keyInfo.value = t.value('tenant.willUse') + bn + ' (upload it if not yet on server)'
  }
}

// --- key management ---
function onKeySelect() {
  if (form.keyFile) {
    keyInfo.value = t.value('tenant.serverKey') + form.keyFile
  } else {
    keyInfo.value = ''
  }
}

async function onKeyFilePicked(e) {
  const file = e.target.files[0]
  if (!file) return
  await uploadKeyFile(file)
  e.target.value = ''
}

async function onBatchKeysPicked(e) {
  const files = Array.from(e.target.files)
  if (!files.length) return
  const fd = new FormData()
  for (const f of files) {
    fd.append('files', f)
  }
  try {
    await uploadKeys(fd)
    ElMessage.success(files.length + ' key(s) uploaded')
    await loadKeys()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Batch upload failed')
  }
  e.target.value = ''
}

async function uploadKeyFile(file) {
  if (!file.name.toLowerCase().endsWith('.pem')) {
    ElMessage.warning('Only .pem files are allowed')
    return
  }
  const fd = new FormData()
  fd.append('files', file)
  try {
    const result = await uploadKeys(fd)
    if (result.saved && result.saved.length) {
      const name = result.saved[0]
      form.keyFile = name
      keyInfo.value = t.value('tenant.uploaded') + name + ' - will use this key'
    }
    await loadKeys()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Upload failed')
  }
}

async function handleDrop(e) {
  dragOver.value = false
  const file = e.dataTransfer.files[0]
  if (!file) return
  await uploadKeyFile(file)
}

// --- CRUD ---
async function handleSave() {
  if (!form.name.trim()) {
    ElMessage.warning('Tenant name is required')
    return
  }
  if (!form.keyFile) {
    ElMessage.warning('Please select or upload a .pem key file')
    return
  }
  saving.value = true
  try {
    await createTenant({
      name: form.name,
      tenancyOcid: form.tenancyOcid,
      userOcid: form.userOcid,
      region: form.region,
      fingerprint: form.fingerprint,
      keyFile: form.keyFile
    })
    ElMessage.success('Tenant created')
    dialogVisible.value = false
    loadTenants()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to create tenant')
  }
  saving.value = false
}

async function handleDelete(id) {
  try {
    await ElMessageBox.confirm(
      'This will permanently delete this tenant and all its instances. Continue?',
      'Delete Tenant',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    await deleteTenant(id)
    ElMessage.success('Tenant deleted')
    loadTenants()
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error(err.response?.data?.error || 'Delete failed')
    }
  }
}

async function handleSync(id) {
  syncingId.value = id
  try {
    const result = await syncTenant(id)
    ElMessage.success('Synced ' + (result.count || 0) + ' instances')
    loadTenants()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Sync failed')
  }
  syncingId.value = null
}
</script>

<style scoped>
.tenants-page {
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

.search-bar {
  margin-bottom: 16px;
  max-width: 400px;
}

.pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 16px;
}

.key-section {
  display: flex;
  gap: 8px;
  align-items: center;
  width: 100%;
  margin-bottom: 6px;
}

.upload-zone {
  border: 2px dashed var(--el-border-color);
  border-radius: 6px;
  padding: 12px;
  text-align: center;
  color: var(--el-text-color-secondary);
  font-size: 12px;
  cursor: pointer;
  transition: border-color 0.2s, color 0.2s;
}

.upload-zone:hover,
.upload-zone.dragover {
  border-color: var(--el-color-primary);
  color: var(--el-color-primary);
}

.key-info {
  font-size: 12px;
  margin-top: 4px;
  color: var(--el-color-success);
}
</style>
