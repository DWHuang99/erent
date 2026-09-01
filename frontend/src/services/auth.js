import request from '../axios/index.js'
import {
  clearSession,
  getAccessToken,
  getRememberedUsername,
  hasAccessToken,
  saveSession,
} from './session.js'

export { clearSession, getAccessToken, getRememberedUsername, hasAccessToken }

function responseData(body, fallbackMessage) {
  if (body?.code === 0) return body.data
  throw new Error(body?.message ?? fallbackMessage)
}

function requestError(error, messageByStatus, fallbackMessage) {
  const status = error?.response?.status
  const backendMessage = error?.response?.data?.message
  return new Error(messageByStatus[status] ?? backendMessage ?? fallbackMessage)
}

export async function login({ username, password, remember }) {
  try {
    const response = await request.post({
      url: '/api/v1/auth/login',
      data: { username, password },
      skipAuth: true,
      skipAuthRefresh: true,
    })
    const data = responseData(response, '登录响应无效')
    if (!data?.accessToken) throw new Error('登录响应中缺少访问凭证')
    saveSession(data.accessToken, username, remember)
    return data
  } catch (error) {
    if (error instanceof Error && !error.isAxiosError) throw error
    throw requestError(
      error,
      {
        400: '请输入有效的用户名和密码',
        401: '用户名或密码不正确',
        403: '该账号已被停用',
      },
      '服务暂时不可用，请稍后重试',
    )
  }
}

export async function register({ username, password, checkPassword, code, iAgree }) {
  try {
    const response = await request.post({
      url: '/api/v1/auth/register',
      data: {
        username,
        password,
        check_password: checkPassword,
        code,
        iAgree,
      },
      skipAuth: true,
      skipAuthRefresh: true,
    })
    return responseData(response, '注册响应无效')
  } catch (error) {
    if (error instanceof Error && !error.isAxiosError) throw error
    throw requestError(
      error,
      {
        400: '请检查用户名、密码、注册码和使用协议',
        409: '该用户名已被注册，请更换后重试',
      },
      '注册服务暂时不可用，请稍后重试',
    )
  }
}

export async function getCurrentUser() {
  try {
    const response = await request.get({ url: '/api/v1/users/me' })
    return responseData(response, '用户响应无效')
  } catch (error) {
    if (error instanceof Error && !error.isAxiosError) throw error
    throw requestError(
      error,
      { 401: '登录状态已失效', 403: '该账号已被停用' },
      '登录状态已失效',
    )
  }
}

export async function getReadiness() {
  try {
    await request.get({
      url: '/health/ready',
      skipAuth: true,
      skipAuthRefresh: true,
    })
    return true
  } catch {
    return false
  }
}

export async function logout() {
  try {
    await request.post({
      url: '/api/v1/auth/logout',
      data: null,
      skipAuth: true,
      skipAuthRefresh: true,
    })
  } catch {
    // Local logout must still complete when the API is unavailable.
  } finally {
    clearSession()
  }
}
