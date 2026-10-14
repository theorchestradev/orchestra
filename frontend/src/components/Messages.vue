<script setup>
import { ref, computed } from 'vue'
import { call, copy } from '../shared/api'
import { relativeDate, runLabel, agentIcon, isTerminal } from '../shared/appearance'
import Icon from './Icon.vue'
const props = defineProps({ session: Object, project: Object, runs: Array, preferences: Object })
const emit = defineEmits(['close', 'state', 'error', 'execute'])
const to = ref(props.project.sessions.find((s) => s.id !== props.session.id && !isTerminal(s))?.id || ''),
  text = ref(''),
  error = ref(''),
  busy = ref(false)
const peers = computed(() => props.project.sessions.filter((s) => s.id !== props.session.id && !isTerminal(s)))
const history = computed(() =>
  props.runs.filter((r) => r.fromSessionID === props.session.id || r.toSessionID === props.session.id).toReversed(),
)
const name = (id) => props.project.sessions.find((s) => s.id === id)?.name || 'Closed session'
const canExecute = (run) =>
  props.session.running &&
  props.session.activity !== 'working' &&
  !props.runs.some((r) => r.toSessionID === props.session.id && r.status === 'running') &&
  run.status === 'queued'
async function action(method, ...args) {
  busy.value = true
  error.value = ''
  try {
    const result = await call(method, ...args)
    emit('state', result)
    if (method === 'ExecuteRequest') emit('execute', result)
    if (method === 'SendRequest') text.value = ''
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
</script>
<template>
  <div class="messages-dialog">
    <header>
      <div>
        <h2>Session Messages</h2>
        <p class="muted">{{ session.name }}</p>
      </div>
      <button class="standard-button" @click="emit('close')">Done</button>
    </header>
    <p class="muted">
      <template v-if="preferences?.deliverRequests"
        >Queued requests are submitted automatically once this session's agent is free, or choose Execute Request to
        submit one now.</template
      ><template v-else
        >Read requests through the CLI, or choose Execute Request to paste one into this session and submit
        it.</template
      >
      Replies use orchestra run reply.
    </p>
    <div class="message-history">
      <p v-if="!history.length" class="muted no-messages">No messages yet.</p>
      <article v-for="run in history" :key="run.id" class="message-card">
        <header>
          <strong>{{ name(run.fromSessionID) }} → {{ name(run.toSessionID) }}</strong
          ><span class="muted caption">{{ runLabel(run.status) }}</span>
        </header>
        <p class="message-text">{{ run.prompt }}</p>
        <template v-if="run.response"
          ><hr />
          <h3 class="muted caption">Reply</h3>
          <p class="message-text">{{ run.response }}</p></template
        ><button
          v-if="run.toSessionID === session.id && run.status === 'queued'"
          class="standard-button execute"
          :disabled="busy || !canExecute(run)"
          title="Paste the request and press Enter. Clear any existing draft in the terminal first."
          @click="action('ExecuteRequest', run.id)"
        >
          <Icon name="play.fill" :size="13" />Execute Request
        </button>
        <p v-if="run.detail" class="muted caption">{{ run.detail }}</p>
        <footer>
          <span class="muted caption">{{ relativeDate(run.createdAt) }}</span>
          <div class="spacer" />
          <button class="standard-button caption" @click="copy(run.id).catch((e) => (error = String(e)))">
            Copy Request ID</button
          ><button
            v-if="['queued', 'running'].includes(run.status)"
            class="standard-button caption"
            :disabled="busy"
            @click="action('CancelRequest', run.id)"
          >
            Cancel Request
          </button>
        </footer>
      </article>
    </div>
    <p v-if="error" class="error caption" role="alert">{{ error }}</p>
    <p v-if="!peers.length" class="muted">Create another session in this project to exchange messages.</p>
    <template v-else
      ><label class="recipient-select"
        ><img
          v-if="to"
          :src="agentIcon(project.sessions.find((s) => s.id === to)?.agent, preferences.appearance)"
          alt="" /><select v-model="to" aria-label="Message recipient">
          <option disabled value="">Choose a recipient</option>
          <option v-for="peer in peers" :key="peer.id" :value="peer.id">To: {{ peer.name }}</option></select
        ><Icon name="chevron.down" :size="10" /></label
      ><textarea
        v-model="text"
        aria-label="Message text"
        class="message-editor"
        @keydown.meta.enter.prevent="to && text.trim() && !busy && action('SendRequest', session.id, to, text)"
        @keydown.ctrl.enter.prevent="to && text.trim() && !busy && action('SendRequest', session.id, to, text)"
      />
      <footer>
        <button
          class="standard-button"
          :disabled="busy || !to || !text.trim()"
          @click="action('SendRequest', session.id, to, text)"
        >
          Send Message
        </button>
      </footer></template
    >
  </div>
</template>
