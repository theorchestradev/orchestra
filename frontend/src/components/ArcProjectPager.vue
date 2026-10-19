<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { arcScrollDelta } from '../shared/appearance'
import Icon from './Icon.vue'

defineOptions({ inheritAttrs: false })
const props = defineProps({
  items: Array,
  selectedId: String,
  selectProject: Function,
  createProject: Function,
  openSettings: Function,
})
const viewport = ref(null),
  gesture = ref(null)
const activeIndex = computed(() =>
  Math.max(
    0,
    props.items.findIndex((p) => p.id === props.selectedId),
  ),
)
const visualIndex = computed(() => (gesture.value ? gesture.value.index + gesture.value.progress : activeIndex.value))
// Past either edge, the pan fills that edge's ring and only a complete fill runs its action.
const settingsProgress = computed(() => {
  const g = gesture.value
  return g && g.index === 0 ? Math.max(0, -g.progress) : 0
})
const createProgress = computed(() => {
  const g = gesture.value
  return g && g.index === props.items.length - 1 ? Math.max(0, g.progress) : 0
})
const edgeAction = (index) =>
  index === -1 ? props.openSettings : index === props.items.length ? props.createProject : null
let wheelTimer,
  wheelLocked = false,
  wheelActive = false,
  wheelTail = Infinity,
  lastWheel = null
async function finishGesture() {
  const g = gesture.value
  if (!g || g.saving) return
  const action = edgeAction(g.index + Math.sign(g.progress))
  if (action) {
    gesture.value = null
    if (Math.abs(g.progress) < 1) return
    wheelLocked = true
    action()
    return
  }
  const project = props.items[g.index + Math.sign(g.progress)]
  if (Math.abs(g.progress) * viewport.value.clientWidth < 40 || !project) {
    gesture.value = null
    return
  }
  g.progress = Math.sign(g.progress)
  g.saving = true
  wheelLocked = true
  try {
    await props.selectProject(project)
  } finally {
    if (gesture.value === g) gesture.value = null
  }
}
function idle() {
  wheelActive = false
  finishGesture()
  wheelLocked = false
}
function wheel(event) {
  const el = viewport.value,
    delta = arcScrollDelta(event, el.clientWidth),
    magnitude = Math.abs(delta)
  if (!delta || props.items.length < (props.createProject || props.openSettings ? 1 : 2) || !el.clientWidth) return
  event.preventDefault()
  // A new stroke can overlap the previous momentum in the same direction. Rearm on
  // renewed force after decay as well as a pause/reversal; tiny tail jitter stays locked.
  if (
    lastWheel &&
    (event.timeStamp - lastWheel.time >= 50 ||
      Math.sign(delta) !== lastWheel.direction ||
      (magnitude >= wheelTail * 2 && magnitude - wheelTail >= 8))
  )
    wheelLocked = false
  if (wheelLocked && lastWheel && magnitude < lastWheel.magnitude) wheelTail = Math.min(wheelTail, magnitude)
  lastWheel = { time: event.timeStamp, direction: Math.sign(delta), magnitude }
  wheelActive = true
  clearTimeout(wheelTimer)
  wheelTimer = setTimeout(idle, 150)
  if (wheelLocked || gesture.value?.saving) return
  if (!gesture.value) {
    wheelTail = Infinity
    gesture.value = { index: activeIndex.value, progress: 0, saving: false }
  }
  const g = gesture.value,
    min = g.index > 0 || props.openSettings ? -1 : 0,
    max = g.index < props.items.length - 1 || props.createProject ? 1 : 0
  g.progress = Math.max(min, Math.min(max, g.progress + delta / el.clientWidth))
  if (Math.abs(g.progress) === 1) finishGesture()
}
function routeWheel(event) {
  const el = viewport.value
  // Hit-test the fixed viewport instead of depending on the outgoing page's wheel target.
  if (el?.contains(document.elementFromPoint(event.clientX, event.clientY))) wheel(event)
}
function cancelGesture() {
  gesture.value = null
  wheelLocked = wheelActive
  wheelTail = Infinity
}
watch(
  () => props.selectedId,
  () => {
    if (!gesture.value?.saving) cancelGesture()
  },
)
watch(() => props.items.map((p) => p.id).join(','), cancelGesture)
onMounted(() => window.addEventListener('wheel', routeWheel, { capture: true, passive: false }))
onBeforeUnmount(() => {
  clearTimeout(wheelTimer)
  window.removeEventListener('wheel', routeWheel, true)
})
</script>
<template>
  <div ref="viewport" class="arc-project-pager">
    <div
      class="arc-project-track"
      :class="{ 'arc-project-panning': gesture && !gesture.saving }"
      :style="{ transform: `translateX(-${(visualIndex + (openSettings ? 1 : 0)) * 100}%)` }"
    >
      <div v-if="openSettings" class="arc-project-page arc-project-edge" aria-hidden="true" inert>
        <span class="arc-project-edge-ring" :style="{ '--arc-edge-progress': settingsProgress }">
          <Icon name="gearshape" :size="16" />
        </span>
        <span>Settings</span>
      </div>
      <div
        v-for="p in items"
        :key="p.id"
        class="arc-project-page project-list"
        :aria-hidden="p.id !== selectedId"
        :inert="p.id !== selectedId"
      >
        <slot :item="p" />
      </div>
      <div v-if="createProject" class="arc-project-page arc-project-edge" aria-hidden="true" inert>
        <span class="arc-project-edge-ring" :style="{ '--arc-edge-progress': createProgress }">
          <Icon name="plus" :size="16" />
        </span>
        <span>New Project</span>
      </div>
    </div>
  </div>
</template>
