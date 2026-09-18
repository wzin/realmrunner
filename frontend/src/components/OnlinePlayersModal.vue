<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">Who's Online</div>
      <p class="subtitle">{{ server.name }}</p>

      <div v-if="error" class="alert alert-error">{{ error }}</div>

      <div v-if="players" class="count-row">
        <span class="count">{{ players.online }} / {{ players.max }}</span>
        <span class="count-label">players online</span>
        <button @click="load" class="btn btn-secondary btn-sm" :disabled="loading">Refresh</button>
      </div>

      <p v-if="players && players.names.length === 0" class="empty">Nobody is online right now.</p>

      <ul v-if="players" class="player-list">
        <li v-for="name in players.names" :key="name" class="player">
          <span class="player-name">{{ name }}</span>
          <span class="player-actions">
            <button @click="act('kick', name)" class="btn btn-warning btn-sm" :disabled="busy">Kick</button>
            <button @click="act('ban', name)" class="btn btn-danger btn-sm" :disabled="busy">Ban</button>
            <button @click="act('op', name)" class="btn btn-secondary btn-sm" :disabled="busy">Op</button>
            <button @click="act('deop', name)" class="btn btn-secondary btn-sm" :disabled="busy">Deop</button>
          </span>
        </li>
      </ul>

      <p v-if="message" class="server-reply">{{ message }}</p>

      <div class="modal-actions">
        <button type="button" @click="$emit('close')" class="btn btn-secondary">Close</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
defineEmits(['close'])

const players = ref(null)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const message = ref('')
let timer = null

async function load() {
  loading.value = true
  try {
    players.value = await api.getPlayers(props.server.id)
    error.value = ''
  } catch (err) {
    error.value = err.message || 'Could not reach the server'
  } finally {
    loading.value = false
  }
}

const actions = {
  kick: (id, name) => api.kickPlayer(id, name, 'Kicked by an operator'),
  ban: (id, name) => api.banPlayer(id, name, 'Banned by an operator'),
  op: (id, name) => api.opPlayer(id, name),
  deop: (id, name) => api.deopPlayer(id, name),
}

async function act(action, name) {
  if ((action === 'ban') && !confirm(`Ban ${name} from ${props.server.name}?`)) return

  busy.value = true
  message.value = ''
  try {
    const result = await actions[action](props.server.id, name)
    message.value = result.message || `${action} sent`
    await load()
  } catch (err) {
    error.value = err.message || `Failed to ${action} ${name}`
  } finally {
    busy.value = false
  }
}

onMounted(() => {
  load()
  timer = setInterval(load, 10000)
})
onUnmounted(() => clearInterval(timer))
</script>

<style scoped>
.subtitle { color: var(--text-muted); margin-bottom: 1rem; font-size: 0.875rem; }
.count-row { display: flex; align-items: baseline; gap: 0.5rem; margin-bottom: 1rem; }
.count { font-size: 1.5rem; font-weight: 700; }
.count-label { color: var(--text-muted); font-size: 0.875rem; flex: 1; }
.empty { color: var(--text-muted); font-size: 0.875rem; }
.player-list { list-style: none; padding: 0; margin: 0; }
.player { display: flex; align-items: center; justify-content: space-between; gap: 0.5rem; padding: 0.5rem 0; border-bottom: 1px solid var(--border); flex-wrap: wrap; }
.player-name { font-weight: 600; }
.player-actions { display: flex; gap: 0.25rem; flex-wrap: wrap; }

@media (max-width: 480px) {
  .player { align-items: flex-start; flex-direction: column; gap: 0.35rem; }
  .player-actions { width: 100%; }
  .player-actions .btn { flex: 1; }
}
.server-reply { margin-top: 1rem; font-size: 0.8125rem; color: var(--text-muted); white-space: pre-wrap; }
.modal-actions { display: flex; gap: 1rem; justify-content: flex-end; margin-top: 1.5rem; }
</style>
