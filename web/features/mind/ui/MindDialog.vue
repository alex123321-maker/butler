<template>
  <dialog
    ref="dialog"
    class="mind-dialog"
    :aria-labelledby="id"
    @cancel.prevent="emit('close')"
    @click="outside"
  >
    <header>
      <h2 :id="id">{{ title }}</h2>
      <button
        class="mind-icon-button"
        type="button"
        aria-label="Закрыть"
        @click="emit('close')"
      >
        <MindIcon name="close" />
      </button>
    </header>
    <slot />
  </dialog>
</template>
<script setup lang="ts">
import MindIcon from './MindIcon.vue'
const props = defineProps<{ open: boolean; title: string }>()
const emit = defineEmits<{ close: [] }>()
const dialog = ref<HTMLDialogElement | null>(null)
const id = useId()
watch(
  [() => props.open, dialog],
  ([open]) => {
    if (!dialog.value) return
    if (open && !dialog.value.open) dialog.value.showModal()
    else if (!open && dialog.value.open) dialog.value.close()
  },
  { flush: 'post' },
)
function outside(event: MouseEvent) {
  if (event.target !== dialog.value || !dialog.value) return
  const r = dialog.value.getBoundingClientRect()
  if (
    event.clientX < r.left ||
    event.clientX > r.right ||
    event.clientY < r.top ||
    event.clientY > r.bottom
  )
    emit('close')
}
onBeforeUnmount(() => dialog.value?.close())
</script>
