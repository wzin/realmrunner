<template>
  <div class="server-card card">
    <div class="server-header">
      <div>
        <h3 class="server-name pixel-font">{{ server.name }}</h3>
        <p class="server-version">
          <span class="flavor-badge">{{ server.flavor || 'vanilla' }}</span>
          {{ server.version }}
        </p>
      </div>
      <span :class="['status-badge pixel-font', `status-${server.status}`]">
        {{ server.status }}
      </span>
    </div>

    <div class="server-info">
      <div class="info-item">
        <span class="info-label">Port:</span>
        <span class="info-value">{{ server.port }}</span>
      </div>
      <div class="info-item">
        <span class="info-label">Server Address:</span>
        <span class="info-value connection-url">{{ server.connection_url || `localhost:${server.port}` }}</span>
      </div>
      <div class="info-item">
        <span class="info-label">Created:</span>
        <span class="info-value">{{ formatDate(server.created_at) }}</span>
      </div>
    </div>

    <div v-if="displayMetrics" class="metrics-row">
      <div class="metric">
        <span class="metric-label">CPU</span>
        <span class="metric-value">{{ displayMetrics.cpu_percent.toFixed(1) }}%</span>
      </div>
      <div class="metric">
        <span class="metric-label">RAM</span>
        <span class="metric-value">{{ displayMetrics.memory_mb.toFixed(0) }} MB</span>
      </div>
      <div class="metric" :title="playerTooltip">
        <span class="metric-label">Players</span>
        <span class="metric-value">{{ displayMetrics.player_count }}</span>
      </div>
    </div>

    <div v-if="error" class="alert alert-error">
      {{ error }}
    </div>

    <!-- Power Controls -->
    <div class="action-section">
      <span class="section-label pixel-font">Power</span>
      <div class="action-buttons">
        <button v-if="server.status === 'stopped' && server.ready" @click="handleStart" class="btn btn-success btn-sm" :disabled="loading">Start</button>
        <button v-else-if="server.status === 'stopped' && !server.ready && isStaleDownload" @click="handleStart" class="btn btn-warning btn-sm" :disabled="loading" title="Download may have failed">Retry Start</button>
        <button v-else-if="server.status === 'stopped' && !server.ready" class="btn btn-secondary btn-sm" disabled>Downloading...</button>
        <button v-if="server.status === 'running'" @click="handleStop" class="btn btn-danger btn-sm" :disabled="loading">Stop</button>
        <button v-if="server.status === 'running' || server.status === 'stopping'" @click="handleForceStop" class="btn btn-danger btn-sm" :disabled="loading" title="Force kill the process immediately">Force Kill</button>
        <button v-if="server.status === 'starting' || server.status === 'stopping'" class="btn btn-secondary btn-sm" disabled>{{ server.status }}...</button>
      </div>
    </div>

    <!-- Console & Monitoring -->
    <div class="action-section">
      <span class="section-label pixel-font">Monitor</span>
      <div class="action-buttons">
        <button v-if="server.status === 'running'" @click="$emit('console', server)" class="btn btn-primary btn-sm">Console</button>
        <button v-if="server.status === 'stopped'" @click="$emit('console', server)" class="btn btn-secondary btn-sm">View Logs</button>
        <button @click="$emit('metrics', server)" class="btn btn-secondary btn-sm">Metrics</button>
      </div>
    </div>

    <!-- Configuration -->
    <div class="action-section">
      <span class="section-label pixel-font">Configure</span>
      <div class="action-buttons">
        <button @click="$emit('files', server)" class="btn btn-secondary btn-sm">Config</button>
        <button @click="$emit('players', server)" class="btn btn-secondary btn-sm">Players</button>
        <button @click="$emit('schedule', server)" class="btn btn-secondary btn-sm">Schedule</button>
        <button v-if="server.status === 'stopped'" @click="$emit('limits', server)" class="btn btn-secondary btn-sm">Limits</button>
        <button v-if="server.status === 'stopped'" @click="$emit('upgrade', server)" class="btn btn-secondary btn-sm">Upgrade</button>
        <button v-if="server.flavor && server.flavor !== 'vanilla'" @click="$emit('mods', server)" class="btn btn-secondary btn-sm">Mods</button>
      </div>
    </div>

    <!-- Access & Sharing -->
    <div class="action-section">
      <span class="section-label pixel-font">Access</span>
      <div class="action-buttons">
        <button @click="$emit('viewers', server)" class="btn btn-secondary btn-sm">Viewers</button>
        <button @click="$emit('share', server)" class="btn btn-secondary btn-sm">Share Link</button>
      </div>
    </div>

    <!-- Data Management -->
    <div class="action-section">
      <span class="section-label pixel-font">Data</span>
      <div class="action-buttons">
        <button @click="$emit('backups', server)" class="btn btn-secondary btn-sm">Backups</button>
        <button v-if="server.status === 'stopped'" @click="handleReset" class="btn btn-warning btn-sm" :disabled="loading">Reset World</button>
        <button v-if="server.status === 'stopped'" @click="handleDelete" class="btn btn-danger btn-sm" :disabled="loading">Delete Server</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onUnmounted } from 'vue'
import { api, createWebSocket } from '../api/client'

const props = defineProps({
  server: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['refresh', 'console', 'metrics', 'upgrade', 'limits', 'files', 'players', 'backups', 'mods', 'schedule', 'share', 'viewers'])

const loading = ref(false)
const error = ref('')
const liveMetrics = ref(null)
let metricsWs = null

const displayMetrics = computed(() => {
  return liveMetrics.value || props.server.metrics
})

const isStaleDownload = computed(() => {
  if (props.server.ready) return false
  const created = new Date(props.server.created_at)
  const now = new Date()
  return (now - created) > 10 * 60 * 1000 // 10 minutes
})

const playerTooltip = computed(() => {
  const m = displayMetrics.value
  if (m && m.player_names && m.player_names.length > 0) {
    return m.player_names.join(', ')
  }
  return 'No players online'
})

function connectMetricsWs() {
  if (metricsWs) { metricsWs.close(); metricsWs = null }
  if (props.server.status !== 'running') return

  try {
    metricsWs = createWebSocket(props.server.id)
    metricsWs.onmessage = (event) => {
      const data = JSON.parse(event.data)
      if (data.type === 'metrics') {
        liveMetrics.value = {
          cpu_percent: data.cpu_percent,
          memory_mb: data.memory_mb,
          player_count: data.player_count,
          player_names: data.player_names || [],
        }
      }
    }
    metricsWs.onerror = () => {}
    metricsWs.onclose = () => { metricsWs = null }
  } catch (e) {}
}

watch(() => props.server.status, (newStatus) => {
  if (newStatus === 'running') {
    connectMetricsWs()
  } else {
    if (metricsWs) { metricsWs.close(); metricsWs = null }
    liveMetrics.value = null
  }
}, { immediate: true })

onUnmounted(() => {
  if (metricsWs) { metricsWs.close(); metricsWs = null }
})

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

async function handleForceStop() {
  if (!confirm(`Force kill "${props.server.name}"? This will immediately terminate the process without saving!`)) return
  loading.value = true
  error.value = ''
  try {
    await api.forceStopServer(props.server.id)
    emit('refresh')
  } catch (err) {
    error.value = err.message || 'Failed to force stop server'
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
  font-size: 0.75rem;
  margin-bottom: 0.5rem;
  color: var(--text-primary);
}

.server-version {
  color: var(--text-muted);
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

.status-badge {
  padding: 0.25rem 0.75rem;
  border: 2px solid;
  border-radius: 2px;
  font-size: 0.5rem;
  text-transform: uppercase;
}

.status-stopped {
  background: var(--status-stopped-bg);
  color: var(--status-stopped-text);
  border-color: var(--status-stopped-bg);
}

.status-starting {
  background: var(--status-starting-bg);
  color: var(--status-starting-text);
  border-color: var(--status-starting-bg);
}

.status-running {
  background: var(--status-running-bg);
  color: var(--status-running-text);
  border-color: var(--status-running-bg);
}

.status-stopping {
  background: var(--status-stopping-bg);
  color: var(--status-stopping-text);
  border-color: var(--status-stopping-bg);
}

.metrics-row {
  display: flex;
  gap: 1rem;
  padding: 0.75rem;
  background: var(--bg-input);
  border: 2px solid var(--border);
  border-radius: 2px;
}

.metric {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1;
}

.metric-label {
  font-family: 'Press Start 2P', monospace;
  font-size: 0.4rem;
  color: var(--text-muted);
  text-transform: uppercase;
  margin-bottom: 0.25rem;
}

.metric-value {
  font-family: 'Press Start 2P', monospace;
  font-size: 0.625rem;
  color: var(--accent);
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
  color: var(--text-muted);
  font-size: 0.875rem;
}

.info-value {
  font-weight: 500;
}

.connection-url {
  font-family: monospace;
  color: var(--accent);
}

.action-section {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid var(--border);
}

.action-section:first-of-type {
  border-top: none;
  padding-top: 0;
}

.section-label {
  font-size: 0.35rem;
  color: var(--text-muted);
  text-transform: uppercase;
  min-width: 4.5rem;
  padding-top: 0.375rem;
  flex-shrink: 0;
}

.action-buttons {
  display: flex;
  gap: 0.375rem;
  flex-wrap: wrap;
  flex: 1;
}

.btn-sm {
  padding: 0.375rem 0.75rem;
  font-size: 0.5rem;
}
</style>
