const TOKEN_KEY = 'ai_gateway_access_token'
const USERNAME_KEY = 'ai_gateway_username'

function storageCandidates() {
  return [globalThis.localStorage, globalThis.sessionStorage].filter(Boolean)
}

export function getAccessToken() {
  for (const storage of storageCandidates()) {
    const token = storage.getItem(TOKEN_KEY)
    if (token) return token
  }
  return ''
}

export function hasAccessToken() {
  return Boolean(getAccessToken())
}

export function getRememberedUsername() {
  return globalThis.localStorage?.getItem(USERNAME_KEY) ?? ''
}

function saveSession(accessToken, username, remember) {
  clearSession()
  const storage = remember ? globalThis.localStorage : globalThis.sessionStorage
  storage?.setItem(TOKEN_KEY, accessToken)

  if (remember) {
    globalThis.localStorage?.setItem(USERNAME_KEY, username)
  } else {
    globalThis.localStorage?.removeItem(USERNAME_KEY)
  }
}

export function clearSession() {
  for (const storage of storageCandidates()) {
    storage.removeItem(TOKEN_KEY)
  }
}

async function parseResponse(response) {
  const body = await response.json().catch(() => null)
  if (response.ok && body?.code === 0) return body.data

  const messageByStatus = {
    400: '请输入有效的用户名和密码',
    401: '用户名或密码不正确',
    403: '该账号已被停用',
  }
  throw new Error(messageByStatus[response.status] ?? body?.message ?? '服务暂时不可用，请稍后重试')
}

export async function login({ username, password, remember }) {
  const response = await fetch('/api/v1/auth/login', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  const data = await parseResponse(response)

  if (!data?.accessToken) throw new Error('登录响应中缺少访问凭证')
  saveSession(data.accessToken, username, remember)
  return data
}

export async function register({ username, password, checkPassword, code, iAgree }) {
  const response = await fetch('/api/v1/auth/register', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      username,
      password,
      check_password: checkPassword,
      code,
      iAgree,
    }),
  })
  const body = await response.json().catch(() => null)
  if (response.ok && body?.code === 0) return body.data

  const messageByStatus = {
    400: '请检查用户名、密码、注册码和使用协议',
    409: '该用户名已被注册，请更换后重试',
  }
  throw new Error(messageByStatus[response.status] ?? body?.message ?? '注册服务暂时不可用，请稍后重试')
}

async function refreshAccessToken() {
  const response = await fetch('/api/v1/auth/refresh', {
    method: 'POST',
    credentials: 'include',
  })
  const data = await parseResponse(response)
  if (!data?.accessToken) throw new Error('刷新响应中缺少访问凭证')

  const persistent = Boolean(globalThis.localStorage?.getItem(TOKEN_KEY))
  const storage = persistent ? globalThis.localStorage : globalThis.sessionStorage
  storage?.setItem(TOKEN_KEY, data.accessToken)
  return data.accessToken
}

export async function authenticatedFetch(path, options = {}, retry = true) {
  const token = getAccessToken()
  const headers = new Headers(options.headers)
  if (token) headers.set('Authorization', `Bearer ${token}`)

  let response = await fetch(path, { ...options, headers, credentials: 'include' })
  if (response.status === 401 && retry) {
    try {
      const refreshedToken = await refreshAccessToken()
      headers.set('Authorization', `Bearer ${refreshedToken}`)
      response = await fetch(path, { ...options, headers, credentials: 'include' })
    } catch {
      clearSession()
    }
  }
  return response
}

export async function getCurrentUser() {
  const response = await authenticatedFetch('/api/v1/users/me')
  return parseResponse(response)
}

export async function logout() {
  try {
    await fetch('/api/v1/auth/logout', { method: 'POST', credentials: 'include' })
  } finally {
    clearSession()
  }
}
