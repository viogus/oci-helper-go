<template>
  <div>
    <h3>{{ $t('shell.title') }}</h3>
    <el-card>
      <el-form :model="form" label-width="100px">
        <el-form-item :label="$t('shell.selectTenant')">
          <el-select v-model="form.tenantId" :placeholder="$t('shell.selectTenant')" @change="onTenantChange" style="width:100%">
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('shell.selectInstance')">
          <el-select v-model="form.instanceId" :placeholder="$t('shell.selectInstance')" :disabled="!form.tenantId" filterable style="width:100%">
            <el-option v-for="inst in instances" :key="inst.id" :label="inst.name+' ('+inst.state+')'" :value="inst.id" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('shell.selectSSHKey')">
          <el-select v-model="form.sshKeyId" :placeholder="$t('shell.selectSSHKey')" :disabled="!form.tenantId" style="width:100%">
            <el-option v-for="k in sshKeys" :key="k.id" :label="k.name + ' (' + (k.fingerprint||'').substring(0,16) + '...)'" :value="k.id" />
          </el-select>
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="connect" :loading="connecting" :disabled="!canConnect">
            {{ $t('shell.connect') }}
          </el-button>
          <el-button type="danger" @click="disconnect" :disabled="!connected">
            {{ $t('shell.disconnect') }}
          </el-button>
          <el-tag v-if="statusText" :type="statusType" style="margin-left:8px">{{ statusText }}</el-tag>
        </el-form-item>
      </el-form>

      <el-alert v-if="error" :title="error" type="error" show-icon :closable="true" @close="error=''" style="margin-bottom:12px" />

      <div v-show="connected || connecting" class="terminal-wrapper">
        <div ref="terminalContainer" class="terminal-container"></div>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { listTenants } from '../api/tenants.js'
import { listInstances } from '../api/instances.js'
import { listSSHKeys } from '../api/console.js'

const tenants = ref([])
const instances = ref([])
const sshKeys = ref([])
const connecting = ref(false)
const connected = ref(false)
const statusText = ref('')
const statusType = ref('info')
const error = ref('')
const terminalContainer = ref(null)

let term = null
let fitAddon = null
let ws = null
let resizeTimer = null

const form = reactive({
  tenantId: null,
  instanceId: '',
  sshKeyId: null,
})

const canConnect = computed(() => form.tenantId && form.instanceId && form.sshKeyId && !connected.value)

onMounted(async () => {
  try {
    const res = await listTenants()
    tenants.value = res.data || []
  } catch (e) {
    console.error('load tenants', e)
  }
})

onBeforeUnmount(() => {
  disconnect()
})

async function onTenantChange() {
  form.instanceId = ''
  form.sshKeyId = null
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

function toBase64(str) {
  const bytes = new TextEncoder().encode(str)
  const binStr = Array.from(bytes, (b) => String.fromCharCode(b)).join('')
  return btoa(binStr)
}

function fromBase64(b64) {
  const binStr = atob(b64)
  const bytes = Uint8Array.from(binStr, (c) => c.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

async function connect() {
  error.value = ''
  connecting.value = true
  statusText.value = 'Connecting...'
  statusType.value = 'warning'

  try {
    // Create terminal
    await nextTick()
    if (term) term.dispose()
    term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: {
        background: '#1e1e1e',
        foreground: '#d4d4d4',
        cursor: '#ffffff',
        selectionBackground: '#264f78',
      },
      rows: 24,
      cols: 80,
      allowProposedApi: true,
    })
    fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())
    term.open(terminalContainer.value)
    fitAddon.fit()

    // Build WebSocket URL
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = location.host
    const { rows, cols } = term
    const wsUrl = `${proto}//${host}/api/shell/ws?tenant_id=${form.tenantId}&instance_id=${encodeURIComponent(form.instanceId)}&ssh_key_id=${form.sshKeyId}&rows=${rows}&cols=${cols}`

    ws = new WebSocket(wsUrl)

    ws.onopen = () => {
      connecting.value = false
      connected.value = true
    }

    ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        switch (msg.type) {
          case 'output':
            term.write(fromBase64(msg.data))
            break
          case 'ready':
            statusText.value = 'Connected'
            statusType.value = 'success'
            term.focus()
            break
          case 'error':
            error.value = msg.message
            disconnect()
            break
        }
      } catch {
        // ignore malformed messages
      }
    }

    ws.onclose = () => {
      disconnect()
    }

    ws.onerror = () => {
      error.value = 'WebSocket connection error'
      disconnect()
    }

    // Terminal input → WebSocket
    term.onData((data) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data: toBase64(data) }))
      }
    })

    // Terminal resize → WebSocket (debounced)
    term.onResize(({ rows, cols }) => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        if (resizeTimer) clearTimeout(resizeTimer)
        resizeTimer = setTimeout(() => {
          ws.send(JSON.stringify({ type: 'resize', rows, cols }))
        }, 200)
      }
    })

  } catch (e) {
    error.value = e.message || 'Connection failed'
    connecting.value = false
  }
}

function disconnect() {
  if (ws) {
    ws.close()
    ws = null
  }
  if (resizeTimer) {
    clearTimeout(resizeTimer)
    resizeTimer = null
  }
  if (term) {
    term.dispose()
    term = null
  }
  connecting.value = false
  if (connected.value) {
    statusText.value = 'Disconnected'
    statusType.value = 'info'
  }
  connected.value = false
}
</script>

<style scoped>
.terminal-wrapper {
  margin-top: 16px;
  border: 1px solid var(--el-border-color, #dcdfe6);
  border-radius: 4px;
  overflow: hidden;
}
.terminal-container {
  width: 100%;
  height: 500px;
}
</style>
