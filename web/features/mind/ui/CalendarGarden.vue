<template>
  <section
    ref="root"
    class="calendar-garden"
    :data-motion="motion ? 'on' : 'off'"
    aria-label="Календарь поручений"
  >
    <div
      ref="host"
      class="atmosphere"
      :class="{ 'scene-motion-paused': !motion }"
      aria-hidden="true"
    />
    <div class="viewport">
      <div
        ref="stage"
        class="calendar-stage"
        :style="stageStyle"
        @keydown.left.prevent="select(active - 1)"
        @keydown.right.prevent="select(active + 1)"
        @keydown.home.prevent="select(45)"
        @wheel.passive="wheel"
        @pointerdown="pointerDown"
        @pointerup="pointerUp"
        @pointercancel="pointer = null"
      >
        <div
          class="day-track"
          :style="{ transform: `translate3d(${trackX}px,0,0)` }"
        >
          <article
            v-for="(day, index) in days"
            :key="day.key"
            :class="[
              'day-card',
              {
                'is-active': index === active,
                'is-outside': Math.abs(index - active) > sides,
              },
            ]"
            :role="index === active ? 'group' : 'button'"
            :tabindex="
              index === active ? -1 : Math.abs(index - active) <= sides ? 0 : -1
            "
            :inert="Math.abs(index - active) > sides"
            :aria-label="`${day.label}, ${day.items.length} дел`"
            :style="{
              '--side-h': `${Math.max(69, 130 - (Math.min(Math.abs(index - active), 4) - 1) * 13)}px`,
            }"
            @click="select(index)"
            @keydown.enter.prevent="select(index)"
            @keydown.space.prevent="select(index)"
          >
            <div class="compact-view" :aria-hidden="index === active">
              <div>
                <span class="compact-weekday">{{ day.shortWeekday }}</span
                ><span class="compact-number">{{ day.date.getDate() }}</span>
              </div>
              <div class="compact-bottom">
                <span class="compact-marks"
                  ><i
                    v-for="(item, dot) in day.items"
                    :key="item.id"
                    :style="{
                      '--dot-hue': Math.round(139 - Math.min(1, dot / 7) * 134),
                    }"
                /></span>
              </div>
            </div>
            <div class="expanded-view" :aria-hidden="index !== active">
              <div class="active-top">
                <div class="active-topline">
                  <span class="active-weekday">{{ day.weekday }}</span
                  ><span v-if="day.key === today" class="active-today"
                    ><i />Сегодня</span
                  ><button
                    v-else
                    class="return-button"
                    type="button"
                    :tabindex="index === active ? 0 : -1"
                    @click.stop="select(45)"
                  >
                    К сегодня ↗
                  </button>
                </div>
                <div class="active-date">
                  <span class="active-number">{{ day.date.getDate() }}</span>
                  <div class="active-month">
                    {{ day.month
                    }}<span class="active-year">{{
                      day.date.getFullYear()
                    }}</span>
                  </div>
                </div>
                <div
                  class="active-meter"
                  :class="{ 'is-today': day.key === today }"
                  role="group"
                  aria-label="Время задач"
                >
                  <div class="meter-track" />
                  <span
                    v-if="day.key === today"
                    class="meter-progress"
                    :style="{ width: `${(minute / 1440) * 100}%` }"
                  /><span
                    v-if="day.key === today"
                    class="meter-now"
                    :style="{ left: `${(minute / 1440) * 100}%` }"
                  /><button
                    v-for="(item, eventIndex) in day.items"
                    :key="item.id"
                    type="button"
                    class="meter-dot"
                    :style="{ left: `${(minutes(item.at) / 1440) * 100}%` }"
                    :tabindex="index === active ? 0 : -1"
                    :aria-label="`${timeLabel(item.at)} ${item.title}`"
                    @click.stop="focusItem(item, eventIndex)"
                  />
                </div>
              </div>
              <div class="schedule-section">
                <div class="schedule-spacer" />
                <div class="schedule-head">
                  <span>Расписание</span
                  ><span class="schedule-count">{{
                    countLabel(day.items.length)
                  }}</span>
                </div>
                <div
                  class="events-scroll"
                  :tabindex="index === active ? 0 : -1"
                  :aria-label="`Дела на ${day.label}`"
                >
                  <div
                    class="events-inner"
                    :class="{ 'has-events': day.items.length }"
                  >
                    <button
                      v-for="(item, eventIndex) in day.items"
                      :key="item.id"
                      type="button"
                      class="task"
                      :tabindex="index === active ? 0 : -1"
                      :data-task-state="item.status"
                      @click.stop="focusItem(item, eventIndex)"
                      @pointerenter="garden?.highlight(eventIndex)"
                    >
                      <time class="task-time">{{ timeLabel(item.at) }}</time
                      ><span class="task-content"
                        ><span class="task-title">{{ item.title }}</span
                        ><span class="task-description">{{
                          item.description
                        }}</span></span
                      >
                    </button>
                    <div v-if="!day.items.length" class="empty-tasks">
                      В этот день пока нет дел
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </article>
        </div>
      </div>
      <button
        class="edge-control edge-prev"
        type="button"
        aria-label="Предыдущий день"
        :disabled="active === 0"
        @click="select(active - 1)"
      >
        <MindIcon name="previous" /></button
      ><button
        class="edge-control edge-next"
        type="button"
        aria-label="Следующий день"
        :disabled="active === 90"
        @click="select(active + 1)"
      >
        <MindIcon name="next" />
      </button>
    </div>
    <span class="sr-only" aria-live="polite"
      >{{ days[active]?.label }} ·
      {{ countLabel(days[active]?.items.length || 0) }}</span
    >
  </section>
</template>
<script setup lang="ts">
import { ThoughtGarden } from '../garden/renderer.js'
import type { CalendarItem } from '../model/types'
import { dayKey, timeLabel } from '../model/presentation'
import MindIcon from './MindIcon.vue'
const props = defineProps<{ items: CalendarItem[]; motion: boolean }>()
const emit = defineEmits<{ select: [item: CalendarItem] }>()
const root = ref<HTMLElement | null>(null),
  stage = ref<HTMLElement | null>(null),
  host = ref<HTMLElement | null>(null)
const active = ref(45),
  width = ref(1440),
  now = ref(new Date()),
  today = ref(dayKey(new Date()))
let garden: ThoughtGarden | undefined,
  observer: ResizeObserver | undefined,
  clock: ReturnType<typeof setInterval> | undefined,
  pointer: { x: number; y: number } | null = null,
  lastWheel = 0,
  suppressUntil = 0
const motionQuery = import.meta.client
  ? matchMedia('(prefers-reduced-motion: reduce)')
  : null
const owner = { paused: !props.motion, motion: motionQuery! }
const sides = computed(() =>
  width.value >= 1200
    ? 3
    : width.value >= 1000
      ? 2
      : width.value >= 720
        ? 1
        : 0,
)
const gap = computed(() =>
  width.value >= 1220 ? 15 : width.value >= 900 ? 11 : 8,
)
const activeW = computed(() => Math.min(540, width.value * 0.755))
const sideW = computed(() =>
  sides.value
    ? (width.value -
        2 * (width.value >= 1200 ? 28 : width.value >= 1000 ? 24 : 16) -
        activeW.value -
        gap.value * sides.value * 2) /
      (sides.value * 2)
    : 76,
)
const activeH = computed(() =>
  Math.min(
    width.value <= 760 ? 515 : 548,
    (width.value <= 640 ? 590 : 620) -
      (width.value <= 760 ? 24 : 46) -
      (width.value <= 760 ? 130 : 148),
  ),
)
const stageStyle = computed(() => ({
  '--gap': `${gap.value}px`,
  '--active-w': `${activeW.value}px`,
  '--side-w': `${sideW.value}px`,
  '--active-h': `${activeH.value}px`,
  '--stage-bottom': `${width.value <= 760 ? 24 : 46}px`,
}))
const trackX = computed(
  () =>
    width.value / 2 -
    (active.value * (sideW.value + gap.value) + activeW.value / 2),
)
const minute = computed(
  () => now.value.getHours() * 60 + now.value.getMinutes(),
)
const days = computed(() =>
  Array.from({ length: 91 }, (_, index) => {
    const date = new Date(`${today.value}T12:00:00`)
    date.setDate(date.getDate() + index - 45)
    const key = dayKey(date)
    return {
      date,
      key,
      label: new Intl.DateTimeFormat('ru-RU', {
        day: 'numeric',
        month: 'long',
      }).format(date),
      weekday: new Intl.DateTimeFormat('ru-RU', { weekday: 'long' }).format(
        date,
      ),
      shortWeekday: new Intl.DateTimeFormat('ru-RU', {
        weekday: 'short',
      }).format(date),
      month: new Intl.DateTimeFormat('ru-RU', { month: 'long' }).format(date),
      items: props.items
        .filter((item) => dayKey(item.at) === key)
        .sort((a, b) => a.at.localeCompare(b.at)),
    }
  }),
)
const minutes = (at: string) => {
  const d = new Date(at)
  return d.getHours() * 60 + d.getMinutes()
}
const countLabel = (n: number) =>
  `${n} ${n % 10 === 1 && n % 100 !== 11 ? 'дело' : n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14) ? 'дела' : 'дел'}`
function select(index: number) {
  if (performance.now() < suppressUntil) return
  active.value = Math.max(0, Math.min(90, index))
}
function focusItem(item: CalendarItem, index: number) {
  garden?.highlight(index)
  emit('select', item)
}
function wheel(event: WheelEvent) {
  if (
    Math.abs(event.deltaX) > 17 &&
    Math.abs(event.deltaX) > Math.abs(event.deltaY) * 1.3 &&
    performance.now() - lastWheel > 440
  ) {
    lastWheel = performance.now()
    select(active.value + (event.deltaX > 0 ? 1 : -1))
  }
}
function pointerDown(event: PointerEvent) {
  if (
    event.button !== 0 ||
    (event.target as Element).closest('.events-scroll,.task')
  )
    return
  pointer = { x: event.clientX, y: event.clientY }
}
function pointerUp(event: PointerEvent) {
  if (!pointer) return
  const dx = event.clientX - pointer.x,
    dy = event.clientY - pointer.y
  pointer = null
  if (Math.abs(dx) > 65 && Math.abs(dx) > Math.abs(dy) * 1.45) {
    select(active.value + (dx < 0 ? 1 : -1))
    suppressUntil = performance.now() + 180
  }
}
async function paint() {
  await nextTick()
  if (!garden) return
  const day = days.value[active.value]
  garden.update(
    { base: '#f3f1e8' },
    {
      date: day.date,
      events: day.items.map((item) => ({
        title: item.title,
        time: timeLabel(item.at),
        status: item.status,
        done: item.done,
      })),
    },
  )
}
watch([active, () => props.items, width], paint, { deep: true })
watch(
  () => props.motion,
  (value) => {
    owner.paused = !value
    if (!value) garden?.finish()
    void paint()
  },
)
onMounted(() => {
  if (!root.value || !stage.value || !host.value) return
  const resize = () => {
    width.value = stage.value!.clientWidth
    void nextTick(() => {
      garden?.queueLayout()
    })
  }
  resize()
  garden = new ThoughtGarden(host.value, owner, stage.value)
  garden.setActive(true)
  observer = new ResizeObserver(resize)
  observer.observe(root.value)
  void paint()
  clock = setInterval(() => {
    now.value = new Date()
    today.value = dayKey(now.value)
  }, 30000)
})
onBeforeUnmount(() => {
  observer?.disconnect()
  clearInterval(clock)
  garden?.destroy()
})
defineExpose({
  today: () => {
    active.value = 45
  },
})
</script>
<style>
.calendar-garden {
  --ink: rgb(53, 67, 46);
  --soft-ink: rgb(111, 113, 110);
  --muted: rgb(117, 128, 104);
  --accent: rgb(101, 124, 120);
  --accent-rgb: 101, 124, 120;
  --surface: rgb(243, 241, 232);
  --line: rgba(55, 65, 63, 0.105);
  --ease: cubic-bezier(0.23, 1, 0.32, 1);
  position: relative;
  isolation: isolate;
  overflow: hidden;
  --side-w: 119px;
  --active-w: 540px;
  --active-h: 426px;
  --gap: 13px;
}
.calendar-garden .atmosphere {
  position: absolute;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
  isolation: isolate;
  background: var(--surface);
}
.calendar-garden .viewport {
  height: 620px;
  max-width: 1600px;
  margin-inline: auto;
}
.calendar-garden .day-card {
  border-color: transparent;
}
.calendar-garden .day-card.is-active {
  background: rgb(255, 254, 248);
  border-color: transparent;
}
.calendar-garden .task {
  width: 100%;
  background: transparent;
  border: 0;
  text-align: left;
  font: inherit;
  color: inherit;
}
.calendar-garden .task-content,
.calendar-garden .task-title,
.calendar-garden .task-description {
  display: block;
}
.calendar-garden .meter-dot {
  padding: 0;
  border: 0;
}
.calendar-garden .meter-track {
  position: absolute;
  inset: 50% 0 auto;
  height: 2px;
  background: rgb(226, 229, 216);
}
.calendar-garden .edge-control svg {
  width: 18px;
  height: 18px;
}
.calendar-garden[data-motion='off'] * {
  animation: none !important;
  transition: none !important;
}
@media (max-width: 640px) {
  .calendar-garden .viewport {
    height: 590px;
  }
}
</style>
