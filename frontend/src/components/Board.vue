<script setup>
import { computed, ref } from 'vue'
import { agents, agentIcon, activityLabel, isTerminal } from '../shared/appearance'
import { repositoryLabel } from '../shared/workspace'
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
const columns = computed(() => {
  const groups = [
    { id: 'waiting', name: 'Waiting', description: 'Waiting for your input', sessions: [] },
    { id: 'working', name: 'Doing', description: 'Agents working now', sessions: [] },
    { id: 'inactive', name: 'Inactive', description: 'Ready to start, resume or restart', sessions: [] },
    { id: 'unknown', name: 'Unknown', description: 'Activity is not available yet', sessions: [] },
  ]
  for (const workspace of props.workspaces || []) {
    if (workspaceFilter.value && workspace.id !== workspaceFilter.value) continue
    for (const session of workspace.sessions || []) {
      if (isTerminal(session)) continue
      const activity =
        session.restored || !session.running || session.activity === 'ended'
          ? 'inactive'
          : ['waiting', 'working'].includes(session.activity)
            ? session.activity
            : 'unknown'
      groups.find((group) => group.id === activity).sessions.push({ workspace, session })
    }
  }
  return groups
})
const count = computed(() => columns.value.reduce((n, column) => n + column.sessions.length, 0))
function detail(card) {
  return [card.workspace.name, repositoryLabel(card.workspace, card.session), card.session.worktreeBranch]
    .filter(Boolean)
    .join(' · ')
}
function status(session) {
  if (session.restored)
    return !session.hasLaunched ? 'Ready to start' : session.agentSessionID ? 'Ready to resume' : 'Ready to restart'
  if (!session.running) return session.hasLaunched ? 'Session ended' : 'Ready to start'
  return activityLabel(session.activity)
}
</script>

<template>
  <section class="agent-board" aria-labelledby="board-title">
    <header class="board-header">
      <div>
        <h1 id="board-title">Board</h1>
        <p>{{ count }} {{ count === 1 ? 'agent' : 'agents' }}</p>
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
    <p class="board-hint">Agents move automatically as their activity changes. Select a card to open its session.</p>
    <div class="board-columns" tabindex="0" aria-label="Agent activity columns">
      <section v-for="column in columns" :key="column.id" class="board-column" :aria-labelledby="'board-' + column.id">
        <header>
          <h2 :id="'board-' + column.id">{{ column.name }}</h2>
          <span class="board-count">{{ column.sessions.length }}</span>
        </header>
        <p class="board-column-description">{{ column.description }}</p>
        <div class="board-cards">
          <button
            v-for="card in column.sessions"
            :key="card.session.id"
            class="board-card"
            :title="[card.session.name, detail(card), status(card.session)].filter(Boolean).join('\n')"
            :aria-label="'Open ' + card.session.name + ' in ' + card.workspace.name + ' — ' + status(card.session)"
            @click="$emit('select', card)"
          >
            <span class="board-card-heading">
              <img :src="agentIcon(card.session.agent, appearance)" alt="" />
              <strong>{{ card.session.name }}</strong>
            </span>
            <span class="board-card-agent">{{
              agents.find((agent) => agent.id === card.session.agent)?.name || card.session.agent
            }}</span>
            <span class="board-card-detail">{{ detail(card) }}</span>
            <span class="board-card-status"
              ><span
                class="activity-dot"
                :class="card.session.restored ? 'unknown' : card.session.activity"
                aria-hidden="true"
              />{{ status(card.session) }}</span
            >
          </button>
          <p v-if="!column.sessions.length" class="board-empty">No agents here</p>
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
  padding: 26px 24px 12px;
}
.board-header h1 {
  font-size: 20px;
}
.board-header p,
.board-hint,
.board-column-description,
.board-empty,
.board-card-agent,
.board-card-detail,
.board-card-status {
  color: var(--muted);
  font-size: 12px;
  line-height: 1.5;
}
.board-header p {
  margin-top: 4px;
  overflow-wrap: anywhere;
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
.board-hint {
  padding: 0 24px 20px;
}
.board-columns {
  display: grid;
  grid-template-columns: repeat(4, minmax(220px, 1fr));
  align-items: start;
  gap: 14px;
  padding: 0 24px 24px;
  overflow: auto;
  min-height: 0;
  flex: 1;
}
.board-column {
  background: var(--sidebar);
  border: 1px solid var(--rule);
  border-radius: 10px;
  padding: 12px;
  min-width: 0;
}
.board-column > header {
  display: flex;
  align-items: center;
  gap: 8px;
}
.board-column h2 {
  font-size: 13px;
}
.board-count {
  color: var(--muted);
  background: var(--surface);
  border-radius: 5px;
  padding: 1px 6px;
  font-size: 11px;
}
.board-column-description {
  margin: 6px 0 14px;
}
.board-cards {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.board-card {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
  width: 100%;
  padding: 12px;
  text-align: left;
  background: var(--surface);
  border: 1px solid var(--rule);
  border-radius: 8px;
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
.board-card-heading {
  display: flex;
  align-items: start;
  gap: 8px;
}
.board-card-heading img {
  width: 18px;
  height: 18px;
  flex-shrink: 0;
}
.board-card-heading strong {
  min-width: 0;
  font-size: 13px;
  line-height: 1.4;
}
.board-card-status {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 4px;
}
.board-card-status .activity-dot {
  flex-shrink: 0;
}
.board-empty {
  padding: 18px 0;
  text-align: center;
}
</style>
