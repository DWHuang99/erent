import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import { ref } from 'vue'
import { setImmediate } from 'node:timers/promises'

// Execute the actual setup handlers with controlled HTTP promises and router lifecycle.
const source = readFileSync(new URL('../src/views/OAuthView.vue', import.meta.url), 'utf8')
  .split('<script setup>')[1].split('</script>')[0].replace(/^import .*$/gm, '')
function deferred() {
  let resolve, reject
  const promise = new Promise((yes, no) => { resolve = yes; reject = no })
  return { promise, resolve, reject }
}
const info = { device_auth_id: 'id', user_code: 'CODE', verification_url: 'https://auth.openai.com/codex/device' }
function setup(start, complete, environment = {}) {
  const routes = []
  let unmount
  const api = runInNewContext(`${source}\n;({ startDevice, cancelDevice, copyDeviceCode, copyMessage, device, deviceLoading, deviceWaiting, error, success })`, {
    ref, AbortController, setTimeout, clearTimeout,
    useRouter: () => ({ push: async (route) => { routes.push(route.name) } }),
    onUnmounted: (fn) => { unmount = fn },
    startCodexDeviceLogin: start, completeCodexDeviceLogin: complete,
    ...environment,
  })
  return { ...api, routes, unmount: () => unmount() }
}

test('device code uses Clipboard API when available', async () => {
  const copied = []
  const view = setup(null, null, {
    navigator: { clipboard: { writeText: async (text) => copied.push(text) } },
  })
  view.device.value = info
  await view.copyDeviceCode()
  assert.deepEqual(copied, ['CODE'])
  assert.equal(view.copyMessage.value, '授权码已复制')
})

for (const outcome of ['success', 'false', 'throw']) {
  test(`HTTP device code copy handles ${outcome} and cleans up`, async () => {
    const calls = []
    const input = {
      style: {},
      select() { calls.push('select') },
      setSelectionRange(start, end) { assert.deepEqual([start, end], [0, 4]) },
      remove() { calls.push('remove') },
    }
    const view = setup(null, null, {
      navigator: {},
      document: {
        activeElement: { focus() { calls.push('focus') } },
        createElement(tag) { assert.equal(tag, 'textarea'); return input },
        body: { appendChild(element) { assert.equal(element, input); calls.push('append') } },
        execCommand(command) {
          assert.equal(command, 'copy')
          assert.equal(input.value, 'CODE')
          calls.push('copy')
          if (outcome === 'throw') throw new Error('blocked')
          return outcome === 'success'
        },
      },
    })
    view.device.value = info
    await view.copyDeviceCode()
    assert.deepEqual(calls, ['append', 'select', 'copy', 'remove', 'focus'])
    assert.equal(view.copyMessage.value, outcome === 'success'
      ? '授权码已复制' : '复制失败，请手动选择并复制授权码。')
  })
}

test('clipboard rejection reports failure and late success does not update a cancelled device', async () => {
  const pending = deferred()
  const view = setup(null, null, {
    navigator: { clipboard: { writeText: () => pending.promise } },
  })
  view.device.value = info
  const copy = view.copyDeviceCode()
  view.cancelDevice(false)
  pending.resolve()
  await copy
  assert.equal(view.copyMessage.value, '')
  await view.copyDeviceCode()
  assert.equal(view.copyMessage.value, '')

  const rejected = setup(null, null, {
    navigator: { clipboard: { writeText: async () => { throw new Error('blocked') } } },
  })
  rejected.device.value = info
  await rejected.copyDeviceCode()
  assert.equal(rejected.copyMessage.value, '复制失败，请手动选择并复制授权码。')
})

test('device view prevents duplicate requests and navigates only after saved success', async () => {
  const pending = deferred()
  let starts = 0, completes = 0
  const view = setup(async () => { starts++; return info }, async () => { completes++; await pending.promise })
  const flow = view.startDevice()
  await view.startDevice()
  assert.equal(starts, 1)
  assert.equal(completes, 1)
  assert.equal(view.deviceWaiting.value, true)
  assert.deepEqual(view.routes, [])
  pending.resolve()
  await flow
  assert.equal(view.success.value, true)
  assert.deepEqual(view.routes, ['authorized-accounts'])
})

test('cancel aborts waiting and stale success cannot change a restarted flow', async () => {
  const old = deferred(), fresh = deferred()
  const signals = []
  const view = setup(async () => info, (id, signal) => {
    signals.push(signal)
    return signals.length === 1 ? old.promise : fresh.promise
  })
  const first = view.startDevice()
  await setImmediate()
  view.cancelDevice()
  assert.equal(signals[0].aborted, true)
  const second = view.startDevice()
  await setImmediate()
  old.resolve()
  await first
  assert.equal(view.deviceWaiting.value, true)
  assert.deepEqual(view.routes, [])
  fresh.reject(new Error('设备授权已失效'))
  await second
  assert.match(view.error.value, /已失效/)
  assert.equal(view.device.value, null)
  assert.equal(view.deviceWaiting.value, false)
})

test('leaving during start aborts acquisition and never starts completion', async () => {
  const pending = deferred()
  let signal, completes = 0
  const view = setup((value) => { signal = value; return pending.promise }, async () => { completes++ })
  const flow = view.startDevice()
  view.unmount()
  assert.equal(signal.aborted, true)
  pending.resolve(info)
  await flow
  assert.equal(completes, 0)
  assert.equal(view.device.value, null)
  assert.deepEqual(view.routes, [])
})

test('leaving while waiting aborts completion and ignores late errors', async () => {
  const pending = deferred()
  let signal
  const view = setup(async () => info, (id, value) => { signal = value; return pending.promise })
  const flow = view.startDevice()
  await setImmediate()
  view.unmount()
  assert.equal(signal.aborted, true)
  pending.reject(new Error('late failure'))
  await flow
  assert.equal(view.error.value, '')
  assert.deepEqual(view.routes, [])
})
