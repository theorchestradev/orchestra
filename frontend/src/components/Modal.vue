<script>
// Open modals, innermost last. Only the innermost one handles Escape and Tab, so a dialog opened
// from Settings closes alone.
const stack = []
</script>
<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
const props = defineProps({ label: String, kind: { type: String, default: 'sheet' } })
const emit = defineEmits(['close'])
const element = ref(null)
let previous
const token = {}
function keys(event) {
  if (stack.at(-1) !== token) return
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    emit('close')
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
  ;(
    element.value.querySelector('[autofocus]') ||
    element.value.querySelector('input,textarea') ||
    element.value
  ).focus()
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', keys, true)
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
