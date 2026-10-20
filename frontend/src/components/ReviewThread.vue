<script setup>
// One review comment in the Changes pane: pending until sent, then with the agent's reply.
import { openURL } from '../shared/api'
import { agentIcon } from '../shared/appearance'
import { renderMarkdown } from '../shared/markdown'
import { rangeLabel } from '../shared/review'
defineProps({
  comment: Object,
  run: Object,
  index: Number,
  outdated: Boolean,
  busy: Boolean,
  session: Object,
  appearance: String,
})
const emit = defineEmits(['edit', 'delete'])
// Links in a reply open in the external browser and never navigate the webview.
function linkClick(event) {
  const link = event.target.closest('a')
  if (!link) return
  event.preventDefault()
  openURL(link.href)
}
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
    <section v-if="run" class="review-reply" :aria-label="'Reply from ' + session.name">
      <header class="review-author">
        <img :src="agentIcon(session.agent, appearance)" alt="" /><strong class="truncate">{{ session.name }}</strong>
      </header>
      <div
        v-if="comment.reply"
        class="markdown review-markdown"
        @click="linkClick"
        @auxclick.prevent
        v-html="renderMarkdown(comment.reply)"
      />
      <p v-else-if="['queued', 'running'].includes(run.status)" class="muted caption">
        {{ run.status === 'queued' ? 'Waiting to be sent to the agent.' : 'Working on a reply…' }}
      </p>
      <p v-else class="muted caption">No reply.</p>
    </section>
    <footer v-else>
      <button class="standard-button caption" :disabled="busy" @click="emit('edit', comment)">Edit</button
      ><button class="standard-button caption" :disabled="busy" @click="emit('delete', comment)">Delete</button>
    </footer>
  </article>
</template>
