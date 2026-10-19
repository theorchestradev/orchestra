<script setup>
import { computed, ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import Icon from './Icon.vue'
import { spaceSymbol } from '../shared/project'
const props = defineProps({ projects: Array, selectedId: String, settingsOpen: Boolean })
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
onBeforeUnmount(() => observer.disconnect())
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
      :class="{ compact }"
      :style="{ '--visible-spaces': Math.min(projects.length, 8) || 1 }"
      aria-label="Project spaces"
      @keydown="focusSpace"
    >
      <button
        v-for="p in projects"
        :key="p.id"
        class="arc-space"
        :class="{ selected: p.id === selectedId, emoji: !compact && p.emoji }"
        :aria-label="'Switch to ' + p.name"
        :aria-pressed="p.id === selectedId"
        :title="p.description ? p.name + '\n' + p.description : p.name"
        @click="emit('select', p)"
        @contextmenu.prevent="emit('options', { event: $event, project: p })"
        @keydown.shift.f10.prevent="emit('options', { event: $event, project: p })"
      >
        <span v-if="compact" class="arc-space-dot" aria-hidden="true"></span>
        <span v-else aria-hidden="true">{{ spaceSymbol(p) }}</span>
      </button>
    </nav>
    <button class="icon-button arc-footer-action" aria-label="New Project" title="New Project" @click="emit('create')">
      <Icon name="plus" :size="12" />
    </button>
  </footer>
</template>
