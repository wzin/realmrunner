<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header">Create New Server</div>

      <form @submit.prevent="handleCreate">
        <div v-if="error" class="alert alert-error">
          {{ error }}
        </div>

        <div class="form-group">
          <label for="name" class="form-label">Server Name</label>
          <input
            id="name"
            v-model="form.name"
            type="text"
            class="input"
            placeholder="My Minecraft Server"
            required
            :disabled="loading"
          />
        </div>

        <div class="form-group">
          <label for="version" class="form-label">Minecraft Version</label>
          <select
            id="version"
            v-model="form.version"
            class="input"
            required
            :disabled="loading || loadingVersions"
          >
            <option value="">{{ loadingVersions ? 'Loading...' : 'Select version' }}</option>
            <option v-for="version in versions" :key="version" :value="version">
              {{ version }}
            </option>
          </select>
        </div>

        <div class="form-group">
          <label for="port" class="form-label">Port</label>
          <input
            id="port"
            v-model.number="form.port"
            type="number"
            class="input"
            :class="{ 'input-error': portError }"
            placeholder="25565"
            required
            min="1"
            max="65535"
            :disabled="loading"
          />
          <p v-if="portError" class="error-text">{{ portError }}</p>
          <p v-else class="help-text">Default Minecraft port is 25565. Occupied ports: {{ occupiedPorts.join(', ') || 'None' }}</p>
        </div>

        <div class="modal-actions">
          <button
            type="button"
            @click="$emit('close')"
            class="btn btn-secondary"
            :disabled="loading"
          >
            Cancel
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="loading || loadingVersions || !!portError"
          >
            {{ loading ? 'Creating...' : 'Create Server' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { api } from '../api/client'

const emit = defineEmits(['close', 'created'])

const form = ref({
  name: '',
  version: '',
  port: null
})

const versions = ref([])
const servers = ref([])
const loading = ref(false)
const loadingVersions = ref(false)
const error = ref('')

const occupiedPorts = computed(() => {
  return servers.value.map(s => s.port).sort((a, b) => a - b)
})

const portError = computed(() => {
  const port = form.value.port

  if (!port) {
    return ''
  }

  if (port < 1 || port > 65535) {
    return 'Port must be between 1 and 65535'
  }

  if (occupiedPorts.value.includes(port)) {
    return `Port ${port} is already in use by another server`
  }

  return ''
})

async function loadVersions() {
  loadingVersions.value = true
  try {
    const response = await api.getVersions()
    versions.value = response.versions || []
  } catch (err) {
    error.value = 'Failed to load versions'
  } finally {
    loadingVersions.value = false
  }
}

async function loadServers() {
  try {
    servers.value = await api.getServers()
  } catch (err) {
    console.error('Failed to load servers:', err)
  }
}

async function handleCreate() {
  // Check if there's a port error
  if (portError.value) {
    return
  }

  loading.value = true
  error.value = ''

  try {
    await api.createServer(form.value.name, form.value.version, form.value.port)
    emit('created')
  } catch (err) {
    error.value = err.message || 'Failed to create server'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadVersions()
  loadServers()
})
</script>

<style scoped>
.help-text {
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: #94a3b8;
}

.error-text {
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: #ef4444;
}

.input-error {
  border-color: #ef4444;
}

.modal-actions {
  display: flex;
  gap: 1rem;
  justify-content: flex-end;
  margin-top: 1.5rem;
}
</style>
