import test from 'node:test'
import assert from 'node:assert/strict'
import createDOMPurify from 'dompurify'
import { JSDOM } from 'jsdom'
import { renderMarkdown, sanitize } from '../src/shared/markdown.js'

const purifier = createDOMPurify(new JSDOM('').window)

test('renders GitHub-flavored Markdown', () => {
  const html = renderMarkdown(
    '# Plan\n\n- [x] Done\n- [ ] Next\n\n| A | B |\n|---|---|\n| 1 | 2 |\n\n```go\nfmt.Println()\n```\n\n> Note `code` ~~old~~\n\n---\n\nhttps://example.com',
    purifier,
  )
  for (const part of [
    '<h1>Plan</h1>',
    '<table>',
    '<td>1</td>',
    '<pre><code class="language-go">',
    '<blockquote>',
    '<code>code</code>',
    '<del>old</del>',
    '<hr>',
    '<a href="https://example.com">',
    'role="checkbox" aria-checked="true" aria-disabled="true"',
    'aria-checked="false"',
  ])
    assert.ok(html.includes(part), part + ' missing from ' + html)
  assert.ok(!html.includes('<input'))
})

test('strips scripts, handlers, javascript links and frames', () => {
  const html = renderMarkdown(
    [
      '<script>alert(1)</script>',
      '<img src="x" onerror="alert(2)">',
      '[click](javascript:alert(3))',
      '<a href="javascript:alert(4)">raw</a>',
      '<iframe src="https://example.com"></iframe>',
      '<p style="color:red" onclick="alert(5)">styled</p>',
    ].join('\n\n'),
    purifier,
  )
  assert.doesNotMatch(html, /<script|onerror|onclick|javascript:|<iframe|style=/i)
  assert.match(html, /<img src="x">/)
  assert.match(html, /styled/)
})

test('sanitizes GitHub HTML with the same rules', () => {
  assert.equal(sanitize('<form><input></form><b>ok</b>', purifier), '<b>ok</b>')
})

test('empty text renders nothing', () => {
  assert.equal(renderMarkdown('', purifier), '')
})
