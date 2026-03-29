<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal modal-large">
      <div class="modal-header pixel-font">
        <span>{{ server.name }} - Metrics</span>
        <button @click="$emit('close')" class="close-btn">&times;</button>
      </div>

      <div class="range-selector">
        <button
          v-for="r in ranges"
          :key="r.value"
          :class="['btn btn-sm', selectedRange === r.value ? 'btn-primary' : 'btn-secondary']"
          @click="selectRange(r.value)"
        >
          {{ r.label }}
        </button>
      </div>

      <div v-if="loading" class="loading-state">Loading metrics...</div>
      <div v-else-if="!points.length" class="empty-state">No metrics data available yet.</div>

      <div v-else class="charts-container">
        <div class="chart-section">
          <h3 class="chart-title pixel-font">CPU Usage (%)</h3>
          <div ref="cpuChart" class="chart"></div>
        </div>
        <div class="chart-section">
          <h3 class="chart-title pixel-font">Memory (MB)</h3>
          <div ref="memChart" class="chart"></div>
        </div>
        <div class="chart-section">
          <h3 class="chart-title pixel-font">Players</h3>
          <div ref="playerChart" class="chart"></div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick, watch } from 'vue'
import { api } from '../api/client'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'

const props = defineProps({
  server: { type: Object, required: true }
})

defineEmits(['close'])

const ranges = [
  { label: '1H', value: '1h' },
  { label: '24H', value: '24h' },
  { label: '7D', value: '7d' },
  { label: '30D', value: '30d' },
]

const selectedRange = ref('24h')
const loading = ref(false)
const points = ref([])

const cpuChart = ref(null)
const memChart = ref(null)
const playerChart = ref(null)

let charts = []

function destroyCharts() {
  charts.forEach(c => c.destroy())
  charts = []
}

function getThemeColors() {
  const style = getComputedStyle(document.documentElement)
  return {
    accent: style.getPropertyValue('--accent').trim(),
    danger: style.getPropertyValue('--danger').trim(),
    warning: style.getPropertyValue('--warning').trim(),
    text: style.getPropertyValue('--text-primary').trim(),
    muted: style.getPropertyValue('--text-muted').trim(),
    grid: style.getPropertyValue('--border').trim(),
    bg: style.getPropertyValue('--bg-input').trim(),
  }
}

function buildChart(el, timestamps, values, label, color) {
  if (!el) return null
  const colors = getThemeColors()
  const opts = {
    width: el.clientWidth || 700,
    height: 160,
    cursor: { show: true },
    scales: { x: { time: true }, y: { auto: true } },
    axes: [
      {
        stroke: colors.muted,
        grid: { stroke: colors.grid, width: 1 },
        ticks: { stroke: colors.grid },
        font: '9px sans-serif',
      },
      {
        stroke: colors.muted,
        grid: { stroke: colors.grid, width: 1 },
        ticks: { stroke: colors.grid },
        font: '9px sans-serif',
        size: 50,
      },
    ],
    series: [
      {},
      {
        label,
        stroke: color,
        width: 2,
        fill: color + '20',
      },
    ],
  }

  const chart = new uPlot(opts, [timestamps, values], el)
  return chart
}

async function loadData() {
  loading.value = true
  try {
    const resp = await api.getMetricsHistory(props.server.id, selectedRange.value)
    points.value = resp.points || []
  } catch (err) {
    console.error('Failed to load metrics:', err)
    points.value = []
  } finally {
    loading.value = false
  }

  await nextTick()
  renderCharts()
}

function renderCharts() {
  destroyCharts()
  if (!points.value.length) return

  const timestamps = points.value.map(p => Math.floor(new Date(p.timestamp).getTime() / 1000))
  const cpuValues = points.value.map(p => p.cpu_percent)
  const memValues = points.value.map(p => p.memory_mb)
  const playerValues = points.value.map(p => p.player_count)

  const colors = getThemeColors()

  const c1 = buildChart(cpuChart.value, timestamps, cpuValues, 'CPU %', colors.accent)
  const c2 = buildChart(memChart.value, timestamps, memValues, 'Memory MB', colors.warning)
  const c3 = buildChart(playerChart.value, timestamps, playerValues, 'Players', colors.danger)

  if (c1) charts.push(c1)
  if (c2) charts.push(c2)
  if (c3) charts.push(c3)
}

function selectRange(r) {
  selectedRange.value = r
  loadData()
}

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.modal-large {
  max-width: 850px;
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

.range-selector {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 1rem;
}

.loading-state, .empty-state {
  text-align: center;
  padding: 3rem;
  color: var(--text-muted);
}

.charts-container {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.chart-section {
  background: var(--bg-input);
  border: 2px solid var(--border);
  border-radius: 2px;
  padding: 0.75rem;
}

.chart-title {
  font-size: 0.5rem;
  color: var(--text-muted);
  margin-bottom: 0.5rem;
}

.chart {
  width: 100%;
  overflow: hidden;
}

/* Override uPlot default styles to match theme */
:deep(.u-wrap) {
  background: transparent !important;
}
</style>
