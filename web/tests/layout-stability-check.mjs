import { chromium } from 'playwright';
import assert from 'node:assert/strict';
const browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || 'msedge' });
try {
  const context = await browser.newContext();
  const page = await context.newPage();
  const root = 'http://127.0.0.1:5175';
  await page.setViewportSize({ width: 1366, height: 900 });
  await page.goto(root);
  await page.getByLabel('Логин', { exact: true }).fill('coffee');
  await page.getByLabel('Пароль', { exact: true }).fill('demo12345');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await page.getByRole('heading', { name: 'Обзор', exact: true }).waitFor();
  const response = await context.request.post(root + '/api/applications', { headers: { 'Idempotency-Key': crypto.randomUUID() }, data: { inn: '0000100012', product: 'working_capital', purpose: 'purchase', representative_role: 'director', amount_kop: 250000000, term_months: 24, bki_consent: true } });
  const app = await response.json();
  let release;
  let hold = true;
  const gate = new Promise(resolve => { release = resolve; });
  await page.route('**/api/applications?*', async route => {
    if (hold) await gate;
    await route.continue();
  });
  await page.goto(root + '/client/applications');
  await page.locator('.initial-loading').waitFor();
  assert.equal(await page.getByText(/Всего заявок:/).count(), 0, 'No fake zero total before data');
  const toolbar = await page.locator('.application-toolbar').boundingBox();
  hold = false; release();
  await page.getByText('Всего заявок: 1. Страница 1', { exact: true }).waitFor();
  const readyToolbar = await page.locator('.application-toolbar').boundingBox();
  assert.ok(Math.abs(toolbar.height - readyToolbar.height) < 0.5);
  const cardBefore = await page.locator('.application-card').boundingBox();
  const refreshed = page.waitForResponse(response => response.url().includes('/api/applications?') && response.status() === 200);
  await page.evaluate(() => window.dispatchEvent(new Event('application-refresh')));
  await refreshed;
  await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  assert.equal(await page.locator('.initial-loading').count(), 0, 'Background refresh does not insert loading content');
  const cardAfter = await page.locator('.application-card').boundingBox();
  assert.ok(Math.abs(cardBefore.y - cardAfter.y) < 0.5);
  await page.evaluate(() => {
    window.layoutSamples = [];
    window.keepSampling = true;
    function sample() {
      const panel = document.querySelector('.sidebar');
      if (panel) {
        const box = panel.getBoundingClientRect();
        window.layoutSamples.push({ x: box.x, width: box.width });
      }
      if (window.keepSampling) requestAnimationFrame(sample);
    }
    sample();
  });
  for (let index = 0; index < 2; index++) {
    await page.getByRole('link', { name: 'Обзор', exact: true }).click();
    await page.getByRole('heading', { name: 'Обзор', exact: true }).waitFor();
    await page.getByRole('link', { name: 'Мои заявки', exact: true }).click();
    await page.getByText('Всего заявок: 1. Страница 1', { exact: true }).waitFor();
    await page.locator('.application-card a').click();
    await page.getByRole('heading', { name: `Заявка № ${app.number}`, exact: true }).waitFor();
    await page.getByRole('navigation', {name:'Разделы кабинета'}).getByRole('link', { name: 'Новая заявка', exact: true }).click();
    await page.getByLabel('ИНН компании', { exact: true }).waitFor();
  }
  const samples = await page.evaluate(() => { window.keepSampling = false; return window.layoutSamples; });
  assert.ok(samples.length > 3);
  for (const key of ['x', 'width']) assert.ok(Math.max(...samples.map(value => value[key])) - Math.min(...samples.map(value => value[key])) < 0.5, `Sidebar ${key} remains stable across routes`);
  for (const width of [1366, 390]) {
    await page.setViewportSize({ width, height: 900 });
    const tops = [];
    for (const [label, surface] of [['Обзор', '.dashboard-promo'], ['Мои заявки', '.workspace-content'], ['Новая заявка', '.workspace-content']]) {
      await page.getByRole('navigation', { name: 'Разделы кабинета' }).getByRole('link', { name: label, exact: true }).click();
      await page.getByRole('heading', { name: label, exact: true }).waitFor();
      await page.evaluate(() => window.scrollTo(0, 0));
      tops.push((await page.locator(surface).boundingBox()).y);
    }
    assert.ok(Math.max(...tops) - Math.min(...tops) < 0.5, `Page surfaces share the same top at ${width}px: ${tops}`);
  }
  console.log('Layout: no fake empty state, stable toolbar, silent refresh and sidebar coordinates across routes passed');
} finally { await browser.close(); }
