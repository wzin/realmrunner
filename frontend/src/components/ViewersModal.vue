<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">
        <span>{{ server.name }} - Viewers</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>

      <div class="add-form">
        <select v-model="selectedUser" class="input">
          <option value="">Add viewer...</option>
          <option v-for="u in availableUsers" :key="u.id" :value="u.id">{{ u.username }} ({{ u.role }})</option>
        </select>
        <button @click="addViewer" class="btn btn-primary btn-sm" :disabled="!selectedUser">Add</button>
      </div>

      <div class="viewer-list">
        <div v-if="loading" class="loading">Loading...</div>
        <div v-else-if="!viewers.length" class="empty">No viewers assigned</div>
        <div v-for="v in viewers" :key="v.id" class="viewer-item">
          <span class="viewer-name">{{ v.username }}</span>
          <button @click="removeViewer(v)" class="btn btn-danger btn-sm">Remove</button>
        </div>
      </div>

      <p class="help-text">Viewers get read-only access to this server's logs, metrics, and config files.</p>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
defineEmits(['close'])

const viewers = ref([])
const users = ref([])
const loading = ref(false)
const error = ref('')
const selectedUser = ref('')

const availableUsers = computed(() => {
  const viewerIds = new Set(viewers.value.map(v => v.id))
  return users.value.filter(u => !viewerIds.has(u.id))
})

async function loadViewers() {
  loading.value = true
  try {
    const resp = await api.getServerViewers(props.server.id)
    viewers.value = resp.viewers || []
  } catch (err) {
    error.value = err.message || 'Failed to load viewers'
  } finally {
    loading.value = false
  }
}

async function loadUsers() {
  try {
    const resp = await api.getUsers()
    users.value = resp.users || []
  } catch { /* non-admin won't see users, that's ok */ }
}

async function addViewer() {
  if (!selectedUser.value) return
  error.value = ''
  try {
    await api.addServerViewer(props.server.id, selectedUser.value)
    selectedUser.value = ''
    await loadViewers()
  } catch (err) {
    error.value = err.message || 'Failed to add viewer'
  }
}

async function removeViewer(v) {
  error.value = ''
  try {
    await api.removeServerViewer(props.server.id, v.id)
    await loadViewers()
  } catch (err) {
    error.value = err.message || 'Failed to remove viewer'
  }
}

onMounted(() => {
  loadViewers()
  loadUsers()
})
</script>

<style scoped>
.modal-header { display: flex; justify-content: space-between; align-items: center; }
.close-btn { background: none; border: none; color: var(--text-primary); font-size: 2rem; line-height: 1; cursor: pointer; padding: 0; font-family: inherit; }
.close-btn:hover { color: var(--accent); }
.add-form { display: flex; gap: 0.5rem; margin-bottom: 1rem; }
.add-form .input { flex: 1; }
.viewer-list { max-height: 300px; overflow-y: auto; }
.loading, .empty { text-align: center; padding: 2rem; color: var(--text-muted); font-size: 0.875rem; }
.viewer-item { display: flex; justify-content: space-between; align-items: center; padding: 0.5rem; border-bottom: 1px solid var(--border); }
.viewer-name { font-weight: 500; }
.help-text { margin-top: 1rem; font-size: 0.6875rem; color: var(--text-muted); }
</style>
