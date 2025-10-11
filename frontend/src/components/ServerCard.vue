<template>
  <div class="server-card card">
    <div class="server-header">
      <div>
        <h3 class="server-name">{{ server.name }}</h3>
        <p class="server-version">Version: {{ server.version }}</p>
      </div>
      <span :class="['status-badge', `status-${server.status}`]">
        {{ server.status }}
      </span>
    </div>

    <div class="server-info">
      <div class="info-item">
        <span class="info-label">Port:</span>
        <span class="info-value">{{ server.port }}</span>
      </div>
      <div class="info-item">
        <span class="info-label">Created:</span>
        <span class="info-value">{{ formatDate(server.created_at) }}</span>
      </div>
    </div>

    <div v-if="error" class="alert alert-error">
      {{ error }}
    </div>

    <div class="server-actions">
      <button
        v-if="server.status === 'stopped'"
        @click="handleStart"
        class="btn btn-success btn-sm"
        :disabled="loading"
      >
        Start
      </button>
      <button
        v-else-if="server.status === 'running'"
        @click="handleStop"
        class="btn btn-danger btn-sm"
        :disabled="loading"
      >
        Stop
      </button>
      <button
        v-else
        class="btn btn-secondary btn-sm"
        disabled
      >
        {{ server.status }}
      </button>

      <button
        v-if="server.status === 'running'"
        @click="$emit('console', server)"
        class="btn btn-primary btn-sm"
      >
        Console
      </button>

      <button
        v-if="server.status === 'stopped'"
        @click="$emit('console', server)"
        class="btn btn-secondary btn-sm"
      >
        View Logs
      </button>

      <button
        v-if="server.status === 'stopped'"
        @click="handleReset"
        class="btn btn-warning btn-sm"
        :disabled="loading"
      >
        Reset
      </button>

      <button
        v-if="server.status === 'stopped'"
        @click="handleDelete"
        class="btn btn-danger btn-sm"
        :disabled="loading"
      >
        Delete
      </button>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { api } from '../api/client'

const props = defineProps({
  server: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['refresh', 'console'])

const loading = ref(false)
const error = ref('')

async function handleStart() {
  loading.value = true
  error.value = ''

  try {
    await api.startServer(props.server.id)
    emit('refresh')
  } catch (err) {
    error.value = err.message || 'Failed to start server'
  } finally {
    loading.value = false
  }
}

async function handleStop() {
  loading.value = true
  error.value = ''

  try {
    await api.stopServer(props.server.id)
    emit('refresh')
  } catch (err) {
    error.value = err.message || 'Failed to stop server'
  } finally {
    loading.value = false
  }
}

async function handleReset() {
  if (!confirm(`Are you sure you want to reset "${props.server.name}"? This will delete the world directory and start fresh.`)) {
    return
  }

  loading.value = true
  error.value = ''

  try {
    await api.resetServer(props.server.id)
    emit('refresh')
  } catch (err) {
    error.value = err.message || 'Failed to reset server'
  } finally {
    loading.value = false
  }
}

async function handleDelete() {
  if (!confirm(`Are you sure you want to delete "${props.server.name}"? This will permanently delete all server data!`)) {
    return
  }

  loading.value = true
  error.value = ''

  try {
    await api.wipeoutServer(props.server.id)
    emit('refresh')
  } catch (err) {
    error.value = err.message || 'Failed to delete server'
  } finally {
    loading.value = false
  }
}

function formatDate(dateString) {
  const date = new Date(dateString)
  return date.toLocaleDateString()
}
</script>

<style scoped>
.server-card {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.server-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
}

.server-name {
  font-size: 1.25rem;
  font-weight: 600;
  margin-bottom: 0.25rem;
}

.server-version {
  color: #94a3b8;
  font-size: 0.875rem;
}

.status-badge {
  padding: 0.25rem 0.75rem;
  border-radius: 9999px;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
}

.status-stopped {
  background: #475569;
  color: #e2e8f0;
}

.status-starting {
  background: #f59e0b;
  color: #78350f;
}

.status-running {
  background: #10b981;
  color: #064e3b;
}

.status-stopping {
  background: #ef4444;
  color: #7f1d1d;
}

.server-info {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.info-item {
  display: flex;
  justify-content: space-between;
}

.info-label {
  color: #94a3b8;
  font-size: 0.875rem;
}

.info-value {
  font-weight: 500;
}

.server-actions {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.btn-sm {
  padding: 0.375rem 0.75rem;
  font-size: 0.875rem;
}
</style>
