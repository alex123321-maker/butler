// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  ssr: false,
  compatibilityDate: '2025-01-01',
  devtools: { enabled: process.env.NUXT_DEVTOOLS_ENABLED === 'true' },

  modules: ['@nuxtjs/tailwindcss', '@pinia/nuxt'],

  alias: {
    '@app': '~/app',
    '@widgets': '~/widgets',
    '@features': '~/features',
    '@entities': '~/entities',
    '@shared': '~/shared',
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
    '~/assets/css/main.css'
  ],

  typescript: {
    strict: true,
  },
})
