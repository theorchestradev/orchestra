<script setup>
import Icon from './Icon.vue'
import WorkOnButton from './WorkOnButton.vue'
// A pull request's confirmed merge conflict, with a Resolve conflicts session to start.
defineProps({
  conflict: Object,
  defaultAgent: String,
  profiles: Array,
  profileId: String,
  visible: Boolean,
  // Set once GitHub refused an edit, which suggests the user can't push either.
  readOnly: Boolean,
})
const emit = defineEmits(['resolve'])
</script>
<template>
  <section class="conflict-notice" aria-label="Merge conflicts">
    <p>
      <Icon name="exclamationmark.triangle" :size="14" /><span>
        This branch has conflicts with <code>{{ conflict.base }}</code> that must be resolved.<span
          v-if="conflict.forkWithoutEdits || readOnly"
          class="muted"
        >
          {{
            conflict.forkWithoutEdits
              ? 'It comes from a fork that doesn’t allow edits by maintainers, so the push may fail.'
              : 'You may not have push access to this repository, so the push may fail.'
          }}</span
        ></span
      >
    </p>
    <WorkOnButton
      label="Resolve conflicts"
      :default-agent="defaultAgent"
      :profiles="profiles"
      :profile-id="profileId"
      :visible="visible"
      @work="emit('resolve', $event)"
    />
  </section>
</template>
