<script setup>
import Icon from './Icon.vue'
import { spaceSymbol } from '../shared/project'
defineProps({ projects: Array, selectedId: String, settingsOpen: Boolean })
const emit = defineEmits(['select', 'options', 'settings', 'create'])
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
    <nav class="arc-spaces" aria-label="Project spaces" @keydown="focusSpace">
      <button
        v-for="p in projects"
        :key="p.id"
        class="arc-space"
        :class="{ selected: p.id === selectedId, emoji: p.emoji }"
        :aria-label="'Switch to ' + p.name"
        :aria-pressed="p.id === selectedId"
        :title="p.description ? p.name + '\n' + p.description : p.name"
        @click="emit('select', p)"
        @contextmenu.prevent="emit('options', { event: $event, project: p })"
        @keydown.shift.f10.prevent="emit('options', { event: $event, project: p })"
      >
        <span aria-hidden="true">{{ spaceSymbol(p) }}</span>
      </button>
    </nav>
    <button class="icon-button arc-footer-action" aria-label="New Project" title="New Project" @click="emit('create')">
      <Icon name="plus" :size="12" />
    </button>
  </footer>
</template>
