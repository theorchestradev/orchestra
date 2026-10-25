<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { themeOptions, themeSource, agents, windowOpacityRange, effectiveAppearance } from '../shared/appearance'
import Icon from './Icon.vue'
import Modal from './Modal.vue'
import AgentUsage from './AgentUsage.vue'
import AgentSkill from './AgentSkill.vue'
import Profiles from './Profiles.vue'
import { call, openURL } from '../shared/api'
import { privacyURL } from '../shared/telemetry'
import { mac, shortcutLabel } from '../shared/platform'
import { shortcuts, shortcutKeys } from '../shared/shortcuts'
const props = defineProps({
  preferences: Object,
  themeErrors: Array,
  themeDirectory: String,
  reduceTransparency: Boolean,
  telemetryStatus: Object,
  telemetryBusy: Boolean,
  profiles: Array,
  project: Object,
  // The profile Usage shows first: the selected session's, or else the project's.
  usageProfileId: String,
  // The section to open with, and the id of a setting in it to focus.
  initialSection: String,
  initialFocus: String,
})
const builtInThemes = computed(() => themeOptions().filter((t) => !t.custom))
const customThemes = computed(() => themeOptions().filter((t) => t.custom))
const emit = defineEmits(['change', 'close', 'duplicated', 'consent', 'check-updates', 'state', 'login'])
const section = ref(props.initialSection || 'General')
watch(
  () => props.initialSection,
  (value) => value && (section.value = value),
)
const sections = {
  General: 'gearshape',
  Agents: 'person.2',
  'Source Control': 'arrow.triangle.branch',
  Profiles: 'person.crop.circle',
  Appearance: 'paintpalette',
  Terminal: 'terminal',
  Shortcuts: 'keyboard',
  Usage: 'circle.dotted',
  Privacy: 'shield',
}
const shortcutGroups = [...new Set(shortcuts.map((s) => s.group))].map((name) => ({
  name,
  entries: shortcuts.filter((s) => s.group === name),
}))
// Opacity matters only while the window is actually translucent.
const opacityEnabled = computed(() => props.preferences.translucent && !props.reduceTransparency)
const windowOpacity = computed(() => props.preferences.windowOpacity || windowOpacityRange.default)
const shownAppearance = computed(() => effectiveAppearance(props.preferences.appearance))
const modes = [
  { id: 'system', name: 'System', icon: 'circle.lefthalf.filled' },
  { id: 'light', name: 'Light', icon: 'sun.max' },
  { id: 'dark', name: 'Dark', icon: 'moon' },
]
const choose = (key, value) => emit('change', { ...props.preferences, [key]: value })
// Duplicate as Custom Theme writes the selected theme's colors through Go; App then reloads the
// custom themes and selects the copy.
const selectedTheme = computed(() => themeOptions().find((t) => t.id === props.preferences.theme) || themeOptions()[0])
const revealLabel = mac ? 'Reveal in Finder' : 'Open Folder'
const duplicating = ref(false),
  duplicateName = ref(''),
  duplicateError = ref(''),
  duplicateBusy = ref(false),
  created = ref(null),
  doneButton = ref(null)
function openDuplicate() {
  duplicateName.value = selectedTheme.value.name + ' Copy'
  duplicateError.value = ''
  created.value = null
  duplicating.value = true
}
async function duplicate() {
  duplicateBusy.value = true
  duplicateError.value = ''
  try {
    const { light, dark, ansi } = themeSource(props.preferences.theme)
    created.value = await call('DuplicateTheme', duplicateName.value.trim(), light, dark, ansi)
    emit('duplicated', created.value)
    await nextTick()
    doneButton.value?.focus()
  } catch (e) {
    duplicateError.value = String(e)
  } finally {
    duplicateBusy.value = false
  }
}
async function reveal(id) {
  duplicateError.value = ''
  try {
    await call('RevealCustomTheme', id)
  } catch (e) {
    if (duplicating.value) duplicateError.value = String(e)
    else console.warn('Could not reveal the custom theme: ' + e)
  }
}
// Terminal fonts are listed when Settings opens. A saved font that is no longer installed stays
// selected and is labeled, while terminals fall back to System Monospace.
const terminalFonts = ref(null)
call('TerminalFonts')
  .then((list) => (terminalFonts.value = list || []))
  .catch(() => (terminalFonts.value = []))
const savedFont = computed(() => props.preferences.terminalFontFamily || '')
const fontMissing = computed(
  () => !!savedFont.value && !!terminalFonts.value && !terminalFonts.value.includes(savedFont.value),
)
const fontOptions = computed(() => {
  const list = terminalFonts.value || []
  return savedFont.value && !list.includes(savedFont.value) ? [savedFont.value, ...list] : list
})
onMounted(() => props.initialFocus && document.getElementById(props.initialFocus)?.focus())
// macOS shows build details in its About panel; elsewhere General lists them.
const about = ref('')
if (!mac)
  call('AboutInfo')
    .then((text) => (about.value = text))
    .catch(() => {})
</script>
<template>
  <div class="settings-dialog">
    <nav class="settings-rail" aria-label="Settings sections">
      <button
        v-for="(icon, name) in sections"
        :key="name"
        :class="{ selected: section === name }"
        :aria-pressed="section === name"
        @click="section = name"
      >
        <Icon :name="icon" :size="14" />{{ name }}
      </button>
    </nav>
    <main class="settings-main">
      <div class="settings-content">
        <h2>{{ section }}</h2>
        <div v-if="section === 'General'" class="settings-card general-settings">
          <div class="setting-row">
            <div>
              <label for="startup-view">Startup View</label>
              <p class="muted caption">
                Open this view in the last selected project. Sessions stay stopped until you resume them.
              </p>
            </div>
            <div class="theme-select general-select">
              <select
                id="startup-view"
                :value="preferences.startupView"
                @change="choose('startupView', $event.target.value)"
              >
                <option value="last">Last selection</option>
                <option value="scratchpad">Scratchpad</option>
                <option value="github">Issues &amp; PRs, if available</option></select
              ><Icon name="chevron.up.chevron.down" :size="10" />
            </div>
          </div>
          <hr />
          <div
            v-for="setting in [
              {
                key: 'confirmQuit',
                label: 'Confirm Before Quitting',
                description: 'Ask before quitting while sessions are running.',
              },
              {
                key: 'confirmSessionClose',
                label: 'Confirm Before Closing a Session',
                description: 'Ask before stopping and removing a session.',
              },
            ]"
            :key="setting.key"
            class="setting-row confirmation-setting"
          >
            <div>
              <label :for="setting.key">{{ setting.label }}</label>
              <p class="muted caption">{{ setting.description }}</p>
            </div>
            <button
              :id="setting.key"
              class="switch"
              :class="{ on: preferences[setting.key] }"
              role="switch"
              :aria-checked="!!preferences[setting.key]"
              :aria-label="setting.label"
              @click="choose(setting.key, !preferences[setting.key])"
            >
              <span />
            </button>
          </div>
          <hr />
          <div class="setting-row">
            <div>
              <label for="keepSessionsRunning">Keep Sessions Running After Quitting</label>
              <p class="muted caption">
                Sessions started while this is on keep running in the background when you quit Orchestra and reconnect
                when you open it again. While Orchestra is closed, Inbox requests are not delivered and the orchestra
                CLI is unavailable.
              </p>
            </div>
            <button
              id="keepSessionsRunning"
              class="switch"
              :class="{ on: preferences.keepSessionsRunning }"
              role="switch"
              :aria-checked="!!preferences.keepSessionsRunning"
              aria-label="Keep Sessions Running After Quitting"
              @click="choose('keepSessionsRunning', !preferences.keepSessionsRunning)"
            >
              <span />
            </button>
          </div>
          <hr />
          <div class="setting-row">
            <div>
              <label for="github-refresh">GitHub Auto Refresh</label>
              <p class="muted caption">Refresh issues and pull requests while this page is visible.</p>
            </div>
            <div class="theme-select general-select">
              <select
                id="github-refresh"
                :value="preferences.gitHubRefreshInterval"
                @change="choose('gitHubRefreshInterval', Number($event.target.value))"
              >
                <option :value="0">Manual</option>
                <option :value="30">Every 30 seconds</option>
                <option :value="60">Every minute</option>
                <option :value="300">Every 5 minutes</option></select
              ><Icon name="chevron.up.chevron.down" :size="10" />
            </div>
          </div>
          <hr />
          <div class="setting-row">
            <div>
              <label for="updateChecks">Check for Updates Automatically</label>
              <p class="muted caption">Look for a new version at startup and once a day.</p>
            </div>
            <button
              id="updateChecks"
              class="switch"
              :class="{ on: !preferences.updateChecksOff }"
              role="switch"
              :aria-checked="!preferences.updateChecksOff"
              aria-label="Check for Updates Automatically"
              @click="choose('updateChecksOff', !preferences.updateChecksOff)"
            >
              <span />
            </button>
          </div>
          <hr />
          <div class="setting-row">
            <div>
              <label for="betaUpdates">Include Beta Versions</label>
              <p class="muted caption">Offer pre-release versions as well as stable releases.</p>
            </div>
            <button
              id="betaUpdates"
              class="switch"
              :class="{ on: preferences.betaUpdates }"
              role="switch"
              :aria-checked="!!preferences.betaUpdates"
              aria-label="Include Beta Versions"
              @click="choose('betaUpdates', !preferences.betaUpdates)"
            >
              <span />
            </button>
          </div>
          <div class="theme-actions">
            <button class="standard-button" @click="emit('check-updates')">Check for Updates…</button>
          </div>
          <template v-if="about">
            <hr />
            <div class="setting-row">
              <div>
                <h3 class="about-heading">About Orchestra</h3>
                <p class="muted caption about-info">{{ about }}</p>
              </div>
            </div>
          </template>
        </div>
        <div v-else-if="section === 'Agents'" class="settings-card general-settings">
          <div class="setting-row">
            <div>
              <label for="default-agent">Default Agent</label>
              <p class="muted caption">Preselect this agent when creating a session.</p>
            </div>
            <div class="theme-select general-select">
              <select
                id="default-agent"
                :value="preferences.defaultAgent"
                @change="choose('defaultAgent', $event.target.value)"
              >
                <option v-for="agent in agents" :key="agent.id" :value="agent.id">{{ agent.name }}</option></select
              ><Icon name="chevron.up.chevron.down" :size="10" />
            </div>
          </div>
          <hr />
          <div class="setting-row">
            <div>
              <label for="deliverRequests">Deliver Inbox Requests Automatically</label>
              <p class="muted caption">
                Submit queued requests from other sessions once the recipient agent finishes its turn, oldest first.
                Requests wait while you type in its terminal.
              </p>
            </div>
            <button
              id="deliverRequests"
              class="switch"
              :class="{ on: preferences.deliverRequests }"
              role="switch"
              :aria-checked="!!preferences.deliverRequests"
              aria-label="Deliver Inbox Requests Automatically"
              @click="choose('deliverRequests', !preferences.deliverRequests)"
            >
              <span />
            </button>
          </div>
          <hr />
          <div class="setting-row">
            <div>
              <label for="attentionSound">Play Sound When an Agent Needs You</label>
              <p class="muted caption">
                Play a sound when an agent finishes its turn or asks for permission, unless you're already looking at
                that session.
              </p>
            </div>
            <button
              id="attentionSound"
              class="switch"
              :class="{ on: preferences.attentionSound }"
              role="switch"
              :aria-checked="!!preferences.attentionSound"
              aria-label="Play Sound When an Agent Needs You"
              @click="choose('attentionSound', !preferences.attentionSound)"
            >
              <span />
            </button>
          </div>
          <hr />
          <AgentSkill :appearance="shownAppearance" />
        </div>
        <div v-else-if="section === 'Source Control'" class="settings-card general-settings">
          <div class="setting-row">
            <div>
              <label for="keepLocalMainUpdated">Keep Local Main Up to Date</label>
              <p class="muted caption">
                When you create a session worktree, Orchestra fetches the remote base and safely fast-forwards your
                matching local branch, such as <code>main</code> or <code>master</code>. This keeps commands like
                <code>git diff main...HEAD</code> from comparing against stale history. Orchestra skips the update if
                that branch has uncommitted changes or local-only commits.
              </p>
            </div>
            <button
              id="keepLocalMainUpdated"
              class="switch"
              :class="{ on: preferences.keepLocalMainUpdated }"
              role="switch"
              :aria-checked="!!preferences.keepLocalMainUpdated"
              aria-label="Keep Local Main Up to Date"
              @click="choose('keepLocalMainUpdated', !preferences.keepLocalMainUpdated)"
            >
              <span />
            </button>
          </div>
        </div>
        <div v-else-if="section === 'Appearance'" class="settings-card appearance-controls">
          <div class="setting-row">
            <div>
              <span>Mode</span>
              <p v-if="!mac" class="muted caption">System uses Dark on Linux.</p>
            </div>
            <div class="mode-switch appearance-mode" role="group" aria-label="Mode">
              <button
                v-for="mode in modes"
                :key="mode.id"
                :aria-pressed="preferences.appearance === mode.id"
                :class="{ selected: preferences.appearance === mode.id }"
                @click="choose('appearance', mode.id)"
              >
                <Icon :name="mode.icon" :size="14" />{{ mode.name }}
              </button>
            </div>
          </div>
          <hr />
          <div class="setting-row theme-setting">
            <div>
              <label for="theme">Theme</label>
              <p v-if="themeDirectory" class="muted caption">
                Add custom themes in <code>{{ themeDirectory }}</code
                >.
              </p>
            </div>
            <div class="theme-select">
              <i class="theme-dot" /><select
                id="theme"
                aria-label="Theme"
                :value="preferences.theme"
                @change="choose('theme', $event.target.value)"
              >
                <template v-if="customThemes.length">
                  <optgroup label="Built-in">
                    <option v-for="theme in builtInThemes" :value="theme.id" :key="theme.id">{{ theme.name }}</option>
                  </optgroup>
                  <optgroup label="Custom">
                    <option v-for="theme in customThemes" :value="theme.id" :key="theme.id">{{ theme.name }}</option>
                  </optgroup>
                </template>
                <template v-else>
                  <option v-for="theme in builtInThemes" :value="theme.id" :key="theme.id">{{ theme.name }}</option>
                </template></select
              ><Icon name="chevron.up.chevron.down" :size="10" />
            </div>
          </div>
          <div class="theme-actions">
            <button class="standard-button" @click="openDuplicate">Duplicate as Custom Theme…</button
            ><button v-if="selectedTheme.custom" class="standard-button" @click="reveal(selectedTheme.id)">
              {{ revealLabel }}
            </button>
          </div>
          <div v-if="themeErrors?.length" class="theme-errors" role="status">
            <p>Some custom themes were skipped:</p>
            <ul>
              <li v-for="message in themeErrors" :key="message">{{ message }}</li>
            </ul>
          </div>
          <template v-if="mac">
            <hr />
            <div class="setting-row translucent-setting">
              <div>
                <label for="translucent">Translucent Window</label>
                <p class="muted caption">
                  {{
                    reduceTransparency
                      ? 'Reduce Transparency is on in macOS Accessibility settings, so the window stays opaque.'
                      : 'Show the desktop through the window with a blur. Dialogs and menus stay opaque.'
                  }}
                </p>
              </div>
              <button
                id="translucent"
                class="switch"
                :class="{ on: preferences.translucent }"
                role="switch"
                :aria-checked="!!preferences.translucent"
                aria-label="Translucent Window"
                @click="choose('translucent', !preferences.translucent)"
              >
                <span />
              </button>
            </div>
            <div class="window-opacity-setting" :class="{ disabled: !opacityEnabled }">
              <div class="setting-row">
                <label for="window-opacity">Opacity</label><span class="muted">{{ windowOpacity }}%</span>
              </div>
              <div class="font-slider">
                <input
                  id="window-opacity"
                  type="range"
                  :min="windowOpacityRange.min"
                  :max="windowOpacityRange.max"
                  step="1"
                  aria-label="Window Opacity"
                  :aria-valuetext="windowOpacity + '%'"
                  :disabled="!opacityEnabled"
                  @pointerdown="$event.currentTarget.focus()"
                  :style="{
                    '--slider-progress':
                      ((windowOpacity - windowOpacityRange.min) / (windowOpacityRange.max - windowOpacityRange.min)) *
                        100 +
                      '%',
                  }"
                  :value="windowOpacity"
                  @input="choose('windowOpacity', Number($event.target.value))"
                />
              </div>
            </div>
          </template>
          <hr />
          <div class="setting-row arc-mode-setting">
            <div>
              <label for="arc-mode">Arc Mode</label>
              <p class="muted caption">Show one project at a time, with project icons at the bottom of the sidebar.</p>
            </div>
            <button
              id="arc-mode"
              class="switch"
              :class="{ on: preferences.arcMode }"
              role="switch"
              :aria-checked="!!preferences.arcMode"
              aria-label="Arc Mode"
              @click="choose('arcMode', !preferences.arcMode)"
            >
              <span />
            </button>
          </div>
        </div>
        <div v-else-if="section === 'Terminal'" class="settings-card terminal-settings">
          <div class="setting-row font-family-setting">
            <div>
              <label for="font-family">Font</label>
              <p v-if="fontMissing" id="font-family-note" class="muted caption">
                Not installed. Terminals use System Monospace.
              </p>
            </div>
            <div class="theme-select general-select">
              <select
                id="font-family"
                :value="savedFont"
                :aria-describedby="fontMissing ? 'font-family-note' : undefined"
                @change="choose('terminalFontFamily', $event.target.value)"
              >
                <option value="">System Monospace</option>
                <option v-for="font in fontOptions" :key="font" :value="font">
                  {{ fontMissing && font === savedFont ? font + ' (Not installed)' : font }}
                </option></select
              ><Icon name="chevron.up.chevron.down" :size="10" />
            </div>
          </div>
          <hr />
          <div class="setting-row">
            <label for="font-size">Font Size</label><span class="muted">{{ preferences.terminalFontSize }} pt</span>
          </div>
          <div class="font-slider">
            <input
              id="font-size"
              type="range"
              min="10"
              max="24"
              step="1"
              aria-label="Terminal Font Size"
              @pointerdown="$event.currentTarget.focus()"
              :style="{ '--slider-progress': ((preferences.terminalFontSize - 10) / 14) * 100 + '%' }"
              :value="preferences.terminalFontSize"
              @input="choose('terminalFontSize', Number($event.target.value))"
            />
            <div class="slider-ticks" aria-hidden="true"><i v-for="step in 15" :key="step" /></div>
          </div>
          <hr />
          <div class="setting-row">
            <label for="padding-x">Horizontal Padding</label
            ><span class="muted">{{ preferences.terminalPaddingX }} px</span>
          </div>
          <div class="font-slider">
            <input
              id="padding-x"
              type="range"
              min="0"
              max="24"
              step="2"
              aria-label="Terminal Horizontal Padding"
              @pointerdown="$event.currentTarget.focus()"
              :style="{ '--slider-progress': (preferences.terminalPaddingX / 24) * 100 + '%' }"
              :value="preferences.terminalPaddingX"
              @input="choose('terminalPaddingX', Number($event.target.value))"
            />
            <div class="slider-ticks" aria-hidden="true"><i v-for="step in 13" :key="step" /></div>
          </div>
          <hr />
          <div class="setting-row">
            <label for="padding-y">Vertical Padding</label
            ><span class="muted">{{ preferences.terminalPaddingY }} px</span>
          </div>
          <div class="font-slider">
            <input
              id="padding-y"
              type="range"
              min="0"
              max="24"
              step="2"
              aria-label="Terminal Vertical Padding"
              @pointerdown="$event.currentTarget.focus()"
              :style="{ '--slider-progress': (preferences.terminalPaddingY / 24) * 100 + '%' }"
              :value="preferences.terminalPaddingY"
              @input="choose('terminalPaddingY', Number($event.target.value))"
            />
            <div class="slider-ticks" aria-hidden="true"><i v-for="step in 13" :key="step" /></div>
          </div>
          <hr />
          <div class="setting-row smoothing-row">
            <div>
              <label for="smoothing">Font Smoothing</label>
              <p class="muted caption">Use macOS font smoothing for fuller, heavier terminal text.</p>
            </div>
            <button
              id="smoothing"
              class="switch"
              :class="{ on: preferences.terminalFontSmoothing }"
              role="switch"
              :aria-checked="preferences.terminalFontSmoothing"
              @click="choose('terminalFontSmoothing', !preferences.terminalFontSmoothing)"
            >
              <span />
            </button>
          </div>
          <hr />
          <div class="setting-row">
            <div>
              <label for="open-in-button">Show Open in Button</label>
              <p class="muted caption">
                Show a button above sessions that opens their folder in an editor, terminal or Git tool. The session
                menu and the command bar keep these actions.
              </p>
            </div>
            <button
              id="open-in-button"
              class="switch"
              :class="{ on: !preferences.hideOpenInButton }"
              role="switch"
              :aria-checked="!preferences.hideOpenInButton"
              aria-label="Show Open in Button"
              @click="choose('hideOpenInButton', !preferences.hideOpenInButton)"
            >
              <span />
            </button>
          </div>
        </div>
        <div v-else-if="section === 'Shortcuts'" class="settings-card general-settings shortcut-settings">
          <p class="muted caption">
            The command bar ({{ shortcutLabel('⌘K') }}) lists every command.<template v-if="!mac">
              Inside terminals, app shortcuts and copy and paste add Shift, so Ctrl+B, Ctrl+K and Ctrl+N reach the
              terminal.</template
            >
          </p>
          <section
            v-for="(group, i) in shortcutGroups"
            :key="group.name"
            class="shortcut-group"
            :aria-labelledby="'shortcut-group-' + i"
          >
            <h3 :id="'shortcut-group-' + i">{{ group.name }}</h3>
            <dl>
              <div v-for="entry in group.entries" :key="entry.id" class="shortcut-row">
                <dt>{{ entry.name }}</dt>
                <dd>
                  <span v-for="key in shortcutKeys(entry)" :key="key.label" class="shortcut-keys"
                    ><kbd aria-hidden="true">{{ key.label }}</kbd
                    ><span class="visually-hidden">{{ key.name }}</span
                    ><small v-if="key.note" class="muted caption">{{ key.note }}</small></span
                  >
                </dd>
              </div>
            </dl>
          </section>
        </div>
        <div v-else-if="section === 'Privacy'" class="settings-card general-settings">
          <div class="setting-row">
            <div>
              <label for="telemetry-enabled">Share anonymous usage data</label>
              <p class="muted caption">
                Share feature usage and approximate city and country with PostHog. IPs are discarded. Prompts, code,
                terminal content, names and paths are never collected. Off by default.
              </p>
            </div>
            <button
              id="telemetry-enabled"
              class="switch"
              :class="{ on: preferences.telemetryEnabled && telemetryStatus?.available }"
              role="switch"
              :aria-checked="!!preferences.telemetryEnabled && !!telemetryStatus?.available"
              aria-label="Share anonymous usage data"
              aria-describedby="telemetry-status"
              :disabled="!telemetryStatus?.available && !preferences.telemetryEnabled"
              :aria-disabled="telemetryBusy"
              @click="!telemetryBusy && emit('consent', !preferences.telemetryEnabled)"
            >
              <span />
            </button>
          </div>
          <p id="telemetry-status" class="muted caption" role="status">
            {{
              telemetryStatus?.reason ||
              (telemetryStatus?.debug
                ? 'Debug mode: events are written to the local log and never sent.'
                : 'Turning sharing off discards queued events and deletes the anonymous installation ID.')
            }}
          </p>
          <button class="text-link" @click="openURL(privacyURL)">
            Privacy &amp; usage data: every event and property
          </button>
        </div>
        <Profiles
          v-else-if="section === 'Profiles'"
          :profiles="profiles"
          :project="project"
          :appearance="shownAppearance"
          @state="emit('state', $event)"
          @login="emit('login', $event)"
        />
        <template v-else
          ><div class="settings-card general-settings usage-status-setting">
            <div class="setting-row">
              <div>
                <label for="usage-status">Show Usage in Status Bar</label>
                <p class="muted caption">
                  Show how much of the session agent's limit is used and when it resets. Click it for every agent.
                </p>
              </div>
              <button
                id="usage-status"
                class="switch"
                :class="{ on: !preferences.hideUsageStatus }"
                role="switch"
                :aria-checked="!preferences.hideUsageStatus"
                aria-label="Show Usage in Status Bar"
                @click="choose('hideUsageStatus', !preferences.hideUsageStatus)"
              >
                <span />
              </button>
            </div>
          </div>
          <AgentUsage :appearance="shownAppearance" :profiles="profiles" :profile-id="usageProfileId"
        /></template>
      </div>
    </main>
    <button class="settings-close icon-button" aria-label="Close Settings" @click="emit('close')">
      <Icon name="xmark" :size="11" />
    </button>
    <Teleport to=".app-shell">
      <Modal v-if="duplicating" label="Duplicate as Custom Theme" @close="!duplicateBusy && (duplicating = false)"
        ><form v-if="!created" class="rename-dialog duplicate-dialog" @submit.prevent="duplicate">
          <h2>Duplicate as Custom Theme</h2>
          <p class="muted">
            Save the colors of {{ selectedTheme.name }} as a new custom theme<template v-if="themeDirectory">
              in <code>{{ themeDirectory }}</code></template
            >.
          </p>
          <label>Theme Name<input v-model="duplicateName" autofocus maxlength="100" aria-label="Theme Name" /></label>
          <p v-if="duplicateError" class="error" role="alert">{{ duplicateError }}</p>
          <footer>
            <button type="button" class="standard-button" :disabled="duplicateBusy" @click="duplicating = false">
              Cancel</button
            ><button type="submit" class="standard-button primary" :disabled="duplicateBusy || !duplicateName.trim()">
              {{ duplicateBusy ? 'Duplicating…' : 'Duplicate' }}
            </button>
          </footer>
        </form>
        <div v-else class="close-dialog duplicate-dialog">
          <h2>Custom Theme Created</h2>
          <p>
            {{ created.name }} is now the selected theme. Edit its <code>theme.json</code> to change the colors;
            Orchestra reloads it when the window gains focus.
          </p>
          <p v-if="duplicateError" class="error" role="alert">{{ duplicateError }}</p>
          <footer>
            <button type="button" class="standard-button" @click="reveal(created.id)">{{ revealLabel }}</button
            ><button
              ref="doneButton"
              type="button"
              class="standard-button primary"
              data-default-action
              @click="duplicating = false"
            >
              Done
            </button>
          </footer>
        </div></Modal
      >
    </Teleport>
  </div>
</template>
