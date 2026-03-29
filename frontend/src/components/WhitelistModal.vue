<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">
        <span>{{ server.name }} - Players</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div class="tabs">
        <button :class="['tab', { active: tab === 'whitelist' }]" @click="tab = 'whitelist'; loadList()">Whitelist</button>
        <button :class="['tab', { active: tab === 'ops' }]" @click="tab = 'ops'; loadList()">Operators</button>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>

      <div class="add-form">
        <input v-model="newPlayer" type="text" class="input" placeholder="Player username" @keyup.enter="addPlayer" :disabled="adding" />
        <button @click="addPlayer" class="btn btn-primary btn-sm" :disabled="!newPlayer.trim() || adding">
          {{ adding ? '...' : 'Add' }}
        </button>
      </div>

      <div class="player-list">
        <div v-if="loading" class="loading">Loading...</div>
        <div v-else-if="!players.length" class="empty">No players in {{ tab }}</div>
        <div v-for="p in players" :key="p.uuid" class="player-item">
          <span class="player-name">{{ p.name }}</span>
          <span class="player-uuid">{{ p.uuid }}</span>
          <button @click="removePlayer(p)" class="btn btn-danger btn-sm remove-btn">X</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
defineEmits(['close'])

const tab = ref('whitelist')
const players = ref([])
const loading = ref(false)
const error = ref('')
const newPlayer = ref('')
const adding = ref(false)

async function loadList() {
  loading.value = true
  error.value = ''
  try {
    const resp = tab.value === 'whitelist'
      ? await api.getWhitelist(props.server.id)
      : await api.getOps(props.server.id)
    players.value = resp.players || []
  } catch (err) {
    error.value = err.message || 'Failed to load'
  } finally {
    loading.value = false
  }
}

async function addPlayer() {
  if (!newPlayer.value.trim()) return
  adding.value = true
  error.value = ''
  try {
    const resp = tab.value === 'whitelist'
      ? await api.addToWhitelist(props.server.id, newPlayer.value.trim())
      : await api.addOp(props.server.id, newPlayer.value.trim())
    if (resp.players) players.value = resp.players
    else await loadList()
    newPlayer.value = ''
  } catch (err) {
    error.value = err.message || 'Failed to add player'
  } finally {
    adding.value = false
  }
}

async function removePlayer(p) {
  error.value = ''
  try {
    const resp = tab.value === 'whitelist'
      ? await api.removeFromWhitelist(props.server.id, p.uuid)
      : await api.removeOp(props.server.id, p.uuid)
    if (resp.players) players.value = resp.players
    else await loadList()
  } catch (err) {
    error.value = err.message || 'Failed to remove player'
  }
}

onMounted(loadList)
</script>

<style scoped>
.close-btn {
  background: none; border: none; color: var(--text-primary);
  font-size: 2rem; line-height: 1; cursor: pointer; padding: 0; font-family: inherit;
}
.close-btn:hover { color: var(--accent); }
.modal-header { display: flex; justify-content: space-between; align-items: center; }

.tabs { display: flex; gap: 0.5rem; margin-bottom: 1rem; }
.tab {
  padding: 0.375rem 0.75rem; border: 2px solid var(--border); border-radius: 2px;
  background: var(--bg-input); color: var(--text-muted); cursor: pointer;
  font-family: 'Press Start 2P', monospace; font-size: 0.5rem;
}
.tab.active { background: var(--accent); color: var(--accent-text); border-color: var(--accent); }

.add-form { display: flex; gap: 0.5rem; margin-bottom: 1rem; }
.add-form .input { flex: 1; }

.player-list { max-height: 300px; overflow-y: auto; }
.loading, .empty { text-align: center; padding: 2rem; color: var(--text-muted); font-size: 0.875rem; }

.player-item {
  display: flex; align-items: center; gap: 0.5rem; padding: 0.5rem;
  border-bottom: 1px solid var(--border);
}
.player-name { font-weight: 500; flex: 1; }
.player-uuid { font-size: 0.625rem; color: var(--text-muted); font-family: monospace; }
.remove-btn { padding: 0.25rem 0.5rem; font-size: 0.5rem; }
</style>
