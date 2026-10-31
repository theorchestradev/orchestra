<script setup>
import { computed, ref, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { call } from '../shared/api'
import Modal from './Modal.vue'
import Avatar from './Avatar.vue'
import Icon from './Icon.vue'
// repository is the workspace repository the item belongs to.
const props = defineProps({
  workspace: Object,
  repository: Object,
  item: Object,
  pull: Boolean,
  field: String,
  anchor: Object,
})
const emit = defineEmits(['close', 'saved', 'busy'])
const inline = props.field !== 'labels',
  popup = ref(null),
  position = ref({})
function place() {
  if (!inline || !popup.value || !props.anchor) return
  const r = props.anchor.getBoundingClientRect(),
    height = popup.value.offsetHeight,
    width = popup.value.offsetWidth
  position.value = {
    left: Math.max(8, Math.min(r.left, innerWidth - width - 8)) + 'px',
    top: Math.max(8, r.bottom + 6 + height > innerHeight ? r.top - height - 6 : r.bottom + 6) + 'px',
  }
}
function outside(event) {
  if (!popup.value?.contains(event.target) && !props.anchor?.contains(event.target)) close()
}
function escape(event) {
  if (event.key === 'Escape') {
    event.preventDefault()
    event.stopPropagation()
    if (!saving.value) {
      props.anchor?.focus()
      close()
    }
  }
}
const title = { state: 'Status', assignees: 'Assignees', labels: 'Labels' }[props.field]
const key = props.field === 'assignees' ? 'login' : 'name'
const original =
  props.field === 'state'
    ? props.item.state === 'closed' && !props.pull
      ? ['closed', props.item.state_reason || 'completed']
      : [props.item.state]
    : (props.item[props.field] || []).map((o) => o[key])
const values = ref([...original]),
  options = ref([]),
  query = ref(''),
  loading = ref(props.field !== 'state'),
  saving = ref(false),
  error = ref('')
const duplicates = ref(false),
  duplicateItems = ref([])
const duplicateChoices = computed(() =>
  duplicateItems.value.filter(
    (i) =>
      !i.pull_request &&
      i.number !== props.item.number &&
      (i.title.toLowerCase().includes(query.value.toLowerCase()) || ('#' + i.number).includes(query.value)),
  ),
)
const statusKey = props.item.state === 'open' ? 'open' : props.pull ? 'closed' : props.item.state_reason || 'completed'
const statuses = props.pull
  ? [
      { id: 'open', label: 'Open', icon: 'open' },
      { id: 'closed', label: 'Closed', icon: 'completed' },
    ]
  : [
      { id: 'open', label: 'Open', icon: 'open' },
      { id: 'completed', label: 'Close as completed', icon: 'completed' },
      { id: 'not_planned', label: 'Close as not planned', icon: 'not-planned' },
      { id: 'duplicate', label: 'Close as duplicate', icon: 'duplicate' },
    ]
let active = true
onBeforeUnmount(() => {
  active = false
  if (inline) {
    document.removeEventListener('pointerdown', outside)
    document.removeEventListener('keydown', escape, true)
    window.removeEventListener('scroll', place, true)
    window.removeEventListener('resize', place)
  }
})
watch(query, async () => {
  await nextTick()
  place()
})
const choices = computed(() => options.value.filter((o) => o[key].toLowerCase().includes(query.value.toLowerCase())))
const changed = computed(() => JSON.stringify([...values.value].sort()) !== JSON.stringify([...original].sort()))
async function load() {
  loading.value = true
  error.value = ''
  try {
    const available = await call('GitHubMetadataOptions', props.workspace.id, props.repository.id, props.field)
    if (!active) return
    const all = new Map([...available, ...(props.item[props.field] || [])].map((o) => [o[key], o]))
    options.value = [...all.values()].sort((a, b) => a[key].localeCompare(b[key]))
  } catch (e) {
    if (active) error.value = String(e)
  } finally {
    if (active) {
      loading.value = false
      await nextTick()
      place()
      if (inline && (document.activeElement === document.body || document.activeElement === props.anchor))
        popup.value.querySelector('input,button')?.focus()
    }
  }
}
onMounted(async () => {
  if (props.field !== 'state') load()
  if (inline) {
    await nextTick()
    place()
    ;(
      popup.value.querySelector('.metadata-state-option.selected') || popup.value.querySelector('input,button')
    )?.focus()
    document.addEventListener('pointerdown', outside)
    document.addEventListener('keydown', escape, true)
    window.addEventListener('scroll', place, true)
    window.addEventListener('resize', place)
  }
})
function patch() {
  return props.field === 'state'
    ? { state: values.value[0], state_reason: values.value[0] === 'open' ? 'reopened' : values.value[1] || null }
    : { [props.field]: options.value.filter((o) => values.value.includes(o[key])) }
}
async function save() {
  if (saving.value || !changed.value) return
  saving.value = true
  emit('busy', true)
  error.value = ''
  try {
    await call(
      'UpdateGitHubMetadata',
      props.workspace.id,
      props.repository.id,
      props.item.number,
      props.pull,
      props.field,
      [...values.value],
    )
    if (active) emit('saved', patch())
  } catch (e) {
    if (active) {
      error.value = String(e)
      if (props.field === 'assignees') values.value = [...original]
    }
  } finally {
    if (active) {
      saving.value = false
      emit('busy', false)
      await nextTick()
      place()
    }
  }
}
function menuKeys(event) {
  if (props.field !== 'state' || !['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return
  event.preventDefault()
  const buttons = [...popup.value.querySelectorAll('button:not(:disabled)')],
    index = buttons.indexOf(document.activeElement)
  const next =
    event.key === 'Home'
      ? 0
      : event.key === 'End'
        ? buttons.length - 1
        : (index + (event.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
  buttons[next]?.focus()
}
async function chooseState(value) {
  if (value === statusKey) {
    close()
    return
  }
  values.value = value === 'open' ? ['open'] : props.pull ? ['closed'] : ['closed', value]
  await save()
}
async function showDuplicates() {
  duplicates.value = true
  query.value = ''
  error.value = ''
  loading.value = true
  try {
    const items = await call('GitHubItems', props.workspace.id, props.repository.id, 'all')
    if (active) duplicateItems.value = items
  } catch (e) {
    if (active) error.value = String(e)
  } finally {
    if (active) {
      loading.value = false
      await nextTick()
      place()
      popup.value.querySelector('input')?.focus()
    }
  }
}
async function duplicateOf(item) {
  values.value = ['closed', 'duplicate', String(item.number)]
  await save()
}
async function statusMenu() {
  duplicates.value = false
  error.value = ''
  await nextTick()
  place()
  popup.value.querySelector('.metadata-state-option.selected')?.focus()
}
function close() {
  if (!saving.value) emit('close')
}
</script>
<template>
  <Teleport to=".app-shell"
    ><component
      :is="inline ? 'section' : Modal"
      ref="popup"
      :class="{
        'metadata-popover': inline,
        'metadata-state-popover': field === 'state',
        'metadata-assignee-popover': field === 'assignees',
      }"
      :style="inline ? position : undefined"
      :role="inline ? (field === 'state' && !duplicates ? 'menu' : 'region') : undefined"
      :aria-label="inline ? 'Select ' + title : undefined"
      :label="inline ? undefined : 'Edit ' + title"
      @close="close"
      ><form class="metadata-editor" @submit.prevent="save" @keydown.stop="menuKeys">
        <header v-if="!inline">
          <h2>Edit {{ title }}</h2>
          <p class="muted caption">{{ repository.gitHub }} #{{ item.number }}</p>
        </header>
        <template v-if="field === 'state'">
          <template v-if="!duplicates"
            ><button
              v-for="option in statuses"
              :key="option.id"
              type="button"
              class="metadata-state-option"
              :class="{ selected: statusKey === option.id }"
              :role="option.id === 'duplicate' ? 'menuitem' : 'menuitemradio'"
              :aria-checked="option.id === 'duplicate' ? undefined : statusKey === option.id"
              :aria-haspopup="option.id === 'duplicate' ? 'true' : undefined"
              :disabled="saving"
              @click="option.id === 'duplicate' ? showDuplicates() : chooseState(option.id)"
            >
              <Icon v-if="option.icon === 'duplicate'" name="doc.on.doc" :size="18" class="muted" /><span
                v-else
                class="status-choice-icon"
                :class="option.icon"
                aria-hidden="true"
              /><span>{{ option.label }}</span
              ><Icon v-if="option.id === 'duplicate'" name="chevron.right" :size="10" class="muted" /></button
          ></template>
          <template v-else
            ><button type="button" class="duplicate-back muted" :disabled="saving" @click="statusMenu">
              <Icon name="chevron.left" :size="10" />Close as duplicate</button
            ><input
              autocorrect="off"
              autocapitalize="off"
              v-model="query"
              aria-label="Search duplicate issues"
              placeholder="Search issues by title or number"
              :disabled="saving"
            />
            <div v-if="loading" class="metadata-loading" role="status"><span class="spinner" />Loading issues…</div>
            <div v-else class="duplicate-options">
              <button
                v-for="item in duplicateChoices"
                :key="item.number"
                type="button"
                class="metadata-state-option"
                :title="item.title"
                :disabled="saving"
                @click="duplicateOf(item)"
              >
                <span class="muted caption">#{{ item.number }}</span
                ><span class="truncate">{{ item.title }}</span>
              </button>
              <p v-if="!duplicateChoices.length" class="muted caption">No matching issues.</p>
            </div>
            <button
              v-if="error && !duplicateItems.length"
              type="button"
              class="standard-button"
              @click="showDuplicates"
            >
              Try Again
            </button></template
          >
          <p v-if="saving" class="muted caption" role="status">Saving…</p></template
        >
        <template v-else
          ><input
            autocorrect="off"
            autocapitalize="off"
            v-if="field === 'labels' || options.length > 8"
            v-model="query"
            autofocus
            :aria-label="'Search ' + title.toLowerCase()"
            :placeholder="'Search ' + title.toLowerCase()"
            :disabled="saving"
          />
          <div v-if="loading" class="metadata-loading" role="status">
            <span class="spinner" />Loading {{ title.toLowerCase() }}…
          </div>
          <div v-else class="metadata-options" :aria-label="title">
            <label v-for="option in choices" :key="option[key]" class="metadata-option"
              ><input
                type="checkbox"
                :aria-label="option[key]"
                v-model="values"
                :value="option[key]"
                :disabled="saving"
                @change="field === 'assignees' && save()"
              /><Avatar v-if="field === 'assignees'" :author="option" /><i
                v-else
                class="label-dot"
                :style="{ background: /^[0-9a-f]{6}$/i.test(option.color) ? '#' + option.color : 'var(--muted)' }"
              /><span>{{ option[key] }}</span></label
            >
            <p v-if="!choices.length" class="muted">
              {{ query ? 'No matches.' : 'No available ' + title.toLowerCase() + '.' }}
            </p>
          </div>
          <div v-if="field === 'labels'" class="metadata-selection">
            <span class="muted caption">{{ values.length }} selected</span
            ><button
              type="button"
              class="text-link accent"
              :disabled="loading || saving || !values.length"
              @click="values = []"
            >
              Clear selection
            </button>
          </div></template
        >
        <p v-if="error" class="metadata-error" role="alert">{{ error }}</p>
        <button
          v-if="error && field !== 'state' && !options.length"
          type="button"
          class="standard-button"
          :disabled="saving || loading"
          @click="load"
        >
          Try Again
        </button>
        <footer v-if="field === 'labels'">
          <button type="button" class="standard-button" :disabled="saving" @click="close">Cancel</button
          ><button
            type="submit"
            class="standard-button primary"
            :disabled="loading || saving || !changed || (field !== 'state' && !!error && !options.length)"
          >
            {{ saving ? 'Saving…' : 'Save' }}
          </button>
        </footer>
      </form></component
    ></Teleport
  >
</template>
