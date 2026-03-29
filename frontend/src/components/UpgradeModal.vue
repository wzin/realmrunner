<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">Upgrade {{ server.name }}</div>

      <p class="current-info">
        Currently: <span class="flavor-badge">{{ server.flavor || 'vanilla' }}</span> {{ server.version }}
      </p>

      <form @submit.prevent="handleUpgrade">
        <div v-if="error" class="alert alert-error">{{ error }}</div>

        <div class="form-group">
          <label for="flavor" class="form-label">Server Type</label>
          <select id="flavor" v-model="form.flavor" class="input" :disabled="loading" @change="onFlavorChange">
            <option v-for="f in flavors" :key="f" :value="f">
              {{ flavorLabels[f] || f }}
            </option>
          </select>
        </div>

        <div class="form-group">
          <label for="version" class="form-label">New Version</label>
          <div class="version-row">
            <select id="version" v-model="form.version" class="input" required :disabled="loading || loadingVersions">
              <option value="">{{ loadingVersions ? 'Loading...' : 'Select version' }}</option>
              <option v-for="v in versions" :key="v.id" :value="v.id">
                {{ v.id }}{{ v.type && v.type !== 'release' ? ` (${v.type})` : '' }}
              </option>
            </select>
            <label v-if="form.flavor === 'vanilla'" class="snapshot-toggle">
              <input type="checkbox" v-model="includeSnapshots" @change="loadVersions" />
              <span class="toggle-label">All</span>
            </label>
          </div>
        </div>

        <div class="modal-actions">
          <button type="button" @click="$emit('close')" class="btn btn-secondary" :disabled="loading">Cancel</button>
          <button type="submit" class="btn btn-warning" :disabled="loading || !form.version">
            {{ loading ? 'Upgrading...' : 'Upgrade Server' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
const emit = defineEmits(['close', 'upgraded'])

const form = ref({
  flavor: props.server.flavor || 'vanilla',
  version: '',
})

const versions = ref([])
const flavors = ref(['vanilla'])
const loading = ref(false)
const loadingVersions = ref(false)
const error = ref('')
const includeSnapshots = ref(false)

const flavorLabels = {
  vanilla: 'Vanilla (Official Mojang)',
  paper: 'Paper (Performance + Plugins)',
  purpur: 'Purpur (Paper fork, extra features)',
}

function onFlavorChange() {
  form.value.version = ''
  loadVersions()
}

async function loadVersions() {
  loadingVersions.value = true
  try {
    const resp = await api.getVersions(form.value.flavor, includeSnapshots.value)
    versions.value = resp.version_details || (resp.versions || []).map(v => ({ id: v, type: 'release' }))
  } catch (err) {
    error.value = 'Failed to load versions'
  } finally {
    loadingVersions.value = false
  }
}

async function loadFlavors() {
  try {
    const resp = await api.getFlavors()
    flavors.value = resp.flavors || ['vanilla']
  } catch (err) {
    console.error('Failed to load flavors:', err)
  }
}

async function handleUpgrade() {
  loading.value = true
  error.value = ''
  try {
    await api.upgradeServer(props.server.id, form.value.version, form.value.flavor)
    emit('upgraded')
  } catch (err) {
    error.value = err.message || 'Failed to upgrade server'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadFlavors()
  loadVersions()
})
</script>

<style scoped>
.current-info {
  color: var(--text-muted);
  margin-bottom: 1.5rem;
  font-size: 0.875rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.flavor-badge {
  font-family: 'Press Start 2P', monospace;
  font-size: 0.4rem;
  background: var(--accent);
  color: var(--accent-text);
  padding: 0.125rem 0.375rem;
  border-radius: 2px;
  text-transform: uppercase;
}

.modal-actions {
  display: flex;
  gap: 1rem;
  justify-content: flex-end;
  margin-top: 1.5rem;
}

.version-row {
  display: flex;
  gap: 0.75rem;
  align-items: center;
}

.version-row .input { flex: 1; }

.snapshot-toggle {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  white-space: nowrap;
  cursor: pointer;
}

.snapshot-toggle input[type="checkbox"] { accent-color: var(--accent); }
.toggle-label { font-size: 0.75rem; color: var(--text-muted); }
</style>
