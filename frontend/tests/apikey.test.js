import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'
import { axiosInstance } from '../src/axios/service.js'
import { getApiKeys, createApiKey, updateApiKey, replaceApiKeyAccounts, deleteApiKey } from '../src/services/apikey.js'

const adapter = axiosInstance.defaults.adapter
afterEach(() => { axiosInstance.defaults.adapter = adapter })
function respond(config, data) { return { config, data, headers: {}, status: 200, statusText: 'OK' } }

test('API key operations follow backend routes and request contracts', async () => {
  const cases = [
    ['get', '', undefined, { apikeylist: [] }, () => getApiKeys(), []],
    ['post', '', { oauth_info: [10, 11], external_api_key_ids: [] }, { api_key: 'sk-example-secret-key' }, () => createApiKey([10, 11]), 'sk-example-secret-key'],
    ['post', '', { oauth_info: [], external_api_key_ids: [3] }, { api_key: 'sk-external-secret-key' }, () => createApiKey([], [3]), 'sk-external-secret-key'],
    ['post', '', { oauth_info: [10], external_api_key_ids: [3] }, { api_key: 'sk-mixed-secret-key' }, () => createApiKey([10], [3]), 'sk-mixed-secret-key'],
    ['patch', '/7', { disabled: false, expires_at: null }, null, () => updateApiKey(7, { disabled: false, expires_at: null }), null],
    ['put', '/7/accounts', { oauth_info: [11], external_api_key_ids: [3] }, null, () => replaceApiKeyAccounts(7, [11], [3]), null],
    ['delete', '/7', undefined, null, () => deleteApiKey(7), null],
  ]
  for (const [method, path, data, result, run, expected] of cases) {
    axiosInstance.defaults.adapter = async (config) => {
      assert.equal(config.method, method)
      assert.equal(config.url, `/api/v1/api-keys${path}`)
      assert.equal(config.skipAuth, undefined)
      assert.deepEqual(config.data ? JSON.parse(config.data) : undefined, data)
      return respond(config, { code: 0, data: result })
    }
    assert.deepEqual(await run(), expected)
  }
})

test('malformed results and rejected mutations never appear successful', async () => {
  for (const data of [null, {}, { apikeylist: null }]) {
    axiosInstance.defaults.adapter = async (config) => respond(config, { code: 0, data })
    await assert.rejects(getApiKeys(), /有效的 API 密钥列表/)
  }
  axiosInstance.defaults.adapter = async (config) => respond(config, { code: 0, data: { api_key: 'sk-prefix' } })
  await assert.rejects(createApiKey([1]), /完整密钥/)
  axiosInstance.defaults.adapter = async (config) => respond(config, { code: 50000 })
  await assert.rejects(deleteApiKey(7), /未确认操作成功/)
  for (const [status, message] of [[400, /请求无效/], [401, /重新登录/], [404, /不存在或接口不可用/], [500, /检查结果/]]) {
    let calls = 0
    axiosInstance.defaults.adapter = async () => { calls++; throw { response: { status } } }
    await assert.rejects(createApiKey([1]), message)
    assert.equal(calls, 1)
  }
})
