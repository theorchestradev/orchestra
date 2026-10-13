<script setup>
import { computed } from 'vue'
import { sessionsLabel } from '../shared/github'
import Icon from './Icon.vue'
const props = defineProps({ sessions: Array })
const emit = defineEmits(['open'])
const label = computed(() => sessionsLabel(props.sessions))
</script>
<template>
  <button
    v-if="sessions.length"
    class="icon-button github-sessions"
    :aria-label="label"
    :title="label"
    @click.stop="emit('open', sessions.at(-1))"
    @contextmenu.stop
  >
    <Icon name="terminal" :size="13" />
  </button>
</template>
