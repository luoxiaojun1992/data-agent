import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  timeout: 60_000,
  reporter: [
    ['allure-playwright'],
    ['list'],
    ['json', { outputFile: 'artifacts/playwright-report/results.json' }],
  ],
  outputDir: 'artifacts/test-results',
  use: {
    baseURL: process.env.UI_BASE_URL || 'http://localhost:3000',
    trace: 'retain-on-failure',
    screenshot: { mode: 'on', fullPage: true },
  },
  projects: [
    {
      name: 'chromium',
      use: {
        // 本地可经 PW_CHANNEL=chrome 复用系统 Chrome，避免下载 Playwright 自带浏览器；
        // CI 不设该变量，仍使用 Playwright 内置 chromium（行为不变）。
        ...(process.env.PW_CHANNEL ? { channel: process.env.PW_CHANNEL } : {}),
        // 本地环境常被注入 HTTP_PROXY/HTTPS_PROXY（WorkBuddy/clash），浏览器 API 请求
        // 会被代理劫持导致「服务离线」+ 登录失败。PW_NO_PROXY=1 时强制直连。
        ...(process.env.PW_NO_PROXY
          ? { launchOptions: { args: ['--no-proxy-server'] } }
          : {}),
      },
    },
  ],
  webServer: undefined,
});
