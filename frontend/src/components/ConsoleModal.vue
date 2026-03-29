<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal modal-large">
      <div class="modal-header pixel-font">
        <span>{{ server.name }} - Console</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div class="console-container">
        <div ref="logContainer" class="log-output">
          <div v-if="logs.length === 0" class="empty-logs">
            <span v-if="server.status === 'stopped'">No logs available. Start the server to generate logs.</span>
            <span v-else>Waiting for logs...</span>
          </div>
          <div
            v-for="(log, index) in logs"
            :key="index"
            class="log-line"
          >
            <span class="log-time">{{ log.timestamp }}</span>
            <span class="log-message">{{ log.message }}</span>
          </div>
        </div>

        <form @submit.prevent="sendCommand" class="command-form">
          <input
            v-model="command"
            type="text"
            class="input"
            placeholder="Enter command (e.g., /say Hello)"
            :disabled="server.status !== 'running'"
          />
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="server.status !== 'running' || !command.trim()"
          >
            Send
          </button>
        </form>

        <div v-if="error" class="alert alert-error">
          {{ error }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'
import { api, createWebSocket } from '../api/client'

const props = defineProps({
  server: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['close'])

const logs = ref([])
const command = ref('')
const error = ref('')
const logContainer = ref(null)
let ws = null

function connectWebSocket() {
  try {
    ws = createWebSocket(props.server.id)

    ws.onmessage = (event) => {
      const data = JSON.parse(event.data)

      if (data.type === 'log') {
        logs.value.push({
          timestamp: new Date(data.timestamp).toLocaleTimeString(),
          message: data.message
        })

        // Auto-scroll to bottom
        nextTick(() => {
          if (logContainer.value) {
            logContainer.value.scrollTop = logContainer.value.scrollHeight
          }
        })

        // Limit logs to last 500 lines
        if (logs.value.length > 500) {
          logs.value.shift()
        }
      } else if (data.type === 'status') {
        // Update server status if needed
      }
    }

    ws.onerror = (err) => {
      error.value = 'WebSocket connection error'
      console.error('WebSocket error:', err)
    }

    ws.onclose = () => {
      console.log('WebSocket closed')
    }
  } catch (err) {
    error.value = 'Failed to connect to server'
    console.error('WebSocket connection failed:', err)
  }
}

async function sendCommand() {
  if (!command.value.trim()) return

  error.value = ''

  try {
    await api.sendCommand(props.server.id, command.value)
    command.value = ''
  } catch (err) {
    error.value = err.message || 'Failed to send command'
  }
}

onMounted(() => {
  // Connect WebSocket for both running and stopped servers
  // Stopped servers will show historical logs
  connectWebSocket()
})

onUnmounted(() => {
  if (ws) {
    ws.close()
  }
})
</script>

<style scoped>
.modal-large {
  max-width: 800px;
  height: 80vh;
  display: flex;
  flex-direction: column;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.close-btn {
  background: none;
  border: none;
  color: var(--text-primary);
  font-size: 2rem;
  line-height: 1;
  cursor: pointer;
  padding: 0;
  width: 2rem;
  height: 2rem;
  font-family: inherit;
}

.close-btn:hover {
  color: var(--accent);
}

.console-container {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  min-height: 0;
}

.log-output {
  flex: 1;
  background: var(--bg-input);
  border: 2px solid var(--border);
  border-radius: 2px;
  padding: 1rem;
  overflow-y: auto;
  font-family: 'Courier New', monospace;
  font-size: 0.875rem;
  box-shadow:
    inset 2px 2px 0 rgba(0,0,0,0.2),
    inset -2px -2px 0 rgba(255,255,255,0.05);
}

.empty-logs {
  color: var(--text-muted);
  text-align: center;
  padding: 2rem;
}

.log-line {
  margin-bottom: 0.25rem;
  line-height: 1.5;
}

.log-time {
  color: var(--text-muted);
  margin-right: 0.5rem;
}

.log-message {
  color: var(--text-primary);
}

.command-form {
  display: flex;
  gap: 0.5rem;
}

.command-form .input {
  flex: 1;
}
</style>
