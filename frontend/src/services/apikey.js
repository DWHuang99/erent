import service from '../axios/service.js'

async function send(method, path = '', data) {
  let body
  try {
    body = await service.request({ method, url: `/api/v1/api-keys${path}`, data })
  } catch (error) {
    const messages = {
      400: '请求无效，请检查授权账号、外部密钥及有效期后重试。',
      401: '登录状态已失效，请重新登录。',
      403: '你没有操作此 API 密钥的权限。',
      404: 'API 密钥不存在或接口不可用，请刷新列表。',
    }
    throw new Error(messages[error?.response?.status] ?? (method === 'get'
      ? 'API 密钥列表加载失败，请稍后重试。'
      : '操作未获确认，请刷新列表检查结果后再重试。'))
  }
  if (body?.code !== 0) throw new Error('服务端未确认操作成功，请刷新列表检查结果。')
  return body.data
}

export async function getApiKeys() {
  const data = await send('get')
  if (!Array.isArray(data?.apikeylist)) throw new Error('服务端未返回有效的 API 密钥列表。')
  return data.apikeylist
}

export async function createApiKey(ids, externalIds = []) {
  const data = await send('post', '', { oauth_info: ids, external_api_key_ids: externalIds })
  if (typeof data?.api_key !== 'string' || !data.api_key.startsWith('sk-') || data.api_key.length <= 12) {
    throw new Error('服务端未返回完整密钥，请刷新列表检查创建结果。')
  }
  return data.api_key
}

export function updateApiKey(id, data) { return send('patch', `/${id}`, data) }
export function replaceApiKeyAccounts(id, ids, externalIds) { return send('put', `/${id}/accounts`, { oauth_info: ids, external_api_key_ids: externalIds }) }
export function deleteApiKey(id) { return send('delete', `/${id}`) }
