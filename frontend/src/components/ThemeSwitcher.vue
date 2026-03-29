<template>
  <div class="theme-switcher">
    <button
      v-for="t in themes"
      :key="t.id"
      :class="['theme-btn', { active: current === t.id }]"
      :title="t.label"
      @click="setTheme(t.id)"
    >
      <span class="theme-icon">{{ t.icon }}</span>
    </button>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const themes = [
  { id: 'dirt', label: 'Dirt & Wood', icon: '\u2591' },
  { id: 'stone', label: 'Stone & Dark', icon: '\u2593' },
  { id: 'creeper', label: 'Creeper Green', icon: '\u2588' },
]

const current = ref(localStorage.getItem('theme') || 'dirt')

function setTheme(id) {
  current.value = id
  document.documentElement.setAttribute('data-theme', id)
  localStorage.setItem('theme', id)
}
</script>

<style scoped>
.theme-switcher {
  display: flex;
  gap: 4px;
}

.theme-btn {
  width: 28px;
  height: 28px;
  border: 2px solid var(--border);
  border-radius: 2px;
  background: var(--bg-input);
  color: var(--text-muted);
  font-size: 0.875rem;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: border-color 0.05s;
  padding: 0;
}

.theme-btn:hover {
  border-color: var(--accent);
  color: var(--text-primary);
}

.theme-btn.active {
  border-color: var(--accent);
  background: var(--accent);
  color: var(--accent-text);
}

.theme-icon {
  line-height: 1;
}
</style>
