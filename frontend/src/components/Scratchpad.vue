<script setup>
import { computed, nextTick, ref, watch, onBeforeUnmount } from 'vue'
import { openURL } from '../shared/api'
import { renderMarkdown } from '../shared/markdown'
import { mac, shortcutLabel } from '../shared/platform'
import { reconcileScratchpad } from '../shared/scratchpad.js'
import Icon from './Icon.vue'
// draft holds edits that were rejected after a previous editor closed.
const props = defineProps({ project: Object, draft: Object })
const emit = defineEmits(['save', 'mode'])
const text = ref(props.draft?.text ?? (props.project.scratchpadText || '')),
  saving = ref(false),
  conflict = ref(false),
  mode = ref(props.project.scratchpadMode === 'read' ? 'read' : 'edit'),
  editor = ref(null),
  reader = ref(null)
// Read mode shows the editor's text, so CLI writes appear as soon as the editor takes them.
const rendered = computed(() => (mode.value === 'read' ? renderMarkdown(text.value) : ''))
// base is the stored text and revision the editor last matched; saves are based on it.
let base = props.draft?.base ?? { text: text.value, revision: props.project.scratchpadRevision || 0 },
  submitted = [],
  timer
function save() {
  clearTimeout(timer)
  saving.value = false
  if (text.value === base.text) return
  submitted = [...submitted.slice(-9), text.value]
  emit('save', props.project.id, text.value, base)
}
function schedule() {
  clearTimeout(timer)
  saving.value = !conflict.value && text.value !== base.text
  if (!saving.value) return
  timer = setTimeout(save, 300)
}
defineExpose({
  projectID: props.project.id,
  takeDraft: () => {
    clearTimeout(timer)
    saving.value = false
    return { id: props.project.id, text: text.value, base }
  },
})
watch(text, schedule)
// Writes from orchestra scratchpad arrive with a new revision. Unsaved edits are kept.
watch(
  () => [props.project.scratchpadText || '', props.project.scratchpadRevision || 0],
  ([incomingText, revision]) => {
    const result = reconcileScratchpad(text.value, base, { text: incomingText, revision }, submitted)
    if (!result) return
    base = result.base
    const echo = submitted.indexOf(base.text)
    submitted = echo >= 0 ? submitted.slice(echo + 1) : []
    conflict.value ||= result.conflict
    if (result.text !== text.value) text.value = result.text
    else if (text.value !== base.text) schedule()
  },
  { immediate: Boolean(props.draft) },
)
if (props.draft) schedule()
watch(
  () => props.project.scratchpadMode,
  (value) => (mode.value = value === 'read' ? 'read' : 'edit'),
)
function setMode(value) {
  if (value === mode.value) return
  if (value === 'read' && saving.value) save()
  mode.value = value
  emit('mode', props.project.id, value)
}
async function toggleMode() {
  setMode(mode.value === 'read' ? 'edit' : 'read')
  await nextTick()
  ;(mode.value === 'read' ? reader : editor).value?.focus()
}
function keys(event) {
  if ((mac ? event.metaKey : event.ctrlKey) && !event.shiftKey && !event.altKey && event.code === 'KeyE') {
    event.preventDefault()
    toggleMode()
  }
}
// Links open in the external browser and never navigate the webview.
function linkClick(event) {
  const link = event.target.closest('a')
  if (!link) return
  event.preventDefault()
  openURL(link.href)
}
function keepMine() {
  conflict.value = false
  schedule()
}
function loadLatest() {
  conflict.value = false
  text.value = base.text
}
onBeforeUnmount(() => {
  clearTimeout(timer)
  save()
})
</script>
<template>
  <section class="scratchpad" @keydown="keys">
    <!-- Floats over the page's top-right corner, so the notes fill the view. -->
    <div class="scratchpad-tools">
      <span class="caption">{{ conflict ? 'Not saved' : saving ? 'Saving…' : 'Saved automatically' }}</span>
      <div class="scratchpad-mode" role="group" aria-label="Scratchpad mode">
        <button
          v-for="value in ['edit', 'read']"
          :key="value"
          class="icon-button"
          :aria-label="value === 'edit' ? 'Edit' : 'Read'"
          :title="(value === 'edit' ? 'Edit' : 'Read') + ' (' + shortcutLabel('⌘E') + ' toggles)'"
          :aria-pressed="mode === value"
          :class="{ selected: mode === value }"
          @click="setMode(value)"
        >
          <Icon :name="value === 'edit' ? 'square.and.pencil' : 'book'" :size="16" />
        </button>
      </div>
    </div>
    <div v-if="conflict" class="scratchpad-conflict" role="status">
      <span>This Scratchpad was changed outside the editor. Your unsaved edits are still here.</span>
      <button class="standard-button" @click="loadLatest">Load Latest</button>
      <button class="standard-button" @click="keepMine">Keep My Edits</button>
    </div>
    <div
      v-if="mode === 'read'"
      ref="reader"
      class="scratchpad-read"
      tabindex="0"
      aria-label="Project scratchpad, rendered"
      @click="linkClick"
      @auxclick.prevent
    >
      <div v-if="rendered" class="markdown" v-html="rendered" />
      <p v-else class="muted">Nothing here yet. Switch to Edit to write.</p>
    </div>
    <textarea
      v-else
      ref="editor"
      v-model="text"
      aria-label="Project scratchpad"
      placeholder="Write notes for this workspace…"
      spellcheck="false"
    />
  </section>
</template>
