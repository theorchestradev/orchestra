<script setup>
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { call, copy, on, paste } from '../shared/api'
import { mac } from '../shared/platform'
import { terminalTheme, customThemes } from '../shared/appearance'
const props = defineProps({ session: Object, preferences: Object, translucent: Boolean, visible: Boolean })
const emit = defineEmits(['error'])
const host = ref(null)
let terminal,
  fit,
  observer,
  unsubscribe,
  position = 0,
  attaching = true,
  pending = [],
  alive = true
function append(output) {
  if (output.id !== props.session.id) return
  if (attaching) {
    pending.push(output)
    return
  }
  if (output.end <= position) return
  const bytes = Uint8Array.from(atob(output.data), (c) => c.charCodeAt(0))
  const skip = Math.max(0, position - output.start)
  terminal.write(bytes.subarray(skip))
  position = output.end
}
function pad() {
  terminal.element.style.padding = `${props.preferences.terminalPaddingY}px ${props.preferences.terminalPaddingX}px`
}
async function resize() {
  if (!alive || !terminal || !props.visible || !host.value?.clientWidth || !host.value?.clientHeight) return
  fit.fit()
  if (props.session.running)
    try {
      await call('ResizeTerminal', props.session.id, terminal.cols, terminal.rows)
    } catch (e) {
      emit('error', String(e))
    }
}
async function attach() {
  attaching = true
  pending = []
  try {
    let snapshot
    if (!props.session.running && !props.session.hasLaunched) {
      snapshot = await call('StartSession', props.session.id, false, terminal.cols, terminal.rows)
    } else snapshot = await call('TerminalSnapshot', props.session.id)
    if (!alive) return
    attaching = false
    append(snapshot)
    for (const output of pending) append(output)
    pending = []
  } catch (e) {
    attaching = false
    emit('error', String(e))
  }
}
onMounted(async () => {
  terminal = new Terminal({
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, monospace',
    fontSize: props.preferences.terminalFontSize,
    theme: terminalTheme(props.preferences, props.translucent),
    allowTransparency: true,
    cursorBlink: true,
    scrollback: 10000,
    allowProposedApi: false,
  })
  fit = new FitAddon()
  terminal.loadAddon(fit)
  terminal.open(host.value)
  pad()
  // App shortcuts are left to App.vue. Off macOS, Ctrl+B, Ctrl+K and Ctrl+N belong to the
  // terminal, so app shortcuts and copy/paste use Ctrl+Shift there.
  terminal.attachCustomKeyEventHandler((e) => {
    if (mac)
      return !(
        e.metaKey &&
        (['n', 'k', ',', 'b'].includes(e.key.toLowerCase()) ||
          (e.shiftKey && e.code === 'KeyB') ||
          (e.shiftKey && ['BracketLeft', 'BracketRight'].includes(e.code)))
      )
    if (!e.ctrlKey) return true
    if (!e.shiftKey) return e.key !== ','
    if (e.code === 'KeyC' || e.code === 'KeyV') {
      e.preventDefault()
      if (e.type === 'keydown' && e.code === 'KeyC' && terminal.hasSelection()) copy(terminal.getSelection())
      if (e.type === 'keydown' && e.code === 'KeyV')
        paste()
          .then((text) => text && terminal.paste(text))
          .catch((err) => emit('error', String(err)))
      return false
    }
    return !['KeyN', 'KeyK', 'KeyB', 'BracketLeft', 'BracketRight'].includes(e.code)
  })
  terminal.onData((data) => call('WriteTerminal', props.session.id, data).catch((e) => emit('error', String(e))))
  unsubscribe = on('terminal', append)
  observer = new ResizeObserver(resize)
  observer.observe(host.value)
  await nextTick()
  await resize()
  await attach()
  if (props.visible) terminal.focus()
})
watch(
  () => props.visible,
  async (visible) => {
    if (visible) {
      await nextTick()
      await resize()
      terminal?.focus()
    }
  },
)
// Custom themes can change on disk while selected, so their reload also updates the terminal.
watch(
  [() => props.preferences, () => props.translucent, customThemes],
  async () => {
    if (!terminal) return
    terminal.options.theme = terminalTheme(props.preferences, props.translucent)
    terminal.options.fontSize = props.preferences.terminalFontSize
    pad()
    await resize()
  },
  { deep: true },
)
onBeforeUnmount(() => {
  alive = false
  observer?.disconnect()
  unsubscribe?.()
  terminal?.dispose()
})
</script>
<template>
  <div
    class="terminal-host"
    ref="host"
    :class="{ smoothed: preferences.terminalFontSmoothing }"
    :aria-label="session.name"
  />
</template>
