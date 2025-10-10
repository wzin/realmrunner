<template>
  <div class="login-page">
    <div class="login-card card">
      <h1 class="login-title">RealmRunner</h1>
      <p class="login-subtitle">Minecraft Server Manager</p>

      <form @submit.prevent="handleLogin">
        <div v-if="error" class="alert alert-error">
          {{ error }}
        </div>

        <div class="form-group">
          <label for="password" class="form-label">Password</label>
          <input
            id="password"
            v-model="password"
            type="password"
            class="input"
            placeholder="Enter password"
            required
            :disabled="loading"
          />
        </div>

        <button type="submit" class="btn btn-primary w-full" :disabled="loading">
          {{ loading ? 'Logging in...' : 'Login' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api/client'

const router = useRouter()
const password = ref('')
const error = ref('')
const loading = ref(false)

async function handleLogin() {
  error.value = ''
  loading.value = true

  try {
    const response = await api.login(password.value)
    localStorage.setItem('token', response.token)
    router.push('/dashboard')
  } catch (err) {
    error.value = err.message || 'Login failed'
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1rem;
}

.login-card {
  width: 100%;
  max-width: 400px;
}

.login-title {
  font-size: 2rem;
  font-weight: 700;
  text-align: center;
  margin-bottom: 0.5rem;
  color: #3b82f6;
}

.login-subtitle {
  text-align: center;
  color: #94a3b8;
  margin-bottom: 2rem;
}

.w-full {
  width: 100%;
}
</style>
