import assert from 'node:assert/strict'
import { afterEach, test } from 'node:test'
import { axiosInstance } from '../src/axios/service.js'
import { getExternalApiKeys, createExternalApiKey, updateExternalApiKey, deleteExternalApiKey } from '../src/services/external-apikey.js'

const adapter = axiosInstance.defaults.adapter
afterEach(() => { axiosInstance.defaults.adapter = adapter })
const payload = { external_apikey: 'provider-secret', endpoint: 'https://example.com', suffix: { chat_completions: '', responses: '/responses', messages: '' } }

test('external key CRUD uses authenticated backend contracts including empty suffixes', async () => {
  for (const [method, path, body, result, run] of [
    ['get', '', undefined, { apikeylist: [{ id: 3, endpoint: payload.endpoint }] }, getExternalApiKeys],
    ['post', '', payload, { id: 3 }, () => createExternalApiKey(payload)],
    ['put', '/3', payload, null, () => updateExternalApiKey(3, payload)],
    ['delete', '/3', undefined, null, () => deleteExternalApiKey(3)],
  ]) {
    axiosInstance.defaults.adapter = async (config) => {
      assert.equal(config.method, method)
      assert.equal(config.url, `/api/v1/external-api-keys${path}`)
      assert.equal(config.skipAuth, undefined)
      assert.deepEqual(config.data ? JSON.parse(config.data) : undefined, body)
      return { config, data: { code: 0, data: result }, headers: {}, status: 200, statusText: 'OK' }
    }
    assert.deepEqual(await run(), method === 'get' ? result.apikeylist : result)
  }
})

test('external key failures do not expose request credentials or report success', async () => {
  axiosInstance.defaults.adapter = async (config) => ({ config, data: { code: 0, data: {} }, headers: {}, status: 200 })
  await assert.rejects(getExternalApiKeys(), /有效的外部密钥列表/)
  axiosInstance.defaults.adapter = async () => { throw new Error(payload.external_apikey) }
  await assert.rejects(createExternalApiKey(payload), /操作未获确认/)
  axiosInstance.defaults.adapter = async (config) => ({ config, data: { code: 40000 }, headers: {}, status: 200 })
  await assert.rejects(deleteExternalApiKey(3), /未确认操作成功/)
})
