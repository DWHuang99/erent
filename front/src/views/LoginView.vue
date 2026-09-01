<script setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowRight, CheckCircle2, Eye, EyeOff, LockKeyhole, UserRound } from '@lucide/vue'

import AppLogo from '../components/AppLogo.vue'
import { getRememberedUsername, login } from '../services/auth.js'

const router = useRouter()
const route = useRoute()
const username = ref('')
const password = ref('')
const remember = ref(false)
const showPassword = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
const successMessage = ref('')

const canSubmit = computed(
  () => username.value.trim().length >= 3 && password.value.length >= 8 && !submitting.value,
)

onMounted(() => {
  const registeredUsername = typeof route.query.username === 'string' ? route.query.username : ''
  if (route.query.registered === '1') {
    successMessage.value = '注册成功，请使用新账号登录'
  }
  if (registeredUsername) {
    username.value = registeredUsername
    return
  }
  const remembered = getRememberedUsername()
  if (remembered) {
    username.value = remembered
    remember.value = true
  }
})

async function handleSubmit() {
  if (!canSubmit.value) return
  errorMessage.value = ''
  submitting.value = true

  try {
    await login({
      username: username.value.trim(),
      password: password.value,
      remember: remember.value,
    })
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '登录失败，请稍后重试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="login-shell">
    <section class="brand-stage" aria-label="AI Gateway 品牌展示">
      <AppLogo inverse class="stage-logo" />
      <div class="brand-words" aria-hidden="true">
        <span>AI</span>
        <span>GATE</span>
        <span>WAY</span>
      </div>
      <p class="stage-caption">统一连接 · 安全访问 · 清晰掌控</p>
    </section>

    <section class="login-panel">
      <div class="mobile-logo">
        <AppLogo />
      </div>
      <div class="login-card">
        <header class="login-header">
          <span class="eyebrow">CONTROL PLANE</span>
          <h1>欢迎回来</h1>
          <p>登录 AI Gateway，查看服务状态与访问权限。</p>
        </header>

        <form class="login-form" novalidate @submit.prevent="handleSubmit">
          <p v-if="successMessage" class="form-success" role="status">
            <CheckCircle2 :size="17" aria-hidden="true" />
            <span>{{ successMessage }}</span>
          </p>

          <label class="field-label" for="username">用户名</label>
          <div class="input-shell">
            <UserRound :size="18" aria-hidden="true" />
            <input
              id="username"
              v-model="username"
              autocomplete="username"
              placeholder="请输入用户名"
              required
            />
          </div>

          <label class="field-label" for="password">密码</label>
          <div class="input-shell">
            <LockKeyhole :size="18" aria-hidden="true" />
            <input
              id="password"
              v-model="password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              placeholder="请输入密码"
              required
            />
            <button
              class="password-toggle"
              type="button"
              :aria-label="showPassword ? '隐藏密码' : '显示密码'"
              @click="showPassword = !showPassword"
            >
              <EyeOff v-if="showPassword" :size="18" />
              <Eye v-else :size="18" />
            </button>
          </div>

          <div class="form-options">
            <label class="remember-option">
              <input v-model="remember" type="checkbox" />
              <span>记住登录状态</span>
            </label>
            <span class="security-note">凭证仅用于当前设备</span>
          </div>

          <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

          <button class="submit-button" type="submit" :disabled="!canSubmit">
            <span>{{ submitting ? '正在验证…' : '进入管理中心' }}</span>
            <ArrowRight v-if="!submitting" :size="18" />
            <span v-else class="spinner" aria-hidden="true"></span>
          </button>
        </form>

        <div class="register-prompt">
          <span>还没有账号？</span>
          <RouterLink to="/register">创建账号</RouterLink>
        </div>

        <footer class="login-footer">
          <span class="status-dot"></span>
          <span>连接地址自动使用当前服务</span>
        </footer>
      </div>
      <p class="copyright">AI Gateway · Local Management Console</p>
    </section>
  </main>
</template>

<style scoped>
.login-shell {
  min-height: 100vh;
  display: grid;
  grid-template-columns: minmax(430px, 1.05fr) minmax(500px, 0.95fr);
  background: var(--bg-secondary);
}

.brand-stage {
  position: relative;
  min-height: 100vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  justify-content: center;
  background: #151412;
  padding: 46px clamp(36px, 5vw, 78px);
  color: #fffdf9;
}

.brand-stage::after {
  position: absolute;
  inset: 0;
  background-image: linear-gradient(rgb(255 255 255 / 3%) 1px, transparent 1px),
    linear-gradient(90deg, rgb(255 255 255 / 3%) 1px, transparent 1px);
  background-size: 42px 42px;
  content: '';
  mask-image: linear-gradient(to bottom, transparent 4%, black 34%, black 70%, transparent 96%);
}

.stage-logo {
  position: absolute;
  z-index: 2;
  top: 38px;
  left: clamp(36px, 5vw, 78px);
}

.brand-words {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-size: clamp(90px, 12.5vw, 208px);
  font-weight: 900;
  letter-spacing: -0.075em;
  line-height: 0.73;
}

.brand-words span {
  animation: word-in 0.8s var(--ease-out-strong) both;
}

.brand-words span:nth-child(1) {
  opacity: 0.96;
}

.brand-words span:nth-child(2) {
  opacity: 0.68;
  animation-delay: 100ms;
}

.brand-words span:nth-child(3) {
  opacity: 0.4;
  animation-delay: 200ms;
}

.stage-caption {
  position: absolute;
  z-index: 2;
  bottom: 42px;
  left: clamp(36px, 5vw, 78px);
  margin: 0;
  color: rgb(255 253 249 / 55%);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.2em;
}

.login-panel {
  display: flex;
  min-height: 100vh;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  padding: 56px 40px 34px;
}

.mobile-logo {
  display: none;
}

.login-card {
  width: min(100%, 430px);
  padding: 34px;
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: var(--bg-primary);
  box-shadow: var(--shadow-lg);
}

.eyebrow {
  color: var(--text-tertiary);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.18em;
}

.login-header h1 {
  margin: 10px 0 7px;
  color: var(--text-primary);
  font-size: 28px;
  letter-spacing: -0.035em;
}

.login-header p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 14px;
  line-height: 1.7;
}

.login-form {
  margin-top: 27px;
}

.field-label {
  display: block;
  margin: 0 0 8px;
  color: var(--text-primary);
  font-size: 13px;
  font-weight: 700;
}

.input-shell + .field-label {
  margin-top: 18px;
}

.input-shell {
  display: flex;
  height: 48px;
  align-items: center;
  gap: 11px;
  padding: 0 14px;
  border: 1px solid var(--border-primary);
  border-radius: 8px;
  background: var(--bg-secondary);
  color: var(--text-tertiary);
  transition: border-color 160ms ease, box-shadow 160ms ease, background 160ms ease;
}

.input-shell:focus-within {
  border-color: var(--primary-color);
  background: var(--floating-surface);
  box-shadow: 0 0 0 3px var(--primary-10);
  color: var(--text-primary);
}

.input-shell input {
  min-width: 0;
  flex: 1;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  font-size: 14px;
}

.input-shell input::placeholder {
  color: var(--text-tertiary);
}

.password-toggle {
  display: grid;
  padding: 5px;
  border: 0;
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  place-items: center;
}

.form-options {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin: 17px 0 20px;
  color: var(--text-secondary);
  font-size: 12px;
}

.remember-option {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.remember-option input {
  width: 15px;
  height: 15px;
  margin: 0;
  accent-color: var(--primary-active);
}

.security-note {
  color: var(--text-tertiary);
}

.form-error {
  margin: -4px 0 16px;
  padding: 10px 12px;
  border: 1px solid var(--warning-border);
  border-radius: 8px;
  background: var(--warning-bg);
  color: var(--warning-color);
  font-size: 12px;
  line-height: 1.5;
}

.form-success {
  display: flex;
  align-items: center;
  gap: 9px;
  margin: 0 0 18px;
  padding: 10px 12px;
  border: 1px solid var(--success-badge-border);
  border-radius: 8px;
  background: var(--success-badge-bg);
  color: var(--success-badge-text);
  font-size: 12px;
  line-height: 1.5;
}

.submit-button {
  display: flex;
  width: 100%;
  height: 48px;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border: 0;
  border-radius: 8px;
  background: var(--text-primary);
  color: var(--primary-contrast);
  cursor: pointer;
  font: inherit;
  font-size: 14px;
  font-weight: 800;
  transition: transform var(--dur-press), opacity var(--dur-hover), background var(--dur-hover);
}

.submit-button:hover:not(:disabled) {
  background: #151412;
  transform: translateY(-1px);
}

.submit-button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgb(255 255 255 / 35%);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}

.register-prompt {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  margin-top: 20px;
  color: var(--text-secondary);
  font-size: 12px;
}

.register-prompt a {
  color: var(--text-primary);
  font-weight: 800;
  text-decoration: none;
}

.register-prompt a:hover {
  text-decoration: underline;
  text-underline-offset: 3px;
}

.login-footer {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid var(--border-color);
  color: var(--text-tertiary);
  font-size: 11px;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--success-color);
  box-shadow: 0 0 0 4px rgb(16 185 129 / 12%);
}

.copyright {
  margin: 24px 0 0;
  color: var(--text-tertiary);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

@keyframes word-in {
  from {
    opacity: 0;
    transform: translateY(-18px);
  }
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 860px) {
  .login-shell {
    grid-template-columns: 1fr;
  }

  .brand-stage {
    display: none;
  }

  .login-panel {
    padding: 28px 22px;
  }

  .mobile-logo {
    display: block;
    width: min(100%, 430px);
    margin-bottom: 34px;
  }
}

@media (max-width: 520px) {
  .login-panel {
    justify-content: flex-start;
    padding-top: 32px;
  }

  .mobile-logo {
    margin-bottom: 54px;
  }

  .login-card {
    padding: 0;
    border: 0;
    background: transparent;
    box-shadow: none;
  }

  .security-note {
    display: none;
  }
}
</style>
