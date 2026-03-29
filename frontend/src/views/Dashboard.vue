<template>
  <div class="dashboard">
    <header class="header">
      <div class="container">
        <div class="header-content">
          <div>
            <h1 class="title pixel-font">RealmRunner</h1>
            <p class="subtitle">Minecraft Server Manager</p>
          </div>
          <div class="header-right">
            <ThemeSwitcher />
            <button @click="handleLogout" class="btn btn-secondary">Logout</button>
          </div>
        </div>
      </div>
    </header>

    <main class="container">
      <div class="actions">
        <button @click="showCreateModal = true" class="btn btn-primary">
          Create Server
        </button>
        <button @click="loadServers" class="btn btn-secondary" :disabled="loading">
          Refresh
        </button>
      </div>

      <div v-if="error" class="alert alert-error">
        {{ error }}
      </div>

      <div v-if="loading && servers.length === 0" class="loading">
        Loading servers...
      </div>

      <div v-else-if="servers.length === 0" class="empty-state">
        <p>No servers yet. Create your first server to get started!</p>
      </div>

      <div v-else class="server-grid">
        <ServerCard
          v-for="server in servers"
          :key="server.id"
          :server="server"
          @refresh="loadServers"
          @console="openConsole"
          @metrics="openMetrics"
        />
      </div>
    </main>

    <CreateModal
      v-if="showCreateModal"
      @close="showCreateModal = false"
      @created="handleServerCreated"
    />

    <ConsoleModal
      v-if="selectedServer"
      :server="selectedServer"
      @close="selectedServer = null"
    />

    <MetricsModal
      v-if="metricsServer"
      :server="metricsServer"
      @close="metricsServer = null"
    />
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import ServerCard from '../components/ServerCard.vue'
import CreateModal from '../components/CreateModal.vue'
import ConsoleModal from '../components/ConsoleModal.vue'
import MetricsModal from '../components/MetricsModal.vue'
import ThemeSwitcher from '../components/ThemeSwitcher.vue'

const router = useRouter()
const servers = ref([])
const loading = ref(false)
const error = ref('')
const showCreateModal = ref(false)
const selectedServer = ref(null)
const metricsServer = ref(null)

async function loadServers() {
  loading.value = true
  error.value = ''

  try {
    servers.value = await api.getServers()
  } catch (err) {
    error.value = err.message || 'Failed to load servers'
  } finally {
    loading.value = false
  }
}

function handleLogout() {
  localStorage.removeItem('token')
  router.push('/login')
}

function handleServerCreated() {
  showCreateModal.value = false
  loadServers()
}

function openConsole(server) {
  selectedServer.value = server
}

function openMetrics(server) {
  metricsServer.value = server
}

onMounted(() => {
  loadServers()
  // Refresh servers every 5 seconds
  setInterval(loadServers, 5000)
})
</script>

<style scoped>
.header {
  background: var(--bg-header);
  border-bottom: 2px solid var(--border);
  padding: 1.5rem 0;
  margin-bottom: 2rem;
  box-shadow: 0 2px 0 var(--border-shadow);
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 1rem;
}

.title {
  font-size: 1.125rem;
  color: var(--accent);
  text-shadow: 2px 2px 0 var(--border-shadow);
}

.subtitle {
  color: var(--text-muted);
  margin-top: 0.25rem;
  font-size: 0.75rem;
}

.actions {
  display: flex;
  gap: 1rem;
  margin-bottom: 2rem;
}

.loading, .empty-state {
  text-align: center;
  padding: 3rem;
  color: var(--text-muted);
}

.server-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 1.5rem;
}
</style>
