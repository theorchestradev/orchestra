<script setup>
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { emojiGroupIndex, emojiGroups, searchEmoji } from '../shared/emoji'
import Icon from './Icon.vue'
import { track } from '../shared/telemetry'
const props = defineProps({ value: String, id: String })
const emit = defineEmits(['pick'])
const columns = 8
const trigger = ref(null),
  popup = ref(null),
  search = ref(null),
  grid = ref(null),
  open = ref(false),
  position = ref({}),
  query = ref(''),
  group = ref(0),
  active = ref(0)
const results = computed(() => (query.value.trim() ? searchEmoji(query.value) : emojiGroups[group.value].emojis))
function place() {
  if (!popup.value || !trigger.value) return
  const r = trigger.value.getBoundingClientRect(),
    height = popup.value.offsetHeight,
    width = popup.value.offsetWidth
  position.value = {
    left: Math.max(8, Math.min(r.left, innerWidth - width - 8)) + 'px',
    top: Math.max(8, r.bottom + 6 + height > innerHeight ? r.top - height - 6 : r.bottom + 6) + 'px',
  }
}
function outside(event) {
  if (!popup.value?.contains(event.target) && !trigger.value?.contains(event.target)) close()
}
// The window capture phase runs before Modal's document listener, so Escape closes only the picker.
function escape(event) {
  if (event.key !== 'Escape') return
  event.preventDefault()
  event.stopPropagation()
  close(true)
}
function listen(on) {
  const method = on ? 'addEventListener' : 'removeEventListener'
  document[method]('pointerdown', outside)
  window[method]('keydown', escape, true)
  window[method]('scroll', place, true)
  window[method]('resize', place)
}
async function toggle() {
  if (open.value) return close(true)
  query.value = ''
  group.value = emojiGroupIndex(props.value)
  active.value = Math.max(
    0,
    results.value.findIndex((e) => e.emoji === props.value),
  )
  open.value = true
  track('feature_used', { feature: 'emoji_picker' })
  listen(true)
  await nextTick()
  place()
  search.value?.focus()
  grid.value?.children[active.value]?.scrollIntoView({ block: 'center' })
}
function close(restoreFocus = false) {
  if (!open.value) return
  open.value = false
  listen(false)
  if (restoreFocus) trigger.value?.focus()
}
function pick(emoji) {
  emit('pick', emoji)
  close(true)
}
function reset() {
  active.value = 0
  if (grid.value) grid.value.scrollTop = 0
}
function choose(index) {
  query.value = ''
  group.value = index
  reset()
}
function moveGroup(event, step) {
  choose((group.value + step + emojiGroups.length) % emojiGroups.length)
  nextTick(() => event.target.parentElement.children[group.value]?.focus())
}
function focusActive() {
  const button = grid.value?.children[active.value]
  button?.focus()
  button?.scrollIntoView({ block: 'nearest' })
}
function searchKeys(event) {
  if (event.key === 'ArrowDown' && results.value.length) {
    event.preventDefault()
    focusActive()
  } else if (event.key === 'Enter') {
    // Enter picks the first match instead of submitting Edit Workspace.
    event.preventDefault()
    if (query.value.trim() && results.value.length) pick(results.value[0].emoji)
  }
}
function gridKeys(event) {
  const last = results.value.length - 1,
    step = { ArrowRight: 1, ArrowLeft: -1, ArrowDown: columns, ArrowUp: -columns }[event.key]
  let next
  if (step) next = active.value + step
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = last
  else return
  event.preventDefault()
  if (next < 0) return search.value?.focus()
  active.value = Math.min(Math.max(0, next), last)
  focusActive()
}
onBeforeUnmount(() => listen(false))
</script>
<template>
  <button
    ref="trigger"
    :id="id"
    type="button"
    class="standard-button emoji-picker-trigger"
    :aria-label="value ? `Change Emoji, ${value}` : 'Choose Emoji'"
    :title="value ? 'Change Emoji' : 'Choose Emoji'"
    aria-haspopup="dialog"
    :aria-expanded="open"
    @click="toggle"
  >
    <span v-if="value" class="emoji-value">{{ value }}</span
    ><Icon v-else name="face.smiling" :size="16" />
  </button>
  <section v-if="open" ref="popup" class="emoji-picker" :style="position" role="dialog" aria-label="Choose Emoji">
    <span class="search-field emoji-search"
      ><Icon name="magnifyingglass" :size="12" /><input
        autocorrect="off"
        autocapitalize="off"
        ref="search"
        v-model="query"
        placeholder="Search"
        aria-label="Search Emoji"
        autocomplete="off"
        spellcheck="false"
        @input="reset"
        @keydown="searchKeys"
    /></span>
    <div class="emoji-groups" role="tablist" aria-label="Emoji Categories">
      <button
        v-for="(g, i) in emojiGroups"
        :key="g.name"
        type="button"
        role="tab"
        :class="{ selected: !query.trim() && group === i }"
        :aria-selected="!query.trim() && group === i"
        aria-controls="emoji-grid"
        :aria-label="g.name"
        :title="g.name"
        :tabindex="group === i ? 0 : -1"
        @click="choose(i)"
        @keydown.right.prevent="moveGroup($event, 1)"
        @keydown.left.prevent="moveGroup($event, -1)"
      >
        {{ g.icon }}
      </button>
    </div>
    <div
      v-if="results.length"
      id="emoji-grid"
      ref="grid"
      class="emoji-grid"
      role="listbox"
      :aria-label="query.trim() ? 'Search Results' : emojiGroups[group].name"
      @keydown="gridKeys"
    >
      <button
        v-for="(e, i) in results"
        :key="e.emoji"
        type="button"
        role="option"
        :class="{ selected: e.emoji === value }"
        :aria-selected="e.emoji === value"
        :aria-label="e.name"
        :title="e.name"
        :tabindex="i === active ? 0 : -1"
        @focus="active = i"
        @click="pick(e.emoji)"
      >
        {{ e.emoji }}
      </button>
    </div>
    <p v-else class="muted caption emoji-empty" role="status">No Emoji Found</p>
  </section>
</template>
