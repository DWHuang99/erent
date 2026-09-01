<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  Activity,
  ArrowUpRight,
  Bell,
  Boxes,
  ChevronRight,
  CircleGauge,
  FileClock,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Menu,
  Moon,
  RefreshCw,
  ServerCog,
  ShieldCheck,
  Sun,
  UsersRound,
  X,
} from '@lucide/vue'

import AppLogo from '../components/AppLogo.vue'
import { clearSession, getCurrentUser, logout } from '../services/auth.js'

const router = useRouter()
const loading = ref(true)
const refreshing = ref(false)
const sidebarOpen = ref(false)
const userMenuOpen = ref(false)
const errorMessage = ref('')
const user = ref(null)
const serviceOnline = ref(false)
const lastCheckedAt = ref(new Date())
const theme = ref(globalThis.localStorage?.getItem('ai_gateway_theme') ?? 'light')

const initials = computed(() => (user.value?.username ?? 'AG').slice(0, 2).toUpperCase())
const primaryRole = computed(() => user.value?.roles?.[0] ?? user.value?.roleCode ?? 'user')
const permissionCount = computed(() => user.value?.permissions?.length ?? 0)
const checkedTime = computed(() =>
  lastCheckedAt.value.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }),
)

const navGroups = [
  {
    label: '运行',
    items: [
      { label: '运行总览', icon: LayoutDashboard, active: true },
      { label: '用量统计', icon: Activity, upcoming: true },
    ],
  },
  {
    label: '网关',
    items: [
      { label: 'API Key', icon: KeyRound, upcoming: true },
      { label: 'Provider', icon: Boxes, upcoming: true },
      { label: '调用日志', icon: FileClock, upcoming: true },
    ],
  },
  {
    label: '控制',
    items: [
      { label: '用户与权限', icon: UsersRound, upcoming: true },
      { label: '系统设置', icon: ServerCog, upcoming: true },
    ],
  },
]

const capabilities = computed(() => [
  {
    icon: ShieldCheck,
    title: '访问认证',
    description: 'JWT 访问凭证与 HttpOnly 刷新会话已启用',
    status: serviceOnline.value ? '运行中' : '等待服务',
    tone: serviceOnline.value ? 'success' : 'neutral',
  },
  {
    icon: UsersRound,
    title: '权限控制',
    description: `${primaryRole.value} 角色 · ${permissionCount.value} 项有效权限`,
    status: '已授权',
    tone: 'success',
  },
  {
    icon: Boxes,
    title: 'Provider 路由',
    description: '上游模型接入与调度能力将在后续版本开放',
    status: '规划中',
    tone: 'neutral',
  },
])

async function loadDashboard(showRefresh = false) {
  if (showRefresh) refreshing.value = true
  errorMessage.value = ''

  try {
    const [currentUser, healthResponse] = await Promise.all([
      getCurrentUser(),
      fetch('/health/ready').catch(() => null),
    ])
    user.value = currentUser
    serviceOnline.value = healthResponse?.ok ?? false
    lastCheckedAt.value = new Date()
  } catch (error) {
    clearSession()
    errorMessage.value = error instanceof Error ? error.message : '登录状态已失效'
    await router.replace({ name: 'login', query: { redirect: '/' } })
  } finally {
    loading.value = false
    refreshing.value = false
  }
}

function applyTheme() {
  document.documentElement.dataset.theme = theme.value
}

function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  globalThis.localStorage?.setItem('ai_gateway_theme', theme.value)
  applyTheme()
}

async function handleLogout() {
  await logout()
  await router.replace({ name: 'login' })
}

function closeMenus(event) {
  if (!event.target.closest('.account-area')) userMenuOpen.value = false
}

onMounted(() => {
  applyTheme()
  document.addEventListener('click', closeMenus)
  loadDashboard()
})

onUnmounted(() => document.removeEventListener('click', closeMenus))
</script>

<template>
  <div class="dashboard-shell">
    <div v-if="sidebarOpen" class="sidebar-backdrop" @click="sidebarOpen = false"></div>

    <aside class="sidebar" :class="{ open: sidebarOpen }">
      <div class="sidebar-heading">
        <AppLogo inverse />
        <button class="sidebar-close" aria-label="关闭导航" @click="sidebarOpen = false">
          <X :size="19" />
        </button>
      </div>

      <nav class="navigation" aria-label="主导航">
        <section v-for="group in navGroups" :key="group.label" class="nav-group">
          <h2>{{ group.label }}</h2>
          <button
            v-for="item in group.items"
            :key="item.label"
            class="nav-item"
            :class="{ active: item.active }"
            :disabled="item.upcoming"
            :title="item.upcoming ? '该功能尚未开放' : item.label"
          >
            <component :is="item.icon" :size="18" />
            <span>{{ item.label }}</span>
            <span v-if="item.upcoming" class="soon-badge">Soon</span>
          </button>
        </section>
      </nav>

      <div class="sidebar-status">
        <div class="status-row">
          <span>API 服务</span>
          <span class="connection-state" :class="{ online: serviceOnline }">
            <i></i>{{ serviceOnline ? '已连接' : '检查中' }}
          </span>
        </div>
        <p>本地管理控制台</p>
      </div>
    </aside>

    <div class="workspace">
      <header class="topbar">
        <div class="topbar-left">
          <button class="icon-button menu-button" aria-label="打开导航" @click="sidebarOpen = true">
            <Menu :size="20" />
          </button>
          <div>
            <span class="breadcrumb">运行 /</span>
            <strong>运行总览</strong>
          </div>
        </div>

        <div class="topbar-actions">
          <button class="icon-button" :aria-label="theme === 'dark' ? '切换浅色主题' : '切换深色主题'" @click="toggleTheme">
            <Sun v-if="theme === 'dark'" :size="18" />
            <Moon v-else :size="18" />
          </button>
          <button class="icon-button notification-button" aria-label="通知">
            <Bell :size="18" />
            <span></span>
          </button>
          <div class="account-area">
            <button class="account-button" @click.stop="userMenuOpen = !userMenuOpen">
              <span class="avatar">{{ initials }}</span>
              <span class="account-copy">
                <strong>{{ user?.username ?? '正在载入' }}</strong>
                <small>{{ primaryRole }}</small>
              </span>
              <ChevronRight :size="15" :class="{ rotated: userMenuOpen }" />
            </button>
            <div v-if="userMenuOpen" class="account-menu">
              <button @click="handleLogout"><LogOut :size="16" />退出登录</button>
            </div>
          </div>
        </div>
      </header>

      <main class="dashboard-content">
        <section class="page-heading">
          <div>
            <span class="eyebrow">OVERVIEW</span>
            <h1>运行总览</h1>
            <p>查看 AI Gateway 的连接状态、当前身份与已开放能力。</p>
          </div>
          <button class="refresh-button" :disabled="refreshing" @click="loadDashboard(true)">
            <RefreshCw :size="16" :class="{ spinning: refreshing }" />
            {{ refreshing ? '刷新中' : '刷新状态' }}
          </button>
        </section>

        <div v-if="errorMessage" class="error-banner" role="alert">{{ errorMessage }}</div>

        <section class="status-hero" :class="{ loading }">
          <div class="hero-copy">
            <span class="hero-kicker">
              <i :class="{ online: serviceOnline }"></i>
              {{ loading ? '正在检查服务' : serviceOnline ? '服务已连接' : '服务状态未知' }}
            </span>
            <h2>{{ loading ? '正在同步运行状态…' : serviceOnline ? '网关运行平稳' : '登录成功，等待健康检查' }}</h2>
            <p>身份认证与权限控制已就绪。最近检查于 {{ checkedTime }}。</p>
          </div>
          <div class="hero-score">
            <span>SESSION</span>
            <strong>{{ serviceOnline ? 'LIVE' : '—' }}</strong>
            <small>secure access</small>
          </div>
        </section>

        <section class="metric-grid" aria-label="当前状态摘要">
          <article class="metric-card">
            <div class="metric-icon"><UsersRound :size="19" /></div>
            <div>
              <span>当前账号</span>
              <strong>{{ user?.username ?? '—' }}</strong>
              <small>已通过身份验证</small>
            </div>
          </article>
          <article class="metric-card">
            <div class="metric-icon"><ShieldCheck :size="19" /></div>
            <div>
              <span>有效角色</span>
              <strong>{{ primaryRole }}</strong>
              <small>Casbin RBAC</small>
            </div>
          </article>
          <article class="metric-card">
            <div class="metric-icon"><KeyRound :size="19" /></div>
            <div>
              <span>有效权限</span>
              <strong>{{ permissionCount }}</strong>
              <small>{{ user?.permissions?.[0] ?? '暂无权限明细' }}</small>
            </div>
          </article>
          <article class="metric-card">
            <div class="metric-icon"><CircleGauge :size="19" /></div>
            <div>
              <span>API 状态</span>
              <strong>{{ serviceOnline ? 'Ready' : '—' }}</strong>
              <small>PostgreSQL + Redis</small>
            </div>
          </article>
        </section>

        <section class="content-grid">
          <article class="panel capability-panel">
            <div class="panel-heading">
              <div>
                <span class="panel-kicker">CAPABILITIES</span>
                <h2>服务能力</h2>
              </div>
              <span class="panel-note">当前版本</span>
            </div>

            <div class="capability-list">
              <div v-for="item in capabilities" :key="item.title" class="capability-item">
                <span class="capability-icon"><component :is="item.icon" :size="19" /></span>
                <div>
                  <strong>{{ item.title }}</strong>
                  <p>{{ item.description }}</p>
                </div>
                <span class="status-badge" :class="item.tone">{{ item.status }}</span>
              </div>
            </div>
          </article>

          <article class="panel identity-panel">
            <div class="panel-heading">
              <div>
                <span class="panel-kicker">IDENTITY</span>
                <h2>当前身份</h2>
              </div>
              <ShieldCheck :size="20" />
            </div>
            <div class="identity-profile">
              <span class="large-avatar">{{ initials }}</span>
              <div>
                <strong>{{ user?.username ?? '—' }}</strong>
                <p>安全会话已建立</p>
              </div>
            </div>
            <dl class="identity-details">
              <div><dt>用户 ID</dt><dd>{{ user?.id ?? '—' }}</dd></div>
              <div><dt>角色</dt><dd>{{ primaryRole }}</dd></div>
              <div><dt>状态</dt><dd class="active-text">正常</dd></div>
            </dl>
          </article>
        </section>

        <section class="coming-panel">
          <div class="coming-icon"><ArrowUpRight :size="21" /></div>
          <div>
            <span>下一阶段</span>
            <h2>Provider、API Key 与用量能力正在规划</h2>
            <p>导航入口已预留；待后端接口完成后，可在现有页面结构上直接扩展。</p>
          </div>
        </section>
      </main>
    </div>
  </div>
</template>

<style scoped>
.dashboard-shell {
  display: flex;
  min-height: 100vh;
  background: var(--bg-secondary);
}

.sidebar {
  position: sticky;
  top: 0;
  z-index: 30;
  display: flex;
  width: 248px;
  height: 100vh;
  flex: 0 0 248px;
  flex-direction: column;
  background: #151412;
  color: rgb(255 253 249 / 68%);
}

.sidebar-heading {
  display: flex;
  height: 78px;
  align-items: center;
  justify-content: space-between;
  padding: 0 22px;
  border-bottom: 1px solid rgb(255 255 255 / 7%);
}

.sidebar-close {
  display: none;
  border: 0;
  background: transparent;
  color: inherit;
}

.navigation {
  flex: 1;
  overflow-y: auto;
  padding: 14px 12px;
}

.nav-group {
  margin-bottom: 18px;
}

.nav-group h2 {
  margin: 0 10px 7px;
  color: rgb(255 253 249 / 28%);
  font-size: 9px;
  font-weight: 800;
  letter-spacing: 0.18em;
  text-transform: uppercase;
}

.nav-item {
  display: flex;
  width: 100%;
  min-height: 42px;
  align-items: center;
  gap: 11px;
  margin-bottom: 3px;
  padding: 0 11px;
  border: 0;
  border-radius: 7px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  font: inherit;
  font-size: 13px;
  text-align: left;
}

.nav-item:not(:disabled):hover,
.nav-item.active {
  background: rgb(255 255 255 / 8%);
  color: #fffdf9;
}

.nav-item.active {
  box-shadow: inset 2px 0 0 #fffdf9;
}

.nav-item:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.soon-badge {
  margin-left: auto;
  padding: 2px 6px;
  border: 1px solid rgb(255 255 255 / 12%);
  border-radius: 999px;
  color: rgb(255 255 255 / 44%);
  font-size: 8px;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.sidebar-status {
  margin: 12px;
  padding: 14px;
  border: 1px solid rgb(255 255 255 / 8%);
  border-radius: 8px;
  background: rgb(255 255 255 / 4%);
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  color: #fffdf9;
  font-size: 11px;
  font-weight: 700;
}

.connection-state {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: rgb(255 255 255 / 48%);
  font-size: 10px;
}

.connection-state i,
.hero-kicker i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--text-tertiary);
}

.connection-state.online,
.connection-state.online i {
  color: #6ee7b7;
}

.connection-state.online i,
.hero-kicker i.online {
  background: var(--success-color);
  box-shadow: 0 0 0 4px rgb(16 185 129 / 12%);
}

.sidebar-status p {
  margin: 8px 0 0;
  color: rgb(255 255 255 / 30%);
  font-size: 9px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.workspace {
  min-width: 0;
  flex: 1;
}

.topbar {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  height: 66px;
  align-items: center;
  justify-content: space-between;
  padding: 0 clamp(18px, 3vw, 38px);
  border-bottom: 1px solid var(--border-color);
  background: color-mix(in srgb, var(--bg-primary) 88%, transparent);
  backdrop-filter: blur(12px);
}

.topbar-left,
.topbar-actions {
  display: flex;
  align-items: center;
}

.topbar-left {
  gap: 12px;
  color: var(--text-secondary);
  font-size: 12px;
}

.topbar-left strong {
  margin-left: 4px;
  color: var(--text-primary);
}

.topbar-actions {
  gap: 7px;
}

.icon-button {
  position: relative;
  display: grid;
  width: 35px;
  height: 35px;
  border: 1px solid var(--border-color);
  border-radius: 7px;
  background: var(--bg-primary);
  color: var(--text-secondary);
  cursor: pointer;
  place-items: center;
}

.icon-button:hover {
  border-color: var(--border-hover);
  color: var(--text-primary);
}

.menu-button {
  display: none;
}

.notification-button > span {
  position: absolute;
  top: 7px;
  right: 7px;
  width: 5px;
  height: 5px;
  border: 1px solid var(--bg-primary);
  border-radius: 50%;
  background: var(--warning-color);
}

.account-area {
  position: relative;
  margin-left: 5px;
}

.account-button {
  display: flex;
  height: 42px;
  align-items: center;
  gap: 9px;
  padding: 0 8px;
  border: 0;
  border-radius: 8px;
  background: transparent;
  color: var(--text-primary);
  cursor: pointer;
  font: inherit;
  text-align: left;
}

.account-button:hover {
  background: var(--bg-tertiary);
}

.avatar,
.large-avatar {
  display: grid;
  border-radius: 7px;
  background: var(--text-primary);
  color: var(--bg-primary);
  font-weight: 800;
  place-items: center;
}

.avatar {
  width: 29px;
  height: 29px;
  font-size: 10px;
}

.account-copy {
  display: flex;
  min-width: 76px;
  flex-direction: column;
}

.account-copy strong {
  max-width: 110px;
  overflow: hidden;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-copy small {
  margin-top: 2px;
  color: var(--text-tertiary);
  font-size: 9px;
  text-transform: capitalize;
}

.account-button svg {
  color: var(--text-tertiary);
  transition: transform 180ms ease;
}

.account-button svg.rotated {
  transform: rotate(90deg);
}

.account-menu {
  position: absolute;
  top: calc(100% + 7px);
  right: 0;
  width: 150px;
  padding: 5px;
  border: 1px solid var(--border-color);
  border-radius: 8px;
  background: var(--floating-surface);
  box-shadow: var(--floating-shadow);
}

.account-menu button {
  display: flex;
  width: 100%;
  align-items: center;
  gap: 9px;
  padding: 9px 10px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--warning-color);
  cursor: pointer;
  font: inherit;
  font-size: 12px;
}

.account-menu button:hover {
  background: var(--warning-bg);
}

.dashboard-content {
  width: min(100%, 1440px);
  margin: 0 auto;
  padding: 34px clamp(18px, 3vw, 38px) 54px;
}

.page-heading {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 24px;
}

.eyebrow,
.panel-kicker {
  color: var(--text-tertiary);
  font-size: 9px;
  font-weight: 800;
  letter-spacing: 0.18em;
}

.page-heading h1 {
  margin: 6px 0 5px;
  color: var(--text-primary);
  font-size: clamp(26px, 3vw, 36px);
  letter-spacing: -0.045em;
}

.page-heading p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 13px;
}

.refresh-button {
  display: inline-flex;
  height: 37px;
  align-items: center;
  gap: 8px;
  padding: 0 13px;
  border: 1px solid var(--border-primary);
  border-radius: 7px;
  background: var(--bg-primary);
  color: var(--text-primary);
  cursor: pointer;
  font: inherit;
  font-size: 11px;
  font-weight: 700;
}

.refresh-button:hover:not(:disabled) {
  border-color: var(--border-hover);
  background: var(--bg-tertiary);
}

.refresh-button:disabled {
  cursor: wait;
  opacity: 0.6;
}

.spinning {
  animation: spin 0.8s linear infinite;
}

.error-banner {
  margin-bottom: 16px;
  padding: 11px 14px;
  border: 1px solid var(--warning-border);
  border-radius: 8px;
  background: var(--warning-bg);
  color: var(--warning-color);
  font-size: 12px;
}

.status-hero {
  position: relative;
  display: flex;
  min-height: 230px;
  align-items: flex-end;
  justify-content: space-between;
  overflow: hidden;
  padding: 32px;
  border-radius: 12px;
  background: #151412;
  color: #fffdf9;
}

.status-hero::before {
  position: absolute;
  right: -100px;
  bottom: -210px;
  width: 440px;
  height: 440px;
  border: 1px solid rgb(255 255 255 / 10%);
  border-radius: 50%;
  box-shadow: 0 0 0 54px rgb(255 255 255 / 2%), 0 0 0 110px rgb(255 255 255 / 2%);
  content: '';
}

.status-hero.loading {
  opacity: 0.88;
}

.hero-copy,
.hero-score {
  position: relative;
  z-index: 1;
}

.hero-kicker {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: rgb(255 253 249 / 52%);
  font-size: 10px;
  font-weight: 800;
  letter-spacing: 0.14em;
}

.hero-copy h2 {
  margin: 13px 0 7px;
  font-size: clamp(26px, 4vw, 42px);
  letter-spacing: -0.05em;
}

.hero-copy p {
  margin: 0;
  color: rgb(255 253 249 / 48%);
  font-size: 12px;
}

.hero-score {
  display: flex;
  min-width: 115px;
  flex-direction: column;
  align-items: flex-end;
}

.hero-score span,
.hero-score small {
  color: rgb(255 253 249 / 35%);
  font-size: 8px;
  font-weight: 800;
  letter-spacing: 0.19em;
}

.hero-score strong {
  margin: 5px 0;
  font-size: 34px;
  letter-spacing: -0.05em;
}

.metric-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-top: 12px;
}

.metric-card {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 13px;
  padding: 17px;
  border: 1px solid var(--border-color);
  border-radius: 9px;
  background: var(--bg-primary);
}

.metric-icon,
.capability-icon,
.coming-icon {
  display: grid;
  flex: 0 0 auto;
  border-radius: 7px;
  background: var(--bg-tertiary);
  color: var(--text-secondary);
  place-items: center;
}

.metric-icon {
  width: 38px;
  height: 38px;
}

.metric-card > div:last-child {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.metric-card span,
.metric-card small {
  overflow: hidden;
  color: var(--text-tertiary);
  font-size: 9px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-card strong {
  overflow: hidden;
  margin: 4px 0 2px;
  color: var(--text-primary);
  font-size: 16px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.content-grid {
  display: grid;
  grid-template-columns: minmax(0, 1.7fr) minmax(280px, 0.8fr);
  gap: 12px;
  margin-top: 12px;
}

.panel {
  padding: 22px;
  border: 1px solid var(--border-color);
  border-radius: 9px;
  background: var(--bg-primary);
}

.panel-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 17px;
  color: var(--text-tertiary);
}

.panel-heading h2 {
  margin: 5px 0 0;
  color: var(--text-primary);
  font-size: 16px;
  letter-spacing: -0.02em;
}

.panel-note {
  font-size: 10px;
}

.capability-list {
  border-top: 1px solid var(--border-color);
}

.capability-item {
  display: grid;
  grid-template-columns: 38px minmax(0, 1fr) auto;
  align-items: center;
  gap: 13px;
  padding: 15px 0;
  border-bottom: 1px solid var(--border-color);
}

.capability-item:last-child {
  padding-bottom: 0;
  border-bottom: 0;
}

.capability-icon {
  width: 38px;
  height: 38px;
}

.capability-item strong {
  color: var(--text-primary);
  font-size: 12px;
}

.capability-item p {
  margin: 4px 0 0;
  color: var(--text-secondary);
  font-size: 10px;
  line-height: 1.5;
}

.status-badge {
  padding: 4px 8px;
  border: 1px solid var(--border-color);
  border-radius: 999px;
  color: var(--text-secondary);
  font-size: 9px;
  font-weight: 700;
}

.status-badge.success {
  border-color: var(--success-badge-border);
  background: var(--success-badge-bg);
  color: var(--success-badge-text);
}

.identity-profile {
  display: flex;
  align-items: center;
  gap: 13px;
  padding: 17px 0;
  border-top: 1px solid var(--border-color);
  border-bottom: 1px solid var(--border-color);
}

.large-avatar {
  width: 48px;
  height: 48px;
  font-size: 14px;
}

.identity-profile strong {
  color: var(--text-primary);
  font-size: 14px;
}

.identity-profile p {
  margin: 5px 0 0;
  color: var(--text-tertiary);
  font-size: 10px;
}

.identity-details {
  margin: 8px 0 0;
}

.identity-details div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0;
  font-size: 10px;
}

.identity-details dt {
  color: var(--text-tertiary);
}

.identity-details dd {
  max-width: 65%;
  margin: 0;
  overflow: hidden;
  color: var(--text-primary);
  font-weight: 700;
  text-overflow: ellipsis;
  text-transform: capitalize;
  white-space: nowrap;
}

.identity-details dd.active-text {
  color: var(--success-color);
}

.coming-panel {
  display: flex;
  align-items: center;
  gap: 17px;
  margin-top: 12px;
  padding: 20px 22px;
  border: 1px dashed var(--border-primary);
  border-radius: 9px;
  background: color-mix(in srgb, var(--bg-primary) 72%, transparent);
}

.coming-icon {
  width: 43px;
  height: 43px;
}

.coming-panel span {
  color: var(--text-tertiary);
  font-size: 9px;
  font-weight: 800;
  letter-spacing: 0.15em;
  text-transform: uppercase;
}

.coming-panel h2 {
  margin: 5px 0 4px;
  color: var(--text-primary);
  font-size: 13px;
}

.coming-panel p {
  margin: 0;
  color: var(--text-secondary);
  font-size: 10px;
  line-height: 1.5;
}

.sidebar-backdrop {
  display: none;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}

@media (max-width: 1100px) {
  .metric-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 900px) {
  .sidebar {
    position: fixed;
    left: 0;
    transform: translateX(-100%);
    transition: transform 220ms var(--ease-out-strong);
  }

  .sidebar.open {
    transform: translateX(0);
  }

  .sidebar-close,
  .menu-button {
    display: grid;
  }

  .sidebar-backdrop {
    position: fixed;
    inset: 0;
    z-index: 29;
    display: block;
    background: rgb(21 20 18 / 55%);
    backdrop-filter: blur(2px);
  }

  .content-grid {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 620px) {
  .breadcrumb,
  .account-copy,
  .account-button > svg {
    display: none;
  }

  .account-button {
    padding: 0 3px;
  }

  .dashboard-content {
    padding-top: 26px;
  }

  .page-heading {
    align-items: flex-start;
  }

  .page-heading p {
    max-width: 270px;
    line-height: 1.6;
  }

  .refresh-button {
    width: 37px;
    flex: 0 0 auto;
    padding: 0;
    justify-content: center;
    font-size: 0;
  }

  .status-hero {
    min-height: 260px;
    align-items: flex-start;
    flex-direction: column;
    padding: 25px;
  }

  .hero-score {
    align-items: flex-start;
  }

  .metric-grid {
    grid-template-columns: 1fr;
  }

  .capability-item {
    grid-template-columns: 38px minmax(0, 1fr);
  }

  .status-badge {
    grid-column: 2;
    justify-self: start;
  }

  .coming-panel {
    align-items: flex-start;
  }
}
</style>
