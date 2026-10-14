<script setup>
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { call } from '../shared/api'
import { agentIcon } from '../shared/appearance'
import { findProfile } from '../shared/profiles'
import { resetLabel, remainingLabel } from '../shared/usage'
import Icon from './Icon.vue'
import OpenRouterUsage from './OpenRouterUsage.vue'
const props = defineProps({ appearance: String, profiles: Array, profileId: String })
// Codex and Claude limits belong to the accounts of one profile; "" is Default.
const profile = ref(findProfile(props.profiles, props.profileId) ? props.profileId : '')
const providers = ref([]),
  loading = ref(false),
  error = ref(''),
  now = ref(Date.now())
const openRouter = ref(null)
let timer,
  request = 0
async function refresh(force = false) {
  if (loading.value) return
  loading.value = true
  error.value = ''
  // A reading for a profile that is no longer chosen is dropped.
  const current = ++request
  try {
    const [values] = await Promise.all([call('AgentUsage', force, profile.value), openRouter.value?.refresh(force)])
    if (current !== request) return
    providers.value = values
    now.value = Date.now()
  } catch {
    if (current === request) error.value = 'Could not check usage. Try refreshing again.'
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
onUnmounted(() => clearInterval(timer))
watch(profile, () => {
  providers.value = []
  loading.value = false
  request++
  refresh()
})
const date = (value) => new Date(value * 1000).toLocaleString()
</script>
<template>
  <section class="agent-usage" :aria-busy="loading">
    <header class="usage-toolbar">
      <p class="muted caption">Account limits and credits shared across sessions.</p>
      <div v-if="profiles?.length" class="theme-select general-select usage-profile">
        <select v-model="profile" aria-label="Profile">
          <option value="">Default</option>
          <option v-for="p in profiles" :key="p.id" :value="p.id">
            {{ p.emoji ? p.emoji + ' ' : '' }}{{ p.name }}
          </option></select
        ><Icon name="chevron.up.chevron.down" :size="10" />
      </div>
      <button class="standard-button" :disabled="loading" @click="refresh(true)">
        <Icon name="arrow.clockwise" :size="12" />{{ loading ? 'Checking…' : 'Refresh' }}
      </button>
    </header>
    <p v-if="error" role="alert" class="muted">{{ error }}</p>
    <p v-if="loading && !providers.length" class="muted" role="status">Checking Codex and Claude limits…</p>
    <section v-for="provider in providers" :key="provider.agent" class="usage-provider">
      <header>
        <img :src="agentIcon(provider.agent, appearance)" alt="" />
        <h3>{{ provider.agent === 'codex' ? 'Codex' : 'Claude Code' }}</h3>
        <span v-if="provider.resetCredits != null" class="usage-credits muted caption"
          >{{ provider.resetCredits }} reset {{ provider.resetCredits === 1 ? 'credit' : 'credits' }}</span
        >
      </header>
      <p v-if="provider.message" class="muted usage-unavailable">{{ provider.message }}</p>
      <div v-for="(window, index) in provider.windows" :key="index" class="usage-window">
        <div class="usage-window-heading">
          <span>{{ window.label }}</span
          ><strong :class="{ muted: window.resetsAt && window.resetsAt * 1000 <= now }">{{
            window.resetsAt && window.resetsAt * 1000 <= now
              ? 'Awaiting refresh'
              : remainingLabel(window.remaining) + ' left'
          }}</strong>
        </div>
        <div
          class="usage-meter"
          role="progressbar"
          :aria-label="
            (provider.agent === 'codex' ? 'Codex' : 'Claude Code') + ' ' + window.label + ' remaining allowance'
          "
          :aria-valuenow="window.remaining"
          :aria-valuemin="0"
          :aria-valuemax="100"
          :aria-valuetext="remainingLabel(window.remaining) + ' remaining at last check'"
          :class="{ expired: window.resetsAt && window.resetsAt * 1000 <= now }"
        >
          <span :style="{ width: window.remaining + '%' }" />
        </div>
        <p class="muted caption usage-reset" :title="window.resetsAt ? date(window.resetsAt) : ''">
          {{ resetLabel(window.resetsAt, now) }}
        </p>
      </div>
      <p class="muted caption usage-checked">Checked {{ date(provider.checkedAt) }}</p>
    </section>
    <OpenRouterUsage ref="openRouter" :profile-id="profile" />
    <p v-if="providers.length" class="muted caption usage-footnote">
      Codex and Claude Code use the CLI accounts signed in with the chosen profile. OpenRouter uses the management key
      you provide for it.
    </p>
  </section>
</template>
