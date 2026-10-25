<script setup>
import { computed, ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { call, copy, openURL } from '../shared/api'
import { relativeDate } from '../shared/appearance'
import { canWorkOn, descriptionSource, isPermissionError, itemSessions, pullConflict } from '../shared/github'
import { sanitize } from '../shared/markdown'
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
  project: Object,
  visible: Boolean,
  refreshInterval: Number,
  defaultAgent: String,
  profiles: Array,
})
const emit = defineEmits(['work', 'resolve', 'session', 'error'])
const detailElement = ref(null),
  itemMenu = ref(null),
  editing = ref(null),
  descriptionEditor = ref(null),
  descriptionButton = ref(null)
const state = ref('open'),
  pull = ref(false),
  search = ref(''),
  items = ref([]),
  loading = ref(false),
  error = ref(''),
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
const visibleItems = computed(() =>
  items.value.filter(
    (item) =>
      Boolean(item.pull_request) === pull.value &&
      (!search.value ||
        item.title.toLowerCase().includes(search.value.toLowerCase()) ||
        ('#' + item.number).includes(search.value)),
  ),
)
const status = (item) =>
  item.merged || item.pull_request?.merged_at
    ? 'Merged'
    : item.state === 'closed'
      ? 'Closed'
      : item.draft
        ? 'Draft'
        : 'Open'
const statusIcon = (item) =>
  item.state === 'open'
    ? 'open'
    : item.state_reason === 'not_planned'
      ? 'not-planned'
      : item.state_reason === 'duplicate'
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
  if (background !== true) {
    loading.value = true
    items.value = []
  }
  error.value = ''
  try {
    const result = await call('GitHubItems', props.project.id, state.value)
    if (token === request) items.value = result
  } catch (e) {
    if (token === request) error.value = String(e)
  } finally {
    if (token === request) loading.value = false
  }
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
    const result = await call('GitHubDetail', props.project.id, item.number, Boolean(item.pull_request))
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
  if (!item.pull_request) return
  try {
    const result = await call('GitHubPullRequest', props.project.id, item.number)
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
  if (props.visible && selected.value?.pull_request)
    checksTimer = setTimeout(
      () => (document.hidden ? scheduleChecks() : loadChecks(selected.value, true)),
      refreshDelay(pullStatus.value),
    )
}
const loadFailure = (jobID) => call('GitHubCheckFailure', props.project.id, jobID)
function back() {
  if (editing.value || draft.value?.saving) return
  discardDraft(() => {
    selected.value = null
    detailRequest++
    loadChecks({})
  })
}
// The draft keeps the source it started from so saving can detect edits made on GitHub.
const draftChanged = computed(() => draft.value && draft.value.text !== draft.value.source)
const conflict = computed(() => selected.value?.pull_request && pullConflict(content.value?.detail))
const canEditDescription = computed(
  () => content.value && !readOnly.value.has(`${Boolean(selected.value.pull_request)}#${selected.value.number}`),
)
async function editDescription() {
  draft.value = {
    original: content.value.detail.body ?? '',
    source: descriptionSource(content.value.detail),
    text: descriptionSource(content.value.detail),
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
    await call('UpdateGitHubMetadata', props.project.id, item.number, Boolean(item.pull_request), 'body', [
      current.text,
      current.original,
    ])
  } catch (e) {
    current.error = String(e)
    if (isPermissionError(e)) readOnly.value.add(`${Boolean(item.pull_request)}#${item.number}`)
    return
  } finally {
    current.saving = false
  }
  if (draft.value === current) await closeDescription()
  await Promise.all([selected.value === item && detail(item, true), load(true)])
}
function editMetadata(field, event) {
  if (editing.value?.saving) return
  editing.value = editing.value?.field === field ? null : { field, anchor: event.currentTarget }
}
async function metadataSaved(patch) {
  const anchor = editing.value?.anchor
  editing.value = null
  Object.assign(content.value.detail, patch)
  const row = items.value.find((i) => i.number === selected.value.number)
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
    } else if (selected.value?.pull_request) loadChecks(selected.value, true)
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
const detailURL = computed(() => content.value?.detail.html_url || selected.value?.html_url)
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
  () => [props.visible, props.refreshInterval],
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
  clearInterval(refreshTimer)
  clearTimeout(checksTimer)
  checksRequest++
})
watch(state, () => load())
onMounted(() => load())
</script>
<template>
  <section v-if="!selected" class="github-page">
    <header class="github-header">
      <div class="github-heading">
        <div>
          <h2>Issues &amp; PRs</h2>
          <button class="text-link muted caption" @click="openURL('https://github.com/' + project.repository)">
            {{ project.repository }}
          </button>
        </div>
        <button class="standard-button muted" :disabled="loading" @click="load">
          <Icon name="arrow.clockwise" :size="14" />Refresh
        </button>
      </div>
      <div class="github-filters">
        <div class="type-tabs" role="tablist" aria-label="Item type">
          <button
            v-for="type in [false, true]"
            :key="String(type)"
            role="tab"
            :aria-selected="pull === type"
            :class="{ active: pull === type }"
            @click="pull = type"
          >
            <Icon :name="type ? 'arrow.triangle.branch' : 'circle.dotted'" :size="14" />{{
              type ? 'Pull requests' : 'Issues'
            }}<span class="count">{{
              loading ? '—' : items.filter((i) => Boolean(i.pull_request) === type).length
            }}</span>
          </button>
        </div>
        <label class="search-field"
          ><Icon name="magnifyingglass" :size="14" /><input
            v-model="search"
            aria-label="Search issues and PRs"
            placeholder="Search by title or number" /><button
            v-if="search"
            class="icon-button"
            aria-label="Clear Search"
            @click="search = ''"
          >
            <Icon name="xmark.circle.fill" :size="12" /></button></label
        ><label class="status-filter"
          ><Icon name="line.3.horizontal.decrease" :size="14" /><select v-model="state" aria-label="Status">
            <option value="open">Open</option>
            <option value="closed">Closed</option>
            <option value="all">All</option></select
          ><Icon name="chevron.down" :size="9"
        /></label>
      </div>
    </header>
    <div v-if="loading" class="empty-state">
      <span class="spinner" />
      <p class="muted">Loading issues and PRs…</p>
    </div>
    <div v-else-if="error" class="empty-state github-unavailable">
      <Icon name="exclamationmark.triangle" :size="42" />
      <h2>Could not load GitHub</h2>
      <p>{{ error }}</p>
      <button class="standard-button" @click="load">Try Again</button>
    </div>
    <div v-else-if="!visibleItems.length" class="empty-state github-unavailable">
      <Icon :name="pull ? 'arrow.triangle.branch' : 'circle.dotted'" :size="42" />
      <h2>{{ pull ? 'No Pull Requests' : 'No Issues' }}</h2>
      <p>{{ search ? 'No items match your search.' : 'There are no items with this status.' }}</p>
    </div>
    <div v-else class="github-table">
      <div class="github-table-head">
        <span>ID</span><span>TITLE</span><span class="wide-column"><span class="visually-hidden">Sessions</span></span
        ><span class="wide-column">ASSIGNEES</span><span>STATUS</span><span class="wide-column">UPDATED</span>
      </div>
      <div class="github-rows">
        <div
          v-for="item in visibleItems"
          :key="item.number"
          class="github-row"
          @click="detail(item)"
          @contextmenu="showItemMenu($event, item)"
        >
          <span class="item-id"
            ><Icon :name="item.pull_request ? 'arrow.triangle.branch' : 'circle.dotted'" :size="13" />#{{
              item.number
            }}</span
          ><span class="item-context"
            ><button class="item-title-line item-open">
              <strong :title="item.title">{{ item.title }}</strong
              ><span class="muted caption item-byline"
                >by {{ item.user?.login || 'Deleted user'
                }}<span class="compact-date"> · {{ relativeDate(item.updated_at) }}</span></span
              ></button
            ><span v-if="item.labels?.length" class="labels"
              ><span v-for="label in item.labels.slice(0, 3)" :key="label.name" class="label-chip"
                ><i
                  :style="{ background: /^[0-9a-f]{6}$/i.test(label.color) ? '#' + label.color : 'var(--muted)' }"
                /><span>{{ label.name }}</span></span
              ><span v-if="item.labels.length > 3" class="muted caption">+{{ item.labels.length - 3 }}</span></span
            ><span
              v-if="item.assignees?.length || itemSessions(project, item).length"
              class="compact-assignees assignees"
              ><Avatar v-for="assignee in item.assignees?.slice(0, 3)" :key="assignee.login" :author="assignee" /><span
                v-if="item.assignees?.length > 3"
                class="muted caption"
                >+{{ item.assignees.length - 3 }}</span
              ><GitHubSessions :sessions="itemSessions(project, item)" @open="emit('session', $event)" /></span></span
          ><span class="wide-column"
            ><GitHubSessions :sessions="itemSessions(project, item)" @open="emit('session', $event)" /></span
          ><span class="wide-column assignees"
            ><Avatar v-for="assignee in item.assignees?.slice(0, 3)" :key="assignee.login" :author="assignee" /><span
              v-if="item.assignees?.length > 3"
              >+{{ item.assignees.length - 3 }}</span
            ><span v-if="!item.assignees?.length" class="muted">—</span></span
          ><span class="status-badge" :class="status(item).toLowerCase()"
            ><Icon
              :name="
                {
                  Open: 'circle.dotted',
                  Closed: 'checkmark.circle',
                  Merged: 'arrow.triangle.merge',
                  Draft: 'circle.dashed',
                }[status(item)]
              "
              :size="12"
            />{{ status(item) }}</span
          ><span class="wide-column muted caption">{{ relativeDate(item.updated_at) }}</span
          ><Icon class="compact-chevron" name="chevron.right" :size="8" />
        </div>
      </div>
    </div>
  </section>
  <section v-else ref="detailElement" class="github-detail">
    <header class="detail-toolbar">
      <button class="text-link muted" @click="back"><Icon name="chevron.left" :size="12" />GitHub list</button
      ><span class="muted">/</span><span class="truncate repository">{{ project.repository }}</span
      ><button
        class="text-link item-number"
        :aria-label="`Open #${selected.number} on GitHub`"
        @click="openURL(detailURL)"
        @contextmenu="showItemMenu($event, { ...selected, html_url: detailURL })"
        @keydown.shift.f10="showItemMenu($event, { ...selected, html_url: detailURL })"
        @keydown.context-menu="showItemMenu($event, { ...selected, html_url: detailURL })"
      >
        #{{ selected.number }}
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
          :class="{ 'without-work-on': !canWorkOn(content.detail) && !itemSessions(project, selected).length }"
        >
          <div v-if="canWorkOn(content.detail) || itemSessions(project, selected).length" class="work-on">
            <GitHubSessions :sessions="itemSessions(project, selected)" @open="emit('session', $event)" /><WorkOnButton
              v-if="canWorkOn(content.detail)"
              :label="selected.pull_request ? 'Review' : 'Work on'"
              :default-agent="defaultAgent"
              :profiles="profiles"
              :profile-id="project.profileID || ''"
              :visible="visible"
              @work="emit('work', selected, $event)"
            />
          </div>
          <div class="issue-title-line">
            <h1>
              {{ content.detail.title }} <span class="muted">#{{ content.detail.number }}</span>
            </h1>
            <span class="muted caption detail-byline"
              >by {{ content.detail.user?.login || 'Deleted user' }} · updated
              {{ relativeDate(content.detail.updated_at || content.detail.created_at) }}</span
            >
          </div>
        </section>
        <section class="metadata">
          <div>
            <h3>STATUS</h3>
            <button
              class="metadata-trigger status-badge"
              :class="status(content.detail).toLowerCase()"
              aria-label="Edit status"
              :disabled="status(content.detail) === 'Merged'"
              :title="
                status(content.detail) === 'Merged' ? 'A merged pull request cannot change status.' : 'Edit status'
              "
              :aria-expanded="editing?.field === 'state'"
              aria-haspopup="menu"
              @click="editMetadata('state', $event)"
            >
              <Icon v-if="status(content.detail) === 'Merged'" name="arrow.triangle.merge" :size="14" /><Icon
                v-else-if="statusIcon(content.detail) === 'duplicate'"
                name="doc.on.doc"
                :size="14"
              /><span v-else class="status-choice-icon" :class="statusIcon(content.detail)" aria-hidden="true" />{{
                status(content.detail)
              }}<Icon v-if="status(content.detail) !== 'Merged'" name="chevron.down" :size="9" />
            </button>
          </div>
          <div>
            <h3>ASSIGNEES</h3>
            <button
              class="metadata-trigger metadata-people"
              aria-label="Edit assignees"
              :title="
                'Edit assignees' +
                (content.detail.assignees?.length ? ': ' + content.detail.assignees.map((a) => a.login).join(', ') : '')
              "
              :aria-expanded="editing?.field === 'assignees'"
              @click="editMetadata('assignees', $event)"
            >
              <span v-if="!content.detail.assignees?.length" class="muted">Unassigned</span
              ><Avatar
                v-for="assignee in content.detail.assignees?.slice(0, 3)"
                :key="assignee.login"
                :author="assignee"
              /><span v-if="content.detail.assignees?.length > 3" class="caption"
                >+{{ content.detail.assignees.length - 3 }}</span
              ><Icon name="chevron.down" :size="9" />
            </button>
          </div>
          <div>
            <h3>LABELS</h3>
            <button
              class="metadata-trigger labels"
              aria-label="Edit labels"
              title="Edit labels"
              :aria-expanded="editing?.field === 'labels'"
              aria-haspopup="dialog"
              @click="editMetadata('labels', $event)"
            >
              <span v-if="!content.detail.labels?.length" class="muted">No labels</span
              ><span v-for="label in content.detail.labels" :key="label.name" class="label-chip"
                ><i
                  :style="{ background: /^[0-9a-f]{6}$/i.test(label.color) ? '#' + label.color : 'var(--muted)' }"
                /><span>{{ label.name }}</span></span
              ><Icon name="chevron.down" :size="9" />
            </button>
          </div>
        </section>
        <div v-if="content.detail.head" class="branches">
          <code>{{ content.detail.head.label }}</code
          ><span class="muted">→</span><code>{{ content.detail.base.label }}</code
          ><span class="muted">{{ content.detail.changed_files }} files changed</span
          ><span class="status-open">+{{ content.detail.additions }}</span
          ><span class="status-error">−{{ content.detail.deletions }}</span>
        </div>
        <ConflictNotice
          v-if="conflict"
          :conflict="conflict"
          :default-agent="defaultAgent"
          :profiles="profiles"
          :profile-id="project.profileID || ''"
          :visible="visible"
          :read-only="readOnly.has(`true#${selected.number}`)"
          @resolve="emit('resolve', conflict, $event)"
        />
        <template v-if="selected.pull_request">
          <PullRequestChecks
            v-if="pullStatus"
            :key="selected.number"
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
                ref="descriptionEditor"
                v-model="draft.text"
                class="description-editor"
                aria-label="Description"
                :aria-describedby="`description-hint-${project.id}`"
                spellcheck="true"
                :readonly="draft.saving"
                :aria-invalid="Boolean(draft.error)"
                :aria-errormessage="draft.error ? `description-error-${project.id}` : undefined"
                @keydown.meta.enter.prevent="saveDescription"
                @keydown.ctrl.enter.prevent="saveDescription"
                @keydown.esc.stop.prevent="cancelDescription"
              />
              <p v-if="draft.error" :id="`description-error-${project.id}`" class="status-error" role="alert">
                {{ draft.error }}
              </p>
              <footer>
                <span :id="`description-hint-${project.id}`" class="muted caption"
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
                v-if="content.detail.body_html"
                class="description-body markdown"
                v-html="sanitizeBody(content.detail.body_html)"
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
          <h4>
            Comments <span class="muted">({{ content.comments.length }})</span>
          </h4>
          <p v-if="!content.comments.length" class="muted">No comments.</p>
          <article v-for="comment in content.comments" :key="comment.id">
            <header>
              <Avatar :author="comment.user" /><strong>{{ comment.user?.login || 'Deleted user' }}</strong
              ><time class="muted">{{ date(comment.created_at) }}</time>
            </header>
            <div class="markdown" v-html="sanitizeBody(comment.body_html)" />
          </article>
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
        @click="openURL(itemMenu.item.html_url);dismissItemMenu()"
      >
        Open on GitHub</button
      ><!-- prettier-ignore-attribute @click --><button
        role="menuitem"
        @click="copy(itemMenu.item.html_url).catch(e=>emit('error',String(e)));dismissItemMenu()"
      >
        Copy Link
      </button>
    </div></Teleport
  >
  <Teleport to=".app-shell"
    ><Modal v-if="discarding" label="Discard Description Changes" @close="discarding = null"
      ><div class="close-dialog">
        <h2>Discard your changes?</h2>
        <p>Your edits to the description of #{{ selected?.number }} have not been saved.</p>
        <footer>
          <button class="standard-button" autofocus @click="discarding = null">Keep Editing</button
          ><button class="standard-button destructive" @click="confirmDiscard">Discard Changes</button>
        </footer>
      </div></Modal
    ></Teleport
  >
  <GitHubMetadataEditor
    v-if="editing && content"
    :project="project"
    :item="content.detail"
    :pull="Boolean(selected.pull_request)"
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
