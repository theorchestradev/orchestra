<script setup>
import { ref, onBeforeUnmount } from 'vue'
import { sidebarDragLayout } from '../shared/sidebarDrag'

const props = defineProps({ items: Array, reorder: Function, handle: String, kind: String })
const list = ref(null),
  drag = ref(null)
let pending,
  ignoreClick = false,
  frame = 0
function begin(event, item) {
  if (!pending) ignoreClick = false
  if (event.button !== 0 || !event.target.closest(props.handle) || pending) return
  const element = event.target.closest(props.handle),
    scroller = element.closest('.project-list')
  pending = {
    id: item.id,
    pointer: event.pointerId,
    element,
    scroller,
    startX: event.clientX,
    startY: event.clientY,
    y: event.clientY,
    scrollTop: scroller.scrollTop,
    from: props.items.findIndex((s) => s.id === item.id),
    rows: [...list.value.children].map((el) => el.getBoundingClientRect()),
  }
  element.setPointerCapture(event.pointerId)
  addEventListener('keydown', cancelKey)
  addEventListener('blur', cancel)
}
function update() {
  if (!pending || !drag.value) return
  const d = pending,
    viewport = d.scroller.getBoundingClientRect()
  const speed = d.y < viewport.top + 28 ? -8 : d.y > viewport.bottom - 28 ? 8 : 0
  if (speed) d.scroller.scrollTop += speed
  drag.value = { id: d.id, ...sidebarDragLayout(d.rows, d.from, d.y - d.startY + d.scroller.scrollTop - d.scrollTop) }
  frame = requestAnimationFrame(update)
}
function move(event) {
  const d = pending
  if (!d || event.pointerId !== d.pointer || d.saving) return
  d.y = event.clientY
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
  if (!drag.value) {
    cancel()
    return
  }
  d.y = event.clientY
  cancelAnimationFrame(frame)
  const layout = sidebarDragLayout(d.rows, d.from, d.y - d.startY + d.scroller.scrollTop - d.scrollTop)
  drag.value = { id: d.id, ...layout, y: layout.settleY }
  d.saving = true
  try {
    if (layout.index !== d.from) await props.reorder(d.id, layout.index)
  } finally {
    cancel()
  }
}
function cancel() {
  const d = pending
  pending = null
  drag.value = null
  cancelAnimationFrame(frame)
  removeEventListener('keydown', cancelKey)
  removeEventListener('blur', cancel)
  if (d?.element.hasPointerCapture(d.pointer)) d.element.releasePointerCapture(d.pointer)
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
function style(index) {
  const d = drag.value
  if (!d) return
  const y = props.items[index].id === d.id ? d.y : d.offsets[index]
  return { transform: `translateY(${y || 0}px)` }
}
onBeforeUnmount(cancel)
</script>
<template>
  <div
    ref="list"
    class="sidebar-sort-list"
    :class="[kind, { 'sidebar-sort-active': drag }]"
    @click.capture="click"
    @dragstart.prevent
  >
    <div
      v-for="(item, index) in items"
      :key="item.id"
      class="sidebar-sort-item"
      :class="{ 'sidebar-sort-dragging': drag?.id === item.id }"
      :style="style(index)"
      @pointerdown="begin($event, item)"
      @pointermove="move"
      @pointerup="finish"
      @pointercancel="cancel"
      @lostpointercapture="lostCapture"
    >
      <slot :item="item" />
    </div>
  </div>
</template>
