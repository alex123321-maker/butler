<template>
  <form class="mind-form mind-connection" @submit.prevent="connect">
    <label
      >Доступ владельца<input
        v-model="store.token"
        type="password"
        autocomplete="off"
        aria-label="Доступ владельца" /></label
    ><button
      class="mind-button primary"
      :disabled="!store.token.trim() || store.connecting"
    >
      {{
        store.connecting
          ? 'Подключаюсь…'
          : store.connected
            ? 'Обновить подключение'
            : 'Подключить'
      }}
    </button>
    <p v-if="store.connectionError" role="alert">{{ store.connectionError }}</p>
    <small>Доступ хранится только до перезагрузки страницы.</small>
  </form>
  <template v-if="store.connected"
    ><label class="mind-setting-row"
      ><span>Плавное движение</span
      ><input
        v-model="store.motion"
        type="checkbox"
        role="switch"
        aria-label="Плавное движение" /></label
    ><label class="mind-setting-row"
      ><span>Тихо на этом экране</span
      ><input
        v-model="store.quiet"
        type="checkbox"
        role="switch"
        aria-label="Тихо на этом экране"
    /></label>
    <p v-if="store.quiet" class="mind-muted">
      Без пульсации входящих. Карточки остаются здесь; доставка в каналы
      меняется отдельно.
    </p>
    <details class="mind-settings-advanced">
      <summary>Модель и настройки Butler</summary>
      <p v-if="settingsError" role="alert">{{ settingsError }}</p>
      <p v-if="pending" class="mind-muted">Загружаю настройки…</p>
      <section v-for="group in settings" :key="group.name">
        <h3>{{ group.name }}</h3>
        <label
          v-for="item in group.settings"
          :key="item.key"
          class="mind-setting-field"
          ><span>{{ item.description || item.key }}</span
          ><select
            v-if="item.allowed_values?.length"
            v-model="values[item.key]"
            :aria-label="item.key"
          >
            <option
              v-for="value in item.allowed_values"
              :key="value"
              :value="value"
            >
              {{ value }}
            </option></select
          ><input
            v-else
            v-model="values[item.key]"
            :type="item.is_secret ? 'password' : 'text'"
            :placeholder="item.is_secret ? 'Заменить значение…' : ''"
            :aria-label="item.key"
            autocomplete="off"
          /><span class="mind-setting-foot"
            ><small
              >{{ item.source
              }}{{ item.requires_restart ? ' · после перезапуска' : '' }}</small
            ><button
              class="mind-text-button"
              type="button"
              :disabled="saving === item.key || !canSave(item)"
              @click="save(item)"
            >
              Сохранить
            </button></span
          ></label
        >
      </section>
    </details>
    <details class="mind-settings-advanced">
      <summary>Ещё возможности</summary>
      <button
        v-for="entry in advanced"
        :key="entry.id"
        class="mind-panel-row"
        type="button"
        @click="openAdvanced(entry.id)"
      >
        {{ entry.title }}<span aria-hidden="true">↗</span>
      </button>
    </details>
  </template>
</template>
<script setup lang="ts">
import type { SettingItem, SettingsComponent } from '~/composables/useSettings'
import { useMind } from '../model/store'
const store = useMind(),
  settings = ref<SettingsComponent[]>([]),
  values = ref<Record<string, string>>({}),
  settingsError = ref(''),
  saving = ref(''),
  pending = ref(false)
const advanced = [
  { id: 'settings', title: 'Провайдеры, авторизация и системный промпт' },
  { id: 'goals', title: 'Цели и полномочия' },
  { id: 'initiatives', title: 'Расписания и инициативы' },
  { id: 'skills', title: 'Навыки' },
  { id: 'extensions', title: 'Расширения' },
  { id: 'converters', title: 'Конвертеры' },
  { id: 'memory', title: 'Хранилище и политики памяти' },
  { id: 'mobile', title: 'Телефон' },
  { id: 'browser', title: 'Браузер' },
  { id: 'voice', title: 'Голос' },
  { id: 'doctor', title: 'Диагностика' },
  { id: 'system', title: 'Состояние системы' },
  { id: 'artifacts', title: 'Сохранённые результаты' },
  { id: 'activity', title: 'История действий' },
  { id: 'sessions', title: 'Разговоры' },
]
async function connect() {
  await store.connect()
  if (store.connected) await load()
}
async function load() {
  const epoch = store.epoch
  pending.value = true
  try {
    const data = await store.request<{ components: SettingsComponent[] }>(
      '/api/v1/settings',
    )
    if (store.epoch !== epoch) return
    settings.value = data.components || []
    values.value = Object.fromEntries(
      settings.value.flatMap((g) =>
        g.settings.map((s) => [s.key, s.is_secret ? '' : s.value]),
      ),
    )
  } catch {
    if (store.epoch === epoch)
      settingsError.value = 'Настройки пока недоступны.'
  } finally {
    if (store.epoch === epoch) pending.value = false
  }
}
function canSave(item: SettingItem) {
  return item.is_secret
    ? !!values.value[item.key]
    : values.value[item.key] !== item.value
}
async function save(item: SettingItem) {
  const epoch = store.epoch
  saving.value = item.key
  settingsError.value = ''
  try {
    await store.request(
      `/api/v1/settings/${encodeURIComponent(item.key)}`,
      { value: values.value[item.key] },
      { method: 'PUT' },
    )
    if (epoch !== store.epoch) return
    await load()
  } catch {
    if (epoch === store.epoch)
      settingsError.value = 'Изменение не подтверждено. Обновите настройки.'
  } finally {
    if (epoch === store.epoch) saving.value = ''
  }
}
function openAdvanced(id: string) {
  store.advanced = id
  store.panel = 'advanced'
}
watch(
  () => store.epoch,
  () => {
    settings.value = []
    values.value = {}
    settingsError.value = ''
    saving.value = ''
    pending.value = false
  },
)
onMounted(() => {
  if (store.connected) void load()
})
</script>
