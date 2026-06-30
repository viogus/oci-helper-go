# Feature Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close feature parity gap between Java `Yohann0617/oci-helper` and Go `viogus/oci-helper-go` — 5 new frontend views, WebSocket log streaming, full Telegram bot expansion, backend unit tests.

**Architecture:** Frontend views follow existing Vue 3 + Element Plus + Pinia + Vue Router pattern. WebSocket logs reuse gorilla/websocket from Cloud Shell. Telegram expansion adds ~24 new callback handlers to existing handler_tgmenu.go. Tests use stdlib testing with httptest.Server + :memory: SQLite.

**Tech Stack:** Go 1.26 (net/http, gorilla/websocket, golang.org/x/crypto/ssh), Vue 3 (Composition API, Element Plus, Pinia, Vue Router, vue-i18n), SQLite (modernc.org/sqlite), OCI Go SDK v65

**Source spec:** `docs/superpowers/specs/2026-06-30-feature-parity-design.md`

---

## File Map

### New Files (12)
```
frontend/src/views/Users.vue
frontend/src/views/IpPool.vue
frontend/src/views/InstancePlans.vue
frontend/src/views/Defense.vue
frontend/src/views/SshKeys.vue
internal/handler/handler_wslog.go
internal/handler/test_helpers_test.go
internal/handler/handler_users_test.go
internal/handler/handler_ipdata_test.go
internal/handler/handler_instanceplans_test.go
internal/handler/handler_defense_test.go
internal/handler/handler_ssh_test.go
```

### Modified Files (10)
```
internal/handler/handler.go              — +1 route for /api/logs/ws
internal/handler/handler_tgmenu.go       — +~24 TG callback handlers
frontend/src/router/index.js             — +5 routes
frontend/src/components/AppLayout.vue    — +5 sidebar items
frontend/src/views/InstanceCreate.vue    — plan_id query param integration
frontend/src/views/Logs.vue              — WebSocket live tail mode
frontend/src/locales/en.json             — +~120 keys
frontend/src/locales/zh-CN.json          — +~120 keys
```

---

### Task 1: Branch and baseline

**Files:** None

- [ ] **Step 1: Create feature branch**

```bash
git checkout -b feature/java-parity && git status
```

Expected: `On branch feature/java-parity`, clean working tree.

- [ ] **Step 2: Verify current build**

```bash
cd frontend && npm run build && cd .. && CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server
```

Expected: Build succeeds, binary at `./oci-helper`.

---

### Task 2: Users.vue — User Management Page

**Files:**
- Create: `frontend/src/views/Users.vue`
- Modify: `frontend/src/router/index.js`
- Modify: `frontend/src/components/AppLayout.vue`
- Modify: `frontend/src/locales/en.json`
- Modify: `frontend/src/locales/zh-CN.json`

- [ ] **Step 1: Create Users.vue**

```vue
<template>
  <div class="users-page">
    <div class="page-header">
      <h3>{{ $t('users.title') }}</h3>
      <el-button type="primary" @click="handleAdd">
        <el-icon><Plus /></el-icon> {{ $t('users.addUser') }}
      </el-button>
    </div>

    <el-table :data="users" stripe v-loading="loading" :empty-text="$t('users.notFound')" border style="width: 100%">
      <el-table-column prop="username" :label="$t('users.username')" min-width="160" />
      <el-table-column prop="email" :label="$t('users.email')" min-width="200" />
      <el-table-column :label="$t('users.role')" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="row.role === 'admin' ? 'primary' : 'info'" size="small">
            {{ row.role || 'user' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="$t('users.actions')" width="280" fixed="right" align="center">
        <template #default="{ row }">
          <el-button type="warning" link size="small" @click="handleResetPassword(row)">
            {{ $t('users.resetPassword') }}
          </el-button>
          <el-button type="warning" link size="small" @click="handleClearMFA(row)">
            {{ $t('users.clearMFA') }}
          </el-button>
          <el-button type="danger" link size="small" @click="handleDelete(row)">
            {{ $t('common.delete') }}
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && users.length === 0" :description="$t('users.notFound')" />

    <!-- Add User Dialog -->
    <el-dialog v-model="addVisible" :title="$t('users.addTitle')" width="420px" :close-on-click-modal="false" @closed="resetAddForm">
      <el-form :model="addForm" label-width="100px">
        <el-form-item :label="$t('users.username')" required>
          <el-input v-model="addForm.username" maxlength="64" />
        </el-form-item>
        <el-form-item :label="$t('users.password')" required>
          <el-input v-model="addForm.password" type="password" show-password maxlength="128" />
        </el-form-item>
        <el-form-item :label="$t('users.email')">
          <el-input v-model="addForm.email" maxlength="128" />
        </el-form-item>
        <el-form-item :label="$t('users.role')">
          <el-select v-model="addForm.role" style="width: 100%">
            <el-option label="user" value="user" />
            <el-option label="admin" value="admin" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="addVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleSaveUser">{{ $t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- Reset Password Dialog -->
    <el-dialog v-model="pwdVisible" :title="$t('users.resetPasswordTitle')" width="380px" :close-on-click-modal="false">
      <p style="margin-bottom:12px">{{ $t('users.resetPasswordFor') }} <strong>{{ resetTarget?.username }}</strong></p>
      <el-input v-model="newPassword" type="password" show-password :placeholder="$t('users.newPassword')" maxlength="128" />
      <template #footer>
        <el-button @click="pwdVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleDoResetPassword">{{ $t('common.confirm') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { get, post, del } from '../api/index.js'
import { useAuthStore } from '../stores/auth.js'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const auth = useAuthStore()

const users = ref([])
const loading = ref(false)
const saving = ref(false)

// Add dialog
const addVisible = ref(false)
const addForm = reactive({ username: '', password: '', email: '', role: 'user' })
function resetAddForm() {
  addForm.username = ''
  addForm.password = ''
  addForm.email = ''
  addForm.role = 'user'
}

// Reset password
const pwdVisible = ref(false)
const resetTarget = ref(null)
const newPassword = ref('')

async function loadUsers() {
  loading.value = true
  try {
    const res = await get('/users')
    users.value = res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message)
  } finally {
    loading.value = false
  }
}

function handleAdd() {
  resetAddForm()
  addVisible.value = true
}

async function handleSaveUser() {
  if (!addForm.username.trim() || !addForm.password) {
    ElMessage.warning(t('users.username') + ' / ' + t('users.password') + ' required')
    return
  }
  saving.value = true
  try {
    await post('/users', { ...addForm })
    ElMessage.success('User created')
    addVisible.value = false
    await loadUsers()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message)
  } finally {
    saving.value = false
  }
}

function handleResetPassword(row) {
  resetTarget.value = row
  newPassword.value = ''
  pwdVisible.value = true
}

async function handleDoResetPassword() {
  if (!newPassword.value || newPassword.value.length < 6) {
    ElMessage.warning('Password must be at least 6 characters')
    return
  }
  saving.value = true
  try {
    await del('/users/' + resetTarget.value.id + '/reset-password', { password: newPassword.value })
    ElMessage.success('Password reset')
    pwdVisible.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message)
  } finally {
    saving.value = false
  }
}

async function handleClearMFA(row) {
  try {
    await ElMessageBox.confirm(
      t('users.confirmClearMFA'),
      t('users.clearMFA'),
      { confirmButtonText: t('common.confirm'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
    await del('/users/' + row.id + '/mfa')
    ElMessage.success('MFA cleared')
  } catch (err) {
    if (err !== 'cancel') ElMessage.error(err.response?.data?.error || err.message)
  }
}

async function handleDelete(row) {
  if (row.username === auth.user?.name) {
    ElMessage.warning(t('users.cannotDeleteSelf'))
    return
  }
  const admins = users.value.filter(u => u.role === 'admin')
  if (row.role === 'admin' && admins.length <= 1) {
    ElMessage.warning(t('users.cannotDeleteLastAdmin'))
    return
  }
  try {
    await ElMessageBox.confirm(
      t('users.confirmDelete'),
      t('common.delete'),
      { confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel'), type: 'warning' }
    )
    await del('/users/' + row.id)
    ElMessage.success('User deleted')
    await loadUsers()
  } catch (err) {
    if (err !== 'cancel') ElMessage.error(err.response?.data?.error || err.message)
  }
}

onMounted(() => loadUsers())
</script>

<style scoped>
.users-page { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-header h3 { margin: 0; font-size: 20px; font-weight: 600; }
</style>
```

- [ ] **Step 2: Add route in router/index.js**

In `/Users/cdf/Codes/oci-helper-go/frontend/src/router/index.js`, add before the closing `]` of the children array:

```js
  { path: 'users', name: 'Users', component: () => import('../views/Users.vue'), meta: { titleKey: 'route.users', icon: 'User' } },
```

- [ ] **Step 3: Add sidebar item in AppLayout.vue**

In `/Users/cdf/Codes/oci-helper-go/frontend/src/components/AppLayout.vue`, inside the System el-sub-menu, after the `<el-menu-item index="/tenants">` line:

```html
          <el-menu-item index="/users">{{ $t('menu.users') }}</el-menu-item>
```

- [ ] **Step 4: Add i18n keys — en.json**

In `/Users/cdf/Codes/oci-helper-go/frontend/src/locales/en.json`, add to `menu`:
```json
    "users": "Users",
```
Add to `route`:
```json
    "users": "Users",
```
Add new block after `inMemoryTasks`:
```json
  "users": {
    "title": "Users",
    "addUser": "Add User",
    "addTitle": "Add User",
    "username": "Username",
    "password": "Password",
    "email": "Email",
    "role": "Role",
    "actions": "Actions",
    "resetPassword": "Reset PW",
    "resetPasswordTitle": "Reset Password",
    "resetPasswordFor": "Reset password for",
    "newPassword": "New Password",
    "clearMFA": "Clear MFA",
    "confirmClearMFA": "Clear MFA for this user? They will need to re-enroll.",
    "confirmDelete": "Permanently delete this user?",
    "notFound": "No users found",
    "cannotDeleteSelf": "Cannot delete your own account",
    "cannotDeleteLastAdmin": "Cannot delete the last admin user"
  },
```

- [ ] **Step 5: Add i18n keys — zh-CN.json**

In `/Users/cdf/Codes/oci-helper-go/frontend/src/locales/zh-CN.json`, add to `menu`:
```json
    "users": "用户管理",
```
Add to `route`:
```json
    "users": "用户管理",
```
Add new block after `inMemoryTasks`:
```json
  "users": {
    "title": "用户管理",
    "addUser": "添加用户",
    "addTitle": "添加用户",
    "username": "用户名",
    "password": "密码",
    "email": "邮箱",
    "role": "角色",
    "actions": "操作",
    "resetPassword": "重置密码",
    "resetPasswordTitle": "重置密码",
    "resetPasswordFor": "重置密码 -",
    "newPassword": "新密码",
    "clearMFA": "清除 MFA",
    "confirmClearMFA": "确定清除此用户的 MFA 吗？需要重新绑定。",
    "confirmDelete": "确定永久删除此用户吗？",
    "notFound": "未找到用户",
    "cannotDeleteSelf": "不能删除自己的账户",
    "cannotDeleteLastAdmin": "不能删除最后一个管理员"
  },
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/views/Users.vue frontend/src/router/index.js frontend/src/components/AppLayout.vue frontend/src/locales/en.json frontend/src/locales/zh-CN.json
git commit -m "feat: add user management page (Users.vue)"
```

---

### Task 3: IpPool.vue — IP Pool Management Page

**Files:**
- Create: `frontend/src/views/IpPool.vue`
- Modify: `frontend/src/router/index.js`
- Modify: `frontend/src/components/AppLayout.vue`
- Modify: `frontend/src/locales/en.json`
- Modify: `frontend/src/locales/zh-CN.json`

- [ ] **Step 1: Create IpPool.vue**

The view follows the Audit.vue pattern but with tenant selector, type tabs, and CRUD dialogs. Create at `frontend/src/views/IpPool.vue`:

```vue
<template>
  <div class="ippool-page">
    <div class="page-header">
      <h3>{{ $t('ipPool.title') }}</h3>
    </div>

    <!-- Tenant selector -->
    <div class="filter-bar">
      <el-select v-model="tenantId" :placeholder="$t('instance.allTenants')" clearable @change="loadData" style="width: 200px">
        <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
      </el-select>
      <el-button type="primary" @click="handleAdd">{{ $t('ipPool.add') }}</el-button>
      <el-button @click="handleImportOCI" :loading="importing">{{ $t('ipPool.importOci') }}</el-button>
    </div>

    <!-- Type tabs -->
    <el-tabs v-model="activeType" @tab-change="loadData" style="margin-top: 8px">
      <el-tab-pane label="Pool" name="pool" />
      <el-tab-pane label="Whitelist" name="whitelist" />
      <el-tab-pane label="Blacklist" name="deny" />
    </el-tabs>

    <el-table :data="items" stripe v-loading="loading" :empty-text="$t('ipPool.notFound')" border style="width: 100%">
      <el-table-column prop="cidr" :label="$t('ipPool.cidr')" min-width="200">
        <template #default="{ row }"><code>{{ row.cidr }}</code></template>
      </el-table-column>
      <el-table-column prop="label" :label="$t('ipPool.label')" min-width="160" />
      <el-table-column :label="$t('ipPool.enabled')" width="90" align="center">
        <template #default="{ row }">
          <el-switch :model-value="row.enabled" @change="(v) => handleToggle(row, v)" size="small" />
        </template>
      </el-table-column>
      <el-table-column :label="$t('ipPool.actions')" width="120" align="center">
        <template #default="{ row }">
          <el-button type="primary" link size="small" @click="handleEdit(row)">{{ $t('common.edit') || 'Edit' }}</el-button>
          <el-button type="danger" link size="small" @click="handleDelete(row)">{{ $t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-empty v-if="!loading && items.length === 0 && tenantId" :description="$t('ipPool.notFound')" />
    <el-empty v-if="!tenantId" description="Select a tenant to view IP pool" />

    <!-- Add/Edit Dialog -->
    <el-dialog v-model="dialogVisible" :title="editingId ? $t('ipPool.editTitle') : $t('ipPool.addTitle')" width="420px" :close-on-click-modal="false">
      <el-form :model="form" label-width="80px">
        <el-form-item :label="$t('ipPool.cidr')" required>
          <el-input v-model="form.cidr" placeholder="e.g. 10.0.0.0/8" />
        </el-form-item>
        <el-form-item :label="$t('ipPool.label')">
          <el-input v-model="form.label" />
        </el-form-item>
        <el-form-item :label="$t('ipPool.type')">
          <el-select v-model="form.type" style="width: 100%">
            <el-option label="Pool" value="pool" />
            <el-option label="Whitelist" value="whitelist" />
            <el-option label="Blacklist" value="deny" />
          </el-select>
        </el-form-item>
        <el-form-item :label="$t('ipPool.enabled')">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">{{ $t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { get, post, put, del } from '../api/index.js'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()

const tenants = ref([])
const tenantId = ref(undefined)
const activeType = ref('pool')
const items = ref([])
const loading = ref(false)
const saving = ref(false)
const importing = ref(false)

const dialogVisible = ref(false)
const editingId = ref(null)
const form = reactive({ cidr: '', label: '', type: 'pool', enabled: true })

async function loadTenants() {
  try { const res = await get('/tenants'); tenants.value = res.data || [] } catch {}
}

function resetForm() {
  form.cidr = ''; form.label = ''; form.type = activeType.value; form.enabled = true
}

async function loadData() {
  if (!tenantId.value) { items.value = []; return }
  loading.value = true
  try {
    const res = await get('/ip-data', { tenant_id: tenantId.value, type: activeType.value })
    items.value = res.data || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message)
  } finally { loading.value = false }
}

function handleAdd() {
  if (!tenantId.value) { ElMessage.warning('Select a tenant first'); return }
  editingId.value = null
  resetForm()
  dialogVisible.value = true
}

function handleEdit(row) {
  editingId.value = row.id
  form.cidr = row.cidr; form.label = row.label || ''; form.type = row.type; form.enabled = row.enabled
  dialogVisible.value = true
}

async function handleSave() {
  if (!form.cidr.trim()) { ElMessage.warning('CIDR is required'); return }
  saving.value = true
  try {
    if (editingId.value) {
      await put('/ip-data/' + editingId.value, { ...form, tenant_id: tenantId.value })
    } else {
      await post('/ip-data', { ...form, tenant_id: tenantId.value })
    }
    ElMessage.success(editingId.value ? 'Updated' : 'Created')
    dialogVisible.value = false
    await loadData()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message)
  } finally { saving.value = false }
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(t('ipPool.confirmDelete'), t('common.delete'), { type: 'warning' })
    await del('/ip-data/' + row.id)
    ElMessage.success('Deleted')
    await loadData()
  } catch (err) { if (err !== 'cancel') ElMessage.error(err.response?.data?.error || err.message) }
}

async function handleToggle(row, val) {
  try {
    await put('/ip-data/' + row.id, { cidr: row.cidr, label: row.label, type: row.type, enabled: val, tenant_id: tenantId.value })
    row.enabled = val
  } catch (e) { ElMessage.error(e.response?.data?.error || e.message) }
}

async function handleImportOCI() {
  if (!tenantId.value) { ElMessage.warning('Select a tenant first'); return }
  importing.value = true
  try {
    const res = await post('/ip-data', { action: 'load_oci', tenant_id: tenantId.value })
    ElMessage.success(t('ipPool.importSuccess', { count: res.added || 0 }))
    await loadData()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || e.message)
  } finally { importing.value = false }
}

onMounted(() => { loadTenants() })
</script>

<style scoped>
.ippool-page { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-header h3 { margin: 0; font-size: 20px; font-weight: 600; }
.filter-bar { display: flex; gap: 12px; align-items: center; }
</style>
```

- [ ] **Step 2: Add route in router/index.js**

After the existing `/ips` route line, add:
```js
  { path: 'ip-pool', name: 'IpPool', component: () => import('../views/IpPool.vue'), meta: { titleKey: 'route.ipPool', icon: 'Connection' } },
```

- [ ] **Step 3: Add sidebar item in AppLayout.vue**

In Network submenu, after VCNs line:
```html
          <el-menu-item index="/ip-pool">{{ $t('menu.ipPool') }}</el-menu-item>
```

- [ ] **Step 4: Add i18n keys**

Add to en.json `menu`: `"ipPool": "IP Pool"`
Add to en.json `route`: `"ipPool": "IP Pool"`
Add to zh-CN.json `menu`: `"ipPool": "IP 池"`
Add to zh-CN.json `route`: `"ipPool": "IP 池"`

Add full `ipPool` block (see spec for all keys) to both en.json and zh-CN.json.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/IpPool.vue frontend/src/router/index.js frontend/src/components/AppLayout.vue frontend/src/locales/en.json frontend/src/locales/zh-CN.json
git commit -m "feat: add IP pool management page (IpPool.vue)"
```

---

### Task 4: InstancePlans.vue — Instance Plans Page

**Files:**
- Create: `frontend/src/views/InstancePlans.vue`
- Modify: `frontend/src/router/index.js`
- Modify: `frontend/src/components/AppLayout.vue`
- Modify: `frontend/src/views/InstanceCreate.vue`
- Modify: `frontend/src/locales/en.json`
- Modify: `frontend/src/locales/zh-CN.json`

- [ ] **Step 1: Create InstancePlans.vue**

Create at `frontend/src/views/InstancePlans.vue`. This view uses card grid (not table) to display plan templates.

```vue
<template>
  <div class="plans-page">
    <div class="page-header">
      <h3>{{ $t('instancePlans.title') }}</h3>
      <el-button type="primary" @click="handleAdd">
        <el-icon><Plus /></el-icon> {{ $t('instancePlans.add') }}
      </el-button>
    </div>

    <div class="filter-bar">
      <el-select v-model="tenantId" :placeholder="$t('instance.allTenants')" clearable @change="loadPlans" style="width: 200px">
        <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
      </el-select>
    </div>

    <el-empty v-if="!tenantId" description="Select a tenant to view plans" style="margin-top: 60px" />
    <el-empty v-if="tenantId && !loading && plans.length === 0" :description="$t('instancePlans.notFound')" />

    <el-row :gutter="20" v-loading="loading" v-if="plans.length > 0">
      <el-col :span="8" v-for="plan in plans" :key="plan.id" style="margin-bottom: 20px">
        <el-card shadow="hover">
          <template #header>
            <div class="plan-card-header">
              <span class="plan-name">{{ plan.name }}</span>
              <div class="plan-actions">
                <el-button type="primary" link size="small" @click="handleUse(plan)">{{ $t('instancePlans.usePlan') }}</el-button>
                <el-button link size="small" @click="handleEdit(plan)">Edit</el-button>
                <el-button type="danger" link size="small" @click="handleDelete(plan)">Delete</el-button>
              </div>
            </div>
          </template>
          <el-descriptions :column="2" size="small" border>
            <el-descriptions-item :label="$t('instancePlans.shape')">{{ plan.shape }}</el-descriptions-item>
            <el-descriptions-item :label="$t('instancePlans.ocpu')">{{ plan.ocpus }}</el-descriptions-item>
            <el-descriptions-item :label="$t('instancePlans.memoryGB')">{{ plan.memoryGB }} GB</el-descriptions-item>
            <el-descriptions-item :label="$t('instancePlans.bootGB')">{{ plan.bootVolumeSizeGB }} GB</el-descriptions-item>
            <el-descriptions-item :label="$t('instancePlans.ad')" :span="2">{{ plan.availabilityDomain }}</el-descriptions-item>
            <el-descriptions-item :label="$t('instancePlans.image')" :span="2">
              <span style="font-size:11px;font-family:monospace">{{ plan.imageId?.substring(0, 40) }}...</span>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
    </el-row>

    <!-- Add/Edit Dialog -->
    <el-dialog v-model="dialogVisible" :title="editingId ? $t('instancePlans.editTitle') : $t('instancePlans.addTitle')" width="580px" :close-on-click-modal="false">
      <el-form :model="form" label-width="140px">
        <el-form-item :label="$t('instancePlans.name')" required>
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="AD" required>
          <el-select v-model="form.ad" style="width: 100%" :disabled="!tenantId" @change="onADChange">
            <el-option v-for="a in ads" :key="a.name" :label="a.name" :value="a.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="Image" required>
          <el-select v-model="form.imageId" style="width: 100%" :disabled="!tenantId" filterable @change="onImageChange">
            <el-option v-for="img in images" :key="img.id" :label="img.displayName + ' (' + img.operatingSystem + ')'" :value="img.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="Shape" required>
          <el-select v-model="form.shape" style="width: 100%" :disabled="!form.imageId">
            <el-option v-for="s in shapes" :key="s.shape" :label="s.shape" :value="s.shape" />
          </el-select>
        </el-form-item>
        <el-form-item label="OCPU">
          <el-input-number v-model="form.ocpus" :min="1" :max="128" style="width: 180px" />
        </el-form-item>
        <el-form-item label="Memory GB">
          <el-input-number v-model="form.memoryGB" :min="1" :max="2048" style="width: 180px" />
        </el-form-item>
        <el-form-item label="VCN">
          <el-select v-model="form.vcnId" style="width: 100%" :disabled="!tenantId" @change="onVcnChange">
            <el-option v-for="v in vcns" :key="v.id" :label="v.displayName || v.id" :value="v.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="Subnet" required>
          <el-select v-model="form.subnetId" style="width: 100%" :disabled="!form.vcnId">
            <el-option v-for="s in subnets" :key="s.id" :label="s.displayName || s.id" :value="s.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="Boot Volume GB">
          <el-input-number v-model="form.bootGB" :min="50" :max="2048" style="width: 180px" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">{{ $t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { get, post, put, del } from '../api/index.js'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
const router = useRouter()

const tenants = ref([])
const tenantId = ref(undefined)
const plans = ref([])
const loading = ref(false)
const saving = ref(false)

const dialogVisible = ref(false)
const editingId = ref(null)
const ads = ref([]); const images = ref([]); const shapes = ref([]); const vcns = ref([]); const subnets = ref([])
const form = reactive({ name: '', ad: '', imageId: '', shape: '', ocpus: 1, memoryGB: 1, vcnId: '', subnetId: '', bootGB: 50 })

async function loadTenants() { try { const res = await get('/tenants'); tenants.value = res.data || [] } catch {} }
async function loadPlans() { /* GET /instance-plans?tenant_id=X */ }
async function loadADs() { /* GET /availability-domains?tenant_id=X */ }
async function loadImages() { /* GET /images?tenant_id=X */ }
async function loadShapes() { /* GET /shapes?tenant_id=X&image_id=Y */ }
async function loadVCNs() { /* GET /vcns?tenant_id=X */ }
async function loadSubnets() { /* GET /subnets?tenant_id=X&vcn_id=Y */ }

async function onADChange() { /* stub */ }
async function onImageChange() { form.shape = ''; await loadShapes() }
async function onVcnChange() { form.subnetId = ''; await loadSubnets() }

function resetForm() { /* reset all fields */ }

function handleAdd() {
  if (!tenantId.value) { ElMessage.warning('Select a tenant first'); return }
  editingId.value = null; resetForm()
  loadADs(); loadImages(); loadVCNs()
  dialogVisible.value = true
}

function handleEdit(plan) {
  editingId.value = plan.id
  form.name = plan.name; form.ad = plan.availabilityDomain
  form.imageId = plan.imageId; form.shape = plan.shape
  form.ocpus = plan.ocpus || 1; form.memoryGB = plan.memoryGB || 1
  form.bootGB = plan.bootVolumeSizeGB || 50; form.subnetId = plan.subnetId
  loadADs(); loadImages(); loadVCNs()
  dialogVisible.value = true
}

async function handleSave() {
  if (!form.name || !form.subnetId) { ElMessage.warning('Name and Subnet required'); return }
  saving.value = true
  const payload = { name: form.name, tenant_id: tenantId.value, shape: form.shape, image_id: form.imageId,
    subnet_id: form.subnetId, availability_domain: form.ad, boot_volume_size_gb: form.bootGB,
    ocpus: form.ocpus, memory_gb: form.memoryGB }
  try {
    if (editingId.value) { await put('/instance-plans/' + editingId.value, payload) }
    else { await post('/instance-plans', payload) }
    ElMessage.success(editingId.value ? 'Updated' : 'Created')
    dialogVisible.value = false; await loadPlans()
  } catch (e) { ElMessage.error(e.response?.data?.error || e.message) }
  finally { saving.value = false }
}

function handleUse(plan) { router.push('/instances/create?plan_id=' + plan.id) }

async function handleDelete(plan) {
  try {
    await ElMessageBox.confirm(t('instancePlans.confirmDelete'), t('common.delete'), { type: 'warning' })
    await del('/instance-plans/' + plan.id)
    ElMessage.success('Deleted'); await loadPlans()
  } catch (err) { if (err !== 'cancel') ElMessage.error(err.response?.data?.error || err.message) }
}

onMounted(() => { loadTenants() })
</script>
```

- [ ] **Step 2: Add route and sidebar**

Router: `{ path: 'instance-plans', name: 'InstancePlans', component: () => import('../views/InstancePlans.vue'), meta: { titleKey: 'route.instancePlans', icon: 'Tickets' } },`

Sidebar (Resources submenu): `<el-menu-item index="/instance-plans">{{ $t('menu.instancePlans') }}</el-menu-item>`

- [ ] **Step 3: Modify InstanceCreate.vue — plan_id integration**

In `InstanceCreate.vue` script setup, add after the `onMounted` block or as a new lifecycle hook:

```js
import { useRoute } from 'vue-router'
const route = useRoute()

// On mount, check for plan_id query parameter
onMounted(async () => {
  // ... existing mount logic ...
  const planId = route.query.plan_id
  if (planId) {
    try {
      const res = await get('/instance-plans', { tenant_id: undefined })  // get all plans
      const plans = res.data || []
      const plan = plans.find(p => p.id == planId)
      if (plan) {
        tenantId.value = plan.tenantId  // or plan.tenant_id
        displayName.value = plan.name
        availabilityDomain.value = plan.availabilityDomain
        imageId.value = plan.imageId
        shape.value = plan.shape
        subnetId.value = plan.subnetId
        bootVolumeSizeGB.value = plan.bootVolumeSizeGB || 50
        // Need to load VCN that contains the subnet — set vcnId if stored
      }
    } catch (e) { /* plan not found, ignore */ }
  }
})
```

Add a "Load from Plan" el-select at the top of the form (optional convenience):
```html
<el-select v-model="selectedPlanId" :placeholder="$t('instancePlans.loadFromPlan')" clearable @change="onPlanSelect" style="width: 280px; margin-bottom: 16px">
  <el-option v-for="p in availablePlans" :key="p.id" :label="p.name" :value="p.id" />
</el-select>
```

- [ ] **Step 4: Add i18n keys** for `instancePlans` block (see spec) to en.json and zh-CN.json.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/views/InstancePlans.vue frontend/src/views/InstanceCreate.vue frontend/src/router/index.js frontend/src/components/AppLayout.vue frontend/src/locales/en.json frontend/src/locales/zh-CN.json
git commit -m "feat: add instance plans page and plan_id integration in InstanceCreate"
```

---

### Task 5: Defense.vue — IP Defense Page

**Files:**
- Create: `frontend/src/views/Defense.vue`
- Modify: `frontend/src/router/index.js`
- Modify: `frontend/src/components/AppLayout.vue`
- Modify: `frontend/src/locales/en.json`
- Modify: `frontend/src/locales/zh-CN.json`

- [ ] **Step 1: Create Defense.vue**

Create at `frontend/src/views/Defense.vue`. This page manages enabling/disabling IP defense on a VCN and viewing the current blacklist.

```vue
<template>
  <div class="defense-page">
    <div class="page-header"><h3>{{ $t('defense.title') }}</h3></div>

    <div class="filter-bar">
      <el-select v-model="tenantId" :placeholder="$t('instance.allTenants')" clearable @change="onTenantChange" style="width: 200px">
        <el-option v-for="t in tenants" :key="t.id" :label="t.name" :value="t.id" />
      </el-select>
      <el-select v-model="vcnId" :placeholder="$t('defense.selectVcn')" :disabled="!tenantId" @change="onVcnChange" style="width: 280px">
        <el-option v-for="v in vcns" :key="v.id" :label="v.displayName || v.id" :value="v.id" />
      </el-select>
    </div>

    <!-- Status Banner -->
    <el-alert v-if="defenseActive" :title="$t('defense.active') + ' — ' + blockedCount + ' CIDR(s) blocked'" type="success" :closable="false" show-icon style="margin-bottom: 16px" />
    <el-alert v-else-if="vcnId" :title="$t('defense.inactive')" type="info" :closable="false" show-icon style="margin-bottom: 16px" />

    <!-- Enable Section -->
    <el-card v-if="vcnId && !defenseActive" style="margin-bottom: 16px">
      <template #header><strong>{{ $t('defense.enable') }}</strong></template>
      <el-input v-model="cidrText" type="textarea" :rows="4" :placeholder="$t('defense.cidrHint')" style="margin-bottom: 12px" />
      <el-button type="primary" :loading="enabling" @click="handleEnable">{{ $t('defense.enable') }}</el-button>
    </el-card>

    <!-- Disable Section -->
    <el-card v-if="vcnId && defenseActive" style="margin-bottom: 16px">
      <template #header><strong>{{ $t('defense.disable') }}</strong></template>
      <el-button type="danger" :loading="disabling" @click="handleDisable">{{ $t('defense.disable') }}</el-button>
    </el-card>

    <!-- Blacklist Table -->
    <el-card v-if="defenseActive">
      <template #header><strong>{{ $t('defense.blacklist') }}</strong></template>
      <el-table :data="blacklist" stripe border size="small" :empty-text="$t('defense.blacklistEmpty')">
        <el-table-column prop="cidr" label="CIDR"><template #default="{ row }"><code>{{ row.cidr }}</code></template></el-table-column>
        <el-table-column prop="label" :label="$t('defense.label') || 'Label'" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { get, post } from '../api/index.js'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()

const tenants = ref([])
const tenantId = ref(undefined)
const vcns = ref([])
const vcnId = ref('')
const defenseActive = ref(false)
const blockedCount = ref(0)
const blacklist = ref([])
const cidrText = ref('')
const enabling = ref(false)
const disabling = ref(false)

async function loadTenants() { try { const res = await get('/tenants'); tenants.value = res.data || [] } catch {} }

async function onTenantChange() {
  vcnId.value = ''; defenseActive.value = false
  if (!tenantId.value) return
  try { const res = await get('/vcns', { tenant_id: tenantId.value }); vcns.value = res.data || [] } catch {}
}

async function onVcnChange() {
  if (!vcnId.value) { defenseActive.value = false; return }
  try {
    const blRes = await get('/ip-blacklist', { tenant_id: tenantId.value })
    blacklist.value = blRes.data || []
    blockedCount.value = blacklist.value.length
    const cfg = await get('/config')
    defenseActive.value = cfg.defense_enabled === 'true' && cfg.defense_vcn === vcnId.value
  } catch {}
}

async function handleEnable() {
  const cidrs = cidrText.value.split('\n').map(s => s.trim()).filter(Boolean)
  if (cidrs.length === 0) { ElMessage.warning('Enter at least one CIDR'); return }
  enabling.value = true
  try {
    await post('/defense/enable', { tenant_id: tenantId.value, vcn_id: vcnId.value, blacklist: cidrs })
    ElMessage.success('Defense enabled')
    cidrText.value = ''
    await onVcnChange()
  } catch (e) { ElMessage.error(e.response?.data?.error || e.message) }
  finally { enabling.value = false }
}

async function handleDisable() {
  try {
    await ElMessageBox.confirm(t('defense.confirmDisable'), t('defense.confirmDisableTitle'), { type: 'warning' })
    disabling.value = true
    await post('/defense/disable', { tenant_id: tenantId.value, vcn_id: vcnId.value })
    ElMessage.success('Defense disabled')
    defenseActive.value = false
  } catch (err) {
    if (err !== 'cancel') ElMessage.error(err.response?.data?.error || err.message)
  } finally { disabling.value = false }
}

onMounted(() => { loadTenants() })
</script>

<style scoped>
.defense-page { padding: 20px; }
.page-header { margin-bottom: 16px; }
.page-header h3 { margin: 0; font-size: 20px; font-weight: 600; }
.filter-bar { display: flex; gap: 12px; margin-bottom: 16px; }
</style>
```

- [ ] **Step 2: Add route and sidebar**

Router: `{ path: 'defense', name: 'Defense', component: () => import('../views/Defense.vue'), meta: { titleKey: 'route.defense', icon: 'Lock' } },`

Sidebar (Network submenu): `<el-menu-item index="/defense">{{ $t('menu.defense') }}</el-menu-item>`

- [ ] **Step 3: Add i18n keys** for `defense` block to en.json and zh-CN.json.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/Defense.vue frontend/src/router/index.js frontend/src/components/AppLayout.vue frontend/src/locales/en.json frontend/src/locales/zh-CN.json
git commit -m "feat: add IP defense management page (Defense.vue)"
```

---

### Task 6: SshKeys.vue — SSH Key Management Page

**Files:**
- Create: `frontend/src/views/SshKeys.vue`
- Modify: `frontend/src/router/index.js`
- Modify: `frontend/src/components/AppLayout.vue`
- Modify: `frontend/src/locales/en.json`
- Modify: `frontend/src/locales/zh-CN.json`

- [ ] **Step 1: Create SshKeys.vue**

Create at `frontend/src/views/SshKeys.vue`. This page lists, generates, and deletes SSH keys.

Key differentiator from other CRUD views: the Generate dialog shows the public key in a readonly textarea with a copy button, and the private key warning.

```vue
<template>
  <div class="sshkeys-page">
    <div class="page-header">
      <h3>{{ $t('sshKeys.title') }}</h3>
      <div style="display:flex;gap:8px">
        <el-button type="primary" @click="handleGenerate">{{ $t('sshKeys.generate') }}</el-button>
        <el-button @click="$refs.uploadInput.click()">{{ $t('sshKeys.upload') }}</el-button>
        <input ref="uploadInput" type="file" accept=".pem" style="display:none" @change="onUploadPicked" />
      </div>
    </div>

    <el-table :data="keys" stripe v-loading="loading" :empty-text="$t('sshKeys.notFound')" border>
      <el-table-column prop="name" :label="$t('sshKeys.name')" min-width="160" />
      <el-table-column :label="$t('sshKeys.fingerprint')" width="240">
        <template #default="{ row }">
          <el-tooltip :content="row.fingerprint" placement="top">
            <code>{{ row.fingerprint?.substring(0, 24) }}...</code>
          </el-tooltip>
        </template>
      </el-table-column>
      <el-table-column :label="$t('sshKeys.type')" width="100" align="center">
        <template #default="{ row }">
          <el-tag :type="row.publicKey?.includes('RSA') ? 'primary' : 'success'" size="small">
            {{ row.publicKey?.includes('RSA') ? 'RSA' : 'ED25519' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="$t('sshKeys.publicKey')" min-width="200">
        <template #default="{ row }">
          <code style="font-size:11px">{{ row.publicKey?.substring(0, 60) }}...</code>
          <el-button link size="small" @click="handleCopyKey(row)">
            <el-icon><CopyDocument /></el-icon>
          </el-button>
        </template>
      </el-table-column>
      <el-table-column :label="$t('sshKeys.actions')" width="100" align="center">
        <template #default="{ row }">
          <el-button type="danger" link size="small" @click="handleDelete(row)">{{ $t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- Generate Dialog -->
    <el-dialog v-model="genVisible" :title="$t('sshKeys.generateTitle')" width="520px" :close-on-click-modal="false">
      <el-form :model="genForm" label-width="100px">
        <el-form-item :label="$t('sshKeys.name')" required>
          <el-input v-model="genForm.name" />
        </el-form-item>
        <el-form-item :label="$t('sshKeys.type')">
          <el-select v-model="genForm.keyType" style="width:100%">
            <el-option label="ED25519" value="ed25519" />
            <el-option label="RSA 4096" value="rsa" />
          </el-select>
        </el-form-item>
      </el-form>
      <div v-if="generatedKey" style="margin-top:12px">
        <el-alert :title="$t('sshKeys.keyWarning')" type="warning" :closable="false" show-icon style="margin-bottom:8px" />
        <el-input v-model="generatedKey" type="textarea" :rows="6" readonly />
        <el-button size="small" style="margin-top:8px" @click="handleCopyGenerated">{{ $t('sshKeys.copyKey') }}</el-button>
      </div>
      <template #footer>
        <el-button @click="genVisible = false">{{ $t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="handleDoGenerate" v-if="!generatedKey">{{ $t('sshKeys.generate') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { CopyDocument } from '@element-plus/icons-vue'
import { get, post, del, upload } from '../api/index.js'
import { useI18n } from 'vue-i18n'
const { t } = useI18n()

const keys = ref([])
const loading = ref(false)
const saving = ref(false)

// Generate dialog
const genVisible = ref(false)
const genForm = reactive({ name: '', keyType: 'ed25519' })
const generatedKey = ref('')

async function loadKeys() {
  loading.value = true
  try { const res = await get('/ssh/keys'); keys.value = res.data || [] } catch (e) { ElMessage.error(e.message) }
  finally { loading.value = false }
}

function handleGenerate() {
  genForm.name = ''; genForm.keyType = 'ed25519'; generatedKey.value = ''
  genVisible.value = true
}

async function handleDoGenerate() {
  if (!genForm.name.trim()) { ElMessage.warning('Name required'); return }
  saving.value = true
  try {
    const res = await post('/ssh/keys?action=generate', { name: genForm.name, key_type: genForm.keyType })
    generatedKey.value = res.public_key || res.publicKey || ''
    ElMessage.success('Key generated')
    await loadKeys()
  } catch (e) { ElMessage.error(e.response?.data?.error || e.message) }
  finally { saving.value = false }
}

async function handleCopyKey(row) {
  try { await navigator.clipboard.writeText(row.publicKey); ElMessage.success('Copied') }
  catch { ElMessage.warning('Failed to copy') }
}

async function handleCopyGenerated() {
  try { await navigator.clipboard.writeText(generatedKey.value); ElMessage.success('Copied') }
  catch { ElMessage.warning('Failed to copy') }
}

async function onUploadPicked(e) {
  const file = e.target.files[0]
  if (!file) return
  const fd = new FormData(); fd.append('files', file)
  try { await upload('/ssh/keys', fd); ElMessage.success('Uploaded'); await loadKeys() }
  catch (e) { ElMessage.error(e.response?.data?.error || e.message) }
  e.target.value = ''
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(t('sshKeys.confirmDelete'), t('common.delete'), { type: 'warning' })
    await del('/ssh/keys/' + row.id)
    ElMessage.success('Deleted')
    await loadKeys()
  } catch (err) { if (err !== 'cancel') ElMessage.error(err.response?.data?.error || err.message) }
}

onMounted(() => loadKeys())
</script>

<style scoped>
.sshkeys-page { padding: 20px; }
.page-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 16px; }
.page-header h3 { margin: 0; font-size: 20px; font-weight: 600; }
</style>
```

- [ ] **Step 2: Add route and sidebar**

Router: `{ path: 'ssh-keys', name: 'SshKeys', component: () => import('../views/SshKeys.vue'), meta: { titleKey: 'route.sshKeys', icon: 'Key' } },`

Sidebar (System submenu): `<el-menu-item index="/ssh-keys">{{ $t('menu.sshKeys') }}</el-menu-item>`

- [ ] **Step 3: Add i18n keys** for `sshKeys` block to en.json and zh-CN.json.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/SshKeys.vue frontend/src/router/index.js frontend/src/components/AppLayout.vue frontend/src/locales/en.json frontend/src/locales/zh-CN.json
git commit -m "feat: add SSH key management page (SshKeys.vue)"
```

---

### Task 7: WebSocket Log Streaming

**Files:**
- Create: `internal/handler/handler_wslog.go`
- Modify: `internal/handler/handler.go`
- Modify: `frontend/src/views/Logs.vue`
- Modify: `frontend/src/locales/en.json`
- Modify: `frontend/src/locales/zh-CN.json`

- [ ] **Step 1: Create handler_wslog.go**

Create at `internal/handler/handler_wslog.go`:

```go
package handler

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

var logWSUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type logWSMsg struct {
	Type string   `json:"type"`
	Data string   `json:"data,omitempty"`
	Time string   `json:"time,omitempty"`
	Lines []string `json:"lines,omitempty"`
}

func (s *Server) handleLogWS(w http.ResponseWriter, r *http.Request) {
	tail := 100
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			tail = n
		}
	}

	conn, err := logWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[wslog] upgrade error: %v", err)
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	logFile := s.cfg.LogFile
	f, err := os.Open(logFile)
	if err != nil {
		sendLogWS(conn, logWSMsg{Type: "error", Data: "Cannot open log file: " + err.Error()})
		return
	}
	defer f.Close()

	// Read last N lines as initial batch
	initLines := readLastNLines(f, tail)
	if len(initLines) > 0 {
		sendLogWS(conn, logWSMsg{Type: "init", Lines: initLines})
	}

	// Seek to end for live tail
	f.Seek(0, io.SeekEnd)
	lastSize, _ := f.Seek(0, io.SeekCurrent)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			fi, err := os.Stat(logFile)
			if err != nil {
				continue
			}
			if fi.Size() < lastSize {
				// File truncated/rotated
				sendLogWS(conn, logWSMsg{Type: "reset"})
				lastSize = 0
				f.Seek(0, io.SeekStart)
			}
			if fi.Size() > lastSize {
				f.Seek(lastSize, io.SeekStart)
				scanner := bufio.NewScanner(f)
				for scanner.Scan() {
					line := scanner.Text()
					sendLogWS(conn, logWSMsg{
						Type: "line",
						Data: line,
						Time: time.Now().Format(time.RFC3339),
					})
				}
				lastSize, _ = f.Seek(0, io.SeekCurrent)
			}
		}
	}
}

func sendLogWS(conn *websocket.Conn, msg logWSMsg) {
	data, _ := json.Marshal(msg)
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[wslog] write error: %v", err)
	}
}

func readLastNLines(f *os.File, n int) []string {
	f.Seek(0, io.SeekStart)
	var allLines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for long lines
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if len(allLines) <= n {
		return allLines
	}
	return allLines[len(allLines)-n:]
}
```

- [ ] **Step 2: Register route in handler.go**

In `routes()`, add after the `/api/logs` line (line 138):

```go
s.mux.HandleFunc("/api/logs/ws", s.withAuth(s.handleLogWS))
```

- [ ] **Step 3: Modify Logs.vue for live mode**

In `frontend/src/views/Logs.vue`, add to the header section:

```html
<div class="header-right" style="display:flex;gap:12px;align-items:center">
  <span v-if="liveActive" class="live-dot">●</span>
  <el-switch v-model="liveActive" @change="toggleLive" :active-text="$t('logs.live')" />
  <el-button @click="loadLogs" :loading="loading">{{ $t('logs.refresh') }}</el-button>
</div>
```

Add to script setup:

```js
const liveActive = ref(false)
let ws = null

function toggleLive(val) {
  if (val) {
    startLiveWS()
  } else {
    stopLiveWS()
  }
}

function startLiveWS() {
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const url = proto + '//' + location.host + '/api/logs/ws?tail=' + (lines.value.length || 100)
  ws = new WebSocket(url)
  ws.onmessage = (ev) => {
    try {
      const msg = JSON.parse(ev.data)
      if (msg.type === 'init') {
        lines.value = msg.lines || []
      } else if (msg.type === 'line') {
        lines.value.push(msg.data)
        if (lines.value.length > 5000) lines.value.shift()
      } else if (msg.type === 'reset') {
        lines.value = ['--- Log file rotated ---']
      }
      // Auto-scroll
      nextTick(() => {
        const el = document.querySelector('.log-viewport')
        if (el) el.scrollTop = el.scrollHeight
      })
    } catch {}
  }
  ws.onerror = () => { ElMessage.error(t('logs.liveError')); liveActive.value = false }
  ws.onclose = () => { if (liveActive.value) liveActive.value = false }
}

function stopLiveWS() {
  if (ws) { ws.close(); ws = null }
}

onBeforeUnmount(() => { stopLiveWS() })
```

Add scoped CSS:
```css
.live-dot { color: #67C23A; animation: pulse 1s infinite; }
@keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.3; } }
```

- [ ] **Step 4: Add i18n keys**: `logs.live`, `logs.liveError`, `logs.fileRotated` to en.json and zh-CN.json.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/handler_wslog.go internal/handler/handler.go frontend/src/views/Logs.vue frontend/src/locales/en.json frontend/src/locales/zh-CN.json
git commit -m "feat: add WebSocket log tail streaming (/api/logs/ws)"
```

---

### Task 8: Telegram Bot — Full Parity Expansion

**Files:**
- Modify: `internal/handler/handler_tgmenu.go`

- [ ] **Step 1: Expand main keyboard**

Replace `tgMainKeyboard()` (line 17-23) with 4-column layout:

```go
func tgMainKeyboard() telegram.InlineKeyboardMarkup {
	return telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🖥 Instances", CallbackData: "instances:0"}, {Text: "📋 Tasks", CallbackData: "tasks:0"}},
			{{Text: "📊 Status", CallbackData: "status"}, {Text: "❓ Help", CallbackData: "help"}},
			{{Text: "🛡 Defense", CallbackData: "defense"}, {Text: "🚫 Blacklist", CallbackData: "blacklist"}},
			{{Text: "🔑 SSH Keys", CallbackData: "sshkeys"}, {Text: "📌 Version", CallbackData: "version"}},
			{{Text: "💾 Backup", CallbackData: "backup"}, {Text: "📈 Traffic", CallbackData: "traffic"}},
			{{Text: "💿 Volumes", CallbackData: "volumes"}, {Text: "📋 Plans", CallbackData: "plans"}},
			{{Text: "📜 Logs", CallbackData: "logs"}, {Text: "💓 CheckAlive", CallbackData: "checkalive"}},
			{{Text: "⚙️ Configs", CallbackData: "cfg:list:0"}},
		},
	}
}
```

- [ ] **Step 2: Add new callback routes to handleTGCallback switch statement**

Add these cases to the `switch` block in `handleTGCallback` (after the existing `case "help":` block, line 153):

```go
	// --- Defense ---
	case action == "defense":
		s.tgDefenseMenu(bot, chatID, messageID)
	case action == "defense:enable":
		s.tgDefenseEnablePrompt(bot, chatID, messageID)
	case action == "defense:disable":
		s.tgDefenseDisableConfirm(bot, chatID, messageID)

	// --- Blacklist ---
	case action == "blacklist":
		s.tgBlacklistMenu(bot, chatID, messageID, 0)
	case action == "blacklist" && len(parts) >= 2:
		page, _ := strconv.Atoi(parts[1])
		s.tgBlacklistMenu(bot, chatID, messageID, page)
	case action == "blacklist:add":
		s.tgBlacklistAddPrompt(bot, chatID, messageID)
	case action == "blacklist:remove" && len(parts) >= 2:
		id, _ := strconv.ParseInt(parts[1], 10, 64)
		s.tgBlacklistRemoveID(bot, chatID, messageID, id)
	case action == "blacklist:clear":
		s.tgBlacklistClear(bot, chatID, messageID)

	// --- SSH Keys ---
	case action == "sshkeys":
		s.tgSSHKeysList(bot, chatID, messageID, 0)
	case action == "sshkeys" && len(parts) >= 2:
		page, _ := strconv.Atoi(parts[1])
		s.tgSSHKeysList(bot, chatID, messageID, page)
	case action == "sshkeys:generate":
		s.tgSSHKeyGenerate(bot, chatID, messageID)

	// --- Backup ---
	case action == "backup":
		s.tgBackupTrigger(bot, chatID, messageID)

	// --- Traffic ---
	case action == "traffic":
		s.tgTrafficChooseInstance(bot, chatID, messageID, 0)
	case action == "traffic:inst" && len(parts) >= 3:
		page, _ := strconv.Atoi(parts[1])
		s.tgTrafficChooseInstance(bot, chatID, messageID, page)
	case action == "traffic:query" && len(parts) >= 2:
		s.tgTrafficQuery(bot, chatID, messageID, parts[1])

	// --- Volumes ---
	case action == "volumes":
		s.tgVolumeList(bot, chatID, messageID, 0)
	case action == "volumes" && len(parts) >= 2:
		page, _ := strconv.Atoi(parts[1])
		s.tgVolumeList(bot, chatID, messageID, page)

	// --- Plans ---
	case action == "plans":
		s.tgPlansList(bot, chatID, messageID, 0)
	case action == "plans" && len(parts) >= 2:
		page, _ := strconv.Atoi(parts[1])
		s.tgPlansList(bot, chatID, messageID, page)

	// --- Logs ---
	case action == "logs":
		s.tgLogTail(bot, chatID, messageID)

	// --- Version ---
	case action == "version":
		s.tgVersionInfo(bot, chatID, messageID)

	// --- CheckAlive ---
	case action == "checkalive":
		s.tgCheckAlivePrompt(bot, chatID, messageID, 0)
	case action == "checkalive" && len(parts) >= 2:
		page, _ := strconv.Atoi(parts[1])
		s.tgCheckAlivePrompt(bot, chatID, messageID, page)
	case action == "checkalive:do" && len(parts) >= 2:
		s.tgCheckAliveDo(bot, chatID, messageID, parts[1])

	// --- Configs ---
	case action == "cfg:list":
		s.tgConfigList(bot, chatID, messageID, 0)
	case action == "cfg:list" && len(parts) >= 2:
		page, _ := strconv.Atoi(parts[1])
		s.tgConfigList(bot, chatID, messageID, page)
```

- [ ] **Step 3: Implement new handler functions**

Add these functions to `handler_tgmenu.go`. Each follows the same pattern as `tgSendInstanceList`/`tgSendStatus` — fetch data from `s.store`, format text, build keyboard, call `tgSend`.

**Defense handlers:**

```go
func (s *Server) tgDefenseMenu(bot *telegram.Bot, chatID int64, messageID int) {
	kb := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🛡 Enable", CallbackData: "defense:enable"}, {Text: "🚫 Disable", CallbackData: "defense:disable"}},
			{{Text: "🔙 Back", CallbackData: "main"}},
		},
	}
	tgSend(bot, chatID, messageID, "🛡 Defense Mode\n\nEnable: Block specified CIDRs via security list rules.\nDisable: Restore allow-all rule.", &kb)
}

func (s *Server) tgDefenseEnablePrompt(bot *telegram.Bot, chatID int64, messageID int) {
	// List tenants → user picks one → ask for CIDR list
	tenants, _ := s.store.ListTenants()
	// For simplicity: use first tenant or prompt
	text := "To enable defense, use the web UI (/defense).\nProvide: tenant, VCN, and CIDR blacklist (one per line)."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgDefenseDisableConfirm(bot *telegram.Bot, chatID int64, messageID int) {
	text := "To disable defense, use the web UI (/defense).\nThis restores the allow-all ingress rule."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**Blacklist handlers:**

```go
func (s *Server) tgBlacklistMenu(bot *telegram.Bot, chatID int64, messageID int, page int) {
	// List deny-type IP data across all tenants
	allData, _ := s.store.ListIpData(0, "deny")
	kb := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🔙 Back", CallbackData: "main"}},
		},
	}
	if len(allData) == 0 {
		tgSend(bot, chatID, messageID, "🚫 Blacklist\n\nNo blocked IPs.", &kb)
		return
	}
	text := fmt.Sprintf("🚫 Blacklist (%d entries)\n\n", len(allData))
	for _, d := range allData {
		text += fmt.Sprintf("• %s", d.CIDR)
		if d.Label != "" { text += fmt.Sprintf(" (%s)", d.Label) }
		text += "\n"
	}
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgBlacklistAddPrompt(bot *telegram.Bot, chatID int64, messageID int) {
	text := "To add IPs to blacklist, use the web UI (/ip-pool)."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgBlacklistRemoveID(bot *telegram.Bot, chatID int64, messageID int, id int64) {
	s.store.DeleteIpData(id)
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, "IP entry removed.", &kb)
}

func (s *Server) tgBlacklistClear(bot *telegram.Bot, chatID int64, messageID int) {
	allData, _ := s.store.ListIpData(0, "deny")
	for _, d := range allData { s.store.DeleteIpData(d.ID) }
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, fmt.Sprintf("Cleared %d blacklist entries.", len(allData)), &kb)
}
```

**SSH Keys handler:**

```go
func (s *Server) tgSSHKeysList(bot *telegram.Bot, chatID int64, messageID int, page int) {
	keys, _ := s.store.ListSSHKeys(0)
	kb := tgMainKeyboard()
	if len(keys) == 0 {
		tgSend(bot, chatID, messageID, "🔑 SSH Keys\n\nNo keys found.", &kb)
		return
	}
	text := fmt.Sprintf("🔑 SSH Keys (%d)\n\n", len(keys))
	for _, k := range keys {
		fp := k.Fingerprint
		if len(fp) > 16 { fp = fp[:16] + "..." }
		keyType := "ED25519"
		if strings.Contains(k.PublicKey, "RSA") { keyType = "RSA" }
		text += fmt.Sprintf("• %s (%s) — %s\n", k.Name, keyType, fp)
	}
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgSSHKeyGenerate(bot *telegram.Bot, chatID int64, messageID int) {
	text := "To generate SSH keys, use the web UI (/ssh-keys)."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**Backup handler:**

```go
func (s *Server) tgBackupTrigger(bot *telegram.Bot, chatID int64, messageID int) {
	text := "To create/restore backups, use the web UI (/backup)."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**Traffic handler:**

```go
func (s *Server) tgTrafficChooseInstance(bot *telegram.Bot, chatID int64, messageID int, page int) {
	instances, _ := s.store.ListInstances(0)
	kb := tgInstanceKeyboard(instanceSliceToShort(instances), page, (len(instances)+7)/8)
	tgSend(bot, chatID, messageID, "📈 Traffic — Select an instance:", &kb)
}

func (s *Server) tgTrafficQuery(bot *telegram.Bot, chatID int64, messageID int, instanceID string) {
	inst, err := s.store.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "Instance not found.", &kb)
		return
	}
	tenant, _ := s.store.GetTenant(inst.TenantID)
	if tenant == nil { return }
	client, err := s.clientFor(tenant)
	if err != nil { return }
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Hour)
	vnicTraffic, err := client.GetVNICTtraffic(ctx, tenant.TenancyOCID, inst.ID, startTime, endTime)
	if err != nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, fmt.Sprintf("Traffic query failed: %v", err), &kb)
		return
	}
	text := fmt.Sprintf("📈 Traffic — %s (last hour)\n\n", inst.Name)
	if len(vnicTraffic) > 0 {
		last := vnicTraffic[len(vnicTraffic)-1]
		text += fmt.Sprintf("Bytes In:  %.1f KB/s\nBytes Out: %.1f KB/s\nPoints: %d",
			last.BytesInPerSec/1024, last.BytesOutPerSec/1024, len(vnicTraffic))
	} else {
		text += "No traffic data."
	}
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**Helper needed for instance list conversion:**

```go
func instanceSliceToShort(instances []db.Instance) []tgInstanceShort {
	var items []tgInstanceShort
	for _, inst := range instances {
		items = append(items, tgInstanceShort{ID: inst.ID, Name: inst.Name})
	}
	return items
}
```

**Volumes handler:**

```go
func (s *Server) tgVolumeList(bot *telegram.Bot, chatID int64, messageID int, page int) {
	tenants, _ := s.store.ListTenants()
	var allVolumes []db.BootVolume
	for _, t := range tenants {
		client, err := s.clientFor(&t)
		if err != nil { continue }
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		vols, _ := client.ListBootVolumes(ctx, t.TenancyOCID)
		cancel()
		// Convert core.BootVolume to simplified display — show name + size
		for _, v := range vols {
			allVolumes = append(allVolumes, db.BootVolume{
				ID:          *v.Id,
				DisplayName: strOrNil(v.DisplayName),
				SizeInGBs:   int64(strOrNilF64(v.SizeInGBs)),
				State:       string(v.LifecycleState),
			})
		}
	}
	kb := tgMainKeyboard()
	if len(allVolumes) == 0 {
		tgSend(bot, chatID, messageID, "💿 Boot Volumes\n\nNo volumes found. Sync tenants first.", &kb)
		return
	}
	text := fmt.Sprintf("💿 Boot Volumes (%d)\n\n", len(allVolumes))
	show := allVolumes
	if len(show) > 15 { show = show[:15] }
	for _, v := range show {
		text += fmt.Sprintf("• %s — %d GB [%s]\n", v.DisplayName, v.SizeInGBs, v.State)
	}
	if len(allVolumes) > 15 { text += fmt.Sprintf("\n... and %d more", len(allVolumes)-15) }
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**Helper functions needed:**

```go
func strOrNil(s *string) string {
	if s == nil { return "" }
	return *s
}

func strOrNilF64(f *float64) float64 {
	if f == nil { return 0 }
	return *f
}
```

**Plans handler:**

```go
func (s *Server) tgPlansList(bot *telegram.Bot, chatID int64, messageID int, page int) {
	plans, _ := s.store.ListInstancePlans(0)
	kb := tgMainKeyboard()
	if len(plans) == 0 {
		tgSend(bot, chatID, messageID, "📋 Instance Plans\n\nNo plans found.", &kb)
		return
	}
	text := fmt.Sprintf("📋 Instance Plans (%d)\n\n", len(plans))
	for _, p := range plans {
		text += fmt.Sprintf("• %s — %s | OCPU:%.0f Mem:%.0fGB Boot:%dGB\n", p.Name, p.Shape, p.OCPUs, p.MemoryGB, p.BootVolumeSizeGB)
	}
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**Logs handler:**

```go
func (s *Server) tgLogTail(bot *telegram.Bot, chatID int64, messageID int) {
	f, err := os.Open(s.cfg.LogFile)
	if err != nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, fmt.Sprintf("Cannot open log: %v", err), &kb)
		return
	}
	defer f.Close()
	lines := readLastNLines(f, 20)
	text := "📜 Recent Logs (last 20 lines)\n\n"
	if len(lines) == 0 {
		text += "(empty)"
	} else {
		for _, l := range lines {
			if len(l) > 80 { l = l[:80] + "..." }
			text += l + "\n"
		}
	}
	text = strings.TrimRight(text, "\n")
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**Version handler:**

```go
func (s *Server) tgVersionInfo(bot *telegram.Bot, chatID int64, messageID int) {
	text := "📌 oci-helper-go\n\nVersion: latest\nRepo: github.com/viogus/oci-helper-go\nTech: Go 1.26 + Vue 3 + SQLite"
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**CheckAlive handlers:**

```go
func (s *Server) tgCheckAlivePrompt(bot *telegram.Bot, chatID int64, messageID int, page int) {
	instances, _ := s.store.ListInstances(0)
	kb := tgInstanceKeyboard(instanceSliceToShort(instances), page, (len(instances)+7)/8)
	tgSend(bot, chatID, messageID, "💓 Check Alive — Select an instance:", &kb)
}

func (s *Server) tgCheckAliveDo(bot *telegram.Bot, chatID int64, messageID int, instanceID string) {
	inst, err := s.store.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "Instance not found.", &kb)
		return
	}
	alive := inst.PublicIP != "" && checkTCPPort(inst.PublicIP, 22, 5*time.Second)
	status := "❌ DEAD"
	if alive { status = "✅ ALIVE" }
	text := fmt.Sprintf("💓 Check Alive\n\n%s (%s)\nStatus: %s\nIP: %s",
		inst.Name, inst.Shape, status, strOr(&inst.PublicIP, "N/A"))
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}
```

**Helper function for TCP check:**

```go
func checkTCPPort(addr string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, strconv.Itoa(port)), timeout)
	if err != nil { return false }
	conn.Close()
	return true
}
```

**Configs handler:**

```go
func (s *Server) tgConfigList(bot *telegram.Bot, chatID int64, messageID int, page int) {
	tenants, _ := s.store.ListTenants()
	kb := tgMainKeyboard()
	if len(tenants) == 0 {
		tgSend(bot, chatID, messageID, "⚙️ Tenant Configs\n\nNo tenants configured.", &kb)
		return
	}
	text := fmt.Sprintf("⚙️ Tenant Configs (%d)\n\n", len(tenants))
	for _, t := range tenants {
		status := "✅"
		if t.Status == "error" { status = "❌" }
		text += fmt.Sprintf("%s %s — %s\n", status, t.Name, t.Region)
	}
	tgSend(bot, chatID, messageID, text, &kb)
}
```

- [ ] **Step 4: Add imports to handler_tgmenu.go**

Add these imports at the top:
```go
import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/telegram"
)
```

- [ ] **Step 5: Fix callback routing order**

The existing switch statement in `handleTGCallback` uses `action == "instances" && len(parts) >= 1` which shadows `parts[0] == "instances" && len(parts) >= 3 && parts[1] == "detail"`. The Go switch evaluates cases in order, and the first matching case wins. The existing code works because it uses `case action == "instances" && len(parts) >= 1:` BEFORE the `case parts[0] == "instances" && ...` cases.

For the new handlers, need to ensure ordering:
- Generic cases like `action == "blacklist"` come BEFORE specific cases like `action == "blacklist:add"`
- Cases with `len(parts)` checks come after exact matches

This is already handled correctly in the code shown above because:
- `case action == "blacklist":` matches exactly "blacklist" (no colons in `:add`)
- `case action == "blacklist:add":` has colons, so `action` won't be "blacklist"

Wait — actually, `action` is `parts[0]` from `strings.SplitN(data, ":", 4)`. So for callback data "blacklist", action = "blacklist". For "blacklist:add", action = "blacklist" and parts[1] = "add".

Let me fix the case logic. The issue is:
- `case action == "blacklist":` matches BOTH "blacklist" AND "blacklist:..." callbacks since action is always just the first part.

Correct approach: check `data` directly or check `len(parts)`:

```go
case action == "blacklist" && len(parts) == 1:
    s.tgBlacklistMenu(bot, chatID, messageID, 0)
case action == "blacklist" && len(parts) >= 2 && parts[1] == "add":
    s.tgBlacklistAddPrompt(bot, chatID, messageID)
case action == "blacklist" && len(parts) >= 3 && parts[1] == "remove":
    id, _ := strconv.ParseInt(parts[2], 10, 64)
    s.tgBlacklistRemoveID(bot, chatID, messageID, id)
// etc.
```

- [ ] **Step 6: Commit**

```bash
git add internal/handler/handler_tgmenu.go
git commit -m "feat: expand Telegram bot to full parity — 20+ new handlers (defense, blacklist, SSH keys, backup, traffic, volumes, plans, logs, version, checkalive, configs)"
```

---

### Task 9: Tests — Test Helpers

**Files:**
- Create: `internal/handler/test_helpers_test.go`

- [ ] **Step 1: Create test helpers**

```go
package handler

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/viogus/oci-helper-go/internal/auth"
	"github.com/viogus/oci-helper-go/internal/config"
	"github.com/viogus/oci-helper-go/internal/db"
)

func setupTestServer(t *testing.T) (*Server, *db.Store, func()) {
	t.Helper()

	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}

	cfg := &config.Config{
		Username: "admin",
		Password: "test123",
		DBPath:   ":memory:",
		KeysDir:  "/tmp/oci-test-keys",
	}

	authSvc, err := auth.New(cfg.Username, cfg.Password, cfg.MFASecret, cfg.MFA)
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	srv := New(cfg, store, authSvc)
	cleanup := func() {
		store.Close()
	}

	return srv, store, cleanup
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return b
}

func mustLogin(t *testing.T, srv *Server) string {
	t.Helper()

	body := mustJSON(t, map[string]string{"username": "admin", "password": "test123"})
	req := httptest.NewRequest("POST", "/api/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == "oci_session" {
			return c.Value
		}
	}
	t.Fatal("login failed: no session cookie returned")
	return ""
}

func authedReq(t *testing.T, srv *Server, method, path string, body []byte, sessionCookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: "oci_session", Value: sessionCookie})
	}
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	return w
}

func seedTenant(t *testing.T, store *db.Store) *db.Tenant {
	t.Helper()
	tenant := &db.Tenant{
		Name:        "test-tenant",
		Region:      "us-ashburn-1",
		TenancyOCID: "ocid1.tenancy.test",
		UserOCID:    "ocid1.user.test",
		Fingerprint: "aa:bb:cc",
		KeyFile:     "test.pem",
		Status:      "active",
	}
	err := store.CreateTenant(tenant)
	if err != nil {
		t.Fatalf("failed to seed tenant: %v", err)
	}
	// Re-fetch to get the ID
	tenants, _ := store.ListTenants()
	if len(tenants) > 0 {
		return &tenants[len(tenants)-1]
	}
	return tenant
}
```

Add import for `net/http`:
```go
import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/viogus/oci-helper-go/internal/auth"
	"github.com/viogus/oci-helper-go/internal/config"
	"github.com/viogus/oci-helper-go/internal/db"
)
```

- [ ] **Step 2: Commit**

```bash
git add internal/handler/test_helpers_test.go
git commit -m "test: add test helpers (setupTestServer, authedReq, seedTenant)"
```

---

### Task 10: Tests — Users

**Files:**
- Create: `internal/handler/handler_users_test.go`

- [ ] **Step 1: Create handler_users_test.go**

```go
package handler

import (
	"encoding/json"
	"testing"
)

func TestHandleUsers_List(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	w := authedReq(t, srv, "GET", "/api/users", nil, cookie)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	// Default admin user should exist
	if len(resp.Data) < 1 {
		t.Error("expected at least 1 user (admin)")
	}
}

func TestHandleUsers_Create(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]interface{}{
		"username": "testuser",
		"password": "secret123",
		"email":    "test@example.com",
		"role":     "user",
	})
	w := authedReq(t, srv, "POST", "/api/users", body, cookie)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify user appears in list
	w2 := authedReq(t, srv, "GET", "/api/users", nil, cookie)
	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w2.Body.Bytes(), &resp)
	found := false
	for _, u := range resp.Data {
		if u["username"] == "testuser" {
			found = true
			break
		}
	}
	if !found {
		t.Error("created user not found in list")
	}
}

func TestHandleUsers_Create_MissingUsername(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]string{"password": "secret123"})
	w := authedReq(t, srv, "POST", "/api/users", body, cookie)
	if w.Code >= 200 && w.Code < 300 {
		t.Error("expected error for missing username")
	}
}

func TestHandleUsers_Create_Duplicate(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]string{"username": "dupuser", "password": "pw123"})
	authedReq(t, srv, "POST", "/api/users", body, cookie)
	w := authedReq(t, srv, "POST", "/api/users", body, cookie)
	if w.Code >= 200 && w.Code < 300 {
		t.Error("expected error for duplicate username")
	}
}

func TestHandleUserByID_Delete(t *testing.T) {
	srv, store, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	// Create a user first
	body := mustJSON(t, map[string]string{"username": "todelete", "password": "pw"})
	authedReq(t, srv, "POST", "/api/users", body, cookie)

	// Find user ID
	w := authedReq(t, srv, "GET", "/api/users", nil, cookie)
	var resp struct{ Data []map[string]interface{} `json:"data"` }
	json.Unmarshal(w.Body.Bytes(), &resp)
	var userID float64
	for _, u := range resp.Data {
		if u["username"] == "todelete" {
			userID = u["id"].(float64)
			break
		}
	}

	w2 := authedReq(t, srv, "DELETE", "/api/users/"+formatInt64(int64(userID)), nil, cookie)
	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	_ = store // suppress unused warning
}

func TestHandleUserByID_ResetPassword(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	// Create user
	authedReq(t, srv, "POST", "/api/users", mustJSON(t, map[string]string{"username": "pwreset", "password": "old"}), cookie)

	// Find ID
	w := authedReq(t, srv, "GET", "/api/users", nil, cookie)
	var resp struct{ Data []map[string]interface{} `json:"data"` }
	json.Unmarshal(w.Body.Bytes(), &resp)
	var userID float64
	for _, u := range resp.Data {
		if u["username"] == "pwreset" {
			userID = u["id"].(float64)
			break
		}
	}

	body := mustJSON(t, map[string]string{"password": "newpw123"})
	w2 := authedReq(t, srv, "DELETE", "/api/users/"+formatInt64(int64(userID))+"/reset-password", body, cookie)
	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestHandleUserByID_InvalidID(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	w := authedReq(t, srv, "DELETE", "/api/users/0", nil, cookie)
	if w.Code < 400 {
		t.Error("expected error for invalid ID")
	}
}

func formatInt64(n int64) string {
	if n == 0 { return "0" }
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
```

Wait — `formatInt64` is overcomplicated. Simpler:

```go
func formatInt64(n int64) string {
	// Simple conversion since we can't use strconv.FormatInt
	result := ""
	if n == 0 { return "0" }
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
```

Actually, just import `strconv` and use `strconv.FormatInt`:

```go
import (
	"encoding/json"
	"strconv"
	"testing"
)
```

- [ ] **Step 2: Run users tests**

```bash
go test -v -count=1 -run TestHandleUsers ./internal/handler/...
```

Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/handler/handler_users_test.go
git commit -m "test: add user management handler tests"
```

---

### Task 11: Tests — IP Data, Instance Plans, Defense, SSH

**Files:**
- Create: `internal/handler/handler_ipdata_test.go`
- Create: `internal/handler/handler_instanceplans_test.go`
- Create: `internal/handler/handler_defense_test.go`
- Create: `internal/handler/handler_ssh_test.go`

- [ ] **Step 1: Create handler_ipdata_test.go**

Tests for: list (all, by tenant, by type), create (success, missing CIDR), update, delete, load_oci.

```go
package handler

import (
	"encoding/json"
	"testing"
)

func TestHandleIpData_List(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)
	w := authedReq(t, srv, "GET", "/api/ip-data", nil, cookie)
	if w.Code != 200 { t.Fatalf("expected 200, got %d", w.Code) }
}

func TestHandleIpData_List_ByType(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)
	w := authedReq(t, srv, "GET", "/api/ip-data?type=pool", nil, cookie)
	if w.Code != 200 { t.Fatalf("expected 200, got %d", w.Code) }
}

func TestHandleIpData_Create(t *testing.T) {
	srv, store, cleanup := setupTestServer(t)
	defer cleanup()
	_ = store

	// Seed a tenant
	tenant := seedTenant(t, store)
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]interface{}{
		"tenant_id": tenant.ID,
		"cidr":      "10.0.0.0/8",
		"label":     "test-pool",
		"type":      "pool",
		"enabled":   true,
	})
	w := authedReq(t, srv, "POST", "/api/ip-data", body, cookie)
	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["cidr"] != "10.0.0.0/8" {
		t.Error("CIDR mismatch")
	}
}

func TestHandleIpData_Create_MissingCIDR(t *testing.T) {
	srv, store, cleanup := setupTestServer(t)
	defer cleanup()
	tenant := seedTenant(t, store)
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]interface{}{"tenant_id": tenant.ID, "type": "pool"})
	w := authedReq(t, srv, "POST", "/api/ip-data", body, cookie)
	if w.Code >= 200 && w.Code < 300 {
		t.Error("expected error for missing CIDR")
	}
}

func TestHandleIpDataByID_Update(t *testing.T) {
	srv, store, cleanup := setupTestServer(t)
	defer cleanup()
	tenant := seedTenant(t, store)
	cookie := mustLogin(t, srv)

	// Create first
	createBody := mustJSON(t, map[string]interface{}{
		"tenant_id": tenant.ID, "cidr": "192.168.0.0/16", "type": "pool", "enabled": true,
	})
	w := authedReq(t, srv, "POST", "/api/ip-data", createBody, cookie)
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := int64(created["id"].(float64))

	// Update
	updateBody := mustJSON(t, map[string]interface{}{
		"cidr": "172.16.0.0/12", "label": "updated", "type": "whitelist", "enabled": false,
	})
	w2 := authedReq(t, srv, "PUT", "/api/ip-data/"+strconv.FormatInt(id, 10), updateBody, cookie)
	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestHandleIpDataByID_Delete(t *testing.T) {
	srv, store, cleanup := setupTestServer(t)
	defer cleanup()
	tenant := seedTenant(t, store)
	cookie := mustLogin(t, srv)

	createBody := mustJSON(t, map[string]interface{}{
		"tenant_id": tenant.ID, "cidr": "10.0.0.0/24", "type": "pool", "enabled": true,
	})
	w := authedReq(t, srv, "POST", "/api/ip-data", createBody, cookie)
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := int64(created["id"].(float64))

	w2 := authedReq(t, srv, "DELETE", "/api/ip-data/"+strconv.FormatInt(id, 10), nil, cookie)
	if w2.Code != 200 {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
}
```

- [ ] **Step 2: Create handler_instanceplans_test.go**

```go
package handler

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestHandleInstancePlans_List(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)
	w := authedReq(t, srv, "GET", "/api/instance-plans", nil, cookie)
	if w.Code != 200 { t.Fatalf("expected 200, got %d", w.Code) }
}

func TestHandleInstancePlans_Create(t *testing.T) {
	srv, store, cleanup := setupTestServer(t)
	defer cleanup()
	tenant := seedTenant(t, store)
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]interface{}{
		"name":             "test-plan",
		"tenant_id":        tenant.ID,
		"shape":            "VM.Standard.E5.Flex",
		"image_id":         "ocid1.image.test",
		"subnet_id":        "ocid1.subnet.test",
		"availability_domain": "AD-1",
		"boot_volume_size_gb": int64(100),
		"ocpus":            float64(2),
		"memory_gb":        float64(16),
	})
	w := authedReq(t, srv, "POST", "/api/instance-plans", body, cookie)
	if w.Code != 200 { t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String()) }
}

func TestHandleInstancePlanByID_Delete(t *testing.T) {
	srv, store, cleanup := setupTestServer(t)
	defer cleanup()
	tenant := seedTenant(t, store)
	cookie := mustLogin(t, srv)

	createBody := mustJSON(t, map[string]interface{}{
		"name": "to-delete", "tenant_id": tenant.ID, "shape": "VM.Standard.E5.Flex",
		"image_id": "ocid1.image.test", "subnet_id": "ocid1.subnet.test",
		"availability_domain": "AD-1", "boot_volume_size_gb": int64(50),
	})
	w := authedReq(t, srv, "POST", "/api/instance-plans", createBody, cookie)
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := int64(created["id"].(float64))

	w2 := authedReq(t, srv, "DELETE", "/api/instance-plans/"+strconv.FormatInt(id, 10), nil, cookie)
	if w2.Code != 200 { t.Fatalf("expected 200, got %d", w2.Code) }
}
```

- [ ] **Step 3: Create handler_defense_test.go**

Tests for: enable, disable, list blacklist. Note: defense operations require OCI client which needs a real tenant config. These are integration-level tests that will fail in unit test mode. We test the HTTP-level behavior (auth check, parameter validation) and note that full end-to-end requires OCI credentials.

```go
package handler

import (
	"testing"
)

func TestHandleDefense_Enable_MissingParams(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]interface{}{})
	w := authedReq(t, srv, "POST", "/api/defense/enable", body, cookie)
	if w.Code >= 200 && w.Code < 300 {
		t.Error("expected error for missing tenant_id")
	}
}

func TestHandleDefense_Disable_MissingParams(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]interface{}{})
	w := authedReq(t, srv, "POST", "/api/defense/disable", body, cookie)
	if w.Code >= 200 && w.Code < 300 {
		t.Error("expected error for missing tenant_id")
	}
}

func TestHandleIPBlacklist_List(t *testing.T) {
	srv, store, cleanup := setupTestServer(t)
	defer cleanup()
	tenant := seedTenant(t, store)
	cookie := mustLogin(t, srv)

	w := authedReq(t, srv, "GET", "/api/ip-blacklist?tenant_id="+formatInt64(tenant.ID), nil, cookie)
	if w.Code != 200 { t.Fatalf("expected 200, got %d", w.Code) }
}
```

- [ ] **Step 4: Create handler_ssh_test.go**

```go
package handler

import (
	"testing"
)

func TestHandleSSHKeys_List(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	w := authedReq(t, srv, "GET", "/api/ssh/keys", nil, cookie)
	if w.Code != 200 { t.Fatalf("expected 200, got %d", w.Code) }
}

func TestHandleSSHKeys_Generate(t *testing.T) {
	srv, _, cleanup := setupTestServer(t)
	defer cleanup()
	cookie := mustLogin(t, srv)

	body := mustJSON(t, map[string]string{"name": "test-key", "key_type": "ed25519"})
	w := authedReq(t, srv, "POST", "/api/ssh/keys?action=generate", body, cookie)
	if w.Code != 200 { t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String()) }
}
```

- [ ] **Step 5: Run all tests**

```bash
go test -v -count=1 ./internal/handler/...
```

- [ ] **Step 6: Commit**

```bash
git add internal/handler/handler_ipdata_test.go internal/handler/handler_instanceplans_test.go internal/handler/handler_defense_test.go internal/handler/handler_ssh_test.go
git commit -m "test: add handler tests for ip-data, instance-plans, defense, SSH keys"
```

---

### Task 12: Final Build Verification

- [ ] **Step 1: Frontend build**

```bash
cd frontend && npm run build && cd ..
```

Expected: Build succeeds, new chunks in `internal/handler/dist/`:
- `Users-*.js`
- `IpPool-*.js`
- `InstancePlans-*.js`
- `Defense-*.js`
- `SshKeys-*.js`

- [ ] **Step 2: Backend build**

```bash
CGO_ENABLED=0 go build -ldflags="-s -w" -o oci-helper ./cmd/server
```

Expected: Binary at `./oci-helper`, ~26MB.

- [ ] **Step 3: Full test suite**

```bash
go test -v -count=1 ./internal/handler/...
```

Expected: All tests pass.

- [ ] **Step 4: Binary health check**

```bash
./oci-helper health
```

Expected: `{"status":"healthy"}` or health check output.

- [ ] **Step 5: Commit any remaining changes**

```bash
git add -A && git status
```

Verify only intended files are staged, then:

```bash
git commit -m "chore: final build verification — all features + tests pass"
```

- [ ] **Step 6: Push and create PR**

```bash
git push origin feature/java-parity
gh pr create --title "feat: Java feature parity — 5 new frontend views, WS logs, full TG bot, tests" --body "Closes feature parity gap between Java oci-helper and Go rewrite.

## Added
- **Users.vue** — User management page (CRUD, reset password, clear MFA)
- **IpPool.vue** — IP pool management (CIDR CRUD, OCI import, type tabs)
- **InstancePlans.vue** — Instance launch plans (card grid, use-as-template)
- **Defense.vue** — IP defense management (enable/disable, blacklist viewer)
- **SshKeys.vue** — SSH key management (generate, upload, copy public key)
- **WebSocket log streaming** — `/api/logs/ws` endpoint + live tail in Logs.vue
- **Telegram bot expansion** — 20+ new callback handlers (defense, blacklist, SSH keys, backup, traffic, volumes, plans, logs, version, checkalive, configs)
- **Backend unit tests** — 6 test files covering CRUD handlers (users, ip-data, instance-plans, defense, SSH keys)

## Modified
- `InstanceCreate.vue` — plan_id query param integration
- `Logs.vue` — live WebSocket tail mode
- Router, sidebar, i18n (en + zh-CN) for all new pages

🤖 Generated with [Claude Code](https://claude.com/claude-code)"
```

---

## Implementation Order

| Task | Group | Effort | Depends on |
|------|-------|--------|------------|
| 1 | Setup | Small | — |
| 2 | Users.vue | Small | 1 |
| 3 | IpPool.vue | Small | 1 |
| 4 | InstancePlans.vue | Medium | 1 |
| 5 | Defense.vue | Small | 1 |
| 6 | SshKeys.vue | Small | 1 |
| 7 | WS Logs | Medium | 1 |
| 8 | TG Bot | Large | 1 |
| 9 | Test Helpers | Small | 1 |
| 10 | Tests: Users | Small | 9 |
| 11 | Tests: IP/Plans/Defense/SSH | Medium | 9 |
| 12 | Final Build | Small | 2-11 |
