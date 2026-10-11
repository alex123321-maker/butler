<template>
  <AppPanel title="Recorded execution">
    <p>Action evidence and notification delivery are separate from task completion.</p>
    <div class="execution-grid">
      <section>
        <h3>Model response</h3>
        <p v-if="final?.privacy_redacted">Response content was removed under the source retention policy.</p>
        <p v-else-if="final?.answer_state === 'partial'">A partial response was saved. Its output is incomplete.</p>
        <p v-else-if="final?.answer_state === 'final'">A final response was saved. Task verification remains separate.</p>
        <p v-else>Saved response status is unavailable.</p>
        <NuxtLink v-if="runId" :to="`/runs/${encodeURIComponent(runId)}`">View this attempt</NuxtLink>
      </section>
      <section>
        <h3>Action effects</h3>
        <p v-if="!final || final.privacy_redacted">Action evidence status is unavailable.</p>
        <p v-else-if="final.limitations?.includes('effect_evidence_unavailable')">Action evidence was unavailable when the response was saved.</p>
        <p v-else-if="!final?.effects?.length">No action evidence is recorded in this response.</p>
        <ul v-if="final?.effects?.length && !final.privacy_redacted">
          <li v-for="effect in final.effects" :key="effect.action_id">
            <AppBadge :tone="effectTone(effect.effect_state)">{{ effectLabel(effect.effect_state) }}</AppBadge>
            <details>
              <summary>Evidence references</summary>
              <p>{{ effect.action_id }}</p>
              <ul v-if="effect.evidence_refs.length"><li v-for="ref in effect.evidence_refs" :key="ref">{{ ref }}</li></ul>
              <p v-else>No evidence references were saved.</p>
            </details>
          </li>
        </ul>
        <p v-if="final?.effects?.length && !final.privacy_redacted">This snapshot records evidence at response time; later reconciliation is a separate check.</p>
      </section>
      <section>
        <h3>Notification delivery</h3>
        <p>{{ deliveryLabel(delivery?.state || '') }}</p>
      </section>
    </div>
  </AppPanel>
</template>

<script setup lang="ts">
import AppPanel from '~/shared/ui/AppPanel.vue'
import AppBadge from '~/shared/ui/AppBadge.vue'
import type { TaskFinalResult } from '~/entities/task/api'
import type { Tone } from '~/entities/task/presentation'

const props = defineProps<{
  result?: TaskFinalResult | { state: string } | null
  delivery?: { channel?: string; state?: string }
  runId?: string
}>()
const final = computed(() => props.result && 'result_id' in props.result ? props.result : null)
const effectLabels: Record<string, string> = {
  effect_confirmed: 'Effect confirmed',
  effect_absent: 'Effect confirmed absent',
  effect_unknown: 'Effect unknown; do not repeat automatically',
  prepared: 'Prepared; effect unverified',
  allowed: 'Authorized; effect unverified',
  executing: 'Dispatch started; effect unverified',
}
const effectLabel = (state: string): string => effectLabels[state] || 'Effect status unavailable'
const effectTone = (state: string): Tone => state === 'effect_confirmed' ? 'success' : state === 'effect_absent' ? 'info' : 'warning'
const deliveryLabels: Record<string, string> = {
  result_ready: 'Response is stored and available in Web. Reading it is not tracked.',
  sent: 'Channel accepted the notification. Reading it is not confirmed.',
  pending: 'Notification is waiting for delivery. The saved response remains available.',
  sending: 'Notification delivery is in progress.',
  failed: 'Notification failed. The saved response remains available.',
  delivery_unknown: 'Notification delivery is unknown. It will not be sent again automatically.',
}
const deliveryLabel = (state: string): string => deliveryLabels[state] || 'Notification delivery status is unavailable'
</script>

<style scoped>
.execution-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: var(--space-4); }
li { margin-block: var(--space-2); overflow-wrap: anywhere; }
</style>
