<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal modal-wide">
      <div class="modal-header pixel-font">Connection Health</div>
      <p class="subtitle">{{ server.name }} &mdash; read from this server's log</p>

      <div v-if="error" class="alert alert-error">{{ error }}</div>
      <p v-if="loading" class="muted">Reading the log...</p>

      <template v-if="diag">
        <div class="summary-row">
          <div class="summary">
            <span class="summary-value">{{ totalDisconnects }}</span>
            <span class="summary-label">disconnects</span>
          </div>
          <div class="summary">
            <span class="summary-value">{{ diag.lag_warnings }}</span>
            <span class="summary-label">lag warnings</span>
          </div>
          <div class="summary">
            <span class="summary-value">{{ diag.worst_lag_ms }}ms</span>
            <span class="summary-label">worst stall</span>
          </div>
          <div class="summary">
            <span class="summary-value">{{ diag.restarts }}</span>
            <span class="summary-label">server starts</span>
          </div>
        </div>

        <ul v-if="diag.notes && diag.notes.length" class="notes">
          <li v-for="(note, i) in diag.notes" :key="i">{{ note }}</li>
        </ul>

        <template v-if="diag.memory">
          <h4 class="section-title">Memory</h4>
          <div class="summary-row">
            <div class="summary">
              <span class="summary-value">{{ diag.memory.live_set_mb }} MB</span>
              <span class="summary-label">needed after a collection</span>
            </div>
            <div class="summary">
              <span class="summary-value">{{ diag.memory.heap_mb }} MB</span>
              <span class="summary-label">heap given</span>
            </div>
            <div class="summary">
              <span class="summary-value">{{ diag.memory.used_percent }}%</span>
              <span class="summary-label">of heap in use</span>
            </div>
            <div class="summary">
              <span class="summary-value">{{ diag.memory.longest_pause_ms }}ms</span>
              <span class="summary-label">longest GC pause</span>
            </div>
          </div>
          <p class="verdict">
            {{ diag.memory.verdict }}
            <strong v-if="diag.memory.recommendation"> {{ diag.memory.recommendation }}</strong>
          </p>
          <p class="meaning">
            Resident memory is not a useful signal here: the heap is committed when the server
            starts, so the process is the same size busy or idle. What counts is how much survives
            a collection.
          </p>
        </template>

        <h4 v-if="diag.disconnects && diag.disconnects.length" class="section-title">Why players dropped</h4>
        <div v-if="diag.disconnects && diag.disconnects.length" class="table-scroll">
        <table class="table">
          <tbody>
            <tr v-for="d in diag.disconnects" :key="d.reason">
              <td class="count-cell">{{ d.count }}&times;</td>
              <td>
                <span class="reason">{{ d.reason }}</span>
                <p v-if="d.meaning" class="meaning">{{ d.meaning }}</p>
              </td>
            </tr>
          </tbody>
        </table>
        </div>

        <h4 v-if="diag.players && diag.players.length" class="section-title">Per player</h4>
        <div v-if="diag.players && diag.players.length" class="table-scroll">
        <table class="table">
          <thead>
            <tr><th>Player</th><th>Drops</th><th>Logins</th><th>Address</th></tr>
          </thead>
          <tbody>
            <tr v-for="p in diag.players" :key="p.name">
              <td>{{ p.name }}</td>
              <td>{{ p.disconnects }}</td>
              <td>{{ p.logins }}</td>
              <td class="address">{{ (p.ips || []).join(', ') }}</td>
            </tr>
          </tbody>
        </table>
        </div>

        <p v-if="!totalDisconnects" class="muted">No disconnects found in the current log.</p>
      </template>

      <div class="modal-actions">
        <button type="button" @click="load" class="btn btn-secondary" :disabled="loading">Refresh</button>
        <button type="button" @click="$emit('close')" class="btn btn-secondary">Close</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
defineEmits(['close'])

const diag = ref(null)
const loading = ref(false)
const error = ref('')

const totalDisconnects = computed(() =>
  (diag.value?.disconnects || []).reduce((sum, d) => sum + d.count, 0)
)

async function load() {
  loading.value = true
  error.value = ''
  try {
    diag.value = await api.getDiagnostics(props.server.id)
  } catch (err) {
    error.value = err.message || 'Could not read the log'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.subtitle { color: var(--text-muted); margin-bottom: 1rem; font-size: 0.875rem; }
.muted { color: var(--text-muted); font-size: 0.875rem; }
.modal-wide { max-width: 44rem; }
.summary-row { display: flex; gap: 1.5rem; flex-wrap: wrap; margin-bottom: 1rem; }
.table { min-width: 22rem; }
.summary { display: flex; flex-direction: column; }
.summary-value { font-size: 1.5rem; font-weight: 700; }
.summary-label { font-size: 0.75rem; color: var(--text-muted); }
.notes { margin: 0 0 1rem; padding-left: 1.1rem; }
.notes li { font-size: 0.8125rem; margin-bottom: 0.4rem; }
.section-title { margin: 1.25rem 0 0.5rem; font-size: 0.875rem; }
.table { width: 100%; border-collapse: collapse; font-size: 0.8125rem; }
.table th { text-align: left; color: var(--text-muted); font-weight: 600; padding: 0.25rem 0.5rem 0.25rem 0; }
.table td { padding: 0.4rem 0.5rem 0.4rem 0; border-top: 1px solid var(--border); vertical-align: top; }
.count-cell { white-space: nowrap; font-weight: 700; width: 3rem; }
.reason { font-weight: 600; }
.meaning { margin: 0.15rem 0 0; color: var(--text-muted); }
.address { font-family: monospace; }
.verdict { font-size: 0.8125rem; margin: 0.25rem 0; }
.modal-actions { display: flex; gap: 1rem; justify-content: flex-end; margin-top: 1.5rem; }
</style>
