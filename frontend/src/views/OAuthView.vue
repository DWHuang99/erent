<script setup>
import { onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowUpRight, CheckCircle2, ExternalLink, LoaderCircle, ShieldCheck, Terminal, X } from '@lucide/vue'
import { completeCodexLogin, startCodexLogin, startCodexDeviceLogin, completeCodexDeviceLogin } from '../services/oauth.js'

const router = useRouter()
const device = ref(null)
const deviceLoading = ref(false)
const deviceWaiting = ref(false)
const copyMessage = ref('')
let deviceController

async function startDevice() {
  if (loading.value || saving.value || deviceLoading.value || deviceWaiting.value || disposed) return
  close()
  error.value = ''
  success.value = false
  expired.value = false
  copyMessage.value = ''
  device.value = null
  const controller = new AbortController()
  deviceController = controller
  const active = () => !disposed && deviceController === controller && !controller.signal.aborted
  deviceLoading.value = true
  try {
    const info = await startCodexDeviceLogin(controller.signal)
    if (!active()) return
    device.value = info
    deviceLoading.value = false
    deviceWaiting.value = true
    await completeCodexDeviceLogin(info.device_auth_id, controller.signal)
    if (!active()) return
    success.value = true
    device.value = null
    await router.push({ name: 'authorized-accounts' })
  } catch (cause) {
    if (active()) {
      error.value = cause.message
      device.value = null
    }
  } finally {
    if (active()) {
      deviceLoading.value = false
      deviceWaiting.value = false
      deviceController = undefined
    }
  }
}

function cancelDevice(showMessage = true) {
  deviceController?.abort()
  deviceController = undefined
  device.value = null
  deviceLoading.value = false
  deviceWaiting.value = false
  copyMessage.value = ''
  if (showMessage) error.value = '已停止等待。若已在授权页面确认，请先查看已授权账号；若未保存，请重新开始授权。'
}

async function copyDeviceCode() {
  const current = device.value
  try {
    await navigator.clipboard.writeText(current.user_code)
    if (!disposed && device.value === current) copyMessage.value = '授权码已复制'
  } catch {
    if (!disposed && device.value === current) copyMessage.value = '复制失败，请手动选择并复制授权码。'
  }
}

const providers = [
  { id: 'kimi', name: 'Kimi', description: '通过设备授权快速登录 Kimi，完成后自动获取并保存授权凭证。', featured: true },
  { id: 'codex', name: 'Codex', description: '通过 OAuth 流程登录 Codex 服务，完成授权后自动获取并保存凭证。', enabled: true },
  { id: 'anthropic', name: 'Anthropic', description: '通过 OAuth 流程登录 Anthropic（Claude）服务，连接你的 Claude 账号。' },
  { id: 'antigravity', name: 'Antigravity', description: '通过 OAuth 流程登录 Antigravity（Google 账号）服务，连接你的 Google 账号。' },
]
const loading = ref(false)
const saving = ref(false)
const authUrl = ref('')
const callbackUrl = ref('')
const error = ref('')
const success = ref(false)
const expired = ref(false)
let expiryTimer
let disposed = false

async function start() {
  if (loading.value || saving.value || deviceLoading.value || deviceWaiting.value) return
  loading.value = true
  error.value = ''
  success.value = false
  authUrl.value = ''
  callbackUrl.value = ''
  expired.value = false
  clearTimeout(expiryTimer)
  try {
    const url = await startCodexLogin()
    if (disposed) return
    authUrl.value = url
    expiryTimer = setTimeout(() => {
      expired.value = true
      authUrl.value = ''
      callbackUrl.value = ''
    }, 5 * 60 * 1000)
  } catch (cause) {
    error.value = cause.message
  } finally {
    loading.value = false
  }
}

async function complete() {
  if (saving.value || !authUrl.value) return
  saving.value = true
  error.value = ''
  try {
    await completeCodexLogin(callbackUrl.value, authUrl.value)
    success.value = true
    close()
  } catch (cause) {
    error.value = cause.message
  } finally {
    saving.value = false
  }
}

function close() {
  clearTimeout(expiryTimer)
  authUrl.value = ''
  callbackUrl.value = ''
}

onUnmounted(() => {
  disposed = true
  cancelDevice(false)
  close()
})
</script>

<template>
  <main class="oauth-page">
    <header class="oauth-heading">
      <h1>OAuth 登录</h1>
      <p>连接你的 AI 账号，安全管理上游服务授权。</p>
    </header>

    <div class="provider-list">
      <article v-for="provider in providers" :key="provider.id" class="provider-card" :class="{ featured: provider.featured }" :aria-labelledby="`${provider.id}-title`">
        <div class="provider-top">
          <div class="provider-title">
            <span class="provider-mark" :class="provider.id" aria-hidden="true">
              <span v-if="provider.id === 'kimi'">K<span class="kimi-dot">·</span></span>
              <Terminal v-else-if="provider.id === 'codex'" :size="25" />
              <span v-else-if="provider.id === 'anthropic'">✳</span>
              <span v-else>Λ</span>
            </span>
            <h2 :id="`${provider.id}-title`">{{ provider.name }} OAuth</h2>
          </div>
          <div class="provider-actions">
            <span v-if="!provider.enabled" class="unavailable">暂未接入</span>
            <button v-if="provider.enabled" class="login-button" :disabled="loading || saving || deviceLoading || deviceWaiting" @click="startDevice">
              <LoaderCircle v-if="deviceLoading || deviceWaiting" class="spin" :size="17" />
              {{ deviceLoading ? '正在获取设备码' : deviceWaiting ? '等待设备授权' : '设备授权' }}
            </button>
            <button class="login-button" :disabled="!provider.enabled || loading || saving || deviceLoading || deviceWaiting" :aria-busy="provider.enabled && loading" @click="start">
              <LoaderCircle v-if="provider.enabled && loading" class="spin" :size="17" />
              {{ provider.enabled && loading ? '正在获取授权链接' : `开始 ${provider.name} 登录` }}
            </button>
          </div>
        </div>
        <p class="provider-description">{{ provider.description }}</p>

        <template v-if="provider.enabled">
          <div v-if="error" class="feedback error" role="alert">{{ error }}</div>
          <div v-if="success" class="feedback success" role="status"><CheckCircle2 :size="18" />Codex 授权成功，凭证已安全保存。</div>
          <div v-if="expired" class="feedback error" role="status">本次授权链接已过期，请重新开始登录。</div>
          <section v-if="deviceLoading || deviceWaiting" class="authorization-panel" aria-label="Codex 设备授权" :aria-busy="deviceLoading">
            <div class="authorization-heading">
              <h3>Codex 设备授权</h3>
              <button class="close-button" aria-label="取消设备授权" @click="cancelDevice()"><X :size="18" /></button>
            </div>
            <p v-if="deviceLoading" role="status">正在获取设备授权码…</p>
            <template v-if="device">
              <p>打开授权页面，登录并输入下方授权码。请勿将授权码分享给他人。</p>
              <div class="device-code-row">
                <code class="device-code">{{ device.user_code }}</code>
                <button class="login-button" @click="copyDeviceCode">复制授权码</button>
              </div>
              <p v-if="copyMessage" role="status">{{ copyMessage }}</p>
              <a class="authorization-link" :href="device.verification_url" target="_blank" rel="noopener noreferrer">打开授权页面 <ExternalLink :size="16" /></a>
              <p role="status">正在等待授权，完成后会自动保存并进入已授权账号页。请保持本页打开。</p>
              <p>本次设备会话最长有效期为 15 分钟，实际结果以服务端为准。取消后需重新获取授权码。</p>
            </template>
          </section>
          <section v-if="authUrl" class="authorization-panel" aria-label="完成 Codex 授权">
            <div class="authorization-heading">
              <h3>完成 Codex 授权</h3>
              <button class="close-button" aria-label="关闭授权步骤" :disabled="saving" @click="close"><X :size="18" /></button>
            </div>
            <p>在新标签页中登录并授权。授权链接有效期为 5 分钟，请勿分享给他人。</p>
            <a class="authorization-link" :href="authUrl" target="_blank" rel="noopener noreferrer">打开授权页面 <ExternalLink :size="16" /></a>
            <p>如果回调页面显示「oauth credentials saved」，说明凭证已保存，可直接关闭该标签页。</p>
            <details>
              <summary>授权后回调页面无法打开？</summary>
              <p>复制该页面地址栏中的完整 URL 并在下方提交。已显示保存成功的回调无需重复提交。</p>
              <form @submit.prevent="complete">
                <label for="callback-url">回调 URL</label>
                <input id="callback-url" v-model="callbackUrl" type="url" placeholder="粘贴授权后的完整回调 URL" autocomplete="off" spellcheck="false" :disabled="saving" required />
                <button class="login-button" :disabled="saving || !callbackUrl.trim()"><LoaderCircle v-if="saving" class="spin" :size="16" />{{ saving ? '正在保存' : '提交授权' }}<ArrowUpRight v-if="!saving" :size="16" /></button>
              </form>
            </details>
          </section>
        </template>
      </article>
    </div>
    <RouterLink class="accounts-link" :to="{ name: 'authorized-accounts' }">查看已授权账号 <ArrowUpRight :size="16" /></RouterLink>
    <p class="security-note"><ShieldCheck :size="16" />授权凭证由服务端加密保存，绑定当前控制台账号。</p>
  </main>
</template>

<style scoped>
.device-code-row { display: flex; align-items: center; flex-wrap: wrap; gap: 16px; margin: 16px 0; }
.device-code { font-size: 24px; font-weight: 700; letter-spacing: .12em; overflow-wrap: anywhere; user-select: all; }
.accounts-link { display: inline-flex; align-items: center; gap: 8px; margin-top: 26px; color: var(--text-primary); font-size: 14px; text-underline-offset: 4px; }
.oauth-page { max-width: 1680px; margin: 0 auto; padding: 40px clamp(20px, 3.4vw, 58px) 36px; }
.oauth-heading { margin-bottom: 34px; }
.oauth-heading h1 { margin: 0 0 10px; font-size: clamp(28px, 3vw, 36px); letter-spacing: -.035em; }
.oauth-heading p { margin: 0; color: var(--text-secondary); font-size: 14px; }
.provider-list { display: grid; gap: 28px; }
.provider-card { padding: 28px; border: 1px solid var(--border-color); border-radius: 14px; background: var(--floating-surface); box-shadow: 0 2px 3px rgb(0 0 0 / 4%); }
.provider-card.featured { border-color: #768b9e; background: linear-gradient(110deg, #e8f2ff, #f8fbff 52%, #d9eaff); box-shadow: 0 5px 16px rgb(30 117 206 / 7%); }
.provider-top { display: flex; justify-content: space-between; align-items: center; gap: 24px; }
.provider-title { display: flex; align-items: center; gap: 15px; }
.provider-title h2 { margin: 0; font-size: clamp(18px, 1.6vw, 23px); letter-spacing: -.025em; }
.provider-mark { width: 38px; height: 38px; display: grid; place-items: center; flex: 0 0 auto; font-weight: 800; }
.kimi { border-radius: 11px; background: #080808; color: white; font-size: 28px; position: relative; }
.kimi-dot { position: absolute; color: #278eff; top: -9px; right: 4px; }
.codex { border-radius: 12px; background: linear-gradient(145deg, #b5acff, #6762f8); color: white; }
.anthropic { color: #d77b60; font-size: 43px; font-weight: 400; }
.antigravity { font-size: 44px; line-height: 1; background: linear-gradient(180deg, #f7aa48 15%, #4cb995 40%, #3485ff 70%); color: transparent; background-clip: text; }
.provider-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 14px; flex-shrink: 0; }
.unavailable { color: var(--text-secondary); font-size: 12px; }
.provider-description { margin: 24px 0 0; font-size: 14px; line-height: 1.8; color: var(--text-secondary); }
.login-button, .authorization-link { display: inline-flex; align-items: center; justify-content: center; gap: 8px; min-height: 45px; padding: 11px 18px; border: 0; border-radius: 9px; background: #187cf2; color: white; font-size: 14px; font-weight: 650; cursor: pointer; text-decoration: none; }
.login-button:hover:not(:disabled), .authorization-link:hover { background: #0865d5; }
.login-button:disabled { background: var(--primary-color); color: var(--primary-contrast); opacity: .72; cursor: not-allowed; }
.security-note { display: flex; align-items: center; justify-content: center; gap: 8px; margin: 25px 0 0; color: var(--text-secondary); font-size: 12px; line-height: 1.6; }
.security-note svg { flex-shrink: 0; }
.authorization-panel { margin-top: 24px; border-top: 1px solid var(--border-color); padding-top: 22px; }
.authorization-heading { display: flex; align-items: center; justify-content: space-between; }
.authorization-heading h3 { margin: 0; font-size: 16px; }
.close-button { display: grid; place-items: center; width: 32px; height: 32px; border: 0; border-radius: 6px; background: var(--bg-tertiary); color: var(--text-secondary); cursor: pointer; }
.authorization-panel p { font-size: 13px; color: var(--text-secondary); line-height: 1.8; }
details { margin-top: 18px; font-size: 13px; }
summary { cursor: pointer; color: var(--text-secondary); }
form { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px; margin-top: 16px; }
form label { grid-column: 1 / -1; font-weight: 600; }
form input { min-width: 0; width: 100%; border: 1px solid var(--border-primary); background: var(--bg-secondary); color: var(--text-primary); border-radius: 8px; padding: 12px; font-size: 13px; }
.feedback { display: flex; align-items: center; gap: 8px; margin-top: 20px; padding: 12px 14px; border-radius: 8px; font-size: 13px; line-height: 1.7; }
.error { background: var(--warning-bg); color: var(--warning-color); }
.success { background: var(--success-badge-bg); color: var(--success-badge-text); }
.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
:global([data-theme='dark']) .provider-card.featured { background: linear-gradient(110deg, #1c2939, #1d232b 52%, #20334b); border-color: #486078; }
@media (max-width: 1100px) { .provider-top { flex-wrap: wrap; gap: 20px; } .provider-actions { margin-left: auto; } }
@media (max-width: 620px) { .oauth-page { padding-top: 28px; } .oauth-heading { margin-bottom: 25px; } .provider-list { gap: 18px; } .provider-card { padding: 22px 18px; } .provider-actions { width: 100%; justify-content: space-between; } .provider-actions .login-button { margin-left: auto; } .provider-description { margin-top: 18px; } form { grid-template-columns: 1fr; } .security-note { align-items: flex-start; justify-content: flex-start; } }
</style>
