<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal modal-large">
      <div class="modal-header pixel-font">
        <span>{{ server.name }} - Mods &amp; Plugins</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div class="tabs">
        <button :class="['tab', { active: tab === 'installed' }]" @click="tab = 'installed'">Installed</button>
        <button :class="['tab', { active: tab === 'search' }]" @click="tab = 'search'">Search Modrinth</button>
      </div>

      <div v-if="error" class="alert alert-error">{{ error }}</div>
      <div v-if="success" class="alert alert-success">{{ success }}</div>

      <!-- Installed Tab -->
      <div v-if="tab === 'installed'" class="tab-content">
        <div v-if="loadingInstalled" class="loading">Loading...</div>
        <div v-else-if="!installed.length" class="empty">No mods installed</div>
        <div v-for="m in installed" :key="m.id" class="mod-item">
          <div class="mod-info">
            <span class="mod-name">{{ m.name }}</span>
            <span class="mod-meta">v{{ m.version }} &middot; {{ m.filename }}</span>
          </div>
          <button @click="removeMod(m)" class="btn btn-danger btn-sm">Remove</button>
        </div>
      </div>

      <!-- Search Tab -->
      <div v-if="tab === 'search'" class="tab-content">
        <div class="search-bar">
          <input v-model="searchQuery" class="input" placeholder="Search mods..." @keyup.enter="doSearch" :disabled="searching" />
          <button @click="doSearch" class="btn btn-primary btn-sm" :disabled="!searchQuery.trim() || searching">
            {{ searching ? '...' : 'Search' }}
          </button>
        </div>

        <div v-if="searchResults.length" class="search-results">
          <div v-for="r in searchResults" :key="r.slug" class="mod-item search-item">
            <div class="mod-info">
              <div class="mod-title-row">
                <img v-if="r.icon_url" :src="r.icon_url" class="mod-icon" />
                <span class="mod-name">{{ r.title }}</span>
                <span class="mod-downloads">{{ formatDownloads(r.downloads) }}</span>
              </div>
              <span class="mod-desc">{{ r.description }}</span>
            </div>
            <div class="mod-actions">
              <button v-if="!showVersions[r.slug]" @click="loadVersions(r)" class="btn btn-primary btn-sm">
                Install
              </button>
              <div v-else class="version-picker">
                <select v-model="selectedVersion[r.slug]" class="input input-sm">
                  <option value="">Select version</option>
                  <option v-for="v in modVersions[r.slug]" :key="v.id" :value="v.id">
                    {{ v.version_number }} ({{ v.game_versions.join(', ') }})
                  </option>
                </select>
                <button @click="installMod(r)" class="btn btn-success btn-sm" :disabled="!selectedVersion[r.slug] || installing">
                  {{ installing ? '...' : 'Go' }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div class="mod-warning">Server restart required to load/unload mods.</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
defineEmits(['close'])

const tab = ref('installed')
const installed = ref([])
const loadingInstalled = ref(false)
const error = ref('')
const success = ref('')

// Search
const searchQuery = ref('')
const searchResults = ref([])
const searching = ref(false)
const installing = ref(false)
const showVersions = reactive({})
const modVersions = reactive({})
const selectedVersion = reactive({})

async function loadInstalled() {
  loadingInstalled.value = true
  try {
    const resp = await api.getMods(props.server.id)
    installed.value = resp.mods || []
  } catch (err) {
    error.value = err.message || 'Failed to load mods'
  } finally {
    loadingInstalled.value = false
  }
}

async function doSearch() {
  if (!searchQuery.value.trim()) return
  searching.value = true
  error.value = ''
  try {
    const resp = await api.searchMods(props.server.id, searchQuery.value, 20)
    searchResults.value = resp.hits || []
  } catch (err) {
    error.value = err.message || 'Search failed'
  } finally {
    searching.value = false
  }
}

async function loadVersions(mod) {
  showVersions[mod.slug] = true
  try {
    const resp = await api.getModVersions(props.server.id, mod.slug)
    modVersions[mod.slug] = resp.versions || []
  } catch (err) {
    error.value = 'Failed to load versions'
  }
}

async function installMod(mod) {
  const versionId = selectedVersion[mod.slug]
  if (!versionId) return
  installing.value = true
  error.value = ''
  success.value = ''
  try {
    await api.installMod(props.server.id, mod.slug, versionId)
    success.value = `Installed ${mod.title}`
    showVersions[mod.slug] = false
    await loadInstalled()
  } catch (err) {
    error.value = err.message || 'Failed to install mod'
  } finally {
    installing.value = false
  }
}

async function removeMod(mod) {
  if (!confirm(`Remove ${mod.name}?`)) return
  error.value = ''
  try {
    await api.removeMod(props.server.id, mod.id)
    await loadInstalled()
  } catch (err) {
    error.value = err.message || 'Failed to remove mod'
  }
}

function formatDownloads(n) {
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'K'
  return n
}

onMounted(loadInstalled)
</script>

<style scoped>
.modal-large { max-width: 800px; height: 85vh; display: flex; flex-direction: column; }
.modal-header { display: flex; justify-content: space-between; align-items: center; }
.close-btn { background: none; border: none; color: var(--text-primary); font-size: 2rem; line-height: 1; cursor: pointer; padding: 0; font-family: inherit; }
.close-btn:hover { color: var(--accent); }

.tabs { display: flex; gap: 0.5rem; margin-bottom: 1rem; }
.tab { padding: 0.375rem 0.75rem; border: 2px solid var(--border); border-radius: 2px; background: var(--bg-input); color: var(--text-muted); cursor: pointer; font-family: 'Press Start 2P', monospace; font-size: 0.5rem; }
.tab.active { background: var(--accent); color: var(--accent-text); border-color: var(--accent); }

.tab-content { flex: 1; overflow-y: auto; }
.loading, .empty { text-align: center; padding: 2rem; color: var(--text-muted); }

.mod-item { display: flex; justify-content: space-between; align-items: center; padding: 0.75rem; border-bottom: 1px solid var(--border); gap: 0.75rem; }
.search-item { align-items: flex-start; }
.mod-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 0.25rem; }
.mod-title-row { display: flex; align-items: center; gap: 0.5rem; }
.mod-icon { width: 24px; height: 24px; border-radius: 2px; }
.mod-name { font-weight: 600; font-size: 0.875rem; }
.mod-downloads { font-size: 0.625rem; color: var(--text-muted); }
.mod-meta { font-size: 0.6875rem; color: var(--text-muted); font-family: monospace; }
.mod-desc { font-size: 0.75rem; color: var(--text-muted); overflow: hidden; text-overflow: ellipsis; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.mod-actions { flex-shrink: 0; }

.search-bar { display: flex; gap: 0.5rem; margin-bottom: 1rem; }
.search-bar .input { flex: 1; }
.search-results { overflow-y: auto; }

.version-picker { display: flex; gap: 0.375rem; align-items: center; }
.version-picker .input { width: 200px; font-size: 0.75rem; padding: 0.25rem; }

.mod-warning { margin-top: 0.75rem; font-size: 0.6875rem; color: var(--warning); text-align: center; }
</style>
