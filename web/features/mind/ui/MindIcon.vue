<template>
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="paths[name] || paths.document" />
  </svg>
</template>
<script setup lang="ts">
defineProps<{ name: string }>()
const paths: Record<string, string> = {
  browser: 'M3 5h18v15H3V5Zm0 4h18M6 7h.01M9 7h.01m1 7 2 2 4-4',
  search: 'M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0Zm-2 5 6 6',
  settings:
    'M3 7h18M3 17h18M11 7a3 3 0 1 1-6 0 3 3 0 0 1 6 0Zm8 10a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z',
  stack: 'm4 7 8-4 8 4-8 4-8-4Zm0 5 8 4 8-4m-16 5 8 4 8-4',
  tray: 'M4 4h16v16H4V4Zm0 10h5l2 3h2l2-3h5',
  message: 'M4 5h16v12H9l-5 4V5Zm4 4h8m-8 4h5',
  document: 'M7 3h7l4 4v14H7V3Zm7 0v5h4m-8 4h5m-5 4h5',
  event: 'm13 3-9 10h7l-1 8L21 9h-8l1-6',
  moon: 'M19 15.5A8 8 0 0 1 8.5 5 8 8 0 1 0 19 15.5Z',
  plus: 'M12 5v14M5 12h14',
  close: 'm6 6 12 12M6 18 18 6',
  pause: 'M8 5v14M16 5v14',
  play: 'm8 5 11 7-11 7V5Z',
  arrow: 'M12 19V5m-5 5 5-5 5 5',
  next: 'm9 5 7 7-7 7',
  previous: 'm15 5-7 7 7 7',
  check: 'm5 12 4 4L19 6',
  lock: 'M5 10h14v11H5V10Zm3 0V7a4 4 0 0 1 8 0v3m-4 5v2',
  refresh: 'M20 8a8 8 0 1 0 0 8M20 3v5h-5',
  link: 'm9 15 6-6m-5-3 2-2a4 4 0 0 1 6 6l-2 2m-2 6-2 2a4 4 0 0 1-6-6l2-2',
  more: 'M5 12h.01M12 12h.01M19 12h.01',
}
</script>
