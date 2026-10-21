<script>
// Files start collapsed. Scroll positions and expanded or collapsed files survive refreshes, session switches and reopening the pane.
const memory = new Map()
</script>
<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { call } from '../shared/api'
import {
  parsePatch,
  splitPath,
  statusLabels,
  clampDiffWidth,
  diffPlaceholder,
  minDiffWidth,
  maxDiffWidth,
} from '../shared/diff'
import { shortcutLabel } from '../shared/platform'
import Icon from './Icon.vue'
import SessionFiles from './SessionFiles.vue'
const props = defineProps({
  session: Object,
  width: Number,
  selectedPath: String,
  tab: { type: String, default: 'Changes' },
})
const emit = defineEmits(['close', 'resize', 'resized', 'open-file', 'tab'])
const activeTab = computed({ get: () => props.tab, set: (tab) => emit('tab', tab) }),
  fileBrowser = ref(null)
function switchTab(event) {
  const parent = event.currentTarget.parentElement
  activeTab.value = activeTab.value === 'Changes' ? 'Files' : 'Changes'
  nextTick(() => parent.querySelector('[aria-selected="true"]')?.focus())
}
const markers = { add: '+', del: '-', context: ' ', note: '' }
const diff = ref(null),
  error = ref(''),
  loading = ref(false),
  scroller = ref(null),
  overrides = ref(new Map())
const files = computed(() =>
  (diff.value?.files || []).map((file) => ({
    ...file,
    ...splitPath(file.path),
    id: file.path,
    hunks: parsePatch(file.patch),
  })),
)
const scopeLabel = computed(() =>
  diff.value?.scope === 'worktree'
    ? 'Since the worktree started' + (props.session.worktreeBranch ? ' · ' + props.session.worktreeBranch : '')
    : 'Uncommitted changes in the project folder',
)
const scopeDetail = computed(() =>
  diff.value?.scope === 'worktree'
    ? `Commits and uncommitted changes in ${diff.value.directory}, compared with ${diff.value.base.slice(0, 12)}.`
    : `Changes in ${diff.value?.directory} compared with HEAD. Other sessions in this folder share these changes.`,
)
function remembered(id = props.session.id) {
  if (!memory.has(id)) memory.set(id, { scroll: 0, collapsed: new Map() })
  return memory.get(id)
}
const collapsed = (file) => overrides.value.get(file.id) ?? true
function toggle(file, value = !collapsed(file)) {
  overrides.value.set(file.id, value)
  overrides.value = new Map(overrides.value)
  remembered().collapsed = overrides.value
}
let request = 0,
  activityTimer
async function refresh() {
  const id = props.session.id,
    current = ++request
  loading.value = true
  try {
    const result = await call('SessionDiff', id)
    if (current !== request) return
    const scroll = diff.value ? scroller.value?.scrollTop : remembered(id).scroll
    const paths = new Set((result.files || []).map((f) => f.path))
    overrides.value = new Map([...remembered(id).collapsed].filter(([path]) => paths.has(path)))
    remembered(id).collapsed = overrides.value
    diff.value = result
    error.value = ''
    await nextTick()
    if (scroller.value) scroller.value.scrollTop = scroll || 0
  } catch (e) {
    if (current === request) {
      error.value = String(e)
      diff.value = null
    }
  } finally {
    if (current === request) loading.value = false
  }
}
function saveScroll(id = props.session.id) {
  if (diff.value && scroller.value) remembered(id).scroll = scroller.value.scrollTop
}
// Up and Down move between file and hunk headings; Left and Right collapse and expand a file.
function navigate(event) {
  const target = event.target.closest?.('.diff-file-toggle, .diff-hunk-header')
  if (!target) return
  if (['ArrowLeft', 'ArrowRight'].includes(event.key) && target.classList.contains('diff-file-toggle')) {
    event.preventDefault()
    const file = files.value.find((f) => f.id === target.closest('[data-path]').dataset.path)
    if (file) toggle(file, event.key === 'ArrowLeft')
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const items = [...scroller.value.querySelectorAll('.diff-file-toggle, .diff-hunk-header')]
  const index = items.indexOf(target)
  const next =
    event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? items.length - 1
        : Math.max(0, Math.min(items.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)))
  items[next]?.focus()
}
let dragCleanup
function drag(event) {
  const start = event.clientX,
    width = props.width
  const move = (e) => emit('resize', clampDiffWidth(width + start - e.clientX))
  const stop = () => {
    removeEventListener('pointermove', move)
    removeEventListener('pointerup', stop)
    dragCleanup = null
    emit('resized')
  }
  addEventListener('pointermove', move)
  addEventListener('pointerup', stop)
  dragCleanup = stop
}
function resizeKeys(event) {
  const step = event.shiftKey ? 64 : 16
  const width = {
    ArrowLeft: props.width + step,
    ArrowRight: props.width - step,
    Home: maxDiffWidth,
    End: minDiffWidth,
  }[event.key]
  if (width === undefined) return
  event.preventDefault()
  emit('resize', clampDiffWidth(width))
  emit('resized')
}
watch(
  () => props.session.id,
  (id, previous) => {
    saveScroll(previous)
    diff.value = null
    error.value = ''
    overrides.value = remembered(id).collapsed
    refresh()
  },
)
// An agent turn finishing or a process exiting usually means files changed.
watch([() => props.session.activity, () => props.session.running], () => {
  clearTimeout(activityTimer)
  activityTimer = setTimeout(refresh, 400)
})
onMounted(() => {
  overrides.value = remembered().collapsed
  refresh()
})
onBeforeUnmount(() => {
  saveScroll()
  clearTimeout(activityTimer)
  dragCleanup?.()
})
</script>
<template>
  <aside class="diff-pane" :aria-label="activeTab" :style="{ '--diff-pane-width': width + 'px' }">
    <div
      class="diff-resizer"
      role="separator"
      tabindex="0"
      :aria-label="`Resize ${activeTab}`"
      aria-orientation="vertical"
      :aria-valuenow="width"
      :aria-valuemin="minDiffWidth"
      :aria-valuemax="maxDiffWidth"
      @pointerdown.prevent="drag"
      @keydown="resizeKeys"
    />
    <header class="diff-header">
      <div class="diff-title">
        <div class="session-pane-tabs" role="tablist" aria-label="Session sidebar">
          <button
            v-for="tab in ['Changes', 'Files']"
            :id="`session-tab-${tab}`"
            :key="tab"
            role="tab"
            :aria-selected="activeTab === tab"
            :aria-controls="`session-panel-${tab}`"
            :tabindex="activeTab === tab ? 0 : -1"
            @click="activeTab = tab"
            @keydown.left.prevent="switchTab"
            @keydown.right.prevent="switchTab"
          >
            {{ tab }}
          </button>
        </div>
        <span class="spacer" /><button
          class="icon-button"
          :class="{ spinning: activeTab === 'Changes' ? loading : fileBrowser?.loading }"
          :aria-label="`Refresh ${activeTab}`"
          :title="`Refresh ${activeTab}`"
          :disabled="activeTab === 'Changes' ? loading : fileBrowser?.loading"
          @click="activeTab === 'Changes' ? refresh() : fileBrowser?.refresh()"
        >
          <Icon name="arrow.clockwise" :size="14" /></button
        ><button
          class="icon-button"
          aria-label="Hide sidebar"
          :title="`Hide sidebar (${shortcutLabel('⇧⌘B')})`"
          @click="emit('close')"
        >
          <Icon name="xmark" :size="12" />
        </button>
      </div>
      <p v-if="activeTab === 'Changes' && diff?.scope" class="muted caption truncate" :title="scopeDetail">
        {{ scopeLabel }}
      </p>
      <p v-if="activeTab === 'Changes' && diff?.scope" class="diff-totals caption" aria-live="polite">
        {{ files.length }} {{ files.length === 1 ? 'file' : 'files' }}
        <span class="diff-additions">+{{ diff.additions }}</span>
        <span class="diff-deletions">−{{ diff.deletions }}</span>
        <span v-if="diff.truncated" class="muted">· Some changes are too large to show</span>
      </p>
    </header>
    <SessionFiles
      v-if="activeTab === 'Files'"
      id="session-panel-Files"
      ref="fileBrowser"
      role="tabpanel"
      aria-labelledby="session-tab-Files"
      :session="session"
      :selected-path="selectedPath"
      @open="emit('open-file', $event)"
    />
    <div
      v-show="activeTab === 'Changes'"
      id="session-panel-Changes"
      class="session-changes"
      role="tabpanel"
      aria-labelledby="session-tab-Changes"
    >
      <div v-if="error" class="diff-empty" role="alert">
        <Icon name="exclamationmark.triangle" :size="28" />
        <p>{{ error }}</p>
        <button class="standard-button" @click="refresh">Try Again</button>
      </div>
      <div v-else-if="!diff" class="diff-empty"><span class="spinner" /></div>
      <div v-else-if="!diff.scope" class="diff-empty">
        <Icon name="arrow.triangle.branch" :size="28" />
        <p>{{ diff.message }}</p>
      </div>
      <div v-else-if="!files.length" class="diff-empty">
        <Icon name="checkmark.circle" :size="28" />
        <p>
          {{
            diff.scope === 'worktree'
              ? 'No changes since the worktree started.'
              : 'No uncommitted changes in the project folder.'
          }}
        </p>
      </div>
      <div v-else ref="scroller" class="diff-scroller" @keydown="navigate" @scroll.passive="saveScroll()">
        <section v-for="file in files" :key="file.id" class="diff-file" :data-path="file.id">
          <h3>
            <button
              class="diff-file-toggle"
              :aria-expanded="!collapsed(file)"
              :title="(file.oldPath ? file.oldPath + ' → ' : '') + file.path"
              @click="toggle(file)"
            >
              <Icon :name="collapsed(file) ? 'chevron.right' : 'chevron.down'" :size="9" /><span
                class="diff-status"
                :class="'status-' + file.status"
                aria-hidden="true"
                >{{ file.status }}</span
              ><span class="visually-hidden">{{ statusLabels[file.status] }}: </span
              ><span class="diff-path"
                ><span class="diff-folder">{{ file.folder }}</span
                ><span class="diff-name">{{ file.name }}</span></span
              ><span v-if="!file.binary" class="diff-counts"
                ><span class="diff-additions">+{{ file.additions }}</span
                ><span class="diff-deletions">−{{ file.deletions }}</span></span
              >
            </button>
          </h3>
          <template v-if="!collapsed(file)">
            <p v-if="!file.hunks.length" class="diff-placeholder muted">{{ diffPlaceholder(file) }}</p>
            <div v-else class="diff-body">
              <div class="diff-lines">
                <template v-for="(hunk, index) in file.hunks" :key="index"
                  ><div class="diff-hunk-header" tabindex="0" :aria-label="'Hunk ' + hunk.header">
                    {{ hunk.header }}
                  </div>
                  <div v-for="(line, n) in hunk.lines" :key="n" class="diff-line" :class="line.kind">
                    <span class="diff-number">{{ line.old ?? '' }}</span
                    ><span class="diff-number">{{ line.new ?? '' }}</span
                    ><span class="diff-marker">{{ markers[line.kind] }}</span
                    ><span class="diff-text">{{ line.text }}</span>
                  </div></template
                >
              </div>
            </div>
          </template>
          <p v-else-if="file.binary || file.truncated" class="diff-placeholder muted">{{ diffPlaceholder(file) }}</p>
        </section>
      </div>
    </div>
  </aside>
</template>
