<template>
  <span :aria-label="text"
    ><span aria-hidden="true">{{ visible }}</span
    ><i
      v-if="live && !finished && animate"
      class="mind-caret"
      aria-hidden="true"
  /></span>
</template>
<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    text: string
    live?: boolean
    motion?: boolean
    paused?: boolean
  }>(),
  { live: false, motion: true, paused: false },
)
const visible = ref(props.live && props.motion ? '' : props.text),
  reduced = ref(false)
const animate = computed(() => props.motion && !reduced.value)
const finished = computed(() => visible.value === props.text)
let timer: ReturnType<typeof setTimeout> | undefined,
  disposed = false
function tick() {
  clearTimeout(timer)
  if (disposed || props.paused) return
  if (!animate.value || !props.live) {
    visible.value = props.text
    return
  }
  if (document.hidden) {
    timer = setTimeout(tick, 200)
    return
  }
  if (!props.text.startsWith(visible.value)) visible.value = ''
  if (visible.value.length < props.text.length) {
    visible.value = props.text.slice(0, visible.value.length + 2)
    timer = setTimeout(tick, 26)
  }
}
watch([() => props.text, () => props.live, animate, () => props.paused], tick)
onMounted(() => {
  const media = matchMedia('(prefers-reduced-motion: reduce)')
  const sync = () => {
    reduced.value = media.matches
    tick()
  }
  media.addEventListener('change', sync)
  onBeforeUnmount(() => media.removeEventListener('change', sync))
  sync()
})
onBeforeUnmount(() => {
  disposed = true
  clearTimeout(timer)
})
</script>
