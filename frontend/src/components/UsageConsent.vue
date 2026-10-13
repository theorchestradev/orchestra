<script setup>
import { openURL } from '../shared/api'
import { privacyURL } from '../shared/telemetry'
defineProps({ status: Object, busy: Boolean })
const emit = defineEmits(['choose'])
</script>
<template>
  <aside class="usage-consent" aria-label="Anonymous usage sharing">
    <div>
      <p>Help improve Orchestra by sharing feature usage and approximate city and country, with IPs discarded.</p>
      <button class="text-link" @click="openURL(privacyURL)">What is collected?</button>
      <p v-if="status?.reason" class="muted caption">{{ status.reason }}</p>
    </div>
    <div class="usage-consent-actions">
      <button class="standard-button" :disabled="busy" @click="emit('choose', false)">No Thanks</button>
      <button class="standard-button primary" :disabled="busy || !status?.available" @click="emit('choose', true)">
        Share Anonymous Usage
      </button>
    </div>
  </aside>
</template>
