import assert from 'node:assert/strict'
import { afterEach, beforeEach, test } from 'node:test'

import {
  clearSession,
  getAccessToken,
  getCurrentUser,
  getRememberedUsername,
  hasAccessToken,
  login,
  register,
} from '../src/services/auth.js'
import { axiosInstance, refreshClient } from '../src/axios/service.js'

const defaultAdapter = axiosInstance.defaults.adapter
const defaultRefreshAdapter = refreshClient.defaults.adapter

function memoryStorage() {
  const values = new Map()
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
  }
}

function response(config, status, data) {
  return { config, data, headers: {}, request: {}, status, statusText: String(status) }
}

function responseError(config, status, data) {
  const error = new Error(`Request failed with status code ${status}`)
  error.config = config
  error.isAxiosError = true
  error.response = response(config, status, data)
  return Promise.reject(error)
}

beforeEach(() => {
  globalThis.localStorage = memoryStorage()
  globalThis.sessionStorage = memoryStorage()
})

afterEach(() => {
  axiosInstance.defaults.adapter = defaultAdapter
  refreshClient.defaults.adapter = defaultRefreshAdapter
  delete globalThis.localStorage
  delete globalThis.sessionStorage
})

test('login stores a persistent token and remembered username when requested', async () => {
  axiosInstance.defaults.adapter = async (config) =>
    response(config, 200, { code: 0, data: { accessToken: 'token-1' }, message: 'success' })

  await login({ username: 'admin', password: 'admin12345', remember: true })

  assert.equal(getAccessToken(), 'token-1')
  assert.equal(getRememberedUsername(), 'admin')
  assert.equal(hasAccessToken(), true)
})

test('login reports the backend unauthorized response in user-facing Chinese', async () => {
  axiosInstance.defaults.adapter = (config) =>
    responseError(config, 401, {
      code: 40100,
      data: null,
      message: 'invalid username or password',
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
  axiosInstance.defaults.adapter = async (config) => {
    requestBody = JSON.parse(config.data)
    return response(config, 201, { code: 0, data: null, message: 'register successful' })
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
  axiosInstance.defaults.adapter = (config) =>
    responseError(config, 409, {
      code: 10002,
      data: null,
      message: 'username already exists',
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

test('401 refreshes once, updates the token, and retries the original request', async () => {
  globalThis.sessionStorage.setItem('ai_gateway_access_token', 'token-old')
  let userRequestCount = 0
  let refreshRequestCount = 0

  refreshClient.defaults.adapter = async (config) => {
    refreshRequestCount += 1
    return response(config, 200, {
      code: 0,
      data: { accessToken: 'token-new' },
      message: 'success',
    })
  }
  axiosInstance.defaults.adapter = async (config) => {
    if (config.url === '/api/v1/users/me') {
      userRequestCount += 1
      if (userRequestCount === 1) {
        return responseError(config, 401, { code: 40100, data: null, message: 'invalid token' })
      }
      assert.equal(config.headers.get('Authorization'), 'Bearer token-new')
      return response(config, 200, {
        code: 0,
        data: { id: 1, username: 'admin' },
        message: 'success',
      })
    }
    throw new Error(`Unexpected request: ${config.url}`)
  }

  const currentUser = await getCurrentUser()

  assert.equal(currentUser.username, 'admin')
  assert.equal(getAccessToken(), 'token-new')
  assert.equal(refreshRequestCount, 1)
  assert.equal(userRequestCount, 2)
})

test('concurrent 401 responses share one refresh request', async () => {
  globalThis.sessionStorage.setItem('ai_gateway_access_token', 'token-old')
  let refreshRequestCount = 0
  let retriedRequestCount = 0

  refreshClient.defaults.adapter = async (config) => {
    refreshRequestCount += 1
    await new Promise((resolve) => setTimeout(resolve, 5))
    return response(config, 200, {
      code: 0,
      data: { accessToken: 'token-new' },
      message: 'success',
    })
  }
  axiosInstance.defaults.adapter = async (config) => {
    if (config.url === '/api/v1/users/me') {
      const authorization = config.headers.get('Authorization')
      if (authorization === 'Bearer token-old') {
        return responseError(config, 401, { code: 40100, data: null, message: 'invalid token' })
      }
      retriedRequestCount += 1
      return response(config, 200, {
        code: 0,
        data: { id: retriedRequestCount, username: 'admin' },
        message: 'success',
      })
    }
    throw new Error(`Unexpected request: ${config.url}`)
  }

  const users = await Promise.all([getCurrentUser(), getCurrentUser()])

  assert.equal(users.length, 2)
  assert.equal(refreshRequestCount, 1)
  assert.equal(retriedRequestCount, 2)
})
