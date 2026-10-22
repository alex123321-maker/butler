import { defineConfig, devices } from '@playwright/test'

const port = Number(process.env.BUTLER_WEB_TEST_PORT || 4173)
if (!Number.isInteger(port) || port < 1024 || port > 65535) throw new Error('Invalid Web test port')

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: true,
  retries: 0,
  reporter: 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
    screenshot: 'on',
    video: 'off',
  },
  webServer: {
    command: `npm run build && npx nuxi preview --port ${port}`,
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: true,
    timeout: 300000,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
})
