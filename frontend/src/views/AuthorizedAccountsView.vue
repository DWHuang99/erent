<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { ArrowUpRight, ChevronDown, RefreshCw, ShieldCheck, Terminal, Trash2, UsersRound, X } from '@lucide/vue'
import { deleteOAuthAccount, getOAuthList, refreshOAuthAccount } from '../services/oauth.js'

const accounts = ref([])
const loading = ref(false)
const error = ref('')
const loaded = ref(false)
const refreshingIds = ref(new Set())
const deletingIds = ref(new Set())
const deleteDialog = ref(null)
const pendingDelete = ref(null)
const accountFeedback = ref({})
const enabledCount = computed(() => accounts.value.filter((account) => !account.disabled).length)
let disposed = false
let listRequest

async function refreshAccounts(afterMutation = false) {
  while (listRequest) {
    await listRequest
    if (!afterMutation) return
  }
  if (disposed) return
  listRequest = loadAccounts()
  try {
    await listRequest
  } finally {
    listRequest = undefined
  }
}

async function loadAccounts() {
  loading.value = true
  error.value = ''
  try {
    const items = await getOAuthList()
    if (!disposed) {
      accounts.value = items
      loaded.value = true
    }
  } catch (cause) {
    if (!disposed) error.value = cause.message
  } finally {
    if (!disposed) loading.value = false
  }
}

async function refreshCredential(account) {
  if (refreshingIds.value.has(account.id) || deletingIds.value.has(account.id) || disposed) return
  refreshingIds.value.add(account.id)
  accountFeedback.value[account.id] = { message: '正在刷新凭证…', error: false }
  try {
    await refreshOAuthAccount(account.id)
    if (disposed) return
    await refreshAccounts(true)
    if (!disposed) accountFeedback.value[account.id] = {
      message: error.value ? '凭证已刷新，账号信息未能更新，请刷新列表。' : '凭证刷新成功',
      error: Boolean(error.value),
    }
  } catch (cause) {
    if (!disposed) accountFeedback.value[account.id] = { message: cause.message, error: true }
  } finally {
    refreshingIds.value.delete(account.id)
  }
}

async function requestDelete(account) {
  if (deletingIds.value.has(account.id) || refreshingIds.value.has(account.id) || disposed) return
  pendingDelete.value = account
  await nextTick()
  if (!disposed) deleteDialog.value.showModal()
}

function cancelDelete() {
  deleteDialog.value.close()
  pendingDelete.value = null
}

function confirmDelete() {
  const account = pendingDelete.value
  if (!account) return
  cancelDelete()
  deleteAccount(account)
}

async function deleteAccount(account) {
  if (deletingIds.value.has(account.id) || refreshingIds.value.has(account.id) || disposed) return
  deletingIds.value.add(account.id)
  accountFeedback.value[account.id] = { message: '正在删除账号…', error: false }
  try {
    await deleteOAuthAccount(account.id)
    if (disposed) return
    // Wait for an older list request before removing the deleted account.
    if (listRequest) await listRequest
    if (disposed) return
    accounts.value = accounts.value.filter((item) => item.id !== account.id)
    delete accountFeedback.value[account.id]
  } catch (cause) {
    if (!disposed) accountFeedback.value[account.id] = { message: cause.message, error: true }
  } finally {
    deletingIds.value.delete(account.id)
  }
}

function handleFocus() {
  refreshAccounts()
}

function formatTime(value) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('zh-CN', { hour12: false })
}

onMounted(() => {
  refreshAccounts()
  window.addEventListener('focus', handleFocus)
})
onUnmounted(() => {
  disposed = true
  window.removeEventListener('focus', handleFocus)
})
</script>

<template>
  <main class="accounts-page">
    <header class="page-heading">
      <div>
        <h1>授权账号处理</h1>
        <p class="account-summary" aria-live="polite">
          <span><strong>{{ loaded ? accounts.length : '—' }}</strong> 个账号</span>
          <span class="separator" aria-hidden="true">·</span>
          <span class="enabled-count"><strong>{{ loaded ? enabledCount : '—' }}</strong> 个启用</span>
        </p>
      </div>
      <button class="refresh-button" :disabled="loading" :aria-busy="loading" @click="refreshAccounts()">
        <RefreshCw :size="19" :class="{ spinning: loading }" />刷新
      </button>
    </header>

    <div v-if="error" class="error-banner" role="alert">{{ error }}<span v-if="loaded"> 当前显示上次加载的账号信息。</span></div>
    <section :aria-busy="loading" aria-label="已授权账号">
      <div v-if="!loaded && loading" class="empty-state" role="status"><RefreshCw class="spinning" :size="28" /><p>正在加载授权账号…</p></div>
      <div v-else-if="loaded && accounts.length === 0" class="empty-state">
        <UsersRound :size="36" />
        <h2>暂无授权账号</h2>
        <p>连接你的 AI 账号，完成授权后即可在这里查看。</p>
        <RouterLink :to="{ name: 'oauth' }" class="authorize-link">添加授权账号 <ArrowUpRight :size="16" /></RouterLink>
      </div>
      <div v-else-if="loaded" class="account-grid">
        <article v-for="account in accounts" :key="account.id" class="account-card" :aria-label="account.email || account.accountId || '授权账号'">
          <div class="card-heading">
            <span class="provider-icon" aria-hidden="true"><Terminal v-if="account.type === 'codex'" :size="27" /><ShieldCheck v-else :size="27" /></span>
            <div class="account-identity">
              <span class="provider-badge">{{ account.type || 'OAuth' }}</span>
              <h2 :title="account.email || account.accountId">{{ account.email || account.accountId || '未提供邮箱' }}</h2>
            </div>
            <span class="status-pill" :class="{ disabled: account.disabled }"><i aria-hidden="true"></i>{{ account.disabled ? '禁用' : '启用' }}</span>
          </div>
          <p class="account-id" :title="account.accountId">{{ account.accountId || '未提供账号 ID' }}</p>
          <div class="health-heading"><span>健康状态</span><span>暂无记录</span></div>
          <div class="health-bar" aria-label="暂无健康状态记录"><div aria-hidden="true"><i v-for="segment in 20" :key="segment"></i></div><span>—</span></div>
          <p class="refresh-time">最近刷新 <time>{{ formatTime(account.lastRefresh) }}</time></p>
          <footer class="card-footer">
            <button class="credential-refresh" :disabled="refreshingIds.has(account.id) || deletingIds.has(account.id)" :aria-busy="refreshingIds.has(account.id)" :aria-label="`刷新 ${account.email || account.accountId || '账号'} 的授权凭证`" title="刷新授权凭证" @click="refreshCredential(account)">
              <RefreshCw :size="20" :class="{ spinning: refreshingIds.has(account.id) }" />
            </button>
            <button class="credential-refresh credential-delete" :disabled="deletingIds.has(account.id) || refreshingIds.has(account.id)" :aria-busy="deletingIds.has(account.id)" :aria-label="`删除 ${account.email || account.accountId || '账号'} 的授权`" title="删除授权账号" @click="requestDelete(account)">
              <Trash2 :size="20" />
            </button>
            <details class="account-details">
              <summary>账号详情 <ChevronDown :size="16" /></summary>
              <dl>
                <div><dt>令牌到期时间</dt><dd>{{ formatTime(account.expired) }}</dd></div>
                <div><dt>创建时间</dt><dd>{{ formatTime(account.createdAt) }}</dd></div>
                <div><dt>更新时间</dt><dd>{{ formatTime(account.updatedAt) }}</dd></div>
              </dl>
            </details>
            <RouterLink :to="{ name: 'oauth' }" class="reauthorize-link">重新授权 <ArrowUpRight :size="16" /></RouterLink>
          </footer>
          <p class="credential-feedback" :class="{ 'is-error': accountFeedback[account.id]?.error }" role="status" aria-live="polite">{{ accountFeedback[account.id]?.message || '\u00a0' }}</p>
        </article>
      </div>
    </section>
    <dialog ref="deleteDialog" class="delete-dialog" aria-labelledby="delete-dialog-title" aria-describedby="delete-dialog-description" @cancel.prevent="cancelDelete">
      <header class="delete-dialog-heading">
        <h2 id="delete-dialog-title">删除账号授权</h2>
        <button class="delete-dialog-close" aria-label="关闭" @click="cancelDelete"><X :size="22" /></button>
      </header>
      <p id="delete-dialog-description">确认要删除 {{ pendingDelete?.email || pendingDelete?.accountId || '当前' }} 账号的授权？</p>
      <footer class="delete-dialog-actions">
        <button class="delete-cancel" autofocus @click="cancelDelete">取消</button>
        <button class="delete-confirm" @click="confirmDelete">确认</button>
      </footer>
    </dialog>
  </main>
</template>

<style scoped>
.delete-dialog { width: min(620px, calc(100vw - 32px)); max-height: calc(100dvh - 32px); box-sizing: border-box; padding: 0; border: 1px solid var(--border-primary); border-radius: 16px; background: var(--floating-surface); color: var(--text-primary); box-shadow: 0 24px 80px #0003; }
.delete-dialog::backdrop { background: #0006; }
.delete-dialog-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 20px 24px; border-bottom: 1px solid var(--border-color); }
.delete-dialog-heading h2 { margin: 0; font-size: 22px; font-weight: 700; }
.delete-dialog-close { display: grid; place-items: center; flex-shrink: 0; width: 38px; height: 38px; padding: 0; border: 1px solid var(--border-color); border-radius: 50%; background: transparent; color: var(--text-secondary); cursor: pointer; }
.delete-dialog p { margin: 0; padding: 42px 24px; font-size: 17px; line-height: 1.8; overflow-wrap: anywhere; }
.delete-dialog-actions { display: flex; justify-content: flex-end; gap: 16px; padding: 0 24px 24px; }
.delete-dialog-actions button { min-height: 44px; padding: 10px 22px; border: 0; border-radius: 10px; font-size: 16px; font-weight: 650; cursor: pointer; }
.delete-cancel { background: transparent; color: var(--text-secondary); }
.delete-confirm { background: #c45543; color: white; }
.delete-confirm:hover { background: #ae4434; }
.delete-cancel:hover, .delete-dialog-close:hover { background: var(--bg-secondary); }
.delete-dialog button:focus-visible { outline: 2px solid var(--primary-color); outline-offset: 3px; }
.accounts-page { max-width: 1680px; margin: 0 auto; padding: 40px clamp(20px, 3.4vw, 58px) 48px; }
.page-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; margin-bottom: 42px; }
h1 { margin: 0 0 16px; font-size: clamp(30px, 3.2vw, 44px); line-height: 1.25; letter-spacing: -.04em; font-weight: 800; }
.account-summary { display: flex; align-items: center; flex-wrap: wrap; gap: 13px; margin: 0; padding-left: 15px; border-left: 4px solid var(--success-color); color: var(--text-secondary); font-size: 15px; line-height: 1.3; }
.account-summary strong { margin-right: 5px; font-variant-numeric: tabular-nums; }
.separator { color: var(--text-tertiary); }
.enabled-count { color: var(--success-color); }
.refresh-button { display: inline-flex; align-items: center; gap: 9px; padding: 9px 0 9px 12px; border: 0; background: transparent; color: var(--text-secondary); font-size: 15px; cursor: pointer; white-space: nowrap; }
.refresh-button:hover { color: var(--text-primary); }
.refresh-button:disabled { cursor: wait; }
.refresh-button svg { flex-shrink: 0; }
.account-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 390px), 1fr)); gap: 24px; align-items: start; }
.account-card { min-width: 0; padding: 24px; border: 1px solid var(--border-primary); border-radius: 20px; background: var(--floating-surface); }
.card-heading { display: flex; align-items: flex-start; gap: 14px; }
.provider-icon { display: grid; place-items: center; flex: 0 0 48px; height: 48px; border: 9px solid #eae6ff; border-radius: 14px; background: linear-gradient(140deg, #b9aaff, #665eff); color: white; }
.account-identity { flex: 1; min-width: 0; }
.provider-badge { display: inline-block; padding: 4px 9px; border-radius: 8px; background: #eae6ff; color: #493be6; font-size: 12px; line-height: 1.2; font-weight: 750; letter-spacing: .06em; text-transform: uppercase; }
.account-identity h2 { overflow-wrap: anywhere; margin: 10px 0 0; font-size: 18px; line-height: 1.4; font-weight: 650; }
.status-pill { display: inline-flex; align-items: center; gap: 7px; flex-shrink: 0; padding: 5px 10px; border: 1px solid var(--success-badge-border); border-radius: 999px; background: var(--success-badge-bg); color: var(--success-badge-text); font-size: 12px; }
.status-pill i { width: 6px; height: 6px; border-radius: 50%; background: var(--success-color); }
.status-pill.disabled { border-color: var(--border-primary); background: var(--bg-tertiary); color: var(--text-secondary); }
.status-pill.disabled i { background: var(--text-secondary); }
.account-id { margin: 18px 0 25px; color: var(--text-secondary); font-family: ui-monospace, monospace; font-size: 13px; line-height: 1.6; overflow-wrap: anywhere; }
.health-heading { display: flex; justify-content: space-between; gap: 12px; color: var(--text-secondary); font-size: 13px; }
.health-bar { display: flex; align-items: center; gap: 20px; margin-top: 14px; color: var(--text-tertiary); }
.health-bar > div { display: grid; grid-template-columns: repeat(20, minmax(0, 1fr)); gap: 3px; flex: 1; }
.health-bar i { height: 19px; border-radius: 3px; background: var(--bg-tertiary); }
.refresh-time { display: flex; flex-wrap: wrap; gap: 8px; margin: 21px 0 23px; font-size: 13px; color: var(--text-secondary); }
time { font-variant-numeric: tabular-nums; }
.card-footer { position: relative; padding-top: 17px; border-top: 1px solid var(--border-color); }
.credential-refresh { position: absolute; top: 17px; left: 112px; display: grid; place-items: center; width: 39px; height: 39px; padding: 0; border: 1px solid var(--border-color); border-radius: 10px; background: var(--floating-surface); color: var(--text-secondary); cursor: pointer; }
.credential-refresh:hover:not(:disabled) { background: var(--bg-secondary); color: var(--text-primary); }
.credential-refresh:disabled { cursor: wait; }
.credential-delete { left: 161px; color: #c2413b; }
.credential-delete:hover:not(:disabled) { color: #b42318; background: var(--warning-bg); }
.credential-refresh:disabled { opacity: .5; }
.credential-refresh:focus-visible { outline: 2px solid var(--primary-color); outline-offset: 3px; }
.account-card { container-type: inline-size; }

.credential-feedback { min-height: 20px; margin: 12px 0 0; font-size: 12px; line-height: 20px; color: var(--success-badge-text); overflow-wrap: anywhere; }
.credential-feedback.is-error { color: var(--warning-color); }
.account-details summary { display: inline-flex; align-items: center; gap: 8px; min-height: 39px; padding: 8px 12px; border: 1px solid var(--border-color); border-radius: 10px; background: var(--bg-secondary); font-size: 13px; font-weight: 650; cursor: pointer; list-style: none; }
.account-details summary::-webkit-details-marker { display: none; }
.account-details summary:focus-visible { outline: 2px solid var(--primary-color); outline-offset: 3px; }
.account-details[open] summary svg { transform: rotate(180deg); }
.account-details dl { display: grid; gap: 12px; margin: 18px 0 0; padding-top: 16px; border-top: 1px solid var(--border-color); font-size: 12px; }
.account-details dl > div { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 8px; }
.account-details dt { color: var(--text-secondary); }
.account-details dd { margin: 0; font-variant-numeric: tabular-nums; }
.reauthorize-link { position: absolute; top: 17px; right: 0; display: inline-flex; align-items: center; gap: 6px; min-height: 39px; color: var(--text-secondary); font-size: 13px; text-decoration: none; }
.reauthorize-link:hover { color: var(--text-primary); }
.empty-state { display: flex; flex-direction: column; align-items: center; padding: 70px 24px; border: 1px dashed var(--border-primary); border-radius: 20px; color: var(--text-secondary); text-align: center; }
.empty-state h2 { margin: 20px 0 0; color: var(--text-primary); font-size: 20px; }
.empty-state p { font-size: 14px; line-height: 1.7; }
.authorize-link { display: inline-flex; align-items: center; gap: 8px; margin-top: 10px; color: var(--text-primary); text-underline-offset: 4px; }
.error-banner { margin-bottom: 24px; padding: 14px 18px; border-radius: 10px; color: var(--warning-color); background: var(--warning-bg); font-size: 14px; line-height: 1.6; }
.spinning { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
@media (min-width: 1400px) { .account-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 620px) { .accounts-page { padding-top: 28px; } .page-heading { gap: 12px; margin-bottom: 30px; } .account-summary { font-size: 13px; gap: 9px; } .account-card { padding: 20px 16px; border-radius: 16px; } .card-heading { gap: 10px; flex-wrap: wrap; } .provider-icon { flex-basis: 42px; height: 42px; border-width: 7px; } .account-identity h2 { font-size: 16px; } .status-pill { padding: 4px 8px; } }
@container (max-width: 310px) { .reauthorize-link { position: static; margin-top: 12px; } }
</style>
