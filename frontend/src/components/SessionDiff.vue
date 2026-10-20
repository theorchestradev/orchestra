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
import {
  commentable,
  excerpt,
  lineNumber,
  lineSide,
  outdated,
  rangeLabel,
  reviewStatus,
  sessionReviews,
} from '../shared/review'
import { relativeDate } from '../shared/appearance'
import Icon from './Icon.vue'
import ReviewThread from './ReviewThread.vue'
const props = defineProps({ session: Object, width: Number, runs: { type: Array, default: () => [] } })
const emit = defineEmits(['close', 'resize', 'resized', 'state'])
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
const collapsed = (file) => overrides.value.get(file.id) ?? !commentCount(file)
function toggle(file, value = !collapsed(file)) {
  overrides.value.set(file.id, value)
  overrides.value = new Map(overrides.value)
  remembered().collapsed = overrides.value
}

// Review comments. Pending ones belong to the session; sent ones live in their Inbox request.
const pending = computed(() => props.session.reviewComments || [])
const reviews = computed(() => sessionReviews(props.runs, props.session.id))
const filesByPath = computed(() => new Map(files.value.map((f) => [f.id, f])))
const isOutdated = (comment) => outdated(comment, filesByPath.value.get(comment.path))
// Threads shown under the last line of their range, keyed by path, side and line.
const threads = computed(() => {
  const map = new Map()
  const add = (key, item) => map.set(key, [...(map.get(key) || []), item])
  for (const run of reviews.value)
    (run.comments || []).forEach((comment, index) => {
      const item = { comment, run, index, outdated: isOutdated(comment) }
      add(item.outdated ? comment.path : threadKey(comment.path, comment.side, comment.end), item)
    })
  for (const comment of pending.value) {
    const item = { comment, outdated: isOutdated(comment) }
    add(item.outdated ? comment.path : threadKey(comment.path, comment.side, comment.end), item)
  }
  return map
})
const threadKey = (path, side, n) => `${path}\n${side}\n${n}`
const lineThreads = (file, line) => threads.value.get(threadKey(file.id, lineSide(line), lineNumber(line))) || []
const commentCount = (file) =>
  pending.value.filter((c) => c.path === file.id).length +
  reviews.value.reduce((n, r) => n + (r.comments || []).filter((c) => c.path === file.id).length, 0)
const reviewOpen = ref(false),
  composer = ref(null),
  composerField = ref(null),
  reviewBusy = ref(false),
  reviewError = ref('')
let anchor = null,
  dragging = false
const selected = (file, line) =>
  composer.value?.path === file.id &&
  composer.value.side === lineSide(line) &&
  lineNumber(line) >= composer.value.start &&
  lineNumber(line) <= composer.value.end
const composerAt = (file, line) =>
  composer.value?.path === file.id && composer.value.side === lineSide(line) && composer.value.end === lineNumber(line)
const lineLabel = (line) =>
  `Line ${lineNumber(line)}, ${{ add: 'added', del: 'removed', context: 'unchanged' }[line.kind]}`
// Choosing a line opens the composer there; Shift extends the range on the same side of the file.
function pick(file, line, extend = false) {
  const side = lineSide(line),
    n = lineNumber(line)
  const current = composer.value
  if (current?.id && !(extend && current.path === file.id && current.side === side)) return focusComposer()
  if (extend && current?.path === file.id && current.side === side && anchor !== null) {
    composer.value = { ...current, start: Math.min(anchor, n), end: Math.max(anchor, n) }
  } else {
    anchor = n
    composer.value = { path: file.id, side, start: n, end: n, text: current?.text || '' }
  }
  focusComposer()
}
function focusComposer() {
  nextTick(() => composerField.value?.[0]?.focus?.() ?? composerField.value?.focus?.())
}
function startDrag(file, line, event) {
  if (event.button !== 0) return
  dragging = true
  pick(file, line, event.shiftKey)
  addEventListener('pointerup', () => (dragging = false), { once: true })
}
function dragOver(file, line) {
  if (dragging && composer.value?.path === file.id && composer.value.side === lineSide(line)) pick(file, line, true)
}
function edit(comment) {
  anchor = comment.start
  composer.value = { ...comment }
  const file = filesByPath.value.get(comment.path)
  if (file) toggle(file, false)
  focusComposer()
}
function cancelComposer() {
  const previous = composer.value
  composer.value = null
  anchor = null
  if (previous) nextTick(() => focusLine(previous.path, previous.side, previous.end))
}
async function review(method, ...args) {
  reviewBusy.value = true
  reviewError.value = ''
  try {
    const state = await call(method, ...args)
    emit('state', state)
    return state
  } catch (e) {
    reviewError.value = String(e)
  } finally {
    reviewBusy.value = false
  }
}
async function saveComment(sendNow = false) {
  const draft = composer.value
  if (!draft?.text.trim() || reviewBusy.value) return
  const file = filesByPath.value.get(draft.path)
  const before = new Set(pending.value.map((c) => c.id))
  const comment = {
    id: draft.id || '',
    path: draft.path,
    side: draft.side,
    start: draft.start,
    end: draft.end,
    excerpt: file ? excerpt(file.hunks, draft.side, draft.start, draft.end) : draft.excerpt || '',
    text: draft.text,
  }
  const state = await review('SaveReviewComment', props.session.id, comment)
  if (!state) return
  composer.value = null
  anchor = null
  if (!sendNow) return
  const saved = findSession(state)?.reviewComments?.find((c) => c.id === draft.id || !before.has(c.id))
  if (saved) await sendReview([saved.id])
}
const findSession = (state) => state.projects?.flatMap((p) => p.sessions).find((s) => s.id === props.session.id)
async function sendReview(ids = []) {
  if (await review('SendReview', props.session.id, ids)) reviewOpen.value = true
}
function jump(comment) {
  const file = filesByPath.value.get(comment.path)
  if (!file) return
  toggle(file, false)
  nextTick(() => focusLine(comment.path, comment.side, comment.end, true))
}
function focusLine(path, side, n, center = false) {
  const section = [...(scroller.value?.querySelectorAll('[data-path]') || [])].find((el) => el.dataset.path === path)
  const target = section?.querySelector(`[data-line="${side}:${n}"]`)
  if (!target) return
  if (center) target.scrollIntoView({ block: 'center' })
  target.focus({ preventScroll: center })
}
// On a line, Enter or C comments; Shift with Up or Down extends the composer's range.
function lineKeys(event, file, line) {
  if ((event.key === 'Enter' || event.key.toLowerCase() === 'c') && !event.metaKey && !event.ctrlKey) {
    event.preventDefault()
    if (selected(file, line)) focusComposer()
    else pick(file, line, event.shiftKey)
  }
}
const composerKeys = (event) => {
  if (event.key === 'Escape') {
    event.preventDefault()
    cancelComposer()
  } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    saveComment(event.shiftKey)
  }
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
const navigable = '.diff-file-toggle, .diff-hunk-header, .diff-line[tabindex]'
function navigate(event) {
  const target = event.target.closest?.(navigable)
  if (!target || event.target.closest('.review-composer, .review-thread')) return
  if (['ArrowLeft', 'ArrowRight'].includes(event.key) && target.classList.contains('diff-file-toggle')) {
    event.preventDefault()
    const file = files.value.find((f) => f.id === target.closest('[data-path]').dataset.path)
    if (file) toggle(file, event.key === 'ArrowLeft')
    return
  }
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const items = [...scroller.value.querySelectorAll(navigable)]
  const index = items.indexOf(target)
  const next =
    event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? items.length - 1
        : Math.max(0, Math.min(items.length - 1, index + (event.key === 'ArrowDown' ? 1 : -1)))
  // Shift with Up or Down selects lines on one side of a file; Enter or C then comments on them.
  const from = target.dataset.line?.split(':'),
    to = items[next]?.dataset.line?.split(':')
  const path = target.closest('[data-path]').dataset.path
  if (event.shiftKey && from && to && items[next].closest('[data-path]').dataset.path === path && from[0] === to[0]) {
    const current = composer.value
    if (current?.id) return items[next].focus()
    if (current?.path !== path || current.side !== from[0] || anchor === null) {
      anchor = +from[1]
      composer.value = { path, side: from[0], start: anchor, end: anchor, text: current?.text || '' }
    }
    composer.value = { ...composer.value, start: Math.min(anchor, +to[1]), end: Math.max(anchor, +to[1]) }
  }
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
    composer.value = null
    reviewError.value = ''
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
  <aside class="diff-pane" aria-label="Changes" :style="{ '--diff-pane-width': width + 'px' }">
    <div
      class="diff-resizer"
      role="separator"
      tabindex="0"
      aria-label="Resize Changes"
      aria-orientation="vertical"
      :aria-valuenow="width"
      :aria-valuemin="minDiffWidth"
      :aria-valuemax="maxDiffWidth"
      @pointerdown.prevent="drag"
      @keydown="resizeKeys"
    />
    <header class="diff-header">
      <div class="diff-title">
        <h2>Changes</h2>
        <span class="spacer" /><button
          class="icon-button"
          :class="{ spinning: loading }"
          aria-label="Refresh Changes"
          title="Refresh Changes"
          :disabled="loading"
          @click="refresh"
        >
          <Icon name="arrow.clockwise" :size="14" /></button
        ><button
          class="icon-button"
          aria-label="Hide Changes"
          :title="`Hide Changes (${shortcutLabel('⇧⌘B')})`"
          @click="emit('close')"
        >
          <Icon name="xmark" :size="12" />
        </button>
      </div>
      <p v-if="diff?.scope" class="muted caption truncate" :title="scopeDetail">{{ scopeLabel }}</p>
      <p v-if="diff?.scope" class="diff-totals caption" aria-live="polite">
        {{ files.length }} {{ files.length === 1 ? 'file' : 'files' }}
        <span class="diff-additions">+{{ diff.additions }}</span>
        <span class="diff-deletions">−{{ diff.deletions }}</span>
        <span v-if="diff.truncated" class="muted">· Some changes are too large to show</span>
      </p>
    </header>
    <section v-if="pending.length || reviews.length" class="review-bar" aria-label="Review">
      <div class="review-bar-row">
        <button class="review-bar-toggle" :aria-expanded="reviewOpen" @click="reviewOpen = !reviewOpen">
          <Icon :name="reviewOpen ? 'chevron.down' : 'chevron.right'" :size="9" /><Icon
            name="text.bubble"
            :size="13"
          /><span class="truncate">{{
            pending.length
              ? `${pending.length} pending ${pending.length === 1 ? 'comment' : 'comments'}`
              : reviewStatus(reviews[0])
          }}</span>
        </button>
        <button
          v-if="pending.length"
          class="standard-button primary caption"
          :disabled="reviewBusy"
          :title="`Send the pending comments to ${session.name}`"
          @click="sendReview()"
        >
          Send Comments
        </button>
      </div>
      <p v-if="reviewError" class="error caption" role="alert">{{ reviewError }}</p>
      <div v-if="reviewOpen" class="review-list">
        <ul v-if="pending.length" class="review-items" aria-label="Pending comments">
          <li v-for="comment in pending" :key="comment.id">
            <button class="review-item" @click="jump(comment)">
              <span class="review-item-place truncate"
                >{{ comment.path.split('/').pop() }} · {{ rangeLabel(comment) }}</span
              ><span class="review-item-text">{{ comment.text }}</span>
            </button>
          </li>
        </ul>
        <article v-for="run in reviews" :key="run.id" class="review-sent">
          <header>
            <strong
              >{{ run.comments?.length || 0 }} {{ run.comments?.length === 1 ? 'comment' : 'comments' }} ·
              {{ reviewStatus(run) }}</strong
            ><span class="muted caption">{{ relativeDate(run.createdAt) }}</span>
          </header>
          <ol class="review-items">
            <li v-for="(comment, index) in run.comments" :key="comment.id">
              <button class="review-item" @click="jump(comment)">
                <span class="review-item-place truncate"
                  >{{ index + 1 }}. {{ comment.path.split('/').pop() }} · {{ rangeLabel(comment) }}</span
                ><span class="review-item-text">{{ comment.text }}</span
                ><span v-if="comment.reply" class="review-item-reply">{{ comment.reply }}</span>
              </button>
            </li>
          </ol>
          <p v-if="run.response" class="review-item-reply">{{ run.response }}</p>
          <p v-if="run.detail" class="muted caption">{{ run.detail }}</p>
          <footer>
            <button
              v-if="['queued', 'running'].includes(run.status)"
              class="standard-button caption"
              :disabled="reviewBusy"
              @click="review('CancelRequest', run.id)"
            >
              Stop Waiting</button
            ><button
              v-else
              class="standard-button caption"
              :disabled="reviewBusy"
              @click="review('DismissReview', run.id)"
            >
              Dismiss
            </button>
          </footer>
        </article>
      </div>
    </section>
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
            ><span v-if="commentCount(file)" class="diff-comment-count" :aria-label="`${commentCount(file)} comments`"
              ><Icon name="text.bubble" :size="11" />{{ commentCount(file) }}</span
            ><span v-if="!file.binary" class="diff-counts"
              ><span class="diff-additions">+{{ file.additions }}</span
              ><span class="diff-deletions">−{{ file.deletions }}</span></span
            >
          </button>
        </h3>
        <template v-if="!collapsed(file)">
          <div v-if="threads.get(file.id)" class="review-outdated">
            <ReviewThread
              v-for="item in threads.get(file.id)"
              :key="(item.run?.id || '') + item.comment.id"
              v-bind="item"
              :busy="reviewBusy"
              @edit="edit"
              @delete="review('DeleteReviewComment', session.id, $event.id)"
            />
          </div>
          <p v-if="!file.hunks.length" class="diff-placeholder muted">{{ diffPlaceholder(file) }}</p>
          <div v-else class="diff-body">
            <div class="diff-lines">
              <template v-for="(hunk, index) in file.hunks" :key="index"
                ><div class="diff-hunk-header" tabindex="0" :aria-label="'Hunk ' + hunk.header">
                  {{ hunk.header }}
                </div>
                <template v-for="(line, n) in hunk.lines" :key="n"
                  ><div
                    class="diff-line"
                    :class="[line.kind, { selected: commentable(line) && selected(file, line) }]"
                    :tabindex="commentable(line) ? -1 : undefined"
                    :data-line="commentable(line) ? lineSide(line) + ':' + lineNumber(line) : undefined"
                    :aria-label="commentable(line) ? lineLabel(line) + ': ' + line.text : undefined"
                    @keydown="commentable(line) && lineKeys($event, file, line)"
                    @pointerenter="commentable(line) && dragOver(file, line)"
                  >
                    <span class="diff-number">{{ line.old ?? '' }}</span
                    ><span class="diff-number"
                      >{{ line.new ?? ''
                      }}<button
                        v-if="commentable(line)"
                        class="diff-comment-button"
                        tabindex="-1"
                        :aria-label="'Comment on ' + lineLabel(line)"
                        title="Comment (Shift-click to extend)"
                        @pointerdown.prevent="startDrag(file, line, $event)"
                      >
                        <Icon name="plus" :size="10" /></button></span
                    ><span class="diff-marker">{{ markers[line.kind] }}</span
                    ><span class="diff-text">{{ line.text }}</span>
                  </div>
                  <template v-if="commentable(line)"
                    ><ReviewThread
                      v-for="item in lineThreads(file, line)"
                      :key="(item.run?.id || '') + item.comment.id"
                      v-bind="item"
                      :busy="reviewBusy"
                      @edit="edit"
                      @delete="review('DeleteReviewComment', session.id, $event.id)"
                    />
                    <div
                      v-if="composerAt(file, line)"
                      class="review-composer"
                      role="group"
                      :aria-label="'Comment on ' + rangeLabel(composer)"
                    >
                      <header>
                        <strong>{{ composer.id ? 'Edit comment on' : 'Comment on' }} {{ rangeLabel(composer) }}</strong
                        ><span v-if="!composer.id" class="muted caption">Shift-click a line to extend</span>
                      </header>
                      <textarea
                        ref="composerField"
                        v-model="composer.text"
                        aria-label="Comment"
                        placeholder="Leave a comment for the agent"
                        rows="3"
                        @keydown="composerKeys"
                      />
                      <footer>
                        <span class="muted caption">{{ shortcutLabel('⌘↩') }} to add</span><span class="spacer" />
                        <button class="standard-button" @click="cancelComposer">Cancel</button
                        ><button
                          v-if="!composer.id"
                          class="standard-button"
                          :disabled="reviewBusy || !composer.text.trim()"
                          :title="`Send only this comment to ${session.name} now`"
                          @click="saveComment(true)"
                        >
                          Send Now</button
                        ><button
                          class="standard-button primary"
                          :disabled="reviewBusy || !composer.text.trim()"
                          @click="saveComment()"
                        >
                          {{ composer.id ? 'Save' : 'Add Comment' }}
                        </button>
                      </footer>
                    </div></template
                  ></template
                ></template
              >
            </div>
          </div>
        </template>
        <p v-else-if="file.binary || file.truncated" class="diff-placeholder muted">{{ diffPlaceholder(file) }}</p>
      </section>
    </div>
  </aside>
</template>
