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

export function saveSession(accessToken, username, remember) {
  clearSession()
  const storage = remember ? globalThis.localStorage : globalThis.sessionStorage
  storage?.setItem(TOKEN_KEY, accessToken)

  if (remember) {
    globalThis.localStorage?.setItem(USERNAME_KEY, username)
  } else {
    globalThis.localStorage?.removeItem(USERNAME_KEY)
  }
}

export function replaceAccessToken(accessToken) {
  const persistent = Boolean(globalThis.localStorage?.getItem(TOKEN_KEY))
  const storage = persistent ? globalThis.localStorage : globalThis.sessionStorage
  storage?.setItem(TOKEN_KEY, accessToken)
}

export function clearSession() {
  for (const storage of storageCandidates()) {
    storage.removeItem(TOKEN_KEY)
  }
}
