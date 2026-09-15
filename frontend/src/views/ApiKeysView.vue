<script setup>
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { getApiKeys, createApiKey, updateApiKey, replaceApiKeyAccounts, deleteApiKey } from '../services/apikey.js'
import { getOAuthList } from '../services/oauth.js'

const keys = ref([])
const loaded = ref(false)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const dialog = ref(null)
const mode = ref('')
const selectedKey = ref(null)
const accounts = ref([])
const accountsLoading = ref(false)
const accountsError = ref('')
const selectedIds = ref([])
const disabled = ref(false)
const expiry = ref('')
const formError = ref('')
const formNotice = ref('')
const secret = ref('')
let disposed = false

async function loadKeys() {
  if (loading.value || disposed) return
  loading.value = true
  error.value = ''
  try {
    const result = await getApiKeys()
    if (!disposed) { keys.value = result; loaded.value = true }
  } catch (cause) {
    if (!disposed) error.value = cause.message
  } finally { loading.value = false }
}

function localTime(value) {
  if (!value) return ''
  const date = new Date(value)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 19)
}

function status(key) {
  if (key.disabled) return '已禁用'
  if (key.expires_at && new Date(key.expires_at) <= new Date()) return '已过期'
  return '已启用'
}

async function loadAccounts() {
  accountsLoading.value = true
  accountsError.value = ''
  try {
    const result = await getOAuthList()
    if (!disposed) accounts.value = result
  } catch (cause) {
    if (!disposed) accountsError.value = cause.message
  } finally { accountsLoading.value = false }
}

async function openDialog(action, key = null) {
  mode.value = action
  selectedKey.value = key
  selectedIds.value = key?.accounts.map((account) => account.id) ?? []
  disabled.value = key?.disabled ?? false
  expiry.value = localTime(key?.expires_at)
  formError.value = ''
  formNotice.value = ''
  secret.value = ''
  accounts.value = []
  await nextTick()
  if (disposed) return
  dialog.value.showModal()
  if (action !== 'delete') await loadAccounts()
}

function closeDialog() {
  if (busy.value || accountsLoading.value) return
  dialog.value.close()
  secret.value = ''
  mode.value = ''
}

async function save(action) {
  if (busy.value) return
  formError.value = ''
  formNotice.value = ''
  notice.value = ''
  if ((action === 'create' || action === 'accounts') && !selectedIds.value.length) {
    formError.value = '请至少选择一个授权账号。'
    return
  }
  busy.value = true
  try {
    if (action === 'create') {
      const raw = await createApiKey(selectedIds.value)
      if (disposed) return
      secret.value = raw
      mode.value = 'secret'
    } else if (action === 'settings') {
      // Keep the original timestamp when untouched, including its precision.
      const expiresAt = expiry.value === localTime(selectedKey.value.expires_at)
        ? selectedKey.value.expires_at : expiry.value ? new Date(expiry.value).toISOString() : null
      await updateApiKey(selectedKey.value.id, { disabled: disabled.value, expires_at: expiresAt })
      if (disposed) return
      selectedKey.value = { ...selectedKey.value, disabled: disabled.value, expires_at: expiresAt }
      formNotice.value = '状态与有效期已保存。'
    } else if (action === 'accounts') {
      await replaceApiKeyAccounts(selectedKey.value.id, selectedIds.value)
      if (disposed) return
      formNotice.value = '授权范围已保存。'
    } else {
      await deleteApiKey(selectedKey.value.id)
      if (disposed) return
      keys.value = keys.value.filter((key) => key.id !== selectedKey.value.id)
      dialog.value.close()
      mode.value = ''
      notice.value = 'API 密钥已删除。'
    }
    await loadKeys()
  } catch (cause) {
    if (!disposed) formError.value = cause.message
  } finally { busy.value = false }
}

async function copySecret() {
  formError.value = ''
  try {
    await navigator.clipboard.writeText(secret.value)
    if (!disposed && secret.value) formNotice.value = '完整密钥已复制。'
  } catch {
    if (!disposed && secret.value) formError.value = '复制失败，请手动选择下方完整密钥并复制。'
  }
}

onMounted(loadKeys)
onUnmounted(() => { disposed = true; secret.value = '' })
</script>

<template>
  <main class="keys-page">
    <header class="keys-heading">
      <h1>API 密钥列表 <span>(api-keys)</span></h1>
      <div class="actions">
        <button :disabled="loading || busy" @click="loadKeys">{{ loading ? '加载中…' : '刷新' }}</button>
        <button class="primary" :disabled="loading || busy" @click="openDialog('create')">添加 API 密钥</button>
      </div>
    </header>
    <p class="hint">完整密钥仅在创建成功时显示一次，请及时复制并妥善保存。</p>
    <p v-if="error" class="error" role="alert">{{ error }}<span v-if="loaded"> 当前显示上次加载的数据。</span></p>
    <p v-if="notice" class="feedback" role="status">{{ notice }}</p>
    <section aria-label="API 密钥列表" :aria-busy="loading" class="key-list">
      <p v-if="loading && !loaded" class="empty" role="status">正在加载 API 密钥…</p>
      <div v-else-if="loaded && !keys.length" class="empty"><h2>暂无 API 密钥</h2><p>点击“添加 API 密钥”，选择可使用的授权账号。</p></div>
      <article v-for="key in keys" :key="key.id" class="key-card">
        <div class="key-info">
          <div class="identity"><span class="key-id">#{{ key.id }}</span><span class="status" :class="{ inactive: status(key) !== '已启用' }">{{ status(key) }}</span></div>
          <h2>API 密钥</h2>
          <code>{{ key.key_prefix }}******</code>
          <p class="metadata">有效期：{{ key.expires_at ? new Date(key.expires_at).toLocaleString('zh-CN', { hour12: false }) : '永不过期' }}</p>
          <div class="scope"><span>授权范围</span><span v-if="!key.accounts.length">暂无绑定账号</span><span v-for="account in key.accounts" :key="account.id" class="account-tag">{{ account.email || `账号 #${account.id}` }} · {{ account.type }}{{ account.disabled ? '（已禁用）' : '' }}</span></div>
        </div>
        <div class="card-controls">
          <div class="actions">
            <button disabled title="完整密钥仅在创建成功时提供，历史密钥无法再次复制">复制</button>
            <button :disabled="loading || busy" :aria-label="`编辑 API 密钥 #${key.id}`" @click="openDialog('edit', key)">编辑</button>
            <button class="danger" :disabled="loading || busy" :aria-label="`删除 API 密钥 #${key.id}`" @click="openDialog('delete', key)">删除</button>
          </div>
          <small>完整密钥仅创建时可复制</small>
        </div>
      </article>
    </section>

    <dialog ref="dialog" aria-labelledby="key-dialog-title" @cancel.prevent="closeDialog">
      <header class="dialog-heading"><h2 id="key-dialog-title">{{ mode === 'create' ? '添加 API 密钥' : mode === 'secret' ? 'API 密钥创建成功' : mode === 'edit' ? `编辑 API 密钥 #${selectedKey?.id}` : '删除 API 密钥' }}</h2><button aria-label="关闭弹窗" :disabled="busy || accountsLoading" @click="closeDialog">×</button></header>
      <div class="dialog-body">
        <p v-if="formError" class="error" role="alert">{{ formError }}</p>
        <p v-if="formNotice" class="feedback" role="status">{{ formNotice }}</p>
        <template v-if="mode === 'secret'">
          <p>请立即保存完整密钥。关闭此窗口后将无法再次查看。</p>
          <textarea aria-label="完整 API 密钥" :value="secret" readonly rows="3" spellcheck="false" @focus="$event.target.select()"></textarea>
          <div class="actions end"><button class="primary" @click="copySecret">复制完整密钥</button><button :disabled="busy" @click="closeDialog">我已保存</button></div>
        </template>
        <template v-else-if="mode === 'delete'">
          <p>确认删除 API 密钥 #{{ selectedKey?.id }}（{{ selectedKey?.key_prefix }}******）？删除后，使用此密钥的请求将无法通过认证。</p>
          <div class="actions end"><button :disabled="busy" autofocus @click="closeDialog">取消</button><button class="danger" :disabled="busy" @click="save('delete')">{{ busy ? '删除中…' : '确认删除' }}</button></div>
        </template>
        <template v-else>
          <form v-if="mode === 'edit'" @submit.prevent="save('settings')">
            <fieldset :disabled="busy">
              <legend>状态与有效期</legend>
              <label class="check"><input v-model="disabled" type="checkbox" />禁用此密钥</label>
              <label class="expiry">到期时间（本地时间，留空为永不过期）<input v-model="expiry" type="datetime-local" step="1" /></label>
              <div class="actions end"><button type="submit" class="primary">保存状态与有效期</button></div>
            </fieldset>
          </form>
          <form @submit.prevent="save(mode === 'create' ? 'create' : 'accounts')">
            <fieldset :disabled="busy || accountsLoading">
              <legend>授权账号范围</legend>
              <p class="hint">选择此密钥可使用的账号，至少选择一个。已禁用账号暂不可用于调用。</p>
              <p v-if="accountsLoading" role="status">正在加载授权账号…</p>
              <p v-else-if="accountsError" class="error" role="alert">{{ accountsError }} <button type="button" @click="loadAccounts">重试</button></p>
              <p v-else-if="!accounts.length">暂无授权账号，请先前往 <RouterLink :to="{ name: 'oauth' }">OAuth 登录</RouterLink> 添加账号。</p>
              <div v-else class="account-options">
                <label v-for="account in accounts" :key="account.id" class="check account-option"><input v-model="selectedIds" type="checkbox" :value="account.id" /><span>{{ account.email || account.accountId || `账号 #${account.id}` }}<small>{{ account.type }} · #{{ account.id }}{{ account.disabled ? ' · 已禁用' : '' }}</small></span></label>
              </div>
              <p class="hint">已选择 {{ selectedIds.length }} 个账号</p>
              <div class="actions end"><button class="primary" type="submit" :disabled="!!accountsError || !accounts.length || !selectedIds.length">{{ busy ? '保存中…' : mode === 'create' ? '创建密钥' : '保存授权范围' }}</button></div>
            </fieldset>
          </form>
        </template>
      </div>
    </dialog>
  </main>
</template>

<style scoped>
.keys-page { max-width: 1680px; margin: 0 auto; padding: 36px clamp(20px, 3.4vw, 58px) 48px; }
.keys-heading, .dialog-heading, .identity, .actions { display: flex; align-items: center; gap: 12px; }
.keys-heading, .dialog-heading { justify-content: space-between; }
h1 { margin: 0; font-size: clamp(23px, 2vw, 30px); line-height: 1.4; }
h1 span { font-weight: 600; }
h2 { margin: 0; font-size: 22px; }
button { min-height: 44px; padding: 10px 18px; border: 1px solid var(--border-color); border-radius: 11px; background: var(--bg-secondary); color: var(--text-primary); font-size: 15px; font-weight: 650; cursor: pointer; }
button:hover:not(:disabled) { filter: brightness(.94); }
button:disabled { opacity: .5; cursor: not-allowed; }
button.primary { background: var(--primary-color); border-color: var(--primary-color); color: var(--primary-contrast); }
button.danger { background: #c45543; border-color: #c45543; color: white; }
.hint, .metadata, .scope, small { color: var(--text-secondary); font-size: 13px; line-height: 1.7; }
.keys-page > .hint { margin: 12px 0 26px; }
.key-list { display: grid; gap: 16px; }
.key-card { display: flex; justify-content: space-between; align-items: center; gap: 28px; padding: 24px; border: 1px solid var(--border-color); border-radius: 16px; background: var(--floating-surface); }
.key-info { min-width: 0; }
.key-id { display: inline-block; min-width: 108px; padding: 8px 14px; border: 1px solid var(--border-color); border-radius: 9px; color: var(--text-secondary); font-size: 14px; }
.status { color: var(--success-badge-text); background: var(--success-badge-bg); border-radius: 6px; padding: 4px 8px; font-size: 12px; }
.status.inactive { color: var(--text-secondary); background: var(--bg-tertiary); }
.key-card h2 { margin: 13px 0 8px; }
code { font-size: 23px; color: var(--text-secondary); overflow-wrap: anywhere; }
.metadata { margin: 14px 0 7px; }
.scope { display: flex; flex-wrap: wrap; align-items: center; gap: 7px; }
.account-tag { padding: 2px 8px; border: 1px solid var(--border-color); border-radius: 5px; overflow-wrap: anywhere; }
.card-controls { flex-shrink: 0; text-align: right; }
.card-controls small { display: block; margin-top: 10px; }
.card-controls button { font-size: 17px; min-height: 50px; }
.empty { padding: 60px 24px; text-align: center; border: 1px dashed var(--border-primary); border-radius: 16px; color: var(--text-secondary); }
.error, .feedback { border-radius: 8px; padding: 12px; font-size: 14px; line-height: 1.7; overflow-wrap: anywhere; }
.error { color: var(--warning-color); background: var(--warning-bg); }
.feedback { color: var(--success-badge-text); background: var(--success-badge-bg); }
dialog { width: min(650px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); padding: 0; border: 1px solid var(--border-primary); border-radius: 16px; background: var(--floating-surface); color: var(--text-primary); box-shadow: 0 24px 80px #0003; }
dialog::backdrop { background: #0006; }
.dialog-heading { padding: 20px 24px; border-bottom: 1px solid var(--border-color); }
.dialog-heading button { font-size: 24px; padding: 2px 14px; }
.dialog-body { padding: 24px; line-height: 1.8; }
.dialog-body > p:first-child { margin-top: 0; }
fieldset { min-width: 0; margin: 0 0 20px; padding: 0; border: 0; }
legend { font-weight: 700; margin-bottom: 8px; }
.check { display: flex; align-items: center; gap: 12px; }
input[type=checkbox] { width: 18px; height: 18px; flex-shrink: 0; accent-color: var(--primary-color); }
.expiry { display: grid; gap: 8px; margin: 16px 0; font-size: 14px; }
input[type=datetime-local], textarea { width: 100%; min-width: 0; padding: 12px; border: 1px solid var(--border-primary); border-radius: 8px; background: var(--bg-secondary); color: var(--text-primary); font-size: 15px; }
textarea { resize: vertical; font-family: monospace; overflow-wrap: anywhere; }
.account-options { max-height: 250px; overflow-y: auto; border: 1px solid var(--border-color); border-radius: 9px; }
.account-option { padding: 12px; border-bottom: 1px solid var(--border-color); overflow-wrap: anywhere; }
.account-option:last-child { border: 0; }
.account-option small { display: block; }
.end { justify-content: flex-end; margin-top: 16px; }
a { color: var(--text-primary); }
@media (max-width: 760px) { .keys-heading { align-items: flex-start; flex-direction: column; } .key-card { align-items: stretch; flex-direction: column; padding: 20px; gap: 20px; } .card-controls { text-align: left; } .actions { flex-wrap: wrap; } .dialog-body, .dialog-heading { padding: 18px; } }
</style>
