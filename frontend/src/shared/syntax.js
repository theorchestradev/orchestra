import hljs from 'highlight.js/lib/common'
import { sanitize } from './markdown.js'

const languages = {
  txt: 'plaintext',
  text: 'plaintext',
  vue: 'xml',
  html: 'xml',
  htm: 'xml',
  svg: 'xml',
  xml: 'xml',
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  jsx: 'javascript',
  ts: 'typescript',
  mts: 'typescript',
  cts: 'typescript',
  tsx: 'typescript',
  py: 'python',
  rb: 'ruby',
  rs: 'rust',
  sh: 'bash',
  zsh: 'bash',
  bash: 'bash',
  h: 'c',
  cc: 'cpp',
  hpp: 'cpp',
  cs: 'csharp',
  m: 'objectivec',
  mm: 'objectivec',
  yml: 'yaml',
  md: 'markdown',
  mdx: 'markdown',
  toml: 'ini',
  env: 'ini',
  kt: 'kotlin',
  kts: 'kotlin',
  gql: 'graphql',
  patch: 'diff',
}
export function fileLanguage(path) {
  const name = path.split('/').at(-1).toLowerCase()
  if (/^(makefile|gnumakefile)$/.test(name)) return 'makefile'
  if (/^\.env(?:\.|$)/.test(name)) return 'ini'
  const extension = name.split('.').at(-1)
  const language = languages[extension] || extension
  return hljs.getLanguage(language) ? language : 'plaintext'
}
export function renderCode(path, text, purifier) {
  return sanitize(hljs.highlight(text, { language: fileLanguage(path), ignoreIllegals: true }).value, purifier)
}
