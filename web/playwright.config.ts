import { defineConfig, devices } from '@playwright/test';

// гоняем против поднятого compose, своего сервера не запускаем
export default defineConfig({
  testDir: 'e2e',
  timeout: 90_000,
  // процесс заявки идёт через temporal и моки, решение приходит за секунды, но не мгновенно
  expect: { timeout: 30_000 },
  fullyParallel: true,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8000',
    locale: 'ru-RU',
    trace: 'retain-on-failure',
  },
  projects: [
    { name: 'setup', testMatch: /auth\.setup\.ts/ },
    { name: 'chromium', use: devices['Desktop Chrome'], dependencies: ['setup'] },
  ],
});
