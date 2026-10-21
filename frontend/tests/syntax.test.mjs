import test from 'node:test'
import assert from 'node:assert/strict'
import createDOMPurify from 'dompurify'
import { JSDOM } from 'jsdom'
import { fileLanguage, renderCode } from '../src/shared/syntax.js'
import { syntaxTokens, terminalTheme, setCustomThemes } from '../src/shared/appearance.js'

const window = new JSDOM('').window
const purifier = createDOMPurify(window)
test('file extensions select the grammar and unknown files stay plain', () => {
  for (const [path, language] of Object.entries({
    'App.vue': 'xml',
    'test.TSX': 'typescript',
    'main.go': 'go',
    'test.py': 'python',
    'config.json': 'json',
    'styles.css': 'css',
    'deploy.yml': 'yaml',
    'README.md': 'markdown',
    Makefile: 'makefile',
    '.env.local': 'ini',
    'notes.txt': 'plaintext',
    'file.unknown': 'plaintext',
  }))
    assert.equal(fileLanguage(path), language)
})
test('highlighting preserves source text and cannot create executable file HTML', () => {
  for (const [path, source] of Object.entries({
    'main.go': 'package main\nfunc main() { println("hello") }\n',
    'App.vue': '<template><img src="x" onerror="alert(1)"></template>\n<script>alert(2)</script>',
    'test.ts': 'const count: number = 42',
  })) {
    const html = renderCode(path, source, purifier)
    assert.match(html, /class="hljs-/)
    const node = window.document.createElement('div')
    node.innerHTML = html
    assert.equal(node.textContent, source)
    assert.equal(node.querySelector('script, img, [onerror]'), null)
  }
  assert.equal(renderCode('unknown.txt', '<script>&', purifier), '&lt;script&gt;&amp;')
})
test('syntax tokens follow the shared terminal palette in light, dark and custom themes', () => {
  setCustomThemes([
    {
      id: 'custom:syntax',
      light: ['#FFFFFF', '#FFFFFF', '#FFFFFF', '#000000', '#000000', '#000000', '#000000', '#FFFFFF'],
      dark: ['#000000', '#000000', '#000000', '#FFFFFF', '#FFFFFF', '#FFFFFF', '#FFFFFF', '#000000'],
      ansi: Array(16).fill('#ABCDEF'),
    },
  ])
  try {
    for (const preferences of [
      { theme: 'system', appearance: 'light' },
      { theme: 'dracula', appearance: 'dark' },
      { theme: 'custom:syntax', appearance: 'dark' },
    ]) {
      const terminal = terminalTheme(preferences)
      assert.deepEqual(syntaxTokens(preferences), {
        '--syntax-keyword': terminal.magenta,
        '--syntax-string': terminal.green,
        '--syntax-number': terminal.yellow,
        '--syntax-title': terminal.blue,
        '--syntax-builtin': terminal.cyan,
      })
    }
  } finally {
    setCustomThemes([])
  }
})
