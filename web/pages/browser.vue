<template>
  <main class="page">
    <section class="page-header"><div><h2 class="page-title">Server browser</h2><p class="page-copy">Watch Chromium on the server or take control of its page.</p></div></section>
    <AppPanel title="Owner access">
      <label>Owner control token <input v-model="token" type="password" autocomplete="off" aria-label="Owner control token" /></label>
      <AppButton :disabled="!token || pending" @click="load">Load browsers</AppButton>
      <p>Access and typed values stay in this page's memory. Closing this page leaves manual control paused until you explicitly return it.</p>
      <AppAlert v-if="error" tone="error">{{ error }}</AppAlert>
    </AppPanel>
    <AppPanel title="Browser sessions">
      <p v-if="loaded && !sessions.length">No live browser sessions. Ask the agent to open a server browser.</p>
      <label v-if="sessions.length">Session <select :value="selectedID" aria-label="Browser session" :disabled="pending || manual" @change="selectSession(($event.target as HTMLSelectElement).value)"><option v-for="session in sessions" :key="session.browser_session_id" :value="session.browser_session_id">{{ session.task_id || session.run_id }} · {{ session.mode }}</option></select></label>
      <template v-if="selected">
        <p>{{ manual ? 'You control this browser. Agent browser actions are paused.' : selected.mode === 'owner' ? 'Manual control is active. Take control to recover this session.' : 'Viewing agent actions.' }}</p>
        <p v-if="frame?.url" class="current-url">Current page: {{ frame.url }}</p>
        <div class="actions">
          <AppButton :disabled="pending || manual || !frame" @click="take">Take control</AppButton>
          <AppButton :disabled="pending || !manual" @click="release">Return to agent</AppButton>
          <AppButton :disabled="pending" @click="refresh">Refresh view</AppButton>
        </div>
        <form v-if="manual" class="actions" @submit.prevent="navigate"><label>Address <input v-model="address" aria-label="Browser address" placeholder="https://example.com" /></label><AppButton :disabled="pending || !address" type="submit">Go</AppButton></form>
        <div class="viewport" :class="{ controlling: manual }" tabindex="0" role="application" aria-label="Remote browser viewport" @keydown.prevent="keyboard" @contextmenu.prevent>
          <img v-if="frame" :src="`data:image/jpeg;base64,${frame.jpeg}`" alt="Live server browser page" draggable="false" @click="pointer" @wheel.prevent="wheel" />
          <p v-else>Waiting for a browser frame.</p>
        </div>
        <form v-if="manual" class="actions" @submit.prevent="insertText"><label>Type or paste text <input v-model="text" type="password" autocomplete="off" aria-label="Remote text" /></label><AppButton type="submit" :disabled="pending || !text">Send text</AppButton></form>
        <p>Taking control resumes the site's background requests until you return control or this session expires.</p>
        <p v-if="manual">Click the page, then use your keyboard. Tab, Enter, arrows, shortcuts and scrolling are supported. Input is sent once; after an error, refresh and check the page before trying again.</p>
      </template>
    </AppPanel>
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppButton from '~/shared/ui/AppButton.vue'
import AppAlert from '~/shared/ui/AppAlert.vue'
import { ownerAuthorityRequest } from '~/shared/api/owner-goals'

interface BrowserSession { browser_session_id: string; run_id: string; task_id: string; mode: string; epoch: number }
interface Frame { jpeg: string; frame_id: string; width: number; height: number; mode: string; epoch: number; url?: string }
interface Reply { ok: boolean; sessions?: BrowserSession[]; frame?: Frame }
const token = ref(''), sessions = ref<BrowserSession[]>([]), selectedID = ref(''), frame = ref<Frame | null>(null)
const pending = ref(false), loaded = ref(false), error = ref(''), manual = ref(false), text = ref(''), address = ref('')
const selected = computed(() => sessions.value.find(item => item.browser_session_id === selectedID.value))
let controller = '', epoch = 0, timer: ReturnType<typeof setTimeout> | undefined, generation = 0, disposed = false, pumping = false
const keys: string[] = []
const stop = () => { if (timer) clearTimeout(timer); timer = undefined }
const request = (body: Record<string, unknown>) => ownerAuthorityRequest<Reply>('/api/v4/server-browser', token.value, body)
const schedule = () => { stop(); if (!disposed && selectedID.value && token.value) timer = setTimeout(() => { void refresh() }, 900) }
async function action(body: Record<string, unknown>) {
  if (pending.value) return
  stop(); pending.value = true; if (body.operation !== 'frame') error.value = ''
  const current = generation
  try {
    const reply = await request(body)
    if (current !== generation || disposed) return
    if (reply.frame) {
      frame.value = reply.frame
      if (manual.value && (reply.frame.epoch !== epoch || reply.frame.mode !== 'owner')) { manual.value = false; controller = ''; keys.length = 0; text.value = ''; error.value = 'Control changed. Load browsers before taking control again.' }
    }
    return reply
  } catch { if (current === generation) { keys.length = 0; error.value = 'Browser unavailable or page changed. Refresh and check the page before another input.' } }
  finally { pending.value = false; schedule(); if (keys.length && !pumping) void pumpKeys() }
}
async function load() {
  const reply = await action({ operation: 'list' })
  if (!reply) return
  loaded.value = true; sessions.value = reply.sessions || []
  if (!selected.value) { manual.value = false; controller = ''; frame.value = null; selectedID.value = sessions.value[0]?.browser_session_id || '' }
  if (selectedID.value) await refresh()
}
async function selectSession(id: string) { generation++; stop(); selectedID.value = id; manual.value = false; controller = ''; frame.value = null; await refresh() }
async function refresh() { if (!selectedID.value || pending.value) return; await action({ operation: 'frame', browser_session_id: selectedID.value }) }
async function take() {
  if (!selected.value) return
  controller = [...crypto.getRandomValues(new Uint8Array(16))].map(value => value.toString(16).padStart(2, '0')).join('')
  epoch = selected.value.epoch
  const reply = await action({ operation: 'take', browser_session_id: selectedID.value, controller, epoch })
  if (reply) { epoch++; manual.value = true; selected.value.mode = 'owner'; selected.value.epoch = epoch }
  else { controller = ''; manual.value = false; await load() }
}
async function release() {
  keys.length = 0
  const reply = await action({ operation: 'release', browser_session_id: selectedID.value, controller, epoch })
  if (reply) { manual.value = false; controller = ''; text.value = ''; await load() }
}
async function input(operation: string, extra: Record<string, unknown>) {
  if (!manual.value || !frame.value || pending.value) return
  await action({ operation, browser_session_id: selectedID.value, controller, epoch, frame_id: frame.value.frame_id, ...extra })
}
function coordinates(event: MouseEvent | WheelEvent) {
  const rect = (event.currentTarget as HTMLImageElement).getBoundingClientRect()
  return { x: Math.max(0, Math.min(1279, (event.clientX - rect.left) * 1280 / rect.width)), y: Math.max(0, Math.min(719, (event.clientY - rect.top) * 720 / rect.height)) }
}
function pointer(event: MouseEvent) { (event.currentTarget as HTMLElement).parentElement?.focus(); void input('click', { ...coordinates(event), button: 'left' }) }
function wheel(event: WheelEvent) { void input('wheel', { ...coordinates(event), delta_x: Math.max(-2000, Math.min(2000, event.deltaX)), delta_y: Math.max(-2000, Math.min(2000, event.deltaY)) }) }
function keyboard(event: KeyboardEvent) {
  if (['Shift', 'Control', 'Alt', 'Meta'].includes(event.key) || event.repeat) return
  const key = [event.ctrlKey ? 'Control' : '', event.metaKey ? 'Meta' : '', event.altKey ? 'Alt' : '', event.shiftKey && event.key.length > 1 ? 'Shift' : '', event.key === ' ' ? 'Space' : event.key].filter(Boolean).join('+')
  if (!manual.value) return
  if (keys.length >= 64) { error.value = 'Keyboard queue is full. Wait for the page to catch up.'; return }
  keys.push(key); void pumpKeys()
}
async function pumpKeys() {
  if (pumping || pending.value) return
  pumping = true
  try { while (keys.length && manual.value && !disposed) { const key = keys.shift(); const reply = await action({ operation: 'key', browser_session_id: selectedID.value, controller, epoch, frame_id: frame.value?.frame_id, key }); if (!reply) { keys.length = 0; break } } }
  finally { pumping = false }
}
async function insertText() { const value = text.value; text.value = ''; await input('text', { text: value }) }
async function navigate() { await input('navigate', { url: address.value }) }
watch(token, () => { generation++; stop(); keys.length = 0; frame.value = null; sessions.value = []; selectedID.value = ''; manual.value = false; controller = ''; text.value = ''; address.value = ''; loaded.value = false })
onBeforeUnmount(() => { disposed = true; generation++; stop(); keys.length = 0; token.value = ''; frame.value = null; text.value = ''; address.value = ''; controller = '' })
</script>

<style scoped>
.actions { display: flex; align-items: end; gap: 12px; flex-wrap: wrap; margin: 12px 0; }
label { display: inline-flex; gap: 8px; align-items: center; flex-wrap: wrap; }
input, select { padding: 8px; border: 1px solid var(--color-border-default); background: var(--color-bg-surface); border-radius: 6px; }
.viewport { max-width: 1280px; min-height: 160px; background: var(--color-bg-surface); border: 2px solid var(--color-border-default); border-radius: 8px; overflow: hidden; }
.viewport.controlling { border-color: var(--color-accent-primary); }
.viewport:focus { outline: 2px solid var(--color-accent-primary); }
.viewport img { width: 100%; display: block; user-select: none; }
.viewport p { padding: 24px; color: var(--color-text-primary); }
.current-url { overflow-wrap: anywhere; }
</style>
