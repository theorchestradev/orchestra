<script>
// Model catalogs come from the installed CLIs; list each agent's once per app run.
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
  loading = ref(false),
  error = ref('')
const choices = computed(() => (props.profiles.length ? 'profile, agent, model and effort' : 'agent, model and effort'))
const agentName = computed(() => agents.find((a) => a.id === agent.value)?.name)
const efforts = computed(() => modelEfforts(models.value, model.value))
let request = 0
async function loadModels() {
  const id = agent.value,
    token = ++request
  model.value = ''
  effort.value = ''
  models.value = []
  error.value = ''
  loading.value = true
  try {
    if (!catalogs.has(id)) catalogs.set(id, call('AgentModels', id))
    const list = await catalogs.get(id)
    if (token === request) models.value = list || []
  } catch (e) {
    catalogs.delete(id)
    if (token === request) error.value = String(e)
  } finally {
    if (token === request) {
      loading.value = false
      await nextTick()
      place()
    }
  }
}
watch(agent, loadModels)
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
  if (!agents.some((a) => a.id === agent.value))
    agent.value = agents.some((a) => a.id === props.defaultAgent) ? props.defaultAgent : agents[0].id
  // Each opening starts from the project's profile, or Default when it was removed.
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
              <option v-for="a in agents" :key="a.id" :value="a.id">{{ a.name }}</option></select
            ><Icon name="chevron.up.chevron.down" :size="10" /></span
        ></label>
        <div v-if="loading" class="metadata-loading" role="status"><span class="spinner" />Loading models…</div>
        <template v-else-if="error"
          ><p class="metadata-error" role="alert">{{ error }}</p>
          <button type="button" class="standard-button" @click="loadModels">Try Again</button></template
        >
        <p v-else-if="!models.length" class="muted caption">{{ agentName }} starts with its own model and effort.</p>
        <template v-else
          ><label
            >Model<span class="theme-select general-select"
              ><select v-model="model">
                <option value="">Default</option>
                <option v-for="m in models" :key="m.id" :value="m.id">{{ m.name }}</option></select
              ><Icon name="chevron.up.chevron.down" :size="10" /></span
          ></label>
          <label v-if="efforts.length"
            >Effort<span class="theme-select general-select"
              ><select v-model="effort">
                <option value="">Default</option>
                <option v-for="e in efforts" :key="e" :value="e">{{ effortLabel(e) }}</option></select
              ><Icon name="chevron.up.chevron.down" :size="10" /></span></label
        ></template>
        <footer>
          <button type="button" class="standard-button" @click="close(true)">Cancel</button
          ><button type="submit" class="standard-button primary" :disabled="loading">{{ label }}</button>
        </footer>
      </form>
    </section></Teleport
  >
</template>
