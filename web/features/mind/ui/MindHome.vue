<template>
  <div class="mind-app" :data-motion="store.motion ? 'on' : 'off'">
    <div class="mind-toolbar">
      <span>{{ month }}</span
      ><small>{{ zone }}</small>
      <div>
        <button class="mind-text-button" type="button" @click="garden?.today()">
          Сегодня ↗</button
        ><button
          class="mind-icon-button"
          type="button"
          aria-label="Поиск"
          @click="focusMemory"
        >
          <MindIcon name="search" /></button
        ><button
          class="mind-icon-button"
          type="button"
          aria-label="Браузер"
          :disabled="!store.connected"
          @click="openAdvanced('browser')"
        >
          <MindIcon name="browser" /></button
        ><button
          class="mind-icon-button"
          type="button"
          aria-label="Настройки"
          @click="store.panel = 'settings'"
        >
          <MindIcon name="settings" />
        </button>
      </div>
    </div>
    <CalendarGarden
      ref="garden"
      :items="store.calendar"
      :motion="store.motion"
      @select="selectCalendar"
    />
    <main class="mind-home">
      <section class="mind-work" aria-label="Текущее поручение и разговор">
        <article>
          <div class="mind-work-top">
            <span
              class="mind-presence"
              :title="stateTitle"
              role="status"
              :aria-label="stateTitle"
              ><span
                v-if="store.working && streamConnected"
                class="mind-working"
                aria-hidden="true"
                ><i /><i /><i /></span
              ><MindIcon
                v-else
                :name="
                  store.selectedTask?.needs_user_action
                    ? 'lock'
                    : store.selectedTask?.task_status === 'paused'
                      ? 'pause'
                      : 'event'
                " /></span
            ><button
              class="mind-text-button"
              type="button"
              @click="store.panel = 'tasks'"
            >
              {{ store.tasks.length }} поручений</button
            ><button
              v-if="store.durable"
              class="mind-icon-button"
              type="button"
              :aria-label="
                store.durable.status === 'paused'
                  ? 'Продолжить поручение'
                  : 'Приостановить поручение'
              "
              :disabled="
                store.busy ||
                !!store.pendingControl ||
                ['completed', 'cancelled'].includes(store.durable.status)
              "
              @click="
                store.control(
                  store.durable.status === 'paused' ? 'resume' : 'pause',
                )
              "
            >
              <MindIcon
                :name="store.durable.status === 'paused' ? 'play' : 'pause'"
              />
            </button>
          </div>
          <h1>
            <button type="button" @click="store.panel = 'task'">
              {{
                store.selectedTask
                  ? taskTitle(store.selectedTask)
                  : store.connected
                    ? 'Пока нет поручений'
                    : 'Butler'
              }}
            </button>
          </h1>
          <MindRun
            v-if="store.runId && store.connected"
            :key="`${store.epoch}-${store.runId}`"
            :run-id="store.runId"
            :activity="store.activity"
            :tools="store.toolCalls"
            :motion="store.motion"
            @connection="streamConnected = $event"
            @events="runEvents = $event"
            @result="refreshLog"
            @ended="refreshLog"
          />
          <p v-else class="mind-muted">
            {{
              store.loadingTask
                ? 'Открываю поручение…'
                : store.connected
                  ? 'Можно просто написать.'
                  : 'Подключитесь в настройках.'
            }}
          </p>
          <button
            v-if="store.selectedTask"
            class="mind-text-button mind-journal-button"
            type="button"
            @click="store.panel = 'journal'"
          >
            Весь журнал
          </button>
        </article>
        <section class="mind-conversation" aria-label="Разговор с Батлером">
          <div
            ref="chat"
            class="mind-messages"
            role="log"
            aria-label="Сообщения"
          >
            <div
              v-for="message in store.conversation.slice(-6)"
              :key="message.message_id"
              :class="
                message.role === 'user'
                  ? 'mind-user-message'
                  : 'mind-assistant-message'
              "
            >
              <span class="sr-only">{{
                message.role === 'user' ? 'Вы' : 'Батлер'
              }}</span>
              <p>{{ message.content }}</p>
            </div>
            <p
              v-if="store.connected && !store.conversation.length"
              class="mind-chat-empty"
            >
              Что у вас на уме?
            </p>
          </div>
          <form class="mind-composer" @submit.prevent="store.send()">
            <button
              class="mind-icon-button"
              type="button"
              aria-label="Что можно поручить"
              @click="store.panel = 'tools'"
            >
              <MindIcon name="plus" /></button
            ><label class="sr-only" for="mind-message">Сообщение Батлеру</label
            ><textarea
              id="mind-message"
              v-model="store.draft"
              rows="1"
              maxlength="12000"
              placeholder="Напишите Батлеру…"
              :disabled="
                !store.connected || store.sending || !!store.pendingIngress
              "
              @keydown="messageKey"
            /><button
              class="mind-send"
              :disabled="
                !store.connected ||
                store.sending ||
                !store.draft.trim() ||
                !!store.pendingIngress
              "
              aria-label="Отправить сообщение"
            >
              <MindIcon name="arrow" />
            </button>
          </form>
          <span v-if="store.receipt" class="mind-input-state" role="status">{{
            receiptText
          }}</span
          ><button
            v-if="store.pendingIngress"
            class="mind-text-button"
            :disabled="store.sending"
            type="button"
            @click="store.send(true)"
          >
            Повторить тот же запрос
          </button>
        </section>
      </section>
      <p v-if="store.error" class="mind-error" role="alert">
        {{ store.error }}
        <button
          v-if="store.pendingControl"
          type="button"
          class="mind-text-button"
          :disabled="store.busy"
          @click="store.control('', true)"
        >
          Повторить действие
        </button>
      </p>
      <button
        v-if="store.approvals.some((a) => a.status === 'pending')"
        class="mind-permission"
        type="button"
        @click="store.panel = 'approvals'"
      >
        <i /><span>{{
          store.approvals.find((a) => a.status === 'pending')?.owner_facts
            ?.summary || 'Нужно ваше разрешение'
        }}</span
        ><span aria-hidden="true">↗</span>
      </button>
      <div class="mind-saved">
        <MindMemory />
        <section aria-label="Входящие">
          <div class="mind-section-head">
            <h2>
              <span class="sr-only">Входящие</span
              ><MindIcon
                name="tray"
                :class="{
                  'mind-attention':
                    !store.quiet &&
                    store.notices.some((n) => n.approvalId || n.taskId),
                }"
              /><span>{{ store.notices.length }}</span>
            </h2>
            <button
              class="mind-icon-button"
              :aria-pressed="store.quiet"
              type="button"
              aria-label="Тихо на этом экране"
              @click="store.quiet = !store.quiet"
            >
              <MindIcon name="moon" />
            </button>
          </div>
          <button
            v-for="notice in store.notices.slice(0, 6)"
            :key="notice.id"
            type="button"
            class="mind-notice"
            @click="openNotice(notice)"
          >
            <span
              ><span class="mind-notice-title">{{ notice.title }}</span
              ><small :class="notice.tone">{{
                notice.deliveryState
                  ? `Доставка: ${notice.deliveryState}`
                  : notice.source
              }}</small></span
            ><time>{{ dateLabel(notice.at) }}</time>
          </button>
          <p v-if="store.connected && !store.notices.length" class="mind-muted">
            Всё спокойно.
          </p>
          <button
            class="mind-text-button"
            type="button"
            @click="openAdvanced('activity')"
          >
            История
          </button>
        </section>
      </div>
      <footer class="mind-footer">
        <span
          :title="store.connected ? 'Butler подключён' : 'Нет подключения'"
          :class="{ connected: store.connected }"
          >butler</span
        ><button
          class="mind-text-button"
          type="button"
          @click="store.panel = 'settings'"
        >
          Настройки
        </button>
      </footer>
    </main>
    <MindDialog :open="!!store.panel" :title="panelTitle" @close="closePanel">
      <MindSettings v-if="store.panel === 'settings'" />
      <div v-else-if="store.panel === 'input' && store.receipt">
        <p class="mind-readable" data-testid="receipt-status">
          {{ store.receipt.status }}
        </p>
        <p class="mind-muted">
          {{ receiptText }}. Готовность поручения проверяется отдельно.
        </p>
        <p v-if="store.receipt.checkpoint" class="mind-muted">
          {{ store.receipt.checkpoint }}
        </p>
        <NuxtLink
          v-if="store.receipt.task_id"
          class="mind-button"
          :to="`/tasks/${encodeURIComponent(store.receipt.task_id)}`"
          >Открыть поручение</NuxtLink
        ><NuxtLink
          v-else-if="store.receipt.run_id"
          class="mind-button"
          :to="`/runs/${encodeURIComponent(store.receipt.run_id)}`"
          >Открыть попытку</NuxtLink
        >
      </div>
      <MindMemoryEditor
        v-else-if="store.panel === 'memory' || store.panel === 'note'"
      />
      <div v-else-if="store.panel === 'tasks'">
        <button
          v-for="task in store.tasks"
          :key="task.task_id"
          class="mind-panel-row"
          type="button"
          @click="store.selectTask(task.task_id, true)"
        >
          <span
            >{{ taskTitle(task)
            }}<small>{{ statusLabel(taskStatus(task)) }}</small></span
          ><span>↗</span>
        </button>
        <p v-if="!store.tasks.length" class="mind-muted">Поручений пока нет.</p>
      </div>
      <MindTaskDetail v-else-if="store.panel === 'task'" />
      <MindEventLog
        v-else-if="store.panel === 'journal'"
        :rows="journalRows"
        :motion="store.motion"
        :working="streamConnected && store.working"
      />
      <MindApprovals v-else-if="store.panel === 'approvals'" />
      <div v-else-if="store.panel === 'notice' && store.selectedNotice">
        <p class="mind-readable">{{ store.selectedNotice.body }}</p>
        <p v-if="store.selectedNotice.deliveryState" class="mind-muted">
          Доставка: {{ store.selectedNotice.deliveryState }}
        </p>
        <div class="mind-actions">
          <button
            v-if="store.selectedNotice.taskId"
            class="mind-button"
            type="button"
            @click="store.selectTask(store.selectedNotice!.taskId!, true)"
          >
            Открыть поручение</button
          ><button class="mind-button" type="button" @click="hideNotice">
            Убрать с экрана
          </button>
        </div>
      </div>
      <div v-else-if="store.panel === 'tools'">
        <button
          v-for="tool in quickTools"
          :key="tool.id"
          type="button"
          class="mind-panel-row"
          @click="toolAction(tool.id)"
        >
          {{ tool.title }}<span>↗</span>
        </button>
      </div>
      <MindHistory
        v-else-if="store.panel === 'history'"
        :kind="historyKind"
        :id="historyId"
        :key="`${store.epoch}-${historyKind}-${historyId}`"
      /><MindAdvanced
        v-else-if="store.panel === 'advanced'"
        :kind="store.advanced"
        :owner-token="store.token"
        :key="`${store.epoch}-${store.advanced}`"
      />
    </MindDialog>
  </div>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import type { RunEvent } from '~/composables/useRunEvents'
import { runLogRows } from '../model/run-log'
import type { CalendarItem, Notice, Panel } from '../model/types'
import {
  dateLabel,
  timeLabel,
  statusLabel,
  taskStatus,
  taskTitle,
} from '../model/presentation'
import CalendarGarden from './CalendarGarden.vue'
import MindIcon from './MindIcon.vue'
import MindRun from './MindRun.vue'
import MindEventLog from './MindEventLog.vue'
import MindDialog from './MindDialog.vue'
import MindMemory from './MindMemory.vue'
import MindMemoryEditor from './MindMemoryEditor.vue'
import MindSettings from './MindSettings.vue'
import MindTaskDetail from './MindTaskDetail.vue'
import MindApprovals from './MindApprovals.vue'
import MindAdvanced from './MindAdvanced.vue'
import MindHistory from './MindHistory.vue'
const historyKind = ref<'run' | 'session'>('run'),
  historyId = ref('')
const store = useMind(),
  route = useRoute(),
  garden = ref<InstanceType<typeof CalendarGarden> | null>(null),
  chat = ref<HTMLElement | null>(null),
  streamConnected = ref(false)
const zone = Intl.DateTimeFormat().resolvedOptions().timeZone
const runEvents = ref<RunEvent[]>([])
const journalRows = computed(() =>
  runLogRows(store.runId, store.activity, runEvents.value, store.toolCalls),
)
watch(
  () => [store.epoch, store.runId],
  () => {
    runEvents.value = []
  },
  { flush: 'sync' },
)
let logRefresh: ReturnType<typeof setTimeout> | undefined
function refreshLog() {
  clearTimeout(logRefresh)
  logRefresh = setTimeout(() => {
    void store.refreshTask()
  }, 100)
}
watch(
  () => [store.epoch, store.runId],
  () => clearTimeout(logRefresh),
  { flush: 'sync' },
)
onUnmounted(() => clearTimeout(logRefresh))
const month = new Intl.DateTimeFormat('ru-RU', {
  month: 'long',
  year: 'numeric',
}).format(new Date())
const stateTitle = computed(() =>
  !store.connected
    ? 'Нет подключения'
    : store.selectedTask
      ? statusLabel(taskStatus(store.selectedTask))
      : 'Нет активных поручений',
)
const panelTitle = computed(
  () =>
    (
      ({
        settings: 'Настройки',
        tasks: 'Поручения',
        task: store.selectedTask ? taskTitle(store.selectedTask) : 'Поручение',
        journal: 'Журнал',
        memory: store.inspection?.source.title || 'Память',
        note: 'Оставить заметку',
        approvals: 'Нужно ваше решение',
        notice: store.selectedNotice?.title || 'Входящие',
        tools: 'Поручить',
        input: 'Состояние запроса',
        history: historyKind.value === 'run' ? 'Попытка' : 'Разговор',
        advanced: advancedNames[store.advanced] || 'Butler',
      }) as Record<Panel, string>
    )[store.panel] || 'Butler',
)
const advancedNames: Record<string, string> = {
  settings: 'Провайдеры и подключения',
  goals: 'Цели и полномочия',
  initiatives: 'Расписания',
  skills: 'Навыки',
  extensions: 'Расширения',
  converters: 'Конвертеры',
  mobile: 'Телефон',
  memory: 'Хранилище и политики памяти',
  browser: 'Браузер',
  voice: 'Голос',
  doctor: 'Диагностика',
  system: 'Система',
  artifacts: 'Результаты',
  activity: 'История',
  sessions: 'Разговоры',
}
const quickTools = [
  { id: 'morning', title: 'Сводка дня' },
  { id: 'document', title: 'Разобраться в документе' },
  { id: 'reminder', title: 'Напомнить' },
  { id: 'browser', title: 'Открыть управление браузером' },
  { id: 'skills', title: 'Навыки' },
]
const receiptText = computed(() => {
  const status = store.receipt?.status
  return status === 'completed'
    ? 'Запрос обработан'
    : status === 'waiting_for_user'
      ? 'Нужно уточнение'
      : status === 'failed'
        ? 'Не удалось обработать запрос'
        : status === 'cancelled'
          ? 'Запрос остановлен'
          : status === 'expired'
            ? 'Срок запроса истёк'
            : 'Принято в работу'
})
function openAdvanced(kind: string) {
  store.advanced = kind
  store.panel = 'advanced'
}
function hideNotice() {
  if (store.selectedNotice) store.noticeHidden.push(store.selectedNotice.id)
  closePanel()
}
function closePanel() {
  if (store.panel === 'memory' || store.panel === 'note')
    store.clearInspection()
  store.panel = ''
}
function messageKey(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault()
    void store.send()
  }
}
function selectCalendar(item: CalendarItem) {
  if (item.taskId) void store.selectTask(item.taskId, true)
  else {
    store.advanced = 'initiatives'
    store.panel = 'advanced'
  }
}
function focusMemory() {
  document.getElementById('mind-memory-query')?.focus()
}
function openNotice(notice: Notice) {
  store.selectedNotice = notice
  store.panel = notice.approvalId ? 'approvals' : 'notice'
}
function toolAction(id: string) {
  if (id === 'skills' || id === 'browser') {
    store.advanced = id
    store.panel = 'advanced'
    return
  }
  store.draft =
    id === 'morning'
      ? 'Подготовь сводку на сегодня.'
      : id === 'document'
        ? 'Помоги разобраться в документе.'
        : 'Напомни мне '
  closePanel()
  nextTick(() => document.getElementById('mind-message')?.focus())
}
async function openRoute() {
  if (!store.connected) return
  const path = route.path
    .split('/')
    .filter(Boolean)
    .map((segment) => {
      try {
        return decodeURIComponent(segment)
      } catch {
        return segment
      }
    })
  if (path[0] === 'tasks' && path[1]) await store.selectTask(path[1], true)
  else if (path[0] === 'tasks') store.panel = 'tasks'
  else if (path[0] === 'sessions' && path[1]) {
    historyKind.value = 'session'
    historyId.value = path[1]
    store.panel = 'history'
  } else if (path[0] === 'memory' && path[1] === 'manage') {
    store.advanced = 'memory'
    store.panel = 'advanced'
  } else if (path[0] === 'settings' && path[1] !== 'providers') {
    store.panel = 'settings'
  } else if (path[0] === 'memory') {
    if (typeof route.query.source_id === 'string')
      await store.inspectMemory(route.query.source_id)
    else {
      store.panel = ''
      nextTick(focusMemory)
    }
  } else if (path[0] === 'approvals') store.panel = 'approvals'
  else if (path[0] && advancedNames[path[0]]) {
    store.advanced = path[0]
    store.panel = 'advanced'
  } else if (path[0] === 'runs' && path[1]) {
    historyKind.value = 'run'
    historyId.value = path[1]
    store.panel = 'history'
  } else if (path[0] === 'inputs' && path[1]) {
    try {
      await store.openReceipt(path[1])
      store.panel = 'input'
    } catch {
      store.error = 'Этот запрос пока недоступен.'
    }
  }
}
watch(
  () => route.fullPath,
  () => void openRoute(),
)
watch(
  () => store.connected,
  (value) => {
    if (value) void openRoute()
  },
)
watch(
  () => store.selectedId,
  () => {
    streamConnected.value = false
  },
)
watch(
  () => store.conversation.length,
  () =>
    nextTick(() => {
      if (chat.value) chat.value.scrollTop = chat.value.scrollHeight
    }),
)
watch(
  () => store.motion,
  (value) => {
    try {
      localStorage.setItem('butler-mind-motion', String(value))
    } catch {
      /* Optional visual preference only. */
    }
  },
)
onMounted(() => {
  try {
    store.motion = localStorage.getItem('butler-mind-motion') !== 'false'
  } catch {
    /* Default motion remains. */
  }
  if (!store.connected) store.panel = 'settings'
})
useHead({ title: 'Butler — дома', htmlAttrs: { lang: 'ru' } })
</script>
