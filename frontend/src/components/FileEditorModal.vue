<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal modal-large">
      <div class="modal-header pixel-font">
        <span>{{ server.name }} - Config Files</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div class="editor-container">
        <div class="file-tree">
          <div v-if="loadingFiles" class="tree-loading">Loading...</div>
          <div v-else-if="!files.length" class="tree-empty">No editable files</div>
          <template v-else>
            <FileTreeNode
              v-for="node in fileTree"
              :key="node.path || node.name"
              :node="node"
              :selected="selectedFile"
              :depth="0"
              @select="selectFile"
            />
          </template>
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
            <div ref="editorEl" class="codemirror-wrapper"></div>
          </template>
          <div v-if="error" class="alert alert-error">{{ error }}</div>
        </div>
      </div>

      <div class="editor-warning">Changes may require a server restart to take effect.</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick, watch, onUnmounted, h as createElement, defineComponent } from 'vue'
import { api } from '../api/client'
import { EditorView, keymap, lineNumbers, highlightActiveLine } from '@codemirror/view'
import { EditorState } from '@codemirror/state'
import { json } from '@codemirror/lang-json'
import { yaml } from '@codemirror/lang-yaml'
import { StreamLanguage } from '@codemirror/language'
import { oneDark } from '@codemirror/theme-one-dark'
import { defaultKeymap } from '@codemirror/commands'

// Simple properties/ini language definition
const propertiesLang = StreamLanguage.define({
  token(stream) {
    if (stream.match(/^#.*/)) return 'comment'
    if (stream.match(/^;.*/)) return 'comment'
    if (stream.match(/^[a-zA-Z0-9._-]+(?==)/)) return 'propertyName'
    if (stream.match(/^=/)) return 'operator'
    stream.next()
    return null
  }
})

function getLanguageExt(filename) {
  const ext = filename.split('.').pop().toLowerCase()
  switch (ext) {
    case 'json': return json()
    case 'yml':
    case 'yaml': return yaml()
    case 'properties':
    case 'ini':
    case 'cfg':
    case 'conf':
    case 'toml':
      return propertiesLang
    default: return propertiesLang
  }
}

// FileTreeNode component (inline)
const FileTreeNode = defineComponent({
  name: 'FileTreeNode',
  props: {
    node: Object,
    selected: String,
    depth: Number,
  },
  emits: ['select'],
  setup(props, { emit }) {
    const expanded = ref(props.depth < 1) // auto-expand first level

    function toggle() {
      if (props.node.children) {
        expanded.value = !expanded.value
      } else {
        emit('select', props.node)
      }
    }

    return () => {
      const items = []
      const isDir = !!props.node.children
      const isActive = !isDir && props.selected === props.node.path
      const indent = `${props.depth * 12 + 8}px`

      items.push(
        createElement('div', {
          class: ['tree-item', { active: isActive, dir: isDir }],
          style: { paddingLeft: indent },
          onClick: toggle,
        }, [
          createElement('span', { class: 'tree-icon' }, isDir ? (expanded.value ? '\u25BC' : '\u25B6') : '\u2022'),
          createElement('span', { class: 'tree-label' }, props.node.name),
          !isDir ? createElement('span', { class: 'tree-size' }, formatSize(props.node.size)) : null,
        ])
      )

      if (isDir && expanded.value) {
        for (const child of props.node.children) {
          items.push(
            createElement(FileTreeNode, {
              node: child,
              selected: props.selected,
              depth: props.depth + 1,
              onSelect: (f) => emit('select', f),
            })
          )
        }
      }

      return createElement('div', null, items)
    }
  }
})

function formatSize(bytes) {
  if (!bytes) return ''
  if (bytes < 1024) return bytes + 'B'
  return (bytes / 1024).toFixed(1) + 'K'
}

function buildTree(files) {
  // Build a nested tree from flat file paths
  const root = { children: {} }

  for (const f of files) {
    const parts = f.path.split('/')
    let node = root
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i]
      if (!node.children[part]) {
        node.children[part] = { name: part, children: {} }
      }
      if (i === parts.length - 1) {
        // Leaf file
        node.children[part].path = f.path
        node.children[part].size = f.size
        node.children[part].isFile = true
      }
      node = node.children[part]
    }
  }

  function toArray(node) {
    const entries = Object.values(node.children)
    const dirs = entries.filter(e => !e.isFile).map(e => ({
      name: e.name,
      children: toArray(e),
    })).sort((a, b) => a.name.localeCompare(b.name))
    const fileNodes = entries.filter(e => e.isFile).map(e => ({
      name: e.name,
      path: e.path,
      size: e.size,
    })).sort((a, b) => a.name.localeCompare(b.name))
    return [...dirs, ...fileNodes]
  }

  return toArray(root)
}

const props = defineProps({ server: { type: Object, required: true } })
defineEmits(['close'])

const files = ref([])
const fileTree = ref([])
const loadingFiles = ref(true)
const selectedFile = ref(null)
const content = ref('')
const loadingContent = ref(false)
const saving = ref(false)
const saved = ref(false)
const error = ref('')
const editorEl = ref(null)
let editorView = null

async function loadFiles() {
  loadingFiles.value = true
  try {
    const resp = await api.getFiles(props.server.id)
    files.value = resp.files || []
    fileTree.value = buildTree(files.value)
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
    loadingContent.value = false
    await nextTick()
    initEditor(f.path, resp.content)
  } catch (err) {
    error.value = 'Failed to load file'
    content.value = ''
    loadingContent.value = false
  }
}

function initEditor(filename, text) {
  if (editorView) {
    editorView.destroy()
    editorView = null
  }
  if (!editorEl.value) return

  const langExt = getLanguageExt(filename)

  editorView = new EditorView({
    state: EditorState.create({
      doc: text,
      extensions: [
        lineNumbers(),
        highlightActiveLine(),
        keymap.of(defaultKeymap),
        langExt,
        oneDark,
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            content.value = update.state.doc.toString()
            saved.value = false
          }
        }),
        EditorView.theme({
          '&': { height: '100%', fontSize: '13px' },
          '.cm-scroller': { overflow: 'auto', fontFamily: "'Courier New', monospace" },
          '.cm-content': { caretColor: 'var(--accent)' },
          '&.cm-focused .cm-cursor': { borderLeftColor: 'var(--accent)' },
        }),
      ],
    }),
    parent: editorEl.value,
  })
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

onUnmounted(() => {
  if (editorView) editorView.destroy()
})

onMounted(loadFiles)
</script>

<style scoped>
.modal-large {
  max-width: 950px;
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

/* Tree */
.file-tree {
  width: 220px;
  flex-shrink: 0;
  overflow-y: auto;
  border: 2px solid var(--border);
  border-radius: 2px;
  background: var(--bg-input);
  font-size: 0.75rem;
}

.tree-loading, .tree-empty {
  padding: 1rem;
  text-align: center;
  color: var(--text-muted);
}

:deep(.tree-item) {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.3rem 0.5rem;
  cursor: pointer;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  border-bottom: 1px solid transparent;
}

:deep(.tree-item:hover) { background: var(--border); }
:deep(.tree-item.active) { background: var(--accent); color: var(--accent-text); }
:deep(.tree-item.active .tree-size) { color: var(--accent-text); }
:deep(.tree-item.dir) { font-weight: 600; }

:deep(.tree-icon) {
  font-size: 0.5rem;
  width: 0.75rem;
  text-align: center;
  flex-shrink: 0;
}

:deep(.tree-label) {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
}

:deep(.tree-size) {
  font-size: 0.5625rem;
  color: var(--text-muted);
  flex-shrink: 0;
}

/* Editor */
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

.editor-path {
  font-size: 0.6875rem;
  color: var(--text-muted);
  font-family: monospace;
  background: var(--bg-input);
  padding: 0.25rem 0.5rem;
  border: 1px solid var(--border);
  border-radius: 2px;
}

.editor-actions { display: flex; align-items: center; gap: 0.5rem; }
.save-indicator { font-size: 0.625rem; color: var(--accent); font-family: 'Press Start 2P', monospace; }

.codemirror-wrapper {
  flex: 1;
  border: 2px solid var(--border);
  border-radius: 2px;
  overflow: hidden;
}

.codemirror-wrapper :deep(.cm-editor) {
  height: 100%;
}

.editor-warning {
  margin-top: 0.75rem;
  font-size: 0.6875rem;
  color: var(--warning);
  text-align: center;
}
</style>
