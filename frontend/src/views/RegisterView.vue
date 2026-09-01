<script setup>
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowLeft,
  ArrowRight,
  BadgeCheck,
  Eye,
  EyeOff,
  KeyRound,
  LockKeyhole,
  UserRound,
} from '@lucide/vue'

import AppLogo from '../components/AppLogo.vue'
import { register } from '../services/auth.js'

const router = useRouter()
const username = ref('')
const code = ref('')
const password = ref('')
const checkPassword = ref('')
const iAgree = ref(false)
const showPassword = ref(false)
const submitting = ref(false)
const errorMessage = ref('')

const passwordsMatch = computed(
  () => checkPassword.value.length === 0 || password.value === checkPassword.value,
)
const canSubmit = computed(
  () =>
    username.value.trim().length >= 3 &&
    username.value.trim().length <= 50 &&
    password.value.length >= 8 &&
    password.value.length <= 72 &&
    password.value === checkPassword.value &&
    code.value.trim().length > 0 &&
    iAgree.value &&
    !submitting.value,
)

async function handleSubmit() {
  if (!canSubmit.value) return
  errorMessage.value = ''
  submitting.value = true

  try {
    const normalizedUsername = username.value.trim()
    await register({
      username: normalizedUsername,
      password: password.value,
      checkPassword: checkPassword.value,
      code: code.value.trim(),
      iAgree: iAgree.value,
    })
    await router.replace({
      name: 'login',
      query: { registered: '1', username: normalizedUsername },
    })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : '注册失败，请稍后重试'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <main class="register-shell">
    <section class="brand-stage" aria-label="AI Gateway 品牌展示">
      <AppLogo inverse class="stage-logo" />
      <div class="brand-words" aria-hidden="true">
        <span>JOIN</span>
        <span>THE</span>
        <span>GATE</span>
      </div>
      <p class="stage-caption">创建身份 · 进入网关 · 开始连接</p>
    </section>

    <section class="register-panel">
      <div class="mobile-logo"><AppLogo /></div>
      <div class="register-card">
        <RouterLink class="back-link" to="/login">
          <ArrowLeft :size="16" />
          返回登录
        </RouterLink>

        <header class="register-header">
          <span class="eyebrow">CREATE IDENTITY</span>
          <h1>创建账号</h1>
          <p>注册后将获得基础用户权限，并返回登录页面。</p>
        </header>

        <form class="register-form" novalidate @submit.prevent="handleSubmit">
          <div class="field-grid">
            <div class="form-field full-field">
              <label for="register-username">用户名</label>
              <div class="input-shell">
                <UserRound :size="18" aria-hidden="true" />
                <input
                  id="register-username"
                  v-model="username"
                  autocomplete="username"
                  maxlength="50"
                  placeholder="3–50 个字符"
                  required
                />
              </div>
            </div>

            <div class="form-field full-field">
              <label for="register-code">注册码</label>
              <div class="input-shell">
                <KeyRound :size="18" aria-hidden="true" />
                <input
                  id="register-code"
                  v-model="code"
                  autocomplete="one-time-code"
                  placeholder="请输入管理员提供的注册码"
                  required
                />
              </div>
              <small>当前注册合同要求填写非空注册码</small>
            </div>

            <div class="form-field">
              <label for="register-password">设置密码</label>
              <div class="input-shell">
                <LockKeyhole :size="18" aria-hidden="true" />
                <input
                  id="register-password"
                  v-model="password"
                  :type="showPassword ? 'text' : 'password'"
                  autocomplete="new-password"
                  maxlength="72"
                  placeholder="至少 8 个字符"
                  required
                />
                <button
                  class="password-toggle"
                  type="button"
                  :aria-label="showPassword ? '隐藏密码' : '显示密码'"
                  @click="showPassword = !showPassword"
                >
                  <EyeOff v-if="showPassword" :size="17" />
                  <Eye v-else :size="17" />
                </button>
              </div>
            </div>

            <div class="form-field">
              <label for="register-password-confirm">确认密码</label>
              <div class="input-shell" :class="{ invalid: !passwordsMatch }">
                <BadgeCheck :size="18" aria-hidden="true" />
                <input
                  id="register-password-confirm"
                  v-model="checkPassword"
                  :type="showPassword ? 'text' : 'password'"
                  autocomplete="new-password"
                  maxlength="72"
                  placeholder="再次输入密码"
                  required
                />
              </div>
              <small v-if="!passwordsMatch" class="field-error">两次输入的密码不一致</small>
            </div>
          </div>

          <label class="agreement-option">
            <input v-model="iAgree" type="checkbox" />
            <span>我已了解该账号仅用于访问本地 AI Gateway 管理中心</span>
          </label>

          <p v-if="errorMessage" class="form-error" role="alert">{{ errorMessage }}</p>

          <button class="submit-button" type="submit" :disabled="!canSubmit">
            <span>{{ submitting ? '正在创建账号…' : '创建账号' }}</span>
            <ArrowRight v-if="!submitting" :size="18" />
            <span v-else class="spinner" aria-hidden="true"></span>
          </button>
        </form>

        <p class="login-prompt">
          已有账号？
          <RouterLink to="/login">直接登录</RouterLink>
        </p>
      </div>
      <p class="copyright">AI Gateway · Local Management Console</p>
    </section>
  </main>
</template>

<style scoped>
.register-shell {
  display: grid;
  min-height: 100vh;
  grid-template-columns: minmax(430px, 0.95fr) minmax(590px, 1.05fr);
  background: var(--bg-secondary);
}

.brand-stage {
  position: relative;
  display: flex;
  min-height: 100vh;
  flex-direction: column;
  justify-content: center;
  overflow: hidden;
  padding: 46px clamp(36px, 5vw, 78px);
  background: #151412;
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
  font-size: clamp(78px, 10vw, 168px);
  font-weight: 900;
  letter-spacing: -0.075em;
  line-height: 0.76;
}

.brand-words span {
  animation: word-in 0.8s var(--ease-out-strong) both;
}

.brand-words span:nth-child(1) { opacity: 0.96; }
.brand-words span:nth-child(2) { opacity: 0.68; animation-delay: 100ms; }
.brand-words span:nth-child(3) { opacity: 0.4; animation-delay: 200ms; }

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

.register-panel {
  display: flex;
  min-height: 100vh;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  padding: 42px 36px 30px;
}

.mobile-logo { display: none; }

.register-card {
  width: min(100%, 560px);
  padding: 32px;
  border: 1px solid var(--border-color);
  border-radius: 14px;
  background: var(--bg-primary);
  box-shadow: var(--shadow-lg);
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  margin-bottom: 24px;
  color: var(--text-secondary);
  font-size: 12px;
  font-weight: 700;
  text-decoration: none;
}

.back-link:hover { color: var(--text-primary); }

.eyebrow {
  color: var(--text-tertiary);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.18em;
}

.register-header h1 {
  margin: 9px 0 6px;
  color: var(--text-primary);
  font-size: 28px;
  letter-spacing: -0.035em;
}

.register-header p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
  line-height: 1.65;
}

.register-form { margin-top: 24px; }

.field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 17px 12px;
}

.full-field { grid-column: 1 / -1; }

.form-field label {
  display: block;
  margin-bottom: 8px;
  color: var(--text-primary);
  font-size: 12px;
  font-weight: 700;
}

.form-field small {
  display: block;
  margin-top: 6px;
  color: var(--text-tertiary);
  font-size: 9px;
}

.form-field small.field-error { color: var(--warning-color); }

.input-shell {
  display: flex;
  height: 46px;
  align-items: center;
  gap: 10px;
  padding: 0 13px;
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

.input-shell.invalid { border-color: var(--warning-color); }

.input-shell input {
  min-width: 0;
  flex: 1;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--text-primary);
  font: inherit;
  font-size: 13px;
}

.input-shell input::placeholder { color: var(--text-tertiary); }

.password-toggle {
  display: grid;
  padding: 5px;
  border: 0;
  background: transparent;
  color: var(--text-tertiary);
  cursor: pointer;
  place-items: center;
}

.agreement-option {
  display: flex;
  align-items: flex-start;
  gap: 9px;
  margin: 19px 0;
  color: var(--text-secondary);
  cursor: pointer;
  font-size: 11px;
  line-height: 1.5;
}

.agreement-option input {
  width: 15px;
  height: 15px;
  flex: 0 0 auto;
  margin: 1px 0 0;
  accent-color: var(--primary-active);
}

.form-error {
  margin: -2px 0 16px;
  padding: 10px 12px;
  border: 1px solid var(--warning-border);
  border-radius: 8px;
  background: var(--warning-bg);
  color: var(--warning-color);
  font-size: 12px;
  line-height: 1.5;
}

.submit-button {
  display: flex;
  width: 100%;
  height: 47px;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border: 0;
  border-radius: 8px;
  background: var(--text-primary);
  color: var(--primary-contrast);
  cursor: pointer;
  font: inherit;
  font-size: 13px;
  font-weight: 800;
  transition: transform var(--dur-press), opacity var(--dur-hover), background var(--dur-hover);
}

.submit-button:hover:not(:disabled) {
  background: #151412;
  transform: translateY(-1px);
}

.submit-button:disabled { cursor: not-allowed; opacity: 0.45; }

.spinner {
  width: 16px;
  height: 16px;
  border: 2px solid rgb(255 255 255 / 35%);
  border-top-color: #fff;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}

.login-prompt {
  margin: 19px 0 0;
  color: var(--text-secondary);
  font-size: 11px;
  text-align: center;
}

.login-prompt a {
  color: var(--text-primary);
  font-weight: 800;
  text-decoration: none;
}

.login-prompt a:hover { text-decoration: underline; text-underline-offset: 3px; }

.copyright {
  margin: 20px 0 0;
  color: var(--text-tertiary);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

@keyframes word-in {
  from { opacity: 0; transform: translateY(-18px); }
}

@keyframes spin { to { transform: rotate(360deg); } }

@media (max-width: 920px) {
  .register-shell { grid-template-columns: 1fr; }
  .brand-stage { display: none; }
  .register-panel { padding: 28px 22px; }
  .mobile-logo {
    display: block;
    width: min(100%, 560px);
    margin-bottom: 30px;
  }
}

@media (max-width: 580px) {
  .register-panel {
    justify-content: flex-start;
    padding-top: 30px;
  }

  .mobile-logo { margin-bottom: 44px; }

  .register-card {
    padding: 0;
    border: 0;
    background: transparent;
    box-shadow: none;
  }

  .field-grid { grid-template-columns: 1fr; }
  .full-field { grid-column: auto; }
}
</style>
