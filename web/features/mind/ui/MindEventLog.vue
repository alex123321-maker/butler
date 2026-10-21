<template>
  <div
    class="mind-log"
    :aria-label="compact ? 'Последние действия' : 'Все действия'"
  >
    <TransitionGroup name="mind-entry" tag="div">
      <details
        v-for="item in visible"
        :key="item.id"
        :data-event-type="item.type"
        :class="{ 'mind-log-failed': item.failed }"
      >
        <summary
          :title="item.label"
          :aria-label="
            [timeLabel(item.at), item.label, item.text]
              .filter(Boolean)
              .join(' · ')
          "
        >
          <time :datetime="item.at">{{ timeLabel(item.at) }}</time>
          <MindIcon
            :name="item.type"
            :class="{ 'mind-tool-pending': item.pending && working }"
          />
          <MindTypewriter
            v-if="item.text"
            :text="item.text"
            :live="item.pending && working"
            :motion="motion"
          />
          <small v-if="item.duration" class="mind-log-duration">{{
            item.duration
          }}</small>
          <MindIcon
            v-if="item.sections.length"
            name="next"
            class="mind-log-disclosure"
          />
        </summary>
        <div v-if="item.sections.length" class="mind-log-body">
          <div
            v-for="(section, index) in item.sections"
            :key="index"
            class="mind-log-section"
          >
            <span :title="section.label" class="mind-log-section-icon"
              ><MindIcon :name="section.icon" /><span class="sr-only">{{
                section.label
              }}</span></span
            >
            <pre :aria-label="section.label">{{ section.text }}</pre>
          </div>
        </div>
      </details>
    </TransitionGroup>
    <p v-if="!visible.length" class="mind-muted">
      {{ empty || 'Журнал пока пуст.' }}
    </p>
  </div>
</template>
<script setup lang="ts">
import type { LogRow } from '../model/run-log'
import { timeLabel } from '../model/presentation'
import MindIcon from './MindIcon.vue'
import MindTypewriter from './MindTypewriter.vue'
const props = defineProps<{
  rows: LogRow[]
  motion: boolean
  compact?: boolean
  working?: boolean
  empty?: string
}>()
const visible = computed(() =>
  props.compact ? props.rows.slice(-3) : props.rows,
)
</script>
