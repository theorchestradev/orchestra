<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { call } from '../shared/api'
import Icon from './Icon.vue'
const props = defineProps({ session: Object, selectedPath: String })
const emit = defineEmits(['open'])
const folders = ref(new Map()),
  expanded = ref(new Set()),
  error = ref(''),
  loading = ref(false)
let generation = 0,
  timer
const rows = computed(() => {
  const result = []
  function append(path, depth) {
    for (const entry of folders.value.get(path)?.entries || []) {
      result.push({ ...entry, depth })
      if (entry.directory && expanded.value.has(entry.path)) append(entry.path, depth + 1)
    }
  }
  append('', 0)
  return result
})
async function load(path, current = generation) {
  const result = await call('SessionFiles', props.session.id, path)
  if (current !== generation) return
  folders.value.set(path, result)
  return result
}
async function refresh() {
  const current = ++generation
  loading.value = true
  error.value = ''
  try {
    const paths = ['', ...expanded.value].sort((a, b) => a.split('/').length - b.split('/').length)
    folders.value = new Map()
    for (const path of paths) {
      if (
        path &&
        ![...folders.value.values()].some((folder) =>
          folder.entries.some((entry) => entry.path === path && entry.directory),
        )
      )
        continue
      await load(path, current)
      if (current !== generation) return
    }
  } catch (e) {
    if (current === generation) {
      error.value = String(e)
    }
  } finally {
    if (current === generation) loading.value = false
  }
}
async function open(entry, event) {
  event?.currentTarget?.focus()
  error.value = ''
  const current = generation
  try {
    if (entry.directory) {
      if (expanded.value.has(entry.path)) expanded.value.delete(entry.path)
      else {
        if (!folders.value.has(entry.path)) await load(entry.path, current)
        if (current === generation) expanded.value.add(entry.path)
      }
    } else {
      emit('open', entry.path)
    }
  } catch (e) {
    if (current === generation) {
      error.value = String(e)
    }
  }
}
function navigate(event) {
  const buttons = [...event.currentTarget.querySelectorAll('.session-file-row')]
  const index = buttons.indexOf(event.target)
  if (index < 0) return
  const entry = rows.value[index]
  if (event.key === 'ArrowRight' && entry.directory && !expanded.value.has(entry.path)) {
    event.preventDefault()
    open(entry)
    return
  }
  if (event.key === 'ArrowLeft' && entry.directory && expanded.value.has(entry.path)) {
    event.preventDefault()
    expanded.value.delete(entry.path)
    return
  }
  const next = { ArrowDown: index + 1, ArrowUp: index - 1, Home: 0, End: buttons.length - 1 }[event.key]
  if (next !== undefined) {
    event.preventDefault()
    buttons[Math.max(0, Math.min(buttons.length - 1, next))]?.focus()
  }
}
watch(
  () => props.session.id,
  () => {
    expanded.value = new Set()
    refresh()
  },
  { immediate: true },
)
watch([() => props.session.activity, () => props.session.running], () => {
  clearTimeout(timer)
  timer = setTimeout(refresh, 400)
})
onBeforeUnmount(() => {
  generation++
  clearTimeout(timer)
})
defineExpose({ refresh, loading })
</script>
<template>
  <div class="session-files">
    <p v-if="error" class="diff-placeholder" role="alert">
      {{ error }} <button class="standard-button" @click="refresh">Try Again</button>
    </p>
    <div v-if="loading && !folders.size" class="diff-empty"><span class="spinner" /></div>
    <div v-else class="session-file-tree" aria-label="Session files" @keydown="navigate">
      <p v-if="!rows.length && !error" class="diff-placeholder muted">This folder is empty.</p>
      <button
        v-for="entry in rows"
        :key="entry.path"
        class="session-file-row"
        :class="{ selected: selectedPath === entry.path }"
        :style="{ paddingLeft: 12 + entry.depth * 16 + 'px' }"
        :title="entry.path"
        :aria-expanded="entry.directory ? expanded.has(entry.path) : undefined"
        :aria-pressed="entry.directory ? undefined : selectedPath === entry.path"
        @click="open(entry, $event)"
      >
        <Icon
          :name="entry.directory ? (expanded.has(entry.path) ? 'chevron.down' : 'chevron.right') : 'note.text'"
          :size="10"
        />
        <Icon v-if="entry.directory" name="folder" :size="13" />
        <span class="truncate">{{ entry.name }}</span>
      </button>
      <p v-if="[...folders.values()].some((folder) => folder.truncated)" class="diff-placeholder muted">
        Some folders contain more than 2,000 entries. Only the first 2,000 are shown.
      </p>
    </div>
  </div>
</template>
