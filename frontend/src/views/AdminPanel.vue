<template>
  <div class="admin">
    <header class="header">
      <div class="container">
        <div class="header-content">
          <div>
            <h1 class="title pixel-font">Admin Panel</h1>
            <p class="subtitle">Manage Realms &amp; Users</p>
          </div>
          <div class="header-right">
            <router-link to="/dashboard" class="btn btn-secondary">Dashboard</router-link>
          </div>
        </div>
      </div>
    </header>

    <main class="container">
      <div class="tabs">
        <button :class="['tab', { active: tab === 'realms' }]" @click="tab = 'realms'">Realms</button>
        <button :class="['tab', { active: tab === 'users' }]" @click="tab = 'users'">Users</button>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>
      <div v-if="success" class="alert alert-success">{{ success }}</div>

      <!-- Realms Tab -->
      <div v-if="tab === 'realms'">
        <div class="section-header">
          <h2 class="section-title pixel-font">Realms</h2>
          <button @click="showRealmForm = !showRealmForm" class="btn btn-primary btn-sm">
            {{ showRealmForm ? 'Cancel' : 'Create Realm' }}
          </button>
        </div>

        <div v-if="showRealmForm" class="form-card card">
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">Name</label>
              <input v-model="realmForm.name" class="input" placeholder="Realm name" />
            </div>
            <div class="form-group">
              <label class="form-label">Max CPU Cores</label>
              <input v-model.number="realmForm.maxCpu" type="number" step="0.5" min="0" class="input" placeholder="0 = unlimited" />
            </div>
            <div class="form-group">
              <label class="form-label">Max Memory (MB)</label>
              <input v-model.number="realmForm.maxMem" type="number" min="0" step="1024" class="input" placeholder="0 = unlimited" />
            </div>
            <div class="form-group">
              <label class="form-label">Max Servers</label>
              <input v-model.number="realmForm.maxServers" type="number" min="1" class="input" />
            </div>
          </div>
          <button @click="createRealm" class="btn btn-success btn-sm" :disabled="!realmForm.name">Create</button>
        </div>

        <div class="items-list">
          <div v-for="realm in realms" :key="realm.id" class="item-card card">
            <div class="item-header">
              <h3 class="item-name">{{ realm.name }}</h3>
              <button @click="deleteRealm(realm)" class="btn btn-danger btn-sm">Delete</button>
            </div>
            <div class="item-details">
              <span class="detail">CPU: {{ realm.max_cpu_cores || 'unlimited' }}</span>
              <span class="detail">RAM: {{ realm.max_memory_mb ? realm.max_memory_mb + ' MB' : 'unlimited' }}</span>
              <span class="detail">Servers: {{ realm.max_servers }}</span>
            </div>
            <div class="admin-section">
              <span class="admin-label">Admins:</span>
              <span v-for="a in realmAdmins[realm.id] || []" :key="a.id" class="admin-badge">
                {{ a.username }}
                <button @click="removeAdmin(realm.id, a.id)" class="remove-x">&times;</button>
              </span>
              <div class="add-admin">
                <select v-model="addAdminUser[realm.id]" class="input input-sm">
                  <option value="">Add admin...</option>
                  <option v-for="u in adminCandidates" :key="u.id" :value="u.id">{{ u.username }}</option>
                </select>
                <button v-if="addAdminUser[realm.id]" @click="addAdmin(realm.id)" class="btn btn-primary btn-sm">Add</button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Users Tab -->
      <div v-if="tab === 'users'">
        <div class="section-header">
          <h2 class="section-title pixel-font">Users</h2>
          <button @click="showUserForm = !showUserForm" class="btn btn-primary btn-sm">
            {{ showUserForm ? 'Cancel' : 'Create User' }}
          </button>
        </div>

        <div v-if="showUserForm" class="form-card card">
          <div class="form-row">
            <div class="form-group">
              <label class="form-label">Username</label>
              <input v-model="userForm.username" class="input" placeholder="username" />
            </div>
            <div class="form-group">
              <label class="form-label">Password</label>
              <input v-model="userForm.password" type="password" class="input" placeholder="password" />
            </div>
            <div class="form-group">
              <label class="form-label">Role</label>
              <select v-model="userForm.role" class="input">
                <option value="viewer">Viewer</option>
                <option value="admin">Admin</option>
                <option value="owner">Owner</option>
              </select>
            </div>
          </div>
          <button @click="createUser" class="btn btn-success btn-sm" :disabled="!userForm.username || !userForm.password">Create</button>
        </div>

        <div class="items-list">
          <div v-for="user in users" :key="user.id" class="item-card card">
            <div class="item-header">
              <h3 class="item-name">{{ user.username }}</h3>
              <span :class="['role-badge', `role-${user.role}`]">{{ user.role }}</span>
              <button v-if="user.role !== 'owner' || users.filter(u => u.role === 'owner').length > 1" @click="deleteUser(user)" class="btn btn-danger btn-sm">Delete</button>
            </div>
            <div class="item-details">
              <span class="detail">Role:
                <select :value="user.role" @change="updateRole(user, $event.target.value)" class="input input-sm inline-select">
                  <option value="viewer">Viewer</option>
                  <option value="admin">Admin</option>
                  <option value="owner">Owner</option>
                </select>
              </span>
              <span class="detail">Created: {{ new Date(user.created_at).toLocaleDateString() }}</span>
            </div>
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { api } from '../api/client'

const tab = ref('realms')
const error = ref('')
const success = ref('')
const realms = ref([])
const users = ref([])
const realmAdmins = reactive({})
const addAdminUser = reactive({})

const showRealmForm = ref(false)
const realmForm = ref({ name: '', maxCpu: 0, maxMem: 0, maxServers: 10 })
const showUserForm = ref(false)
const userForm = ref({ username: '', password: '', role: 'viewer' })

const adminCandidates = computed(() => users.value.filter(u => u.role === 'admin' || u.role === 'viewer'))

async function loadRealms() {
  try {
    const resp = await api.getRealms()
    realms.value = resp.realms || []
    for (const r of realms.value) {
      const admResp = await api.getRealmAdmins(r.id)
      realmAdmins[r.id] = admResp.admins || []
    }
  } catch (err) {
    error.value = err.message || 'Failed to load realms'
  }
}

async function loadUsers() {
  try {
    const resp = await api.getUsers()
    users.value = resp.users || []
  } catch (err) {
    error.value = err.message || 'Failed to load users'
  }
}

async function createRealm() {
  error.value = ''; success.value = ''
  try {
    await api.createRealm(realmForm.value.name, realmForm.value.maxCpu, realmForm.value.maxMem, realmForm.value.maxServers)
    success.value = 'Realm created'
    showRealmForm.value = false
    realmForm.value = { name: '', maxCpu: 0, maxMem: 0, maxServers: 10 }
    await loadRealms()
  } catch (err) { error.value = err.message }
}

async function deleteRealm(realm) {
  if (!confirm(`Delete realm "${realm.name}"?`)) return
  error.value = ''
  try {
    await api.deleteRealm(realm.id)
    await loadRealms()
  } catch (err) { error.value = err.message }
}

async function addAdmin(realmId) {
  const userId = addAdminUser[realmId]
  if (!userId) return
  try {
    await api.addRealmAdmin(realmId, userId)
    addAdminUser[realmId] = ''
    const resp = await api.getRealmAdmins(realmId)
    realmAdmins[realmId] = resp.admins || []
  } catch (err) { error.value = err.message }
}

async function removeAdmin(realmId, userId) {
  try {
    await api.removeRealmAdmin(realmId, userId)
    const resp = await api.getRealmAdmins(realmId)
    realmAdmins[realmId] = resp.admins || []
  } catch (err) { error.value = err.message }
}

async function createUser() {
  error.value = ''; success.value = ''
  try {
    await api.createUser(userForm.value.username, userForm.value.password, userForm.value.role)
    success.value = 'User created'
    showUserForm.value = false
    userForm.value = { username: '', password: '', role: 'viewer' }
    await loadUsers()
  } catch (err) { error.value = err.message }
}

async function deleteUser(user) {
  if (!confirm(`Delete user "${user.username}"?`)) return
  try {
    await api.deleteUser(user.id)
    await loadUsers()
  } catch (err) { error.value = err.message }
}

async function updateRole(user, newRole) {
  try {
    await api.updateUser(user.id, newRole)
    await loadUsers()
  } catch (err) { error.value = err.message }
}

onMounted(() => {
  loadRealms()
  loadUsers()
})
</script>

<style scoped>
.header {
  background: var(--bg-header);
  border-bottom: 2px solid var(--border);
  padding: 1.5rem 0;
  margin-bottom: 2rem;
  box-shadow: 0 2px 0 var(--border-shadow);
}
.header-content { display: flex; justify-content: space-between; align-items: center; }
.header-right { display: flex; gap: 1rem; align-items: center; }
.title { font-size: 1.125rem; color: var(--accent); text-shadow: 2px 2px 0 var(--border-shadow); }
.subtitle { color: var(--text-muted); margin-top: 0.25rem; font-size: 0.75rem; }

.tabs { display: flex; gap: 0.5rem; margin-bottom: 1.5rem; }
.tab { padding: 0.5rem 1rem; border: 2px solid var(--border); border-radius: 2px; background: var(--bg-input); color: var(--text-muted); cursor: pointer; font-family: 'Press Start 2P', monospace; font-size: 0.5rem; }
.tab.active { background: var(--accent); color: var(--accent-text); border-color: var(--accent); }

.section-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; }
.section-title { font-size: 0.625rem; color: var(--text-primary); }

.form-card { margin-bottom: 1.5rem; }
.form-row { display: flex; gap: 1rem; flex-wrap: wrap; margin-bottom: 1rem; }
.form-row .form-group { flex: 1; min-width: 150px; }

.items-list { display: flex; flex-direction: column; gap: 1rem; }
.item-card { }
.item-header { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem; }
.item-name { font-size: 1rem; font-weight: 600; }
.item-details { display: flex; gap: 1.5rem; flex-wrap: wrap; margin-bottom: 0.5rem; }
.detail { font-size: 0.8125rem; color: var(--text-muted); }
.inline-select { display: inline; width: auto; padding: 0.125rem 0.25rem; font-size: 0.75rem; }

.role-badge { font-family: 'Press Start 2P', monospace; font-size: 0.4rem; padding: 0.125rem 0.375rem; border-radius: 2px; text-transform: uppercase; }
.role-owner { background: var(--warning); color: var(--warning-text); }
.role-admin { background: var(--accent); color: var(--accent-text); }
.role-viewer { background: var(--secondary); color: var(--text-primary); }

.admin-section { display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap; margin-top: 0.5rem; }
.admin-label { font-size: 0.75rem; color: var(--text-muted); }
.admin-badge { font-size: 0.75rem; background: var(--accent); color: var(--accent-text); padding: 0.125rem 0.5rem; border-radius: 2px; display: inline-flex; align-items: center; gap: 0.25rem; }
.remove-x { background: none; border: none; color: var(--accent-text); cursor: pointer; font-size: 0.875rem; padding: 0; line-height: 1; }
.add-admin { display: flex; gap: 0.375rem; align-items: center; }
.add-admin .input { width: 150px; font-size: 0.75rem; padding: 0.25rem; }

.input-sm { font-size: 0.75rem; padding: 0.25rem 0.375rem; }
</style>
