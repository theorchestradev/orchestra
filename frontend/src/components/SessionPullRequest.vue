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
  pullRequestActivity,
  refreshDelay,
  stateIcons,
  stateLabels,
} from '../shared/pullRequest'
import Avatar from './Avatar.vue'
import ConflictNotice from './ConflictNotice.vue'
import Icon from './Icon.vue'
import PullRequestChecks from './PullRequestChecks.vue'
const props = defineProps({ session: Object, defaultAgent: String, profiles: Array, profileId: String })
const emit = defineEmits(['resolve'])
const error = ref(''),
  loading = ref(false),
  activityOpen = ref(true),
  copied = ref('')
const status = computed(() => pullRequests.get(props.session.id))
const activity = computed(() => pullRequestActivity(status.value))
const counts = computed(() => activityCounts(activity.value))
const conflict = computed(
  () =>
    status.value?.conflicts && {
      number: status.value.number,
      title: status.value.title,
      url: status.value.url,
      base: status.value.baseBranch,
      head: status.value.branch,
      forkWithoutEdits: status.value.forkWithoutEdits,
    },
)
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
function flash(key) {
  copied.value = key
  clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => (copied.value = ''), 1500)
}
const loadFailure = (jobID) => call('PullRequestCheckFailure', props.session.id, jobID)
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
watch(
  () => props.session.id,
  () => {
    generation++
    error.value = ''
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
      <ConflictNotice
        v-if="conflict"
        :conflict="conflict"
        :default-agent="defaultAgent"
        :profiles="profiles"
        :profile-id="profileId"
        visible
        @resolve="emit('resolve', conflict, $event)"
      />
      <PullRequestChecks :key="session.id" :status="status" :load-failure="loadFailure" @error="error = $event" />
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
