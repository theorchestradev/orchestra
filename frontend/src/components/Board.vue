<script setup>
import { computed, onMounted, ref } from 'vue'
import { agents, agentIcon, activityLabel, isTerminal } from '../shared/appearance'
import { repositoryLabel, sessionRepository } from '../shared/workspace'
import { loadPullRequest, pullRequests } from './SessionPullRequest.vue'
import Icon from './Icon.vue'

const props = defineProps({ workspaces: Array, currentWorkspaceId: String, appearance: String })
defineEmits(['select', 'create'])
const workspaceFilter = ref('')
const createWorkspace = computed(
  () =>
    props.workspaces.find((workspace) => workspace.id === workspaceFilter.value) ||
    props.workspaces.find((workspace) => workspace.id === props.currentWorkspaceId) ||
    props.workspaces[0],
)
const cards = computed(() =>
  (props.workspaces || [])
    .filter((workspace) => !workspaceFilter.value || workspace.id === workspaceFilter.value)
    .flatMap((workspace) =>
      (workspace.sessions || []).filter((session) => !isTerminal(session)).map((session) => ({ workspace, session })),
    ),
)
// A merged or closed pull request finishes a session. Otherwise a session waiting for input comes first,
// then a working agent, then an open pull request. Launched sessions without one stay in progress.
function stage(session) {
  const state = pullRequests.get(session.id)?.state
  if (state === 'merged' || state === 'closed') return 'done'
  const active = session.running && !session.restored
  if (active && session.activity === 'waiting') return 'needs'
  if (active && session.activity === 'working') return 'progress'
  if (state === 'open' || state === 'draft') return 'review'
  return !session.hasLaunched && !session.running ? 'todo' : 'progress'
}
const columns = computed(() => {
  const groups = [
    { id: 'todo', name: 'To do', sessions: [] },
    { id: 'progress', name: 'In progress', sessions: [] },
    { id: 'needs', name: 'Needs you', sessions: [] },
    { id: 'review', name: 'In review', sessions: [] },
    { id: 'done', name: 'Done', sessions: [] },
  ]
  for (const card of cards.value) groups.find((group) => group.id === stage(card.session)).sessions.push(card)
  return groups
})
const count = computed(() => cards.value.length)
// Pull requests load one at a time so a large Board doesn't start a GitHub request per session at once.
onMounted(async () => {
  for (const { workspace, session } of cards.value)
    if (sessionRepository(workspace, session)?.gitHub) await loadPullRequest(session.id).catch(() => {})
})
function branch(session) {
  return session.worktreeBranch || pullRequests.get(session.id)?.branch || ''
}
function place(card) {
  return [card.workspace.name, repositoryLabel(card.workspace, card.session)].filter(Boolean).join(' · ')
}
function agentName(session) {
  return agents.find((agent) => agent.id === session.agent)?.name || session.agent
}
function status(session) {
  if (session.restored)
    return !session.hasLaunched ? 'Ready to start' : session.agentSessionID ? 'Ready to resume' : 'Ready to restart'
  if (!session.running) return session.hasLaunched ? 'Session ended' : 'Ready to start'
  return activityLabel(session.activity)
}
</script>

<template>
  <section class="agent-board content-page" aria-labelledby="board-title">
    <header class="board-header">
      <div>
        <h1 id="board-title">Board</h1>
        <p>{{ count }} {{ count === 1 ? 'session' : 'sessions' }}</p>
      </div>
      <label class="board-workspace-filter">
        <span>Workspace</span>
        <select v-model="workspaceFilter" aria-label="Filter Board by workspace">
          <option value="">All Workspaces</option>
          <option v-for="workspace in workspaces" :key="workspace.id" :value="workspace.id">
            {{ workspace.name }}
          </option>
        </select>
      </label>
      <button class="standard-button" :disabled="!createWorkspace" @click="$emit('create', createWorkspace)">
        <Icon name="plus" :size="12" />New Session
      </button>
    </header>
    <div class="board-columns" tabindex="0" aria-label="Session columns">
      <section v-for="column in columns" :key="column.id" class="board-column" :aria-labelledby="'board-' + column.id">
        <header>
          <i class="board-column-dot" :class="column.id" aria-hidden="true" />
          <h2 :id="'board-' + column.id">{{ column.name }}</h2>
          <span class="board-count">{{ column.sessions.length }}</span>
        </header>
        <div class="board-cards">
          <button
            v-for="card in column.sessions"
            :key="card.session.id"
            class="board-card"
            :title="
              [card.session.name, branch(card.session), place(card), status(card.session)].filter(Boolean).join('\n')
            "
            :aria-label="'Open ' + card.session.name + ' in ' + card.workspace.name + ' — ' + status(card.session)"
            @click="$emit('select', card)"
          >
            <strong class="board-card-title">{{ card.session.name }}</strong>
            <span v-if="branch(card.session)" class="board-card-branch"
              ><Icon name="arrow.triangle.branch" :size="12" />{{ branch(card.session) }}</span
            >
            <span class="board-card-place"><Icon name="folder" :size="11" />{{ place(card) }}</span>
            <span class="board-card-status">
              <span
                class="activity-dot"
                :class="card.session.restored || !card.session.running ? 'unknown' : card.session.activity"
                aria-hidden="true"
              />
              <img :src="agentIcon(card.session.agent, appearance)" alt="" />
              <span>{{ agentName(card.session) }} · {{ status(card.session) }}</span>
            </span>
          </button>
          <p v-if="!column.sessions.length" class="board-empty">No sessions</p>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.agent-board {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  color: var(--text);
  background: var(--canvas);
}
.board-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
  padding: 26px 24px 18px;
}
.board-header h1 {
  font-size: 20px;
}
.board-header p,
.board-empty {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.5;
}
.board-header p {
  margin-top: 4px;
}
.board-header .standard-button {
  flex-shrink: 0;
}
.board-workspace-filter {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  color: var(--muted);
  font-size: 12px;
}
.board-workspace-filter select {
  min-width: 160px;
  max-width: min(260px, 32vw);
  padding: 7px 10px;
  color: var(--text);
}
.board-columns {
  display: grid;
  grid-template-columns: repeat(5, minmax(170px, 1fr));
  align-items: stretch;
  gap: 12px;
  padding: 0 24px 24px;
  overflow: auto;
  min-height: 0;
  flex: 1;
}
.board-column {
  display: flex;
  flex-direction: column;
  background: var(--sidebar);
  border-radius: 14px;
  padding: 14px 10px 10px;
  min-width: 0;
  min-height: 0;
}
.board-column > header {
  display: flex;
  align-items: center;
  gap: 9px;
  padding: 0 6px 12px;
}
.board-column h2 {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  font-weight: 500;
  overflow-wrap: anywhere;
}
.board-column-dot {
  width: 9px;
  height: 9px;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--muted);
}
.board-column-dot.progress {
  background: var(--accent);
}
.board-column-dot.needs {
  background: #32d74b;
}
.board-column-dot.review {
  background: var(--status-merged);
}
.board-column-dot.done {
  background: var(--status-open);
}
.board-count {
  color: var(--muted);
  font:
    11px ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
}
.board-cards {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.board-card {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
  width: 100%;
  padding: 13px 14px;
  text-align: left;
  color: var(--text);
  background: var(--canvas);
  border: 1px solid var(--rule);
  border-radius: 12px;
  overflow-wrap: anywhere;
}
.board-card:hover {
  background: var(--selection);
}
.board-card:focus-visible,
.board-columns:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: -2px;
}
.board-card-title {
  font-size: 13px;
  font-weight: 500;
  line-height: 1.35;
}
.board-card-branch,
.board-card-place,
.board-card-status {
  display: flex;
  align-items: center;
  gap: 7px;
  min-width: 0;
  color: var(--muted);
  font:
    10.5px/1.4 ui-monospace,
    SFMono-Regular,
    Menlo,
    monospace;
}
.board-card-branch :deep(.symbol),
.board-card-place :deep(.symbol) {
  flex-shrink: 0;
}
.board-card-place {
  align-self: start;
  max-width: 100%;
  padding: 3px 9px;
  border: 1px solid var(--rule);
  border-radius: 20px;
  background: var(--surface);
}
.board-card-status {
  margin-top: 2px;
  padding-top: 10px;
  border-top: 1px solid var(--rule);
}
.board-card-status img {
  width: 13px;
  height: 13px;
  flex-shrink: 0;
}
.board-card-status .activity-dot {
  flex-shrink: 0;
}
.board-empty {
  padding: 18px 0;
  text-align: center;
}
</style>
