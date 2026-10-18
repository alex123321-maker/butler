<template>
  <main class="mobile-page">
    <h1>Android devices</h1>
    <p>Connect your phone to collect available notifications with your permission. Observations stay private; automatic model processing is disabled.</p>
    <label>Owner control token <input v-model="token" type="password" autocomplete="off" /></label>
    <button :disabled="busy || !token" @click="load">Load devices</button>
    <p role="status">{{ error }}</p>
    <section v-if="loaded">
      <h2>Pair a phone</h2>
      <p>Request a code on the phone and enter it here, or create a one-use code here. Codes expire in five minutes. Confirm only your own phone.</p>
      <input v-model="phoneCode" aria-label="Phone verification code" />
      <button :disabled="busy" @click="approve">Confirm phone code</button>
      <button :disabled="busy" @click="createCode">Create pairing code</button>
      <output v-if="code">{{ code }}</output>
      <h2>Devices and delivery</h2>
      <article v-for="d in devices" :key="d.device_id">
        <h3>{{ d.device_name }} · {{ d.status }}</h3>
        <p>Android API {{ d.android_sdk }} · {{ d.app_version }} · Last contact {{ d.last_seen_at || 'No sync yet' }}</p>
        <p>Accepted {{ d.accepted_count }}, duplicates {{ d.duplicate_count }}, rejected {{ d.rejected_count }}</p>
        <label>Retention days <input v-model.number="d.retention_days" type="number" min="1" max="365" /></label>
        <button :disabled="busy || d.status === 'REVOKED'" @click="update(d, d.status)">Save retention</button>
        <button :disabled="busy || d.status === 'REVOKED'" @click="update(d, d.status === 'PAUSED' ? 'ACTIVE' : 'PAUSED')">{{ d.status === 'PAUSED' ? 'Resume' : 'Pause' }} delivery</button>
        <button :disabled="busy || d.status === 'REVOKED'" @click="update(d, 'REVOKED')">Revoke device</button>
      </article>
      <h2>Recent observations</h2>
      <label>Device <select v-model="selectedDevice"><option value="">All devices</option><option v-for="d in devices" :key="d.device_id" :value="d.device_id">{{ d.device_name }}</option></select></label>
      <label>Search <input v-model="query" /></label>
      <label><input v-model="reveal" type="checkbox" /> Reveal sensitive notification content</label>
      <button :disabled="busy" @click="search">Search observations</button>
      <p>Up to 100 recent matches. App filters are configured on the phone. Deletion removes the source and its derivatives; delivery tombstones prevent replay.</p>
      <article v-for="e in events" :key="e.source_id">
        <strong>{{ e.package_name }} · {{ e.change_type }} · {{ e.status }}</strong>
        <p>Observed {{ e.observed_at }} · received {{ e.received_at }}</p>
        <pre v-if="reveal && e.body">{{ e.body }}</pre>
        <button :disabled="busy || e.status === 'deleted' || e.status === 'revoked'" @click="remove(e)">Delete source and derivatives</button>
      </article>
    </section>
  </main>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
interface Device { device_id: string; device_name: string; status: string; android_sdk: number; app_version: string; last_seen_at: string | null; accepted_count: number; duplicate_count: number; rejected_count: number; retention_days: number }
interface Observation { source_id: string; revision: number; package_name: string; change_type: string; status: string; observed_at: string; received_at: string; body?: string }
const config = useRuntimeConfig()
const token = ref(''), phoneCode = ref(''), code = ref(''), query = ref(''), selectedDevice = ref(''), error = ref('')
const reveal = ref(false), busy = ref(false), loaded = ref(false)
const devices = ref<Device[]>([]), events = ref<Observation[]>([])
let epoch = 0
let controller = new AbortController()
function clear() { epoch++; controller.abort(); controller = new AbortController(); devices.value = []; events.value = []; code.value = ''; phoneCode.value = ''; query.value = ''; selectedDevice.value = ''; reveal.value = false; loaded.value = false; busy.value = false; error.value = '' }
watch(token, clear, { flush: 'sync' })
watch(reveal, () => { events.value = [] })
onBeforeUnmount(clear)
async function request<T>(path: string, body?: Record<string, unknown>): Promise<T> {
  return await $fetch<T>(`${config.public.apiBase}/api/v1/mobile-owner/${path}`, { method: body ? 'POST' : 'GET', headers: { Authorization: `Bearer ${token.value}` }, body, signal: controller.signal })
}
async function action(work: (current: () => boolean) => Promise<void>) {
  const stamp = epoch
  busy.value = true; error.value = ''
  try { await work(() => stamp === epoch) } catch { if (stamp === epoch) error.value = 'Request failed. Check owner proof, device status and server connectivity.' } finally { if (stamp === epoch) busy.value = false }
}
function load() { return action(async current => { const v = await request<Device[]>('devices'); if (current()) { devices.value = v; loaded.value = true } }) }
function createCode() { return action(async current => { const v = await request<{ pairing_code: string }>('pairing-code', {}); if (current()) code.value = v.pairing_code }) }
function approve() { return action(async current => { await request('approve', { code: phoneCode.value }); if (current()) { phoneCode.value = ''; error.value = 'Phone approved. Complete pairing on the phone.' } }) }
function update(d: Device, status: string) { return action(async current => { await request('device', { device_id: d.device_id, status, retention_days: d.retention_days }); if (current()) d.status = status }) }
function search() { return action(async current => { const disclosed = reveal.value; const v = await request<Observation[]>(`events?${new URLSearchParams({ device_id: selectedDevice.value, q: query.value, reveal: String(disclosed) })}`); if (current() && disclosed === reveal.value) events.value = v }) }
function remove(e: Observation) { return action(async current => { await request('delete', { source_id: e.source_id, revision: e.revision }); if (current()) { e.body = ''; e.status = 'deleted' } }) }
</script>

<style scoped>
.mobile-page { max-width: 1000px; margin: auto; padding: 24px; }
section, article { margin: 20px 0; }
article { border: 1px solid var(--color-border-default); padding: 16px; border-radius: 12px; }
label { display: inline-flex; gap: 8px; margin: 8px; align-items: center; }
input, select, button { padding: 8px; border: 1px solid var(--color-border-strong); border-radius: 6px; color: inherit; background: transparent; }
button { margin: 6px; cursor: pointer; }
button:disabled { opacity: 0.5; }
pre, output { display: block; white-space: pre-wrap; overflow-wrap: anywhere; margin: 12px 0; }
</style>
