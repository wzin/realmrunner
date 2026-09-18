<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">Sleep &amp; Recovery</div>
      <p class="subtitle">{{ server.name }}</p>

      <form @submit.prevent="handleSave">
        <div v-if="error" class="alert alert-error">{{ error }}</div>

        <div class="form-group">
          <label class="checkbox-row">
            <input v-model="form.autoSleep" type="checkbox" :disabled="loading || !canChangeSleep" />
            <span>Sleep when empty</span>
          </label>
          <p class="help-text">
            RealmRunner holds port {{ server.port }} while the realm sleeps. It still appears in the
            server list, and starts automatically when a player joins.
          </p>
          <p v-if="!canChangeSleep" class="help-text warning">Stop the server to change this setting.</p>
        </div>

        <div class="form-group">
          <label class="form-label">Sleep after (minutes empty)</label>
          <input v-model.number="form.idleTimeoutMin" type="number" class="input" min="1" max="1440"
                 :disabled="loading || !form.autoSleep || !canChangeSleep" />
          <p class="help-text">How long the realm stays up with nobody online.</p>
        </div>

        <div class="form-group">
          <label class="checkbox-row">
            <input v-model="form.autoRestart" type="checkbox" :disabled="loading" />
            <span>Restart after a crash</span>
          </label>
          <p class="help-text">
            Restarts with a growing delay, giving up after five crashes in a row so a broken realm
            is not restarted forever.
          </p>
        </div>

        <div class="modal-actions">
          <button type="button" @click="$emit('close')" class="btn btn-secondary" :disabled="loading">Cancel</button>
          <button type="submit" class="btn btn-primary" :disabled="loading">
            {{ loading ? 'Saving...' : 'Save' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
const emit = defineEmits(['close', 'saved'])

const form = ref({
  autoSleep: !!props.server.auto_sleep,
  idleTimeoutMin: props.server.idle_timeout_min || 15,
  autoRestart: props.server.auto_restart !== false,
})
const loading = ref(false)
const error = ref('')

const canChangeSleep = computed(() =>
  ['stopped', 'sleeping', 'crashed'].includes(props.server.status)
)

async function handleSave() {
  loading.value = true
  error.value = ''
  try {
    if (form.value.autoRestart !== (props.server.auto_restart !== false)) {
      await api.setAutoRestart(props.server.id, form.value.autoRestart)
    }
    if (canChangeSleep.value) {
      await api.setAutoSleep(props.server.id, form.value.autoSleep, form.value.idleTimeoutMin)
    }
    emit('saved')
  } catch (err) {
    error.value = err.message || 'Failed to save settings'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.subtitle { color: var(--text-muted); margin-bottom: 1.5rem; font-size: 0.875rem; }
.help-text { margin-top: 0.25rem; font-size: 0.75rem; color: var(--text-muted); }
.help-text.warning { color: var(--status-starting-text, #b45309); }
.checkbox-row { display: flex; align-items: center; gap: 0.5rem; font-weight: 600; }
.modal-actions { display: flex; gap: 1rem; justify-content: flex-end; margin-top: 1.5rem; }
</style>
