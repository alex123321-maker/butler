<template>
  <figure class="mind-publication-media">
    <img v-if="image && url" :src="url" :alt="alt || file.name" />
    <a v-else-if="url" class="mind-text-button" :href="url" :download="file.name">Скачать {{ file.name }}</a>
    <p v-else class="mind-muted" role="status">{{ error ? 'Вложение недоступно или изменилось.' : 'Проверяю вложение…' }}</p>
    <figcaption v-if="caption || alt">{{ caption || alt }}</figcaption>
  </figure>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import type { PublicationFile } from '../model/communications'
const props = defineProps<{ publication: string; file: PublicationFile; alt?: string; caption?: string; image?: boolean }>()
const store = useMind(), url = ref(''), error = ref(false)
let version = 0
function clear() { version++; if (url.value) URL.revokeObjectURL(url.value); url.value = '' }
watch([
  () => props.publication, () => props.file.artifact_ref, () => props.file.sha256,
  () => props.file.size, () => props.file.mime, () => props.file.name, () => store.epoch,
], async () => {
  clear(); error.value = false
  const current = version, stamp = store.epoch
  try {
    if (props.image && !['image/png', 'image/jpeg'].includes(props.file.mime)) throw new Error('invalid_image')
    const response = await store.privateRead(`/api/v3/communications/${encodeURIComponent(props.publication)}/media?artifact_ref=${encodeURIComponent(props.file.artifact_ref)}`)
    if (!response.ok) throw new Error('media_unavailable')
    const data = await response.arrayBuffer()
    const digest = [...new Uint8Array(await crypto.subtle.digest('SHA-256', data))].map(b => b.toString(16).padStart(2, '0')).join('')
    if (current !== version || stamp !== store.epoch) return
    if (data.byteLength !== props.file.size || data.byteLength > 10 * 1024 * 1024 || digest !== props.file.sha256) throw new Error('media_changed')
    url.value = URL.createObjectURL(new Blob([data], { type: props.image ? props.file.mime : 'application/octet-stream' }))
  } catch { if (current === version && stamp === store.epoch) error.value = true }
}, { immediate: true })
onBeforeUnmount(clear)
</script>
<style scoped>
.mind-publication-media { margin: 12px 0; }
.mind-publication-media img { display: block; max-width: 100%; height: auto; }
.mind-publication-media figcaption { white-space: pre-wrap; overflow-wrap: anywhere; }
</style>
