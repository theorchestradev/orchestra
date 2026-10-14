<script setup>
import { computed, ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { call, on, copy, preview } from './shared/api'
import { mac } from './shared/platform'
import { appCommand } from './shared/shortcuts'
import { gitHubItem } from './shared/github'
import { projectTitle } from './shared/project'
import { findProfile, profileName, sessionProfile } from './shared/profiles'
import {
  palette,
  effectiveAppearance,
  agents,
  agentIcon,
  destination,
  adjacentItem,
  startupDestination,
  setSystemPalettes,
  setCustomThemes,
  customThemes,
  missingCustomTheme,
  accentForeground,
  diffTokens,
  translucencyTokens,
  activityLabel,
  visibleSidebarProjects,
  isTerminal,
  projectRows,
} from './shared/appearance'
import Icon from './components/Icon.vue'
import Modal from './components/Modal.vue'
import EmojiPicker from './components/EmojiPicker.vue'
import Settings from './components/Settings.vue'
import UsageConsent from './components/UsageConsent.vue'
import { track } from './shared/telemetry'
import CommandBar from './components/CommandBar.vue'
import Scratchpad from './components/Scratchpad.vue'
import TerminalView from './components/Terminal.vue'
import GitHub from './components/GitHub.vue'
import Messages from './components/Messages.vue'
import StatusBar from './components/StatusBar.vue'
import SidebarList from './components/SidebarList.vue'
import ArcSpaces from './components/ArcSpaces.vue'
import ArcProjectPager from './components/ArcProjectPager.vue'
import SessionDiff from './components/SessionDiff.vue'
import UpdateDialog from './components/UpdateDialog.vue'
import { pendingUpdate } from './shared/update'
import { sessionsNeedingYou, attentionSound } from './shared/attention'
const workspace = ref({
  projects: [],
  profiles: [],
  runs: [],
  preferences: {
    appearance: 'system',
    appIcon: 'white',
    theme: 'system',
    terminalFontFamily: '',
    terminalFontSize: 14,
    terminalFontSmoothing: true,
    terminalPaddingX: 12,
    terminalPaddingY: 8,
    sidebarWidth: 280,
    diffPaneWidth: 440,
    diffPaneOpen: false,
    arcMode: false,
    defaultAgent: 'claude',
    startupView: 'last',
    confirmQuit: true,
    confirmSessionClose: true,
    gitHubRefreshInterval: 0,
    deliverRequests: false,
    attentionSound: false,
  },
})
// Update checks run in Go; the status bar offers a found release unless it was skipped.
const updateStatus = ref(null)
const update = computed(() => pendingUpdate(updateStatus.value, preferences.value))
const loading = ref(true),
  error = ref(''),
  modal = ref(''),
  menu = ref(null),
  busy = ref(false)
const projectName = ref(''),
  projectDescription = ref(''),
  projectEmoji = ref(''),
  directory = ref(''),
  renameID = ref(''),
  renameValue = ref(''),
  renameKind = ref(''),
  sessionID = ref(''),
  sessionProjectID = ref(''),
  // The profile a new session or an edited project uses; "" is Default.
  sessionProfileID = ref(''),
  projectProfileID = ref(''),
  formError = ref('')
const visitedTerminals = ref(new Set()),
  visitedGitHub = ref(new Set()),
  terminalVersions = ref({}),
  sidebarHidden = ref(false)
const scratchpadView = ref(null),
  quitting = ref(false),
  newWorktree = ref(true),
  removeWorktree = ref(false)
const activeSessionCount = computed(
  () => workspace.value.projects.flatMap((p) => p.sessions).filter((s) => s.running).length,
)
const sessionAgents = computed(() =>
  [...agents].sort(
    (a, b) => Number(b.id === preferences.value.defaultAgent) - Number(a.id === preferences.value.defaultAgent),
  ),
)
const scratchpadProjectID = ref('')
const scratchpadProject = computed(() => workspace.value.projects.find((p) => p.id === scratchpadProjectID.value))
const removalProjectID = ref('')
const removalProject = computed(() => workspace.value.projects.find((p) => p.id === removalProjectID.value))
const project = computed(() => workspace.value.projects.find((p) => p.id === workspace.value.selectedProjectID))
const currentDestination = computed(() => destination(project.value))
const session = computed(() => project.value?.sessions.find((s) => s.id === currentDestination.value))
const modalSession = computed(() =>
  workspace.value.projects.flatMap((p) => p.sessions).find((s) => s.id === sessionID.value),
)
const modalProject = computed(
  () => workspace.value.projects.find((p) => p.id === sessionProjectID.value) || project.value,
)
const preferences = computed(() => workspace.value.preferences)
// Views get the appearance shown, so System mode needs no handling outside Settings.
const shownPreferences = computed(() => ({
  ...preferences.value,
  appearance: effectiveAppearance(preferences.value.appearance),
}))
const telemetryStatus = ref({ available: false, reason: '', debug: false }),
  telemetryBusy = ref(false)
let telemetryRequest = Promise.resolve()
async function chooseTelemetry(enabled) {
  if (telemetryBusy.value) return
  telemetryBusy.value = true
  try {
    telemetryRequest = call('SetTelemetryConsent', enabled)
    apply(await telemetryRequest)
  } catch (e) {
    error.value = String(e)
  } finally {
    telemetryBusy.value = false
  }
}
watch(error, (value) => {
  if (value) track('error_shown', { code: 'operation_failed' })
})
// Custom themes are read again whenever a theme picker opens or the window gains focus.
const themeErrors = ref([]),
  themeDirectory = ref(''),
  themesLoaded = ref(false)
async function loadCustomThemes() {
  try {
    const result = await call('CustomThemes')
    setCustomThemes(result?.themes)
    themeErrors.value = result?.errors || []
    themeDirectory.value = result?.directory || ''
    themeErrors.value.forEach((e) => console.warn('Custom theme skipped: ' + e))
    themesLoaded.value = true
  } catch (e) {
    console.warn('Could not load custom themes: ' + e)
  }
}
watch(modal, (kind) => (kind === 'settings' || kind === 'command') && loadCustomThemes())
// Duplicate as Custom Theme wrote a new folder; load it before selecting, so the selection is not
// treated as a missing theme.
async function selectDuplicatedTheme(theme) {
  await loadCustomThemes()
  changePreferences({ ...preferences.value, theme: theme.id })
}
watch([() => preferences.value.theme, customThemes, themesLoaded, loading], () => {
  if (themesLoaded.value && !loading.value && missingCustomTheme(preferences.value.theme))
    changePreferences({ ...preferences.value, theme: 'system' })
})
// Translucency is in effect only while macOS Reduce Transparency is off, which can change while the
// app is in the background, so it is checked again when the window gains focus.
const translucent = ref(false),
  reduceTransparency = ref(false)
async function applyTranslucency() {
  const wanted = Boolean(preferences.value.translucent)
  try {
    translucent.value = await call('WindowTranslucency', wanted)
    reduceTransparency.value = wanted && !translucent.value
  } catch (e) {
    translucent.value = reduceTransparency.value = false
    console.warn('Could not change window translucency: ' + e)
  }
}
watch(() => preferences.value.translucent, applyTranslucency)
const appearance = computed(() => {
  const colors = palette(shownPreferences.value),
    dark = shownPreferences.value.appearance === 'dark'
  return {
    ...Object.fromEntries(Object.entries(colors).map(([key, value]) => ['--' + key, value])),
    ...diffTokens(shownPreferences.value),
    ...translucencyTokens(preferences.value.windowOpacity),
    '--on-accent': accentForeground(colors.accent),
    '--control-thumb': palette({ theme: 'system', appearance: 'light' }).canvas,
    '--status-open': dark ? '#4ade80' : '#15803d',
    '--status-merged': dark ? '#c4a0ff' : '#8250df',
    '--status-deleted': dark ? '#f87171' : '#cf222e',
  }
})
const sidebarProjects = computed(() =>
  visibleSidebarProjects(workspace.value.projects, workspace.value.selectedProjectID, preferences.value.arcMode),
)
const sidebarWidth = computed(() => preferences.value.sidebarWidth || 280)
const showsDiff = computed(() => Boolean(preferences.value.diffPaneOpen && session.value))
// Codex reports a user interrupt as waiting, so a session the user just typed in stays quiet.
const lastKey = {}
const chime = attentionSound(() => new Audio('/sounds/attention.wav').play().catch(() => {}))
function announce(previous, next) {
  if (!preferences.value.attentionSound) return
  const watching = (s) => s.id === session.value?.id && !modal.value && document.hasFocus()
  if (sessionsNeedingYou(previous, next, (s) => watching(s) || Date.now() - (lastKey[s.id] || 0) < 2000).length) chime()
}
function apply(state) {
  if (state?.projects) {
    const localPrefs = workspace.value.preferences
    announce(workspace.value.projects, state.projects)
    workspace.value = { ...state, profiles: state.profiles || [], runs: state.runs || [] }
    if (preferencePending)
      workspace.value.preferences = {
        ...localPrefs,
        telemetryEnabled: state.preferences.telemetryEnabled,
        telemetryChoiceMade: state.preferences.telemetryChoiceMade,
        telemetryInstallID: state.preferences.telemetryInstallID,
      }
  }
  return state
}
async function action(method, ...args) {
  try {
    return apply(await call(method, ...args))
  } catch (e) {
    error.value = String(e)
    return null
  }
}
function openModal(kind, p = project.value) {
  if (kind === 'command' && modal.value !== kind) track('feature_used', { feature: 'command_bar' })
  menu.value = null
  formError.value = ''
  modal.value = kind
  if (kind === 'project') {
    projectName.value = ''
    projectDescription.value = ''
    directory.value = ''
  }
  if (kind === 'session') {
    newWorktree.value = true
    if (p) sessionProjectID.value = p.id
    sessionProfileID.value = findProfile(workspace.value.profiles, p?.profileID) ? p.profileID : ''
  }
}
// The status bar toggle and the command switch to the opposite of what is shown, leaving System.
function toggleAppearance() {
  changePreferences({
    ...preferences.value,
    appearance: shownPreferences.value.appearance === 'light' ? 'dark' : 'light',
  })
}
function command(kind) {
  if (kind === 'previous' || kind === 'next') {
    navigate(kind === 'next' ? 1 : -1)
    return
  }
  if (kind === 'command' && modal.value === 'command') {
    closeModal()
    return
  }
  if (kind === 'sidebar') {
    toggleSidebar()
    return
  }
  if (kind === 'diff') {
    toggleDiff()
    return
  }
  if (kind === 'update') {
    checkForUpdates()
    return
  }
  if (kind === 'session' && !project.value) return
  openModal(kind)
}
async function runCommand({ command: kind, item }) {
  closeModal()
  if (kind === 'goto') return select(item.project, item.destination)
  if (kind === 'github' || kind === 'scratchpad') return select(project.value, kind)
  if (kind === 'theme') return changePreferences({ ...preferences.value, theme: item.theme })
  if (kind === 'appearance') return toggleAppearance()
  if (kind === 'terminal') return newTerminal(project.value)
  if (kind === 'session' || kind === 'worktreeSession') {
    sessionProjectID.value = project.value.id
    await createSession(item.agent, '', kind === 'worktreeSession')
    if (formError.value) {
      error.value = formError.value
      formError.value = ''
    }
    return
  }
  await nextTick()
  command(kind)
}
function navigate(step) {
  if (modal.value) return
  const item = adjacentItem(
    preferences.value.arcMode ? sidebarProjects.value.map((p) => ({ ...p, isExpanded: true })) : sidebarProjects.value,
    project.value?.id,
    currentDestination.value,
    step,
  )
  if (item) select(item.project, item.destination)
}
function toggleSidebar() {
  if (modal.value) return
  const hiding = !sidebarHidden.value,
    inSidebar = document.activeElement?.closest('.sidebar')
  sidebarHidden.value = hiding
  if (hiding && inSidebar) nextTick(() => document.querySelector('.show-sidebar')?.focus())
}
// The pane's open state is a preference, so it follows the user across sessions and restarts.
function toggleDiff() {
  if (modal.value) return
  const closing = preferences.value.diffPaneOpen,
    inPane = document.activeElement?.closest('.diff-pane')
  changePreferences({ ...preferences.value, diffPaneOpen: !closing })
  if (!closing) track('feature_used', { feature: 'diff_view' })
  if (closing && inPane) closeModal()
}
function closeModal() {
  modal.value = ''
  formError.value = ''
  nextTick(() => document.querySelector('.terminal-pane:not([style*="display: none"]) .xterm-helper-textarea')?.focus())
}
async function select(p, dest) {
  menu.value = null
  const state = await action('Select', p.id, dest)
  if (dest === 'github') visitedGitHub.value.add(p.id)
  const sess = p.sessions.find((s) => s.id === dest)
  if (sess) {
    visitedTerminals.value.add(sess.id)
    if (sess.restored && (!sess.hasLaunched || sess.agentSessionID || isTerminal(sess))) await start(sess, false)
  }
  return state
}
async function start(sess, newConversation) {
  busy.value = true
  try {
    await call('StartSession', sess.id, newConversation, 100, 32)
    terminalVersions.value[sess.id] = (terminalVersions.value[sess.id] || 0) + 1
    visitedTerminals.value.add(sess.id)
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
async function chooseFolder() {
  try {
    const result = await call('ChooseFolder')
    if (result) {
      directory.value = result
      if (!projectName.value.trim()) projectName.value = result.split('/').filter(Boolean).at(-1)
    }
  } catch (e) {
    formError.value = String(e)
  }
}
async function createProject() {
  busy.value = true
  try {
    apply(await call('CreateProject', projectName.value, directory.value, projectDescription.value))
    closeModal()
    await detectAll()
  } catch (e) {
    formError.value = String(e)
  } finally {
    busy.value = false
  }
}
async function createSession(
  agent,
  prompt = '',
  createWorktree = false,
  name = '',
  source = null,
  model = '',
  effort = '',
  profileID = (modalProject.value || project.value)?.profileID || '',
) {
  busy.value = true
  try {
    const state = apply(
      await call(
        'CreateSession',
        modalProject.value?.id || project.value.id,
        agent,
        name,
        prompt,
        createWorktree,
        source,
        model,
        effort,
        profileID,
      ),
    )
    visitedTerminals.value.add(state.projects.find((p) => p.id === state.selectedProjectID).selectedSessionID)
    closeModal()
  } catch (e) {
    formError.value = String(e)
  } finally {
    busy.value = false
  }
}
async function newTerminal(p) {
  menu.value = null
  sessionProjectID.value = p.id
  await createSession('terminal')
  if (formError.value) {
    error.value = formError.value
    formError.value = ''
  }
}
async function workOn(item, choice = {}) {
  if (busy.value) return
  sessionProjectID.value = project.value.id
  const url = item.pull_request?.html_url || item.html_url
  // Agents often read only the description, so ask for the discussion explicitly.
  await createSession(
    choice.agent || preferences.value.defaultAgent,
    item.pull_request
      ? `Review this GitHub pull request: ${url}. First read its description, comments and reviews (gh pr view ${url} --json title,body,comments,reviews) and its inline review comments (gh api ${new URL(url).pathname.slice(1).replace(/^([^/]+\/[^/]+)\/pull\//, 'repos/$1/pulls/')}/comments).`
      : `Work on this GitHub issue: ${url}. First read the issue and all of its comments (gh issue view ${url} --json title,body,comments).`,
    true,
    `#${item.number} ${item.title || ''}`,
    gitHubItem(item),
    choice.model,
    choice.effort,
  )
  if (formError.value) {
    error.value = formError.value
    formError.value = ''
  }
}
function edit(id, name, kind, description = '', emoji = '', profileID = '') {
  projectProfileID.value = profileID
  renameID.value = id
  renameValue.value = name
  renameKind.value = kind
  projectDescription.value = description
  projectEmoji.value = emoji
  openModal('rename')
}
async function rename() {
  busy.value = true
  try {
    let state = await (renameKind.value === 'Project'
      ? call('EditProject', renameID.value, renameValue.value, projectDescription.value, projectEmoji.value)
      : call(renameKind.value === 'Scratchpad' ? 'RenameScratchpad' : 'Rename', renameID.value, renameValue.value))
    const edited = state.projects?.find((p) => p.id === renameID.value)
    if (renameKind.value === 'Project' && edited && (edited.profileID || '') !== projectProfileID.value)
      state = await call('SetProjectProfile', renameID.value, projectProfileID.value)
    apply(state)
    closeModal()
  } catch (e) {
    formError.value = String(e)
  } finally {
    busy.value = false
  }
}
function scratchpadMenu(event, p) {
  event.preventDefault()
  const rect = event.currentTarget.getBoundingClientRect()
  menu.value = {
    kind: 'scratchpad',
    project: p,
    x: Math.min(event.clientX ?? rect.left, innerWidth - 230),
    y: Math.min(event.clientY ?? rect.bottom, innerHeight - 200),
  }
}
function deleteScratchpad(p) {
  scratchpadProjectID.value = p.id
  openModal('deleteScratchpad')
}
function removeProject(p) {
  removalProjectID.value = p.id
  openModal('removeProject')
}
async function confirmRemoveProject() {
  const p = removalProject.value
  if (!p || busy.value) return
  busy.value = true
  try {
    apply(await call('RemoveProject', p.id))
    visitedGitHub.value.delete(p.id)
    for (const sess of p.sessions) {
      visitedTerminals.value.delete(sess.id)
      delete terminalVersions.value[sess.id]
    }
    closeModal()
  } catch (e) {
    formError.value = String(e)
  } finally {
    busy.value = false
  }
}
async function confirmDeleteScratchpad() {
  busy.value = true
  try {
    scratchpadDrafts.delete(scratchpadProjectID.value)
    apply(await call('DeleteScratchpad', scratchpadProjectID.value))
    closeModal()
  } catch (e) {
    formError.value = String(e)
  } finally {
    busy.value = false
  }
}
function sessionMenu(event, p, sess) {
  event.preventDefault()
  const rect = event.currentTarget.getBoundingClientRect()
  menu.value = {
    kind: 'session',
    project: p,
    session: sess,
    x: Math.min(event.clientX ?? rect.left, innerWidth - 230),
    y: Math.min(event.clientY ?? rect.bottom, innerHeight - 330),
  }
}
function projectMenu(event, p, kind) {
  const rect = event.currentTarget.getBoundingClientRect()
  menu.value = {
    kind,
    project: p,
    x: Math.min(rect.left, innerWidth - 230),
    y: Math.min(
      rect.bottom + 3,
      innerHeight -
        (kind === 'project'
          ? 200 + workspace.value.profiles.length * 30 + (workspace.value.profiles.length ? 70 : 0)
          : 150),
    ),
  }
}
// Signing in runs in a terminal session; Settings closes so the user can complete it there.
function loggingIn(state) {
  apply(state)
  visitedTerminals.value.add(state.projects.find((p) => p.id === state.selectedProjectID).selectedSessionID)
  closeModal()
}
function messages(p, sess) {
  sessionID.value = sess.id
  sessionProjectID.value = p.id
  openModal('messages')
}
function openInbox() {
  const pending = (sess) =>
    workspace.value.runs.some((r) => r.toSessionID === sess.id && ['queued', 'needsAttention'].includes(r.status))
  const sessions = workspace.value.projects.flatMap((p) => p.sessions.map((sess) => ({ p, sess })))
  const target =
    sessions.find(({ sess }) => sess.id === session.value?.id && pending(sess)) ||
    sessions.find(({ sess }) => pending(sess))
  if (target) messages(target.p, target.sess)
}
async function closeSession(p, sess) {
  menu.value = null
  sessionID.value = sess.id
  sessionProjectID.value = p.id
  removeWorktree.value = preferences.value.confirmSessionClose
  if (preferences.value.confirmSessionClose) openModal('close')
  else await confirmClose()
}
async function confirmClose() {
  const id = sessionID.value
  const result = await action('CloseSession', id, removeWorktree.value)
  if (result) {
    visitedTerminals.value.delete(id)
    closeModal()
  }
}
let scratchpadQueue = Promise.resolve(true)
// Saves queued from one editor share a base revision; each successful save forwards it.
const scratchpadForwards = new Map()
// Edits rejected after their editor closed reopen in the editor instead of being lost.
const scratchpadDrafts = new Map()
async function saveScratchpad(id, text, base) {
  let revision = base.revision
  const p = workspace.value.projects.find((p) => p.id === id)
  if (!p?.hasScratchpad) return true
  p.scratchpadText = text
  scratchpadQueue = scratchpadQueue
    .then(() => {
      while (scratchpadForwards.has(id + ':' + revision)) revision = scratchpadForwards.get(id + ':' + revision)
      return call('SaveScratchpad', id, text, revision)
    })
    .then((saved) => {
      if (saved !== revision) scratchpadForwards.set(id + ':' + revision, saved)
      const current = workspace.value.projects.find((p) => p.id === id)
      if (current && saved > (current.scratchpadRevision || 0)) current.scratchpadRevision = saved
      return true
    })
    .catch(async (e) => {
      if (!workspace.value.projects.find((p) => p.id === id)?.hasScratchpad) return true
      // An agent changed the Scratchpad first. The editor reconciles with the stored text.
      if (!String(e).includes('changed since it was read')) error.value = String(e)
      else {
        if (scratchpadView.value?.projectID !== id) scratchpadDrafts.set(id, { text, base })
        apply(await call('Workspace').catch(() => null))
      }
      return false
    })
  return scratchpadQueue
}
function requestQuit(state) {
  if (state?.projects) apply(state)
  if (quitting.value || modal.value === 'quit') return
  if (preferences.value.confirmQuit && activeSessionCount.value) {
    openModal('quit')
    return
  }
  finishQuit()
}
async function finishQuit() {
  if (quitting.value) return
  quitting.value = true
  try {
    const draft = scratchpadView.value?.takeDraft()
    if (draft && !(await saveScratchpad(draft.id, draft.text, draft.base))) return
    if (!(await scratchpadQueue)) return
    await preferenceQueue
    await telemetryRequest
    await call('ConfirmQuit')
  } catch (e) {
    error.value = String(e)
  } finally {
    quitting.value = false
  }
}

let preferenceQueue = Promise.resolve()
function changePreferences(value) {
  workspace.value.preferences = { ...value }
  preferencePending++
  preferenceQueue = preferenceQueue
    .then(() => call('SetPreferences', value))
    .catch((e) => (error.value = String(e)))
    .finally(() => preferencePending--)
}
// The dialog shows the check's progress and result; automatic checks only update the status bar.
function checkForUpdates() {
  openModal('update')
  call('CheckForUpdates')
    .then((status) => (updateStatus.value = status))
    .catch((e) => (error.value = String(e)))
}
// Install When I Quit stages the verified update; Install and Restart then quits through the
// normal flow, which settles pending writes before Go stops sessions and swaps the app.
async function installUpdate(restart) {
  const version = updateStatus.value?.available?.version
  let status
  try {
    status = await call('InstallUpdate', version, restart)
  } catch (e) {
    error.value = String(e)
    return
  }
  updateStatus.value = status
  if (status.staged !== version) return
  closeModal()
  if (restart) finishQuit()
}
function skipUpdate(version) {
  changePreferences({ ...preferences.value, skippedUpdate: version })
  closeModal()
}
// Turning Beta on or off changes which releases count, so check again once it is saved.
watch(
  () => preferences.value.betaUpdates,
  (_, previous) => {
    if (previous === undefined || !updateStatus.value?.supported) return
    preferenceQueue
      .then(() => call('CheckForUpdates'))
      .then((status) => (updateStatus.value = status))
      .catch(() => {})
  },
)
function menuKeys(event) {
  if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault()
    const items = [...event.currentTarget.querySelectorAll('button:not(:disabled)')]
    const index = items.indexOf(document.activeElement)
    const next =
      event.key === 'Home'
        ? 0
        : event.key === 'End'
          ? items.length - 1
          : (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length
    items[next]?.focus()
  }
}
watch(menu, async (value) => {
  if (value) {
    await nextTick()
    document.querySelector('.context-menu button:not(:disabled)')?.focus()
  }
})
let unsubs = [],
  resizeCleanup
// Store owns persisted sidebar order; pointer dragging only previews it.
function orderOf(kind, group) {
  const p = workspace.value.projects.find((p) => p.id === group)
  return (kind === 'project' ? workspace.value.projects : p ? projectRows(p) : []).map((item) => item.id)
}
function move(kind, id, index, group) {
  return kind === 'project'
    ? action('MoveProject', id, index)
    : id === 'scratchpad'
      ? action('MoveScratchpad', group, index)
      : action('MoveSession', id, index)
}
function moveBy(kind, group, id, step) {
  menu.value = null
  const ids = orderOf(kind, group),
    index = ids.indexOf(id) + step
  if (index >= 0 && index < ids.length) move(kind, id, index, group)
}
function dragSidebar(event) {
  const start = event.clientX,
    width = sidebarWidth.value
  const move = (e) => {
    workspace.value.preferences.sidebarWidth = Math.max(220, Math.min(400, width + e.clientX - start))
  }
  const stop = () => {
    removeEventListener('pointermove', move)
    removeEventListener('pointerup', stop)
    changePreferences({ ...preferences.value })
    resizeCleanup = null
  }
  addEventListener('pointermove', move)
  addEventListener('pointerup', stop)
  resizeCleanup = stop
}
function keys(event) {
  if (event.key === 'Escape') {
    menu.value = null
    if (!modal.value) error.value = ''
    return
  }
  if (!event.metaKey && !event.ctrlKey) return
  if (preview && event.key.toLowerCase() === 'q') {
    event.preventDefault()
    requestQuit()
    return
  }
  const id = appCommand(event)
  if (id) {
    event.preventDefault()
    command(id)
  }
}
async function detectAll() {
  await Promise.allSettled(workspace.value.projects.map((p) => action('DetectRepository', p.id)))
}
watch(
  currentDestination,
  (value) => {
    if (value === 'github' && project.value) visitedGitHub.value.add(project.value.id)
    if (session.value && !session.value.restored) visitedTerminals.value.add(session.value.id)
  },
  { immediate: true },
)
onMounted(async () => {
  unsubs = [
    on('workspace', apply),
    on('command', command),
    on('quit-request', requestQuit),
    on('update', (status) => (updateStatus.value = status)),
  ]
  addEventListener('keydown', keys)
  addEventListener('focus', loadCustomThemes)
  addEventListener('focus', applyTranslucency)
  try {
    setSystemPalettes(await call('SystemPalettes'))
    await loadCustomThemes()
    apply(await call('Workspace'))
    call('UpdateStatus')
      .then((status) => (updateStatus.value = status))
      .catch(() => {})
    telemetryStatus.value = await call('TelemetryStatus')
    await detectAll()
    const initialView = startupDestination(project.value, preferences.value)
    if (initialView) await select(project.value, initialView)
    if (currentDestination.value === 'github' && project.value) visitedGitHub.value.add(project.value.id)
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
})
// Preferences update optimistically; backend serializes writes and publishes the same values.
let preferencePending = 0
onBeforeUnmount(() => {
  unsubs.forEach((fn) => fn?.())
  removeEventListener('keydown', keys)
  removeEventListener('focus', loadCustomThemes)
  removeEventListener('focus', applyTranslucency)
  resizeCleanup?.()
})
</script>
<template>
  <div
    class="app-shell"
    :class="{
      'chrome-over-overlay': modal === 'settings' || modal === 'command',
      translucent,
      'native-titlebar': !mac,
      'has-usage-consent': !loading && !preferences.telemetryChoiceMade,
    }"
    :style="{ ...appearance, colorScheme: shownPreferences.appearance, '--sidebar-width': sidebarWidth + 'px' }"
    :data-appearance="shownPreferences.appearance"
    :data-theme="preferences.theme"
    @pointerdown="menu = null"
  >
    <aside v-show="!sidebarHidden" class="sidebar" :class="{ 'arc-sidebar': preferences.arcMode }">
      <div class="sidebar-titlebar" data-wails-drag>
        <span v-if="preview" class="preview-label">Visual preview</span
        ><button
          class="icon-button sidebar-toggle"
          aria-label="Hide Sidebar"
          title="Hide Sidebar"
          @click="toggleSidebar"
        >
          <Icon name="sidebar.left" :size="16" />
        </button>
      </div>
      <div v-if="!preferences.arcMode" class="sidebar-heading">
        <strong>Projects</strong
        ><button class="icon-button" aria-label="New Project" title="New Project" @click="openModal('project')">
          <Icon name="plus" :size="16" />
        </button>
      </div>
      <div class="sidebar-projects" :class="{ 'project-list': !preferences.arcMode }">
        <component
          :is="preferences.arcMode ? ArcProjectPager : SidebarList"
          :items="preferences.arcMode ? workspace.projects : sidebarProjects"
          :selected-id="sidebarProjects[0]?.id"
          handle=".project-title"
          kind="project-sort-list"
          :reorder="(id, index) => move('project', id, index)"
          :select-project="preferences.arcMode ? (p) => select(p, 'project') : undefined"
          :create-project="preferences.arcMode ? () => openModal('project') : undefined"
          :open-settings="preferences.arcMode ? () => openModal('settings') : undefined"
          ><template #default="{ item: p }"
            ><section class="project-group">
              <div class="project-heading">
                <button
                  class="project-title"
                  :title="p.description ? p.description + '\n\n' + p.directory : p.directory"
                  @click="select(p, 'project')"
                >
                  <span v-if="p.emoji" class="project-emoji" aria-hidden="true">{{ p.emoji }}</span
                  ><Icon v-else :name="p.isExpanded ? 'folder' : 'folder.fill'" :size="16" /><span>{{
                    p.name
                  }}</span></button
                ><button
                  v-if="!preferences.arcMode"
                  class="icon-button hover-control"
                  :aria-label="(p.isExpanded ? 'Collapse ' : 'Expand ') + p.name"
                  @click="action('ToggleProject', p.id)"
                >
                  <Icon :name="p.isExpanded ? 'chevron.down' : 'chevron.right'" :size="9" /></button
                ><span class="spacer" /><button
                  class="icon-button hover-control"
                  :aria-label="'Options for ' + p.name"
                  @pointerdown.stop
                  @click.stop="projectMenu($event, p, 'project')"
                >
                  <Icon name="ellipsis" /></button
                ><button
                  class="icon-button hover-control"
                  :aria-label="'Add to ' + p.name"
                  @pointerdown.stop
                  @click.stop="projectMenu($event, p, 'add')"
                >
                  <Icon name="plus" :size="14" />
                </button>
              </div>
              <div v-if="preferences.arcMode || p.isExpanded" class="project-children">
                <button
                  v-if="p.repository"
                  class="sidebar-row"
                  :class="{ selected: project?.id === p.id && currentDestination === 'github' }"
                  @click="select(p, 'github')"
                >
                  <Icon name="list.bullet.rectangle" :size="15" /><span>Issues &amp; PRs</span></button
                ><SidebarList
                  :items="projectRows(p)"
                  handle=".session-row,.scratchpad-row"
                  kind="session-sort-list"
                  :reorder="(id, index) => move('session', id, index, p.id)"
                  ><template #default="{ item: sess }"
                    ><button
                      v-if="sess.scratchpad"
                      class="sidebar-row scratchpad-row"
                      :class="{ selected: project?.id === p.id && currentDestination === 'scratchpad' }"
                      @click="select(p, 'scratchpad')"
                      @contextmenu="scratchpadMenu($event, p)"
                      @keydown.shift.f10.prevent="scratchpadMenu($event, p)"
                    >
                      <Icon name="note.text" :size="15" /><span class="truncate">{{
                        p.scratchpadName || 'Scratchpad'
                      }}</span></button
                    ><template v-else
                      ><button
                        class="sidebar-row session-row"
                        :class="{ selected: project?.id === p.id && currentDestination === sess.id }"
                        :title="
                          (sess.worktreeBranch ? sess.worktreeBranch + ' · ' + sess.worktreeDirectory + ' — ' : '') +
                          sess.name +
                          ' — Profile: ' +
                          profileName(workspace.profiles, sess.profileID) +
                          ' — ' +
                          (isTerminal(sess)
                            ? sess.restored
                              ? 'Select to start a new shell'
                              : 'Terminal'
                            : sess.restored
                              ? 'Select to start or resume'
                              : activityLabel(sess.activity))
                        "
                        @click="select(p, sess.id)"
                        @contextmenu="sessionMenu($event, p, sess)"
                        @keydown.shift.f10.prevent="sessionMenu($event, p, sess)"
                      >
                        <Icon v-if="isTerminal(sess)" name="terminal" :size="12" /><img
                          v-else
                          :src="agentIcon(sess.agent, shownPreferences.appearance)"
                          alt=""
                        /><span class="session-name"
                          ><span>{{ sess.name }}</span
                          ><span v-if="!sess.restored && sess.exitMessage" class="visually-hidden">, ended</span
                          ><small v-if="sess.restored">{{
                            !sess.hasLaunched || isTerminal(sess)
                              ? 'Ready to start'
                              : sess.agentSessionID
                                ? 'Ready to resume'
                                : 'Resume unavailable'
                          }}</small
                          ><small v-if="sess.worktreeBranch" class="truncate">{{ sess.worktreeBranch }}</small></span
                        ><span
                          v-if="sessionProfile(workspace.profiles, p, sess)"
                          class="profile-badge"
                          :class="{ missing: sessionProfile(workspace.profiles, p, sess).missing }"
                          role="img"
                          :aria-label="'Profile: ' + sessionProfile(workspace.profiles, p, sess).name"
                          >{{ sessionProfile(workspace.profiles, p, sess).symbol }}</span
                        ><span
                          v-if="workspace.runs.filter((r) => r.toSessionID === sess.id && r.status === 'queued').length"
                          class="count unread"
                          >{{
                            workspace.runs.filter((r) => r.toSessionID === sess.id && r.status === 'queued').length
                          }}</span
                        ><i v-if="!isTerminal(sess)" class="activity-dot" :class="sess.activity" /></button
                      ><!-- prettier-ignore-attribute @click --><button
                        v-if="sess.statusNeedsSetup"
                        class="status-help"
                        @click="sessionID=sess.id;openModal('status')"
                      >
                        Enable Codex status…
                      </button></template
                    ></template
                  ></SidebarList
                >
              </div>
            </section></template
          ></component
        >
      </div>
      <ArcSpaces
        v-if="preferences.arcMode"
        :projects="workspace.projects"
        :selected-id="sidebarProjects[0]?.id"
        :settings-open="modal === 'settings'"
        @select="select($event, 'project')"
        @options="projectMenu($event.event, $event.project, 'project')"
        @settings="openModal('settings')"
        @create="openModal('project')"
      />
      <div v-else class="sidebar-footer">
        <button class="sidebar-row" :class="{ selected: modal === 'settings' }" @click="openModal('settings')">
          <Icon name="gearshape" :size="14" />Settings
        </button>
      </div>
      <div class="sidebar-resizer" @pointerdown.prevent="dragSidebar" />
    </aside>
    <main class="content" :class="{ 'sidebar-hidden': sidebarHidden, 'modal-open': modal }">
      <div class="content-drag" data-wails-drag />
      <button
        v-if="sidebarHidden"
        class="icon-button sidebar-toggle show-sidebar"
        aria-label="Show Sidebar"
        title="Show Sidebar"
        @click="toggleSidebar"
      >
        <Icon name="sidebar.left" :size="16" />
      </button>
      <div class="content-body">
        <div v-if="loading" class="empty-state"><span class="spinner" /></div>
        <div v-else-if="!project" class="empty-state">
          <Icon name="folder" :size="48" />
          <h2>No Project Selected</h2>
          <p>Create or select a project to access its sessions.</p>
          <button class="standard-button" @click="openModal('project')">New Project</button>
        </div>
        <template v-else
          ><Scratchpad
            ref="scratchpadView"
            v-if="currentDestination === 'scratchpad'"
            :key="project.id"
            :project="project"
            :draft="scratchpadDrafts.get(project.id)"
            @vue:mounted="scratchpadDrafts.delete(project.id)"
            @save="saveScratchpad"
            @mode="(id, mode) => action('SetScratchpadMode', id, mode)"
          />
          <template v-for="p in workspace.projects" :key="p.id"
            ><div
              v-if="visitedGitHub.has(p.id) && p.repository"
              v-show="project?.id === p.id && currentDestination === 'github'"
              class="github-pane"
            >
              <GitHub
                :project="p"
                :refresh-interval="preferences.gitHubRefreshInterval"
                :default-agent="preferences.defaultAgent"
                :visible="project?.id === p.id && currentDestination === 'github' && !modal"
                @work="workOn"
                @session="(sess) => select(p, sess.id)"
                @error="error = $event"
              /></div
          ></template>
          <div v-if="currentDestination === 'empty'" class="empty-state">
            <Icon name="terminal" :size="48" />
            <h2>No Sessions</h2>
            <p>Choose an agent to start a session in this project’s folder.</p>
            <button class="standard-button" @click="openModal('session')">New Session</button>
          </div>
          <div v-if="session?.restored" class="empty-state">
            <Icon name="terminal" :size="48" />
            <h2>
              {{
                !session.hasLaunched
                  ? 'Session Ready to Start'
                  : session.agentSessionID
                    ? 'Session Ready to Resume'
                    : 'Conversation Unavailable'
              }}
            </h2>
            <p>
              {{
                !session.hasLaunched
                  ? `Start ${session.name} in ${session.worktreeBranch ? 'its worktree (' + session.worktreeBranch + ')' : 'this project’s folder'}.`
                  : session.agentSessionID
                    ? `Resume ${session.name} in ${session.worktreeBranch ? 'its worktree (' + session.worktreeBranch + ')' : 'this project’s folder'}.`
                    : 'Orchestra could not capture this agent’s conversation ID. Starting again creates a new conversation.'
              }}
            </p>
            <button
              class="standard-button"
              :disabled="busy"
              @click="start(session, session.hasLaunched && !session.agentSessionID)"
            >
              {{
                !session.hasLaunched
                  ? 'Start Session'
                  : session.agentSessionID
                    ? 'Resume Session'
                    : 'Start New Conversation'
              }}
            </button>
          </div>
        </template>
        <template v-for="p in workspace.projects" :key="'terminals-' + p.id"
          ><template v-for="sess in p.sessions" :key="sess.id"
            ><section
              v-if="visitedTerminals.has(sess.id) && !sess.restored"
              v-show="project?.id === p.id && currentDestination === sess.id"
              class="terminal-pane"
            >
              <div v-if="sess.exitMessage" class="exit-banner">
                <span>{{ sess.exitMessage }}</span
                ><button v-if="isTerminal(sess)" class="standard-button" :disabled="busy" @click="start(sess, true)">
                  Start New Shell</button
                ><template v-else
                  ><button class="standard-button" :disabled="!sess.agentSessionID || busy" @click="start(sess, false)">
                    Resume Session</button
                  ><button v-if="!sess.agentSessionID" class="standard-button" @click="start(sess, true)">
                    Start New Conversation
                  </button></template
                >
              </div>
              <TerminalView
                :key="sess.id + '-' + (terminalVersions[sess.id] || 0)"
                :session="sess"
                :preferences="shownPreferences"
                :translucent="translucent"
                :visible="project?.id === p.id && currentDestination === sess.id"
                @error="error = $event"
                @key="lastKey[sess.id] = Date.now()"
              /></section></template
        ></template>
      </div>
      <SessionDiff
        v-if="showsDiff"
        :session="session"
        :width="preferences.diffPaneWidth || 440"
        @close="toggleDiff"
        @resize="workspace.preferences.diffPaneWidth = $event"
        @resized="changePreferences({ ...preferences })"
      />
    </main>
    <UsageConsent
      v-if="!loading && !preferences.telemetryChoiceMade"
      :status="telemetryStatus"
      :busy="telemetryBusy"
      @choose="chooseTelemetry"
    />
    <StatusBar
      :projects="workspace.projects"
      :runs="workspace.runs"
      :project="project"
      :session="session"
      :profiles="workspace.profiles"
      :destination="currentDestination"
      :preferences="shownPreferences"
      :diff-open="showsDiff"
      :update="update"
      :update-ready="Boolean(updateStatus?.staged)"
      @update="openModal('update')"
      @inbox="openInbox"
      @diff="toggleDiff"
      @appearance="toggleAppearance"
    />
    <div
      v-if="menu"
      class="context-menu"
      role="menu"
      @keydown="menuKeys"
      :style="{ left: menu.x + 'px', top: menu.y + 'px' }"
      @pointerdown.stop
    >
      <template v-if="menu.kind === 'add'"
        ><button role="menuitem" @click="openModal('session', menu.project)">
          <Icon name="terminal" :size="14" />Session</button
        ><button role="menuitem" @click="newTerminal(menu.project)"><Icon name="terminal" :size="14" />Terminal</button
        ><button role="menuitem" @click="select(menu.project, 'scratchpad')">
          <Icon name="note.text" :size="14" />Scratchpad
        </button></template
      ><template v-else-if="menu.kind === 'project'"
        ><button
          role="menuitem"
          @click="
            edit(
              menu.project.id,
              menu.project.name,
              'Project',
              menu.project.description,
              menu.project.emoji,
              menu.project.profileID || '',
            )
          "
        >
          Edit Project…</button
        ><button
          role="menuitem"
          :disabled="workspace.projects[0]?.id === menu.project.id"
          @click="moveBy('project', '', menu.project.id, -1)"
        >
          Move Up</button
        ><button
          role="menuitem"
          :disabled="workspace.projects.at(-1)?.id === menu.project.id"
          @click="moveBy('project', '', menu.project.id, 1)"
        >
          Move Down
        </button>
        <template v-if="workspace.profiles.length"
          ><hr />
          <span class="menu-heading" aria-hidden="true">Profile for New Sessions</span
          ><button
            v-for="profile in [{ id: '', name: 'Default' }, ...workspace.profiles]"
            :key="profile.id"
            role="menuitemradio"
            :aria-checked="(menu.project.profileID || '') === profile.id"
            :aria-label="'Profile for new sessions: ' + profile.name"
            @click="action('SetProjectProfile', menu.project.id, profile.id).then(() => (menu = null))"
          >
            <Icon v-if="(menu.project.profileID || '') === profile.id" name="checkmark" :size="12" /><i
              v-else
              class="menu-check-space"
            /><span class="truncate">{{ profile.emoji ? profile.emoji + ' ' : '' }}{{ profile.name }}</span>
          </button></template
        >
        <hr />
        <button role="menuitem" class="destructive" @click="removeProject(menu.project)">
          <Icon name="trash" :size="14" />Remove Project…
        </button></template
      >
      <template v-else-if="menu.kind === 'scratchpad'"
        ><button
          role="menuitem"
          @click="edit(menu.project.id, menu.project.scratchpadName || 'Scratchpad', 'Scratchpad')"
        >
          Edit Scratchpad…</button
        ><button
          role="menuitem"
          :disabled="orderOf('session', menu.project.id)[0] === 'scratchpad'"
          @click="moveBy('session', menu.project.id, 'scratchpad', -1)"
        >
          Move Up</button
        ><button
          role="menuitem"
          :disabled="orderOf('session', menu.project.id).at(-1) === 'scratchpad'"
          @click="moveBy('session', menu.project.id, 'scratchpad', 1)"
        >
          Move Down
        </button>
        <hr />
        <button role="menuitem" class="destructive" @click="deleteScratchpad(menu.project)">
          <Icon name="trash" :size="14" />Delete Scratchpad…
        </button></template
      >
      <template v-else
        ><template v-if="isTerminal(menu.session)"
          ><!-- prettier-ignore-attribute @click --><button
            v-if="menu.session.activity === 'ended'"
            role="menuitem"
            @click="start(menu.session,true);menu=null"
          >
            Start New Shell
          </button></template
        ><template v-else
          ><!-- prettier-ignore-attribute @click --><button
            v-if="menu.session.activity === 'ended'"
            role="menuitem"
            :disabled="!menu.session.agentSessionID"
            @click="start(menu.session,false);menu=null"
          >
            Resume Session</button
          ><button role="menuitem" @click="messages(menu.project, menu.session)">Messages…</button></template
        ><!-- prettier-ignore-attribute @click --><button
          role="menuitem"
          @click="copy(menu.session.id).catch(e=>error=String(e));menu=null"
        >
          Copy Session ID
        </button>
        <button role="menuitem" disabled>
          <span class="truncate">Profile: {{ profileName(workspace.profiles, menu.session.profileID) }}</span>
        </button>
        <hr />
        <button role="menuitem" @click="edit(menu.session.id, menu.session.name, 'Session')">Edit Session…</button
        ><button
          role="menuitem"
          :disabled="orderOf('session', menu.project.id)[0] === menu.session.id"
          @click="moveBy('session', menu.project.id, menu.session.id, -1)"
        >
          Move Up</button
        ><button
          role="menuitem"
          :disabled="orderOf('session', menu.project.id).at(-1) === menu.session.id"
          @click="moveBy('session', menu.project.id, menu.session.id, 1)"
        >
          Move Down
        </button>
        <hr />
        <button role="menuitem" @click="closeSession(menu.project, menu.session)">
          <Icon name="trash" :size="14" />Close Session…
        </button></template
      >
    </div>
    <div v-if="error || workspace.error" class="error-toast" role="alert">
      <span>{{ error || workspace.error }}</span><!-- prettier-ignore-attribute @click --><button
        class="icon-button"
        aria-label="Dismiss error"
        @click="error='';workspace.error=''"
      >
        <Icon name="xmark" :size="12" />
      </button>
    </div>
    <Modal v-if="modal === 'settings'" kind="settings" label="Settings" @close="closeModal"
      ><Settings
        :preferences="preferences"
        :theme-errors="themeErrors"
        :theme-directory="themeDirectory"
        :reduce-transparency="reduceTransparency"
        :profiles="workspace.profiles"
        :project="project"
        :usage-profile-id="(session || project)?.profileID || ''"
        @change="changePreferences"
        @state="apply"
        @login="loggingIn"
        :telemetry-status="telemetryStatus"
        :telemetry-busy="telemetryBusy"
        @consent="chooseTelemetry"
        @duplicated="selectDuplicatedTheme"
        @check-updates="checkForUpdates"
        @close="closeModal"
    /></Modal>
    <Modal v-else-if="modal === 'command'" kind="command" label="Command Bar" @close="closeModal"
      ><CommandBar
        :preferences="shownPreferences"
        :projects="workspace.projects"
        :project="project"
        :sidebar-hidden="sidebarHidden"
        @run="runCommand"
    /></Modal>
    <Modal v-else-if="modal === 'project'" label="New Project" @close="closeModal"
      ><form class="project-dialog" @submit.prevent="createProject">
        <h2>New Project</h2>
        <label>Project Name<input v-model="projectName" autofocus placeholder="Name" aria-label="Project Name" /></label
        ><label
          >Description (optional)<textarea
            v-model="projectDescription"
            class="description-field"
            rows="3"
            maxlength="500"
            placeholder="What this project is, or why it is here"
            aria-label="Description (optional)"
          />
        </label>
        <div class="folder-field">
          <h3>Project Folder</h3>
          <p v-if="directory" class="muted">{{ directory }}</p>
          <button type="button" class="standard-button" @click="chooseFolder">
            {{ directory ? 'Change Folder…' : 'Choose Folder…' }}
          </button>
        </div>
        <p v-if="formError" class="error" role="alert">{{ formError }}</p>
        <footer>
          <button type="button" class="standard-button" @click="closeModal">Cancel</button
          ><button type="submit" class="standard-button primary" :disabled="busy || !projectName.trim() || !directory">
            Create Project
          </button>
        </footer>
      </form></Modal
    >
    <Modal v-else-if="modal === 'session'" label="New Session" @close="!busy && closeModal()"
      ><div class="session-dialog">
        <h2>New Session</h2>
        <div>
          <p>Choose an agent for {{ projectTitle(modalProject) }}.</p>
          <p class="muted caption project-path">{{ modalProject?.directory }}</p>
        </div>
        <label class="worktree-option" for="new-worktree"
          ><input
            id="new-worktree"
            v-model="newWorktree"
            type="checkbox"
            :disabled="busy"
            aria-label="Create a new worktree"
            aria-describedby="worktree-description"
          /><span
            ><strong>Create a new worktree</strong
            ><small id="worktree-description" class="muted"
              >Start on a new branch from the current commit. Uncommitted changes stay in the original folder. Closing
              the session removes the worktree unless you choose to keep it.</small
            ></span
          ></label
        >
        <div v-if="workspace.profiles.length" class="setting-row session-profile">
          <label for="session-profile">Profile</label>
          <div class="theme-select general-select">
            <select id="session-profile" v-model="sessionProfileID" :disabled="busy">
              <option value="">Default</option>
              <option v-for="profile in workspace.profiles" :key="profile.id" :value="profile.id">
                {{ profile.emoji ? profile.emoji + ' ' : '' }}{{ profile.name }}
              </option></select
            ><Icon name="chevron.up.chevron.down" :size="10" />
          </div>
        </div>
        <div class="agent-options">
          <button
            v-for="agent in sessionAgents"
            :key="agent.id"
            :autofocus="agent.id === preferences.defaultAgent"
            :disabled="busy"
            :aria-label="'Start a ' + agent.name + ' session'"
            @click="createSession(agent.id, '', newWorktree, '', null, '', '', sessionProfileID)"
          >
            <img :src="agentIcon(agent.id, shownPreferences.appearance)" alt="" /><span>{{ agent.name }}</span
            ><small v-if="agent.id === preferences.defaultAgent" class="muted caption">Default</small>
          </button>
        </div>
        <p v-if="busy && newWorktree" class="muted caption" role="status">Creating worktree…</p>
        <p v-if="formError" class="error" role="alert">{{ formError }}</p>
        <footer><button class="standard-button" :disabled="busy" @click="closeModal">Cancel</button></footer>
      </div></Modal
    >
    <Modal v-else-if="modal === 'rename'" :label="'Edit ' + renameKind" @close="closeModal"
      ><form class="rename-dialog" @submit.prevent="rename">
        <h2>Edit {{ renameKind }}</h2>
        <div v-if="renameKind === 'Project'" class="project-name-fields">
          <div class="emoji-field">
            <label for="project-emoji">Emoji</label
            ><span class="emoji-input"
              ><EmojiPicker id="project-emoji" :value="projectEmoji" @pick="projectEmoji = $event" /><button
                v-if="projectEmoji"
                type="button"
                class="icon-button"
                aria-label="Clear Emoji"
                title="Clear Emoji"
                @click="projectEmoji = ''"
              >
                <Icon name="xmark" :size="10" /></button
            ></span>
          </div>
          <label class="name-field"
            >Project Name<input v-model="renameValue" autofocus aria-label="Project Name"
          /></label>
        </div>
        <label v-else
          >{{ renameKind === 'Scratchpad' ? 'Scratchpad Name' : 'Display Name'
          }}<input
            v-model="renameValue"
            autofocus
            :aria-label="renameKind === 'Scratchpad' ? 'Scratchpad Name' : 'Display Name'" /></label
        ><label v-if="renameKind === 'Project'"
          >Description (optional)<textarea
            v-model="projectDescription"
            class="description-field"
            rows="3"
            maxlength="500"
            placeholder="What this project is, or why it is here"
            aria-label="Description (optional)"
          />
        </label>
        <div v-if="renameKind === 'Project' && workspace.profiles.length" class="setting-row session-profile">
          <span>
            <label for="project-profile">Profile</label>
            <small id="project-profile-description" class="muted caption">New sessions use these accounts.</small>
          </span>
          <div class="theme-select general-select">
            <select id="project-profile" v-model="projectProfileID" aria-describedby="project-profile-description">
              <option value="">Default</option>
              <option v-for="profile in workspace.profiles" :key="profile.id" :value="profile.id">
                {{ profile.emoji ? profile.emoji + ' ' : '' }}{{ profile.name }}
              </option></select
            ><Icon name="chevron.up.chevron.down" :size="10" />
          </div>
        </div>
        <p v-if="formError" class="error" role="alert">{{ formError }}</p>
        <footer>
          <button type="button" class="standard-button" @click="closeModal">Cancel</button
          ><button type="submit" class="standard-button primary" :disabled="busy || !renameValue.trim()">Save</button>
        </footer>
      </form></Modal
    >
    <Modal v-else-if="modal === 'removeProject'" label="Remove Project" @close="!busy && closeModal()"
      ><div class="close-dialog">
        <h2>Remove {{ projectTitle(removalProject) || 'Project' }}?</h2>
        <p>
          This removes the project, its sessions and scratchpad from Orchestra. The project folder and its files will
          stay on disk.
        </p>
        <p v-if="removalProject?.sessions.some((s) => s.running)" class="muted">
          Running sessions in this project will be stopped.
        </p>
        <p v-if="formError" class="error" role="alert">{{ formError }}</p>
        <footer>
          <button class="standard-button" :disabled="busy" @click="closeModal">Cancel</button
          ><button
            class="standard-button destructive"
            :disabled="busy || !removalProject"
            @click="confirmRemoveProject"
          >
            {{ busy ? 'Removing…' : 'Remove Project' }}
          </button>
        </footer>
      </div></Modal
    >
    <Modal v-else-if="modal === 'deleteScratchpad'" label="Delete Scratchpad" @close="closeModal"
      ><div class="close-dialog">
        <h2>Delete {{ scratchpadProject?.scratchpadName || 'Scratchpad' }}?</h2>
        <p>This will delete the scratchpad and its notes from this project. This action cannot be undone.</p>
        <p v-if="formError" class="error" role="alert">{{ formError }}</p>
        <footer>
          <button class="standard-button" :disabled="busy" @click="closeModal">Cancel</button
          ><button class="standard-button destructive" :disabled="busy" @click="confirmDeleteScratchpad">
            Delete Scratchpad
          </button>
        </footer>
      </div></Modal
    >
    <Modal v-else-if="modal === 'close'" label="Close Session" @close="closeModal"
      ><div class="close-dialog">
        <h2>Close {{ modalSession?.name || 'Session' }}?</h2>
        <p>
          This will stop the terminal process and remove the session from this project. This action cannot be undone.
        </p>
        <label v-if="modalSession?.worktreeDirectory" class="worktree-option" for="close-worktree"
          ><input
            id="close-worktree"
            v-model="removeWorktree"
            type="checkbox"
            aria-label="Remove the worktree"
            aria-describedby="close-worktree-description"
          /><span
            ><strong>Remove the worktree</strong
            ><small id="close-worktree-description" class="muted"
              >Deletes the checkout at {{ modalSession.worktreeDirectory }}. Sessions with uncommitted changes stay
              open. The {{ modalSession.worktreeBranch }} branch is kept if it has unmerged commits.</small
            ></span
          ></label
        >
        <footer>
          <button class="standard-button" @click="closeModal">Cancel</button
          ><button class="standard-button destructive" @click="confirmClose">Close Session</button>
        </footer>
      </div></Modal
    >
    <Modal v-else-if="modal === 'quit'" label="Quit Orchestra" @close="!quitting && closeModal()"
      ><div class="close-dialog">
        <h2>Quit Orchestra?</h2>
        <p>
          {{ activeSessionCount }} running {{ activeSessionCount === 1 ? 'session will' : 'sessions will' }} be stopped.
          Saved projects and notes will be available when you reopen Orchestra.
        </p>
        <footer>
          <button class="standard-button" :disabled="quitting" @click="closeModal">Cancel</button
          ><button class="standard-button destructive" :disabled="quitting" @click="finishQuit">
            {{ quitting ? 'Saving…' : 'Quit Orchestra' }}
          </button>
        </footer>
      </div></Modal
    >
    <Modal v-else-if="modal === 'update'" label="Software Update" @close="closeModal"
      ><UpdateDialog
        :status="updateStatus"
        :preferences="preferences"
        :running-sessions="activeSessionCount"
        @check="checkForUpdates"
        @skip="skipUpdate"
        @install="installUpdate"
        @cancel="call('CancelUpdateInstall')"
        @close="closeModal"
    /></Modal>
    <Modal v-else-if="modal === 'status'" label="Enable Codex Status" @close="closeModal"
      ><div class="close-dialog">
        <h2>Enable Codex Status</h2>
        <p>
          Codex has not sent status events. In its terminal, review and trust the Orchestra hooks at startup, or type
          /hooks to review them now. Then submit a prompt to update the indicator.
        </p>
        <footer><button class="standard-button" @click="closeModal">OK</button></footer>
      </div></Modal
    >
    <Modal
      v-else-if="modal === 'messages' && modalSession && modalProject"
      label="Session Messages"
      kind="messages"
      @close="closeModal"
      ><!-- prettier-ignore-attribute @execute --><Messages
        :session="modalSession"
        :project="modalProject"
        :runs="workspace.runs"
        :preferences="shownPreferences"
        @state="apply"
        @execute="visitedTerminals.add($event.projects.find(p=>p.id===$event.selectedProjectID).selectedSessionID);closeModal()"
        @close="closeModal"
    /></Modal>
  </div>
</template>
