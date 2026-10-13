<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { call } from '../shared/api'
import { agentIcon } from '../shared/appearance'
import { resetLabel, remainingLabel } from '../shared/usage'
import Icon from './Icon.vue'
import OpenRouterUsage from './OpenRouterUsage.vue'
defineProps({ appearance: String })
const providers = ref([]),
  loading = ref(false),
  error = ref(''),
  now = ref(Date.now())
const openRouter = ref(null)
let timer
async function refresh(force = false) {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    const [values] = await Promise.all([call('AgentUsage', force), openRouter.value?.refresh(force)])
    providers.value = values
    now.value = Date.now()
  } catch {
    error.value = 'Could not check usage. Try refreshing again.'
  } finally {
    loading.value = false
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
const date = (value) => new Date(value * 1000).toLocaleString()
</script>
<template>
  <section class="agent-usage" :aria-busy="loading">
    <header class="usage-toolbar">
      <p class="muted caption">Account limits and credits shared across sessions.</p>
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
    <OpenRouterUsage ref="openRouter" />
    <p v-if="providers.length" class="muted caption usage-footnote">
      Codex and Claude Code use their signed-in CLI accounts. OpenRouter uses the management key you provide.
    </p>
  </section>
</template>
