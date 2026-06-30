<template>
  <div class="users-page">
    <div class="page-header">
      <h3>{{ $t('users.title') }}</h3>
      <el-button type="primary" @click="handleAdd">
        <el-icon><Plus /></el-icon> {{ $t('users.addUser') }}
      </el-button>
    </div>

    <el-table
      :data="users"
      stripe
      v-loading="loading"
      :empty-text="$t('users.notFound')"
    >
      <el-table-column prop="username" :label="$t('users.username')" min-width="140" />
      <el-table-column prop="email" :label="$t('users.email')" min-width="200">
        <template #default="{ row }">
          {{ row.email || '-' }}
        </template>
      </el-table-column>
      <el-table-column :label="$t('users.role')" width="100">
        <template #default="{ row }">
          <el-tag :type="row.role === 'admin' ? 'primary' : 'info'" size="small">
            {{ row.role || 'user' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" :label="$t('common.createdAt')" width="170">
        <template #default="{ row }">
          {{ formatTime(row.created_at) }}
        </template>
      </el-table-column>
      <el-table-column :label="$t('users.actions')" width="260" fixed="right">
        <template #default="{ row }">
          <el-button
            type="primary"
            link
            size="small"
            @click="handleResetPassword(row)"
          >
            {{ $t('users.resetPassword') }}
          </el-button>
          <el-button
            type="warning"
            link
            size="small"
            @click="handleClearMFA(row)"
          >
            {{ $t('users.clearMFA') }}
          </el-button>
          <el-button
            type="danger"
            link
            size="small"
            @click="handleDelete(row)"
          >
            {{ $t('common.delete') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty
      v-if="!loading && users.length === 0"
      :description="$t('users.notFound')"
    />

    <!-- Add User Dialog -->
    <el-dialog
      v-model="dialogVisible"
      :title="$t('users.addTitle')"
      width="480px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form label-position="top">
        <el-form-item :label="$t('users.username')" required>
          <el-input v-model="form.username" :placeholder="$t('users.username')" />
        </el-form-item>
        <el-form-item :label="$t('users.password')" required>
          <el-input
            v-model="form.password"
            type="password"
            :placeholder="$t('users.password')"
            show-password
          />
        </el-form-item>
        <el-form-item :label="$t('users.email')">
          <el-input v-model="form.email" :placeholder="$t('users.email')" />
        </el-form-item>
        <el-form-item :label="$t('users.role')">
          <el-select v-model="form.role" style="width: 100%">
            <el-option label="admin" value="admin" />
            <el-option label="user" value="user" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">
          {{ $t('common.save') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- Reset Password Dialog -->
    <el-dialog
      v-model="resetPwdVisible"
      :title="$t('users.resetPasswordTitle')"
      width="420px"
      :close-on-click-modal="false"
    >
      <p style="margin-bottom: 12px; color: var(--el-text-color-secondary);">
        {{ $t('users.resetPasswordFor') }}: <strong>{{ resetPwdTarget?.username }}</strong>
      </p>
      <el-form label-position="top">
        <el-form-item :label="$t('users.newPassword')" required>
          <el-input
            v-model="resetPwdForm.password"
            type="password"
            :placeholder="$t('users.newPassword')"
            show-password
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="resetPwdVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="resettingPwd" @click="handleResetPasswordConfirm">
          {{ $t('common.confirm') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
import { get, post, del } from '../api/index.js'
import { useAuthStore } from '../stores/auth.js'

const auth = useAuthStore()

const users = ref([])
const loading = ref(false)
const saving = ref(false)
const dialogVisible = ref(false)

const form = reactive({
  username: '',
  password: '',
  email: '',
  role: 'user'
})

function resetForm() {
  form.username = ''
  form.password = ''
  form.email = ''
  form.role = 'user'
}

// Reset password state
const resetPwdVisible = ref(false)
const resetPwdTarget = ref(null)
const resetPwdForm = reactive({ password: '' })
const resettingPwd = ref(false)

function formatTime(v) {
  if (!v) return '-'
  return new Date(v).toLocaleString()
}

onMounted(() => {
  loadUsers()
})

async function loadUsers() {
  loading.value = true
  try {
    const res = await get('/users')
    users.value = res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to load users')
    users.value = []
  }
  loading.value = false
}

function handleAdd() {
  resetForm()
  dialogVisible.value = true
}

async function handleSave() {
  if (!form.username.trim() || !form.password.trim()) {
    ElMessage.warning(t('users.usernamePasswordRequired'))
    return
  }
  saving.value = true
  try {
    await post('/users', {
      username: form.username,
      password: form.password,
      email: form.email || undefined,
      role: form.role
    })
    ElMessage.success(t('users.userCreated'))
    dialogVisible.value = false
    loadUsers()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to create user')
  }
  saving.value = false
}

async function handleDelete(row) {
  if (auth.user?.name === row.username) {
    ElMessage.warning(t('users.cannotDeleteSelf'))
    return
  }
  const adminUsers = users.value.filter(u => u.role === 'admin')
  if (row.role === 'admin' && adminUsers.length <= 1) {
    ElMessage.warning(t('users.cannotDeleteLastAdmin'))
    return
  }
  try {
    await ElMessageBox.confirm(
      t('users.confirmDelete'),
      t('common.delete'),
      {
        confirmButtonText: t('common.delete'),
        cancelButtonText: t('common.cancel'),
        type: 'warning'
      }
    )
    await del(`/users/${row.id}`)
    ElMessage.success(t('users.userDeleted'))
    loadUsers()
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error(err.response?.data?.error || t('users.deleteFailed'))
    }
  }
}

function handleResetPassword(row) {
  resetPwdTarget.value = row
  resetPwdForm.password = ''
  resetPwdVisible.value = true
}

async function handleResetPasswordConfirm() {
  if (!resetPwdForm.password.trim()) {
    ElMessage.warning('New password is required')
    return
  }
  resettingPwd.value = true
  try {
    await del(`/users/${resetPwdTarget.value.id}/reset-password`, { data: { password: resetPwdForm.password } })
    ElMessage.success(t('users.passwordReset'))
    resetPwdVisible.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.error || 'Failed to reset password')
  }
  resettingPwd.value = false
}

async function handleClearMFA(row) {
  try {
    await ElMessageBox.confirm(
      t('users.confirmClearMFA'),
      t('users.clearMFA'),
      {
        confirmButtonText: t('common.confirm'),
        cancelButtonText: t('common.cancel'),
        type: 'warning'
      }
    )
    await del(`/users/${row.id}/mfa`)
    ElMessage.success(t('users.mfaCleared'))
  } catch (err) {
    if (err !== 'cancel') {
      ElMessage.error(err.response?.data?.error || 'Failed to clear MFA')
    }
  }
}
</script>

<style scoped>
.users-page {
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
</style>
