<script setup>
import { computed } from 'vue'
import { agents, agentIcon, activityLabel, statusSessions, statusSummary, isTerminal } from '../shared/appearance'
import { findProfile, profileName, profileSymbol } from '../shared/profiles'
import { repositories, sessionRepository } from '../shared/workspace'
import Icon from './Icon.vue'
import UsageStatus from './UsageStatus.vue'
import { statusBarLayout, statusBarItemVisible } from '../shared/statusBar'
const layout = computed(() => statusBarLayout(props.preferences))
const props = defineProps({
  workspaces: Array,
  runs: Array,
  workspace: Object,
  workspaceIcon: String,
  session: Object,
  profiles: Array,
  destination: String,
  preferences: Object,
  update: Object,
  updateReady: Boolean,
})
const emit = defineEmits(['inbox', 'update', 'settings', 'change'])
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
// Sessions with requests waiting in their inbox or needing attention.
const inboxSessions = computed(
  () =>
    new Set(
      (props.runs || []).filter((r) => ['queued', 'needsAttention'].includes(r.status)).map((r) => r.toSessionID),
    ),
)
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
    <div v-for="(group, side) in layout" :key="group.name" :class="side === 0 ? 'status-context' : 'status-summary'">
      <template v-for="[id] in group.items" :key="id"
        ><template v-if="statusBarItemVisible(preferences, id)">
          <template v-if="id === 'workspace'"
            ><span
              v-if="workspace"
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
            ></template
          >
          <template v-if="id === 'repository'"
            ><span
              v-if="repository"
              class="status-item status-optional truncate"
              :title="repository.directory + (repository.gitHub ? ' · GitHub repository ' + repository.gitHub : '')"
              ><Icon name="folder" :size="12" /><span class="visually-hidden">Repository: </span
              ><span class="truncate">{{ repository.gitHub || repository.name }}</span></span
            ></template
          >
          <template v-if="id === 'session'"
            ><span v-if="session" class="status-item truncate" :title="agentName + ' — ' + session.name"
              ><Icon v-if="isTerminal(session)" name="terminal" :size="12" /><img
                v-else
                :src="agentIcon(session.agent, preferences.appearance)"
                alt=""
              /><span class="truncate">{{ session.name }}</span></span
            ></template
          >
          <template v-if="id === 'profile'"
            ><span
              v-if="session && profile"
              class="status-item status-optional truncate"
              :title="'Profile: ' + profile.name"
              ><span class="status-emoji" aria-hidden="true">{{ profile.symbol }}</span
              ><span class="truncate"><span class="visually-hidden">Profile: </span>{{ profile.name }}</span></span
            ></template
          >
          <template v-if="id === 'activity'"
            ><span v-if="session" class="status-item" aria-live="polite"
              ><i
                class="activity-dot"
                :class="session.restored ? null : session.running ? session.activity : 'ended'"
              />{{ activity }}</span
            ></template
          >
          <template v-if="id === 'branch'"
            ><span
              v-if="session"
              class="status-item truncate"
              :title="
                session.worktreeBranch
                  ? session.worktreeBranch + ' · ' + session.worktreeDirectory
                  : 'Runs in the ' + (repository?.name || 'repository') + ' folder'
              "
              ><Icon name="arrow.triangle.branch" :size="12" /><span class="truncate">{{
                session.worktreeBranch || 'Repository folder'
              }}</span></span
            ></template
          >
          <template v-if="id === 'exitMessage'"
            ><span
              v-if="session && session.exitMessage"
              class="status-item status-optional truncate"
              :title="session.exitMessage"
              >{{ session.exitMessage }}</span
            ></template
          >
          <template v-if="id === 'update'"
            ><button
              v-if="update"
              class="status-item status-button update-available"
              :aria-label="'Orchestra ' + update.version + (updateReady ? ' is ready to install' : ' is available')"
              :title="'Orchestra ' + update.version + (updateReady ? ' is ready to install' : ' is available')"
              @click="emit('update')"
            >
              <Icon name="arrow.down.circle" :size="12" /><span class="status-optional">{{
                updateReady ? 'Restart to Update' : 'Update'
              }}</span>
            </button></template
          >
          <template v-if="id === 'inbox'"
            ><button
              v-if="inbox"
              class="status-item status-button"
              :class="{ 'status-error': summary.attention }"
              :aria-label="'Messages: ' + inbox"
              :title="statusSessions(workspaces, inbox, (s) => inboxSessions.has(s.id))"
              @click="emit('inbox')"
            >
              <Icon v-if="summary.attention" name="exclamationmark.triangle" :size="12" />Inbox<span
                class="count unread"
                >{{ summary.queued + summary.attention }}</span
              >
            </button></template
          >
          <template v-if="id === 'usage'"
            ><UsageStatus
              :appearance="preferences.appearance"
              :agent="session?.agent"
              :profile-id="usageProfileID"
              :profile-name="profileName(profiles, usageProfileID)"
              :detailed="preferences.usageDetailed"
              @detailed="emit('change', { usageDetailed: $event })"
              @settings="emit('settings', $event)"
          /></template>
          <template v-if="id === 'view'"
            ><span v-if="!session && ['github', 'reviews', 'scratchpad'].includes(destination)" class="status-item"
              ><Icon
                :name="
                  destination === 'github'
                    ? 'list.bullet.rectangle'
                    : destination === 'reviews'
                      ? 'arrow.triangle.pull'
                      : 'note.text'
                "
                :size="12"
              />{{
                destination === 'github'
                  ? 'Work Items'
                  : destination === 'reviews'
                    ? 'Reviews'
                    : workspace?.scratchpadName || 'Scratchpad'
              }}</span
            ></template
          >
          <template v-if="id === 'working'"
            ><span
              v-if="summary.working"
              class="status-item"
              :title="statusSessions(workspaces, 'Working sessions', (s) => s.running && s.activity === 'working')"
              ><i class="activity-dot working" />{{ summary.working }}<span class="status-optional">working</span></span
            ></template
          >
          <template v-if="id === 'waiting'"
            ><span
              v-if="summary.waiting"
              class="status-item"
              :title="statusSessions(workspaces, 'Waiting sessions', (s) => s.running && s.activity === 'waiting')"
              ><i class="activity-dot waiting" />{{ summary.waiting }}<span class="status-optional">waiting</span></span
            ></template
          >
          <template v-if="id === 'running'"
            ><span
              class="status-item"
              :title="summary.running ? statusSessions(workspaces, 'Running sessions', (s) => s.running) : undefined"
              ><template v-if="summary.running"
                ><Icon name="terminal" :size="12" />{{ summary.running
                }}<span class="status-optional">running</span></template
              ><template v-else>No running sessions</template></span
            ></template
          >
        </template></template
      >
    </div>
  </footer>
</template>
