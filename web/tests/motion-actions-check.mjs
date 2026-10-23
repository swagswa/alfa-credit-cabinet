import { chromium } from 'playwright';
import assert from 'node:assert/strict';

const browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || 'msedge' });
try {
  const context = await browser.newContext();
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  const root = 'http://127.0.0.1:5175';
  await page.goto(root);
  await page.getByLabel('Логин', { exact: true }).fill('coffee');
  await page.getByLabel('Пароль', { exact: true }).fill('demo12345');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await page.getByRole('heading', { name: 'Обзор', exact: true }).waitFor();
  const response = await context.request.post(root + '/api/applications', {
    headers: { 'Idempotency-Key': crypto.randomUUID() },
    data: { inn: '0000100012', representative_role: 'director', product: 'working_capital', purpose: 'purchase', amount_kop: 250000000, term_months: 24, bki_consent: true },
  });
  assert.equal(response.status(), 201);
  const app = await response.json();
  await page.goto(root + '/client/applications/' + app.id);
  const summary = page.getByText('Изменить данные заявки', { exact: true });
  await summary.click();
  await page.getByLabel('Сумма, ₽', { exact: true }).fill('2700000');
  // Reverse a disclosure mid-transition. The editor and its input must survive.
  await summary.click();
  await summary.click();
  assert.equal(await page.getByLabel('Сумма, ₽', { exact: true }).inputValue(), '2700000');
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  let intercepted;
  const requested = new Promise(resolve => { intercepted = resolve; });
  await page.route('**/api/applications/' + app.id, async route => {
    if (route.request().method() !== 'PATCH') return route.continue();
    intercepted();
    await gate;
    await route.fulfill({ status: 503, contentType: 'application/problem+json', body: JSON.stringify({ title: 'Сохранение временно недоступно.' }) });
  });
  await page.getByRole('button', { name: 'Сохранить изменения', exact: true }).click();
  await requested;
  assert.equal(await page.locator('.notice-success').count(), 0);
  assert.equal(await page.getByLabel('Сумма, ₽', { exact: true }).isDisabled(), true);
  release();
  await page.getByRole('status').filter({ hasText: 'Сохранение временно недоступно.' }).waitFor();
  assert.equal(await page.locator('.notice-success').count(), 0);
  assert.equal(await page.getByLabel('Сумма, ₽', { exact: true }).inputValue(), '2700000');
  await page.unroute('**/api/applications/' + app.id);
  await page.getByRole('button', { name: 'Сохранить изменения', exact: true }).click();
  await page.locator('.notice-success').getByText('Изменения сохранены.', { exact: true }).waitFor();
  const stored = await (await context.request.get(root + '/api/applications/' + app.id)).json();
  assert.equal(stored.amount_kop, 270000000);
  await page.locator('.notice-success').getByRole('button', { name: 'Закрыть уведомление' }).click();
  await page.locator('.notice-success').waitFor({ state: 'detached' });
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await summary.focus();
  await page.keyboard.press('Enter');
  await page.waitForFunction(() => !document.querySelector('.edit-panel').open);
  await page.keyboard.press('Enter');
  await page.getByLabel('Сумма, ₽', { exact: true }).waitFor();
  await page.waitForFunction(() => document.getAnimations().every(animation => animation.playState !== 'running'));
  for (const width of [390, 1366]) {
    await page.setViewportSize({ width, height: 900 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
  }
  assert.deepEqual(errors, []);
  console.log('Motion actions: interrupted disclosure, pending/error preserve input, success after server, toast exit, keyboard and reduced motion passed');
} finally {
  await browser.close();
}
