<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { call, openURL } from '../shared/api'
import { providerName } from '../shared/workItems'
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
// Linear sources are configured here with their team and project; the others come from the
// repositories' remotes.
const sources = computed(() => props.workspace?.linearSources || [])
const listed = computed(() => props.workspace?.sources || [])
const repositorySources = computed(() => listed.value.filter((s) => s.kind === 'workItems' && s.provider !== 'linear'))
const reviewSources = computed(() => listed.value.filter((s) => s.kind === 'reviews'))
const linearShown = (id) => !listed.value.find((s) => s.id === id)?.hidden
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
// Shows or hides one source in Work Items or Reviews.
async function show(id, shown) {
  error.value = ''
  try {
    emit('state', await call('ShowWorkspaceSource', props.workspace.id, id, shown))
  } catch (e) {
    error.value = String(e)
  }
}
async function loadTeams() {
  teams.value = await call('LinearChoices', props.workspace.id, '')
}
onMounted(() =>
  run(async () => {
    account.value = await call('LinearAccount', props.workspace.id)
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
    const list = await call('LinearChoices', props.workspace.id, id)
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
    account.value = await call('ConnectLinear', props.workspace.id, value)
    await loadTeams()
  })
}
async function disconnect() {
  await run(async () => {
    await call('DisconnectLinear', props.workspace.id)
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
    projects.value = await call('LinearChoices', props.workspace.id, source.teamID)
    project.value = source.projectID || ''
  })
}
</script>
<template>
  <div class="settings-groups general-settings">
    <section class="settings-group" aria-labelledby="work-items-heading">
      <h3 id="work-items-heading" class="settings-group-title">Work Items</h3>
      <div class="settings-card">
        <div v-for="source in repositorySources" :key="source.id" class="setting-row">
          <div>
            <label :for="'source-' + source.id">{{ source.name }}</label>
            <p class="muted caption">{{ providerName(source.provider) }} issues</p>
          </div>
          <button
            :id="'source-' + source.id"
            class="switch"
            :class="{ on: !source.hidden }"
            role="switch"
            :aria-checked="!source.hidden"
            :aria-label="'Show issues from ' + source.name"
            @click="show(source.id, !!source.hidden)"
          >
            <span />
          </button>
        </div>
        <p v-if="!repositorySources.length" class="muted caption">
          Add a repository with a GitHub remote to list its issues.
        </p>
        <div class="setting-row">
          <div>
            <label>Linear</label>
            <p v-if="account?.id" class="muted caption">
              Connected as {{ account.name }}. This connection is only used by this workspace.
            </p>
            <template v-else>
              <p class="muted caption">
                Connect with a personal API key with read access to the teams you need. Stored in macOS Keychain or the
                Linux desktop keyring. This connection is only used by this workspace.
              </p>
              <button type="button" class="text-link" @click="openURL('https://linear.app/settings/account/security')">
                Create a personal API key
              </button>
            </template>
          </div>
          <button v-if="account?.id" class="standard-button" :disabled="busy" @click="disconnect">Disconnect</button>
        </div>
        <div v-for="source in sources" :key="source.id" class="setting-row linear-source">
          <div>
            <label>{{ source.name }}</label>
            <p class="muted caption">
              Default repository:
              {{ workspace.repositories.find((r) => r.id === source.repositoryID)?.name || 'Ask when starting work' }}
            </p>
          </div>
          <span class="source-actions">
            <button
              type="button"
              class="standard-button"
              :disabled="busy || !account?.id"
              :aria-label="'Edit Linear source ' + source.name"
              @click="edit(source)"
            >
              Edit
            </button>
            <button
              type="button"
              class="standard-button"
              :disabled="busy"
              :aria-label="'Remove Linear source ' + source.name"
              @click="run(async () => emit('state', await call('RemoveLinearSource', workspace.id, source.id)))"
            >
              Remove
            </button>
            <button
              class="switch"
              :class="{ on: linearShown(source.id) }"
              role="switch"
              :aria-checked="linearShown(source.id)"
              :aria-label="'Show issues from Linear source ' + source.name"
              @click="show(source.id, !linearShown(source.id))"
            >
              <span />
            </button>
          </span>
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
        <form v-else class="linear-form" @submit.prevent="save">
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
            {{ editing ? 'Save Linear Source' : 'Add Linear Source' }}
          </button>
        </form>
      </div>
    </section>
    <section class="settings-group" aria-labelledby="reviews-heading">
      <h3 id="reviews-heading" class="settings-group-title">Reviews</h3>
      <div class="settings-card">
        <div v-for="source in reviewSources" :key="source.id" class="setting-row">
          <div>
            <label :for="'source-' + source.id">{{ source.name }}</label>
            <p class="muted caption">{{ providerName(source.provider) }} pull requests</p>
          </div>
          <button
            :id="'source-' + source.id"
            class="switch"
            :class="{ on: !source.hidden }"
            role="switch"
            :aria-checked="!source.hidden"
            :aria-label="'Show pull requests from ' + source.name"
            @click="show(source.id, !!source.hidden)"
          >
            <span />
          </button>
        </div>
        <p v-if="!reviewSources.length" class="muted caption">
          Add a repository with a GitHub remote to list its pull requests.
        </p>
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
.setting-row {
  flex-wrap: wrap;
}
.source-actions {
  display: flex;
  gap: 8px;
}
.linear-source {
  padding-left: 16px;
}
.setting-row > div {
  min-width: 0;
  overflow-wrap: anywhere;
}
</style>
