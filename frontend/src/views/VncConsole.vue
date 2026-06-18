<template>
  <div>
    <h3>Cloud Shell / VNC Console</h3>
  <el-card>
    <el-form :model="form" label-width="100px">
      <el-form-item label="Tenant">
        <el-select v-model="form.tenantId" placeholder="Select tenant" @change="onTenantChange" style="width:100%">
          <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="Instance">
        <el-select v-model="form.instanceId" placeholder="Select instance" :disabled="!form.tenantId" filterable style="width:100%">
          <el-option v-for="inst in instances" :key="inst.id" :label="inst.name+' ('+inst.state+')'" :value="inst.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="SSH Key">
        <el-select v-model="form.sshKeyId" placeholder="Select SSH key" :disabled="!form.tenantId" style="width:100%">
          <el-option v-for="k in sshKeys" :key="k.id" :label="k.name + ' (' + (k.fingerprint||'').substring(0,16) + '...)'" :value="k.id" />
        </el-select>
        <div style="margin-top:6px; display:flex; gap:8px">
          <el-button size="small" @click="showGenerateDialog = true" :disabled="!form.tenantId">Generate Key</el-button>
          <el-button size="small" @click="showUploadDialog = true" :disabled="!form.tenantId">Upload Key</el-button>
          <el-button size="small" type="danger" plain :disabled="!form.sshKeyId" @click="handleDeleteKey">Delete Key</el-button>
        </div>
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="startSession" :loading="loading" :disabled="!canStart">Start Console</el-button>
          <el-button type="danger" @click="stopSession" :loading="stopping" :disabled="!session.active">Stop Console</el-button>
        </el-form-item>
      </el-form>

      <el-alert v-if="error" :title="error" type="error" show-icon :closable="true" @close="error=''" style="margin-bottom:12px" />

      <div v-if="session.active" style="margin-top:16px">
        <el-descriptions title="Session" :column="1" border>
          <el-descriptions-item label="Connection URL">
            <code style="word-break:break-all">{{ session.connectionString }}</code>
          </el-descriptions-item>
          <el-descriptions-item label="Console ID">
            <code style="word-break:break-all">{{ session.consoleId }}</code>
          </el-descriptions-item>
          <el-descriptions-item label="Fingerprint">
            <code>{{ session.fingerprint }}</code>
          </el-descriptions-item>
          <el-descriptions-item label="Host Key FP">
            <code>{{ session.serviceHostKeyFp }}</code>
          </el-descriptions-item>
        </el-descriptions>

        <p style="color:#909399;font-size:13px;margin-top:12px">
          VNC console requires websockify proxy (port 6080). Use the connection URL above with your preferred client,
          or configure a websockify proxy to bridge it.
        </p>

        <el-button type="primary" @click="openConsole">
          Open Console (WebSocket)
        </el-button>
      </div>
    </el-card>

    <!-- Generate SSH Key Dialog -->
    <el-dialog v-model="showGenerateDialog" title="Generate SSH Key" width="600px" :close-on-click-modal="false" @close="closeGenerateDialog">
      <el-form :model="genForm" label-width="80px">
        <el-form-item label="Key Name">
          <el-input v-model="genForm.name" placeholder="e.g. my-console-key" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleGenerateKey" :loading="generating" :disabled="!genForm.name">
            Generate
          </el-button>
        </el-form-item>
      </el-form>

      <template v-if="generatedKey">
        <el-divider />
        <el-alert title="Save this private key — it will not be shown again" type="warning" show-icon :closable="false" style="margin-bottom:12px" />
        <el-form label-width="80px">
          <el-form-item label="Fingerprint">
            <code>{{ generatedKey.fingerprint }}</code>
          </el-form-item>
          <el-form-item label="Public Key">
            <el-input type="textarea" :rows="3" :model-value="generatedKey.public_key" readonly />
          </el-form-item>
          <el-form-item label="Private Key">
            <el-input type="textarea" :rows="6" :model-value="generatedKey.private_key" readonly />
          </el-form-item>
          <el-form-item>
            <el-button type="primary" @click="downloadPrivateKey">
              Download .pem
            </el-button>
            <el-button @click="closeGenerateDialog">Done</el-button>
          </el-form-item>
        </el-form>
      </template>
    </el-dialog>

    <!-- Upload SSH Key Dialog -->
    <el-dialog v-model="showUploadDialog" title="Upload SSH Public Key" width="600px" @close="uploadForm.name=''; uploadForm.publicKey=''">
      <el-form :model="uploadForm" label-width="80px">
        <el-form-item label="Key Name">
          <el-input v-model="uploadForm.name" placeholder="e.g. my-key" />
        </el-form-item>
        <el-form-item label="Public Key">
          <el-input v-model="uploadForm.publicKey" type="textarea" :rows="6" placeholder="ssh-rsa AAAAB3Nz... or cat ~/.ssh/id_rsa.pub" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleUploadKey" :loading="uploading" :disabled="!uploadForm.name || !uploadForm.publicKey">
            Upload
          </el-button>
        </el-form-item>
      </el-form>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { listTenants } from '../api/tenants.js'
import { listInstances } from '../api/instances.js'
import { listSSHKeys, createSSHKey, deleteSSHKey, startVNC, stopVNC } from '../api/console.js'
import { ElMessage, ElMessageBox } from 'element-plus'

const tenants = ref([])
const instances = ref([])
const sshKeys = ref([])
const loading = ref(false)
const stopping = ref(false)
const error = ref('')
const showGenerateDialog = ref(false)
const showUploadDialog = ref(false)
const generating = ref(false)
const uploading = ref(false)
const generatedKey = ref(null)

const genForm = reactive({
  name: '',
})

const uploadForm = reactive({
  name: '',
  publicKey: '',
})

const form = reactive({
  tenantId: null,
  instanceId: '',
  sshKeyId: null,
})

const session = reactive({
  active: false,
  connectionString: '',
  consoleId: '',
  vncUrl: '',
  fingerprint: '',
  serviceHostKeyFp: '',
})

const canStart = computed(() => form.tenantId && form.instanceId && form.sshKeyId)

onMounted(async () => {
  try {
    const res = await listTenants()
    tenants.value = res.data || []
  } catch (e) {
    console.error('load tenants', e)
  }
})

async function onTenantChange() {
  form.instanceId = ''
  form.sshKeyId = null
  session.active = false
  instances.value = []
  sshKeys.value = []
  if (!form.tenantId) return
  await Promise.all([
    loadInstances(),
    loadSSHKeys(),
  ])
}

async function loadInstances() {
  try {
    const res = await listInstances({ tenant_id: form.tenantId, size: 500 })
    instances.value = res.data || []
  } catch (e) {
    console.error('load instances', e)
  }
}

async function loadSSHKeys() {
  try {
    const res = await listSSHKeys(form.tenantId)
    sshKeys.value = res.data || []
  } catch (e) {
    console.error('load ssh keys', e)
  }
}

async function handleGenerateKey() {
  if (!genForm.name) {
    ElMessage.warning('Please enter a key name')
    return
  }
  generating.value = true
  generatedKey.value = null
  try {
    const res = await createSSHKey({
      action: 'generate',
      tenant_id: form.tenantId,
      name: genForm.name,
    })
    generatedKey.value = res
    await loadSSHKeys()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message || 'Failed to generate key')
  } finally {
    generating.value = false
  }
}

function downloadPrivateKey() {
  if (!generatedKey.value?.private_key) return
  const blob = new Blob([generatedKey.value.private_key], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = (genForm.name || 'id_rsa').replace(/[^a-zA-Z0-9_-]/g, '_') + '.pem'
  a.click()
  URL.revokeObjectURL(url)
}

function closeGenerateDialog() {
  showGenerateDialog.value = false
  generatedKey.value = null
  genForm.name = ''
}

async function handleUploadKey() {
  if (!uploadForm.name || !uploadForm.publicKey) {
    ElMessage.warning('Please enter both name and public key')
    return
  }
  uploading.value = true
  try {
    await createSSHKey({
      tenant_id: form.tenantId,
      name: uploadForm.name,
      public_key: uploadForm.publicKey,
    })
    ElMessage.success('SSH key uploaded')
    showUploadDialog.value = false
    uploadForm.name = ''
    uploadForm.publicKey = ''
    await loadSSHKeys()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message || 'Failed to upload key')
  } finally {
    uploading.value = false
  }
}

async function handleDeleteKey() {
  if (!form.sshKeyId) return
  try {
    await ElMessageBox.confirm('Are you sure you want to delete this SSH key?', 'Confirm', {
      confirmButtonText: 'Delete',
      cancelButtonText: 'Cancel',
      type: 'warning',
    })
    await deleteSSHKey(form.sshKeyId)
    ElMessage.success('SSH key deleted')
    form.sshKeyId = null
    await loadSSHKeys()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || e.message || 'Failed to delete key')
    }
  }
}

async function startSession() {
  error.value = ''
  loading.value = true
  try {
    const res = await startVNC({
      tenant_id: form.tenantId,
      instance_id: form.instanceId,
      ssh_key_id: form.sshKeyId,
    })
    if (res.status === 'active') {
      session.active = true
      session.connectionString = res.connection_string || ''
      session.consoleId = res.console_id || ''
      session.vncUrl = res.vnc_url || ''
      session.fingerprint = res.fingerprint || ''
      session.serviceHostKeyFp = res.service_host_key_fp || ''
    } else {
      error.value = 'Console connection did not become active'
    }
  } catch (e) {
    error.value = e.response?.data?.error || e.message || 'Failed to start console'
  } finally {
    loading.value = false
  }
}

async function stopSession() {
  error.value = ''
  stopping.value = true
  try {
    await stopVNC({
      tenant_id: form.tenantId,
      instance_id: form.instanceId,
      console_id: session.consoleId,
    })
    session.active = false
    session.connectionString = ''
    session.consoleId = ''
    session.vncUrl = ''
    session.fingerprint = ''
    session.serviceHostKeyFp = ''
  } catch (e) {
    error.value = e.response?.data?.error || e.message || 'Failed to stop console'
  } finally {
    stopping.value = false
  }
}

function openConsole() {
  if (session.vncUrl) {
    window.open(session.vncUrl, '_blank')
  }
}
</script>
