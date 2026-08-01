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

      <el-divider direction="vertical" />

      <el-button @click="$router.push('/shell')">
        {{ $t('instanceDetail.cloudShell') }}
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
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { get, post } from '../api/index.js'
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
  } catch {}
  loading.value = false
})

async function doAction(action) {
  acting.value = action
  const id = decodeURIComponent(route.params.id)
  try {
    await instanceAction(id, action)
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
</script>

<style scoped>
.page-header { display:flex; align-items:center; gap:12px; margin-bottom:12px }
.page-header h3 { margin:0 }
.action-bar { display:flex; gap:8px; align-items:center; flex-wrap:wrap; margin-bottom:4px }
code { font-size:12px; word-break:break-all }
</style>
