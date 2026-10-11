<template>
  <main class="page">
    <section class="page-header">
      <div><p class="page-kicker">Эксперимент</p><h2 class="page-title">Голос</h2>
        <p>Распознавание требует проверки имён, чисел, адресата и отрицаний. Голос не подтверждает личность.</p></div>
    </section>
    <section class="page-section">
      <label>Токен владельца <input v-model="token" type="password" autocomplete="off"></label>
      <button type="button" :disabled="!token || busy" @click="loadStatus">Проверить доступность</button>
      <p role="status">{{ enabled ? 'Локальный эксперимент доступен; проверка качества не пройдена.' : 'Голос выключен. Текстовое управление доступно.' }}</p>
      <p>Звук передаётся только вашему серверу Butler для локального распознавания. Просмотренный текст затем использует обычную политику модели. Не записывайте секреты.</p>
      <label>Роль записи <select v-model="role"><option value="owner_command">Команда владельца с просмотром</option><option value="note">Заметка</option><option value="external">Внешняя запись</option></select></label>
      <p data-testid="microphone-state">{{ mic === 'recording' ? '● Микрофон включён — идёт запись' : mic === 'starting' ? 'Запрашивается доступ к микрофону' : 'Микрофон выключен' }}</p>
      <button type="button" :disabled="!enabled || busy || mic !== 'off' || playing" @click="startCapture">Включить микрофон</button>
      <button type="button" :disabled="mic !== 'recording'" @click="stopCapture">Остановить запись</button>
      <label>Или PCM16 WAV, моно 16 кГц, до 120 секунд <input type="file" accept="audio/wav" :disabled="!enabled || busy || mic !== 'off' || playing || speechPending" @change="chooseFile"></label>
      <audio v-if="recordingUrl && mic === 'off'" :src="recordingUrl" controls @play="playing = true" @pause="playing = false" @ended="playing = false" />
      <label>Доступ к записи (часы) <input v-model.number="accessHours" type="number" min="1" max="168"></label>
      <p>После истечения срока голосовые операции недоступны. Каталог владельца сохраняет оригинал до явного удаления. Для физического удаления звука и производного текста нажмите «Удалить запись»; резервные копии обрабатывает журнал удаления Butler.</p>
      <button type="button" :disabled="!recording || busy || !enabled || mic !== 'off' || !token" @click="upload">Сохранить запись на Butler</button>
      <label>ID сохранённой записи <input v-model="savedClipId" type="text"></label>
      <button type="button" :disabled="!enabled || busy || !savedClipId || mic !== 'off'" @click="loadClip">Открыть сохранённую запись</button>
      <div v-if="clip">
        <p>Запись {{ clip.clip_id }}: {{ clip.state }} · версия {{ clip.revision }}</p>
        <button type="button" :disabled="busy || mic !== 'off'" @click="loadOriginal">Послушать сохранённый оригинал</button>
        <button type="button" :disabled="busy || !['stored', 'failed'].includes(clip.state)" @click="transcribe">Распознать локально</button>
        <button type="button" :disabled="busy || clip.state !== 'pending'" @click="cancelRecognition">Отменить распознавание</button>
        <button type="button" :disabled="busy" @click="remove">Удалить запись</button>
        <label>Текст для просмотра <textarea v-model="reviewText" rows="5" /></label>
        <p v-if="clip.state === 'partial'">Неполная или неопределённая запись: команда не принимается. Используйте текстовое управление.</p>
        <label><input v-model="checked" type="checkbox">Я проверил имена, числа, адресата и отрицания</label>
        <label>Сессия <input v-model="sessionKey" type="text"></label>
        <button type="button" :disabled="!canReview || clip.state !== 'review'" @click="review(false)">Подтвердить обычный ввод</button>
        <button type="button" :disabled="!canReview || !taskId || taskRevision < 1" @click="review(true)">Подтвердить Stop / ограничение задачи</button>
        <button type="button" :disabled="busy || !clip.reviewed_source || mic !== 'off'" @click="speak">Озвучить просмотренный текст</button>
      </div>
      <label>Задача (для управления обязательна) <input v-model="taskId" type="text"></label>
      <label>Текущая версия задачи <input v-model.number="taskRevision" type="number" min="1"></label>
      <button type="button" :disabled="!token || !taskId || busy" @click="stopTask">Stop задачи — текстовое управление</button>
      <button type="button" :disabled="!playing && !speechPending" @click="stopSpeech">Прервать озвучивание</button>
      <p v-if="error" role="alert">{{ error }}</p><p v-if="notice" role="status">{{ notice }}</p>
      <NuxtLink to="/tasks">Текстовое управление задачами</NuxtLink>
    </section>
  </main>
</template>

<script setup lang="ts">
import { VoiceCapture } from '../shared/voice/capture'

type SourceRef = { source_id: string; revision: number; access_revision: number; content_hash: string }
type Clip = { clip_id: string; role: string; state: string; revision: number; transcript: string; reviewed_source?: SourceRef }
const config = useRuntimeConfig()
const base = String(config.public.apiBase || '').replace(/\/$/, '')
const token = ref(''), enabled = ref(false), busy = ref(false), error = ref(''), notice = ref('')
const role = ref('note'), accessHours = ref(24), mic = ref<'off' | 'starting' | 'recording' | 'stopping'>('off')
const recording = ref<ArrayBuffer>(), recordingUrl = ref(''), clip = ref<Clip>(), reviewText = ref(''), checked = ref(false)
const savedClipId = ref('')
const sessionKey = ref(''), taskId = ref(''), taskRevision = ref(1), playing = ref(false), speechPending = ref(false)
let capture: VoiceCapture | undefined, polling: ReturnType<typeof setTimeout> | undefined, speech: HTMLAudioElement | undefined
let speechAbort: AbortController | undefined, speechUrl = '', disposed = false
const canReview = computed(() => !!clip.value && role.value === 'owner_command' && clip.value.role === 'owner_command' && ['review', 'control_pending'].includes(clip.value.state) && checked.value && !!reviewText.value.trim() && !!sessionKey.value && !busy.value)
const headers = (contentType = 'application/json') => ({ Authorization: `Bearer ${token.value}`, 'Content-Type': contentType, 'X-Butler-Delivery-ID': crypto.randomUUID() })
async function request<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await fetch(`${base}${path}`, { method, headers: headers(), body: body === undefined ? undefined : JSON.stringify(body), cache: 'no-store' })
  if (!response.ok) throw new Error('voice_request_failed')
  return await response.json() as T
}
async function run(action: () => Promise<void>) { busy.value = true; error.value = ''; try { await action() } catch { error.value = 'Операция недоступна. Проверьте текущую запись, права и бюджет; текст сохранён.' } finally { busy.value = false } }
async function loadStatus() { await run(async () => { enabled.value = (await request<{ enabled: boolean }>('/api/v3/voice/status')).enabled }) }
async function startCapture() {
  if (!enabled.value || busy.value || mic.value !== 'off' || playing.value || speechPending.value) return
  stopSpeech(); mic.value = 'starting'; error.value = ''; capture = new VoiceCapture()
  try { await capture.start(() => { void stopCapture() }); if (disposed) { await capture.cancel(); return }; mic.value = 'recording' } catch { mic.value = 'off'; error.value = 'Микрофон недоступен. Нужны разрешение браузера и HTTPS; используйте текст.' }
}
function setRecording(value: ArrayBuffer) { if (recordingUrl.value) URL.revokeObjectURL(recordingUrl.value); recording.value = value; recordingUrl.value = URL.createObjectURL(new Blob([value], { type: 'audio/wav' })); clip.value = undefined; reviewText.value = ''; checked.value = false }
async function stopCapture() { if (mic.value !== 'recording') return; mic.value = 'stopping'; try { if (capture) setRecording(await capture.stop()) } catch { error.value = 'Пустая запись не сохраняется.' } finally { mic.value = 'off'; capture = undefined } }
async function chooseFile(event: Event) { const file = (event.target as HTMLInputElement).files?.[0]; if (!file || busy.value || mic.value !== 'off' || playing.value || speechPending.value) return; if (file.size > 3840044) { error.value = 'Запись превышает 120 секунд PCM16.'; return }; setRecording(await file.arrayBuffer()); role.value = 'external' }
async function upload() { await run(async () => { const response = await fetch(`${base}/api/v3/voice/clips?role=${role.value}&access_hours=${accessHours.value}`, { method: 'POST', headers: headers('audio/wav'), body: recording.value }); if (!response.ok) throw new Error('upload_failed'); clip.value = await response.json() as Clip; savedClipId.value = clip.value.clip_id; reviewText.value = clip.value.transcript || '' }) }
async function loadClip() { await run(async () => { stopSpeech(); clearTimeout(polling); const current = await request<Clip>(`/api/v3/voice/clips/${encodeURIComponent(savedClipId.value)}`); clip.value = current; role.value = current.role; reviewText.value = current.transcript || ''; checked.value = false; if (current.state === 'pending') await refresh() }) }
async function loadOriginal() { await run(async () => { if (!clip.value || mic.value !== 'off') return; const response = await fetch(`${base}/api/v3/voice/clips/${encodeURIComponent(clip.value.clip_id)}/original`, { headers: headers('audio/wav'), cache: 'no-store' }); if (!response.ok) throw new Error('original_unavailable'); if (recordingUrl.value) URL.revokeObjectURL(recordingUrl.value); recordingUrl.value = URL.createObjectURL(await response.blob()) }) }
async function refresh() { if (!clip.value || disposed) return; const id = clip.value.clip_id; const current = await request<Clip>(`/api/v3/voice/clips/${id}`); if (disposed || clip.value?.clip_id !== id) return; clip.value = current; reviewText.value = current.transcript || ''; checked.value = false; if (current.state === 'pending') polling = setTimeout(() => { void refresh().catch(() => { error.value = 'Распознавание недоступно; оригинал остаётся сохранён.' }) }, 2000) }
async function transcribe() { await run(async () => { if (!clip.value) return; await request(`/api/v3/voice/clips/${clip.value.clip_id}/transcribe`, 'POST', {}); await refresh() }) }
async function cancelRecognition() { await run(async () => { if (!clip.value) return; clearTimeout(polling); await request(`/api/v3/voice/clips/${clip.value.clip_id}/cancel`, 'POST', { expected_revision: clip.value.revision }); notice.value = 'Распознавание отменено; это не Stop задачи.' }) }
async function remove() { await run(async () => { if (!clip.value) return; stopSpeech(); clearTimeout(polling); await request(`/api/v3/voice/clips/${clip.value.clip_id}`, 'DELETE'); clip.value = undefined; recording.value = undefined; reviewText.value = ''; if (recordingUrl.value) URL.revokeObjectURL(recordingUrl.value); recordingUrl.value = ''; notice.value = 'Звук и производный текст удалены из каталога Butler.' }) }
async function review(control: boolean) { await run(async () => { if (!clip.value) return; clip.value = await request<Clip>(`/api/v3/voice/clips/${clip.value.clip_id}/${control ? 'control' : 'review'}`, 'POST', { expected_revision: clip.value.revision, text: reviewText.value, checked_critical_details: checked.value, target: { session_key: sessionKey.value, task_id: taskId.value, expected_task_revision: taskId.value ? taskRevision.value : 0 } }); checked.value = false; notice.value = 'Просмотренный ввод принят обычным контроллером Butler.' }) }
async function stopTask() { await run(async () => { await request(`/api/v3/tasks/${encodeURIComponent(taskId.value)}/control`, 'POST', { kind: 'stop', expected_revision: taskRevision.value }); notice.value = 'Владельцем запрошен обычный Stop задачи.' }) }
function stopSpeech() { speechAbort?.abort(); speechAbort = undefined; speech?.pause(); speech = undefined; if (speechUrl) URL.revokeObjectURL(speechUrl); speechUrl = ''; playing.value = false; speechPending.value = false }
async function speak() {
  if (mic.value !== 'off' || !clip.value?.reviewed_source) return
  stopSpeech(); speechPending.value = true; speechAbort = new AbortController()
  try { const response = await fetch(`${base}/api/v3/voice/speech`, { method: 'POST', headers: headers(), body: JSON.stringify(clip.value.reviewed_source), signal: speechAbort.signal }); if (!response.ok) throw new Error('speech_failed'); const wav = await response.blob(); if (disposed || !speechPending.value) return; speechUrl = URL.createObjectURL(wav); speech = new Audio(speechUrl); speech.onended = stopSpeech; playing.value = true; speechPending.value = false; await speech.play() } catch { if (speechPending.value) error.value = 'Озвучивание недоступно; текст остаётся видимым.'; stopSpeech() }
}
onBeforeUnmount(() => { disposed = true; clearTimeout(polling); void capture?.cancel(); stopSpeech(); if (recordingUrl.value) URL.revokeObjectURL(recordingUrl.value); token.value = '' })
</script>
