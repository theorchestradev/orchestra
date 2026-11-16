<script setup>
import { computed, ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { call, copy, openURL } from '../shared/api'
import { relativeDate } from '../shared/appearance'
import {
  canWorkOn,
  defaultSortDirection,
  descriptionSource,
  isPermissionError,
  itemKey,
  itemSessions,
  matchesWorkItem,
  providerIcon,
  providerName,
  sortWorkItems,
  workItemAssignees,
} from '../shared/workItems'
import { workspaceSources } from '../shared/workspace'
import { sanitize, renderMarkdown } from '../shared/markdown'
import { shortcutLabel } from '../shared/platform'
import { refreshDelay } from '../shared/pullRequest'
import Icon from './Icon.vue'
import Avatar from './Avatar.vue'
import ConflictNotice from './ConflictNotice.vue'
import GitHubMetadataEditor from './GitHubMetadataEditor.vue'
import GitHubSessions from './GitHubSessions.vue'
import Modal from './Modal.vue'
import PullRequestChecks from './PullRequestChecks.vue'
import WorkOnButton from './WorkOnButton.vue'
const props = defineProps({
  workspace: Object,
  // 'workItems' lists issues; 'reviews' lists pull requests.
  kind: { type: String, default: 'workItems' },
  visible: Boolean,
  countsEnabled: Boolean,
  refreshInterval: Number,
  defaultAgent: String,
  profiles: Array,
})
const emit = defineEmits(['work', 'resolve', 'session', 'error', 'count'])
const detailElement = ref(null),
  itemMenu = ref(null),
  editing = ref(null),
  descriptionEditor = ref(null),
  descriptionButton = ref(null)
const state = ref('open'),
  search = ref(''),
  // The source and assignee filters; '' shows all, and '-' items without assignees.
  source = ref(''),
  assignee = ref(''),
  sort = ref({ column: 'updated', direction: 'desc' }),
  items = ref([]),
  loading = ref(false),
  error = ref(''),
  // Sources that failed while others loaded: [{ name, message }].
  sourceErrors = ref([]),
  selected = ref(null),
  content = ref(null),
  detailsLoading = ref(false),
  detailsError = ref(''),
  draft = ref(null),
  discarding = ref(null),
  readOnly = ref(new Set()),
  pullStatus = ref(null),
  checksError = ref('')
let request = 0,
  detailRequest = 0,
  checksRequest = 0,
  checksTimer
// Each view lists the workspace's shown sources of its kind; items say what their provider supports.
const pull = computed(() => props.kind === 'reviews')
const title = computed(() => (pull.value ? 'Reviews' : 'Work Items'))
const sources = computed(() => workspaceSources(props.workspace, props.kind))
const sourceKey = computed(() => sources.value.map((r) => JSON.stringify(r)).join(','))
const sourceOf = (item) => sources.value.find((r) => r.id === item?.sourceID)
const filters = computed(() => ({ source: source.value, assignee: assignee.value, search: search.value }))
const visibleItems = computed(() =>
  sortWorkItems(
    items.value.filter((item) => matchesWorkItem(item, filters.value)),
    sort.value,
  ),
)
const columns = [
  { id: 'id', label: 'ID' },
  { id: 'title', label: 'Title' },
  { id: 'assignees', label: 'Assignees', wide: true },
  { id: 'status', label: 'Status' },
  { id: 'updated', label: 'Updated', wide: true },
]
// Choosing the sorted column again reverses it.
function sortBy(column) {
  sort.value =
    sort.value.column === column
      ? { column, direction: sort.value.direction === 'asc' ? 'desc' : 'asc' }
      : { column, direction: defaultSortDirection(column) }
}
const sortLabel = (column) =>
  sort.value.column === column.id
    ? `${column.label}, sorted ${sort.value.direction === 'asc' ? 'ascending' : 'descending'}`
    : `Sort by ${column.label}`
const countAvailable = ref(false)
watch(
  () => [countAvailable.value, loading.value, visibleItems.value.length],
  () => emit('count', countAvailable.value && !loading.value ? visibleItems.value.length : null),
  { immediate: true },
)
const assignees = computed(() => workItemAssignees(items.value))
// Rows name their source when several repositories are listed.
const showsSource = computed(() => sources.value.length > 1)
const statusIcon = (item) =>
  item.state === 'open'
    ? 'open'
    : item.stateReason === 'not_planned'
      ? 'not-planned'
      : item.stateReason === 'duplicate'
        ? 'duplicate'
        : 'completed'
const sanitizeBody = (html) => sanitize(html || '<p class="muted">No description.</p>')
function linkClick(event) {
  const link = event.target.closest('a')
  if (link) {
    event.preventDefault()
    openURL(link.href)
  }
}
async function load(background = false) {
  const token = ++request
  countAvailable.value = false
  if (background !== true) {
    loading.value = true
    items.value = []
  }
  error.value = ''
  const listed = sources.value
  const results = await Promise.allSettled(
    listed.map((source) => call('WorkItems', props.workspace.id, source.id, state.value)),
  )
  if (token !== request) return
  const loaded = results.flatMap((r) => (r.status === 'fulfilled' ? [r.value] : []))
  const failed = results.flatMap((r, i) =>
    r.status === 'rejected' ? [{ name: listed[i].name, message: String(r.reason) }] : [],
  )
  if (loaded.length || !failed.length) items.value = loaded.flat()
  sourceErrors.value = loaded.length ? failed : []
  if (!loaded.length && failed.length)
    error.value = failed.map((f) => (listed.length > 1 ? f.name + ': ' : '') + f.message).join('\n')
  countAvailable.value = !failed.length
  loading.value = false
}
async function detail(item, background = false) {
  selected.value = item
  const token = ++detailRequest
  if (!background) {
    detailsLoading.value = true
    content.value = null
  }
  detailsError.value = ''
  if (!background) loadChecks(item)
  try {
    const result = await call('WorkItemDetail', props.workspace.id, item.sourceID, item.id)
    if (token === detailRequest) content.value = result
  } catch (e) {
    if (token === detailRequest) detailsError.value = String(e)
  } finally {
    if (token === detailRequest) detailsLoading.value = false
  }
}
// A pull request's checks load beside its details, and poll on their own: quickly while
// checks run, slowly once they settle, and only while the detail is shown.
async function loadChecks(item, background = false) {
  const token = ++checksRequest
  clearTimeout(checksTimer)
  if (!background) {
    pullStatus.value = null
    checksError.value = ''
  }
  if (!item.can?.checks) return
  try {
    const result = await call('ReviewStatus', props.workspace.id, item.sourceID, item.id)
    if (token === checksRequest) {
      pullStatus.value = result
      checksError.value = ''
    }
  } catch (e) {
    if (token === checksRequest) checksError.value = String(e)
  } finally {
    if (token === checksRequest) scheduleChecks()
  }
}
function scheduleChecks() {
  clearTimeout(checksTimer)
  if (props.visible && selected.value?.can?.checks)
    checksTimer = setTimeout(
      () => (document.hidden ? scheduleChecks() : loadChecks(selected.value, true)),
      refreshDelay(pullStatus.value),
    )
}
const loadFailure = (jobID) => call('ReviewCheckFailure', props.workspace.id, selected.value.sourceID, jobID)
function back() {
  if (editing.value || draft.value?.saving) return
  discardDraft(() => {
    selected.value = null
    detailRequest++
    loadChecks({})
  })
}
// The draft keeps the source it started from so saving can detect edits made on the provider.
const draftChanged = computed(() => draft.value && draft.value.text !== draft.value.source)
const conflict = computed(() => content.value?.conflict)
// readOnly holds the itemKey of items the provider refused to edit.
const canEdit = (capability) => content.value?.can[capability] && !readOnly.value.has(itemKey(selected.value))
const canEditDescription = computed(() => canEdit('editDescription'))
async function editDescription() {
  draft.value = {
    original: content.value.body ?? '',
    source: descriptionSource(content.value),
    text: descriptionSource(content.value),
    saving: false,
    error: '',
  }
  await nextTick()
  descriptionEditor.value?.focus()
}
function discardDraft(then) {
  if (!draftChanged.value) {
    draft.value = null
    then()
  } else discarding.value = then
}
async function closeDescription() {
  draft.value = null
  await nextTick()
  descriptionButton.value?.focus()
}
function cancelDescription() {
  if (!draft.value.saving) discardDraft(closeDescription)
}
async function confirmDiscard() {
  const then = discarding.value
  discarding.value = null
  draft.value = null
  await nextTick()
  then()
}
async function saveDescription() {
  const current = draft.value,
    item = selected.value
  if (!current || current.saving) return
  current.saving = true
  current.error = ''
  try {
    await call('UpdateWorkItem', props.workspace.id, item.sourceID, item.id, 'body', [current.text, current.original])
  } catch (e) {
    current.error = String(e)
    if (isPermissionError(e)) readOnly.value.add(itemKey(item))
    return
  } finally {
    current.saving = false
  }
  if (draft.value === current) await closeDescription()
  await Promise.all([selected.value === item && detail(item, true), load(true)])
}
const editCapability = { state: 'editState', assignees: 'editAssignees', labels: 'editLabels' }
function editMetadata(field, event) {
  if (!canEdit(editCapability[field]) || editing.value?.saving) return
  editing.value = editing.value?.field === field ? null : { field, anchor: event.currentTarget }
}
async function metadataSaved(patch) {
  const anchor = editing.value?.anchor
  editing.value = null
  Object.assign(content.value, patch)
  const row = items.value.find((i) => itemKey(i) === itemKey(selected.value))
  if (row) Object.assign(row, patch)
  await nextTick()
  anchor?.focus()
  await Promise.all([detail(selected.value, true), load(true)])
}
function showItemMenu(event, item) {
  event.preventDefault()
  const rect = event.type === 'keydown' ? event.currentTarget.getBoundingClientRect() : null
  itemMenu.value = {
    item,
    x: Math.max(8, Math.min(rect ? rect.left : event.clientX, innerWidth - 218)),
    y: Math.max(8, Math.min(rect ? rect.bottom + 4 : event.clientY, innerHeight - 84)),
  }
}
function dismissItemMenu() {
  itemMenu.value = null
}
function menuKeys(event) {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault()
    const buttons = [...event.currentTarget.querySelectorAll('button')]
    buttons[(buttons.indexOf(document.activeElement) + 1) % buttons.length]?.focus()
  }
}
watch(itemMenu, async (value) => {
  if (value) {
    await nextTick()
    document.querySelector('.context-menu button')?.focus()
  }
})
watch(
  () => props.visible,
  (visible) => {
    if (!visible) {
      itemMenu.value = null
      editing.value = null
      clearTimeout(checksTimer)
    } else if (selected.value?.can?.checks) loadChecks(selected.value, true)
  },
)
function keys(event) {
  if (!props.visible) return
  if (event.key === 'Escape') dismissItemMenu()
  if (
    selected.value &&
    detailElement.value?.offsetParent !== null &&
    (event.metaKey || event.ctrlKey) &&
    event.key === '['
  ) {
    event.preventDefault()
    back()
  }
}
onMounted(() => {
  document.addEventListener('keydown', keys)
  document.addEventListener('pointerdown', dismissItemMenu)
})
onBeforeUnmount(() => {
  document.removeEventListener('keydown', keys)
  document.removeEventListener('pointerdown', dismissItemMenu)
})
const detailURL = computed(() => content.value?.url || selected.value?.url)
const date = (value) => {
  const d = new Date(value)
  return (
    d.toLocaleDateString('en-GB', { dateStyle: 'medium' }) +
    ' at ' +
    d.toLocaleTimeString('en-GB', { timeStyle: 'short', hour12: false }).replace(/^0/, '')
  )
}
let refreshTimer,
  refreshBusy = false
watch(
  () => [props.visible || props.countsEnabled, props.refreshInterval],
  ([visible, seconds]) => {
    clearInterval(refreshTimer)
    if (visible && seconds > 0)
      refreshTimer = setInterval(() => {
        if (document.hidden || refreshBusy || editing.value || itemMenu.value || loading.value || detailsLoading.value)
          return
        refreshBusy = true
        Promise.all([load(true), selected.value ? detail(selected.value, true) : Promise.resolve()]).finally(
          () => (refreshBusy = false),
        )
      }, seconds * 1000)
  },
  { immediate: true },
)
onBeforeUnmount(() => {
  request++
  clearInterval(refreshTimer)
  clearTimeout(checksTimer)
  checksRequest++
})
watch(state, () => load())
// Adding, removing or detecting a repository changes what the view lists.
watch(sourceKey, () => {
  if (source.value && !sources.value.some((r) => r.id === source.value)) source.value = ''
  if (selected.value && !sourceOf(selected.value)) back()
  load()
})
watch(assignees, (logins) => {
  if (assignee.value && assignee.value !== '-' && !logins.includes(assignee.value)) assignee.value = ''
})
onMounted(() => load())
</script>
<template>
  <section v-if="!selected" class="github-page content-page">
    <header class="github-header">
      <div class="github-heading">
        <div>
          <h2>{{ title }}</h2>
          <span class="work-item-sources"
            ><button
              v-for="source in sources"
              :key="source.id"
              class="text-link muted caption"
              :title="providerName(source.provider) + ' · ' + source.name"
              :aria-label="'Open ' + source.name"
              @click="openURL(source.url)"
            >
              <Icon :name="providerIcon(source.provider)" :size="12" />{{ source.name }}
            </button></span
          >
        </div>
        <button class="standard-button muted" :disabled="loading" @click="load">
          <Icon name="arrow.clockwise" :size="14" />Refresh
        </button>
      </div>
      <div class="github-filters">
        <label class="search-field"
          ><Icon name="magnifyingglass" :size="14" /><input
            autocorrect="off"
            autocapitalize="off"
            v-model="search"
            :aria-label="'Search ' + (pull ? 'pull requests' : 'issues')"
            placeholder="Search by title or number" /><button
            v-if="search"
            class="icon-button"
            aria-label="Clear Search"
            @click="search = ''"
          >
            <Icon name="xmark.circle.fill" :size="12" /></button></label
        ><span class="work-item-filters"
          ><label class="status-filter"
            ><Icon name="line.3.horizontal.decrease" :size="14" /><select v-model="state" aria-label="Status">
              <option value="open">Open</option>
              <option value="closed">Closed</option>
              <option value="all">All</option></select
            ><Icon name="chevron.down" :size="9" /></label
          ><label v-if="sources.length > 1" class="status-filter"
            ><Icon name="folder" :size="14" /><select v-model="source" aria-label="Source">
              <option value="">All sources</option>
              <option v-for="s in sources" :key="s.id" :value="s.id">{{ s.name }}</option></select
            ><Icon name="chevron.down" :size="9" /></label
          ><label class="status-filter"
            ><Icon name="person.crop.circle" :size="14" /><select v-model="assignee" aria-label="Assignee">
              <option value="">Anyone</option>
              <option value="-">Unassigned</option>
              <option v-for="login in assignees" :key="login" :value="login">{{ login }}</option></select
            ><Icon name="chevron.down" :size="9" /></label
        ></span>
      </div>
    </header>
    <div v-if="loading" class="empty-state">
      <span class="spinner" />
      <p class="muted">Loading {{ pull ? 'pull requests' : 'issues' }}…</p>
    </div>
    <div v-else-if="error" class="empty-state github-unavailable">
      <Icon name="exclamationmark.triangle" :size="42" />
      <h2>Could not load {{ pull ? 'pull requests' : 'issues' }}</h2>
      <p class="source-error">{{ error }}</p>
      <button class="standard-button" @click="load">Try Again</button>
    </div>
    <div v-else-if="!visibleItems.length" class="empty-state github-unavailable">
      <Icon :name="pull ? 'arrow.triangle.branch' : 'circle.dotted'" :size="42" />
      <h2>{{ pull ? 'No Pull Requests' : 'No Issues' }}</h2>
      <p>
        {{ search || source || assignee ? 'No items match your filters.' : 'There are no items with this status.' }}
      </p>
    </div>
    <p v-for="failure in sourceErrors" :key="failure.name" class="status-error caption source-warning" role="alert">
      Could not load {{ failure.name }}: {{ failure.message }}
    </p>
    <div
      v-if="!loading && !error && visibleItems.length"
      class="github-table"
      :class="{ 'has-named-items': items.some((i) => !i.number) }"
    >
      <div class="github-table-head">
        <template v-for="column in columns" :key="column.id"
          ><span v-if="column.id === 'assignees'" class="wide-column"
            ><span class="visually-hidden">Sessions</span></span
          ><button
            class="sort-header"
            :class="{
              'wide-column': column.wide,
              sorted: sort.column === column.id,
              ascending: sort.direction === 'asc',
            }"
            :aria-label="sortLabel(column)"
            :title="sortLabel(column)"
            @click="sortBy(column.id)"
          >
            <span class="truncate">{{ column.label.toUpperCase() }}</span
            ><Icon name="chevron.down" :size="8" /></button
        ></template>
      </div>
      <div class="github-rows">
        <div
          v-for="item in visibleItems"
          :key="itemKey(item)"
          class="github-row"
          :class="{ 'named-row': !item.number }"
          @click="detail(item)"
          @contextmenu="showItemMenu($event, item)"
        >
          <span class="item-id" :title="item.identifier"
            ><Icon :name="item.review ? 'arrow.triangle.branch' : 'circle.dotted'" :size="13" /><span
              class="truncate"
              >{{ item.identifier }}</span
            ></span
          ><span class="item-context"
            ><button class="item-title-line item-open">
              <strong :title="item.title">{{ item.title }}</strong
              ><span class="muted caption item-byline"
                ><span
                  v-if="showsSource"
                  class="item-source"
                  :title="providerName(item.provider) + ' · ' + sourceOf(item)?.name"
                  ><Icon :name="providerIcon(item.provider)" :size="10" />{{ sourceOf(item)?.name }}</span
                >{{ showsSource ? ' · ' : '' }}by {{ item.author?.login || 'Deleted user'
                }}<span class="compact-date"> · {{ relativeDate(item.updatedAt) }}</span></span
              ></button
            ><span v-if="item.labels?.length" class="labels"
              ><span v-for="label in item.labels.slice(0, 3)" :key="label.name" class="label-chip"
                ><i
                  :style="{ background: /^[0-9a-f]{6}$/i.test(label.color) ? '#' + label.color : 'var(--muted)' }"
                /><span>{{ label.name }}</span></span
              ><span v-if="item.labels.length > 3" class="muted caption">+{{ item.labels.length - 3 }}</span></span
            ><span
              v-if="item.assignees?.length || itemSessions(workspace, item).length"
              class="compact-assignees assignees"
              ><Avatar v-for="assignee in item.assignees?.slice(0, 3)" :key="assignee.login" :author="assignee" /><span
                v-if="item.assignees?.length > 3"
                class="muted caption"
                >+{{ item.assignees.length - 3 }}</span
              ><GitHubSessions :sessions="itemSessions(workspace, item)" @open="emit('session', $event)" /></span></span
          ><span class="wide-column"
            ><GitHubSessions :sessions="itemSessions(workspace, item)" @open="emit('session', $event)" /></span
          ><span class="wide-column assignees"
            ><Avatar v-for="assignee in item.assignees?.slice(0, 3)" :key="assignee.login" :author="assignee" /><span
              v-if="item.assignees?.length > 3"
              >+{{ item.assignees.length - 3 }}</span
            ><span v-if="!item.assignees?.length" class="muted">—</span></span
          ><span class="status-badge" :class="item.status.toLowerCase()" :title="item.status"
            ><Icon
              :name="
                {
                  Open: 'circle.dotted',
                  Closed: 'checkmark.circle',
                  Merged: 'arrow.triangle.merge',
                  Draft: 'circle.dashed',
                }[item.status] || 'circle.dotted'
              "
              :size="12"
            /><span class="truncate">{{ item.status }}</span></span
          ><span class="wide-column muted caption">{{ relativeDate(item.updatedAt) }}</span
          ><Icon class="compact-chevron" name="chevron.right" :size="8" />
        </div>
      </div>
    </div>
  </section>
  <section v-else ref="detailElement" class="github-detail content-page" :class="{ 'named-detail': !selected.number }">
    <header class="detail-toolbar">
      <button class="text-link muted" @click="back"><Icon name="chevron.left" :size="12" />{{ title }}</button
      ><span class="muted">/</span
      ><span class="truncate repository" :title="sourceOf(selected)?.name">{{ sourceOf(selected)?.name }}</span
      ><button
        class="text-link item-number"
        :aria-label="`Open ${selected.identifier} on ${providerName(selected.provider)}`"
        @click="openURL(detailURL)"
        @contextmenu="showItemMenu($event, { ...selected, url: detailURL })"
        @keydown.shift.f10="showItemMenu($event, { ...selected, url: detailURL })"
        @keydown.context-menu="showItemMenu($event, { ...selected, url: detailURL })"
      >
        {{ selected.identifier }}
      </button>
    </header>
    <div v-if="detailsLoading" class="empty-state">
      <span class="spinner" />
      <p>Loading details…</p>
    </div>
    <div v-else-if="detailsError" class="empty-state github-unavailable">
      <h2>Could not load details</h2>
      <p>{{ detailsError }}</p>
      <button class="standard-button" @click="detail(selected)">Try Again</button>
    </div>
    <main v-else-if="content" class="github-article" @click="linkClick">
      <div class="github-article-content">
        <section
          class="issue-heading"
          :class="{ 'without-work-on': !canWorkOn(content) && !itemSessions(workspace, selected).length }"
        >
          <div v-if="canWorkOn(content) || itemSessions(workspace, selected).length" class="work-on">
            <GitHubSessions
              :sessions="itemSessions(workspace, selected)"
              @open="emit('session', $event)"
            /><WorkOnButton
              v-if="canWorkOn(content)"
              :label="selected.review ? 'Review' : 'Work on'"
              :default-agent="defaultAgent"
              :profiles="profiles"
              :profile-id="workspace.profileID || ''"
              :visible="visible"
              :repositories="content.can.chooseRepository ? workspace.repositories : []"
              :repository-id="selected.repositoryID"
              @work="emit('work', content, $event)"
            />
          </div>
          <div class="issue-title-line">
            <h1>
              {{ content.title }} <span class="muted">{{ content.identifier }}</span>
            </h1>
            <span class="muted caption detail-byline"
              >by {{ content.author?.login || 'Deleted user' }} · updated
              {{ relativeDate(content.updatedAt || content.createdAt) }}</span
            >
          </div>
        </section>
        <section class="metadata">
          <div>
            <h3>STATUS</h3>
            <button
              class="metadata-trigger status-badge"
              :class="content.status.toLowerCase()"
              :aria-label="canEdit('editState') ? 'Edit status' : 'Status'"
              :disabled="!canEdit('editState')"
              :title="
                canEdit('editState')
                  ? 'Edit status'
                  : content.status === 'Merged'
                    ? 'A merged pull request cannot change status.'
                    : content.status
              "
              :aria-expanded="editing?.field === 'state'"
              aria-haspopup="menu"
              @click="editMetadata('state', $event)"
            >
              <Icon v-if="content.status === 'Merged'" name="arrow.triangle.merge" :size="14" /><Icon
                v-else-if="statusIcon(content) === 'duplicate'"
                name="doc.on.doc"
                :size="14"
              /><span v-else class="status-choice-icon" :class="statusIcon(content)" aria-hidden="true" />{{
                content.status
              }}<Icon v-if="canEdit('editState')" name="chevron.down" :size="9" />
            </button>
          </div>
          <div>
            <h3>ASSIGNEES</h3>
            <button
              class="metadata-trigger metadata-people"
              :disabled="!canEdit('editAssignees')"
              :aria-label="canEdit('editAssignees') ? 'Edit assignees' : 'Assignees'"
              :title="
                (canEdit('editAssignees') ? 'Edit assignees' : 'Assignees') +
                (content.assignees?.length ? ': ' + content.assignees.map((a) => a.login).join(', ') : '')
              "
              :aria-expanded="editing?.field === 'assignees'"
              @click="editMetadata('assignees', $event)"
            >
              <span v-if="!content.assignees?.length" class="muted">Unassigned</span
              ><Avatar
                v-for="assignee in content.assignees?.slice(0, 3)"
                :key="assignee.login"
                :author="assignee"
              /><span v-if="content.assignees?.length > 3" class="caption">+{{ content.assignees.length - 3 }}</span
              ><Icon v-if="canEdit('editAssignees')" name="chevron.down" :size="9" />
            </button>
          </div>
          <div>
            <h3>LABELS</h3>
            <button
              class="metadata-trigger labels"
              :disabled="!canEdit('editLabels')"
              :aria-label="canEdit('editLabels') ? 'Edit labels' : 'Labels'"
              :title="canEdit('editLabels') ? 'Edit labels' : 'Labels'"
              :aria-expanded="editing?.field === 'labels'"
              aria-haspopup="dialog"
              @click="editMetadata('labels', $event)"
            >
              <span v-if="!content.labels?.length" class="muted">No labels</span
              ><span v-for="label in content.labels" :key="label.name" class="label-chip"
                ><i
                  :style="{ background: /^[0-9a-f]{6}$/i.test(label.color) ? '#' + label.color : 'var(--muted)' }"
                /><span>{{ label.name }}</span></span
              ><Icon v-if="canEdit('editLabels')" name="chevron.down" :size="9" />
            </button>
          </div>
        </section>
        <div v-if="content.branches" class="branches">
          <code>{{ content.branches.head }}</code
          ><span class="muted">→</span><code>{{ content.branches.base }}</code
          ><span class="muted">{{ content.branches.changedFiles }} files changed</span
          ><span class="status-open">+{{ content.branches.additions }}</span
          ><span class="status-error">−{{ content.branches.deletions }}</span>
        </div>
        <ConflictNotice
          v-if="conflict"
          :conflict="conflict"
          :default-agent="defaultAgent"
          :profiles="profiles"
          :profile-id="workspace.profileID || ''"
          :visible="visible"
          :read-only="readOnly.has(itemKey(selected))"
          @resolve="emit('resolve', { ...conflict, repositoryID: selected.repositoryID }, $event)"
        />
        <template v-if="selected.can.checks">
          <PullRequestChecks
            v-if="pullStatus"
            :key="itemKey(selected)"
            :status="pullStatus"
            :load-failure="loadFailure"
            @error="checksError = $event"
          />
          <p v-else-if="!checksError" class="pr-checks pr-checks-loading muted caption">
            <span class="spinner" />Loading checks…
          </p>
          <p v-if="checksError" class="status-error caption pr-checks-error" role="alert">{{ checksError }}</p>
        </template>
        <section class="discussion">
          <div class="description">
            <form v-if="draft" class="description-form" @submit.prevent="saveDescription">
              <textarea
                autocorrect="off"
                autocapitalize="off"
                ref="descriptionEditor"
                v-model="draft.text"
                class="description-editor"
                aria-label="Description"
                :aria-describedby="`description-hint-${workspace.id}`"
                spellcheck="true"
                :readonly="draft.saving"
                :aria-invalid="Boolean(draft.error)"
                :aria-errormessage="draft.error ? `description-error-${workspace.id}` : undefined"
                @keydown.meta.enter.prevent="saveDescription"
                @keydown.ctrl.enter.prevent="saveDescription"
                @keydown.esc.stop.prevent="cancelDescription"
              />
              <p v-if="draft.error" :id="`description-error-${workspace.id}`" class="status-error" role="alert">
                {{ draft.error }}
              </p>
              <footer>
                <span :id="`description-hint-${workspace.id}`" class="muted caption"
                  >Markdown · {{ shortcutLabel('⌘↩') }} to save · Esc to cancel</span
                ><button type="button" class="standard-button" :disabled="draft.saving" @click="cancelDescription">
                  Cancel</button
                ><button type="submit" class="standard-button primary" :disabled="draft.saving">
                  {{ draft.saving ? 'Saving…' : 'Save' }}
                </button>
              </footer>
            </form>
            <template v-else
              ><div
                v-if="content.bodyHTML || content.body"
                class="description-body markdown"
                v-html="content.bodyHTML ? sanitizeBody(content.bodyHTML) : renderMarkdown(content.body)"
              />
              <p v-else class="description-body muted">No description provided.</p>
              <button
                v-if="canEditDescription"
                ref="descriptionButton"
                class="text-link description-edit"
                aria-label="Edit description"
                @click="editDescription"
              >
                Edit
              </button></template
            >
          </div>
          <template v-if="content.can.comments">
            <h4>
              Comments <span class="muted">({{ content.comments.length }})</span>
            </h4>
            <p v-if="!content.comments.length" class="muted">No comments.</p>
            <article v-for="comment in content.comments" :key="comment.id">
              <header>
                <Avatar :author="comment.author" /><strong>{{ comment.author?.login || 'Deleted user' }}</strong
                ><time class="muted">{{ date(comment.createdAt) }}</time>
              </header>
              <div
                class="markdown"
                v-html="comment.bodyHTML ? sanitizeBody(comment.bodyHTML) : renderMarkdown(comment.body)"
              />
            </article>
          </template>
        </section>
      </div>
    </main>
  </section>

  <Teleport to=".app-shell"
    ><div
      v-if="itemMenu"
      class="context-menu"
      role="menu"
      :style="{ left: itemMenu.x + 'px', top: itemMenu.y + 'px' }"
      @pointerdown.stop
      @keydown="menuKeys"
    >
      <!-- prettier-ignore-attribute @click --><button
        role="menuitem"
        @click="openURL(itemMenu.item.url);dismissItemMenu()"
      >
        Open on {{ providerName(itemMenu.item.provider) }}</button
      ><!-- prettier-ignore-attribute @click --><button
        role="menuitem"
        @click="copy(itemMenu.item.url).catch(e=>emit('error',String(e)));dismissItemMenu()"
      >
        Copy Link
      </button>
    </div></Teleport
  >
  <Teleport to=".app-shell"
    ><Modal v-if="discarding" label="Discard Description Changes" @close="discarding = null"
      ><div class="close-dialog">
        <h2>Discard your changes?</h2>
        <p>Your edits to the description of {{ selected?.identifier }} have not been saved.</p>
        <footer>
          <button class="standard-button" autofocus @click="discarding = null">Keep Editing</button
          ><button class="standard-button destructive" data-default-action @click="confirmDiscard">
            Discard Changes
          </button>
        </footer>
      </div></Modal
    ></Teleport
  >
  <GitHubMetadataEditor
    v-if="editing && content && sourceOf(selected)"
    :workspace="workspace"
    :source="sourceOf(selected)"
    :item="content"
    :field="editing.field"
    :anchor="editing.anchor"
    :key="editing.field"
    @close="editing = null"
    @saved="metadataSaved"
    @busy="
      (value) => {
        if (editing) editing.saving = value
      }
    "
  />
</template>
