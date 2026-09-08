<template>
  <div>
    <div class="page-header">
      <el-button @click="$router.push('/instances')" :icon="'ArrowLeft'" text>{{ $t('common.back') }}</el-button>
      <h3>{{ inst.name || $t('instanceDetail.title') }}</h3>
      <el-tag :type="stateType" size="small">{{ inst.state }}</el-tag>
    </div>

    <!-- Action buttons -->
    <div v-if="inst.id" class="action-bar">
      <el-button
        v-if="inst.state === 'STOPPED' || inst.state === 'TERMINATED'"
        type="success"
        :loading="acting === 'start'"
        @click="doAction('start')"
      >
        {{ $t('instanceDetail.start') }}
      </el-button>
      <el-button
        v-if="inst.state === 'RUNNING'"
        type="warning"
        :loading="acting === 'stop'"
        @click="doAction('stop')"
      >
        {{ $t('instanceDetail.stop') }}
      </el-button>
      <el-button
        v-if="inst.state === 'RUNNING'"
        :loading="acting === 'softstop'"
        @click="doAction('softstop')"
      >
        {{ $t('instanceDetail.softStop') }}
      </el-button>
      <el-button
        v-if="inst.state === 'RUNNING'"
        type="warning"
        :loading="acting === 'reboot'"
        @click="doAction('reboot')"
      >
        {{ $t('instanceDetail.reboot') }}
      </el-button>
      <el-button
        :loading="acting === 'terminate'"
        @click="confirmTerminate"
      >
        {{ $t('instanceDetail.terminate') }}
      </el-button>
      <el-button
        :loading="acting === 'shrinkDisk'"
        @click="handleShrinkDisk"
      >
        Shrink Disk
      </el-button>
      <el-button
        :loading="acting === 'netboot'"
        @click="handleNetbootRescue"
      >
        Netboot Rescue
      </el-button>
      <el-button
        :loading="acting === 'netbootStop'"
        @click="handleNetbootStop"
      >
        Stop Rescue
      </el-button>

      <el-divider direction="vertical" />

      <el-button @click="$router.push('/shell')">
        {{ $t('instanceDetail.cloudShell') }}
      </el-button>
      <el-button @click="openConfigDialog">
        Modify Config
      </el-button>
      <el-button @click="changeIpDialog = true">
        {{ $t('instanceDetail.changeIp') }}
      </el-button>
      <el-button @click="showVncInfo">
        {{ $t('instanceDetail.vnc') }}
      </el-button>
      <el-button @click="rootPasswordDialog = true">
        Root 密码
      </el-button>
    </div>

    <el-card v-loading="loading" style="margin-top:12px">
      <template v-if="!loading && inst.id">
        <el-descriptions :column="2" border>
          <el-descriptions-item :label="$t('instanceDetail.name')">{{ inst.name }}</el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.region')">
            <el-tag size="small">{{ regionName }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.status')">
            <el-tag :type="stateType" size="small">{{ inst.state }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.shape')">
            <code>{{ inst.shape }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.publicIP')">
            <code v-if="inst.publicIp">{{ inst.publicIp }}</code>
            <span v-else style="color:var(--text-muted)">—</span>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.privateIP')">
            <code v-if="inst.privateIp">{{ inst.privateIp }}</code>
            <span v-else style="color:var(--text-muted)">—</span>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.config')">
            {{ inst.ocpu }} {{ $t('instanceDetail.cores') }} / {{ inst.memoryGB }} GB / {{ inst.bootVolumeGB }} GB
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.availabilityDomain')">
            <code>{{ inst.availabilityDomain || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.faultDomain')">
            <code>{{ inst.faultDomain || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.imageID')">
            <code>{{ inst.imageId || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.ocid')" :span="2">
            <code>{{ inst.ocid }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.subnetID')">
            <code>{{ inst.subnetId || '—' }}</code>
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.tenantID')">
            {{ inst.tenantId }}
          </el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.createdAt')">{{ formatTime(inst.createdAt) }}</el-descriptions-item>
          <el-descriptions-item :label="$t('instanceDetail.syncedAt')">{{ formatTime(inst.syncedAt) }}</el-descriptions-item>
        </el-descriptions>
      </template>
      <el-empty v-if="!loading && !inst.id" :description="$t('instanceDetail.notFound')" />
    </el-card>

    <!-- Instance ↔ DNS bindings: which Cloudflare DNS names this instance's
         public IP is synced to. Auto-sync / change-IP update exactly these. -->
    <el-card style="margin-top:12px">
      <template #header>
        <div class="autosync-header">
          <span>Cloudflare DNS Bindings</span>
          <el-button type="primary" size="small" :disabled="!inst.id" @click="openAddBindingDialog">
            Add Binding
          </el-button>
        </div>
      </template>
      <el-table
        :data="bindings"
        v-loading="bindingsLoading"
        border
        stripe
        size="small"
        style="width: 100%"
      >
        <el-table-column prop="name" label="DNS Name" min-width="180" />
        <el-table-column label="Zone" min-width="140">
          <template #default="{ row }">
            {{ zoneName(row.zoneId) }}
          </template>
        </el-table-column>
        <el-table-column label="CF Config" width="120">
          <template #default="{ row }">
            {{ cfCfgName(row.cfCfgId) }}
          </template>
        </el-table-column>
        <el-table-column prop="ttl" label="TTL" width="70" align="center" />
        <el-table-column label="Proxied" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.proxied ? 'success' : 'info'" size="small">{{ row.proxied ? 'Yes' : 'No' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Enabled" width="90" align="center">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? 'On' : 'Off' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Actions" width="130" align="center">
          <template #default="{ row }">
            <el-button type="primary" link size="small" @click="openEditBindingDialog(row)">Edit</el-button>
            <el-button type="danger" link size="small" @click="handleDeleteBinding(row)">Delete</el-button>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="No DNS bindings for this instance." />
        </template>
      </el-table>
    </el-card>

    <!-- Change IP dialog -->
    <el-dialog v-model="changeIpDialog" :title="$t('instanceDetail.changeIpTitle')" width="520px" @closed="changeIpResult = ''">
      <p>{{ $t('instanceDetail.changeIpDesc', { ip: inst.publicIp || inst.privateIp }) }}</p>
      <div style="display:flex; gap:8px; align-items:center">
        <el-input v-model="changeIpCidr" placeholder="e.g. 10.0.0.0/24" />
        <el-button type="primary" :loading="changingIp" @click="doChangeIp">
          {{ $t('instanceDetail.startChange') }}
        </el-button>
      </div>
      <el-form label-position="top" style="margin-top:12px">
        <el-form-item label="更换后同步 Cloudflare DNS">
          <el-switch v-model="changeCfDns" />
        </el-form-item>
        <template v-if="changeCfDns">
          <el-form-item label="CF 配置">
            <el-select v-model="selectedDomainCfgId" style="width:100%">
              <el-option v-for="c in cfConfigs" :key="c.id" :label="c.name" :value="c.id" />
            </el-select>
          </el-form-item>
          <el-form-item label="域名前缀">
            <el-input v-model="domainPrefix" placeholder="sub" />
          </el-form-item>
          <el-form-item label="代理">
            <el-switch v-model="enableProxy" />
          </el-form-item>
          <el-form-item label="TTL">
            <el-input-number v-model="ttl" :min="1" :max="2147483647" />
          </el-form-item>
          <el-form-item label="备注">
            <el-input v-model="remark" />
          </el-form-item>
        </template>
      </el-form>
    </el-dialog>

    <el-dialog v-model="rootPasswordDialog" title="更新 Root 密码标签" width="420px">
      <el-input
        v-model="rootPassword"
        type="password"
        show-password
        placeholder="留空则删除 root-password 标签"
      />
      <template #footer>
        <el-button @click="rootPasswordDialog = false">取消</el-button>
        <el-button type="primary" :loading="savingRootPassword" @click="saveRootPassword">保存</el-button>
      </template>
    </el-dialog>

    <!-- VNC info dialog -->
    <el-dialog v-model="vncDialog" :title="$t('instanceDetail.vnc')" width="500px">
      <p>{{ $t('instanceDetail.vncDesc') }}</p>
      <el-alert
        v-if="vncUrl"
        :title="$t('instanceDetail.vncUrl')"
        :description="vncUrl"
        type="info"
        :closable="false"
        show-icon
      />
      <el-empty v-if="!vncUrl" :description="$t('instanceDetail.vncUnavailable')" />
    </el-dialog>

    <!-- Terminate confirm dialog -->
    <el-dialog v-model="terminateDialog" :title="$t('instanceDetail.terminateTitle')" width="400px">
      <el-alert
        :title="$t('instanceDetail.terminateWarn', { name: inst.name })"
        type="error"
        :closable="false"
        show-icon
      />
      <div style="margin-top:12px;display:flex;justify-content:flex-end;gap:8px">
        <el-button @click="terminateDialog = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="danger" :loading="acting === 'terminate'" @click="doAction('terminate')">
          {{ $t('instanceDetail.confirmTerminate') }}
        </el-button>
      </div>
    </el-dialog>

    <!-- Modify config dialog (Java's OciUpdateInstanceCfg direct mode) -->
    <el-dialog v-model="configDialogVisible" title="Modify Instance Config" width="420px">
      <el-form label-position="top">
        <el-form-item label="Shape">
          <el-input v-model="configForm.shape" placeholder="Keep current shape" />
        </el-form-item>
        <el-form-item label="OCPUs" required>
          <el-input-number v-model="configForm.ocpus" :min="0.1" :step="0.5" controls-position="right" style="width:180px" />
        </el-form-item>
        <el-form-item label="Memory (GB)" required>
          <el-input-number v-model="configForm.memoryGB" :min="1" :step="1" controls-position="right" style="width:180px" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="configDialogVisible = false">Cancel</el-button>
        <el-button type="primary" :loading="acting === 'config'" @click="confirmConfigUpdate">
          Apply
        </el-button>
      </template>
    </el-dialog>

    <!-- Add / Edit DNS Binding Dialog -->
    <el-dialog
      v-model="bindingDialog"
      :title="bindingEditing ? 'Edit DNS Binding' : 'Add DNS Binding'"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-form :model="bindingForm" label-width="110px">
        <el-form-item label="DNS Name" required>
          <el-input v-model="bindingForm.name" placeholder="e.g. vps1.example.com (full record name)" />
        </el-form-item>
        <el-form-item label="Zone" required>
          <el-select v-model="bindingForm.zoneId" filterable placeholder="Zone ID" style="width:100%">
            <el-option v-for="z in zones" :key="z.id" :label="z.name + ' · ' + z.id" :value="z.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="CF Config">
          <el-select v-model="bindingForm.cfCfgId" clearable placeholder="(token fallback)" style="width:100%">
            <el-option v-for="c in cfConfigs" :key="c.id" :label="c.name" :value="c.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="Proxied">
          <el-switch v-model="bindingForm.proxied" />
        </el-form-item>
        <el-form-item label="TTL">
          <el-input-number v-model="bindingForm.ttl" :min="1" :max="2147483647" :step="60" controls-position="right" />
        </el-form-item>
        <el-form-item label="Enabled">
          <el-switch v-model="bindingForm.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bindingDialog = false">Cancel</el-button>
        <el-button type="primary" :loading="savingBinding" @click="handleSaveBinding">
          {{ bindingEditing ? 'Update' : 'Create' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { get, post } from '../api/index.js'
import { listBindings, createBinding, updateBinding, deleteBinding, listZones as cfListZones } from '../api/cloudflare.js'
import { instanceAction } from '../api/instances.js'

const route = useRoute()
const router = useRouter()
const loading = ref(true)
const inst = ref({})
const regionName = ref('')

// Actions
const acting = ref('')
const terminateDialog = ref(false)

// Change IP
const changeIpDialog = ref(false)
const changeIpCidr = ref('')
const changingIp = ref(false)
const changeIpResult = ref('')
const changeCfDns = ref(false)
const selectedDomainCfgId = ref(null)
const domainPrefix = ref('')
const enableProxy = ref(false)
const ttl = ref(120)
const remark = ref('')
const cfConfigs = ref([])

// Root password
const rootPasswordDialog = ref(false)
const rootPassword = ref('')
const savingRootPassword = ref(false)

// VNC
const vncDialog = ref(false)
const vncUrl = ref('')

const stateType = computed(() => {
  const s = inst.value.state
  if (s === 'RUNNING') return 'success'
  if (s === 'STOPPED' || s === 'TERMINATED') return 'danger'
  if (s === 'STARTING' || s === 'STOPPING') return 'warning'
  return 'info'
})

onMounted(async () => {
  const id = decodeURIComponent(route.params.id)
  try {
    const res = await get(`/instances/${id}`)
    inst.value = res || {}

    if (inst.value.tenantId) {
      try {
        const tRes = await get(`/tenants/${inst.value.tenantId}`)
        regionName.value = tRes?.region || tRes?.homeRegion || ''
      } catch {}
    }
    const cfRes = await get('/cloudflare/cfgs')
    cfConfigs.value = cfRes?.data || []
    await Promise.all([loadBindings(), loadZones()])
  } catch {}
  loading.value = false
})

// requestTerminateCaptcha sends a verification code over the configured
// notification channel and returns the user-entered code, or null when the
// user cancels, or false when no channel is available (server then skips the
// check). Mirrors the Instances.vue flow.
async function requestTerminateCaptcha() {
  let recipient = null
  let target = ''
  try {
    const cfg = await get('/config')
    if (cfg.telegram_chat_id) {
      recipient = 'telegram'
      target = String(cfg.telegram_chat_id)
    } else if (cfg.dingtalk_webhook) {
      recipient = 'dingtalk'
      target = 'dingtalk'
    }
  } catch {
    return null
  }
  if (!recipient) return false
  try {
    await post('/captcha/send', { recipient, target })
  } catch (e) {
    ElMessage.error('Failed to send verification code: ' + (e.response?.data?.error || e.message))
    return null
  }
  try {
    const { value } = await ElMessageBox.prompt(
      `A verification code was sent via ${recipient}. Enter it to confirm termination:`,
      'Verification Code',
      { confirmButtonText: 'Terminate', cancelButtonText: 'Cancel', inputPlaceholder: '6-digit code' }
    )
    return { code: (value || '').trim(), target }
  } catch {
    return null
  }
}

// ShrinkDisk recreates the instance with a ~47GB boot volume (Java's
// "缩容到 47GB" flow). Long-running synchronous operation; keep the button
// disabled while it runs.
async function handleShrinkDisk() {
  try {
    await ElMessageBox.confirm(
      `Shrink "${inst.value.name}" boot volume to ~47GB?\n\nThis recreates the instance and may cause brief downtime.`,
      'Shrink Disk',
      { confirmButtonText: 'Shrink', cancelButtonText: 'Cancel', type: 'warning' }
    )
  } catch {
    return
  }
  acting.value = 'shrinkDisk'
  try {
    await post('/instances/shrink-disk', {
      tenant_id: inst.value.tenantId,
      instance_id: decodeURIComponent(route.params.id),
      retain_bl: false,
      retain_nat_gw: false
    })
    ElMessage.success('Shrink disk completed')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Shrink disk failed')
  }
  acting.value = ''
}

// NetbootRescue detaches the boot volume, boots a temp Oracle Linux rescue
// instance and re-attaches it (up to 10 minutes, synchronous). The axios
// default 30s timeout is overridden for this call.
async function handleNetbootRescue() {
  try {
    await ElMessageBox.confirm(
      `Start netboot rescue for "${inst.value.name}"?\n\nThis detaches the boot volume and boots a temporary rescue instance. May take several minutes.`,
      'Netboot Rescue',
      { confirmButtonText: 'Start', cancelButtonText: 'Cancel', type: 'warning' }
    )
  } catch {
    return
  }
  acting.value = 'netboot'
  ElMessage.info('Netboot rescue started — this can take up to 10 minutes...')
  try {
    await post('/instances/netboot-rescue', {
      tenant_id: inst.value.tenantId,
      instance_id: decodeURIComponent(route.params.id),
      rescue_image_id: ''
    }, { timeout: 620000 })
    ElMessage.success('Netboot rescue completed')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Netboot rescue failed')
  }
  acting.value = ''
}

// NetbootStop detaches the rescue boot volume, re-attaches the original boot
// volume and starts the instance (up to 10 minutes, synchronous).
async function handleNetbootStop() {
  try {
    await ElMessageBox.confirm(
      `Stop netboot rescue for "${inst.value.name}"?\n\nThis re-attaches the original boot volume and starts the instance. May take several minutes.`,
      'Stop Netboot Rescue',
      { confirmButtonText: 'Stop', cancelButtonText: 'Cancel', type: 'warning' }
    )
  } catch {
    return
  }
  acting.value = 'netbootStop'
  ElMessage.info('Stopping netboot rescue — this can take up to 10 minutes...')
  try {
    await post('/instances/netboot-rescue/stop', {
      tenant_id: inst.value.tenantId,
      instance_id: decodeURIComponent(route.params.id)
    }, { timeout: 620000 })
    ElMessage.success('Netboot rescue stopped, instance starting')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Stop netboot rescue failed')
  }
  acting.value = ''
}

async function doAction(action) {
  acting.value = action
  const id = decodeURIComponent(route.params.id)
  let extra = {}
  if (action === 'terminate') {
    const captcha = await requestTerminateCaptcha()
    if (!captcha) {
      acting.value = ''
      return // cancelled or send failed
    }
    extra = { captchaCode: captcha.code, captchaTarget: captcha.target }
  }
  try {
    await instanceAction(id, action, extra)
    ElMessage.success(`Action "${action}" sent`)
    terminateDialog.value = false
    // Refresh after short delay
    setTimeout(async () => {
      try {
        const res = await get(`/instances/${id}`)
        inst.value = res || inst.value
      } catch {}
    }, 3000)
  } catch (e) {
    ElMessage.error(e.response?.data?.error || `Action "${action}" failed`)
  }
  acting.value = ''
}

function confirmTerminate() {
  terminateDialog.value = true
}

async function doChangeIp() {
  if (!changeIpCidr.value) return
  changingIp.value = true
  try {
    const res = await post('/instances/change-ip', {
      instance_id: decodeURIComponent(route.params.id),
      tenant_id: inst.value.tenantId,
      cidr_list: [changeIpCidr.value],
      change_cf_dns: changeCfDns.value,
      selected_domain_cfg_id: selectedDomainCfgId.value,
      domain_prefix: domainPrefix.value,
      enable_proxy: enableProxy.value,
      ttl: ttl.value,
      remark: remark.value
    })
    ElMessage.success(res?.message || 'Change IP task created')
    changeIpDialog.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Change IP failed')
  }
  changingIp.value = false
}

async function saveRootPassword() {
  savingRootPassword.value = true
  try {
    await post('/instances/update-password', {
      instance_id: decodeURIComponent(route.params.id),
      tenant_id: inst.value.tenantId,
      password: rootPassword.value
    })
    ElMessage.success('已更新 root 密码标签')
    rootPasswordDialog.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '更新失败')
  } finally {
    savingRootPassword.value = false
  }
}

async function showVncInfo() {
  vncDialog.value = true
  vncUrl.value = ''
  try {
    const id = decodeURIComponent(route.params.id)
    const res = await get(`/instances/${id}`)
    // VNC URL is constructed from instance data
    if (res?.publicIp) {
      vncUrl.value = `vnc://${res.publicIp}:5900`
    }
  } catch {}
}

function formatTime(t) {
  if (!t) return '—'
  return new Date(t).toLocaleString()
}
// Modify instance config (direct OCPU/memory update, Java OciUpdateInstanceCfg
// parity). No frontend caller existed before this dialog.
const configDialogVisible = ref(false)
const configForm = ref({ shape: '', ocpus: 1, memoryGB: 1 })

function openConfigDialog() {
  configForm.value = {
    shape: inst.value.shape || '',
    ocpus: Number(inst.value.ocpu) || 1,
    memoryGB: Number(inst.value.memoryGB) || 1
  }
  configDialogVisible.value = true
}

async function confirmConfigUpdate() {
  acting.value = 'config'
  try {
    await post('/instances/config-update', {
      tenant_id: inst.value.tenantId,
      instance_id: decodeURIComponent(route.params.id),
      display_name: '',
      shape: configForm.value.shape || '',
      ocpus: Number(configForm.value.ocpus) || 1,
      memory_gb: Number(configForm.value.memoryGB) || 1
    })
    ElMessage.success('Instance config updated')
    configDialogVisible.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Config update failed')
  } finally {
    acting.value = ''
  }
}

// ── Cloudflare DNS bindings (per-instance) ─────────────────────────────
const bindings = ref([])
const zones = ref([])
const bindingsLoading = ref(false)
const bindingDialog = ref(false)
const bindingEditing = ref(false)
const editingBindingId = ref('')
const savingBinding = ref(false)
const bindingForm = reactive({
  name: '',
  zoneId: '',
  cfCfgId: null,
  proxied: false,
  ttl: 120,
  enabled: true
})

function currentInstanceID() {
  return inst.value?.id || ''
}

function zoneName(id) {
  const z = zones.value.find(z => z.id === id)
  return z ? z.name : (id || '—')
}

function cfCfgName(id) {
  const c = cfConfigs.value.find(c => c.id === id)
  return c ? c.name : 'default token'
}

async function loadZones() {
  try {
    zones.value = (await cfListZones()) || []
  } catch {
    zones.value = []
  }
}

async function loadBindings() {
  const iid = currentInstanceID()
  if (!iid) {
    bindings.value = []
    return
  }
  bindingsLoading.value = true
  try {
    const res = await listBindings(iid)
    bindings.value = res?.data || []
  } catch {
    bindings.value = []
  } finally {
    bindingsLoading.value = false
  }
}

function resetBindingForm() {
  bindingForm.name = ''
  bindingForm.zoneId = ''
  bindingForm.cfCfgId = null
  bindingForm.proxied = false
  bindingForm.ttl = 120
  bindingForm.enabled = true
}

function openAddBindingDialog() {
  bindingEditing.value = false
  editingBindingId.value = ''
  resetBindingForm()
  bindingDialog.value = true
}

function openEditBindingDialog(row) {
  bindingEditing.value = true
  editingBindingId.value = row.id
  bindingForm.name = row.name
  bindingForm.zoneId = row.zoneId
  bindingForm.cfCfgId = row.cfCfgId || null
  bindingForm.proxied = !!row.proxied
  bindingForm.ttl = row.ttl || 120
  bindingForm.enabled = row.enabled !== false
  bindingDialog.value = true
}

async function handleSaveBinding() {
  if (!currentInstanceID() || !bindingForm.name || !bindingForm.zoneId) {
    ElMessage.warning('DNS Name and Zone are required')
    return
  }
  savingBinding.value = true
  try {
    const payload = {
      instanceId: currentInstanceID(),
      tenantId: inst.value.tenantId || 0,
      name: bindingForm.name,
      zoneId: bindingForm.zoneId,
      cfCfgId: bindingForm.cfCfgId || 0,
      proxied: bindingForm.proxied,
      ttl: bindingForm.ttl || 120,
      enabled: bindingForm.enabled
    }
    if (bindingEditing.value) {
      await updateBinding(editingBindingId.value, payload)
    } else {
      await createBinding(payload)
    }
    ElMessage.success('Binding saved')
    bindingDialog.value = false
    await loadBindings()
  } catch (e) {
    ElMessage.error('Save binding failed: ' + (e.response?.data?.error || e.message))
  } finally {
    savingBinding.value = false
  }
}

async function handleDeleteBinding(row) {
  try {
    await ElMessageBox.confirm(
      `Delete binding "${row.name}"?`,
      'Confirm Delete',
      {
        confirmButtonText: 'Delete',
        cancelButtonText: 'Cancel',
        type: 'warning'
      }
    )
    await deleteBinding(row.id)
    ElMessage.success('Binding deleted')
    await loadBindings()
  } catch {
    // cancelled or error
  }
}
</script>

<style scoped>
.page-header { display:flex; align-items:center; gap:12px; margin-bottom:12px }
.page-header h3 { margin:0 }
.action-bar { display:flex; gap:8px; align-items:center; flex-wrap:wrap; margin-bottom:4px }
code { font-size:12px; word-break:break-all }
</style>
