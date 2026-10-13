<script setup>
import { computed } from 'vue'
import { openURL } from '../shared/api'
import { renderMarkdown } from '../shared/markdown'
const props = defineProps({ status: Object, preferences: Object, runningSessions: Number })
const emit = defineEmits(['check', 'skip', 'install', 'cancel', 'close'])
const release = computed(() => props.status?.available)
const staged = computed(() => release.value && props.status.staged === release.value.version)
const notes = computed(() => (release.value?.notes ? renderMarkdown(release.value.notes) : ''))
const sessionsNote = computed(() =>
  props.runningSessions
    ? `Restarting stops ${props.runningSessions} running ${props.runningSessions === 1 ? 'session' : 'sessions'}.`
    : '',
)
// Release notes link to pull requests and issues; they open in the browser, never the webview.
function linkClick(event) {
  const link = event.target.closest('a')
  if (!link) return
  event.preventDefault()
  openURL(link.href)
}
function download() {
  openURL(release.value.url)
  emit('close')
}
</script>
<template>
  <div class="close-dialog update-dialog">
    <template v-if="status?.checking && !release">
      <h2>Checking for Updates…</h2>
      <p class="muted" role="status">Looking for a newer version of Orchestra.</p>
      <footer><button class="standard-button" @click="emit('close')">Cancel</button></footer>
    </template>
    <template v-else-if="status && !status.supported">
      <h2>Updates Unavailable</h2>
      <p>{{ status.error || 'Development builds cannot check for updates.' }}</p>
      <footer><button class="standard-button" autofocus @click="emit('close')">OK</button></footer>
    </template>
    <template v-else-if="status?.error && !release">
      <h2>Could Not Check for Updates</h2>
      <p class="update-error" role="alert">{{ status.error }}</p>
      <footer>
        <button class="standard-button" @click="emit('close')">OK</button
        ><button class="standard-button primary" autofocus @click="emit('check')">Try Again</button>
      </footer>
    </template>
    <template v-else-if="release && status.preparing">
      <h2>Downloading Orchestra {{ release.version }}…</h2>
      <p class="muted" role="status">
        The update is downloaded and verified before anything changes. You can keep working.
      </p>
      <footer><button class="standard-button" autofocus @click="emit('cancel')">Cancel</button></footer>
    </template>
    <template v-else-if="staged">
      <h2>Orchestra {{ release.version }} Is Ready to Install</h2>
      <p>The update installs when you quit Orchestra. {{ sessionsNote }}</p>
      <footer>
        <button class="standard-button" @click="emit('close')">Later</button
        ><button class="standard-button primary" autofocus @click="emit('install', true)">Restart Now</button>
      </footer>
    </template>
    <template v-else-if="release">
      <h2>Orchestra {{ release.version }} Is Available</h2>
      <p>
        You have version {{ status.current }}.
        {{ release.prerelease ? 'This is a beta release.' : '' }}
        <template v-if="status.installable">{{ sessionsNote }}</template>
        <template v-else>{{
          status.installNote || 'Download the new version and replace the app to update.'
        }}</template>
      </p>
      <!-- prettier-ignore-attribute @click -->
      <div
        v-if="notes"
        class="markdown update-notes"
        tabindex="0"
        aria-label="Release notes"
        @click="linkClick"
        v-html="notes"
      />
      <p v-if="status.installError" class="update-error" role="alert">{{ status.installError }}</p>
      <footer>
        <button class="standard-button update-skip" @click="emit('skip', release.version)">Skip This Version</button
        ><button class="standard-button" @click="emit('close')">Later</button
        ><template v-if="status.installable"
          ><button class="standard-button" @click="emit('install', false)">Install When I Quit</button
          ><button class="standard-button primary" autofocus @click="emit('install', true)">
            Install and Restart
          </button></template
        ><button v-else class="standard-button primary" autofocus @click="download">Download…</button>
      </footer>
    </template>
    <template v-else>
      <h2>Orchestra Is Up to Date</h2>
      <p>
        Version {{ status?.current }} is the newest
        {{ preferences.betaUpdates ? 'release, including betas' : 'release' }}.
      </p>
      <footer><button class="standard-button" autofocus @click="emit('close')">OK</button></footer>
    </template>
  </div>
</template>
