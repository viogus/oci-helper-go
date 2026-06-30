<template>
  <div class="create-instance-page">
    <h3>创建实例</h3>

    <!-- Load from Plan -->
    <div v-if="availablePlans.length > 0" style="margin-bottom:16px">
      <el-select
        v-model="selectedPlanId"
        :placeholder="$t('instancePlans.loadFromPlan')"
        clearable
        @change="onPlanSelect"
        style="width: 320px"
      >
        <el-option
          v-for="p in availablePlans"
          :key="p.id"
          :label="p.name"
          :value="p.id"
        />
      </el-select>
    </div>

    <el-form
      ref="formRef"
      :model="form"
      label-width="180px"
      @submit.prevent="handleLaunch"
    >
      <!-- Tenant Selector -->
      <el-form-item label="租户" required>
        <el-select
          v-model="form.tenantId"
          placeholder="选择租户..."
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
      <el-form-item label="实例名称" required>
        <el-input
          v-model="form.displayName"
          placeholder="实例名称"
          style="width: 400px"
        />
      </el-form-item>

      <!-- Availability Domain -->
      <el-form-item label="可用性域" required>
        <el-select
          v-model="form.availabilityDomain"
          placeholder="选择可用性域..."
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
      <el-form-item label="镜像" required>
        <el-select
          v-model="form.imageId"
          placeholder="选择镜像..."
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
      <el-form-item label="规格" required>
        <el-select
          v-model="form.shape"
          placeholder="选择规格..."
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
          placeholder="选择 VCN..."
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
      <el-form-item label="子网" required>
        <el-select
          v-model="form.subnetId"
          placeholder="选择子网..."
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
      <el-form-item label="引导卷 (GB)">
        <el-input-number
          v-model="form.bootVolumeSizeGB"
          :min="50"
          :max="2048"
          controls-position="right"
          style="width: 180px"
        />
        <span style="margin-left: 8px; color: var(--el-text-color-secondary); font-size: 13px;">
          (默认: 50)
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
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { get, post } from '../api/index.js'

const router = useRouter()
const route = useRoute()

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

// --- plan support ---
const availablePlans = ref([])
const selectedPlanId = ref(null)

async function loadPlansForSelect() {
  try {
    const res = await get('/instance-plans')
    availablePlans.value = res.data || []
  } catch {}
}

function onPlanSelect(planId) {
  if (!planId) return
  const plan = availablePlans.value.find(p => p.id === planId)
  if (!plan) return
  // Pre-fill form fields from plan
  if (plan.tenant_id) {
    form.tenantId = String(plan.tenant_id)
    onTenantChange()
    // Defer dependent field population after tenant loads
    setTimeout(() => {
      if (plan.availability_domain) {
        form.availabilityDomain = plan.availability_domain
      }
      if (plan.image_id) {
        form.imageId = plan.image_id
      }
      if (plan.shape) {
        form.shape = plan.shape
      }
      if (plan.subnet_id) {
        form.subnetId = plan.subnet_id
      }
      form.bootVolumeSizeGB = plan.boot_volume_size_gb || 50
    }, 500)
  }
  if (plan.name) {
    form.displayName = plan.name
  }
  form.bootVolumeSizeGB = plan.boot_volume_size_gb || 50
}

// --- lifecycle ---
onMounted(async () => {
  loadTenants()
  await loadPlansForSelect()
  // Check for plan_id query param
  const planId = route.query.plan_id
  if (planId) {
    try {
      const res = await get('/instance-plans')
      const plans = res.data || []
      const plan = plans.find(p => p.id == planId)
      if (plan) {
        selectedPlanId.value = plan.id
        onPlanSelect(plan.id)
      }
    } catch {}
  }
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
    ElMessage.error('无法加载可用性域')
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
    ElMessage.error('无法加载镜像')
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
    ElMessage.error('无法加载规格')
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
    ElMessage.error('无法加载 VCN')
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
    ElMessage.error('无法加载子网')
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
    ElMessage.warning('请填写所有必填字段')
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
    ElMessage.success('实例创建请求已提交')
    router.push('/instances')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '创建实例失败')
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
