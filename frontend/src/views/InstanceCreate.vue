<template>
  <div class="create-instance-page">
    <h3>Create Instance</h3>

    <el-form
      ref="formRef"
      :model="form"
      label-width="180px"
      @submit.prevent="handleLaunch"
    >
      <!-- Tenant Selector -->
      <el-form-item label="Tenant" required>
        <el-select
          v-model="form.tenantId"
          placeholder="Select Tenant..."
          @change="onTenantChange"
          :loading="loadingTenants"
          style="width: 400px"
        >
          <el-option
            v-for="t in tenants"
            :key="t.id"
            :label="t.name"
            :value="t.id"
          />
        </el-select>
      </el-form-item>

      <!-- Instance Name -->
      <el-form-item label="Instance Name" required>
        <el-input
          v-model="form.displayName"
          placeholder="Instance Name"
          style="width: 400px"
        />
      </el-form-item>

      <!-- Availability Domain -->
      <el-form-item label="Availability Domain" required>
        <el-select
          v-model="form.availabilityDomain"
          placeholder="Select AD..."
          :loading="loadingADs"
          :disabled="!form.tenantId"
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
          :disabled="!form.tenantId"
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
          :disabled="!form.tenantId"
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

      <!-- Submit -->
      <el-form-item>
        <el-button type="primary" :loading="launching" @click="handleLaunch">
          Launch
        </el-button>
        <el-button @click="$router.push('/instances')">
          Cancel
        </el-button>
      </el-form-item>
    </el-form>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { get, post } from '../api/index.js'

const router = useRouter()

// --- form state ---
const form = reactive({
  tenantId: '',
  displayName: '',
  availabilityDomain: '',
  imageId: '',
  shape: '',
  vcnId: '',
  subnetId: '',
  bootVolumeSizeGB: 50
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
const launching = ref(false)

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
  if (!form.tenantId) return
  loadingADs.value = true
  try {
    const res = await get('/availability-domains', { tenant_id: form.tenantId })
    ads.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load availability domains')
    ads.value = []
  }
  loadingADs.value = false
}

async function loadImages() {
  if (!form.tenantId) return
  loadingImages.value = true
  try {
    const res = await get('/images', { tenant_id: form.tenantId })
    images.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load images')
    images.value = []
  }
  loadingImages.value = false
}

async function loadShapes() {
  if (!form.tenantId || !form.imageId) return
  loadingShapes.value = true
  try {
    const res = await get('/shapes', {
      tenant_id: form.tenantId,
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
  if (!form.tenantId) return
  loadingVCNs.value = true
  try {
    const res = await get('/vcns', { tenant_id: form.tenantId })
    vcns.value = Array.isArray(res) ? res : []
  } catch (e) {
    ElMessage.error('Failed to load VCNs')
    vcns.value = []
  }
  loadingVCNs.value = false
}

async function loadSubnets() {
  if (!form.tenantId || !form.vcnId) return
  loadingSubnets.value = true
  try {
    const res = await get('/subnets', {
      tenant_id: form.tenantId,
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
  // Reset dependent fields
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

  if (!form.tenantId) return
  loadADs()
  loadImages()
  loadVCNs()
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

async function handleLaunch() {
  if (!form.tenantId || !form.displayName || !form.availabilityDomain || !form.imageId || !form.shape || !form.subnetId) {
    ElMessage.warning('Please fill in all required fields')
    return
  }

  launching.value = true
  try {
    const body = {
      tenantId: parseInt(form.tenantId),
      displayName: form.displayName,
      imageId: form.imageId,
      shape: form.shape,
      subnetId: form.subnetId,
      availabilityDomain: form.availabilityDomain
    }
    if (form.bootVolumeSizeGB) {
      body.bootVolumeSizeGB = form.bootVolumeSizeGB
    }
    await post('/instances', body)
    ElMessage.success('Instance launch request submitted')
    router.push('/instances')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to launch instance')
  }
  launching.value = false
}
</script>

<style scoped>
.create-instance-page {
  padding: 20px;
}

.create-instance-page h3 {
  margin-bottom: 24px;
  font-size: 20px;
  font-weight: 600;
}
</style>
