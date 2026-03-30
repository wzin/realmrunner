<template>
  <div class="share-page">
    <div class="container">
      <div v-if="loading" class="loading">Loading...</div>
      <div v-else-if="error" class="error-state">
        <h1 class="pixel-font">Invalid Link</h1>
        <p>This share link is invalid or has been revoked.</p>
      </div>
      <template v-else>
        <header class="share-header">
          <h1 class="title pixel-font">{{ server.name }}</h1>
          <div class="header-meta">
            <span class="flavor-badge">{{ server.flavor }}</span>
            <span>{{ server.version }}</span>
            <span :class="['status-badge', `status-${server.status}`]">{{ server.status }}</span>
          </div>
        </header>

        <div v-if="server.metrics" class="metrics-row">
          <div class="metric-card card">
            <span class="metric-label pixel-font">CPU</span>
            <span class="metric-value">{{ server.metrics.cpu_percent.toFixed(1) }}%</span>
          </div>
          <div class="metric-card card">
            <span class="metric-label pixel-font">RAM</span>
            <span class="metric-value">{{ server.metrics.memory_mb.toFixed(0) }} MB</span>
          </div>
          <div class="metric-card card">
            <span class="metric-label pixel-font">Players</span>
            <span class="metric-value">{{ server.metrics.player_count }}</span>
            <span v-if="server.metrics.player_names && server.metrics.player_names.length" class="player-list">
              {{ server.metrics.player_names.join(', ') }}
            </span>
          </div>
        </div>

        <div class="console-section card">
          <h2 class="section-title pixel-font">Console</h2>
          <div class="log-output">
            <div v-if="!logs.length" class="empty-logs">
              {{ server.status === 'running' ? 'Waiting for logs...' : 'Server is stopped' }}
            </div>
            <div v-for="(log, i) in logs" :key="i" class="log-line">
              <span class="log-time">{{ log.timestamp }}</span>
              <span class="log-message">{{ log.message }}</span>
            </div>
          </div>
        </div>

        <p class="share-footer">Powered by RealmRunner</p>
      </template>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'

const props = defineProps({ token: { type: String, required: true } })

const server = ref(null)
const loading = ref(true)
const error = ref(false)
const logs = ref([])
let ws = null
let refreshInterval = null

async function loadServer() {
  try {
    const resp = await fetch(`/api/share/${props.token}`)
    if (!resp.ok) throw new Error()
    server.value = await resp.json()
    error.value = false
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}

function connectWs() {
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  ws = new WebSocket(`${proto}//${window.location.host}/api/share/${props.token}/ws`)

  ws.onmessage = (event) => {
    const data = JSON.parse(event.data)
    if (data.type === 'log') {
      logs.value.push({
        timestamp: new Date(data.timestamp).toLocaleTimeString(),
        message: data.message,
      })
      if (logs.value.length > 200) logs.value.shift()
    } else if (data.type === 'metrics') {
      if (server.value) {
        server.value.metrics = {
          cpu_percent: data.cpu_percent,
          memory_mb: data.memory_mb,
          player_count: data.player_count,
          player_names: data.player_names || [],
        }
      }
    }
  }

  ws.onclose = () => {
    setTimeout(connectWs, 5000)
  }
}

onMounted(async () => {
  await loadServer()
  if (!error.value) {
    connectWs()
    refreshInterval = setInterval(loadServer, 30000)
  }
})

onUnmounted(() => {
  if (ws) ws.close()
  if (refreshInterval) clearInterval(refreshInterval)
})
</script>

<style scoped>
.share-page {
  min-height: 100vh;
  padding: 2rem 0;
}

.loading { text-align: center; padding: 4rem; color: var(--text-muted); }
.error-state { text-align: center; padding: 4rem; }
.error-state h1 { color: var(--danger); margin-bottom: 1rem; font-size: 1rem; }
.error-state p { color: var(--text-muted); }

.share-header { margin-bottom: 1.5rem; }
.title { font-size: 1.25rem; color: var(--accent); text-shadow: 2px 2px 0 var(--border-shadow); margin-bottom: 0.5rem; }
.header-meta { display: flex; gap: 0.75rem; align-items: center; color: var(--text-muted); font-size: 0.875rem; }

.flavor-badge {
  font-family: 'Press Start 2P', monospace; font-size: 0.4rem;
  background: var(--accent); color: var(--accent-text);
  padding: 0.125rem 0.375rem; border-radius: 2px; text-transform: uppercase;
}

.status-badge {
  font-family: 'Press Start 2P', monospace; font-size: 0.4rem;
  padding: 0.125rem 0.5rem; border-radius: 2px; text-transform: uppercase;
}
.status-running { background: var(--status-running-bg); color: var(--status-running-text); }
.status-stopped { background: var(--status-stopped-bg); color: var(--status-stopped-text); }

.metrics-row { display: flex; gap: 1rem; margin-bottom: 1.5rem; }
.metric-card { flex: 1; text-align: center; padding: 1rem; }
.metric-label { font-size: 0.4rem; color: var(--text-muted); display: block; margin-bottom: 0.25rem; }
.metric-value { font-size: 1.25rem; font-weight: 700; color: var(--accent); }
.player-list { display: block; font-size: 0.75rem; color: var(--text-muted); margin-top: 0.25rem; }

.console-section { margin-bottom: 1.5rem; }
.section-title { font-size: 0.625rem; margin-bottom: 0.75rem; }

.log-output {
  background: var(--bg-input); border: 2px solid var(--border); border-radius: 2px;
  padding: 0.75rem; height: 400px; overflow-y: auto;
  font-family: 'Courier New', monospace; font-size: 0.8125rem;
}
.empty-logs { color: var(--text-muted); text-align: center; padding: 2rem; }
.log-line { margin-bottom: 0.125rem; }
.log-time { color: var(--text-muted); margin-right: 0.5rem; }
.log-message { color: var(--text-primary); }

.share-footer { text-align: center; color: var(--text-muted); font-size: 0.75rem; margin-top: 2rem; }
</style>
