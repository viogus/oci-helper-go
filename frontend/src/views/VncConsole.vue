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
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { listTenants } from '../api/tenants.js'
import { listInstances } from '../api/instances.js'
import { listSSHKeys, startVNC, stopVNC } from '../api/console.js'

const tenants = ref([])
const instances = ref([])
const sshKeys = ref([])
const loading = ref(false)
const stopping = ref(false)
const error = ref('')

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
