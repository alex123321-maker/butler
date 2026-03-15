// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',
  devtools: { enabled: true },

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

  css: ['~/assets/css/main.css'],

  typescript: {
    strict: true,
  },
})
