<script setup>
import { computed, ref, watch, nextTick, onBeforeUnmount } from 'vue'
import { call } from '../shared/api'
import { menuKeys } from '../shared/menu'
import Modal from './Modal.vue'
import Icon from './Icon.vue'
import { agents, agentIcon } from '../shared/appearance'
import { repositories, workspaceTitle } from '../shared/workspace'
const props = defineProps({
  state: Object,
  appearance: { type: String, default: 'dark' },
  editorVisible: { type: Boolean, default: true },
})
const emit = defineEmits(['state', 'editing'])
const actionMenu = ref(null),
  menuElement = ref(null),
  menuPosition = ref({})
let menuTrigger
function placeMenu() {
  if (!menuElement.value || !menuTrigger) return
  const r = menuTrigger.getBoundingClientRect()
  menuPosition.value = {
    left:
      Math.max(
        8,
        Math.min(r.right - menuElement.value.offsetWidth, window.innerWidth - menuElement.value.offsetWidth - 8),
      ) + 'px',
    top: Math.max(8, Math.min(r.bottom + 4, window.innerHeight - menuElement.value.offsetHeight - 8)) + 'px',
  }
}
function outsideMenu(event) {
  if (!menuElement.value?.contains(event.target) && !menuTrigger?.contains(event.target)) closeMenu()
}
function listenMenu(on) {
  const method = on ? 'addEventListener' : 'removeEventListener'
  document[method]('pointerdown', outsideMenu, true)
  window[method]('resize', closeMenu)
  document[method]('scroll', closeMenu, true)
}
function closeMenu(restoreFocus = false) {
  actionMenu.value = null
  listenMenu(false)
  if (restoreFocus === true) menuTrigger?.focus()
}
async function toggleMenu(event, item) {
  const wasOpen = actionMenu.value?.id === item.id
  closeMenu()
  if (wasOpen) return
  menuTrigger = event.currentTarget
  actionMenu.value = item
  listenMenu(true)
  await nextTick()
  placeMenu()
  menuElement.value?.querySelector('button')?.focus()
}
function menuKey(event) {
  if (event.key === 'Escape' || event.key === 'Tab') {
    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
    }
    closeMenu(true)
  } else menuKeys(event)
}
function menuAction(kind) {
  const item = props.state.automations?.find((item) => item.id === actionMenu.value?.id)
  closeMenu(true)
  if (!item) return
  if (kind === 'edit') edit(item)
  else if (kind === 'run') runNow(item.id)
  else remove(item.id)
}
onBeforeUnmount(() => listenMenu(false))
const query = ref(''),
  filter = ref('all'),
  notice = ref('')
const filtered = computed(() =>
  (props.state.automations || []).filter((item) => {
    if (filter.value !== 'all' && item.enabled !== (filter.value === 'enabled')) return false
    const workspace = props.state.workspaces.find((p) => p.id === item.workspaceID)
    const repository = repositories(workspace).find((r) => r.id === item.repositoryID)
    return [item.name, item.prompt, agentName(item.agent), workspaceTitle(workspace), repository?.name]
      .join(' ')
      .toLowerCase()
      .includes(query.value.trim().toLowerCase())
  }),
)
watch([() => filtered.value.map((item) => item.id).join(':'), () => props.editorVisible], () => closeMenu())
function clearFilters() {
  query.value = ''
  filter.value = 'all'
}
function agentName(id) {
  return agents.find((a) => a.id === id)?.name || id
}
function context(item) {
  const workspace = props.state.workspaces.find((p) => p.id === item.workspaceID)
  return [
    workspaceTitle(workspace) || 'Workspace unavailable',
    repositories(workspace).find((r) => r.id === item.repositoryID)?.name,
  ]
    .filter(Boolean)
    .join(' · ')
}
const draft = ref(null),
  error = ref(''),
  busy = ref(false)
watch(
  () => props.editorVisible,
  (visible) => {
    if (!visible) draft.value = null
  },
)
const templates = [
  {
    name: 'Morning repository review',
    icon: 'sun.max',
    description: 'Start the day with recent changes, risks and next steps.',
    days: [1, 2, 3, 4, 5],
    time: '09:00',
    prompt:
      'Review repository activity from the last day. Summarize the main changes, flag potential regressions and suggest the most useful next steps. Include file references where relevant. Do not modify files.',
  },
  {
    name: 'Find potential bugs',
    icon: 'magnifyingglass',
    description: 'Look through recent changes for bugs worth investigating.',
    days: [1, 2, 3, 4, 5],
    time: '10:00',
    prompt:
      'Inspect recent commits for concrete bugs and regressions. Trace the affected code paths and report actionable findings with file references, impact and suggested fixes. Prioritize evidence over speculation. Do not modify files.',
  },
  {
    name: 'Review test coverage',
    icon: 'checkmark.circle',
    description: 'Spot important behaviors that need better tests.',
    days: [1],
    time: '09:00',
    prompt:
      'Review recent code changes and their tests. Identify important behaviors and edge cases that are not covered. Suggest a short prioritized list of meaningful tests, explaining what each would catch. Do not modify files.',
  },
  {
    name: 'Weekly changelog',
    icon: 'note.text',
    description: 'Turn this week’s commits into a readable release summary.',
    days: [5],
    time: '16:00',
    prompt:
      'Summarize the last seven days of commits as a draft changelog. Group user-visible changes into features, improvements and fixes, and link to relevant commits. Call out breaking changes if any. Do not modify files or publish anything.',
  },
  {
    name: 'Documentation check',
    icon: 'book',
    description: 'Find documentation that no longer matches the code.',
    days: [3],
    time: '09:00',
    prompt:
      'Compare recent code changes with the repository documentation. Find outdated instructions, missing configuration details and examples that no longer match the implementation. Report specific documentation updates with file references. Do not modify files.',
  },
  {
    name: 'Dependency health check',
    icon: 'shield',
    description: 'Review dependencies for maintenance and security concerns.',
    days: [1],
    time: '10:00',
    prompt:
      'Review dependency manifests and lockfiles for maintenance and security concerns. Use available audit tools without installing or updating dependencies. Report confirmed issues, evidence and practical next steps, and clearly note anything you could not verify. Do not modify files.',
  },
]
function useTemplate(template) {
  edit()
  Object.assign(draft.value, {
    name: template.name,
    prompt: template.prompt,
    days: [...template.days],
    time: template.time,
  })
}
const weekdays = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const frequency = ref('weekly')
const workspace = computed(() => props.state.workspaces.find((p) => p.id === draft.value?.workspaceID))
function edit(item) {
  emit('editing', true)
  error.value = ''
  notice.value = ''
  frequency.value = !item || item.days?.length ? (item?.days?.length === 7 ? 'daily' : 'weekly') : 'once'
  draft.value = item
    ? JSON.parse(JSON.stringify(item))
    : {
        id: '',
        name: '',
        workspaceID: props.state.selectedWorkspaceID || props.state.workspaces[0]?.id || '',
        repositoryID: '',
        agent: props.state.preferences.defaultAgent || 'claude',
        prompt: '',
        enabled: true,
        days: [1, 2, 3, 4, 5],
        time: '09:00',
        date: '',
        timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
      }
  if (!item) chooseRepository()
}
function cancel() {
  emit('editing', false)
  draft.value = null
  error.value = ''
}
function chooseRepository() {
  draft.value.repositoryID = repositories(workspace.value)[0]?.id || ''
}
async function save() {
  busy.value = true
  error.value = ''
  try {
    emit(
      'state',
      await call('SaveAutomation', {
        ...draft.value,
        days:
          frequency.value === 'daily' ? [0, 1, 2, 3, 4, 5, 6] : frequency.value === 'weekly' ? draft.value.days : [],
      }),
    )
    draft.value = null
    emit('editing', false)
    notice.value = 'Automation saved.'
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
async function runNow(id) {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    emit('state', await call('RunAutomation', id))
    notice.value = 'Automation started.'
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
async function remove(id) {
  busy.value = true
  error.value = ''
  try {
    emit('state', await call('DeleteAutomation', id))
    notice.value = 'Automation deleted.'
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
function dateLabel(value) {
  return value && !value.startsWith('0001-')
    ? new Date(value).toLocaleString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      })
    : ''
}
function schedule(item) {
  const days = [...new Set(item.days || [])].sort().join(',')
  const label =
    days === '0,1,2,3,4,5,6'
      ? 'Every day'
      : days === '1,2,3,4,5'
        ? 'Weekdays'
        : item.days?.length
          ? item.days.map((d) => weekdays[d]).join(', ')
          : item.date
  return `${label} · ${item.time}`
}
</script>
<template>
  <section class="automations-page" aria-label="Automations">
    <header class="automations-heading">
      <div>
        <h1 tabindex="-1">Automations</h1>
      </div>
      <button class="standard-button primary" :disabled="!state.workspaces.length || busy" @click="edit()">
        <Icon name="plus" :size="13" />New Automation
      </button>
    </header>
    <div v-if="state.automations?.length" class="automations-toolbar">
      <label class="automation-search"
        ><Icon name="magnifyingglass" :size="14" /><input
          v-model="query"
          type="search"
          aria-label="Search automations"
          placeholder="Search automations…"
          autocorrect="off"
          autocapitalize="off"
          spellcheck="false"
      /></label>
      <div class="theme-select general-select">
        <select v-model="filter" aria-label="Filter automations">
          <option value="all">All automations</option>
          <option value="enabled">Enabled</option>
          <option value="disabled">Disabled</option></select
        ><Icon name="chevron.up.chevron.down" :size="10" />
      </div>
    </div>
    <p v-if="error && !draft" class="error" role="alert">{{ error }}</p>
    <p v-if="notice" class="automation-notice" role="status">{{ notice }}</p>
    <div class="automation-list" :class="{ 'is-empty': !state.automations?.length }" :aria-busy="busy">
      <div v-if="!state.automations?.length" class="automation-empty">
        <Icon name="bot" :size="36" />
        <h2>No automations yet</h2>
        <p>
          {{
            state.workspaces.length
              ? 'Put recurring work on autopilot. Start with a template or create your own.'
              : 'Add a workspace to create your first automation.'
          }}
        </p>
        <button class="standard-button" :disabled="!state.workspaces.length || busy" @click="edit()">
          Create from Scratch
        </button>
        <div class="automation-template-heading">Templates</div>
        <div class="automation-templates" aria-label="Automation templates">
          <button
            v-for="template in templates"
            :key="template.name"
            class="automation-template"
            :disabled="!state.workspaces.length || busy"
            @click="useTemplate(template)"
          >
            <Icon :name="template.icon" :size="19" />
            <span class="automation-template-name">{{ template.name }}</span>
            <span class="automation-template-description">{{ template.description }}</span>
            <span class="automation-template-schedule"><Icon name="clock" :size="12" />{{ schedule(template) }}</span>
          </button>
        </div>
      </div>
      <div v-else-if="!filtered.length" class="automation-empty">
        <Icon name="magnifyingglass" :size="30" />
        <h2>No matching automations</h2>
        <p>Try another search or status.</p>
        <button class="standard-button" @click="clearFilters">Clear Filters</button>
      </div>
      <article v-for="item in filtered" :key="item.id" class="automation-row">
        <div class="automation-identity">
          <img class="automation-agent" :src="agentIcon(item.agent, appearance)" :alt="agentName(item.agent)" />
          <div class="automation-details">
            <div class="automation-title">
              <h2 :title="item.prompt">{{ item.name }}</h2>
              <span v-if="!item.enabled" class="automation-status">Disabled</span>
            </div>
            <p class="automation-context">{{ agentName(item.agent) }} · {{ context(item) }}</p>
            <p v-if="item.lastError" class="error" role="alert">{{ item.lastError }}</p>
          </div>
        </div>
        <p class="automation-schedule" :title="'Time zone: ' + item.timeZone">
          <Icon name="clock" :size="12" />{{ schedule(item) }}
        </p>
        <div
          class="automation-next-run"
          :title="dateLabel(item.lastRun) ? 'Last started: ' + dateLabel(item.lastRun) : undefined"
        >
          <span>Next run</span><time v-if="item.enabled" :datetime="item.nextRun">{{ dateLabel(item.nextRun) }}</time
          ><span v-else>—</span>
        </div>
        <div class="automation-actions">
          <button
            class="icon-button"
            :disabled="busy"
            :aria-label="'Options for ' + item.name"
            title="Automation options"
            aria-haspopup="menu"
            :aria-expanded="actionMenu?.id === item.id"
            @click.stop="toggleMenu($event, item)"
            @keydown.down.prevent="toggleMenu($event, item)"
          >
            <Icon name="ellipsis" :size="16" />
          </button>
        </div>
      </article>
    </div>
    <p class="automations-footnote">
      Orchestra must be open to start automations. Missed runs execute once when it reopens.
    </p>
    <Teleport to=".app-shell">
      <div
        v-if="actionMenu"
        ref="menuElement"
        class="context-menu automation-menu"
        role="menu"
        :aria-label="'Options for ' + actionMenu.name"
        :style="menuPosition"
        @pointerdown.stop
        @keydown="menuKey"
      >
        <button role="menuitem" @click="menuAction('edit')">Edit</button>
        <button role="menuitem" @click="menuAction('run')">Run Now</button>
        <hr role="separator" />
        <button role="menuitem" class="destructive" @click="menuAction('delete')">Delete</button>
      </div>
    </Teleport>
    <Modal v-if="draft" :label="draft.id ? 'Edit Automation' : 'New Automation'" @close="!busy && cancel()">
      <div class="automation-editor">
        <header>
          <h2>{{ draft.id ? 'Edit Automation' : 'New Automation' }}</h2>
          <p>Define when it runs and what your agent should do.</p>
        </header>
        <p v-if="error" class="error" role="alert">{{ error }}</p>
        <form @submit.prevent="save">
          <fieldset :disabled="busy" class="automation-flow">
            <label class="automation-name"
              >Name<input
                v-model="draft.name"
                autofocus
                required
                placeholder="e.g. Morning repository review"
                autocorrect="off"
                autocapitalize="off"
            /></label>
            <fieldset class="automation-step" aria-describedby="automation-trigger-help">
              <legend>Trigger</legend>
              <p id="automation-trigger-help" class="automation-step-help">When should this automation run?</p>
              <div class="automation-trigger-card">
                <div class="automation-trigger-line">
                  <Icon name="clock" :size="18" />
                  <div class="theme-select general-select automation-schedule-picker">
                    <select v-model="frequency" aria-label="Schedule">
                      <option value="once">Once</option>
                      <option value="daily">Every day</option>
                      <option value="weekly">Selected weekdays</option></select
                    ><Icon name="chevron.up.chevron.down" :size="10" />
                  </div>
                  <span>at</span
                  ><input v-model="draft.time" type="time" aria-label="Time" class="automation-token" required />
                </div>
                <fieldset v-if="frequency === 'weekly'" class="weekdays">
                  <legend>Days</legend>
                  <label v-for="(day, index) in weekdays" :key="day" class="weekday-option"
                    ><input v-model="draft.days" type="checkbox" :value="index" />{{ day }}</label
                  >
                </fieldset>
                <label v-if="frequency === 'once'" class="automation-date"
                  >Date<input v-model="draft.date" type="date" required
                /></label>
                <p class="automation-timezone">Time zone: {{ draft.timeZone }}</p>
              </div>
            </fieldset>
            <fieldset class="automation-step" aria-describedby="automation-action-help">
              <legend>Action</legend>
              <p id="automation-action-help" class="automation-step-help">What should your agent do?</p>
              <div class="automation-action-card">
                <label
                  >Agent
                  <div class="theme-select automation-agent-picker">
                    <img :src="agentIcon(draft.agent, appearance)" alt="" /><select v-model="draft.agent" required>
                      <option v-for="agent in agents" :key="agent.id" :value="agent.id">
                        {{ agent.name }}
                      </option></select
                    ><Icon name="chevron.up.chevron.down" :size="10" /></div
                ></label>
                <div class="automation-context-fields">
                  <label
                    >Workspace
                    <div class="theme-select general-select">
                      <select v-model="draft.workspaceID" required @change="chooseRepository">
                        <option v-for="p in state.workspaces" :key="p.id" :value="p.id">
                          {{ workspaceTitle(p) }}
                        </option></select
                      ><Icon name="chevron.up.chevron.down" :size="10" /></div
                  ></label>
                  <label
                    >Repository
                    <div class="theme-select general-select">
                      <select v-model="draft.repositoryID" required>
                        <option v-for="repo in repositories(workspace)" :key="repo.id" :value="repo.id">
                          {{ repo.name || repo.directory }}
                        </option></select
                      ><Icon name="chevron.up.chevron.down" :size="10" /></div
                  ></label>
                </div>
                <label
                  >Prompt<textarea
                    v-model="draft.prompt"
                    required
                    rows="5"
                    placeholder="Describe the task your agent should complete…"
                    autocorrect="off"
                    autocapitalize="off"
                    spellcheck="false"
                  />
                </label>
                <p class="automation-step-help">Creates a new session using the workspace profile.</p>
              </div>
            </fieldset>
            <div class="automation-enabled">
              <label for="automation-enabled">Enabled</label>
              <button
                id="automation-enabled"
                type="button"
                class="switch"
                :class="{ on: draft.enabled }"
                role="switch"
                :aria-checked="!!draft.enabled"
                aria-label="Enabled"
                @click="draft.enabled = !draft.enabled"
              >
                <span />
              </button>
            </div>
          </fieldset>
          <footer>
            <button type="button" class="standard-button" :disabled="busy" @click="cancel">Cancel</button>
            <button
              type="submit"
              class="standard-button primary"
              :disabled="busy || (frequency === 'weekly' && !draft.days.length)"
            >
              {{ busy ? 'Saving…' : 'Save Automation' }}
            </button>
          </footer>
        </form>
      </div>
    </Modal>
  </section>
</template>
<style scoped>
/* Hallmark · pre-emit critique: P4 H4 E4 S4 R5 V3 · existing Orchestra tokens. */
.automations-page {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: 14px;
  padding: 24px;
  min-height: 0;
  min-width: 0;
  color: var(--text);
  background: var(--canvas);
}
.automations-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  flex-wrap: wrap;
}
.automations-heading h1 {
  font-size: 16px;
  line-height: 22px;
  margin: 0;
  font-weight: 600;
}
.automations-heading .standard-button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}
.automations-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.automation-search {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--rule);
  border-radius: 7px;
  padding: 0 10px;
  background: var(--surface);
  color: var(--muted);
  flex: 0 1 320px;
  min-width: 0;
}
.automation-search:focus-within {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}
.automation-search input {
  border: 0;
  outline: 0;
  box-shadow: none;
  background: transparent;
  padding: 8px 0;
  width: 100%;
  min-width: 0;
}
.automation-list {
  flex: 1;
  min-height: 0;
  overflow: auto;
  border-top: 1px solid var(--rule);
}
.automation-list.is-empty {
  border-top: 0;
}
.automation-templates {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 280px), 1fr));
  gap: 12px;
  width: 100%;
  max-width: 1080px;
  margin: 0;
}
.automation-template-heading {
  width: 100%;
  max-width: 1080px;
  margin-top: 28px;
  text-align: left;
  color: var(--muted);
  font-size: 12px;
}
.automation-template {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
  padding: 18px;
  border: 1px solid var(--rule);
  border-radius: 10px;
  background: var(--surface);
  color: var(--text);
  text-align: left;
  overflow-wrap: anywhere;
}
.automation-template:not(:disabled):hover {
  border-color: var(--accent);
  background: var(--selection);
}
.automation-template:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}
.automation-template-name {
  font-size: 13px;
  font-weight: 600;
}
.automation-template-description,
.automation-template-schedule {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.5;
}
.automation-template-schedule {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-top: auto;
  padding-top: 4px;
}
.automation-row {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) minmax(130px, 170px) minmax(130px, 160px) auto;
  align-items: center;
  gap: 20px;
  padding: 16px 0;
  border-bottom: 1px solid var(--rule);
}
.automation-identity {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-width: 0;
}
.automation-agent {
  width: 20px;
  height: 20px;
  flex-shrink: 0;
  margin-top: 2px;
}
.automation-details {
  flex: 1;
  min-width: 0;
}
.automation-title {
  display: flex;
  align-items: baseline;
  gap: 10px;
  flex-wrap: wrap;
}
.automation-title h2 {
  font-size: 13px;
  line-height: 19px;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.automation-status,
.automation-context,
.automation-schedule,
.automation-next-run,
.automations-footnote,
.automation-notice {
  color: var(--muted);
  font-size: 12px;
}
.automation-context {
  margin-top: 3px;
  overflow-wrap: anywhere;
}
.automation-schedule {
  display: flex;
  align-items: center;
  gap: 5px;
  overflow-wrap: anywhere;
  line-height: 1.5;
}
.automation-schedule .symbol {
  flex-shrink: 0;
}
.automation-next-run {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.automation-next-run time {
  color: var(--text);
  line-height: 1.5;
}
.automation-menu {
  min-width: 150px;
}
.automation-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.automation-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 56px 24px;
  text-align: center;
  min-height: min(340px, 100%);
}
.automation-empty > .symbol,
.automation-empty p {
  color: var(--muted);
}
.automation-empty h2 {
  font-size: 15px;
  font-weight: 600;
}
.automation-empty p {
  max-width: 360px;
  line-height: 1.5;
}
.automations-footnote {
  line-height: 1.5;
}
.automation-editor {
  width: min(640px, calc(100vw - 48px));
  min-width: 0;
  padding: 24px;
}
.automation-editor header,
.automation-editor > p {
  margin-bottom: 16px;
}
.automation-editor header p {
  color: var(--muted);
  margin-top: 8px;
}
.automation-flow {
  display: grid;
  gap: 20px;
}
.automation-step {
  position: relative;
}
.automation-step > legend {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 5px;
}
.automation-step-help,
.automation-timezone {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.5;
  font-weight: 400;
}
.automation-step > .automation-step-help {
  margin-bottom: 10px;
}
.automation-trigger-card,
.automation-action-card {
  border: 1px solid var(--rule);
  border-radius: 10px;
  padding: 16px;
  background: var(--surface);
}
.automation-trigger-card {
  display: grid;
  gap: 14px;
}
.automation-trigger-line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}
.automation-trigger-line > .symbol {
  color: var(--muted);
  flex-shrink: 0;
}
.automation-editor .theme-select {
  width: 100%;
  min-width: 0;
}
.automation-trigger-line .automation-schedule-picker {
  width: 190px;
  max-width: 100%;
  flex-shrink: 1;
}
.automation-trigger-line .automation-token {
  width: auto;
  min-width: 0;
  max-width: 100%;
  color: var(--text);
  background: var(--canvas);
  border: 1px solid var(--rule);
  border-radius: 8px;
  padding: 5px 10px;
  font: inherit;
}
.automation-action-card {
  display: grid;
  gap: 16px;
}
.automation-context-fields {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 14px;
}
.automation-agent-picker img {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
  pointer-events: none;
}
.automation-agent-picker select {
  padding-left: 36px;
}
.automation-date {
  max-width: 240px;
}
.automation-editor .weekday-option {
  padding: 6px 8px;
  border: 1px solid var(--rule);
  border-radius: 6px;
  font-weight: 400;
}
.weekday-option:has(input:checked) {
  background: var(--selection);
}
.automation-editor textarea {
  background: var(--canvas);
  font-weight: 400;
  line-height: 1.5;
}
@media (max-width: 760px) {
  .automation-context-fields {
    grid-template-columns: minmax(0, 1fr);
  }
  .automation-trigger-card,
  .automation-action-card {
    padding: 12px;
  }
}

.automation-editor footer {
  margin-top: 20px;
  flex-wrap: wrap;
}
.automation-enabled {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.automation-editor .weekdays label {
  flex-direction: row;
}
.weekdays legend {
  margin-bottom: 8px;
}
fieldset {
  border: 0;
  padding: 0;
  margin: 0;
  min-width: 0;
}
textarea {
  width: 100%;
  background: var(--surface);
  color: var(--text);
  border: 1px solid var(--rule);
  border-radius: 6px;
  padding: 7px;
}
textarea {
  resize: vertical;
}
.weekdays {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.weekdays label {
  display: flex;
  align-items: center;
  gap: 5px;
}
.weekdays input {
  width: auto;
  margin: 0;
}
.error {
  color: var(--status-deleted);
  overflow-wrap: anywhere;
}
@media (max-width: 1150px) {
  .automation-row {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: 10px 16px;
  }
  .automation-identity {
    grid-column: 1;
  }
  .automation-actions {
    grid-column: 2;
    grid-row: 1;
  }
  .automation-schedule {
    grid-column: 1;
    padding-left: 30px;
  }
  .automation-next-run {
    grid-column: 1 / -1;
    padding-left: 30px;
    flex-direction: row;
    flex-wrap: wrap;
    gap: 6px;
  }
}
@media (max-width: 760px) {
  .automations-page {
    padding: 22px 16px 12px;
    gap: 14px;
  }
  .automation-row {
    padding: 14px 0;
  }
}
</style>
