import service from './service.js'

function request(options) {
  const { url, method, params, data, headers, responseType, ...config } = options
  return service.request({
    ...config,
    url,
    method,
    params,
    data,
    headers,
    responseType,
  })
}

export default {
  get(options) {
    return request({ method: 'get', ...options })
  },
  post(options) {
    return request({ method: 'post', ...options })
  },
  put(options) {
    return request({ method: 'put', ...options })
  },
  delete(options) {
    return request({ method: 'delete', ...options })
  },
}
