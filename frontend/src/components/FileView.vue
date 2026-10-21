<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { call } from '../shared/api'
import { fileLanguage, renderCode } from '../shared/syntax.js'
import { worktreeName } from '../shared/openIn.js'
import Icon from './Icon.vue'
const props = defineProps({ file: Object, session: Object })
const emit = defineEmits(['close'])
const preview = ref(null),
  error = ref('')
const highlighted = computed(() => renderCode(props.file.path, preview.value?.content || 'Empty file.'))
const lineNumbers = computed(() =>
  Array.from({ length: (preview.value?.content || '').split('\n').length }, (_, index) => index + 1),
)
let request = 0,
  timer
async function refresh() {
  const current = ++request
  error.value = ''
  try {
    const result = await call('SessionFiles', props.file.sessionID, props.file.path)
    if (current === request) preview.value = result
  } catch (e) {
    if (current === request) {
      error.value = String(e)
      preview.value = null
    }
  }
}
watch(
  () => props.file.id,
  () => {
    preview.value = null
    refresh()
  },
  { immediate: true },
)
watch([() => props.session?.activity, () => props.session?.running], () => {
  clearTimeout(timer)
  timer = setTimeout(refresh, 400)
})
onBeforeUnmount(() => {
  request++
  clearTimeout(timer)
})
</script>
<template>
  <section class="session-file-preview" aria-label="File preview">
    <header>
      <div class="file-view-title">
        <h2 class="truncate" :title="file.path">{{ file.path }}</h2>
        <span class="muted caption">{{ [worktreeName(session), 'Read-only'].filter(Boolean).join(' · ') }}</span>
      </div>
      <button class="icon-button" aria-label="Close file" title="Close file" @click="emit('close')">
        <Icon name="xmark" :size="12" />
      </button>
    </header>
    <div v-if="error" class="diff-empty" role="alert">
      <p>{{ error }}</p>
      <button class="standard-button" @click="refresh">Try Again</button>
    </div>
    <div v-else-if="!preview" class="diff-empty"><span class="spinner" /></div>
    <p v-else-if="preview.message || preview.directory" class="diff-placeholder muted" role="status">
      {{ preview.message || 'This path is now a folder.' }}
    </p>
    <div v-else class="file-code-scroll" tabindex="0" :aria-label="file.path">
      <div class="file-line-numbers" aria-hidden="true">
        <span v-for="number in lineNumbers" :key="number">{{ number }}</span>
      </div>
      <pre><code :class="'language-' + fileLanguage(file.path)" v-html="highlighted"></code></pre>
    </div>
  </section>
</template>
