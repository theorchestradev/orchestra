<script setup>
import { ref, onMounted, watch } from 'vue'
import { call, openURL } from '../shared/api'
import { moneyLabel, countLabel } from '../shared/usage'
// Each profile keeps its own management key; "" is Default.
const props = defineProps({ profileId: { type: String, default: '' } })
const account = ref({ configured: false }),
  key = ref(''),
  busy = ref(false),
  error = ref(''),
  editing = ref(false)
// A reading for a profile that is no longer chosen is dropped.
let request = 0
async function refresh(force = false) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  const current = ++request
  try {
    const value = await call('OpenRouterUsage', force, props.profileId)
    if (current === request) account.value = value
  } catch {
    if (current === request)
      error.value = 'Could not read the OpenRouter key from macOS Keychain. Allow access and try refreshing again.'
  } finally {
    if (current === request) busy.value = false
  }
}
watch(
  () => props.profileId,
  () => {
    request++
    busy.value = false
    account.value = { configured: false }
    key.value = ''
    editing.value = false
    refresh()
  },
)
async function connect() {
  if (busy.value || !key.value.trim()) return
  busy.value = true
  error.value = ''
  try {
    account.value = await call('SetOpenRouterKey', key.value.trim(), props.profileId)
    key.value = ''
    editing.value = false
  } catch (e) {
    error.value = String(e)
  } finally {
    busy.value = false
  }
}
async function remove() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await call('RemoveOpenRouterKey', props.profileId)
    account.value = { configured: false }
    key.value = ''
    editing.value = false
  } catch {
    error.value = 'Could not remove the key from macOS Keychain. Try again.'
  } finally {
    busy.value = false
  }
}
function cancel() {
  key.value = ''
  editing.value = false
  error.value = ''
}
defineExpose({ refresh })
onMounted(refresh)
</script>
<template>
  <section class="usage-provider openrouter-usage" :aria-busy="busy">
    <header>
      <h3>OpenRouter</h3>
      <span v-if="account.configured" class="usage-credits muted caption">Connected</span>
    </header>
    <p v-if="error" class="error usage-unavailable" role="alert">{{ error }}</p>
    <p v-if="account.message" class="muted usage-unavailable" role="status">{{ account.message }}</p>
    <div v-if="account.configured && account.balance != null" class="openrouter-totals">
      <div>
        <span class="muted caption">Available balance</span><strong>{{ moneyLabel(account.balance) }}</strong>
      </div>
      <div>
        <span class="muted caption">Total spent</span><strong>{{ moneyLabel(account.usage) }}</strong>
      </div>
      <div>
        <span class="muted caption">Total credits purchased</span><strong>{{ moneyLabel(account.credits) }}</strong>
      </div>
    </div>
    <p v-if="account.checkedAt" class="muted caption usage-checked">
      Checked {{ new Date(account.checkedAt * 1000).toLocaleString() }}
    </p>
    <section
      v-if="account.configured && account.balance != null"
      class="openrouter-models"
      aria-labelledby="openrouter-model-heading"
    >
      <h4 id="openrouter-model-heading">Usage by model</h4>
      <p class="muted caption">
        Last 30 complete UTC days<span v-if="account.activityFrom && account.activityTo">
          · {{ account.activityFrom }} – {{ account.activityTo }}</span
        >. Today is not included.
      </p>
      <p v-if="account.activityMessage" class="muted usage-unavailable" role="status">{{ account.activityMessage }}</p>
      <p v-else-if="!account.models?.length" class="muted usage-unavailable">No model usage in this period.</p>
      <div
        v-else
        class="openrouter-model-table"
        tabindex="0"
        role="region"
        aria-label="Model usage table, scroll horizontally for all columns"
      >
        <table>
          <thead>
            <tr>
              <th scope="col">Model</th>
              <th scope="col">Spend (credits)</th>
              <th scope="col">Requests</th>
              <th scope="col">Tokens</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="model in account.models" :key="model.model">
              <th scope="row">{{ model.model }}</th>
              <td>
                {{ moneyLabel(model.usage, 6)
                }}<small v-if="model.byok > 0" class="muted">BYOK {{ moneyLabel(model.byok, 6) }}</small>
              </td>
              <td>{{ countLabel(model.requests) }}</td>
              <td>
                {{ countLabel(model.promptTokens + model.completionTokens)
                }}<small class="muted"
                  >In {{ countLabel(model.promptTokens) }} · Out {{ countLabel(model.completionTokens) }}</small
                ><small v-if="model.reasoningTokens > 0" class="muted"
                  >Reasoning {{ countLabel(model.reasoningTokens) }}</small
                >
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-if="account.models?.some((model) => model.byok > 0)" class="muted caption openrouter-byok-note">
        External BYOK inference costs are shown separately from OpenRouter credit spend.
      </p>
    </section>
    <form v-if="!account.configured || editing" class="openrouter-key-form" @submit.prevent="connect">
      <label for="openrouter-management-key">Management key</label>
      <p id="openrouter-key-help" class="muted caption">
        Enter a management key to see your account balance and total spending{{ profileId ? ' for this profile' : '' }}.
        Stored in macOS Keychain.
      </p>
      <input
        id="openrouter-management-key"
        v-model="key"
        type="password"
        autocomplete="off"
        autocapitalize="off"
        spellcheck="false"
        maxlength="2048"
        placeholder="Paste your management key"
        aria-describedby="openrouter-key-help"
        :disabled="busy"
      />
      <div class="openrouter-key-actions">
        <button type="submit" class="standard-button" :disabled="busy || !key.trim()">
          {{ busy ? 'Checking…' : account.configured ? 'Save key' : 'Connect' }}</button
        ><button v-if="account.configured" type="button" class="standard-button" :disabled="busy" @click="cancel">
          Cancel</button
        ><button
          type="button"
          class="text-link muted caption"
          @click="openURL('https://openrouter.ai/settings/management-keys')"
        >
          Get a management key ↗
        </button>
      </div>
    </form>
    <div v-else class="openrouter-key-actions">
      <!-- prettier-ignore-attribute @click --><button
        class="standard-button"
        :disabled="busy"
        @click="editing=true;error=''"
      >
        Change key</button
      ><button class="text-link muted caption" :disabled="busy" @click="remove">Remove key</button>
    </div>
  </section>
</template>
