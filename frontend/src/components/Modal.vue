<script>
// Open modals, innermost last. Only the innermost one handles Escape, Tab and Command-Return, so a
// dialog opened from Settings closes alone. Command-Return clicks the dialog's main action: the
// enabled button marked data-default-action, or else its form's submit button. That button shows
// the shortcut so the action it triggers is visible.
const stack = []
</script>
<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { mac, shortcutLabel } from '../shared/platform.js'
const props = defineProps({ label: String, kind: { type: String, default: 'sheet' } })
const emit = defineEmits(['close'])
const element = ref(null)
let previous
const token = {}
let observer
// Buttons inside a nested dialog belong to that dialog.
function defaultAction() {
  return [
    ...element.value.querySelectorAll('[data-default-action]:not(:disabled), button[type="submit"]:not(:disabled)'),
  ].find((button) => button.closest('.modal') === element.value)
}
function markDefaultAction() {
  const action = defaultAction()
  for (const button of element.value.querySelectorAll('[data-shortcut]'))
    if (button !== action && button.closest('.modal') === element.value) {
      button.removeAttribute('data-shortcut')
      button.removeAttribute('aria-keyshortcuts')
    }
  if (action && !action.hasAttribute('data-shortcut')) {
    action.setAttribute('data-shortcut', shortcutLabel('⌘↩'))
    action.setAttribute('aria-keyshortcuts', mac ? 'Meta+Enter' : 'Control+Enter')
  }
}
function keys(event) {
  if (stack.at(-1) !== token) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    emit('close')
  }
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    const action = defaultAction()
    if (action) {
      event.preventDefault()
      event.stopPropagation()
      action.click()
    }
  }
  if (event.key === 'Tab') {
    const focusable = [
      ...element.value.querySelectorAll(
        'button:not(:disabled),input:not(:disabled),textarea:not(:disabled),select,[tabindex="0"]',
      ),
    ].filter((el) => el.offsetParent !== null)
    // WebKit can skip buttons with macOS's default Tab behavior. Move focus explicitly,
    // including from the dialog itself, so every control stays reachable inside the modal.
    event.preventDefault()
    const index = focusable.indexOf(document.activeElement)
    const next =
      index < 0
        ? event.shiftKey
          ? focusable.length - 1
          : 0
        : (index + (event.shiftKey ? -1 : 1) + focusable.length) % focusable.length
    focusable[next]?.focus()
  }
}
onMounted(() => {
  previous = document.activeElement
  stack.push(token)
  document.addEventListener('keydown', keys, true)
  markDefaultAction()
  observer = new MutationObserver(markDefaultAction)
  observer.observe(element.value, { subtree: true, childList: true, attributeFilter: ['disabled'] })
  ;(
    element.value.querySelector('[autofocus]') ||
    element.value.querySelector('input,textarea') ||
    element.value
  ).focus()
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', keys, true)
  observer.disconnect()
  stack.splice(stack.indexOf(token), 1)
  previous?.focus()
})
</script>
<template>
  <div class="modal-backdrop" :class="kind" @pointerdown.self="emit('close')">
    <section
      ref="element"
      class="modal"
      :class="kind"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      :aria-label="label"
    >
      <slot />
    </section>
  </div>
</template>
