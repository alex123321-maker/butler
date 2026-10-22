<template>
  <p v-if="store.loadingTask" class="mind-muted">Открываю поручение…</p>
  <div v-else-if="store.detail" class="mind-task-detail">
    <div class="mind-task-state">
      <span>{{
        store.selectedTask
          ? statusLabel(taskStatus(store.selectedTask))
          : store.detail.task.status
      }}</span
      ><button
        class="mind-icon-button"
        type="button"
        aria-label="Обновить поручение"
        @click="store.selectTask(store.selectedId)"
      >
        <MindIcon name="refresh" />
      </button>
    </div>
    <p v-if="store.detail.durable_task?.goal" class="mind-readable">
      {{ store.detail.durable_task.goal }}
    </p>
    <dl class="mind-results">
      <div>
        <dt>Ответ</dt>
        <dd>
          {{
            redacted
              ? store.detail.final_result?.privacy_redacted
                ? 'Удалён'
                : 'Недоступен'
              : answerState
          }}
        </dd>
      </div>
      <div>
        <dt>Проверка</dt>
        <dd>
          {{
            store.detail.result?.verified === true ||
            store.selectedTask?.result_verified === true
              ? 'Подтверждена'
              : 'Не подтверждена'
          }}
        </dd>
      </div>
      <div>
        <dt>Доставка</dt>
        <dd>{{ deliveryState }}</dd>
      </div>
    </dl>
    <p
      v-if="!redacted && store.detail.result?.has_result"
      class="mind-readable"
    >
      {{ store.detail.result.outcome_summary }}
    </p>
    <p v-if="store.detail.task.error_summary" class="mind-error">
      {{ store.detail.task.error_summary }}
    </p>
    <section v-if="!redacted && store.detail.final_result?.effects?.length">
      <h3>Внешние действия</h3>
      <div
        v-for="effect in store.detail.final_result.effects"
        :key="effect.action_id"
      >
        <p>{{ effectLabels[effect.effect_state] || 'Эффект неизвестен' }}</p>
        <details>
          <summary>Сохранённые доказательства</summary>
          <p v-if="!effect.evidence_refs?.length" class="mind-muted">
            Ссылки на доказательства не сохранены.
          </p>
          <p
            v-for="reference in effect.evidence_refs || []"
            :key="reference"
            class="mind-hash"
          >
            {{ reference }}
          </p>
        </details>
      </div>
      <small class="mind-muted"
        >Состояние на момент сохранения ответа. Последующая проверка учитывается
        отдельно.</small
      >
    </section>
    <p v-if="redacted" class="mind-muted">
      Содержимое ответа и доказательства недоступны.
    </p>
    <template v-else
      ><p
        v-for="limitation in store.detail.final_result?.limitations || []"
        :key="limitation"
        class="mind-muted"
      >
        {{ limitation }}
      </p></template
    >
    <details v-if="store.detail.durable_task?.completion_criteria?.length">
      <summary>Критерии готовности</summary>
      <ul>
        <li
          v-for="criterion in store.detail.durable_task.completion_criteria"
          :key="criterion"
        >
          {{ criterion }}
        </li>
      </ul>
    </details>
    <details v-if="store.detail.durable_task?.constraints?.length">
      <summary>Договорённости</summary>
      <ul>
        <li
          v-for="constraint in store.detail.durable_task.constraints"
          :key="constraint"
        >
          {{ constraint }}
        </li>
      </ul>
    </details>
    <details>
      <summary>Управление и доказательства</summary>
      <TaskOwnerControls
        :key="`${store.epoch}-${store.selectedId}`"
        :task-id="store.selectedId"
        :initial-owner-token="store.token"
        @changed="store.refreshTask()"
      />
    </details>
    <details v-if="store.detail.run_ids?.length">
      <summary>Попытки</summary>
      <NuxtLink
        v-for="run in store.detail.run_ids"
        :key="run"
        class="mind-panel-row"
        :to="`/runs/${encodeURIComponent(run)}`"
        >{{ run }}</NuxtLink
      >
    </details>
  </div>
  <p v-else class="mind-muted">Поручение недоступно.</p>
</template>
<script setup lang="ts">
import { useMind } from '../model/store'
import { statusLabel, taskStatus } from '../model/presentation'
import TaskOwnerControls from '../controls/TaskOwnerControls.vue'
import MindIcon from './MindIcon.vue'
const store = useMind()
const redacted = computed(
  () =>
    store.detail?.final_result?.privacy_redacted === true ||
    (!!store.detail?.final_result?.state &&
      !store.detail?.final_result?.answer_state),
)
const answerState = computed(
  () =>
    (
      ({
        final: 'Сохранён',
        partial: 'Частичный',
        unavailable: 'Недоступен',
        none: 'Пока нет',
      }) as Record<string, string>
    )[store.detail?.final_result?.answer_state || ''] ||
    (store.detail?.result?.has_result ? 'Сохранён' : 'Пока нет'),
)
const deliveryState = computed(
  () =>
    (
      ({
        result_ready: 'Сохранено в Butler',
        sent: 'Канал принял',
        delivered: 'Доставлено',
        failed: 'Ошибка доставки',
        delivery_failed: 'Ошибка доставки',
        delivery_unknown: 'Неизвестна',
        pending: 'Ожидает отправки',
        not_sent: 'Не отправлено',
      }) as Record<string, string>
    )[store.detail?.delivery?.state || ''] || 'Нет подтверждения',
)
const effectLabels: Record<string, string> = {
  effect_confirmed: 'Эффект подтверждён',
  effect_absent: 'Подтверждено отсутствие эффекта',
  effect_unknown: 'Эффект неизвестен — автоматический повтор недопустим',
  prepared: 'Подготовлено, эффект не подтверждён',
  allowed: 'Разрешено, эффект не подтверждён',
  committed: 'Зафиксировано, внешний эффект требует проверки',
}
</script>
