<script setup>
import { computed } from 'vue'
import { agents, agentIcon, themeOptions, activityLabel, statusSummary, isTerminal } from '../shared/appearance'
import { shortcutLabel } from '../shared/platform'
import Icon from './Icon.vue'
const props = defineProps({
  projects: Array,
  runs: Array,
  project: Object,
  session: Object,
  destination: String,
  preferences: Object,
  diffOpen: Boolean,
  update: Object,
  updateReady: Boolean,
})
const emit = defineEmits(['inbox', 'appearance', 'diff', 'update'])
const summary = computed(() => statusSummary(props.projects, props.runs))
const agentName = computed(() =>
  isTerminal(props.session) ? 'Terminal' : agents.find((a) => a.id === props.session?.agent)?.name || 'Agent',
)
const activity = computed(() =>
  props.session?.restored
    ? 'Not started'
    : props.session?.running
      ? isTerminal(props.session)
        ? 'Shell running'
        : activityLabel(props.session.activity)
      : 'Session ended',
)
const themeName = computed(() => themeOptions().find((t) => t.id === props.preferences.theme)?.name || 'System')
const inbox = computed(() =>
  [
    summary.value.queued && `${summary.value.queued} in inbox`,
    summary.value.attention && `${summary.value.attention} need${summary.value.attention === 1 ? 's' : ''} attention`,
  ]
    .filter(Boolean)
    .join(', '),
)
</script>
<template>
  <footer class="status-bar" aria-label="Status bar">
    <div class="status-context">
      <template v-if="project"
        ><span class="status-item truncate" :title="project.directory"
          ><span v-if="project.emoji" class="status-emoji" aria-hidden="true">{{ project.emoji }}</span
          ><Icon v-else name="folder" :size="12" /><span class="truncate">{{ project.name }}</span></span
        ><span
          v-if="project.repository"
          class="status-item status-optional truncate"
          :title="'GitHub repository ' + project.repository"
          >{{ project.repository }}</span
        ></template
      >
      <template v-if="session"
        ><span class="status-item truncate" :title="agentName + ' — ' + session.name"
          ><Icon v-if="isTerminal(session)" name="terminal" :size="12" /><img
            v-else
            :src="agentIcon(session.agent, preferences.appearance)"
            alt=""
          /><span class="truncate">{{ session.name }}</span></span
        ><span class="status-item" aria-live="polite"
          ><i class="activity-dot" :class="session.restored ? null : session.running ? session.activity : 'ended'" />{{
            activity
          }}</span
        ><span
          class="status-item truncate"
          :title="
            session.worktreeBranch
              ? session.worktreeBranch + ' · ' + session.worktreeDirectory
              : 'Runs in the project folder'
          "
          ><Icon name="arrow.triangle.branch" :size="12" /><span class="truncate">{{
            session.worktreeBranch || 'Project folder'
          }}</span></span
        ><span v-if="session.exitMessage" class="status-item status-optional truncate" :title="session.exitMessage">{{
          session.exitMessage
        }}</span></template
      >
      <span v-else-if="destination === 'github' || destination === 'scratchpad'" class="status-item"
        ><Icon :name="destination === 'github' ? 'list.bullet.rectangle' : 'note.text'" :size="12" />{{
          destination === 'github' ? 'GitHub' : project.scratchpadName || 'Scratchpad'
        }}</span
      >
    </div>
    <div class="status-summary">
      <span
        class="status-item"
        :title="
          summary.running + ' running ' + (summary.running === 1 ? 'session' : 'sessions') + ' across all projects'
        "
        ><template v-if="summary.running"
          ><span v-if="summary.working" class="status-item"
            ><i class="activity-dot working" />{{ summary.working }}<span class="status-optional">working</span></span
          ><span v-if="summary.waiting" class="status-item"
            ><i class="activity-dot waiting" />{{ summary.waiting }}<span class="status-optional">waiting</span></span
          ><span class="status-item"
            ><Icon name="terminal" :size="12" />{{ summary.running }}<span class="status-optional">running</span></span
          ></template
        ><template v-else>No running sessions</template></span
      >
      <button
        v-if="session"
        class="status-item status-button"
        :class="{ selected: diffOpen }"
        aria-label="Changes"
        :aria-pressed="diffOpen"
        :title="(diffOpen ? 'Hide' : 'Show') + ` Changes (${shortcutLabel('⇧⌘B')})`"
        @click="emit('diff')"
      >
        <Icon name="sidebar.right" :size="12" /><span class="status-optional">Changes</span>
      </button>
      <button
        v-if="update"
        class="status-item status-button update-available"
        :aria-label="'Orchestra ' + update.version + (updateReady ? ' is ready to install' : ' is available')"
        :title="'Orchestra ' + update.version + (updateReady ? ' is ready to install' : ' is available')"
        @click="emit('update')"
      >
        <Icon name="arrow.down.circle" :size="12" /><span class="status-optional">{{
          updateReady ? 'Restart to Update' : 'Update'
        }}</span>
      </button>
      <button
        v-if="inbox"
        class="status-item status-button"
        :class="{ 'status-error': summary.attention }"
        :aria-label="'Messages: ' + inbox"
        :title="inbox"
        @click="emit('inbox')"
      >
        <Icon v-if="summary.attention" name="exclamationmark.triangle" :size="12" />Inbox<span class="count unread">{{
          summary.queued + summary.attention
        }}</span>
      </button>
      <button
        class="status-item status-button"
        :aria-label="
          'Theme ' +
          themeName +
          ', ' +
          preferences.appearance +
          ' appearance. Switch to ' +
          (preferences.appearance === 'light' ? 'dark' : 'light')
        "
        :title="'Switch to ' + (preferences.appearance === 'light' ? 'Dark' : 'Light') + ' appearance'"
        @click="emit('appearance')"
      >
        <Icon :name="preferences.appearance === 'light' ? 'sun.max' : 'moon'" :size="12" /><span class="truncate">{{
          themeName
        }}</span>
      </button>
    </div>
  </footer>
</template>
