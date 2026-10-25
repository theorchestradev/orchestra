<script setup>
import { computed, ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import Icon from './Icon.vue'
import { spaceSymbol } from '../shared/project'
import { sidebarDragLayout } from '../shared/sidebarDrag'
const props = defineProps({
  projects: Array,
  projectIcons: { type: Object, default: () => ({}) },
  selectedId: String,
  settingsOpen: Boolean,
  reorder: Function,
})
const emit = defineEmits(['select', 'options', 'settings', 'create'])
const spaces = ref(null),
  width = ref(0)
const compact = computed(
  () => props.projects.length > 8 || (width.value > 0 && props.projects.length * 36 - 6 > width.value),
)
function revealSelected() {
  spaces.value?.querySelector('.selected')?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
}
const observer = new ResizeObserver(([entry]) => {
  width.value = entry.contentRect.width
})
onMounted(() => observer.observe(spaces.value))
onBeforeUnmount(() => {
  observer.disconnect()
  cancel()
})
watch(
  [() => props.selectedId, () => props.projects.map((p) => p.id).join(','), compact, width],
  async () => {
    await nextTick()
    revealSelected()
  },
  { immediate: true },
)
function focusSpace(event) {
  const keys = ['ArrowLeft', 'ArrowRight', 'Home', 'End']
  if (!keys.includes(event.key)) return
  event.preventDefault()
  const buttons = [...event.currentTarget.querySelectorAll('.arc-space')],
    index = buttons.indexOf(document.activeElement)
  const next =
    event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? buttons.length - 1
        : (index + (event.key === 'ArrowRight' ? 1 : -1) + buttons.length) % buttons.length
  buttons[next]?.focus()
}
// Spaces sort like sidebar rows, with the horizontal extents standing in for row tops and bottoms.
const drag = ref(null)
let pending,
  ignoreClick = false,
  frame = 0
function begin(event, project) {
  if (!pending) ignoreClick = false
  if (event.button !== 0 || pending || !props.reorder) return
  const nav = spaces.value
  pending = {
    id: project.id,
    pointer: event.pointerId,
    element: event.currentTarget,
    startX: event.clientX,
    startY: event.clientY,
    x: event.clientX,
    scrollLeft: nav.scrollLeft,
    from: props.projects.findIndex((p) => p.id === project.id),
    spans: [...nav.children].map((el) => {
      const rect = el.getBoundingClientRect()
      return { top: rect.left, bottom: rect.right }
    }),
  }
  event.currentTarget.setPointerCapture(event.pointerId)
  addEventListener('keydown', cancelKey)
  addEventListener('blur', cancel)
}
const layout = (d) => sidebarDragLayout(d.spans, d.from, d.x - d.startX + spaces.value.scrollLeft - d.scrollLeft)
function update() {
  const d = pending
  if (!d || !drag.value) return
  const nav = spaces.value,
    viewport = nav.getBoundingClientRect()
  nav.scrollLeft += d.x < viewport.left + 20 ? -6 : d.x > viewport.right - 20 ? 6 : 0
  drag.value = { id: d.id, ...layout(d) }
  frame = requestAnimationFrame(update)
}
function move(event) {
  const d = pending
  if (!d || event.pointerId !== d.pointer || d.saving) return
  d.x = event.clientX
  if (!drag.value && Math.hypot(event.clientX - d.startX, event.clientY - d.startY) < 5) return
  event.preventDefault()
  if (!drag.value) {
    ignoreClick = true
    drag.value = { id: d.id }
    update()
  }
}
async function finish(event) {
  const d = pending
  if (!d || event.pointerId !== d.pointer || d.saving) return
  if (!drag.value) return cancel()
  d.x = event.clientX
  cancelAnimationFrame(frame)
  const next = layout(d)
  drag.value = { id: d.id, ...next, y: next.settleY }
  d.saving = true
  try {
    if (next.index !== d.from) await props.reorder(d.id, next.index)
  } finally {
    cancel()
  }
}
function cancel() {
  const d = pending
  if (!d) return
  pending = null
  drag.value = null
  cancelAnimationFrame(frame)
  removeEventListener('keydown', cancelKey)
  removeEventListener('blur', cancel)
  if (d.element.hasPointerCapture(d.pointer)) d.element.releasePointerCapture(d.pointer)
}
function cancelKey(event) {
  if (event.key === 'Escape' && !pending?.saving) {
    event.preventDefault()
    cancel()
  }
}
function lostCapture() {
  if (!pending?.saving) cancel()
}
function click(event) {
  if (ignoreClick && event.detail) {
    event.preventDefault()
    event.stopPropagation()
  }
}
function spaceStyle(index) {
  const d = drag.value
  if (!d?.offsets) return
  const x = props.projects[index].id === d.id ? d.y : d.offsets[index]
  return { transform: `translateX(${x || 0}px)` }
}
</script>
<template>
  <footer class="arc-footer">
    <button
      class="icon-button arc-footer-action"
      aria-label="Settings"
      title="Settings"
      :aria-pressed="settingsOpen"
      @click="emit('settings')"
    >
      <Icon name="gearshape" :size="12" />
    </button>
    <nav
      ref="spaces"
      class="arc-spaces"
      :class="{ compact, 'arc-spaces-sorting': drag }"
      :style="{ '--visible-spaces': Math.min(projects.length, 8) || 1 }"
      aria-label="Project spaces"
      @keydown="focusSpace"
      @click.capture="click"
    >
      <button
        v-for="(p, index) in projects"
        :key="p.id"
        class="arc-space"
        :class="{ selected: p.id === selectedId, emoji: !compact && p.emoji, 'arc-space-dragging': drag?.id === p.id }"
        :style="spaceStyle(index)"
        :aria-label="'Switch to ' + p.name"
        :aria-pressed="p.id === selectedId"
        :title="p.description ? p.name + '\n' + p.description : p.name"
        @click="emit('select', p)"
        @contextmenu.prevent="emit('options', { event: $event, project: p })"
        @keydown.shift.f10.prevent="emit('options', { event: $event, project: p })"
        @pointerdown="begin($event, p)"
        @pointermove="move"
        @pointerup="finish"
        @pointercancel="cancel"
        @lostpointercapture="lostCapture"
      >
        <span v-if="compact" class="arc-space-dot" aria-hidden="true"></span>
        <img v-else-if="!p.emoji && projectIcons[p.id]" class="project-icon" :src="projectIcons[p.id]" alt="" />
        <span v-else aria-hidden="true">{{ spaceSymbol(p) }}</span>
      </button>
    </nav>
    <button class="icon-button arc-footer-action" aria-label="New Project" title="New Project" @click="emit('create')">
      <Icon name="plus" :size="12" />
    </button>
  </footer>
</template>
