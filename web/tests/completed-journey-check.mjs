import { chromium } from 'playwright';
import assert from 'node:assert/strict';

const browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || 'msedge' });
const root = 'http://127.0.0.1:5175';
async function fixture(login, inn) {
  const context = await browser.newContext();
  await context.request.post(root + '/api/login', { data: { login, password: 'demo12345' } });
  const created = await context.request.post(root + '/api/applications', { headers: { 'Idempotency-Key': crypto.randomUUID() }, data: { inn, product: 'working_capital', purpose: 'purchase', representative_role: 'director', amount_kop: 250000000, term_months: 24, bki_consent: true, comment: 'Закупка сырья' } });
  const app = await created.json();
  if (login === 'coffee') {
    const uploaded = await context.request.post(root + '/api/applications/' + app.id + '/documents', { multipart: { kind: 'financials', file: { name: 'report.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4 test') } } });
    assert.equal(uploaded.status(), 201);
    const document = await uploaded.json();
    const confirmed = await context.request.post(root + '/api/documents/' + document.id + '/confirm', { data: { financials: { inn, year: 2025, revenue_kop: 1200000000, net_profit_kop: 200000000, liabilities_long_kop: 100000000, liabilities_short_kop: 100000000, balance_total_kop: 1000000000 } } });
    assert.equal(confirmed.status(), 200);
  }
  await context.request.post(root + '/api/applications/' + app.id + '/submit');
  return { context, app };
}
try {
  const { context, app } = await fixture('coffee', '0000100012');
  const approved = await (await context.request.get(root + '/api/applications/' + app.id)).json();
  assert.equal(approved.status, 'approved');
  await context.request.post(root + '/api/applications/' + app.id + '/accept', { data: { version: approved.version } });
  const signing = await (await context.request.post(root + '/api/applications/' + app.id + '/signing-code')).json();
  await context.request.post(root + '/api/applications/' + app.id + '/sign', { data: { code: signing.code } });
  const page = await context.newPage();
  const mutations = [];
  page.on('request', request => { if (!['GET', 'HEAD'].includes(request.method())) mutations.push(request.url()); });
  await page.goto(root + '/client/applications/' + app.id);
  await page.getByRole('heading', { name: 'Всё готово. Деньги на счёте!', exact: true }).waitFor();
  assert.equal(await page.locator('.connection').count(), 0);
  assert.equal(await page.locator('.success-result a, .success-result button').count(), 0);
  assert.equal(await page.getByRole('link', { name: 'К списку заявок', exact: true }).count(), 1);
  assert.equal(await page.locator('article').getByText(/ИНН/).count(), 0);
  for (const [index, title] of ['Заполнение', 'Рассмотрение', 'Предложение банка', 'Документы и подпись', 'Результат'].entries()) {
    await page.getByRole('button', { name: `${index + 1}. ${title}`, exact: true }).click();
    const panel = page.getByRole('region', { name: title, exact: true });
    await panel.waitFor();
    assert.equal(await panel.locator('input, select, textarea, form, button').count(), 0);
    if (index === 0) {
      await panel.getByText('Закупка сырья', { exact: true }).waitFor();
      assert.equal(await panel.getByText('0000100012', { exact: true }).count(), 1);
    }
    if (index === 1) await panel.locator('.archive-timeline').waitFor();
    if (index === 3) {
      await panel.getByText('Документы подписаны.', { exact: true }).waitFor();
      await panel.getByText('Посмотреть подтверждённые данные', { exact: true }).click();
      await panel.getByText('Отчётный год', { exact: true }).waitFor();
      await panel.getByText('2025', { exact: true }).waitFor();
      assert.equal(await panel.locator('input, select, textarea, form').count(), 0);
    }
  }
  assert.deepEqual(mutations, [], 'Viewing completed stages never sends mutation requests');
  const rejected = await fixture('build', '0000400016');
  const other = await rejected.context.newPage();
  await other.goto(root + '/client/applications/' + rejected.app.id);
  await other.getByRole('heading', { name: 'Банк не одобрил заявку', exact: true }).waitFor();
  assert.equal(await other.locator('.rejection-result a, .rejection-result button').count(), 0);
  await other.getByRole('button', { name: '3. Предложение банка', exact: true }).click();
  await other.getByText('По этой заявке предложение банка не сформировано.', { exact: true }).waitFor();
  await other.getByRole('button', { name: '4. Документы и подпись', exact: true }).click();
  await other.getByText('Подписание не завершено.', { exact: true }).waitFor();
  await other.getByRole('button', { name: '5. Результат', exact: true }).focus();
  await other.keyboard.press('Enter');
  await other.getByRole('heading', { name: 'Банк не одобрил заявку', exact: true }).waitFor();
  for (const width of [390, 1366]) {
    await other.setViewportSize({ width, height: 900 });
    assert.equal(await other.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
  }
  await other.getByRole('link', { name: 'К списку заявок', exact: true }).click();
  await other.waitForURL('**/client/applications');
  console.log('Completed journey: compact result, all five read-only stages, no mutations, unvisited stages, keyboard, mobile and back navigation passed');
} finally { await browser.close(); }
