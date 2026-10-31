<script setup>
import { computed, ref, watch, nextTick } from 'vue'
import { palette, agentIcon } from '../shared/appearance'
import { commandItems, filterCommands } from '../shared/commands'
import { shortcutLabel } from '../shared/platform'
import Icon from './Icon.vue'
const props = defineProps({
  preferences: Object,
  workspaces: Array,
  workspace: Object,
  session: Object,
  apps: Array,
  sidebarHidden: Boolean,
})
const emit = defineEmits(['run', 'close'])
const query = ref(''),
  parent = ref(null),
  selected = ref(0),
  search = ref(null),
  list = ref(null)
const items = computed(() => commandItems(props))
const results = computed(() => filterCommands(parent.value ? parent.value.children : items.value, query.value))
const placeholder = computed(() => parent.value?.placeholder || 'Search commands')
watch(query, () => (selected.value = 0))
function heading(result, i) {
  return parent.value
    ? i === 0 && parent.value.name.replace('…', '')
    : (i === 0 || results.value[i - 1].group !== result.group) && result.group
}
function enter(item) {
  parent.value = item
  query.value = ''
  selected.value = 0
  search.value?.focus()
}
function activate(result = results.value[selected.value]) {
  if (!result) return
  if (result.children) enter(result)
  else
    emit(
      'run',
      parent.value
        ? { command: parent.value.id, item: result }
        : { command: result.command || result.id, item: result },
    )
}
function back() {
  if (parent.value && !query.value) enter(null)
}
async function move(delta) {
  selected.value = Math.max(0, Math.min(results.value.length - 1, selected.value + delta))
  await nextTick()
  list.value?.querySelector(`[data-index="${selected.value}"]`)?.scrollIntoView({ block: 'nearest' })
}
</script>
<template>
  <div class="command-search">
    <Icon name="magnifyingglass" /><input
      autocorrect="off"
      autocapitalize="off"
      ref="search"
      autofocus
      :placeholder="placeholder"
      :aria-label="placeholder"
      v-model="query"
      @keydown.down.prevent="move(1)"
      @keydown.up.prevent="move(-1)"
      @keydown.enter.prevent="activate()"
      @keydown.backspace="back"
    /><kbd>esc</kbd>
  </div>
  <div class="command-results">
    <div ref="list" class="command-list" role="listbox" :aria-label="parent ? parent.name : 'Commands'">
      <template v-for="(result, i) in results" :key="result.id"
        ><h3 v-if="heading(result, i)">{{ heading(result, i).toUpperCase() }}</h3>
        <button
          role="option"
          :aria-selected="selected === i"
          :data-index="i"
          :class="{ selected: selected === i }"
          @pointermove="selected = i"
          @click="activate(result)"
        >
          <i
            v-if="result.theme"
            class="theme-dot"
            :style="{ background: palette({ ...preferences, theme: result.theme }).accent }"
          /><img v-else-if="result.image" class="command-agent" :src="result.image" alt="" /><img
            v-else-if="result.agent"
            class="command-agent"
            :src="agentIcon(result.agent, preferences.appearance)"
            alt=""
          /><Icon v-else :name="result.icon" :size="14" /><span class="command-name"
            ><span class="truncate">{{ result.name }}</span
            ><small v-if="result.detail" class="muted truncate">{{ result.detail }}</small></span
          ><Icon
            v-if="result.theme && preferences.theme === result.theme"
            class="accent"
            name="checkmark"
            :size="14"
          /><kbd v-else-if="result.shortcut">{{ shortcutLabel(result.shortcut) }}</kbd
          ><Icon v-else-if="result.children" name="chevron.right" :size="10" /></button
      ></template>
      <p v-if="!results.length" class="muted no-results">No results</p>
    </div>
  </div>
</template>
