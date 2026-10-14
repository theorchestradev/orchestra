<script setup>
import { ref, watch } from 'vue'
import { agents, agentIcon } from '../shared/appearance'
import { call, copy } from '../shared/api'
defineProps({ appearance: String })
// Settings → Agents → Agent Skill. Go builds the pinned `npx skills add` command with explicit
// agent keys and runs it only when the user clicks Install or Update.
const status = ref(null),
  selected = ref([]),
  command = ref(''),
  busy = ref(false),
  result = ref(null),
  error = ref(''),
  copied = ref(false)
const name = (id) => agents.find((a) => a.id === id)?.name || id
call('AgentSkillStatus')
  .then((s) => {
    status.value = s
    selected.value = s.agents.filter((a) => a.installed).map((a) => a.id)
  })
  .catch((e) => (error.value = String(e)))
watch(
  selected,
  async (ids) => {
    command.value = ids.length ? await call('AgentSkillCommand', ids).catch(() => '') : ''
  },
  { deep: true },
)
async function copyCommand() {
  error.value = ''
  try {
    await copy(command.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch (e) {
    error.value = String(e)
  }
}
async function install() {
  busy.value = true
  error.value = ''
  result.value = null
  try {
    result.value = await call('InstallAgentSkill', selected.value)
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <div class="agent-skill" role="group" aria-labelledby="agent-skill-heading">
    <div>
      <h3 id="agent-skill-heading">Agent Skill</h3>
      <p class="muted caption">
        Teach agents to use the <code>orchestra</code> CLI for requests and the Scratchpad. The skill tells them to run
        <code>orchestra guide</code>, which always matches this version of Orchestra. It is installed with the
        <code>skills</code> CLI, only when you click Install or Update. Installing also adds it to each profile.
      </p>
    </div>
    <p v-if="!status && !error" class="muted caption" role="status"><span class="spinner" />Detecting agents…</p>
    <fieldset v-if="status" class="skill-agents" :disabled="busy">
      <legend class="visually-hidden">Agents to install the skill for</legend>
      <label v-for="agent in status.agents" :key="agent.id" class="skill-agent"
        ><input v-model="selected" type="checkbox" :value="agent.id" /><img
          :src="agentIcon(agent.id, appearance)"
          alt=""
        /><span>{{ name(agent.id) }}</span
        ><small v-if="status.detected && !agent.installed" class="muted">Not found</small></label
      >
    </fieldset>
    <template v-if="status">
      <code v-if="command" class="skill-command" aria-label="Install command">{{ command }}</code>
      <p v-else class="muted caption">Choose at least one agent.</p>
      <p v-if="status.detected && !status.node" class="error caption" role="alert">
        Installing the skill requires Node.js (npx). Install Node.js, or run the command in a terminal where npx is
        available.
      </p>
      <div class="theme-actions">
        <button type="button" class="standard-button" :disabled="!command" @click="copyCommand">
          {{ copied ? 'Copied' : 'Copy Command' }}</button
        ><button
          type="button"
          class="standard-button primary"
          :disabled="busy || !command || (status.detected && !status.node)"
          @click="install"
        >
          {{ busy ? 'Installing…' : 'Install or Update' }}
        </button>
      </div>
    </template>
    <p v-if="error" class="error caption" role="alert">{{ error }}</p>
    <div v-if="result" class="skill-result" role="status">
      <p :class="result.exitCode === 0 ? 'muted caption' : 'error caption'">
        {{
          result.exitCode === 0
            ? 'The skill is installed. Start a new agent session to use it.'
            : 'The skills CLI exited with code ' + result.exitCode + '.'
        }}
      </p>
      <pre v-if="result.output" class="skill-output" tabindex="0" aria-label="Skills CLI output">{{
        result.output
      }}</pre>
    </div>
  </div>
</template>
