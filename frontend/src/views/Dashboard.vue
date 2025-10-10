<template>
  <div class="dashboard">
    <header class="header">
      <div class="container">
        <div class="header-content">
          <div>
            <h1 class="title">RealmRunner</h1>
            <p class="subtitle">Minecraft Server Manager</p>
          </div>
          <button @click="handleLogout" class="btn btn-secondary">Logout</button>
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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'
import ServerCard from '../components/ServerCard.vue'
import CreateModal from '../components/CreateModal.vue'
import ConsoleModal from '../components/ConsoleModal.vue'

const router = useRouter()
const servers = ref([])
const loading = ref(false)
const error = ref('')
const showCreateModal = ref(false)
const selectedServer = ref(null)

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

onMounted(() => {
  loadServers()
  // Refresh servers every 5 seconds
  setInterval(loadServers, 5000)
})
</script>

<style scoped>
.header {
  background: #1e293b;
  border-bottom: 1px solid #334155;
  padding: 1.5rem 0;
  margin-bottom: 2rem;
}

.header-content {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.title {
  font-size: 1.875rem;
  font-weight: 700;
  color: #3b82f6;
}

.subtitle {
  color: #94a3b8;
  margin-top: 0.25rem;
}

.actions {
  display: flex;
  gap: 1rem;
  margin-bottom: 2rem;
}

.loading, .empty-state {
  text-align: center;
  padding: 3rem;
  color: #94a3b8;
}

.server-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
  gap: 1.5rem;
}
</style>
