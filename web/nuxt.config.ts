import { fileURLToPath } from 'node:url'

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  ssr: false,
  compatibilityDate: '2025-01-01',
  devtools: { enabled: false },

  modules: ['@nuxtjs/tailwindcss', '@pinia/nuxt'],

  components: [{ path: '~/features/mind/controls', pathPrefix: false }],

  alias: {
    '@app': fileURLToPath(new URL('./app', import.meta.url)),
    '@features': fileURLToPath(new URL('./features', import.meta.url)),
    '@entities': fileURLToPath(new URL('./entities', import.meta.url)),
    '@shared': fileURLToPath(new URL('./shared', import.meta.url)),
  },

  app: {
    head: {
      title: 'Butler',
      meta: [
        { charset: 'utf-8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' },
        { name: 'description', content: 'Butler — self-hosted AI agent platform' },
      ],
    },
  },

  runtimeConfig: {
    public: {
      apiBase: process.env.BUTLER_API_BASE_URL || 'http://localhost:8080',
    },
  },

  css: [
    '~/app/styles/tokens.css',
    '~/features/mind/garden/calendar.css',
    '~/assets/css/mind.css',
    '~/assets/css/memory-explorer.css'
  ],

  typescript: {
    strict: true,
  },
})
