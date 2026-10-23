import { chromium } from 'playwright';
import assert from 'node:assert/strict';
const browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || 'msedge' });
try {
  const context = await browser.newContext();
  const root = 'http://127.0.0.1:5175';
  await context.request.post(root + '/api/login', { data: { login: 'coffee', password: 'demo12345' } });
  async function create() {
    const response = await context.request.post(root + '/api/applications', { headers: { 'Idempotency-Key': crypto.randomUUID() }, data: { inn: '0000100012', representative_role: 'director', purpose: 'purchase', product: 'working_capital', amount_kop: 250000000, term_months: 24, bki_consent: true } });
    assert.equal(response.status(), 201);
    return response.json();
  }
  for (let index = 0; index < 25; index++) await create();
  const offered = await create();
  await context.request.post(root + '/api/applications/' + offered.id + '/submit');
  const issued = await create();
  await context.request.post(root + '/api/applications/' + issued.id + '/submit');
  const approved = await (await context.request.get(root + '/api/applications/' + issued.id)).json();
  await context.request.post(root + '/api/applications/' + issued.id + '/accept', { data: { version: approved.version } });
  const code = await (await context.request.post(root + '/api/applications/' + issued.id + '/signing-code')).json();
  await context.request.post(root + '/api/applications/' + issued.id + '/sign', { data: { code: code.code } });
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.goto(root + '/client');
  await page.getByRole('heading', { name: 'Требует внимания', exact: true }).waitFor();
  assert.equal(await page.locator('.metric-drafts strong').textContent(), '25', 'Count uses all applications, not first page');
  assert.equal(await page.locator('.metric-offers strong').textContent(), '1');
  assert.equal(await page.locator('.metric-completed').count(), 0);
  assert.equal(await page.locator('.dashboard-application').count(), 3);
  assert.equal(await page.locator('.attention-card').count(), 1);
  assert.equal(await page.locator('.dashboard-application-grid .filters, .dashboard-application-grid button').count(), 0);
  assert.equal(await page.getByRole('heading', { name: /Подготовка к сезонным/ }).count(), 0);
  await page.locator('.metric-drafts').click();
  await page.waitForURL('**/client/applications?group=drafts');
  await page.getByText('Всего заявок: 25. Страница 1', { exact: true }).waitFor();
  assert.equal(await page.locator('.application-card').getByText('Черновик', { exact: true }).count(), 20);
  await page.getByRole('link', { name: 'Обзор', exact: true }).click();
  await page.locator('.attention-card a').click();
  await page.waitForURL('**/client/applications/' + offered.id);
  await page.getByRole('button', { name: 'Принять условия', exact: true }).click();
  await page.getByRole('button', { name: 'Получить новый код', exact: true }).waitFor();
  await page.getByRole('link', { name: 'Обзор', exact: true }).click();
  await page.getByRole('heading', { name: 'Осталось подписать документы', exact: true }).waitFor();
  await page.route('**/api/applications?*', route => route.fulfill({ status: 503, contentType: 'application/problem+json', body: '{}' }));
  await page.evaluate(() => window.dispatchEvent(new Event('application-refresh')));
  await page.getByRole('alert').filter({ hasText: 'Не удалось обновить обзор' }).waitFor();
  assert.equal(await page.locator('.metric-drafts strong').textContent(), '25', 'Failed refresh keeps last confirmed summary');
  for (const width of [390, 1366]) {
    await page.setViewportSize({ width, height: 900 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
  }
  assert.deepEqual(errors, []);
  console.log('Dashboard: accurate counts beyond first page, actionable offer/signing, three recent cards, grouped list, isolated error and mobile passed');
} finally { await browser.close(); }
