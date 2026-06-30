<template>
  <div class="defense-page">
    <div class="page-header">
      <h3>{{ $t('defense.title') }}</h3>
    </div>

    <!-- Tenant + VCN cascading selectors -->
    <div class="filter-bar">
      <el-select
        v-model="selectedTenantId"
        :placeholder="$t('tenant.title')"
        clearable
        @change="onTenantChange"
        :loading="tenantsLoading"
        style="width: 260px"
      >
        <el-option
          v-for="t in tenants"
          :key="t.id"
          :label="t.name"
          :value="t.id"
        />
      </el-select>
      <el-select
        v-model="selectedVcnId"
        :placeholder="$t('defense.selectVcn')"
        clearable
        @change="onVcnChange"
        :loading="vcnsLoading"
        :disabled="!selectedTenantId"
        style="width: 300px"
      >
        <el-option
          v-for="v in vcns"
          :key="v.id"
          :label="v.displayName || v.id"
          :value="v.id"
        />
      </el-select>
    </div>

    <!-- Status Banner -->
    <el-alert
      v-if="selectedVcnId && defenseActive"
      :title="`${$t('defense.active')} — ${blacklist.length} CIDR(s) blocked`"
      type="success"
      :closable="false"
      show-icon
      style="margin-bottom: 16px"
    />
    <el-alert
      v-if="selectedVcnId && !defenseActive"
      :title="$t('defense.inactive')"
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 16px"
    />

    <!-- Enable Section (when inactive and VCN selected) -->
    <div
      v-if="selectedVcnId && !defenseActive"
      class="defense-section"
    >
      <el-input
        v-model="cidrInput"
        type="textarea"
        :rows="4"
        :placeholder="$t('defense.cidrHint')"
        style="margin-bottom: 12px"
      />
      <el-button
        type="primary"
        :loading="enabling"
        :disabled="!cidrInput.trim()"
        @click="handleEnable"
      >
        {{ $t('defense.enable') }}
      </el-button>
    </div>

    <!-- Disable Section (when active) -->
    <div
      v-if="selectedVcnId && defenseActive"
      class="defense-section"
    >
      <el-button
        type="danger"
        :loading="disabling"
        @click="handleDisable"
      >
        {{ $t('defense.disable') }}
      </el-button>
    </div>

    <!-- Blacklist Table -->
    <div v-if="selectedVcnId && defenseActive" style="margin-top: 20px">
      <h4 style="margin-bottom: 12px; font-size: 15px; font-weight: 600;">
        {{ $t('defense.blacklist') }}
      </h4>
      <el-table
        :data="blacklist"
        stripe
        v-loading="blacklistLoading"
        :empty-text="$t('defense.blacklistEmpty')"
      >
        <el-table-column :label="$t('ipPool.cidr')" min-width="200">
          <template #default="{ row }">
            <code style="font-family: monospace;">{{ row.cidr }}</code>
          </template>
        </el-table-column>
        <el-table-column prop="label" :label="$t('defense.label')" min-width="160">
          <template #default="{ row }">
            {{ row.label || '-' }}
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-empty
      v-if="!selectedTenantId && !tenantsLoading"
      description="Select a tenant and VCN to manage defense"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
import { get, post } from '../api/index.js'

const tenants = ref([])
const selectedTenantId = ref('')
const vcns = ref([])
const selectedVcnId = ref('')
const tenantsLoading = ref(false)
const vcnsLoading = ref(false)

const defenseActive = ref(false)
const cidrInput = ref('')
const enabling = ref(false)
const disabling = ref(false)

const blacklist = ref([])
const blacklistLoading = ref(false)

onMounted(() => {
  loadTenants()
})

async function loadTenants() {
  tenantsLoading.value = true
  try {
    const res = await get('/tenants')
    tenants.value = res.data || []
  } catch (e) {
    ElMessage.error('Failed to load tenants')
  }
  tenantsLoading.value = false
}

function onTenantChange() {
  vcns.value = []
  selectedVcnId.value = ''
  defenseActive.value = false
  blacklist.value = []
  if (selectedTenantId.value) {
    loadVCNs()
  }
}

async function loadVCNs() {
  if (!selectedTenantId.value) return
  vcnsLoading.value = true
  try {
    const res = await get('/vcns', { tenant_id: selectedTenantId.value })
    vcns.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load VCNs')
    vcns.value = []
  }
  vcnsLoading.value = false
}

async function onVcnChange() {
  defenseActive.value = false
  blacklist.value = []
  cidrInput.value = ''
  if (!selectedVcnId.value) return
  // Check current defense status from config
  await checkDefenseStatus()
  if (defenseActive.value) {
    await loadBlacklist()
  }
}

async function checkDefenseStatus() {
  try {
    const cfg = await get('/config')
    if (cfg.defense_enabled && String(cfg.defense_vcn) === String(selectedVcnId.value)) {
      defenseActive.value = true
    } else {
      defenseActive.value = false
    }
  } catch (e) {
    defenseActive.value = false
  }
}

async function loadBlacklist() {
  if (!selectedTenantId.value) return
  blacklistLoading.value = true
  try {
    const res = await get('/ip-blacklist', { tenant_id: selectedTenantId.value })
    blacklist.value = res.data || []
  } catch (e) {
    blacklist.value = []
  }
  blacklistLoading.value = false
}

async function handleEnable() {
  const cidrs = cidrInput.value
    .split('\n')
    .map(line => line.trim())
    .filter(line => line.length > 0)
  if (cidrs.length === 0) {
    ElMessage.warning('Enter at least one CIDR')
    return
  }
  enabling.value = true
  try {
    await post('/defense/enable', {
      tenant_id: selectedTenantId.value,
      vcn_id: selectedVcnId.value,
      blacklist: cidrs
    })
    ElMessage.success('Defense enabled')
    defenseActive.value = true
    loadBlacklist()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to enable defense')
  }
  enabling.value = false
}

async function handleDisable() {
  try {
    await ElMessageBox.confirm(
      t('defense.confirmDisable'),
      t('defense.confirmDisableTitle'),
      {
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel'),
        type: 'warning'
      }
    )
    disabling.value = true
    await post('/defense/disable', {
      tenant_id: selectedTenantId.value,
      vcn_id: selectedVcnId.value
    })
    ElMessage.success('Defense disabled')
    defenseActive.value = false
    blacklist.value = []
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error(err.response?.data?.error || 'Failed to disable defense')
    }
  }
  disabling.value = false
}
</script>

<style scoped>
.defense-page {
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

.filter-bar {
  display: flex;
  gap: 12px;
  margin-bottom: 16px;
  align-items: center;
}

.defense-section {
  padding: 16px;
  background: var(--el-fill-color-light);
  border-radius: 8px;
}
</style>
