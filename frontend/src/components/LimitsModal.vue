<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">Resource Limits</div>
      <p class="subtitle">{{ server.name }}</p>

      <form @submit.prevent="handleSave">
        <div v-if="error" class="alert alert-error">{{ error }}</div>

        <div class="form-group">
          <label class="form-label">CPU Limit (cores)</label>
          <input v-model.number="form.cpuLimit" type="number" class="input" step="0.1" min="0" placeholder="0 = unlimited" :disabled="loading" />
          <p class="help-text">Number of CPU cores (e.g., 1.5). Set to 0 for unlimited.</p>
        </div>

        <div class="form-group">
          <label class="form-label">Memory Limit (MB)</label>
          <input v-model.number="form.memoryLimitMB" type="number" class="input" min="0" step="256" placeholder="0 = use default" :disabled="loading" />
          <p class="help-text">Memory limit in MB. Set to 0 to use the global default.</p>
        </div>

        <div class="modal-actions">
          <button type="button" @click="$emit('close')" class="btn btn-secondary" :disabled="loading">Cancel</button>
          <button type="submit" class="btn btn-primary" :disabled="loading">
            {{ loading ? 'Saving...' : 'Save Limits' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
const emit = defineEmits(['close', 'saved'])

const form = ref({
  cpuLimit: props.server.cpu_limit || 0,
  memoryLimitMB: props.server.memory_limit_mb || 0,
})
const loading = ref(false)
const error = ref('')

async function handleSave() {
  loading.value = true
  error.value = ''
  try {
    await api.setLimits(props.server.id, form.value.cpuLimit, form.value.memoryLimitMB)
    emit('saved')
  } catch (err) {
    error.value = err.message || 'Failed to save limits'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.subtitle { color: var(--text-muted); margin-bottom: 1.5rem; font-size: 0.875rem; }
.help-text { margin-top: 0.25rem; font-size: 0.75rem; color: var(--text-muted); }
.modal-actions { display: flex; gap: 1rem; justify-content: flex-end; margin-top: 1.5rem; }
</style>
