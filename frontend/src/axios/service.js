import axios, { AxiosHeaders } from 'axios'

import { clearSession, getAccessToken, replaceAccessToken } from '../services/session.js'
import {
  REQUEST_TIMEOUT,
  defaultRequestInterceptor,
  defaultResponseInterceptor,
} from './config.js'

export const axiosInstance = axios.create({
  baseURL: '/',
  timeout: REQUEST_TIMEOUT,
  withCredentials: true,
  headers: { Accept: 'application/json' },
})

export const refreshClient = axios.create({
  baseURL: '/',
  timeout: REQUEST_TIMEOUT,
  withCredentials: true,
  headers: { Accept: 'application/json' },
})

let refreshRequest = null

function requestNewAccessToken() {
  if (!refreshRequest) {
    refreshRequest = refreshClient
      .post('/api/v1/auth/refresh')
      .then((response) => {
        const accessToken = response.data?.data?.accessToken
        if (response.data?.code !== 0 || !accessToken) {
          throw new Error('刷新响应中缺少访问凭证')
        }
        replaceAccessToken(accessToken)
        return accessToken
      })
      .finally(() => {
        refreshRequest = null
      })
  }
  return refreshRequest
}

axiosInstance.interceptors.request.use(defaultRequestInterceptor)
axiosInstance.interceptors.response.use(defaultResponseInterceptor)
axiosInstance.interceptors.response.use(undefined, async (error) => {
  const originalRequest = error?.config
  const canRefresh = Boolean(getAccessToken())

  if (
    error?.response?.status !== 401 ||
    !originalRequest ||
    originalRequest.skipAuthRefresh ||
    originalRequest._authRetried ||
    !canRefresh
  ) {
    return Promise.reject(error)
  }

  originalRequest._authRetried = true

  try {
    const accessToken = await requestNewAccessToken()
    originalRequest.headers = AxiosHeaders.from(originalRequest.headers)
    originalRequest.headers.set('Authorization', `Bearer ${accessToken}`)
    return axiosInstance.request(originalRequest)
  } catch (refreshError) {
    clearSession()
    return Promise.reject(refreshError)
  }
})

const service = {
  request(config) {
    return axiosInstance.request(config)
  },
}

export default service
