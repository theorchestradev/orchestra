<script setup>
import { computed } from 'vue'
import { agents, agentIcon, themeOptions, activityLabel, statusSummary, isTerminal } from '../shared/appearance'
import { shortcutLabel } from '../shared/platform'
import { findProfile, profileName, profileSymbol } from '../shared/profiles'
import { repositories, sessionRepository } from '../shared/workspace'
import Icon from './Icon.vue'
import UsageStatus from './UsageStatus.vue'
const props = defineProps({
  workspaces: Array,
  runs: Array,
  workspace: Object,
  workspaceIcon: String,
  session: Object,
  profiles: Array,
  destination: String,
  preferences: Object,
  diffOpen: Boolean,
  update: Object,
  updateReady: Boolean,
})
const emit = defineEmits(['inbox', 'appearance', 'diff', 'update', 'settings', 'change'])
const summary = computed(() => statusSummary(props.workspaces, props.runs))
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
// A session outside Default names its profile, so its account is clear.
const profile = computed(() =>
  props.session?.profileID
    ? {
        name: profileName(props.profiles, props.session.profileID),
        symbol: profileSymbol(findProfile(props.profiles, props.session.profileID)),
      }
    : null,
)
// Usage follows the account Settings → Usage shows first: the session's profile, or else the workspace's.
const usageProfileID = computed(() => {
  const id = (props.session || props.workspace)?.profileID || ''
  return findProfile(props.profiles, id) ? id : ''
})
// The session's repository, fixed for its life; without a session, the workspace's only one.
const repository = computed(() =>
  props.session
    ? sessionRepository(props.workspace, props.session)
    : repositories(props.workspace).length === 1
      ? repositories(props.workspace)[0]
      : null,
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
      <template v-if="workspace"
        ><span
          class="status-item truncate"
          :title="
            repositories(workspace)
              .map((r) => r.directory)
              .join('\n')
          "
          ><span v-if="workspace.emoji" class="status-emoji" aria-hidden="true">{{ workspace.emoji }}</span
          ><img v-else-if="workspaceIcon" class="status-workspace-icon" :src="workspaceIcon" alt="" /><Icon
            v-else
            name="folder"
            :size="12"
          /><span class="truncate">{{ workspace.name }}</span></span
        ><span
          v-if="repository"
          class="status-item status-optional truncate"
          :title="repository.directory + (repository.gitHub ? ' · GitHub repository ' + repository.gitHub : '')"
          ><Icon name="folder" :size="12" /><span class="visually-hidden">Repository: </span
          ><span class="truncate">{{ repository.gitHub || repository.name }}</span></span
        ></template
      >
      <template v-if="session"
        ><span class="status-item truncate" :title="agentName + ' — ' + session.name"
          ><Icon v-if="isTerminal(session)" name="terminal" :size="12" /><img
            v-else
            :src="agentIcon(session.agent, preferences.appearance)"
            alt=""
          /><span class="truncate">{{ session.name }}</span></span
        ><span v-if="profile" class="status-item status-optional truncate" :title="'Profile: ' + profile.name"
          ><span class="status-emoji" aria-hidden="true">{{ profile.symbol }}</span
          ><span class="truncate"><span class="visually-hidden">Profile: </span>{{ profile.name }}</span></span
        ><span class="status-item" aria-live="polite"
          ><i class="activity-dot" :class="session.restored ? null : session.running ? session.activity : 'ended'" />{{
            activity
          }}</span
        ><span
          class="status-item truncate"
          :title="
            session.worktreeBranch
              ? session.worktreeBranch + ' · ' + session.worktreeDirectory
              : 'Runs in the ' + (repository?.name || 'repository') + ' folder'
          "
          ><Icon name="arrow.triangle.branch" :size="12" /><span class="truncate">{{
            session.worktreeBranch || 'Repository folder'
          }}</span></span
        ><span v-if="session.exitMessage" class="status-item status-optional truncate" :title="session.exitMessage">{{
          session.exitMessage
        }}</span></template
      >
      <span v-else-if="destination === 'github'" class="status-item"
        ><Icon name="list.bullet.rectangle" :size="12" />Work Items</span
      ><span v-else-if="destination === 'reviews'" class="status-item"
        ><Icon name="arrow.triangle.pull" :size="12" />Reviews</span
      ><span v-else-if="destination === 'scratchpad'" class="status-item"
        ><Icon name="note.text" :size="12" />{{ workspace.scratchpadName || 'Scratchpad' }}</span
      >
    </div>
    <div class="status-summary">
      <span
        class="status-item"
        :title="
          summary.running + ' running ' + (summary.running === 1 ? 'session' : 'sessions') + ' across all workspaces'
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
        aria-label="Changes and Files"
        :aria-pressed="diffOpen"
        :title="(diffOpen ? 'Hide' : 'Show') + ` Changes and Files (${shortcutLabel('⇧⌘B')})`"
        @click="emit('diff')"
      >
        <Icon name="sidebar.right" :size="12" /><span class="status-optional">Changes / Files</span>
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
      <UsageStatus
        v-if="!preferences.hideUsageStatus"
        :appearance="preferences.appearance"
        :agent="session?.agent"
        :profile-id="usageProfileID"
        :profile-name="profileName(profiles, usageProfileID)"
        :detailed="preferences.usageDetailed"
        @detailed="emit('change', { usageDetailed: $event })"
        @settings="emit('settings', $event)"
      />
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
