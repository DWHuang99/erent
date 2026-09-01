import { AxiosHeaders } from 'axios'

import { getAccessToken } from '../services/session.js'

export const REQUEST_TIMEOUT = 15_000

export function defaultRequestInterceptor(config) {
  if (config.skipAuth) return config

  const accessToken = getAccessToken()
  if (accessToken) {
    config.headers = AxiosHeaders.from(config.headers)
    config.headers.set('Authorization', `Bearer ${accessToken}`)
  }
  return config
}

export function defaultResponseInterceptor(response) {
  if (response.config.responseType === 'blob') return response
  return response.data
}
