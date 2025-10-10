<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal modal-large">
      <div class="modal-header">
        <span>{{ server.name }} - Console</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div class="console-container">
        <div ref="logContainer" class="log-output">
          <div v-if="logs.length === 0" class="empty-logs">
            Waiting for logs...
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
  if (props.server.status === 'running') {
    connectWebSocket()
  }
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
  color: #e2e8f0;
  font-size: 2rem;
  line-height: 1;
  cursor: pointer;
  padding: 0;
  width: 2rem;
  height: 2rem;
}

.close-btn:hover {
  color: #3b82f6;
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
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 0.375rem;
  padding: 1rem;
  overflow-y: auto;
  font-family: 'Courier New', monospace;
  font-size: 0.875rem;
}

.empty-logs {
  color: #64748b;
  text-align: center;
  padding: 2rem;
}

.log-line {
  margin-bottom: 0.25rem;
  line-height: 1.5;
}

.log-time {
  color: #64748b;
  margin-right: 0.5rem;
}

.log-message {
  color: #e2e8f0;
}

.command-form {
  display: flex;
  gap: 0.5rem;
}

.command-form .input {
  flex: 1;
}
</style>
