import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import { ref } from 'vue'

const source = readFileSync(new URL('../src/views/ApiKeysView.vue', import.meta.url), 'utf8')
  .split('<script setup>')[1].split('</script>')[0].replace(/^import .*$/gm, '')
function setup(overrides = {}) {
  let unmount
  const view = runInNewContext(`${source}\n;({ save, selectedExternalIds, closeDialog, secret, mode, selectedIds, selectedKey, disabled, expiry, formError, formNotice, error, dialog, copySecret })`, {
    ref, nextTick: async () => {}, onMounted() {}, onUnmounted(fn) { unmount = fn },
    getApiKeys: async () => [], createApiKey: async () => 'sk-created-secret',
    updateApiKey: async () => {}, replaceApiKeyAccounts: async () => {}, deleteApiKey: async () => {},
    ...overrides,
  })
  view.dialog.value = { close() {} }
  return { ...view, unmount: () => unmount() }
}

test('editing submits both scopes in one request including external-only scope', async () => {
  const calls = []
  const view = setup({ replaceApiKeyAccounts: async (...args) => calls.push(args) })
  view.selectedKey.value = { id: 7 }
  view.selectedIds.value = [10]
  view.selectedExternalIds.value = [3]
  await view.save('accounts')
  assert.equal(JSON.stringify(calls), '[[7,[10],[3]]]')
  view.selectedIds.value = []
  await view.save('accounts')
  assert.equal(JSON.stringify(calls[1]), '[7,[],[3]]')
})

test('creation requires scope and keeps one-time secret even if list refresh fails', async () => {
  const view = setup({ getApiKeys: async () => { throw new Error('list failed') } })
  await view.save('create')
  assert.match(view.formError.value, /至少选择/)
  view.selectedIds.value = [10]
  await view.save('create')
  assert.equal(view.secret.value, 'sk-created-secret')
  assert.equal(view.mode.value, 'secret')
  assert.equal(view.error.value, 'list failed')
  view.closeDialog()
  assert.equal(view.secret.value, '')
})

test('settings preserve untouched timestamp and scope saving is independent', async () => {
  const calls = []
  const view = setup({
    updateApiKey: async (id, data) => calls.push([id, data]),
    replaceApiKeyAccounts: async () => { throw new Error('scope failed') },
  })
  view.selectedKey.value = { id: 7, expires_at: null }
  view.disabled.value = true
  await view.save('settings')
  assert.equal(calls.length, 1)
  assert.equal(calls[0][1].expires_at, null)
  assert.equal(calls[0][1].disabled, true)
  view.selectedIds.value = [10]
  await view.save('accounts')
  assert.equal(view.formError.value, 'scope failed')
  assert.equal(view.formNotice.value, '')
  assert.equal(calls.length, 1)
})

test('clipboard errors preserve secret for manual copy and unmount clears it', async () => {
  const view = setup({ navigator: {} })
  view.secret.value = 'sk-secret'
  await view.copySecret()
  assert.match(view.formError.value, /手动选择/)
  assert.equal(view.secret.value, 'sk-secret')
  view.unmount()
  assert.equal(view.secret.value, '')
})

 test('creation accepts external-only scope and submits both selections', async () => {
  const calls = []
  const view = setup({ createApiKey: async (...args) => { calls.push(args); return 'sk-external-secret' } })
  view.selectedExternalIds.value = [3]
  await view.save('create')
  assert.equal(calls.length, 1)
  assert.equal(calls[0][0].length, 0)
  assert.equal(calls[0][1][0], 3)
  assert.equal(view.secret.value, 'sk-external-secret')
})
