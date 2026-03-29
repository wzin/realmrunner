<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">Scheduled Restart</div>
      <p class="subtitle">{{ server.name }}</p>

      <form @submit.prevent="handleSave">
        <div v-if="error" class="alert alert-error">{{ error }}</div>
        <div v-if="saved" class="alert alert-success">Schedule saved</div>

        <div class="form-group">
          <label class="form-label">Restart Schedule</label>
          <select v-model="scheduleType" class="input" @change="updateSchedule">
            <option value="">Disabled</option>
            <option value="daily">Daily at specific time</option>
            <option value="interval">Every X hours</option>
          </select>
        </div>

        <div v-if="scheduleType === 'daily'" class="form-group">
          <label class="form-label">Time (HH:MM, 24h format)</label>
          <input v-model="dailyTime" type="time" class="input" @change="updateSchedule" />
        </div>

        <div v-if="scheduleType === 'interval'" class="form-group">
          <label class="form-label">Interval (hours)</label>
          <select v-model="intervalHours" class="input" @change="updateSchedule">
            <option value="4">Every 4 hours</option>
            <option value="6">Every 6 hours</option>
            <option value="8">Every 8 hours</option>
            <option value="12">Every 12 hours</option>
            <option value="24">Every 24 hours</option>
          </select>
        </div>

        <p class="help-text">
          <template v-if="schedule">Current: <strong>{{ schedule }}</strong>. Server will get 5min/1min/10s warnings in chat before restart.</template>
          <template v-else>No scheduled restart.</template>
        </p>

        <div class="modal-actions">
          <button type="button" @click="$emit('close')" class="btn btn-secondary">Cancel</button>
          <button type="submit" class="btn btn-primary" :disabled="saving">{{ saving ? 'Saving...' : 'Save' }}</button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
const emit = defineEmits(['close', 'saved'])

const schedule = ref(props.server.restart_schedule || '')
const scheduleType = ref('')
const dailyTime = ref('04:00')
const intervalHours = ref('6')
const saving = ref(false)
const saved = ref(false)
const error = ref('')

onMounted(() => {
  const s = schedule.value
  if (!s) {
    scheduleType.value = ''
  } else if (s.includes(':') && !s.startsWith('interval')) {
    scheduleType.value = 'daily'
    dailyTime.value = s
  } else if (s.startsWith('interval:')) {
    scheduleType.value = 'interval'
    intervalHours.value = s.replace('interval:', '').replace('h', '')
  }
})

function updateSchedule() {
  if (scheduleType.value === 'daily') {
    schedule.value = dailyTime.value
  } else if (scheduleType.value === 'interval') {
    schedule.value = `interval:${intervalHours.value}h`
  } else {
    schedule.value = ''
  }
}

async function handleSave() {
  saving.value = true
  error.value = ''
  saved.value = false
  try {
    await api.setSchedule(props.server.id, schedule.value)
    saved.value = true
    emit('saved')
  } catch (err) {
    error.value = err.message || 'Failed to save schedule'
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.subtitle { color: var(--text-muted); margin-bottom: 1.5rem; font-size: 0.875rem; }
.help-text { margin-top: 1rem; font-size: 0.75rem; color: var(--text-muted); }
.help-text strong { color: var(--accent); }
.modal-actions { display: flex; gap: 1rem; justify-content: flex-end; margin-top: 1.5rem; }
</style>
