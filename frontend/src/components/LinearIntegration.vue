<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { call, openURL } from '../shared/api'
import Icon from './Icon.vue'
const props = defineProps({ workspace: Object })
const emit = defineEmits(['state'])
const account = ref(null),
  key = ref(''),
  busy = ref(false),
  error = ref(''),
  teams = ref([]),
  projects = ref([])
const team = ref(''),
  project = ref(''),
  repository = ref(''),
  editing = ref('')
const projectsLoading = ref(false)
const sources = computed(() => props.workspace?.linearSources || [])
let projectRequest = 0
async function run(action) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await action()
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
async function loadTeams() {
  teams.value = await call('LinearChoices', '')
}
onMounted(() =>
  run(async () => {
    account.value = await call('LinearAccount')
    if (account.value?.id) await loadTeams()
  }),
)
watch(team, async (id) => {
  const token = ++projectRequest
  project.value = ''
  projects.value = []
  projectsLoading.value = !!id
  if (!id) return
  try {
    const list = await call('LinearChoices', id)
    if (token === projectRequest) projects.value = list
  } catch (e) {
    if (token === projectRequest) error.value = String(e)
  } finally {
    if (token === projectRequest) projectsLoading.value = false
  }
})
watch(
  () => props.workspace?.id,
  () => {
    editing.value = ''
    team.value = ''
    project.value = ''
    repository.value = ''
  },
)
async function connect() {
  const value = key.value
  key.value = ''
  await run(async () => {
    account.value = await call('ConnectLinear', value)
    await loadTeams()
  })
}
async function disconnect() {
  await run(async () => {
    await call('DisconnectLinear')
    account.value = null
    teams.value = []
    team.value = ''
    projectRequest++
  })
}
async function save() {
  await run(async () => {
    emit(
      'state',
      await call('SaveLinearSource', props.workspace.id, {
        id: editing.value,
        teamID: team.value,
        projectID: project.value,
        repositoryID: repository.value,
      }),
    )
    editing.value = ''
    team.value = ''
    project.value = ''
    repository.value = ''
  })
}
async function edit(source) {
  editing.value = source.id
  team.value = source.teamID
  repository.value = source.repositoryID || ''
  await run(async () => {
    projects.value = await call('LinearChoices', source.teamID)
    project.value = source.projectID || ''
  })
}
</script>
<template>
  <div class="settings-groups general-settings">
    <section class="settings-group" aria-label="Linear integration">
      <h3 class="settings-group-title">Linear</h3>
      <div class="settings-card">
        <div class="setting-row">
          <div>
            <label>Linear account</label>
            <p class="muted caption">
              Connect with a personal API key with read access to the teams you need. Stored in macOS Keychain or the
              Linux desktop keyring.
            </p>
            <button class="text-link" @click="openURL('https://linear.app/settings/account/security')">
              Create a personal API key
            </button>
          </div>
          <button v-if="account?.id" class="standard-button" :disabled="busy" @click="disconnect">
            Disconnect {{ account.name }}
          </button>
        </div>
        <form v-if="!account?.id" class="linear-form" @submit.prevent="connect">
          <label
            >Personal API key<input
              v-model="key"
              type="password"
              autocomplete="off"
              spellcheck="false"
              required
              :disabled="busy"
          /></label>
          <button class="standard-button" type="submit" :disabled="busy || !key.trim()">Connect Linear</button>
        </form>
      </div>
    </section>
    <section v-if="workspace" class="settings-group" aria-label="Workspace Linear sources">
      <h3 class="settings-group-title">Sources for {{ workspace.name }}</h3>
      <div class="settings-card">
        <div v-for="source in sources" :key="source.id" class="setting-row">
          <div>
            <label>{{ source.name }}</label>
            <p class="muted caption">
              Default repository:
              {{ workspace.repositories.find((r) => r.id === source.repositoryID)?.name || 'Ask when starting work' }}
            </p>
          </div>
          <button class="standard-button" :disabled="busy || !account?.id" @click="edit(source)">Edit</button>
          <button
            class="standard-button"
            :disabled="busy"
            @click="run(async () => emit('state', await call('RemoveLinearSource', workspace.id, source.id)))"
          >
            Remove
          </button>
        </div>
        <form v-if="account?.id" class="linear-form" @submit.prevent="save">
          <label
            >Team<select v-model="team" required :disabled="busy">
              <option value="" disabled>Choose a team</option>
              <option v-for="t in teams" :key="t.id" :value="t.id">{{ t.name }}</option>
            </select></label
          >
          <label
            >Project<select v-model="project" :disabled="busy || projectsLoading || !team">
              <option value="">All projects</option>
              <option v-for="p in projects" :key="p.id" :value="p.id">{{ p.name }}</option>
            </select></label
          >
          <label
            >Default execution repository<select v-model="repository" :disabled="busy">
              <option value="">Ask when starting work</option>
              <option v-for="r in workspace.repositories" :key="r.id" :value="r.id">{{ r.name }}</option>
            </select></label
          >
          <button class="standard-button" type="submit" :disabled="busy || projectsLoading || !team">
            {{ editing ? 'Save Source' : 'Add Source' }}
          </button>
        </form>
        <p v-else class="muted caption linear-hint">Connect Linear to configure workspace sources.</p>
      </div>
    </section>
    <p v-if="busy" class="muted caption" role="status"><span class="spinner" />Loading Linear…</p>
    <p v-if="error" class="status-error" role="alert">{{ error }}</p>
  </div>
</template>
<style scoped>
.linear-form {
  display: grid;
  gap: 12px;
  padding: 16px;
}
.linear-form label {
  display: grid;
  gap: 6px;
  min-width: 0;
}
.linear-form input,
.linear-form select {
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  color: var(--text);
  background: var(--surface);
  border: 1px solid var(--rule);
  border-radius: 6px;
  padding: 8px;
  font: inherit;
}
.linear-form input:focus-visible,
.linear-form select:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}
.linear-form button {
  justify-self: start;
}
.linear-hint {
  padding: 16px;
}
.setting-row {
  flex-wrap: wrap;
}
.setting-row > div {
  min-width: 0;
  overflow-wrap: anywhere;
}
</style>
