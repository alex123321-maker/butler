<template>
  <div class="memory-graph-wrap">
    <div class="memory-graph-tools">
      <button
        class="mind-text-button"
        type="button"
        :aria-pressed="overview"
        @click="overview = !overview"
      >
        {{ overview ? 'Окружение записи' : 'Весь список' }}
      </button>
      <button class="mind-text-button" type="button" @click="positions = {}">
        Вписать
      </button>
    </div>
    <p class="mind-muted">
      Линии — сохранённые ссылки происхождения. Узлы можно перемещать.
    </p>
    <div class="memory-graph-canvas">
      <svg
        v-if="nodes.length"
        ref="svg"
        :viewBox="`0 0 720 ${height}`"
        :style="{ minHeight: `${height}px` }"
        aria-label="Граф происхождения записей"
        @pointermove="move"
        @pointerup="end"
        @pointercancel="cancel"
      >
        <g
          v-for="edge in edges"
          :key="`${edge.from}:${edge.to}`"
          class="memory-edge"
        >
          <path :d="edgePath(edge.from, edge.to)" />
          <text
            :x="(point(edge.from).x + point(edge.to).x) / 2 + 103"
            :y="Math.min(point(edge.from).y, point(edge.to).y) - 30"
            text-anchor="middle"
          >
            {{ edge.label }}
          </text>
        </g>
        <g
          v-for="node in nodes"
          :key="node.source_id"
          :transform="`translate(${point(node.source_id).x},${point(node.source_id).y})`"
          class="memory-node"
          :class="{ selected: node.source_id === selected }"
          tabindex="0"
          role="button"
          :aria-label="node.title || 'Без названия'"
          :aria-pressed="node.source_id === selected"
          @pointerdown="start($event, node.source_id)"
          @click="activate(node.source_id)"
          @keydown.enter.prevent="emit('select', node.source_id)"
          @keydown.space.prevent="emit('select', node.source_id)"
        >
          <title>{{ node.title }} · {{ memoryOrigin(node) }}</title>
          <rect width="206" height="84" rx="16" />
          <text x="16" y="30">
            {{ shorten(node.title || 'Без названия', 23) }}
          </text>
          <text x="16" y="57" class="memory-node-kind">
            {{ shorten(memoryOrigin(node), 27) }}
          </text>
        </g>
      </svg>
      <p v-else class="memory-empty">
        {{
          sources.length
            ? 'Выберите запись из списка или покажите весь список.'
            : 'Для этого запроса нет записей.'
        }}
      </p>
    </div>
    <p class="mind-muted">
      {{ nodes.length }} узлов из {{ sources.length }} загруженных записей.
      Связи с ещё не загруженными источниками доступны в карточке.
    </p>
  </div>
</template>
<script setup lang="ts">
import type { Source } from '../model/types'
import { memoryEdges } from '../model/memory-explorer'
import { memoryOrigin } from '../model/presentation'
const props = defineProps<{ sources: Source[]; selected: string }>()
const emit = defineEmits<{ select: [id: string] }>()
const overview = ref(false),
  svg = ref<SVGSVGElement | null>(null)
const positions = ref<Record<string, { x: number; y: number }>>({})
const allEdges = computed(() => memoryEdges(props.sources))
const nodes = computed(() => {
  if (overview.value || !props.selected) return props.sources
  const ids = new Set([props.selected])
  for (const edge of allEdges.value) {
    if (edge.from === props.selected) ids.add(edge.to)
    if (edge.to === props.selected) ids.add(edge.from)
  }
  return props.sources.filter((s) => ids.has(s.source_id))
})
const height = computed(() =>
  Math.max(360, Math.ceil(nodes.value.length / 3) * 150 + 60),
)
const edges = computed(() => memoryEdges(nodes.value))
const point = (id: string) =>
  positions.value[id] || {
    x: 16 + (nodes.value.findIndex((n) => n.source_id === id) % 3) * 240,
    y:
      60 +
      Math.floor(nodes.value.findIndex((n) => n.source_id === id) / 3) * 150,
  }
function edgePath(from: string, to: string) {
  const a = point(from),
    b = point(to),
    lane = Math.min(a.y, b.y) - 40
  return `M ${a.x + 103} ${a.y} C ${a.x + 103} ${lane}, ${b.x + 103} ${lane}, ${b.x + 103} ${b.y}`
}
const shorten = (text: string, length: number) =>
  Array.from(text).length > length
    ? Array.from(text)
        .slice(0, length - 1)
        .join('') + '…'
    : text
let drag: {
  id: string
  pointer: number
  x: number
  y: number
  origin: { x: number; y: number }
  moved: boolean
} | null = null
let dragged = false
function location(event: PointerEvent) {
  const matrix = svg.value?.getScreenCTM()
  return matrix
    ? new DOMPoint(event.clientX, event.clientY).matrixTransform(
        matrix.inverse(),
      )
    : new DOMPoint()
}
function start(event: PointerEvent, id: string) {
  if (event.button !== 0) return
  const p = location(event)
  dragged = false
  drag = {
    id,
    pointer: event.pointerId,
    x: p.x,
    y: p.y,
    origin: point(id),
    moved: false,
  }
  ;(event.currentTarget as SVGGElement).setPointerCapture(event.pointerId)
}
function move(event: PointerEvent) {
  if (!drag || event.pointerId !== drag.pointer) return
  const p = location(event),
    dx = p.x - drag.x,
    dy = p.y - drag.y
  drag.moved ||= Math.abs(dx) + Math.abs(dy) > 5
  if (drag.moved)
    positions.value[drag.id] = {
      x: Math.max(4, Math.min(510, drag.origin.x + dx)),
      y: Math.max(50, Math.min(height.value - 88, drag.origin.y + dy)),
    }
}
function end() {
  dragged = !!drag?.moved
  drag = null
}
function cancel() {
  drag = null
  dragged = false
}
function activate(id: string) {
  if (!dragged) emit('select', id)
  dragged = false
}
watch(
  () => props.sources,
  () => {
    positions.value = {}
    cancel()
  },
)
</script>
