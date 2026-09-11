import request from '../axios/index.js'

const messages = {
  400: '授权已失效、被拒绝或已使用，请重新开始登录。',
  401: '登录状态已失效，请重新登录控制台。',
  403: '当前账号无权进行授权。',
  404: 'Codex OAuth 尚未在服务端启用。',
  500: '授权信息保存失败，请稍后重新开始登录。',
  502: '上游授权响应无效，请重新开始登录。',
  503: 'OAuth 服务暂时不可用，请稍后重试。',
  504: '授权请求超时，请重新开始登录。',
}

function failure(error) {
  if (error?.response?.data?.message === 'invalid_provider') {
    return new Error('所选 OAuth 服务未配置或不受支持。')
  }
  return new Error(messages[error?.response?.status] ?? '无法连接授权服务，请检查网络后重试。')
}

export async function startCodexLogin() {
  let body
  try {
    body = await request.get({ url: '/oauth/login', params: { provider: 'oai' }, headers: { Accept: 'application/json' } })
  } catch (error) {
    throw failure(error)
  }
  let url
  try { url = new URL(body?.code === 0 ? body?.data?.url : undefined) } catch { throw new Error('服务端未返回有效的授权链接。') }
  if (url.protocol !== 'https:' || url.username || url.password || !url.searchParams.get('state')) {
    throw new Error('服务端返回的授权链接无效。')
  }
  return url.href
}

function deviceFailure(error) {
  if (error?.code === 'ERR_CANCELED') return error
  if (error?.response?.status === 408) return new Error('设备授权已取消，请重新开始授权。')
  if (!error?.response || error.response.status === 504) {
    return new Error('设备授权等待中断或超时，请先查看已授权账号；若未保存，请重新开始授权。')
  }
  return failure(error)
}

export async function startCodexDeviceLogin(signal) {
  let body
  try {
    body = await request.post({ url: '/oauth/logindevice', params: { provider: 'oai' }, signal })
  } catch (error) {
    throw deviceFailure(error)
  }
  const data = body?.data
  let url
  try { url = new URL(data?.verification_url) } catch { throw new Error('服务端未返回有效的设备授权链接。') }
  if (body?.code !== 0 || url.protocol !== 'https:' || url.username || url.password ||
      typeof data?.device_auth_id !== 'string' || !data.device_auth_id.trim() ||
      typeof data?.user_code !== 'string' || !data.user_code.trim()) {
    throw new Error('服务端返回的设备授权信息无效，请重新开始授权。')
  }
  return { ...data, verification_url: url.href }
}

export async function completeCodexDeviceLogin(deviceAuthId, signal) {
  let body
  try {
    body = await request.post({
      url: '/oauth/callbackdevice', params: { provider: 'oai' },
      data: { device_auth_id: deviceAuthId }, signal,
    })
  } catch (error) {
    throw deviceFailure(error)
  }
  if (body?.code !== 0 || body?.message !== 'oauth credentials saved') {
    throw new Error('服务端未确认凭证已保存，请先查看已授权账号；若未保存，请重新开始授权。')
  }
}

export function parseCallback(value, authorizationUrl) {
  let callback
  try { callback = new URL(value.trim()) } catch { throw new Error('请粘贴浏览器地址栏中的完整回调 URL。') }
  const authorization = new URL(authorizationUrl)
  const expected = new URL(authorization.searchParams.get('redirect_uri'))
  if (callback.origin !== expected.origin || callback.pathname !== expected.pathname) {
    throw new Error('回调地址与本次授权不匹配，请检查粘贴的 URL。')
  }
  if (callback.searchParams.get('state') !== authorization.searchParams.get('state')) {
    throw new Error('该回调不属于本次登录，请使用最新授权页面的回调 URL。')
  }
  if (callback.searchParams.has('error')) throw new Error('你已取消或拒绝授权，请重新开始登录。')
  const code = callback.searchParams.get('code')
  if (!code) throw new Error('回调 URL 中缺少授权码，请先完成授权。')
  return { code, state: callback.searchParams.get('state') }
}

export async function completeCodexLogin(value, authorizationUrl) {
  const params = parseCallback(value, authorizationUrl)
  let body
  try {
    body = await request.get({ url: '/oauth/callback', params, headers: { Accept: 'application/json' }, skipAuth: true, skipAuthRefresh: true })
  } catch (error) {
    throw failure(error)
  }
  if (body?.code !== 0 || body?.message !== 'oauth credentials saved') throw new Error('服务端未确认凭证已保存，请重新授权。')
}

export async function getOAuthList() {
  let body
  try {
    body = await request.get({ url: '/oauth/list' })
  } catch (error) {
    if (error?.response?.status === 401) throw failure(error)
    throw new Error('授权账号列表加载失败，请稍后重试。')
  }
  if (body?.code !== 0 || !Array.isArray(body?.data?.oauthlist)) {
    throw new Error('服务端未返回有效的授权账号列表。')
  }
  return body.data.oauthlist
}

export async function deleteOAuthAccount(id) {
  let body
  try {
    body = await request.post({ url: '/oauth/delete', data: { id } })
  } catch (error) {
    if (error?.response?.status === 401) throw failure(error)
    if (error?.response?.status === 404) {
      const missing = error.response.data?.message === 'oauth credential not found'
      throw new Error(missing ? '授权账号不存在，请刷新账号列表。' : '服务端删除接口不可用，请更新或重启后端服务。')
    }
    throw new Error('删除授权账号失败，请稍后重试。')
  }
  if (body?.code !== 0 || body?.message !== 'success') {
    throw new Error('服务端未确认删除成功，请刷新列表检查账号状态。')
  }
}

export async function refreshOAuthAccount(id) {
  let body
  try {
    body = await request.post({ url: '/oauth/refresh', data: { id } })
  } catch (error) {
    if (error?.response?.status === 404) {
      const body = error.response.data
      const accountMissing = body?.code === 404 && body?.message === 'oauth credential not found'
      throw new Error(accountMissing
        ? '授权账号不存在，请刷新账号列表。'
        : '服务端刷新凭证接口不可用，请更新或重启后端服务。')
    }
    const refreshMessages = {
      400: '账号凭证无法刷新，请重新授权。',
      401: '登录状态已失效，请重新登录控制台。',
      403: '当前账号无权刷新此授权。',
      408: '凭证刷新已取消，请重试。',
      500: '刷新后的凭证保存失败，请稍后重试。',
      502: '上游刷新响应无效，请稍后重试。',
      503: '授权服务暂时不可用，请稍后重试。',
      504: '凭证刷新超时，请稍后重试。',
    }
    throw new Error(refreshMessages[error?.response?.status] ?? '无法连接授权服务，请检查网络后重试。')
  }
  if (body?.code !== 0 || body?.data?.message !== 'refresh success') {
    throw new Error('服务端未确认凭证刷新成功，请刷新列表检查账号状态。')
  }
}
