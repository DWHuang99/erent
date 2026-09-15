import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import { ref } from 'vue'

const source = readFileSync(new URL('../src/views/ApiKeysView.vue', import.meta.url), 'utf8')
  .split('<script setup>')[1].split('</script>')[0].replace(/^import .*$/gm, '')
function setup(overrides = {}) {
  let unmount
  const view = runInNewContext(`${source}\n;({ save, closeDialog, secret, mode, selectedIds, selectedKey, disabled, expiry, formError, formNotice, error, dialog, copySecret })`, {
    ref, nextTick: async () => {}, onMounted() {}, onUnmounted(fn) { unmount = fn },
    getApiKeys: async () => [], createApiKey: async () => 'sk-created-secret',
    updateApiKey: async () => {}, replaceApiKeyAccounts: async () => {}, deleteApiKey: async () => {},
    ...overrides,
  })
  view.dialog.value = { close() {} }
  return { ...view, unmount: () => unmount() }
}

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
