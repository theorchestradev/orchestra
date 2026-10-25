<script setup>
import { computed, onBeforeUnmount, ref } from 'vue'
import { copy, openURL } from '../shared/api'
import { checksSummary, failureContext, ringSegments } from '../shared/pullRequest'
import Icon from './Icon.vue'
// The checks of a pull request, in the session's PR tab and the GitHub view. `status` comes
// from SessionPullRequest or GitHubPullRequest; `loadFailure` reads a failed job's context.
const props = defineProps({ status: Object, loadFailure: Function })
const emit = defineEmits(['error'])
const maxJobs = 5
const checksOpen = ref(true),
  closedGroups = ref(new Set()),
  copied = ref(''),
  copying = ref('')
const summary = computed(() => checksSummary(props.status.checks))
const segments = computed(() => ringSegments(props.status.checks))
const failures = computed(() => props.status.checks.filter((check) => check.status === 'failure'))
let copiedTimer
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
  emit('error', '')
  try {
    const details = new Map()
    for (const check of checks.slice(0, maxJobs))
      if (check.jobID) details.set(check, await props.loadFailure(check.jobID))
    await copy(failureContext(props.status, checks, details, maxJobs))
    flash(key)
  } catch (e) {
    emit('error', String(e))
  } finally {
    copying.value = ''
  }
}
// Arcs for the summary ring, drawn on a circle of circumference 100 with small gaps.
const ringArc = (segment) => {
  const gap = segments.value.length > 1 ? 3 : 0,
    length = Math.max(segment.size * 100 - gap, 1.5)
  return { dasharray: `${length} ${100 - length}`, dashoffset: 25 - segment.start * 100 }
}
onBeforeUnmount(() => clearTimeout(copiedTimer))
</script>
<template>
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
          <Icon :name="closedGroups.has(group.status) ? 'chevron.right' : 'chevron.down'" :size="9" />{{ group.title }}
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
</template>
