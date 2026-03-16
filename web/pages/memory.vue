<template>
  <div class="memory-page">
    <section class="memory-hero">
      <div>
        <p class="eyebrow">Memory Browser</p>
        <h2 class="page-title">Memory</h2>
        <p class="hero-copy">Browse durable working, profile, episodic, and chunk memory by scope without exposing raw secrets.</p>
      </div>
      <div class="memory-controls">
        <label>
          Scope type
          <select v-model="scopeType">
            <option value="session">session</option>
            <option value="user">user</option>
            <option value="global">global</option>
          </select>
        </label>
        <label>
          Scope id
          <input v-model="scopeID" type="text" placeholder="telegram:chat:123 or user id" />
        </label>
        <button type="button" class="refresh-btn" :disabled="pending" @click="refresh">Refresh</button>
      </div>
    </section>

    <p v-if="!hasScope" class="placeholder-text">Enter a scope id to browse memory.</p>
    <p v-else-if="pending" class="placeholder-text">Loading memory…</p>
    <p v-else-if="error" class="placeholder-text">Failed to load memory.</p>

    <div v-else class="memory-grid">
      <section class="memory-card">
        <h3>Working Memory</h3>
        <div v-if="data?.working" class="memory-fields">
          <MemoryField label="Goal" :value="data.working.goal" />
          <MemoryField label="Status" :value="data.working.status" />
          <MemoryField label="Source" :value="`${data.working.source_type}:${data.working.source_id}`" />
          <MemoryField label="Provenance" :value="data.working.provenance" block />
          <MemoryField label="Entities" :value="data.working.entities_json" block />
          <MemoryField label="Pending steps" :value="data.working.pending_steps_json" block />
        </div>
        <p v-else class="placeholder-text">No working memory for this scope.</p>
      </section>

      <section class="memory-card">
        <h3>Profile Memory</h3>
        <div v-if="data?.profile?.length" class="memory-list">
          <article v-for="item in data.profile" :key="item.id" class="memory-item">
            <header>
              <strong>{{ item.key }}</strong>
              <span>{{ item.status }}</span>
            </header>
            <p>{{ item.summary }}</p>
            <MemoryField label="Value" :value="item.value_json" block />
            <MemoryField label="Provenance" :value="item.provenance" block />
            <MemoryField label="Links" :value="formatLinks(item.links)" block />
          </article>
        </div>
        <p v-else class="placeholder-text">No profile memory entries.</p>
      </section>

      <section class="memory-card">
        <h3>Episodic Memory</h3>
        <div v-if="data?.episodic?.length" class="memory-list">
          <article v-for="item in data.episodic" :key="item.id" class="memory-item">
            <header>
              <strong>{{ item.summary }}</strong>
              <span>{{ item.status }}</span>
            </header>
            <p>{{ item.content }}</p>
            <MemoryField label="Tags" :value="item.tags_json" block />
            <MemoryField label="Provenance" :value="item.provenance" block />
            <MemoryField label="Links" :value="formatLinks(item.links)" block />
          </article>
        </div>
        <p v-else class="placeholder-text">No episodic memory entries.</p>
      </section>

      <section class="memory-card">
        <h3>Chunk Memory</h3>
        <div v-if="data?.chunks?.length" class="memory-list">
          <article v-for="item in data.chunks" :key="item.id" class="memory-item">
            <header>
              <strong>{{ item.title }}</strong>
              <span>{{ item.status }}</span>
            </header>
            <p>{{ item.summary }}</p>
            <MemoryField label="Content" :value="item.content" block />
            <MemoryField label="Tags" :value="item.tags_json" block />
            <MemoryField label="Provenance" :value="item.provenance" block />
            <MemoryField label="Links" :value="formatLinks(item.links)" block />
          </article>
        </div>
        <p v-else class="placeholder-text">No chunk memory entries.</p>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMemoryScope, type MemoryLinkRecord } from '~/composables/useApi'

useHead({ title: 'Memory — Butler' })

const scopeType = ref('session')
const scopeID = ref('')
const { data, pending, error, refresh } = useMemoryScope(scopeType, scopeID)
const hasScope = computed(() => scopeID.value.trim().length > 0)

function formatLinks(links: MemoryLinkRecord[]) {
  if (!links || links.length === 0) return '[]'
  return JSON.stringify(links, null, 2)
}
</script>

<style scoped>
.memory-page { display: grid; gap: 24px; }
.memory-hero {
  display: flex; justify-content: space-between; gap: 20px; padding: 24px; border-radius: 24px;
  background: linear-gradient(145deg, rgba(14, 18, 30, 0.94), rgba(8, 12, 20, 0.98));
  border: 1px solid rgba(255,255,255,0.08);
}
.eyebrow { margin: 0 0 8px; text-transform: uppercase; letter-spacing: 0.24em; font-size: 12px; color: rgba(255,255,255,0.5); }
.hero-copy { max-width: 680px; color: rgba(255,255,255,0.72); }
.memory-controls { display: grid; gap: 12px; min-width: 280px; }
.memory-controls label { display: grid; gap: 6px; font-size: 13px; color: rgba(255,255,255,0.72); }
.memory-controls input, .memory-controls select {
  border: 1px solid rgba(255,255,255,0.12); border-radius: 12px; background: rgba(255,255,255,0.03);
  color: #fff; padding: 12px 14px;
}
.refresh-btn { border: 0; border-radius: 999px; padding: 12px 18px; background: #f97316; color: #fff; cursor: pointer; }
.memory-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(320px, 1fr)); gap: 18px; }
.memory-card { display: grid; gap: 14px; padding: 20px; border-radius: 20px; background: rgba(10, 16, 28, 0.88); border: 1px solid rgba(255,255,255,0.08); }
.memory-list { display: grid; gap: 12px; }
.memory-item { display: grid; gap: 8px; padding: 14px; border-radius: 14px; background: rgba(255,255,255,0.04); }
.memory-item header { display: flex; justify-content: space-between; gap: 12px; }
.placeholder-text { color: rgba(255,255,255,0.64); }
@media (max-width: 860px) { .memory-hero { flex-direction: column; } }
</style>
