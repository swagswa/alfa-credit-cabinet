import { chromium } from 'playwright';
import assert from 'node:assert/strict';

const browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || 'msedge' });
try {
  const context = await browser.newContext();
  const page = await context.newPage();
  const root = 'http://127.0.0.1:5175';
  await page.goto(root);
  await page.getByLabel('Логин', { exact: true }).fill('coffee');
  await page.getByLabel('Пароль', { exact: true }).fill('demo12345');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await page.getByRole('heading', {name:'Начните с первой заявки',exact:true}).waitFor();
  const response = await context.request.post(root + '/api/applications', { headers: { 'Idempotency-Key': crypto.randomUUID() }, data: { inn: '0000100012', product: 'working_capital', purpose: 'purchase', representative_role: 'director', amount_kop: 250000000, term_months: 24, bki_consent: true } });
  const app = await response.json();
  await page.getByRole('heading', {name:'Актуальные заявки',exact:true}).waitFor();
  await page.getByRole('heading', { name: 'Изменение заявки № 1', exact: true }).waitFor();
  // A slow server must not blank previously visited sections during navigation.
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  const delayed = async route => { await gate; await route.continue(); };
  await page.route('**/api/applications?*', delayed);
  await page.route('**/api/feed*', delayed);
  await page.route('**/api/products', delayed);
  for (let iteration = 0; iteration < 2; iteration++) {
    await page.getByRole('link', { name: 'Мои заявки', exact: true }).click();
    await page.getByRole('heading', { name: 'Мои заявки', exact: true }).waitFor();
    assert.equal(await page.locator('.application-card').count(), 1);
    assert.equal(await page.locator('.initial-loading').count(), 0);
    await page.getByRole('navigation', {name:'Разделы кабинета'}).getByRole('link', { name: 'Новая заявка', exact: true }).click();
    await page.getByLabel('ИНН компании', { exact: true }).waitFor();
    assert.equal(await page.getByRole('heading', { name: 'Условия продукта', exact: true }).count(), 1);
    await page.getByRole('link', { name: 'Обзор', exact: true }).click();
    await page.getByRole('heading', { name: 'Обзор', exact: true }).waitFor();
    assert.equal(await page.locator('.dashboard-application').count(), 1);
    assert.equal(await page.getByRole('heading', { name: 'Изменение заявки № 1', exact: true }).count(), 1);
    assert.equal(await page.getByText('Загружаем события…', { exact: true }).count(), 0);
  }
  release();
  await page.unroute('**/api/applications?*', delayed);
  await page.unroute('**/api/feed*', delayed);
  await page.unroute('**/api/products', delayed);
  await page.locator('.dashboard-application a').click();
  await page.getByRole('heading', { name: `Заявка № ${app.number}`, exact: true }).waitFor();
  assert.equal(await page.getByText('Как посмотреть эту заявку у банка и пройти до выдачи', { exact: true }).count(), 0);
  await page.getByRole('button', { name: 'Выйти', exact: true }).click();
  await page.getByLabel('Логин', { exact: true }).fill('logistics');
  await page.getByLabel('Пароль', { exact: true }).fill('demo12345');
  await page.getByRole('button', { name: 'Войти', exact: true }).click();
  await page.getByRole('heading', {name:'Начните с первой заявки',exact:true}).waitFor();
  assert.equal(await page.locator('.application-card').count(), 0);
  assert.equal(await page.getByRole('heading', { name: 'Изменение заявки № 1', exact: true }).count(), 0);
  console.log('Navigation: cached sections render without loading under blocked network; form catalog ready, demo guide removed, logout clears user data passed');
} finally { await browser.close(); }
