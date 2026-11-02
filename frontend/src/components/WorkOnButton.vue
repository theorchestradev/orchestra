<script>
// Model catalogs come from the installed CLIs, once per agent and account per app run.
const catalogs = new Map()
</script>
<script setup>
import { computed, ref, watch, nextTick, onBeforeUnmount } from 'vue'
import { call } from '../shared/api'
import { agents } from '../shared/appearance'
import { modelEfforts, effortLabel } from '../shared/github'
import { findProfile } from '../shared/profiles'
import Icon from './Icon.vue'
const props = defineProps({
  label: String,
  defaultAgent: String,
  visible: Boolean,
  profiles: { type: Array, default: () => [] },
  profileId: String,
})
const emit = defineEmits(['work'])
const trigger = ref(null),
  popup = ref(null),
  open = ref(false),
  position = ref({})
const profile = ref(''),
  agent = ref(''),
  model = ref(''),
  effort = ref(''),
  models = ref([]),
  customModels = ref(false),
  customEfforts = ref([]),
  loading = ref(false),
  error = ref('')
const choices = computed(() => (props.profiles.length ? 'profile, agent, model and effort' : 'agent, model and effort'))
const supportsPrompt = (id) => agents.find((a) => a.id === id)?.initialPrompt !== false
const agentName = computed(() => agents.find((a) => a.id === agent.value)?.name)
const efforts = computed(() => (models.value.length ? modelEfforts(models.value, model.value) : customEfforts.value))
let request = 0
async function loadModels() {
  const id = agent.value,
    key = JSON.stringify([id, profile.value]),
    token = ++request
  model.value = ''
  effort.value = ''
  models.value = []
  customModels.value = false
  customEfforts.value = []
  error.value = ''
  loading.value = true
  try {
    if (!catalogs.has(key)) catalogs.set(key, call('AgentLaunchOptions', id, profile.value))
    const options = await catalogs.get(key)
    if (token === request) {
      models.value = options?.models || []
      customModels.value = !!options?.customModel
      customEfforts.value = options?.efforts || []
    }
  } catch (e) {
    catalogs.delete(key)
    if (token === request) error.value = String(e)
  } finally {
    if (token === request) {
      loading.value = false
      await nextTick()
      place()
    }
  }
}
watch([agent, profile], loadModels)
watch(efforts, (list) => {
  if (!list.includes(effort.value)) effort.value = ''
})
function place() {
  if (!popup.value || !trigger.value) return
  const r = trigger.value.getBoundingClientRect(),
    height = popup.value.offsetHeight,
    width = popup.value.offsetWidth
  position.value = {
    left: Math.max(8, Math.min(r.right - width, innerWidth - width - 8)) + 'px',
    top: Math.max(8, r.bottom + 6 + height > innerHeight ? r.top - height - 6 : r.bottom + 6) + 'px',
  }
}
function outside(event) {
  if (!popup.value?.contains(event.target) && !trigger.value?.contains(event.target)) close()
}
function listen(on) {
  const method = on ? 'addEventListener' : 'removeEventListener'
  document[method]('pointerdown', outside)
  window[method]('scroll', place, true)
  window[method]('resize', place)
}
async function toggle() {
  if (open.value) return close()
  if (!agents.some((a) => a.id === agent.value) || !supportsPrompt(agent.value))
    agent.value =
      agents.some((a) => a.id === props.defaultAgent) && supportsPrompt(props.defaultAgent)
        ? props.defaultAgent
        : agents[0].id
  // Each opening starts from the workspace's profile, or Default when it was removed.
  profile.value = findProfile(props.profiles, props.profileId) ? props.profileId : ''
  open.value = true
  listen(true)
  await nextTick()
  place()
  popup.value?.querySelector('select')?.focus()
}
function close(restoreFocus = false) {
  if (!open.value) return
  open.value = false
  listen(false)
  if (restoreFocus) trigger.value?.focus()
}
function start() {
  emit('work', { agent: agent.value, model: model.value, effort: effort.value, profileID: profile.value })
  close()
}
watch(
  () => props.visible,
  (visible) => {
    if (!visible) close()
  },
)
onBeforeUnmount(() => listen(false))
</script>
<template>
  <div class="split-button">
    <button class="standard-button" @click="emit('work')">{{ label }}</button
    ><button
      ref="trigger"
      class="standard-button split-toggle"
      :aria-label="`Choose ${choices} for ${label}`"
      :title="`Choose ${choices}`"
      aria-haspopup="dialog"
      :aria-expanded="open"
      @click="toggle"
    >
      <Icon name="chevron.down" :size="10" />
    </button>
  </div>
  <Teleport to=".app-shell"
    ><section
      v-if="open"
      ref="popup"
      class="metadata-popover work-on-popover"
      :style="position"
      role="dialog"
      :aria-label="`${label} with`"
      @keydown.esc.stop.prevent="close(true)"
    >
      <form class="metadata-editor" @submit.prevent="start">
        <label v-if="profiles.length"
          >Profile<span class="theme-select general-select"
            ><select v-model="profile">
              <option value="">Default</option>
              <option v-for="p in profiles" :key="p.id" :value="p.id">
                {{ p.emoji ? p.emoji + ' ' : '' }}{{ p.name }}
              </option></select
            ><Icon name="chevron.up.chevron.down" :size="10" /></span
        ></label>
        <label
          >Agent<span class="theme-select general-select"
            ><select v-model="agent">
              <option v-for="a in agents" :key="a.id" :value="a.id" :disabled="a.initialPrompt === false">
                {{ a.name }}{{ a.initialPrompt === false ? ' (use New Session)' : '' }}
              </option></select
            ><Icon name="chevron.up.chevron.down" :size="10" /></span
        ></label>
        <div v-if="loading" class="metadata-loading" role="status"><span class="spinner" />Loading models…</div>
        <template v-else-if="error"
          ><p class="metadata-error" role="alert">{{ error }}</p>
          <button type="button" class="standard-button" @click="loadModels">Try Again</button></template
        >
        <p v-else-if="!models.length && !customModels" class="muted caption">
          {{ agentName }} starts with its own model and effort.
        </p>
        <template v-else
          ><label v-if="models.length"
            >Model<span class="theme-select general-select"
              ><select v-model="model">
                <option value="">Default</option>
                <option v-for="m in models" :key="m.id" :value="m.id">{{ m.name }}</option></select
              ><Icon name="chevron.up.chevron.down" :size="10" /></span
          ></label>
          <label v-if="!models.length && customModels"
            >Model ID
            <input
              v-model="model"
              autocorrect="off"
              autocapitalize="off"
              autocomplete="off"
              spellcheck="false"
              placeholder="Use the CLI default"
            />
          </label>
          <p v-if="!models.length && customModels" class="muted caption">
            Enter a model ID configured in {{ agentName }}, or leave blank to use its default.
          </p>
          <label v-if="efforts.length"
            >Effort<span class="theme-select general-select"
              ><select v-model="effort">
                <option value="">Default</option>
                <option v-for="e in efforts" :key="e" :value="e">{{ effortLabel(e) }}</option></select
              ><Icon name="chevron.up.chevron.down" :size="10" /></span></label
        ></template>
        <p v-if="efforts.length && !models.length" class="muted caption">
          Effort support depends on the selected model.
        </p>
        <footer>
          <button type="button" class="standard-button" @click="close(true)">Cancel</button
          ><button type="submit" class="standard-button primary" :disabled="loading">{{ label }}</button>
        </footer>
      </form>
    </section></Teleport
  >
</template>
