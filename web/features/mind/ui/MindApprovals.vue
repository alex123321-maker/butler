<template>
  <div>
    <section
      v-for="approval in store.approvals.filter(
        (a) =>
          a.status === 'pending' ||
          (a.tool_name === 'single_tab.bind.guarded' &&
            a.status === 'approved'),
      )"
      :key="approval.approval_id"
      class="mind-approval"
    >
      <p class="mind-readable">
        {{
          approval.owner_facts?.summary ||
          approval.summary ||
          'Запрос разрешения'
        }}
      </p>
      <template v-if="approval.owner_facts"
        ><dl>
          <div>
            <dt>Действие</dt>
            <dd>{{ approval.tool_name }}</dd>
          </div>
          <div>
            <dt>Последствие</dt>
            <dd>{{ approval.owner_facts.effect }}</dd>
          </div>
          <div>
            <dt>Ресурс</dt>
            <dd>{{ approval.owner_facts.resource }}</dd>
          </div>
        </dl>
        <details>
          <summary>Версии и основание</summary>
          <p>
            Поручение {{ approval.owner_facts.task_revision }} · ограничения
            {{ approval.owner_facts.constraints_revision }}
          </p>
          <p v-if="approval.owner_facts.goal_id">
            Цель {{ approval.owner_facts.goal_id }} · версия
            {{ approval.owner_facts.goal_revision }} · полномочие
            {{ approval.owner_facts.mandate_id }} · версия
            {{ approval.owner_facts.mandate_revision }}
          </p>
          <p v-if="approval.owner_facts.credential_reference_count">
            Ссылок на реквизиты:
            {{ approval.owner_facts.credential_reference_count }}. Значения
            скрыты.
          </p>
          <p class="mind-hash">{{ approval.owner_facts.contract_hash }}</p>
          <p class="mind-hash">{{ approval.owner_facts.args_hash }}</p>
        </details></template
      >
      <p v-else class="mind-muted">
        Описание последствий недоступно. Разрешение не отправляется.
      </p>
      <GuardedTabChoice
        v-if="approval.tool_name === 'single_tab.bind.guarded'"
        :approval-id="approval.approval_id"
        :owner-token="store.token"
        @changed="store.refresh()"
      />
      <div v-else class="mind-actions">
        <button
          class="mind-button"
          :disabled="store.busy"
          @click="store.decideApproval(approval.approval_id, 'reject')"
        >
          Отклонить</button
        ><button
          class="mind-button primary"
          :disabled="
            store.busy || !approval.owner_facts || expired(approval.expires_at)
          "
          @click="store.decideApproval(approval.approval_id, 'approve')"
        >
          Разрешить
        </button>
      </div>
    </section>
    <p
      v-if="
        !store.approvals.some(
          (a) =>
            a.status === 'pending' ||
            (a.tool_name === 'single_tab.bind.guarded' &&
              a.status === 'approved'),
        )
      "
      class="mind-muted"
    >
      Ожидающих запросов нет.
    </p>
    <p v-if="store.error" role="alert" class="mind-error">{{ store.error }}</p>
  </div>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import GuardedTabChoice from '../controls/GuardedTabChoice.vue'
const store = useMind()
const expired = (at?: string) =>
  !!at && (!Number.isFinite(Date.parse(at)) || Date.parse(at) < Date.now())
</script>
