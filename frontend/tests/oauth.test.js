import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'
import { axiosInstance } from '../src/axios/service.js'
import { startCodexLogin, completeCodexLogin, parseCallback, getOAuthList, refreshOAuthAccount, deleteOAuthAccount } from '../src/services/oauth.js'
import { startCodexDeviceLogin, completeCodexDeviceLogin } from '../src/services/oauth.js'

const adapter = axiosInstance.defaults.adapter
const authorization = 'https://auth.example.com/authorize?state=state-1&redirect_uri=http%3A%2F%2Flocalhost%3A1455%2Fauth%2Fcallback'
const callback = 'http://localhost:1455/auth/callback?code=one-time-code&state=state-1'

test('device login validates instructions and completion sends only the ID with authentication', async () => {
  const info = { device_auth_id: 'device-1', user_code: 'ABCD-EFGH', verification_url: 'https://auth.openai.com/codex/device', interval: 5 }
  const controller = new AbortController()
  axiosInstance.defaults.adapter = async (config) => {
    assert.equal(config.method, 'post')
    assert.equal(config.skipAuth, undefined)
    assert.equal(config.headers.get('Accept'), 'application/json')
    assert.deepEqual(config.params, { provider: 'oai' })
    assert.equal(config.signal, controller.signal)
    if (config.url === '/oauth/logindevice') return respond(config, { code: 0, data: info })
    assert.equal(config.url, '/oauth/callbackdevice')
    assert.deepEqual(JSON.parse(config.data), { device_auth_id: 'device-1' })
    assert.equal(config.timeout, 960000)
    return respond(config, { code: 0, message: 'oauth credentials saved' })
  }
  assert.deepEqual(await startCodexDeviceLogin(controller.signal), info)
  await completeCodexDeviceLogin(info.device_auth_id, controller.signal)
})

test('device instructions reject malformed data and unsafe links', async () => {
  const info = { device_auth_id: 'id', user_code: 'code', verification_url: 'https://auth.openai.com/codex/device' }
  for (const data of [null, {}, { ...info, device_auth_id: '' }, { ...info, user_code: 1 },
    { ...info, verification_url: 'javascript:alert(1)' }, { ...info, verification_url: 'https://user:pass@example.com' }]) {
    axiosInstance.defaults.adapter = async (config) => respond(config, { code: 0, data })
    await assert.rejects(startCodexDeviceLogin(), /设备授权/)
  }
})

test('device failures are actionable and completion never retries network or upstream failures', async () => {
  for (const [status, message] of [[400, /失效、被拒绝/], [401, /重新登录/], [408, /取消/], [504, /先查看已授权账号/], [503, /暂时不可用/], [undefined, /先查看已授权账号/]]) {
    let calls = 0
    axiosInstance.defaults.adapter = async () => { calls++; throw { response: status ? { status } : undefined } }
    await assert.rejects(completeCodexDeviceLogin('id'), message)
    assert.equal(calls, 1)
  }
  axiosInstance.defaults.adapter = async (config) => respond(config, { code: 0, message: 'unexpected' })
  await assert.rejects(completeCodexDeviceLogin('id'), /未确认凭证已保存/)
  const controller = new AbortController()
  controller.abort()
  await assert.rejects(completeCodexDeviceLogin('id', controller.signal), { code: 'ERR_CANCELED' })
})

test('delete posts only the selected ID with authentication and validates success', async () => {
  axiosInstance.defaults.adapter = async (config) => {
    assert.equal(config.method, 'post')
    assert.equal(config.url, '/oauth/delete')
    assert.deepEqual(JSON.parse(config.data), { id: 42 })
    assert.equal(config.skipAuth, undefined)
    return respond(config, { code: 0, data: null, message: 'success' })
  }
  await deleteOAuthAccount(42)
  for (const body of [{ code: 500, message: 'success' }, { code: 0 }, {}]) {
    axiosInstance.defaults.adapter = async (config) => respond(config, body)
    await assert.rejects(deleteOAuthAccount(42), /未确认删除成功/)
  }
})

test('delete reports missing accounts, unavailable routes and database failures', async () => {
  for (const [status, data, message] of [
    [404, { code: 404, message: 'oauth credential not found' }, /账号不存在/],
    [404, '404 page not found', /接口不可用/],
    [500, undefined, /删除授权账号失败/],
    [401, undefined, /重新登录/],
  ]) {
    axiosInstance.defaults.adapter = async () => { throw { response: { status, data } } }
    await assert.rejects(deleteOAuthAccount(42), message)
  }
})

test('refresh posts the selected credential ID with console authentication', async () => {
  axiosInstance.defaults.adapter = async (config) => {
    assert.equal(config.method, 'post')
    assert.equal(config.url, '/oauth/refresh')
    assert.deepEqual(JSON.parse(config.data), { id: 42 })
    assert.equal(config.skipAuth, undefined)
    return respond(config, { code: 0, data: { message: 'refresh success' } })
  }
  await refreshOAuthAccount(42)
})

test('refresh rejects unconfirmed responses and reports refresh-specific failures', async () => {
  for (const body of [{ code: 0 }, { code: 500, data: { message: 'refresh success' } }, { code: 0, data: { code: 0, data: null, message: 'unexpected' } }]) {
    axiosInstance.defaults.adapter = async (config) => respond(config, body)
    await assert.rejects(refreshOAuthAccount(42), /未确认凭证刷新成功/)
  }
  for (const [status, message] of [[400, /重新授权/], [403, /无权刷新/], [404, /接口不可用/], [500, /保存失败/], [503, /暂时不可用/], [504, /超时/]]) {
    axiosInstance.defaults.adapter = async () => { throw { response: { status } } }
    await assert.rejects(refreshOAuthAccount(42), message)
  }
})

test('refresh distinguishes missing credentials from an unavailable route', async () => {
  for (const data of ['404 page not found', undefined, { code: 404, message: 'Not Found' }]) {
    axiosInstance.defaults.adapter = async () => { throw { response: { status: 404, data } } }
    await assert.rejects(refreshOAuthAccount(42), /接口不可用/)
  }
  axiosInstance.defaults.adapter = async () => {
    throw { response: { status: 404, data: { code: 404, data: null, message: 'oauth credential not found' } } }
  }
  await assert.rejects(refreshOAuthAccount(42), /授权账号不存在/)
})
afterEach(() => { axiosInstance.defaults.adapter = adapter })
function respond(config, data) { return { config, data, headers: {}, status: 200, statusText: 'OK' } }

test('login requests a JSON authorization URL', async () => {
  axiosInstance.defaults.adapter = async (config) => {
    assert.equal(config.url, '/oauth/login')
    assert.deepEqual(config.params, { provider: 'oai' })
    assert.equal(config.headers.get('Accept'), 'application/json')
    return respond(config, { code: 0, data: { url: authorization }, message: 'success' })
  }
  assert.equal(await startCodexLogin(), authorization)
})

test('rejects unsafe and malformed authorization responses', async () => {
  for (const url of ['javascript:alert(1)', 'http://example.com/?state=1', 'https://example.com/', undefined]) {
    axiosInstance.defaults.adapter = async (config) => respond(config, { code: 0, data: { url }, message: 'success' })
    await assert.rejects(startCodexLogin(), /授权链接/)
  }
})

test('callback must match the current flow and configured redirect', () => {
  assert.deepEqual(parseCallback(callback, authorization), { code: 'one-time-code', state: 'state-1' })
  assert.throws(() => parseCallback(callback.replace('state-1', 'old-state'), authorization), /不属于本次登录/)
  assert.throws(() => parseCallback(callback.replace('localhost', 'evil.example'), authorization), /地址与本次授权不匹配/)
  assert.throws(() => parseCallback('not a URL', authorization), /完整回调/)
  assert.throws(() => parseCallback(callback.replace('code=one-time-code', 'error=access_denied'), authorization), /取消或拒绝/)
})

test('submits only code and state to the local callback and verifies persistence', async () => {
  axiosInstance.defaults.adapter = async (config) => {
    assert.equal(config.url, '/oauth/callback')
    assert.deepEqual(config.params, { code: 'one-time-code', state: 'state-1' })
    assert.equal(config.skipAuthRefresh, true)
    return respond(config, { code: 0, data: null, message: 'oauth credentials saved' })
  }
  await completeCodexLogin(callback, authorization)
  axiosInstance.defaults.adapter = async (config) => respond(config, { code: 0, data: null, message: 'unexpected' })
  await assert.rejects(completeCodexLogin(callback, authorization), /未确认凭证已保存/)
})

test('disabled provider endpoint reports an actionable error', async () => {
  axiosInstance.defaults.adapter = async () => { throw { response: { status: 404 } } }
  await assert.rejects(startCodexLogin(), /尚未在服务端启用/)
  axiosInstance.defaults.adapter = async () => { throw { response: { status: 400, data: { code: 400, data: null, message: 'invalid_provider' } } } }
  await assert.rejects(startCodexLogin(), /未配置或不受支持/)
})

test('loads account metadata and preserves empty lists', async () => {
  for (const items of [[], [{ id: 1, type: 'codex', accountId: 'account' }]]) {
    axiosInstance.defaults.adapter = async (config) => {
      assert.equal(config.url, '/oauth/list')
      assert.equal(config.skipAuth, undefined)
      return respond(config, { code: 0, data: { oauthlist: items } })
    }
    assert.deepEqual(await getOAuthList(), items)
  }
})

test('list failures and malformed responses do not become empty successes', async () => {
  axiosInstance.defaults.adapter = async () => { throw { response: { status: 500 } } }
  await assert.rejects(getOAuthList(), /列表加载失败/)
  for (const body of [{ code: 0, data: { oauthlist: null } }, { code: 500, data: { oauthlist: [] } }]) {
    axiosInstance.defaults.adapter = async (config) => respond(config, body)
    await assert.rejects(getOAuthList(), /有效的授权账号列表/)
  }
})

test('only device completion receives a long request timeout', async () => {
  for (const [url, timeout] of [['/oauth/callbackdevice?provider=oai', 960000], ['/oauth/list', 15000]]) {
    axiosInstance.defaults.adapter = async (config) => {
      assert.equal(config.timeout, timeout)
      return respond(config, { code: 0, data: null })
    }
    await axiosInstance.post(url, {})
  }
})
