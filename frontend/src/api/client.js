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
  login: (password) => request('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ password }),
  }),

  // Servers
  getServers: () => request('/servers'),
  getServer: (id) => request(`/servers/${id}`),
  createServer: (name, version, port) => request('/servers', {
    method: 'POST',
    body: JSON.stringify({ name, version, port }),
  }),
  startServer: (id) => request(`/servers/${id}/start`, { method: 'POST' }),
  stopServer: (id) => request(`/servers/${id}/stop`, { method: 'POST' }),
  wipeoutServer: (id) => request(`/servers/${id}/wipeout`, { method: 'DELETE' }),
  sendCommand: (id, command) => request(`/servers/${id}/command`, {
    method: 'POST',
    body: JSON.stringify({ command }),
  }),

  // Versions
  getVersions: () => request('/versions'),
}

export function createWebSocket(serverId) {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = localStorage.getItem('token')
  const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/${serverId}?token=${token}`)
  return ws
}
