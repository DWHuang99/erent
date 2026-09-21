import service from '../axios/service.js'

async function send(method, path = '', data) {
  let body
  try {
    body = await service.request({ method, url: `/api/v1/external-api-keys${path}`, data })
  } catch (error) {
    const messages = {
      400: '请求无效，请检查密钥、服务地址与路径后缀。',
      401: '登录状态已失效，请重新登录。',
      403: '你没有操作此外部密钥的权限。',
      404: '外部密钥不存在或接口不可用，请刷新列表。',
    }
    throw new Error(messages[error?.response?.status] ?? (method === 'get'
      ? '外部密钥列表加载失败，请稍后重试。'
      : '操作未获确认，请刷新列表检查结果后再重试。'))
  }
  if (body?.code !== 0) throw new Error('服务端未确认操作成功，请刷新列表检查结果。')
  return body.data
}

export async function getExternalApiKeys() {
  const data = await send('get')
  if (!Array.isArray(data?.apikeylist)) throw new Error('服务端未返回有效的外部密钥列表。')
  return data.apikeylist
}

export function createExternalApiKey(data) { return send('post', '', data) }
export function updateExternalApiKey(id, data) { return send('put', `/${id}`, data) }
export function deleteExternalApiKey(id) { return send('delete', `/${id}`) }
