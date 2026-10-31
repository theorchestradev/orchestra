<script setup>
import { computed, nextTick, reactive, ref, watch } from 'vue'
import { agents, agentIcon } from '../shared/appearance'
import { profileAgents, profileSymbol } from '../shared/profiles'
import { workspaceTitle } from '../shared/workspace'
import { call } from '../shared/api'
import Icon from './Icon.vue'
import Modal from './Modal.vue'
import EmojiPicker from './EmojiPicker.vue'
// Settings → Profiles. Orchestra gives each profile its own CLI config folders, so the user only
// names it and signs in; signing in always happens in a terminal session the user drives.
const props = defineProps({ profiles: Array, workspace: Object, appearance: String })
const emit = defineEmits(['state', 'login'])
const agentName = (id) => agents.find((a) => a.id === id)?.name || id
const otherAgents = agents
  .filter((a) => !profileAgents.includes(a.id))
  .map((a) => a.name)
  .join(', ')
const rows = computed(() => [{ id: '', name: 'Default' }, ...(props.profiles || [])])
// Sign-in status by profile ID; "" is Default.
const logins = reactive({}),
  errors = reactive({})
async function check(id) {
  logins[id] = null
  try {
    logins[id] = Object.fromEntries((await call('ProfileLogins', id)).map((l) => [l.agent, l]))
  } catch (e) {
    logins[id] = {}
    errors[id] = String(e)
  }
}
watch(
  () => rows.value.map((p) => p.id).join(),
  () => rows.value.forEach((p) => logins[p.id] === undefined && check(p.id)),
  { immediate: true },
)
function summary(p) {
  const status = logins[p.id]
  if (!status) return 'Checking accounts…'
  if (!p.id) return 'The accounts each CLI uses outside Orchestra'
  const count = profileAgents.filter((a) => status[a]?.loggedIn).length
  return count === profileAgents.length ? 'All accounts logged in' : `${count} of ${profileAgents.length} logged in`
}
const loggedIn = (p, agent) => !!logins[p.id]?.[agent]?.loggedIn
// The Orchestra Skill is installed only when the user clicks Install.
const skills = reactive({})
const missingSkill = (p) => !!logins[p.id] && profileAgents.some((a) => logins[p.id][a] && !logins[p.id][a].skill)
async function installSkill(p) {
  skills[p.id] = { busy: true }
  try {
    const result = await call('InstallProfileSkill', p.id)
    skills[p.id] =
      result.exitCode === 0
        ? {}
        : { error: 'The skills CLI exited with code ' + result.exitCode + '.', output: result.output }
    if (result.exitCode === 0) check(p.id)
  } catch (e) {
    skills[p.id] = { error: String(e) }
  }
}
async function logIn(p, agent) {
  errors[p.id] = ''
  try {
    emit('login', await call('LogIn', props.workspace.id, p.id, agent))
    delete logins[p.id]
  } catch (e) {
    errors[p.id] = String(e)
  }
}
// New Profile and Edit Profile share one dialog; editing never changes a profile's folders.
const editing = ref(null),
  form = reactive({ name: '', emoji: '' }),
  formError = ref(''),
  busy = ref(false),
  addButton = ref(null)
function openEditor(p = { id: '' }) {
  editing.value = p
  form.name = p.name || ''
  form.emoji = p.emoji || ''
  formError.value = ''
}
async function saveEditor() {
  busy.value = true
  formError.value = ''
  try {
    const id = editing.value.id
    emit(
      'state',
      await (id ? call('EditProfile', id, form.name, form.emoji) : call('CreateProfile', form.name, form.emoji)),
    )
    editing.value = null
  } catch (e) {
    formError.value = String(e)
  } finally {
    busy.value = false
  }
}
const removal = ref(null),
  removeError = ref('')
async function remove() {
  removeError.value = ''
  try {
    emit('state', await call('RemoveProfile', removal.value.id))
    removal.value = null
    await nextTick()
    addButton.value?.focus()
  } catch (e) {
    removeError.value = String(e)
  }
}
</script>
<template>
  <div class="profile-settings">
    <p class="muted caption profile-intro">
      A profile is a set of agent accounts. Each workspace chooses the profile its new sessions use, and the New Session
      dialog can choose another. A session keeps its profile for its whole life.
    </p>
    <div class="settings-card general-settings profile-list">
      <template v-for="(p, index) in rows" :key="p.id || 'default'"
        ><hr v-if="index" />
        <section class="profile-row" :aria-labelledby="'profile-name-' + (p.id || 'default')">
          <header class="profile-heading">
            <span class="profile-avatar" aria-hidden="true"
              ><Icon v-if="!p.id" name="person.crop.circle" :size="15" /><template v-else>{{
                profileSymbol(p)
              }}</template></span
            ><span class="profile-title"
              ><strong :id="'profile-name-' + (p.id || 'default')" class="truncate" :title="p.name">{{ p.name }}</strong
              ><small class="muted caption">{{ summary(p) }}</small></span
            ><template v-if="p.id"
              ><button
                type="button"
                class="icon-button profile-action"
                :aria-label="'Edit ' + p.name + '…'"
                title="Edit Profile…"
                @click="openEditor(p)"
              >
                <Icon name="square.and.pencil" :size="13" /></button
              ><button
                type="button"
                class="icon-button profile-action"
                :aria-label="'Remove ' + p.name + '…'"
                title="Remove Profile…"
                @click="((removal = p), (removeError = ''))"
              >
                <Icon name="trash" :size="13" /></button
            ></template>
          </header>
          <ul class="profile-agents" :aria-label="'Accounts in ' + p.name">
            <li v-for="agent in profileAgents" :key="agent" class="profile-agent">
              <img :src="agentIcon(agent, appearance)" alt="" /><span class="profile-agent-name">{{
                agentName(agent)
              }}</span
              ><span v-if="!logins[p.id]" class="muted caption" role="status"
                ><span class="spinner" /><span class="visually-hidden">Checking</span></span
              ><span
                v-else-if="logins[p.id][agent]?.message"
                class="muted caption truncate"
                :title="logins[p.id][agent].message"
                >{{ logins[p.id][agent].message }}</span
              ><span v-else class="profile-login caption" :class="{ on: loggedIn(p, agent) }"
                ><i />{{ loggedIn(p, agent) ? 'Logged in' : 'Not logged in' }}</span
              ><button
                type="button"
                :class="loggedIn(p, agent) ? 'text-link profile-relogin' : 'standard-button'"
                :disabled="!workspace"
                :title="workspace ? 'Opens a terminal in ' + workspaceTitle(workspace) : 'Add a workspace first'"
                :aria-label="
                  (loggedIn(p, agent) ? 'Log In Again to ' : 'Log In to ') + agentName(agent) + ' in ' + p.name
                "
                @click="logIn(p, agent)"
              >
                {{ loggedIn(p, agent) ? 'Log In Again' : 'Log In' }}
              </button>
            </li>
          </ul>
          <div v-if="missingSkill(p)" class="profile-skill">
            <Icon name="exclamationmark.triangle" :size="13" />
            <p class="caption">
              <strong>Orchestra Skill not installed.</strong>
              <span class="muted"> It lets agents in different sessions talk to each other.</span>
            </p>
            <button
              type="button"
              class="standard-button"
              :disabled="skills[p.id]?.busy"
              :aria-label="'Install the Orchestra Skill in ' + p.name"
              @click="installSkill(p)"
            >
              {{ skills[p.id]?.busy ? 'Installing…' : 'Install' }}
            </button>
          </div>
          <p v-if="skills[p.id]?.error" class="error caption" role="alert">{{ skills[p.id].error }}</p>
          <pre v-if="skills[p.id]?.output" class="skill-output" tabindex="0" aria-label="Skills CLI output">{{
            skills[p.id].output
          }}</pre>
          <p v-if="errors[p.id]" class="error caption" role="alert">{{ errors[p.id] }}</p>
        </section></template
      >
      <div class="theme-actions">
        <button ref="addButton" type="button" class="standard-button" @click="openEditor()">
          <Icon name="plus" :size="12" />New Profile…
        </button>
      </div>
    </div>
    <p class="muted caption profile-notes">
      Log In opens a terminal in {{ workspace ? workspaceTitle(workspace) : 'the selected workspace' }} where you
      complete the CLI’s sign-in. A new profile starts without the CLI settings, MCP servers and skills of Default.
      {{ otherAgents }} cannot choose an account yet, so they always use Default’s. In a Terminal session, a shell
      startup file that sets CLAUDE_CONFIG_DIR or CODEX_HOME can still switch account.
    </p>
    <Teleport to=".app-shell">
      <Modal v-if="editing" :label="editing.id ? 'Edit Profile' : 'New Profile'" @close="!busy && (editing = null)"
        ><form class="rename-dialog profile-dialog" @submit.prevent="saveEditor">
          <h2>{{ editing.id ? 'Edit Profile' : 'New Profile' }}</h2>
          <div class="workspace-name-fields">
            <div class="emoji-field">
              <label for="profile-emoji">Emoji</label
              ><span class="emoji-input"
                ><EmojiPicker id="profile-emoji" :value="form.emoji" @pick="form.emoji = $event" /><button
                  v-if="form.emoji"
                  type="button"
                  class="icon-button"
                  aria-label="Clear Emoji"
                  title="Clear Emoji"
                  @click="form.emoji = ''"
                >
                  <Icon name="xmark" :size="10" /></button
              ></span>
            </div>
            <label class="name-field"
              >Profile Name<input
                autocorrect="off"
                autocapitalize="off"
                v-model="form.name"
                autofocus
                maxlength="200"
                placeholder="Name, such as Work"
                aria-label="Profile Name"
            /></label>
            <p v-if="!editing.id" class="muted caption">
              Orchestra keeps this profile’s accounts apart from your other ones. Log in to each agent after creating
              it.
            </p>
          </div>
          <p v-if="formError" class="error" role="alert">{{ formError }}</p>
          <footer>
            <button type="button" class="standard-button" :disabled="busy" @click="editing = null">Cancel</button
            ><button type="submit" class="standard-button primary" :disabled="busy || !form.name.trim()">
              {{ editing.id ? 'Save' : 'Create Profile' }}
            </button>
          </footer>
        </form></Modal
      >
      <Modal v-if="removal" label="Remove Profile" @close="removal = null"
        ><div class="close-dialog profile-remove-dialog">
          <h2>Remove {{ removal.name }}?</h2>
          <p>
            Workspaces that use it move to Default. Sessions that used it can no longer be resumed, so they never switch
            account. Its sign-ins and conversations are kept on disk.
          </p>
          <p v-if="removeError" class="error" role="alert">{{ removeError }}</p>
          <footer>
            <button class="standard-button" autofocus @click="removal = null">Cancel</button
            ><button class="standard-button destructive" data-default-action @click="remove">Remove Profile</button>
          </footer>
        </div></Modal
      >
    </Teleport>
  </div>
</template>
