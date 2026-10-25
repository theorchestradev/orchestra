import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { parse, compileScript } from '@vue/compiler-sfc'
import { JSDOM } from 'jsdom'

test('PR shows grouped checks and activity, and copies failure context', async () => {
  const dom = new JSDOM('<div id="root"></div>')
  for (const key of ['window', 'document', 'Element', 'HTMLElement', 'SVGElement']) globalThis[key] = dom.window[key]
  const calls = [],
    copied = [],
    opened = []
  let reply = {
    number: 9,
    url: 'https://github.com/o/r/pull/9',
    title: 'Session work',
    state: 'open',
    branch: 'feature',
    repository: 'o/r',
    checks: [
      { name: 'build', workflow: 'CI', status: 'success', duration: '16m 18s', url: 'https://example.invalid/build' },
      { name: 'tests', workflow: 'CI', status: 'failure', duration: '2m', jobID: 41 },
      { name: 'deploy', status: 'failure', url: 'https://example.invalid/deploy' },
    ],
    reviews: [{ id: 1, state: 'APPROVED', body: '', submitted_at: '2026-10-21T10:00:00Z', user: { login: 'a' } }],
    comments: [],
  }
  globalThis.pullRequestCall = async (method, ...args) => {
    calls.push([method, ...args])
    if (method === 'PullRequestCheckFailure') return 'Check: tests\nLog:\n```\nFAIL\n```'
    if (reply instanceof Error) throw reply
    return structuredClone(reply)
  }
  globalThis.pullRequestCopy = async (text) => copied.push(text)
  globalThis.pullRequestOpen = (url) => opened.push(url)
  const { descriptor } = parse(
    readFileSync(new URL('../src/components/SessionPullRequest.vue', import.meta.url), 'utf8'),
  )
  const script = compileScript(descriptor, { id: 'pull-request-test', inlineTemplate: true })
  const dir = mkdtempSync(new URL('../node_modules/.pull-request-test-', import.meta.url))
  writeFileSync(
    dir + '/pullRequest.mjs',
    script.content
      .replace(
        /import \{ call \} from ['"]\.\.\/shared\/api['"]/,
        'const call = (...a) => globalThis.pullRequestCall(...a)',
      )
      .replace(
        /import \{ copy, openURL \} from ['"]\.\.\/shared\/api['"]/,
        'const copy = (t) => globalThis.pullRequestCopy(t), openURL = (u) => globalThis.pullRequestOpen(u)',
      )
      .replace(/import \{ relativeDate \} from ['"]\.\.\/shared\/appearance['"]/, 'const relativeDate = (d) => d')
      .replace(/import \{ sanitize \} from ['"]\.\.\/shared\/markdown['"]/, 'const sanitize = (h) => h')
      .replace("'../shared/pullRequest'", JSON.stringify(new URL('../src/shared/pullRequest.js', import.meta.url).href))
      .replace(/import Avatar from ['"]\.\/Avatar.vue['"]/, 'const Avatar = { render: () => null }')
      .replace(/import Icon from ['"]\.\/Icon.vue['"]/, 'const Icon = { render: () => null }'),
  )
  let app
  try {
    const { default: PullRequest, pullRequests } = await import(pathToFileURL(dir + '/pullRequest.mjs').href)
    const { createApp, h, ref } = await import('vue')
    const session = ref({ id: 'one' })
    const flush = () => new Promise((resolve) => setImmediate(resolve))
    app = createApp({ render: () => h(PullRequest, { session: session.value }) })
    app.mount('#root')
    await flush()
    assert.deepEqual(calls[0], ['SessionPullRequest', 'one', true])
    const text = () => document.querySelector('.session-pull-request').textContent
    assert.match(text(), /Session work\s*#9/)
    assert.match(text(), /Some checks were not successful/)
    assert.match(text(), /2 failing, 1 successful checks/)
    assert.match(text(), /1 approved/)
    const groups = () => [...document.querySelectorAll('.pr-check-list .pr-group-toggle')].map((b) => b.textContent)
    assert.deepEqual(groups(), ['2 failing checks', '1 successful check'])
    const names = () => [...document.querySelectorAll('.pr-check-name')].map((b) => b.textContent.trim())
    assert.deepEqual(names(), ['testsCI', 'deploy', 'buildCI'])
    document.querySelectorAll('.pr-check-name')[2].click()
    assert.deepEqual(opened, ['https://example.invalid/build'])

    document.querySelector('.pr-check-list .pr-group-toggle').click()
    await flush()
    assert.deepEqual(names(), ['buildCI'])
    document.querySelector('.pr-check-list .pr-group-toggle').click()
    await flush()

    document.querySelector('.pr-copy').click()
    await flush()
    assert.deepEqual(calls.at(-1), ['PullRequestCheckFailure', 'one', 41])
    assert.equal(copied.length, 1)
    assert.match(copied[0], /^Pull request #9: Session work\nhttps:\/\/github.com\/o\/r\/pull\/9\nBranch: feature/)
    assert.match(copied[0], /Check: tests\nLog:/)
    document.querySelector('.pr-copy-all').click()
    await flush()
    assert.match(copied[1], /Check: tests[\s\S]*---\n\nCheck: deploy\nStatus: failure/)
    assert.match(document.querySelector('.pr-copy-all').textContent, /Copied/)

    reply = { checks: [], branch: 'other', message: 'No pull request for other yet.' }
    session.value = { id: 'two' }
    app._instance.update()
    await flush()
    assert.match(text(), /No pull request for other yet\./)
    assert.equal(pullRequests.get('two').message, 'No pull request for other yet.')

    reply = new Error('gh: HTTP 500')
    session.value = { id: 'three' }
    app._instance.update()
    await flush()
    assert.match(document.querySelector('[role="alert"]').textContent, /gh: HTTP 500/)
  } finally {
    app?.unmount()
    rmSync(dir, { recursive: true, force: true })
  }
})
