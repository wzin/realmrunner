<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">
        <span>{{ server.name }} - Backups</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>
      <div v-if="success" class="alert alert-success">{{ success }}</div>

      <div class="actions-row">
        <button @click="createBackup" class="btn btn-primary btn-sm" :disabled="creating">
          {{ creating ? 'Creating...' : 'Create Backup Now' }}
        </button>
      </div>

      <div class="backup-list">
        <div v-if="loading" class="loading">Loading...</div>
        <div v-else-if="!backups.length" class="empty">No backups yet</div>
        <div v-for="b in backups" :key="b.id" class="backup-item">
          <div class="backup-info">
            <span class="backup-name">{{ b.filename }}</span>
            <span class="backup-meta">{{ formatSize(b.size_bytes) }} &middot; {{ formatDate(b.created_at) }}</span>
          </div>
          <div class="backup-actions">
            <button @click="restoreBackup(b)" class="btn btn-warning btn-sm" :disabled="server.status !== 'stopped'">
              Restore
            </button>
            <button @click="deleteBackup(b)" class="btn btn-danger btn-sm">Delete</button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
defineEmits(['close'])

const backups = ref([])
const loading = ref(false)
const creating = ref(false)
const error = ref('')
const success = ref('')

async function loadBackups() {
  loading.value = true
  try {
    const resp = await api.getBackups(props.server.id)
    backups.value = resp.backups || []
  } catch (err) {
    error.value = err.message || 'Failed to load backups'
  } finally {
    loading.value = false
  }
}

async function createBackup() {
  creating.value = true
  error.value = ''
  success.value = ''
  try {
    await api.createBackup(props.server.id)
    success.value = 'Backup created successfully'
    await loadBackups()
  } catch (err) {
    error.value = err.message || 'Failed to create backup'
  } finally {
    creating.value = false
  }
}

async function restoreBackup(b) {
  if (!confirm(`Restore backup ${b.filename}? This will overwrite current server files.`)) return
  error.value = ''
  success.value = ''
  try {
    await api.restoreBackup(props.server.id, b.id)
    success.value = 'Backup restored successfully'
  } catch (err) {
    error.value = err.message || 'Failed to restore backup'
  }
}

async function deleteBackup(b) {
  if (!confirm(`Delete backup ${b.filename}?`)) return
  error.value = ''
  try {
    await api.deleteBackup(props.server.id, b.id)
    await loadBackups()
  } catch (err) {
    error.value = err.message || 'Failed to delete backup'
  }
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function formatDate(d) {
  return new Date(d).toLocaleString()
}

onMounted(loadBackups)
</script>

<style scoped>
.modal-header { display: flex; justify-content: space-between; align-items: center; }
.close-btn {
  background: none; border: none; color: var(--text-primary);
  font-size: 2rem; line-height: 1; cursor: pointer; padding: 0; font-family: inherit;
}
.close-btn:hover { color: var(--accent); }

.actions-row { margin-bottom: 1rem; }
.backup-list { max-height: 400px; overflow-y: auto; }
.loading, .empty { text-align: center; padding: 2rem; color: var(--text-muted); font-size: 0.875rem; }

.backup-item {
  display: flex; justify-content: space-between; align-items: center;
  padding: 0.75rem; border-bottom: 1px solid var(--border);
}
.backup-info { display: flex; flex-direction: column; gap: 0.25rem; }
.backup-name { font-size: 0.8125rem; font-family: monospace; }
.backup-meta { font-size: 0.6875rem; color: var(--text-muted); }
.backup-actions { display: flex; gap: 0.375rem; }
</style>
