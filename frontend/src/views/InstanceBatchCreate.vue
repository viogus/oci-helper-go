<template>
  <div class="batch-create-page">
    <h3>Batch Create Instances</h3>

    <el-card shadow="never" class="form-card">
      <el-form
        ref="formRef"
        :model="form"
        label-width="180px"
        @submit.prevent="handleBatchCreate"
      >
        <!-- Tenant Selector (multiple) -->
        <el-form-item label="Tenants" required>
          <el-select
            v-model="form.tenantIds"
            multiple
            placeholder="Select tenants..."
            @change="onTenantChange"
            :loading="loadingTenants"
            style="width: 400px"
            collapse-tags
            collapse-tags-tooltip
          >
            <el-option
              v-for="t in tenants"
              :key="t.id"
              :label="t.name"
              :value="t.id"
            />
          </el-select>
          <el-tag
            v-if="form.tenantIds.length > 0"
            type="primary"
            size="small"
            style="margin-left: 8px"
          >
            {{ form.tenantIds.length }} selected
          </el-tag>
        </el-form-item>

        <!-- Instances Per Tenant -->
        <el-form-item label="Instances Per Tenant">
          <el-input-number
            v-model="form.instancesPerTenant"
            :min="1"
            :max="10"
            controls-position="right"
            style="width: 180px"
          />
          <span style="margin-left: 8px; color: var(--el-text-color-secondary); font-size: 13px;">
            Total: {{ form.instancesPerTenant * form.tenantIds.length }}
          </span>
        </el-form-item>

        <!-- Region -->
        <el-form-item label="Region">
          <el-input
            v-model="form.region"
            placeholder="e.g. us-ashburn-1"
            style="width: 400px"
          />
        </el-form-item>

        <!-- Availability Domain -->
        <el-form-item label="Availability Domain" required>
          <el-select
            v-model="form.availabilityDomain"
            placeholder="Select AD..."
            :loading="loadingADs"
            :disabled="form.tenantIds.length === 0"
            style="width: 400px"
          >
            <el-option
              v-for="ad in ads"
              :key="ad.name"
              :label="ad.name"
              :value="ad.name"
            />
          </el-select>
        </el-form-item>

        <!-- Image -->
        <el-form-item label="Image" required>
          <el-select
            v-model="form.imageId"
            placeholder="Select Image..."
            :loading="loadingImages"
            :disabled="form.tenantIds.length === 0"
            style="width: 400px"
            @change="onImageChange"
          >
            <el-option
              v-for="img in images"
              :key="img.id"
              :label="(img.displayName || '') + ' (' + (img.operatingSystem || '') + ')'"
              :value="img.id"
            />
          </el-select>
        </el-form-item>

        <!-- Shape -->
        <el-form-item label="Shape" required>
          <el-select
            v-model="form.shape"
            placeholder="Select Shape..."
            :loading="loadingShapes"
            :disabled="!form.imageId"
            style="width: 400px"
          >
            <el-option
              v-for="s in shapes"
              :key="s.shape"
              :label="s.shape"
              :value="s.shape"
            />
          </el-select>
        </el-form-item>

        <!-- VCN -->
        <el-form-item label="VCN">
          <el-select
            v-model="form.vcnId"
            placeholder="Select VCN..."
            :loading="loadingVCNs"
            :disabled="form.tenantIds.length === 0"
            style="width: 400px"
            @change="onVCNChange"
          >
            <el-option
              v-for="vcn in vcns"
              :key="vcn.id"
              :label="vcn.displayName || vcn.id"
              :value="vcn.id"
            />
          </el-select>
        </el-form-item>

        <!-- Subnet -->
        <el-form-item label="Subnet" required>
          <el-select
            v-model="form.subnetId"
            placeholder="Select Subnet..."
            :loading="loadingSubnets"
            :disabled="!form.vcnId"
            style="width: 400px"
          >
            <el-option
              v-for="sub in subnets"
              :key="sub.id"
              :label="sub.displayName || sub.id"
              :value="sub.id"
            />
          </el-select>
        </el-form-item>

        <!-- Boot Volume Size -->
        <el-form-item label="Boot Volume (GB)">
          <el-input-number
            v-model="form.bootVolumeSizeGB"
            :min="50"
            :max="2048"
            controls-position="right"
            style="width: 180px"
          />
          <span style="margin-left: 8px; color: var(--el-text-color-secondary); font-size: 13px;">
            (default: 50)
          </span>
        </el-form-item>

        <!-- Display Name Prefix -->
        <el-form-item label="Display Name Prefix">
          <el-input
            v-model="form.displayNamePrefix"
            placeholder="oci-helper"
            style="width: 400px"
          />
        </el-form-item>

        <!-- Submit -->
        <el-form-item>
          <el-button
            type="primary"
            :loading="submitting"
            @click="handleBatchCreate"
            :disabled="form.tenantIds.length === 0"
          >
            Start Batch Create
          </el-button>
          <el-button @click="$router.push('/create-tasks')">
            View Tasks
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { get } from '../api/index.js'
import { batchCreate } from '../api/tasks.js'

const router = useRouter()
const formRef = ref(null)

// --- form state ---
const form = reactive({
  tenantIds: [],
  instancesPerTenant: 1,
  region: '',
  availabilityDomain: '',
  imageId: '',
  shape: '',
  vcnId: '',
  subnetId: '',
  bootVolumeSizeGB: 50,
  displayNamePrefix: 'oci-helper'
})

// --- lists ---
const tenants = ref([])
const ads = ref([])
const images = ref([])
const shapes = ref([])
const vcns = ref([])
const subnets = ref([])

// --- loading flags ---
const loadingTenants = ref(false)
const loadingADs = ref(false)
const loadingImages = ref(false)
const loadingShapes = ref(false)
const loadingVCNs = ref(false)
const loadingSubnets = ref(false)
const submitting = ref(false)

// --- lifecycle ---
onMounted(() => {
  loadTenants()
})

// --- data loading ---
async function loadTenants() {
  loadingTenants.value = true
  try {
    const res = await get('/tenants')
    tenants.value = res.data || []
  } catch (e) {
    ElMessage.error('Failed to load tenants')
  }
  loadingTenants.value = false
}

async function loadADs() {
  if (form.tenantIds.length === 0) return
  loadingADs.value = true
  try {
    const res = await get('/availability-domains', { tenant_id: form.tenantIds[0] })
    ads.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load availability domains')
    ads.value = []
  }
  loadingADs.value = false
}

async function loadImages() {
  if (form.tenantIds.length === 0) return
  loadingImages.value = true
  try {
    const res = await get('/images', { tenant_id: form.tenantIds[0] })
    images.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load images')
    images.value = []
  }
  loadingImages.value = false
}

async function loadShapes() {
  if (form.tenantIds.length === 0 || !form.imageId) return
  loadingShapes.value = true
  try {
    const res = await get('/shapes', {
      tenant_id: form.tenantIds[0],
      image_id: form.imageId
    })
    shapes.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load shapes')
    shapes.value = []
  }
  loadingShapes.value = false
}

async function loadVCNs() {
  if (form.tenantIds.length === 0) return
  loadingVCNs.value = true
  try {
    const res = await get('/vcns', { tenant_id: form.tenantIds[0] })
    vcns.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load VCNs')
    vcns.value = []
  }
  loadingVCNs.value = false
}

async function loadSubnets() {
  if (!form.vcnId || form.tenantIds.length === 0) return
  loadingSubnets.value = true
  try {
    const res = await get('/subnets', {
      tenant_id: form.tenantIds[0],
      vcn_id: form.vcnId
    })
    subnets.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load subnets')
    subnets.value = []
  }
  loadingSubnets.value = false
}

// --- event handlers ---
async function onTenantChange() {
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

  if (form.tenantIds.length === 0) return
  loadADs()
  loadImages()
  loadVCNs()

  // Auto-fill region from first tenant if not already set
  if (!form.region) {
    const firstTenant = tenants.value.find(t => t.id === form.tenantIds[0])
    if (firstTenant?.region) {
      form.region = firstTenant.region
    }
  }
}

function onImageChange() {
  form.shape = ''
  shapes.value = []
  if (form.imageId) {
    loadShapes()
  }
}

async function onVCNChange() {
  form.subnetId = ''
  subnets.value = []
  if (form.vcnId) {
    loadSubnets()
  }
}

async function handleBatchCreate() {
  if (form.tenantIds.length === 0) {
    ElMessage.warning('Please select at least one tenant')
    return
  }
  if (!form.availabilityDomain) {
    ElMessage.warning('Please select an availability domain')
    return
  }
  if (!form.imageId) {
    ElMessage.warning('Please select an image')
    return
  }
  if (!form.shape) {
    ElMessage.warning('Please select a shape')
    return
  }
  if (!form.subnetId) {
    ElMessage.warning('Please select a subnet')
    return
  }

  submitting.value = true
  try {
    const body = {
      tenant_ids: form.tenantIds,
      instances_per_tenant: form.instancesPerTenant,
      region: form.region || undefined,
      shape: form.shape,
      image_id: form.imageId,
      subnet_id: form.subnetId,
      availability_domain: form.availabilityDomain
    }
    if (form.bootVolumeSizeGB) {
      body.boot_volume_size_gb = form.bootVolumeSizeGB
    }
    if (form.displayNamePrefix) {
      body.display_name_prefix = form.displayNamePrefix
    }
    const res = await batchCreate(body)
    const count = res.task_ids?.length || 0
    ElMessage.success(`Batch create submitted: ${count} task(s) created`)
    router.push('/create-tasks')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to submit batch create')
  }
  submitting.value = false
}
</script>

<style scoped>
.batch-create-page {
  padding: 20px;
}

.batch-create-page h3 {
  margin-bottom: 24px;
  font-size: 20px;
  font-weight: 600;
}

.form-card {
  max-width: 680px;
}
</style>
