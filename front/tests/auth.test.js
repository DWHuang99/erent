import assert from 'node:assert/strict'
import { afterEach, beforeEach, test } from 'node:test'

import {
  clearSession,
  getAccessToken,
  getRememberedUsername,
  hasAccessToken,
  login,
  register,
} from '../src/services/auth.js'

function memoryStorage() {
  const values = new Map()
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
  }
}

beforeEach(() => {
  globalThis.localStorage = memoryStorage()
  globalThis.sessionStorage = memoryStorage()
})

afterEach(() => {
  delete globalThis.fetch
  delete globalThis.localStorage
  delete globalThis.sessionStorage
})

test('login stores a persistent token and remembered username when requested', async () => {
  globalThis.fetch = async () =>
    new Response(JSON.stringify({ code: 0, data: { accessToken: 'token-1' }, message: 'success' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })

  await login({ username: 'admin', password: 'admin12345', remember: true })

  assert.equal(getAccessToken(), 'token-1')
  assert.equal(getRememberedUsername(), 'admin')
  assert.equal(hasAccessToken(), true)
})

test('login reports the backend unauthorized response in user-facing Chinese', async () => {
  globalThis.fetch = async () =>
    new Response(JSON.stringify({ code: 40100, data: null, message: 'invalid username or password' }), {
      status: 401,
      headers: { 'Content-Type': 'application/json' },
    })

  await assert.rejects(
    login({ username: 'admin', password: 'incorrect', remember: false }),
    /用户名或密码不正确/,
  )
  assert.equal(hasAccessToken(), false)
})

test('clearSession removes access tokens from both browser storage scopes', () => {
  globalThis.localStorage.setItem('ai_gateway_access_token', 'persistent')
  globalThis.sessionStorage.setItem('ai_gateway_access_token', 'temporary')

  clearSession()

  assert.equal(getAccessToken(), '')
})

test('register sends the backend contract without creating a login session', async () => {
  let requestBody
  globalThis.fetch = async (_url, options) => {
    requestBody = JSON.parse(options.body)
    return new Response(JSON.stringify({ code: 0, data: null, message: 'register successful' }), {
      status: 201,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  await register({
    username: 'new-user',
    password: 'password123',
    checkPassword: 'password123',
    code: '123456',
    iAgree: true,
  })

  assert.deepEqual(requestBody, {
    username: 'new-user',
    password: 'password123',
    check_password: 'password123',
    code: '123456',
    iAgree: true,
  })
  assert.equal(hasAccessToken(), false)
})

test('register reports a duplicate username in user-facing Chinese', async () => {
  globalThis.fetch = async () =>
    new Response(JSON.stringify({ code: 10002, data: null, message: 'username already exists' }), {
      status: 409,
      headers: { 'Content-Type': 'application/json' },
    })

  await assert.rejects(
    register({
      username: 'new-user',
      password: 'password123',
      checkPassword: 'password123',
      code: '123456',
      iAgree: true,
    }),
    /该用户名已被注册/,
  )
})
