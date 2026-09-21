<script setup>
import { nextTick, onMounted, onUnmounted, ref } from 'vue'
import { getExternalApiKeys, createExternalApiKey, updateExternalApiKey, deleteExternalApiKey } from '../services/external-apikey.js'

const keys = ref([])
const loaded = ref(false)
const loading = ref(false)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const dialog = ref(null)
const mode = ref('')
const selectedKey = ref(null)
const endpoint = ref('')
const credential = ref('')
const suffix = ref({ chat_completions: '', responses: '', messages: '' })
const formError = ref('')
const protocols = [
  { field: 'chat_completions', label: 'Chat Completions', path: '/v1/chat/completions' },
  { field: 'responses', label: 'Responses', path: '/v1/responses' },
  { field: 'messages', label: 'Messages', path: '/v1/messages' },
]
let disposed = false

async function loadKeys() {
  if (loading.value || disposed) return
  loading.value = true
  error.value = ''
  try {
    const result = await getExternalApiKeys()
    if (!disposed) { keys.value = result; loaded.value = true }
  } catch (cause) {
    if (!disposed) error.value = cause.message
  } finally { loading.value = false }
}

async function openDialog(action, key = null) {
  mode.value = action
  selectedKey.value = key
  endpoint.value = key?.endpoint ?? ''
  credential.value = ''
  suffix.value = { chat_completions: '', responses: '', messages: '', ...key?.suffix }
  formError.value = ''
  await nextTick()
  if (!disposed) dialog.value.showModal()
}

function closeDialog() {
  if (busy.value) return
  dialog.value.close()
  credential.value = ''
  mode.value = ''
}

async function save() {
  if (busy.value) return
  formError.value = ''
  notice.value = ''
  if (mode.value !== 'delete' && (!credential.value.trim() || !endpoint.value.trim())) {
    formError.value = '请填写服务地址与完整 APIKey。'
    return
  }
  busy.value = true
  try {
    if (mode.value === 'delete') {
      await deleteExternalApiKey(selectedKey.value.id)
    } else {
      const data = { endpoint: endpoint.value.trim(), external_apikey: credential.value, suffix: { ...suffix.value } }
      if (mode.value === 'create') await createExternalApiKey(data)
      else await updateExternalApiKey(selectedKey.value.id, data)
    }
    if (disposed) return
    notice.value = mode.value === 'delete' ? '外部密钥已删除。' : '外部密钥已保存。'
    credential.value = ''
    dialog.value.close()
    mode.value = ''
    await loadKeys()
  } catch (cause) {
    if (!disposed) formError.value = cause.message
  } finally { busy.value = false }
}

onMounted(loadKeys)
onUnmounted(() => { disposed = true; credential.value = '' })
</script>

<template>
  <main class="keys-page">
    <header class="keys-heading">
      <h1>外部服务商 APIKey</h1>
      <div class="actions">
        <button :disabled="loading || busy" @click="loadKeys">{{ loading ? '加载中…' : '刷新' }}</button>
        <button class="primary" :disabled="loading || busy" @click="openDialog('create')">添加外部 APIKey</button>
      </div>
    </header>
    <p class="hint">管理外部服务商连接，随后可在创建 <RouterLink :to="{ name: 'api-keys' }">API 密钥</RouterLink> 时选择授权。密钥保存后不再显示。</p>
    <p v-if="error" class="error" role="alert">{{ error }}<span v-if="loaded"> 当前显示上次加载的数据。</span></p>
    <p v-if="notice" class="feedback" role="status">{{ notice }}</p>
    <section class="key-list" aria-label="外部服务商 APIKey 列表" :aria-busy="loading">
      <p v-if="loading && !loaded" class="empty" role="status">正在加载外部密钥…</p>
      <div v-else-if="loaded && !keys.length" class="empty"><h2>暂无外部服务商 APIKey</h2><p>添加服务地址与 APIKey，接入外部服务商。</p></div>
      <article v-for="key in keys" :key="key.id" class="key-card">
        <div class="key-info">
          <span class="key-id">外部密钥 #{{ key.id }}</span>
          <h2 class="endpoint">{{ key.endpoint }}</h2>
          <p class="hint">APIKey 已保存 · 原文不可查看</p>
          <div v-for="protocol in protocols" :key="protocol.field" class="metadata protocol">{{ protocol.label }}：<code>{{ key.suffix?.[protocol.field] || protocol.path }}</code></div>
        </div>
        <div class="actions card-controls">
          <button :disabled="loading || busy" :aria-label="`编辑外部密钥 #${key.id}`" @click="openDialog('edit', key)">编辑</button>
          <button class="danger" :disabled="loading || busy" :aria-label="`删除外部密钥 #${key.id}`" @click="openDialog('delete', key)">删除</button>
        </div>
      </article>
    </section>
    <dialog ref="dialog" aria-labelledby="external-dialog-title" @cancel.prevent="closeDialog">
      <header class="dialog-heading"><h2 id="external-dialog-title">{{ mode === 'create' ? '添加外部 APIKey' : mode === 'edit' ? '编辑外部 APIKey' : '删除外部 APIKey' }}</h2><button aria-label="关闭弹窗" :disabled="busy" @click="closeDialog">×</button></header>
      <div class="dialog-body">
        <p v-if="formError" class="error" role="alert">{{ formError }}</p>
        <template v-if="mode === 'delete'">
          <p>确认删除外部密钥 #{{ selectedKey?.id }}（{{ selectedKey?.endpoint }}）？已授权的 API 密钥将无法再使用此连接。</p>
          <div class="actions end"><button :disabled="busy" autofocus @click="closeDialog">取消</button><button class="danger" :disabled="busy" @click="save">{{ busy ? '删除中…' : '确认删除' }}</button></div>
        </template>
        <form v-else @submit.prevent="save">
          <fieldset :disabled="busy">
            <label class="field">服务地址（Endpoint）<input v-model="endpoint" type="url" required placeholder="https://api.example.com" /></label>
            <p class="hint">使用 HTTP 或 HTTPS 地址，不含用户名、密码、查询参数或片段。请求路径会追加到此地址后。</p>
            <label class="field">{{ mode === 'edit' ? '重新填写完整 APIKey' : 'APIKey' }}<input v-model="credential" type="password" required autocomplete="new-password" spellcheck="false" /></label>
            <p v-if="mode === 'edit'" class="hint">修改连接时需重新填写密钥，可使用原密钥。</p>
            <h3>协议路径后缀（选填）</h3>
            <p class="hint">留空使用下方默认路径；可填写自定义路径，不含域名、查询参数或片段。</p>
            <label v-for="protocol in protocols" :key="protocol.field" class="field">{{ protocol.label }}<input v-model="suffix[protocol.field]" type="text" :placeholder="protocol.path" spellcheck="false" /></label>
            <div class="actions end"><button type="button" @click="closeDialog">取消</button><button class="primary" type="submit">{{ busy ? '保存中…' : '保存外部 APIKey' }}</button></div>
          </fieldset>
        </form>
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

.field { display: grid; gap: 8px; margin: 16px 0; font-size: 14px; }
.field input { width: 100%; min-width: 0; padding: 12px; border: 1px solid var(--border-primary); border-radius: 8px; background: var(--bg-secondary); color: var(--text-primary); font-size: 15px; }
.endpoint { overflow-wrap: anywhere; }
.protocol code { font-size: 14px; }
</style>
