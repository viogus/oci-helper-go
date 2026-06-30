<template>
  <div class="instance-plans-page">
    <div class="page-header">
      <h3>{{ $t('instancePlans.title') }}</h3>
      <el-button type="primary" @click="handleAdd">
        <el-icon><Plus /></el-icon> {{ $t('instancePlans.add') }}
      </el-button>
    </div>

    <!-- Tenant Selector -->
    <div class="filter-bar">
      <el-select
        v-model="selectedTenantId"
        :placeholder="$t('tenant.title')"
        clearable
        @change="onTenantChange"
        :loading="tenantsLoading"
        style="width: 300px"
      >
        <el-option
          v-for="t in tenants"
          :key="t.id"
          :label="t.name"
          :value="t.id"
        />
      </el-select>
    </div>

    <!-- Card Grid -->
    <el-row :gutter="16" v-loading="loading">
      <el-col
        v-for="plan in plans"
        :key="plan.id"
        :span="8"
        style="margin-bottom: 16px"
      >
        <el-card shadow="hover" class="plan-card">
          <template #header>
            <div class="plan-card-header">
              <span class="plan-name">{{ plan.name }}</span>
              <div class="plan-actions">
                <el-button type="primary" link size="small" @click="handleEdit(plan)">
                  <el-icon><Edit /></el-icon>
                </el-button>
                <el-button type="danger" link size="small" @click="handleDelete(plan)">
                  <el-icon><Delete /></el-icon>
                </el-button>
              </div>
            </div>
          </template>
          <div class="plan-body">
            <div class="plan-specs">
              <div class="spec-item">
                <span class="spec-label">{{ $t('instancePlans.shape') }}</span>
                <span class="spec-value">{{ plan.shape || '-' }}</span>
              </div>
              <div class="spec-item">
                <span class="spec-label">{{ $t('instancePlans.ocpu') }}</span>
                <span class="spec-value">{{ plan.ocpus ?? '-' }}</span>
              </div>
              <div class="spec-item">
                <span class="spec-label">{{ $t('instancePlans.memoryGB') }}</span>
                <span class="spec-value">{{ plan.memory_gb ?? '-' }}</span>
              </div>
              <div class="spec-item">
                <span class="spec-label">{{ $t('instancePlans.bootGB') }}</span>
                <span class="spec-value">{{ plan.boot_volume_size_gb ?? '-' }}</span>
              </div>
            </div>
            <div class="plan-extra">
              <div class="extra-item">
                <span class="extra-label">{{ $t('instancePlans.image') }}</span>
                <code class="plan-code">{{ truncate(plan.image_id) }}</code>
              </div>
              <div class="extra-item">
                <span class="extra-label">{{ $t('instancePlans.subnet') }}</span>
                <code class="plan-code">{{ truncate(plan.subnet_id) }}</code>
              </div>
              <div class="extra-item">
                <span class="extra-label">{{ $t('instancePlans.ad') }}</span>
                <span>{{ plan.availability_domain || '-' }}</span>
              </div>
            </div>
            <el-button
              type="primary"
              style="width: 100%; margin-top: 12px"
              @click="$router.push('/instances/create?plan_id=' + plan.id)"
            >
              {{ $t('instancePlans.usePlan') }}
            </el-button>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-empty
      v-if="!loading && selectedTenantId && plans.length === 0"
      :description="$t('instancePlans.notFound')"
    />
    <el-empty
      v-if="!selectedTenantId && !tenantsLoading"
      description="Select a tenant to view instance plans"
    />

    <!-- Add / Edit Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? $t('instancePlans.editTitle') : $t('instancePlans.addTitle')"
      width="600px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form label-position="top">
        <el-form-item :label="$t('instancePlans.name')" required>
          <el-input v-model="form.name" :placeholder="$t('instancePlans.name')" />
        </el-form-item>
        <el-form-item :label="$t('tenant.title')">
          <el-select
            v-model="form.tenantId"
            placeholder="Select tenant..."
            @change="onFormTenantChange"
            :loading="tenantsLoading"
            style="width: 100%"
            :disabled="isEditing"
          >
            <el-option
              v-for="t in tenants"
              :key="t.id"
              :label="t.name"
              :value="t.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('instancePlans.ad')">
          <el-select
            v-model="form.availabilityDomain"
            placeholder="Select AD..."
            :loading="adsLoading"
            :disabled="!form.tenantId"
            style="width: 100%"
          >
            <el-option
              v-for="ad in ads"
              :key="ad.name"
              :label="ad.name"
              :value="ad.name"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('instancePlans.image')">
          <el-select
            v-model="form.imageId"
            placeholder="Select image..."
            :loading="imagesLoading"
            :disabled="!form.tenantId"
            style="width: 100%"
            @change="onFormImageChange"
          >
            <el-option
              v-for="img in images"
              :key="img.id"
              :label="(img.displayName || '') + ' (' + (img.operatingSystem || '') + ')'"
              :value="img.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('instancePlans.shape')">
          <el-select
            v-model="form.shape"
            placeholder="Select shape..."
            :loading="shapesLoading"
            :disabled="!form.imageId"
            style="width: 100%"
          >
            <el-option
              v-for="s in shapes"
              :key="s.shape"
              :label="s.shape"
              :value="s.shape"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="VCN">
          <el-select
            v-model="form.vcnId"
            placeholder="Select VCN..."
            :loading="vcnsLoading"
            :disabled="!form.tenantId"
            style="width: 100%"
            @change="onFormVCNChange"
          >
            <el-option
              v-for="vcn in vcns"
              :key="vcn.id"
              :label="vcn.displayName || vcn.id"
              :value="vcn.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('instancePlans.subnet')">
          <el-select
            v-model="form.subnetId"
            placeholder="Select subnet..."
            :loading="subnetsLoading"
            :disabled="!form.vcnId"
            style="width: 100%"
          >
            <el-option
              v-for="sub in subnets"
              :key="sub.id"
              :label="sub.displayName || sub.id"
              :value="sub.id"
            />
          </el-select>
        </el-form-item>
        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item :label="$t('instancePlans.ocpu')">
              <el-input-number
                v-model="form.ocpus"
                :min="1"
                controls-position="right"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item :label="$t('instancePlans.memoryGB')">
              <el-input-number
                v-model="form.memoryGB"
                :min="1"
                controls-position="right"
                style="width: 100%"
              />
            </el-form-item>
          </el-col>
        </el-row>
        <el-form-item :label="$t('instancePlans.bootGB')">
          <el-input-number
            v-model="form.bootVolumeSizeGB"
            :min="50"
            :max="2048"
            controls-position="right"
            style="width: 180px"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">
          {{ $t('common.save') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Edit, Delete } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
import { get, post, put, del } from '../api/index.js'

const tenants = ref([])
const selectedTenantId = ref('')
const plans = ref([])
const tenantsLoading = ref(false)
const loading = ref(false)
const saving = ref(false)

// Dialog cascading selects state
const dialogVisible = ref(false)
const isEditing = ref(false)
const editingId = ref(null)
const form = reactive({
  name: '',
  tenantId: '',
  availabilityDomain: '',
  imageId: '',
  shape: '',
  vcnId: '',
  subnetId: '',
  ocpus: 1,
  memoryGB: 1,
  bootVolumeSizeGB: 50
})

const ads = ref([])
const images = ref([])
const shapes = ref([])
const vcns = ref([])
const subnets = ref([])
const adsLoading = ref(false)
const imagesLoading = ref(false)
const shapesLoading = ref(false)
const vcnsLoading = ref(false)
const subnetsLoading = ref(false)

function truncate(val) {
  if (!val) return '-'
  return val.length > 30 ? val.substring(0, 27) + '...' : val
}

function resetForm() {
  form.name = ''
  form.tenantId = ''
  form.availabilityDomain = ''
  form.imageId = ''
  form.shape = ''
  form.vcnId = ''
  form.subnetId = ''
  form.ocpus = 1
  form.memoryGB = 1
  form.bootVolumeSizeGB = 50
  ads.value = []
  images.value = []
  shapes.value = []
  vcns.value = []
  subnets.value = []
}

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
  plans.value = []
  if (selectedTenantId.value) {
    loadPlans()
  }
}

async function loadPlans() {
  if (!selectedTenantId.value) return
  loading.value = true
  try {
    const res = await get('/instance-plans', { tenant_id: selectedTenantId.value })
    plans.value = res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to load plans')
    plans.value = []
  }
  loading.value = false
}

// Cascade loading for dialog
async function loadDialogADs() {
  if (!form.tenantId) return
  adsLoading.value = true
  try {
    const res = await get('/availability-domains', { tenant_id: form.tenantId })
    ads.value = Array.isArray(res) ? res : []
  } catch (e) {
    ads.value = []
  }
  adsLoading.value = false
}

async function loadDialogImages() {
  if (!form.tenantId) return
  imagesLoading.value = true
  try {
    const res = await get('/images', { tenant_id: form.tenantId })
    images.value = Array.isArray(res) ? res : []
  } catch (e) {
    images.value = []
  }
  imagesLoading.value = false
}

async function loadDialogShapes() {
  if (!form.tenantId || !form.imageId) return
  shapesLoading.value = true
  try {
    const res = await get('/shapes', { tenant_id: form.tenantId, image_id: form.imageId })
    shapes.value = Array.isArray(res) ? res : []
  } catch (e) {
    shapes.value = []
  }
  shapesLoading.value = false
}

async function loadDialogVCNs() {
  if (!form.tenantId) return
  vcnsLoading.value = true
  try {
    const res = await get('/vcns', { tenant_id: form.tenantId })
    vcns.value = Array.isArray(res) ? res : []
  } catch (e) {
    vcns.value = []
  }
  vcnsLoading.value = false
}

async function loadDialogSubnets() {
  if (!form.tenantId || !form.vcnId) return
  subnetsLoading.value = true
  try {
    const res = await get('/subnets', { tenant_id: form.tenantId, vcn_id: form.vcnId })
    subnets.value = Array.isArray(res) ? res : []
  } catch (e) {
    subnets.value = []
  }
  subnetsLoading.value = false
}

function onFormTenantChange() {
  form.availabilityDomain = ''
  form.imageId = ''
  form.shape = ''
  form.vcnId = ''
  form.subnetId = ''
  ads.value = []
  images.value = []
  shapes.value = []
  vcns.value = []
  subnets.value = []
  if (form.tenantId) {
    loadDialogADs()
    loadDialogImages()
    loadDialogVCNs()
  }
}

function onFormImageChange() {
  form.shape = ''
  shapes.value = []
  if (form.imageId) {
    loadDialogShapes()
  }
}

function onFormVCNChange() {
  form.subnetId = ''
  subnets.value = []
  if (form.vcnId) {
    loadDialogSubnets()
  }
}

function handleAdd() {
  isEditing.value = false
  editingId.value = null
  resetForm()
  dialogVisible.value = true
}

function handleEdit(plan) {
  isEditing.value = true
  editingId.value = plan.id
  form.name = plan.name || ''
  form.tenantId = plan.tenant_id || ''
  form.availabilityDomain = plan.availability_domain || ''
  form.imageId = plan.image_id || ''
  form.shape = plan.shape || ''
  form.vcnId = plan.vcn_id || plan.subnet?.vcn_id || ''
  form.subnetId = plan.subnet_id || ''
  form.ocpus = plan.ocpus || 1
  form.memoryGB = plan.memory_gb || 1
  form.bootVolumeSizeGB = plan.boot_volume_size_gb || 50
  dialogVisible.value = true
  // Pre-load cascading data for the edit tenant
  if (form.tenantId) {
    loadDialogADs()
    loadDialogImages()
    loadDialogVCNs()
    if (form.imageId) loadDialogShapes()
    if (form.vcnId) loadDialogSubnets()
  }
}

async function handleSave() {
  if (!form.name.trim()) {
    ElMessage.warning('Plan name is required')
    return
  }
  saving.value = true
  try {
    const body = {
      name: form.name,
      tenant_id: form.tenantId,
      availability_domain: form.availabilityDomain || undefined,
      image_id: form.imageId || undefined,
      shape: form.shape || undefined,
      subnet_id: form.subnetId || undefined,
      ocpus: form.ocpus,
      memory_gb: form.memoryGB,
      boot_volume_size_gb: form.bootVolumeSizeGB
    }
    if (isEditing.value) {
      await put(`/instance-plans/${editingId.value}`, body)
      ElMessage.success('Plan updated')
    } else {
      await post('/instance-plans', body)
      ElMessage.success('Plan created')
    }
    dialogVisible.value = false
    loadPlans()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to save plan')
  }
  saving.value = false
}

async function handleDelete(plan) {
  try {
    await ElMessageBox.confirm(
      t('instancePlans.confirmDelete'),
      t('common.delete'),
      {
        confirmButtonText: t('common.delete'),
        cancelButtonText: t('common.cancel'),
        type: 'warning'
      }
    )
    await del(`/instance-plans/${plan.id}`)
    ElMessage.success('Plan deleted')
    loadPlans()
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error(err.response?.data?.error || 'Delete failed')
    }
  }
}
</script>

<style scoped>
.instance-plans-page {
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
  margin-bottom: 20px;
}

.plan-card {
  border-radius: 8px;
}

.plan-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.plan-name {
  font-weight: 600;
  font-size: 15px;
}

.plan-actions {
  display: flex;
  gap: 4px;
}

.plan-body {
  font-size: 13px;
}

.plan-specs {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px;
}

.spec-item {
  display: flex;
  flex-direction: column;
}

.spec-label {
  color: var(--el-text-color-secondary);
  font-size: 11px;
  text-transform: uppercase;
}

.spec-value {
  font-weight: 500;
  margin-top: 2px;
}

.plan-extra {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.extra-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.extra-label {
  color: var(--el-text-color-secondary);
  font-size: 11px;
  text-transform: uppercase;
}

.plan-code {
  font-family: monospace;
  font-size: 11px;
  word-break: break-all;
  color: var(--el-color-primary);
}
</style>
