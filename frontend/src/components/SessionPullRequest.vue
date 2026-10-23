<script>
import { reactive } from 'vue'
import { call } from '../shared/api'
// The latest pull request status per session, so the tab label and a reopened pane show it
// at once. `loadPullRequest` shares one request per session.
export const pullRequests = reactive(new Map())
const inFlight = new Map()
export function loadPullRequest(id, activity = false) {
  const key = id + (activity ? ':activity' : '')
  if (!inFlight.has(key))
    inFlight.set(
      key,
      call('SessionPullRequest', id, activity)
        .then((result) => {
          const previous = pullRequests.get(id)
          // A status-only read keeps the activity already shown for the same pull request.
          if (!activity && previous?.number === result.number)
            result = { ...result, reviews: previous.reviews, comments: previous.comments }
          pullRequests.set(id, result)
          return result
        })
        .finally(() => inFlight.delete(key)),
    )
  return inFlight.get(key)
}
</script>
<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { copy, openURL } from '../shared/api'
import { relativeDate } from '../shared/appearance'
import { sanitize } from '../shared/markdown'
import {
  activityCounts,
  activityText,
  checksSummary,
  failureContext,
  pullRequestActivity,
  refreshDelay,
  ringSegments,
  stateIcons,
  stateLabels,
} from '../shared/pullRequest'
import Avatar from './Avatar.vue'
import Icon from './Icon.vue'
const props = defineProps({ session: Object })
const error = ref(''),
  loading = ref(false),
  checksOpen = ref(true),
  activityOpen = ref(true),
  closedGroups = ref(new Set()),
  copied = ref(''),
  copying = ref('')
const status = computed(() => pullRequests.get(props.session.id))
const summary = computed(() => checksSummary(status.value?.checks))
const segments = computed(() => ringSegments(status.value?.checks))
const activity = computed(() => pullRequestActivity(status.value))
const counts = computed(() => activityCounts(activity.value))
const failures = computed(() => (status.value?.checks || []).filter((check) => check.status === 'failure'))
let generation = 0,
  pollTimer,
  activityTimer,
  copiedTimer
async function refresh() {
  const id = props.session.id,
    current = ++generation
  clearTimeout(pollTimer)
  loading.value = true
  try {
    await loadPullRequest(id, true)
    if (current === generation) error.value = ''
  } catch (e) {
    if (current === generation) error.value = String(e)
  } finally {
    if (current === generation) {
      loading.value = false
      schedule()
    }
  }
}
// Polling only runs while this tab is shown, and waits while the window is hidden.
function schedule() {
  clearTimeout(pollTimer)
  pollTimer = setTimeout(() => (document.hidden || loading.value ? schedule() : refresh()), refreshDelay(status.value))
}
function toggleGroup(group) {
  const next = new Set(closedGroups.value)
  next.has(group.status) ? next.delete(group.status) : next.add(group.status)
  closedGroups.value = next
}
function flash(key) {
  copied.value = key
  clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => (copied.value = ''), 1500)
}
const checkKey = (check) => (check.workflow || '') + '/' + check.name
// Copies what an agent needs to fix the failures. Job details are fetched only now, because
// logs are large and change with every run.
async function copyFailures(checks, key) {
  if (copying.value) return
  copying.value = key
  error.value = ''
  try {
    const details = new Map()
    for (const check of checks.slice(0, 5))
      if (check.jobID) details.set(check, await call('PullRequestCheckFailure', props.session.id, check.jobID))
    await copy(failureContext(status.value, checks, details))
    flash(key)
  } catch (e) {
    error.value = String(e)
  } finally {
    copying.value = ''
  }
}
async function copyActivity(item) {
  await copy(activityText(status.value, item))
  flash(item.id)
}
const sanitizeBody = (html) => sanitize(html || '')
function linkClick(event) {
  const link = event.target.closest('a')
  if (link) {
    event.preventDefault()
    openURL(link.href)
  }
}
// Arcs for the summary ring, drawn on a circle of circumference 100 with small gaps.
const ringArc = (segment) => {
  const gap = segments.value.length > 1 ? 3 : 0,
    length = Math.max(segment.size * 100 - gap, 1.5)
  return { dasharray: `${length} ${100 - length}`, dashoffset: 25 - segment.start * 100 }
}
watch(
  () => props.session.id,
  () => {
    generation++
    error.value = ''
    closedGroups.value = new Set()
    refresh()
  },
  { immediate: true },
)
// A finished agent turn may have pushed commits, which starts new checks.
watch([() => props.session.activity, () => props.session.running], () => {
  clearTimeout(activityTimer)
  activityTimer = setTimeout(refresh, 400)
})
onBeforeUnmount(() => {
  generation++
  clearTimeout(pollTimer)
  clearTimeout(activityTimer)
  clearTimeout(copiedTimer)
})
defineExpose({ refresh, loading })
</script>
<template>
  <div class="session-pull-request">
    <div v-if="error && !status?.number" class="diff-empty" role="alert">
      <Icon name="exclamationmark.triangle" :size="28" />
      <p>{{ error }}</p>
      <button class="standard-button" @click="refresh">Try Again</button>
    </div>
    <div v-else-if="!status" class="diff-empty"><span class="spinner" /></div>
    <div v-else-if="!status.number" class="diff-empty">
      <Icon name="arrow.triangle.pull" :size="28" />
      <p>{{ status.message }}</p>
    </div>
    <div v-else class="pr-scroller" @click="linkClick">
      <p v-if="error" class="diff-placeholder" role="alert">{{ error }}</p>
      <section class="pr-header">
        <button class="text-link muted caption pr-view" @click="openURL(status.url)">
          <Icon name="arrow.up.right.square" :size="12" />View on GitHub
        </button>
        <h2>
          {{ status.title }} <span class="muted">#{{ status.number }}</span>
        </h2>
        <p class="pr-meta caption">
          <span class="status-badge" :class="status.state"
            ><Icon :name="stateIcons[status.state]" :size="12" />{{ stateLabels[status.state] }}</span
          ><span class="muted truncate" :title="status.repository + ' · ' + status.branch"
            >{{ status.repository }} · {{ status.branch }}</span
          >
        </p>
      </section>
      <section class="pr-checks" aria-label="Checks">
        <div class="pr-checks-summary" :class="'outcome-' + summary.outcome">
          <button class="pr-summary-toggle" :aria-expanded="checksOpen" @click="checksOpen = !checksOpen">
            <svg class="pr-ring" viewBox="0 0 36 36" aria-hidden="true">
              <circle class="pr-ring-track" cx="18" cy="18" r="15.915" />
              <circle
                v-for="segment in segments"
                :key="segment.status"
                :class="'pr-ring-' + segment.status"
                cx="18"
                cy="18"
                r="15.915"
                :stroke-dasharray="ringArc(segment).dasharray"
                :stroke-dashoffset="ringArc(segment).dashoffset"
              />
            </svg>
            <span class="pr-summary-text"
              ><strong>{{ summary.headline }}</strong
              ><span class="muted caption">{{ summary.detail }}</span></span
            ><Icon v-if="status.checks.length" :name="checksOpen ? 'chevron.up' : 'chevron.down'" :size="10" />
          </button>
          <button
            v-if="failures.length"
            class="standard-button pr-copy-all"
            :disabled="Boolean(copying)"
            :aria-label="`Copy context for ${failures.length} failing ${failures.length === 1 ? 'check' : 'checks'}`"
            @click="copyFailures(failures, 'all')"
          >
            <Icon :name="copied === 'all' ? 'checkmark' : 'doc.on.doc'" :size="12" />{{
              copying === 'all' ? 'Copying…' : copied === 'all' ? 'Copied' : 'Copy All Failures'
            }}
          </button>
        </div>
        <div v-if="checksOpen && status.checks.length" class="pr-check-list">
          <template v-for="group in summary.groups" :key="group.status">
            <button
              class="pr-group-toggle muted caption"
              :aria-expanded="!closedGroups.has(group.status)"
              @click="toggleGroup(group)"
            >
              <Icon :name="closedGroups.has(group.status) ? 'chevron.right' : 'chevron.down'" :size="9" />{{
                group.title
              }}
            </button>
            <ul v-if="!closedGroups.has(group.status)">
              <li v-for="check in group.checks" :key="checkKey(check)" class="pr-check">
                <span class="pr-check-status" :class="'check-' + check.status"
                  ><Icon :name="group.icon" :size="14" /><span class="visually-hidden">{{ group.label }}:</span></span
                ><button
                  class="pr-check-name"
                  :title="check.url ? 'Open ' + check.name + ' on GitHub' : check.name"
                  :disabled="!check.url"
                  @click="openURL(check.url)"
                >
                  <span class="truncate">{{ check.name }}</span
                  ><span v-if="check.workflow" class="muted truncate pr-workflow">{{ check.workflow }}</span>
                </button>
                <button
                  v-if="check.status === 'failure'"
                  class="icon-button pr-copy"
                  :disabled="Boolean(copying)"
                  :aria-label="`Copy failure context for ${check.name}`"
                  :title="copied === checkKey(check) ? 'Copied' : 'Copy failure context'"
                  @click="copyFailures([check], checkKey(check))"
                >
                  <span v-if="copying === checkKey(check)" class="spinner small" /><Icon
                    v-else
                    :name="copied === checkKey(check) ? 'checkmark' : 'doc.on.doc'"
                    :size="12"
                  />
                </button>
                <span v-if="check.duration" class="muted caption pr-duration">{{
                  check.status === 'pending' ? 'running ' + check.duration : check.duration
                }}</span>
              </li>
            </ul>
          </template>
        </div>
      </section>
      <section class="pr-activity" aria-label="Activity">
        <button
          class="pr-group-toggle muted caption"
          :aria-expanded="activityOpen"
          @click="activityOpen = !activityOpen"
        >
          <Icon :name="activityOpen ? 'chevron.down' : 'chevron.right'" :size="9" />Activity<span
            v-if="counts.approved"
            class="pr-pill approved"
            >{{ counts.approved }} approved</span
          ><span v-if="counts.changes" class="pr-pill changes">{{ counts.changes }} changes requested</span
          ><span v-if="counts.comments" class="pr-pill"
            >{{ counts.comments }} {{ counts.comments === 1 ? 'comment' : 'comments' }}</span
          >
        </button>
        <template v-if="activityOpen">
          <div v-if="!status.reviews && !status.comments" class="diff-placeholder"><span class="spinner" /></div>
          <p v-else-if="!activity.length" class="diff-placeholder muted">No activity yet</p>
          <template v-else>
            <article v-for="item in activity" :key="item.id" class="pr-activity-item">
              <header>
                <Avatar :author="item.author" /><strong class="truncate">{{
                  item.author?.login || 'Deleted user'
                }}</strong
                ><span class="pr-review-state caption" :class="item.state?.toLowerCase()">{{ item.label }}</span
                ><time class="muted caption" :datetime="item.date" :title="item.date">{{
                  relativeDate(item.date)
                }}</time
                ><span class="spacer" /><button
                  class="icon-button"
                  :aria-label="`Copy ${item.label.toLowerCase()} by ${item.author?.login || 'Deleted user'}`"
                  :title="copied === item.id ? 'Copied' : 'Copy'"
                  @click="copyActivity(item)"
                >
                  <Icon :name="copied === item.id ? 'checkmark' : 'doc.on.doc'" :size="12" /></button
                ><button
                  v-if="item.url"
                  class="icon-button"
                  aria-label="Open on GitHub"
                  title="Open on GitHub"
                  @click="openURL(item.url)"
                >
                  <Icon name="arrow.up.right.square" :size="12" />
                </button>
              </header>
              <div v-if="item.html" class="markdown" v-html="sanitizeBody(item.html)" />
            </article>
          </template>
        </template>
      </section>
    </div>
  </div>
</template>
