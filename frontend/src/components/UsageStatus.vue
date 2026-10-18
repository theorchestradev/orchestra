<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { call } from '../shared/api'
import { agents, agentIcon } from '../shared/appearance'
import { menuKeys } from '../shared/menu'
import {
  constrainedWindow,
  durationLabel,
  remainingLabel,
  resetLabel,
  usageLevel,
  usageSummary,
  usedPercent,
} from '../shared/usage'
import Icon from './Icon.vue'
// The same Codex and Claude readings as Settings → Usage; Go caches them, so polling stays cheap.
const props = defineProps({
  appearance: String,
  agent: String,
  profileId: String,
  profileName: String,
  detailed: Boolean,
})
const emit = defineEmits(['settings', 'detailed'])
const providers = ref([]),
  loading = ref(false),
  failed = ref(false),
  now = ref(Date.now()),
  open = ref(false),
  position = ref({}),
  trigger = ref(null),
  popup = ref(null)
let timer,
  request = 0
async function refresh(force = false) {
  if (loading.value) return
  loading.value = true
  // A reading for a profile that is no longer chosen is dropped.
  const current = ++request
  try {
    const values = await call('AgentUsage', force, props.profileId || '')
    if (current !== request) return
    providers.value = values || []
    failed.value = false
    now.value = Date.now()
  } catch {
    if (current === request) failed.value = true
  } finally {
    if (current === request) loading.value = false
  }
}
onMounted(() => {
  refresh()
  timer = setInterval(() => {
    now.value = Date.now()
    if (!document.hidden) refresh()
  }, 60000)
})
watch(
  () => props.profileId,
  () => {
    providers.value = []
    loading.value = false
    request++
    refresh()
  },
)
const agentName = (id) => agents.find((a) => a.id === id)?.name || id
const percent = (window) => remainingLabel(usedPercent(window))
const summary = computed(() => usageSummary(providers.value, props.agent, now.value))
const level = computed(() => (summary.value ? usageLevel(usedPercent(summary.value.window)) : ''))
const label = computed(() => {
  if (!summary.value) return loading.value && !providers.value.length ? 'Checking usage' : 'Usage unavailable'
  const { agent, window } = summary.value,
    duration = durationLabel(window.resetsAt, now.value)
  return `Usage: ${agentName(agent)} ${percent(window)} used${duration ? ', resets in ' + duration : ''}`
})
const rows = computed(() =>
  providers.value.map((provider) => ({ ...provider, window: constrainedWindow(provider, now.value) })),
)
const expired = (window) => window.resetsAt && window.resetsAt * 1000 <= now.value
function place() {
  if (!popup.value || !trigger.value) return
  const r = trigger.value.getBoundingClientRect(),
    height = popup.value.offsetHeight,
    width = popup.value.offsetWidth
  position.value = {
    left: Math.max(8, Math.min(r.right - width, innerWidth - width - 8)) + 'px',
    top: Math.max(8, r.top - height - 6) + 'px',
  }
}
function outside(event) {
  if (!popup.value?.contains(event.target) && !trigger.value?.parentElement.contains(event.target)) close()
}
function listen(on) {
  const method = on ? 'addEventListener' : 'removeEventListener'
  document[method]('pointerdown', outside)
  window[method]('resize', place)
}
async function show() {
  if (open.value) return
  open.value = true
  listen(true)
  await nextTick()
  place()
  popup.value?.querySelector('.mode-switch .selected')?.focus()
}
function close(restoreFocus = false) {
  if (!open.value) return
  open.value = false
  listen(false)
  if (restoreFocus) trigger.value?.focus()
}
function settings(section) {
  close()
  emit('settings', section)
}
function keys(event) {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    close(true)
  } else if (event.key === 'Tab') close()
  else menuKeys(event)
}
// Switching views changes the popover's height, so it moves to stay above the status bar.
watch(
  () => props.detailed,
  () => nextTick(place),
)
watch(rows, () => open.value && nextTick(place))
onBeforeUnmount(() => {
  clearInterval(timer)
  listen(false)
})
</script>
<template>
  <span class="status-item usage-status" :aria-busy="loading"
    ><button
      ref="trigger"
      class="status-item status-button usage-status-trigger"
      :class="level"
      :aria-label="label"
      :title="label"
      aria-haspopup="dialog"
      :aria-expanded="open"
      @click="open ? close() : show()"
    >
      <template v-if="summary"
        ><span class="status-optional">Used</span><img :src="agentIcon(summary.agent, appearance)" alt="" /><span
          class="usage-status-value"
          >{{ percent(summary.window) }}</span
        ><span v-if="durationLabel(summary.window.resetsAt, now)" class="status-optional usage-status-value">{{
          durationLabel(summary.window.resetsAt, now)
        }}</span></template
      ><template v-else
        ><Icon name="circle.dotted" :size="12" />{{ loading && !providers.length ? 'Usage…' : 'Usage' }}</template
      ></button
    ><button
      class="status-item status-button usage-status-refresh"
      :class="{ spinning: loading }"
      :disabled="loading"
      aria-label="Refresh Usage"
      title="Refresh Usage"
      @click="refresh(true)"
    >
      <Icon name="arrow.clockwise" :size="11" />
    </button>
    <!-- The status bar mounts with the app, before .app-shell is in the document. -->
    <Teleport defer to=".app-shell"
      ><div
        v-if="open"
        ref="popup"
        class="context-menu usage-popover"
        role="dialog"
        aria-label="Usage"
        :style="position"
        @keydown="keys"
        @pointerdown.stop
      >
        <header class="usage-popover-heading">
          <h2>Usage</h2>
          <span class="muted truncate" :title="'Profile: ' + profileName">{{ profileName }}</span>
          <button
            class="icon-button"
            :class="{ spinning: loading }"
            :disabled="loading"
            aria-label="Refresh Usage"
            title="Refresh Usage"
            @click="refresh(true)"
          >
            <Icon name="arrow.clockwise" :size="12" />
          </button>
        </header>
        <div class="mode-switch usage-popover-mode" role="group" aria-label="Usage View">
          <button :aria-pressed="detailed" :class="{ selected: detailed }" @click="emit('detailed', true)">
            Detailed
          </button>
          <button :aria-pressed="!detailed" :class="{ selected: !detailed }" @click="emit('detailed', false)">
            Compact
          </button>
        </div>
        <p v-if="failed" role="alert" class="muted usage-popover-note">Could not check usage. Try refreshing again.</p>
        <p v-else-if="loading && !providers.length" role="status" class="muted usage-popover-note">
          Checking Codex and Claude limits…
        </p>
        <hr v-if="rows.length" />
        <template v-if="!detailed"
          ><button
            v-for="row in rows"
            :key="row.agent"
            class="usage-popover-row"
            :title="row.message || ''"
            @click="settings('Usage')"
          >
            <img :src="agentIcon(row.agent, appearance)" alt="" /><span class="truncate">{{
              agentName(row.agent)
            }}</span
            ><span class="usage-popover-value"
              ><template v-if="row.window"
                ><span class="muted">{{ durationLabel(row.window.resetsAt, now) }}</span
                ><strong :class="usageLevel(usedPercent(row.window))">{{ percent(row.window) }}</strong></template
              ><span v-else class="muted">Unavailable</span></span
            ><Icon name="chevron.right" :size="10" /></button
        ></template>
        <template v-else
          ><section v-for="row in rows" :key="row.agent" class="usage-popover-agent">
            <h3>
              <img :src="agentIcon(row.agent, appearance)" alt="" /><span class="truncate">{{
                agentName(row.agent)
              }}</span
              ><span v-if="row.resetCredits != null" class="muted caption"
                >{{ row.resetCredits }} reset {{ row.resetCredits === 1 ? 'credit' : 'credits' }}</span
              >
            </h3>
            <p v-if="row.message" class="muted caption">{{ row.message }}</p>
            <div v-for="(window, index) in row.windows" :key="index" class="usage-popover-window">
              <div class="usage-window-heading">
                <span class="truncate">{{ window.label }}</span
                ><strong :class="expired(window) ? 'muted' : usageLevel(usedPercent(window))">{{
                  expired(window) ? 'Awaiting refresh' : percent(window) + ' used'
                }}</strong>
              </div>
              <div
                class="usage-meter"
                role="progressbar"
                :aria-label="agentName(row.agent) + ' ' + window.label + ' allowance used'"
                :aria-valuenow="usedPercent(window)"
                :aria-valuemin="0"
                :aria-valuemax="100"
                :aria-valuetext="percent(window) + ' used at last check'"
                :class="[expired(window) ? 'expired' : usageLevel(usedPercent(window))]"
              >
                <span :style="{ width: usedPercent(window) + '%' }" />
              </div>
              <p class="muted caption">{{ resetLabel(window.resetsAt, now) }}</p>
            </div>
          </section></template
        >
        <hr />
        <button class="usage-popover-row" @click="settings('Usage')">
          <span class="truncate">Usage details &amp; history</span><Icon name="chevron.right" :size="10" />
        </button>
        <button class="usage-popover-row" @click="settings('Profiles')">
          <span class="truncate">Manage Accounts…</span><Icon name="chevron.right" :size="10" />
        </button></div></Teleport
  ></span>
</template>
