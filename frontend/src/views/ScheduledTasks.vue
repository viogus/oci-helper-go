<template>
  <div>
    <div class="page-header">
      <h3>定时开机任务</h3>
      <el-button type="primary" :icon="Plus" @click="openCreate">新建任务</el-button>
    </div>

    <el-card shadow="never">
      <el-table :data="tasks" v-loading="loading" stripe size="small">
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="tenantId" label="租户 ID" width="90" />
        <el-table-column prop="region" label="区域" width="150" />
        <el-table-column label="配置" width="180">
          <template #default="{ row }">
            {{ row.ocpus }}C / {{ row.memoryGB }}G / {{ row.disk }}G
          </template>
        </el-table-column>
        <el-table-column prop="architecture" label="架构" width="90" />
        <el-table-column prop="operationSystem" label="系统" width="120" />
        <el-table-column label="数量" width="70" prop="createNumbers" />
        <el-table-column label="间隔" width="90">
          <template #default="{ row }">{{ row.intervalSeconds }}s</template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.paused ? 'warning' : 'success'" size="small">
              {{ row.paused ? '已暂停' : '运行中' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <el-button v-if="!row.paused" size="small" @click="action(row.id, 'pause')">暂停</el-button>
            <el-button v-else size="small" type="primary" @click="action(row.id, 'resume')">恢复</el-button>
            <el-button size="small" type="danger" @click="action(row.id, 'stop')">停止</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-empty v-if="!loading && tasks.length === 0" description="暂无定时开机任务" />
    </el-card>

    <el-dialog v-model="createVisible" title="新建定时开机任务" width="520px">
      <el-form label-width="140px">
        <el-form-item label="租户" required>
          <el-select v-model="form.tenant_id" style="width: 100%">
            <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="区域">
          <el-input v-model="form.region" placeholder="留空使用租户默认区域" />
        </el-form-item>
        <el-form-item label="CPU/内存/磁盘">
          <div style="display:flex;gap:8px;width:100%">
            <el-input-number v-model="form.ocpus" :min="1" :max="64" />
            <el-input-number v-model="form.memory_gb" :min="1" :max="256" />
            <el-input-number v-model="form.disk" :min="50" :max="2048" />
          </div>
        </el-form-item>
        <el-form-item label="架构">
          <el-select v-model="form.architecture" style="width: 100%">
            <el-option label="AMD (E2.1.Micro)" value="AMD" />
            <el-option label="ARM (A1.Flex)" value="ARM" />
            <el-option label="AMD E5" value="AMD_E5" />
            <el-option label="ARM A2" value="ARM_A2" />
          </el-select>
        </el-form-item>
        <el-form-item label="系统">
          <el-input v-model="form.operation_system" placeholder="Ubuntu / Oracle Linux" />
        </el-form-item>
        <el-form-item label="数量/间隔(秒)">
          <div style="display:flex;gap:8px">
            <el-input-number v-model="form.create_numbers" :min="1" :max="100" />
            <el-input-number v-model="form.interval_seconds" :min="30" :max="86400" />
          </div>
        </el-form-item>
        <el-form-item label="Root 密码" required>
          <el-input v-model="form.root_password" type="password" show-password />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="submit">创建</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { get, post } from '../api/index.js'
import { listTenants } from '../api/tenants.js'

const tasks = ref([])
const tenants = ref([])
const loading = ref(false)
const saving = ref(false)
const createVisible = ref(false)
const form = reactive({
  tenant_id: null,
  region: '',
  ocpus: 1,
  memory_gb: 1,
  disk: 50,
  architecture: 'AMD',
  operation_system: 'Ubuntu',
  create_numbers: 1,
  interval_seconds: 60,
  root_password: ''
})

async function loadTasks() {
  loading.value = true
  try {
    const res = await get('/create-tasks/recurring')
    tasks.value = res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '加载失败')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, {
    tenant_id: tenants.value[0]?.id || null,
    region: '',
    ocpus: 1,
    memory_gb: 1,
    disk: 50,
    architecture: 'AMD',
    operation_system: 'Ubuntu',
    create_numbers: 1,
    interval_seconds: 60,
    root_password: ''
  })
  createVisible.value = true
}

async function submit() {
  if (!form.tenant_id || !form.root_password) {
    ElMessage.warning('租户和 Root 密码必填')
    return
  }
  saving.value = true
  try {
    await post('/create-tasks/recurring', { ...form })
    ElMessage.success('定时任务已创建')
    createVisible.value = false
    loadTasks()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  } finally {
    saving.value = false
  }
}

async function action(id, act) {
  if (act === 'stop') {
    try {
      await ElMessageBox.confirm('停止后任务将被删除，确认？', '提示', { type: 'warning' })
    } catch {
      return
    }
  }
  try {
    await post(`/create-tasks/recurring/${id}/${act}`)
    ElMessage.success('操作成功')
    loadTasks()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '操作失败')
  }
}

onMounted(async () => {
  loadTasks()
  try {
    const res = await listTenants()
    tenants.value = res.data || []
  } catch {}
})
</script>

<style scoped>
.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
</style>
