<template>
  <div class="mind-browser">
    <button class="mind-text-button" type="button" @click="mind.panel='approvals'">Запросы доступа к вкладкам ↗</button>
    <div class="mind-browser-bar"><label v-if="sessions.length"><span class="sr-only">Сессия браузера</span><select :value="selectedID" aria-label="Сессия браузера" :disabled="pending || manual" @change="selectSession(($event.target as HTMLSelectElement).value)"><option v-for="session in sessions" :key="session.browser_session_id" :value="session.browser_session_id">{{ session.task_id || session.run_id }}</option></select></label><span v-else class="mind-muted">{{ loaded?'Нет открытых сессий':'Подключаюсь…' }}</span><button class="mind-icon-button" type="button" aria-label="Обновить сессии браузера" :disabled="pending||manual" @click="load"><MindIcon name="refresh"/></button></div>
    <p v-if="error" class="mind-error" role="alert">{{ error }}</p>
    <template v-if="selected">
      <label v-if="pages.length" class="mind-browser-bar"><span>Страница</span><select aria-label="Страница браузера" :value="selectedPageID" :disabled="pending" @change="selectPage(($event.target as HTMLSelectElement).value)"><option v-for="page in pages" :key="page.page_id" :value="page.page_id">{{ page.url || 'Открывается…' }} · {{ page.page_id.slice(0,6) }}</option></select></label>
      <div class="mind-browser-state"><span role="status">{{ manual?'Вы управляете · агент ждёт':selected.mode==='owner'?'Ручное управление — восстановите доступ':'Управляет агент' }}</span><button v-if="!manual" class="mind-button" type="button" :disabled="pending||!frame" @click="take">Взять управление</button><button v-else class="mind-button primary" type="button" :disabled="pending" @click="release">Вернуть агенту</button></div>
      <form class="mind-browser-address" @submit.prevent="navigate"><MindIcon name="lock"/><label class="sr-only" for="browser-address">Адрес страницы</label><input id="browser-address" v-if="manual" v-model="address" placeholder="https://…" autocomplete="off" :disabled="pending"/><span v-else>{{ frame?.url || 'Страница сервера' }}</span><button v-if="manual" type="submit" class="mind-icon-button" aria-label="Перейти по адресу" :disabled="pending||!address"><MindIcon name="next"/></button><button type="button" class="mind-icon-button" aria-label="Обновить изображение страницы" :disabled="pending" @click="refresh"><MindIcon name="refresh"/></button></form>
      <div class="mind-browser-viewport" :class="{controlling:manual}" tabindex="0" role="application" aria-label="Страница серверного браузера" @keydown.prevent="keyboard" @contextmenu.prevent><img v-if="frame" :src="'data:image/jpeg;base64,'+frame.jpeg" alt="Страница серверного браузера" draggable="false" @click="pointer" @wheel.prevent="wheel"/><p v-else class="mind-muted">Ожидаю изображение страницы…</p></div>
      <form v-if="manual" class="mind-browser-input" @submit.prevent="insertText"><label class="sr-only" for="browser-text">Текст для страницы</label><input id="browser-text" v-model="text" type="password" placeholder="Ввести текст на странице…" autocomplete="off" :disabled="pending"/><button type="submit" class="mind-icon-button" aria-label="Ввести текст" :disabled="pending||!text"><MindIcon name="arrow"/></button></form>
      <p class="mind-muted mind-browser-hint">{{ manual?'Нажимайте на страницу, используйте клавиатуру и прокрутку. При закрытии панели агент продолжит ждать.':'Это браузер на сервере Butler. Можно наблюдать за работой или вмешаться.' }}</p>
      <details v-if="manual" class="mind-browser-details"><summary>О ручном управлении</summary><p class="mind-muted">Сайт продолжает фоновые запросы. Неподтверждённый ввод не повторяется автоматически. Чтобы агент продолжил работу, нажмите «Вернуть агенту».</p></details>
    </template>
    <div v-else-if="loaded" class="mind-browser-empty"><MindIcon name="browser"/><p>Попросите Батлера открыть нужную страницу.</p><button class="mind-button" type="button" @click="askForBrowser">Открыть через Батлера</button></div>
  </div>
</template>
<script setup lang="ts">
const mindPanelProps=defineProps<{ownerToken?:string}>()
import MindIcon from '../ui/MindIcon.vue'
import { useMind } from '../model/store'
const mind=useMind()
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ownerAuthorityRequest } from '~/shared/api/owner-goals'

interface BrowserSession { browser_session_id: string; run_id: string; task_id: string; mode: string; epoch: number }
interface OwnerPage {page_id:string;document_id:string;url:string;state:string}
interface Frame { jpeg: string; frame_id: string; width: number; height: number; mode: string; epoch: number; url?: string;page_id?:string;document_id?:string }
interface Reply { ok: boolean; sessions?: BrowserSession[]; frame?: Frame;pages?:OwnerPage[];managed_pages?:boolean }
const token = ref(mindPanelProps.ownerToken || ''), sessions = ref<BrowserSession[]>([]), selectedID = ref(''), frame = ref<Frame | null>(null)
const pending = ref(false), loaded = ref(false), error = ref(''), manual = ref(false), text = ref(''), address = ref('')
const pages=ref<OwnerPage[]>([]),selectedPageID=ref('')
const selected = computed(() => sessions.value.find(item => item.browser_session_id === selectedID.value))
let controller = '', epoch = 0, timer: ReturnType<typeof setTimeout> | undefined, generation = 0, disposed = false, pumping = false
const keys: {key:string;page?:string;document?:string}[] = []
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
    if(reply.managed_pages){
      pages.value=reply.pages||[]
      if(!pages.value.some(page=>page.page_id===selectedPageID.value)){selectedPageID.value=pages.value[0]?.page_id||'';frame.value=null;keys.length=0;text.value=''}
    }
    if (reply.frame) {
      frame.value = reply.frame
      if(reply.frame.page_id) selectedPageID.value=reply.frame.page_id
      if (manual.value && (reply.frame.epoch !== epoch || reply.frame.mode !== 'owner')) { manual.value = false; controller = ''; keys.length = 0; text.value = ''; error.value = 'Управление изменилось. Обновите сессии перед новым действием.' }
    }
    return reply
  } catch { if (current === generation) { keys.length = 0; error.value = 'Команда не подтверждена или страница изменилась. Проверьте страницу перед следующим действием.' } }
  finally { pending.value = false; schedule(); if (keys.length && !pumping) void pumpKeys() }
}
async function load() {
  if (disposed || !token.value) return
  const reply = await action({ operation: 'list' })
  if (!reply) return
  loaded.value = true; sessions.value = reply.sessions || []
  if (!selected.value) { manual.value = false; controller = ''; frame.value = null; selectedID.value = sessions.value[0]?.browser_session_id || '' }
  if (selectedID.value) await refresh()
}
async function selectSession(id: string) { generation++; stop(); selectedID.value = id; manual.value = false; controller = ''; frame.value = null;pages.value=[];selectedPageID.value='';keys.length=0;text.value=''; await refresh() }
async function selectPage(id:string){if(pending.value)return;generation++;stop();selectedPageID.value=id;frame.value=null;keys.length=0;text.value='';address.value='';const reply=await refresh();address.value=reply?.frame?.url||''}
async function refresh() { if (!selectedID.value || pending.value) return; return await action({ operation: 'frame', browser_session_id: selectedID.value,...(selectedPageID.value?{page_id:selectedPageID.value}:{}) }) }
async function take() {
  if (!selected.value) return
  controller = [...crypto.getRandomValues(new Uint8Array(16))].map(value => value.toString(16).padStart(2, '0')).join('')
  epoch = selected.value.epoch
  const reply = await action({ operation: 'take', browser_session_id: selectedID.value, controller, epoch,...(selectedPageID.value?{page_id:selectedPageID.value}:{}) })
  if (reply) { epoch++; manual.value = true; selected.value.mode = 'owner'; selected.value.epoch = epoch; address.value = frame.value?.url || '' }
  else { controller = ''; manual.value = false; await load() }
}
async function release() {
  keys.length = 0
  const reply = await action({ operation: 'release', browser_session_id: selectedID.value, controller, epoch })
  if (reply) { manual.value = false; controller = ''; text.value = ''; await load() }
}
async function input(operation: string, extra: Record<string, unknown>) {
  if (!manual.value || !frame.value || pending.value) return
  await action({ operation, browser_session_id: selectedID.value, controller, epoch, frame_id: frame.value.frame_id, ...(frame.value.page_id?{page_id:frame.value.page_id,document_id:frame.value.document_id}:{}), ...extra })
}
function coordinates(event: MouseEvent | WheelEvent) {
  const rect = (event.currentTarget as HTMLImageElement).getBoundingClientRect()
  return { x: Math.max(0, Math.min((frame.value?.width || 1280)-1, (event.clientX - rect.left) * (frame.value?.width || 1280) / rect.width)), y: Math.max(0, Math.min((frame.value?.height || 720)-1, (event.clientY - rect.top) * (frame.value?.height || 720) / rect.height)) }
}
function pointer(event: MouseEvent) { (event.currentTarget as HTMLElement).parentElement?.focus(); void input('click', { ...coordinates(event), button: 'left' }) }
function wheel(event: WheelEvent) { void input('wheel', { ...coordinates(event), delta_x: Math.max(-2000, Math.min(2000, event.deltaX)), delta_y: Math.max(-2000, Math.min(2000, event.deltaY)) }) }
function keyboard(event: KeyboardEvent) {
  if (['Shift', 'Control', 'Alt', 'Meta'].includes(event.key) || event.repeat) return
  const key = [event.ctrlKey ? 'Control' : '', event.metaKey ? 'Meta' : '', event.altKey ? 'Alt' : '', event.shiftKey && event.key.length > 1 ? 'Shift' : '', event.key === ' ' ? 'Space' : event.key].filter(Boolean).join('+')
  if (!manual.value) return
  if (keys.length >= 64) { error.value = 'Подождите, пока браузер обработает нажатия.'; return }
  keys.push({key,page:frame.value?.page_id,document:frame.value?.document_id}); void pumpKeys()
}
async function pumpKeys() {
  if (pumping || pending.value) return
  pumping = true
  try { while (keys.length && manual.value && !disposed) { const next = keys.shift(); if(!next || next.page!==frame.value?.page_id || next.document!==frame.value?.document_id){keys.length=0;break} const reply = await action({ operation: 'key', browser_session_id: selectedID.value, controller, epoch, frame_id: frame.value?.frame_id, ...(next.page?{page_id:next.page,document_id:next.document}:{}), key:next.key }); if (!reply) { keys.length = 0; break } } }
  finally { pumping = false }
}
async function insertText() { const value = text.value; text.value = ''; await input('text', { text: value }) }
async function navigate() { await input('navigate', { url: address.value }) }
watch(token, () => { generation++; stop(); keys.length = 0; frame.value = null; sessions.value = []; selectedID.value = '';pages.value=[];selectedPageID.value=''; manual.value = false; controller = ''; text.value = ''; address.value = ''; loaded.value = false })
onBeforeUnmount(() => { disposed = true; generation++; stop(); keys.length = 0; token.value = ''; frame.value = null; text.value = ''; address.value = ''; controller = '' })

onMounted(() => { if (token.value) void load() })
function askForBrowser(){ mind.draft="Открой в серверном браузере "; mind.panel=""; nextTick(()=>document.getElementById("mind-message")?.focus()) }
</script>
