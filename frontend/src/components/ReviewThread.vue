<script setup>
// One review comment in the Changes pane: pending until sent, then with the agent's reply.
import { rangeLabel } from '../shared/review'
defineProps({ comment: Object, run: Object, index: Number, outdated: Boolean, busy: Boolean })
const emit = defineEmits(['edit', 'delete'])
</script>
<template>
  <article class="review-thread" :class="{ outdated }" :aria-label="'Comment on ' + rangeLabel(comment)">
    <header>
      <strong>{{ run ? `${index + 1}. ` : '' }}{{ rangeLabel(comment) }}</strong
      ><span v-if="outdated" class="review-badge">Outdated</span
      ><span v-if="!run" class="review-badge pending">Pending</span>
    </header>
    <p v-if="outdated && comment.excerpt" class="review-excerpt">{{ comment.excerpt }}</p>
    <p class="review-text">{{ comment.text }}</p>
    <div v-if="run" class="review-reply">
      <p v-if="comment.reply" class="review-text">{{ comment.reply }}</p>
      <p v-else-if="['queued', 'running'].includes(run.status)" class="muted caption">
        {{ run.status === 'queued' ? 'Waiting to be sent to the agent.' : 'Waiting for the agent’s reply.' }}
      </p>
      <p v-else class="muted caption">No reply.</p>
    </div>
    <footer v-else>
      <button class="standard-button caption" :disabled="busy" @click="emit('edit', comment)">Edit</button
      ><button class="standard-button caption" :disabled="busy" @click="emit('delete', comment)">Delete</button>
    </footer>
  </article>
</template>
