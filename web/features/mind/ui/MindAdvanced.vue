<template>
  <component :is="selected" v-if="selected" :owner-token="ownerToken" />
  <p v-else class="mind-muted">Этот раздел недоступен.</p>
</template>
<script setup lang="ts">
const props = defineProps<{ kind: string; ownerToken: string }>()
const panels = {
  memory: defineAsyncComponent(() => import('../advanced/memory.vue')),
  settings: defineAsyncComponent(() => import('../advanced/settings.vue')),
  goals: defineAsyncComponent(() => import('../advanced/goals.vue')),
  initiatives: defineAsyncComponent(
    () => import('../advanced/initiatives.vue'),
  ),
  skills: defineAsyncComponent(() => import('../advanced/skills.vue')),
  extensions: defineAsyncComponent(() => import('../advanced/extensions.vue')),
  converters: defineAsyncComponent(() => import('../advanced/converters.vue')),
  mobile: defineAsyncComponent(() => import('../advanced/mobile.vue')),
  voice: defineAsyncComponent(() => import('../advanced/voice.vue')),
  browser: defineAsyncComponent(() => import('../advanced/browser.vue')),
  doctor: defineAsyncComponent(() => import('../advanced/doctor.vue')),
  system: defineAsyncComponent(() => import('../advanced/system.vue')),
  artifacts: defineAsyncComponent(() => import('../advanced/artifacts.vue')),
  activity: defineAsyncComponent(() => import('../advanced/activity.vue')),
  sessions: defineAsyncComponent(() => import('../advanced/sessions.vue')),
}
const selected = computed(() => panels[props.kind as keyof typeof panels])
</script>
