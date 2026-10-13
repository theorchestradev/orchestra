import DOMPurify from 'dompurify'
import { Marked } from 'marked'

// Rendered HTML shares the webview with the Go bridge, and GitHub bodies and agent notes
// are untrusted, so every v-html goes through sanitize. Tests pass a DOM-backed purifier.
export function sanitize(html, purifier = DOMPurify) {
  return purifier.sanitize(html, {
    FORBID_TAGS: ['form', 'input', 'button', 'iframe', 'style', 'script'],
    FORBID_ATTR: ['style'],
  })
}

// Task-list checkboxes would be inputs, which sanitize removes, so they render as
// read-only marks instead.
const marked = new Marked({ gfm: true, async: false }).use({
  renderer: {
    checkbox: ({ checked }) =>
      `<span class="task-mark" role="checkbox" aria-checked="${checked}" aria-disabled="true"></span> `,
  },
})

export function renderMarkdown(text, purifier = DOMPurify) {
  return sanitize(marked.parse(text || ''), purifier)
}
