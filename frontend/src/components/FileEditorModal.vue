<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal modal-large">
      <div class="modal-header pixel-font">
        <span>{{ server.name }} - Config Files</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div class="editor-container">
        <div class="file-list">
          <div
            v-for="f in files"
            :key="f.path"
            :class="['file-item', { active: selectedFile === f.path }]"
            @click="selectFile(f)"
          >
            <span class="file-name">{{ f.name }}</span>
            <span class="file-size">{{ formatSize(f.size) }}</span>
          </div>
          <div v-if="!files.length && !loadingFiles" class="empty-files">No editable files found</div>
        </div>

        <div class="file-editor">
          <div v-if="!selectedFile" class="editor-placeholder">Select a file to edit</div>
          <div v-else-if="loadingContent" class="editor-placeholder">Loading...</div>
          <template v-else>
            <div class="editor-header">
              <span class="editor-path">{{ selectedFile }}</span>
              <div class="editor-actions">
                <span v-if="saved" class="save-indicator">Saved</span>
                <button @click="handleSave" class="btn btn-primary btn-sm" :disabled="saving">
                  {{ saving ? 'Saving...' : 'Save' }}
                </button>
              </div>
            </div>
            <textarea
              v-model="content"
              class="editor-textarea"
              spellcheck="false"
              @input="saved = false"
            ></textarea>
          </template>
          <div v-if="error" class="alert alert-error">{{ error }}</div>
        </div>
      </div>

      <div class="editor-warning">Changes may require a server restart to take effect.</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { api } from '../api/client'

const props = defineProps({ server: { type: Object, required: true } })
defineEmits(['close'])

const files = ref([])
const loadingFiles = ref(true)
const selectedFile = ref(null)
const content = ref('')
const loadingContent = ref(false)
const saving = ref(false)
const saved = ref(false)
const error = ref('')

async function loadFiles() {
  loadingFiles.value = true
  try {
    const resp = await api.getFiles(props.server.id)
    files.value = resp.files || []
  } catch (err) {
    error.value = 'Failed to load files'
  } finally {
    loadingFiles.value = false
  }
}

async function selectFile(f) {
  selectedFile.value = f.path
  loadingContent.value = true
  error.value = ''
  saved.value = false
  try {
    const resp = await api.getFile(props.server.id, f.path)
    content.value = resp.content
  } catch (err) {
    error.value = 'Failed to load file'
    content.value = ''
  } finally {
    loadingContent.value = false
  }
}

async function handleSave() {
  saving.value = true
  error.value = ''
  try {
    await api.saveFile(props.server.id, selectedFile.value, content.value)
    saved.value = true
  } catch (err) {
    error.value = err.message || 'Failed to save file'
  } finally {
    saving.value = false
  }
}

function formatSize(bytes) {
  if (bytes < 1024) return bytes + ' B'
  return (bytes / 1024).toFixed(1) + ' KB'
}

onMounted(loadFiles)
</script>

<style scoped>
.modal-large {
  max-width: 900px;
  height: 85vh;
  display: flex;
  flex-direction: column;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.close-btn {
  background: none; border: none; color: var(--text-primary);
  font-size: 2rem; line-height: 1; cursor: pointer; padding: 0;
  width: 2rem; height: 2rem; font-family: inherit;
}
.close-btn:hover { color: var(--accent); }

.editor-container {
  flex: 1;
  display: flex;
  gap: 1rem;
  min-height: 0;
}

.file-list {
  width: 200px;
  flex-shrink: 0;
  overflow-y: auto;
  border: 2px solid var(--border);
  border-radius: 2px;
  background: var(--bg-input);
}

.file-item {
  padding: 0.5rem 0.75rem;
  cursor: pointer;
  border-bottom: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
}

.file-item:hover { background: var(--border); }
.file-item.active { background: var(--accent); color: var(--accent-text); }
.file-item.active .file-size { color: var(--accent-text); }

.file-name { font-size: 0.75rem; word-break: break-all; }
.file-size { font-size: 0.625rem; color: var(--text-muted); }

.empty-files { padding: 1rem; text-align: center; color: var(--text-muted); font-size: 0.75rem; }

.file-editor {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.editor-placeholder {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
}

.editor-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.5rem;
}

.editor-path { font-size: 0.75rem; color: var(--text-muted); font-family: monospace; }
.editor-actions { display: flex; align-items: center; gap: 0.5rem; }
.save-indicator { font-size: 0.625rem; color: var(--accent); font-family: 'Press Start 2P', monospace; }

.editor-textarea {
  flex: 1;
  background: var(--bg-input);
  color: var(--text-primary);
  border: 2px solid var(--border);
  border-radius: 2px;
  padding: 0.75rem;
  font-family: 'Courier New', monospace;
  font-size: 0.8125rem;
  line-height: 1.5;
  resize: none;
  tab-size: 4;
}

.editor-textarea:focus { outline: none; border-color: var(--accent); }

.editor-warning {
  margin-top: 0.75rem;
  font-size: 0.6875rem;
  color: var(--warning);
  text-align: center;
}
</style>
