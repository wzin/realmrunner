<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal">
      <div class="modal-header pixel-font">Share Server</div>
      <p class="subtitle">{{ server.name }}</p>

      <div v-if="error" class="alert alert-error">{{ error }}</div>

      <div v-if="server.share_token" class="share-active">
        <p class="share-label">Public share link (read-only, no login needed):</p>
        <div class="share-url-row">
          <input ref="urlInput" :value="shareUrl" class="input" readonly @click="selectUrl" />
          <button @click="copyUrl" class="btn btn-primary btn-sm">{{ copied ? 'Copied!' : 'Copy' }}</button>
        </div>
        <p class="share-note">Anyone with this link can view server status, metrics, and console logs.</p>
        <button @click="revoke" class="btn btn-danger btn-sm" style="margin-top:1rem" :disabled="revoking">
          {{ revoking ? 'Revoking...' : 'Revoke Link' }}
        </button>
      </div>

      <div v-else class="share-inactive">
        <p class="share-desc">Generate a public link that allows anonymous read-only access to this server's console and metrics.</p>
        <button @click="generate" class="btn btn-primary" :disabled="generating">
          {{ generating ? 'Generating...' : 'Generate Share Link' }}
        </button>
      </div>

      <div class="modal-actions">
        <button @click="$emit('close')" class="btn btn-secondary">Close</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
const emit = defineEmits(['close', 'updated'])

const error = ref('')
const generating = ref(false)
const revoking = ref(false)
const copied = ref(false)
const urlInput = ref(null)

const shareUrl = computed(() => {
  if (!props.server.share_token) return ''
  return `${window.location.origin}/share/${props.server.share_token}`
})

function selectUrl() {
  if (urlInput.value) urlInput.value.select()
}

async function copyUrl() {
  try {
    await navigator.clipboard.writeText(shareUrl.value)
    copied.value = true
    setTimeout(() => copied.value = false, 2000)
  } catch { /* fallback: user can manually copy from input */ }
}

async function generate() {
  generating.value = true
  error.value = ''
  try {
    await api.generateShareLink(props.server.id)
    emit('updated')
  } catch (err) {
    error.value = err.message || 'Failed to generate link'
  } finally {
    generating.value = false
  }
}

async function revoke() {
  if (!confirm('Revoke this share link? Anyone using it will lose access.')) return
  revoking.value = true
  error.value = ''
  try {
    await api.revokeShareLink(props.server.id)
    emit('updated')
  } catch (err) {
    error.value = err.message || 'Failed to revoke link'
  } finally {
    revoking.value = false
  }
}
</script>

<style scoped>
.subtitle { color: var(--text-muted); margin-bottom: 1.5rem; font-size: 0.875rem; }
.share-label { font-size: 0.75rem; color: var(--text-muted); margin-bottom: 0.5rem; }
.share-url-row { display: flex; gap: 0.5rem; }
.share-url-row .input { flex: 1; font-family: monospace; font-size: 0.75rem; }
.share-note { font-size: 0.6875rem; color: var(--text-muted); margin-top: 0.5rem; }
.share-desc { color: var(--text-muted); margin-bottom: 1rem; font-size: 0.875rem; }
.modal-actions { display: flex; justify-content: flex-end; margin-top: 1.5rem; }
</style>
