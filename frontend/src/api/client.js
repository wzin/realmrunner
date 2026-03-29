const API_BASE = '/api'

function getAuthHeader() {
  const token = localStorage.getItem('token')
  return token ? { 'Authorization': `Bearer ${token}` } : {}
}

async function request(endpoint, options = {}) {
  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeader(),
      ...options.headers,
    },
  })

  if (response.status === 401) {
    localStorage.removeItem('token')
    window.location.href = '/login'
    throw new Error('Unauthorized')
  }

  const data = await response.json()

  if (!response.ok) {
    throw new Error(data.error || 'Request failed')
  }

  return data
}

export const api = {
  // Auth
  login: (username, password) => request('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  }),

  // Users
  getMe: () => request('/me'),
  changePassword: (password) => request('/me/password', { method: 'PUT', body: JSON.stringify({ password }) }),
  getUsers: () => request('/users'),
  createUser: (username, password, role) => request('/users', { method: 'POST', body: JSON.stringify({ username, password, role }) }),
  updateUser: (id, role) => request(`/users/${id}`, { method: 'PUT', body: JSON.stringify({ role }) }),
  deleteUser: (id) => request(`/users/${id}`, { method: 'DELETE' }),

  // Servers
  getServers: () => request('/servers'),
  getServer: (id) => request(`/servers/${id}`),
  createServer: (name, version, port, flavor) => request('/servers', {
    method: 'POST',
    body: JSON.stringify({ name, version, port, flavor: flavor || 'vanilla' }),
  }),
  setSchedule: (id, schedule) => request(`/servers/${id}/schedule`, {
    method: 'PUT',
    body: JSON.stringify({ schedule }),
  }),
  setLimits: (id, cpuLimit, memoryLimitMB) => request(`/servers/${id}/limits`, {
    method: 'PUT',
    body: JSON.stringify({ cpu_limit: cpuLimit, memory_limit_mb: memoryLimitMB }),
  }),
  upgradeServer: (id, version, flavor) => request(`/servers/${id}/upgrade`, {
    method: 'POST',
    body: JSON.stringify({ version, flavor }),
  }),
  startServer: (id) => request(`/servers/${id}/start`, { method: 'POST' }),
  stopServer: (id) => request(`/servers/${id}/stop`, { method: 'POST' }),
  resetServer: (id) => request(`/servers/${id}/reset`, { method: 'POST' }),
  wipeoutServer: (id) => request(`/servers/${id}/wipeout`, { method: 'DELETE' }),
  sendCommand: (id, command) => request(`/servers/${id}/command`, {
    method: 'POST',
    body: JSON.stringify({ command }),
  }),

  // Mods
  getMods: (id) => request(`/servers/${id}/mods`),
  searchMods: (id, query, limit) => request(`/servers/${id}/mods/search`, { method: 'POST', body: JSON.stringify({ query, limit: limit || 20 }) }),
  getModVersions: (id, projectId) => request(`/servers/${id}/mods/versions/${projectId}`),
  installMod: (id, modrinthId, versionId) => request(`/servers/${id}/mods`, { method: 'POST', body: JSON.stringify({ modrinth_id: modrinthId, version_id: versionId }) }),
  removeMod: (id, modId) => request(`/servers/${id}/mods/${modId}`, { method: 'DELETE' }),

  // Backups
  getBackups: (id) => request(`/servers/${id}/backups`),
  createBackup: (id) => request(`/servers/${id}/backups`, { method: 'POST' }),
  restoreBackup: (id, bid) => request(`/servers/${id}/backups/${bid}/restore`, { method: 'POST' }),
  deleteBackup: (id, bid) => request(`/servers/${id}/backups/${bid}`, { method: 'DELETE' }),

  // Whitelist & Ops
  getWhitelist: (id) => request(`/servers/${id}/whitelist`),
  addToWhitelist: (id, name) => request(`/servers/${id}/whitelist`, { method: 'POST', body: JSON.stringify({ name }) }),
  removeFromWhitelist: (id, uuid) => request(`/servers/${id}/whitelist/${uuid}`, { method: 'DELETE' }),
  getOps: (id) => request(`/servers/${id}/ops`),
  addOp: (id, name) => request(`/servers/${id}/ops`, { method: 'POST', body: JSON.stringify({ name }) }),
  removeOp: (id, uuid) => request(`/servers/${id}/ops/${uuid}`, { method: 'DELETE' }),

  // Files
  getFiles: (id) => request(`/servers/${id}/files`),
  getFile: (id, path) => request(`/servers/${id}/files/${path}`),
  saveFile: (id, path, content) => request(`/servers/${id}/files/${path}`, {
    method: 'PUT',
    body: JSON.stringify({ content }),
  }),

  // Metrics
  getMetrics: (id) => request(`/servers/${id}/metrics`),
  getMetricsHistory: (id, range_) => request(`/servers/${id}/metrics/history?range=${range_}`),

  // Realms
  getRealms: () => request('/realms'),
  createRealm: (name, maxCpu, maxMem, maxServers) => request('/realms', {
    method: 'POST', body: JSON.stringify({ name, max_cpu_cores: maxCpu, max_memory_mb: maxMem, max_servers: maxServers }),
  }),
  updateRealm: (id, name, maxCpu, maxMem, maxServers) => request(`/realms/${id}`, {
    method: 'PUT', body: JSON.stringify({ name, max_cpu_cores: maxCpu, max_memory_mb: maxMem, max_servers: maxServers }),
  }),
  deleteRealm: (id) => request(`/realms/${id}`, { method: 'DELETE' }),
  getRealmAdmins: (id) => request(`/realms/${id}/admins`),
  addRealmAdmin: (id, userId) => request(`/realms/${id}/admins`, { method: 'POST', body: JSON.stringify({ user_id: userId }) }),
  removeRealmAdmin: (id, userId) => request(`/realms/${id}/admins/${userId}`, { method: 'DELETE' }),
  getServerViewers: (id) => request(`/servers/${id}/viewers`),
  addServerViewer: (id, userId) => request(`/servers/${id}/viewers`, { method: 'POST', body: JSON.stringify({ user_id: userId }) }),
  removeServerViewer: (id, userId) => request(`/servers/${id}/viewers/${userId}`, { method: 'DELETE' }),

  // Versions & Flavors
  getVersions: (flavor, includeSnapshots) => {
    const params = new URLSearchParams()
    if (flavor) params.set('flavor', flavor)
    if (includeSnapshots) params.set('include_snapshots', 'true')
    return request(`/versions?${params}`)
  },
  getFlavors: () => request('/flavors'),
}

export function createWebSocket(serverId) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = localStorage.getItem('token')
  const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/${serverId}?token=${token}`)
  return ws
}
