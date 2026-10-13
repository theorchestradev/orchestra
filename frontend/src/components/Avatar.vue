<script setup>
import { computed, ref, watch } from 'vue'
const props = defineProps({ author: Object })
const failed = ref(false)
const source = computed(() => (props.author?.avatar_url?.startsWith('https://') ? props.author.avatar_url : ''))
watch(source, () => (failed.value = false))
</script>
<template>
  <span class="avatar" :title="author?.login" :aria-label="author?.login"
    ><img v-if="source && !failed" :src="source" alt="" loading="lazy" @error="failed = true" /><span v-else>{{
      (author?.login || '?').slice(0, 1).toUpperCase()
    }}</span></span
  >
</template>
