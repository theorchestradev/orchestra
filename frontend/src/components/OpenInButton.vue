<script setup>
import { computed, ref, watch, nextTick, onBeforeUnmount } from 'vue'
import { menuKeys } from '../shared/menu'
import { fileManager, lastApp, appSymbol, appGroups } from '../shared/openIn'
import Icon from './Icon.vue'
const props = defineProps({ apps: Array, last: String, visible: Boolean })
const emit = defineEmits(['open', 'copy'])
const trigger = ref(null),
  popup = ref(null),
  open = ref(false),
  position = ref({})
const current = computed(() => lastApp(props.apps, props.last))
const groups = computed(() => appGroups(props.apps))
const files = computed(() => props.apps.find((a) => a.id === fileManager))
const label = computed(() => (current.value ? 'Open in ' + current.value.name : 'Open in…'))
function place() {
  if (!popup.value || !trigger.value) return
  const r = trigger.value.getBoundingClientRect(),
    height = popup.value.offsetHeight,
    width = popup.value.offsetWidth
  position.value = {
    left: Math.max(8, Math.min(r.right - width, innerWidth - width - 8)) + 'px',
    top: Math.max(8, Math.min(r.bottom + 4, innerHeight - height - 8)) + 'px',
  }
}
function outside(event) {
  if (!popup.value?.contains(event.target) && !trigger.value?.parentElement.contains(event.target)) close()
}
function listen(on) {
  const method = on ? 'addEventListener' : 'removeEventListener'
  document[method]('pointerdown', outside)
  window[method]('resize', place)
}
async function show() {
  if (open.value) return
  open.value = true
  listen(true)
  await nextTick()
  place()
  popup.value?.querySelector('button')?.focus()
}
function close(restoreFocus = false) {
  if (!open.value) return
  open.value = false
  listen(false)
  if (restoreFocus) trigger.value?.focus()
}
// The main part opens the last app, or the menu before any choice; with the menu open it closes it.
function primary() {
  const wasOpen = open.value
  close()
  if (current.value) emit('open', current.value.id)
  else if (!wasOpen) show()
}
function choose(event, ...args) {
  close(true)
  emit(event, ...args)
}
function keys(event) {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    close(true)
  } else if (event.key === 'Tab') close()
  else menuKeys(event)
}
watch(
  () => props.visible,
  (visible) => {
    if (!visible) close()
  },
)
onBeforeUnmount(() => listen(false))
</script>
<template>
  <div class="open-in" :class="{ open }">
    <div class="split-button">
      <button
        class="standard-button open-in-main"
        :title="label"
        :aria-haspopup="current ? undefined : 'menu'"
        @click="primary"
      >
        <img v-if="current?.icon" :src="current.icon" alt="" /><Icon
          v-else-if="current"
          :name="appSymbol(current)"
          :size="14"
        /><span class="truncate">{{ label }}</span></button
      ><button
        ref="trigger"
        class="standard-button split-toggle"
        aria-label="Choose an App to Open the Session Folder"
        title="Choose an App"
        aria-haspopup="menu"
        :aria-expanded="open"
        @click="open ? close() : show()"
        @keydown.down.prevent="show"
      >
        <Icon name="chevron.down" :size="10" />
      </button>
    </div>
    <Teleport to=".app-shell"
      ><div
        v-if="open"
        ref="popup"
        class="context-menu open-in-menu"
        role="menu"
        aria-label="Open in"
        :style="position"
        @keydown="keys"
        @pointerdown.stop
      >
        <template v-for="group in groups" :key="group.kind"
          ><div role="group" :aria-label="group.label">
            <button v-for="app in group.apps" :key="app.id" role="menuitem" @click="choose('open', app.id)">
              <img v-if="app.icon" :src="app.icon" alt="" /><Icon v-else :name="appSymbol(app)" :size="14" /><span
                class="truncate"
                >{{ app.name }}</span
              ><Icon v-if="app.id === current?.id" class="open-in-check" name="checkmark" :size="12" />
            </button>
          </div>
          <hr
        /></template>
        <button v-if="files" role="menuitem" @click="choose('open', files.id)">
          <img v-if="files.icon" :src="files.icon" alt="" /><Icon v-else name="folder" :size="14" /><span
            class="truncate"
            >{{ files.name }}</span
          ><Icon v-if="files.id === current?.id" class="open-in-check" name="checkmark" :size="12" /></button
        ><button role="menuitem" @click="choose('copy')"><Icon name="doc.on.doc" :size="14" />Copy Path</button>
      </div></Teleport
    >
  </div>
</template>
