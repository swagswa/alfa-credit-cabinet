import { expect, test, type Page } from '@playwright/test';
import type { components } from '../src/api/schema.gen';

type App = components['schemas']['ApplicationView'];
const company = { id: 'company', inn: '0000100012', short_name: 'ООО «Тёплое зерно»', kind: 'OOO', simulated: true };
const original = {
  id: 'original',
  number: 6,
  version: 1,
  company,
  amount_kop: 600000000,
  term_months: 36,
  product: 'equipment',
  purpose: 'equipment',
  representative_role: 'proxy',
  monthly_revenue_kop: 250000000,
  equipment_cost_kop: 670000000,
  bki_consent: true,
  pd_consent: true,
  allowed_next: [],
  reasons: [],
  documents: [{ id: 'contract', kind: 'loan_agreement', status: 'uploaded', file_name: 'Договор.pdf' }],
  created_at: '2026-10-25T12:00:00Z',
  updated_at: '2026-10-25T12:05:00Z',
  offer: {
    product: 'equipment',
    amount_kop: 390000000,
    term_months: 36,
    rate_bp: 1700,
    monthly_payment_kop: 11972000,
    guarantor_required: true,
    pledge_required: true,
    expires_at: '2026-10-25T12:04:00Z',
  },
};

async function cabinet(page: Page, status: App['status'], changes: Partial<App> = {}) {
  page.on('pageerror', (error) => {
    throw error;
  });
  const card = { ...original, status, ...changes } as App;
  const writes: unknown[] = [];
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (path === '/api/notifications') return route.fulfill({ json: [] });
    if (request.method() !== 'GET') {
      writes.push(request.postDataJSON());
      return route.fulfill({
        json: { ...card, id: 'new-draft', number: 7, status: 'draft', documents: [], offer: undefined },
      });
    }
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    if (path.endsWith('/file')) return route.fulfill({ contentType: 'application/pdf', body: '%PDF-1.4\ncontract' });
    const data =
      path === '/api/me'
        ? {
            user: { id: 'client', login: 'coffee', full_name: 'Анна', role: 'client' },
            companies: [{ company, representative_role: 'proxy' }],
          }
        : path === '/api/products'
          ? [
              {
                code: 'equipment',
                name: 'Кредит на оборудование',
                amount_min_kop: 10000000,
                amount_max_kop: 1000000000,
                term_min_months: 1,
                term_max_months: 60,
                rate_min_bp: 1700,
                rate_max_bp: 2500,
                requirements: [],
                documents: [],
                repayment: 'annuity',
                min_business_age_months: 12,
              },
            ]
          : path.endsWith('/history')
            ? []
            : card;
    return route.fulfill({ json: data });
  });
  await page.goto('/client/applications/original');
  return writes;
}

for (const status of ['issued', 'closed'] as const) {
  test(`${status}: договор и платёж сразу в результате, архив без изменений`, async ({ page }) => {
    const writes = await cabinet(page, status);
    await expect(page.getByRole('heading', { name: 'Всё готово. Деньги на счёте!' })).toBeVisible();
    const documents = page.getByRole('region', { name: 'Документы заявки', exact: true });
    const contract = documents.getByRole('link', { name: 'Договор.pdf' });
    await expect(contract).toHaveAttribute('href', '/api/documents/contract/file');
    await expect(page.locator('.repayment-summary')).toContainText('119 720,00 ₽');
    await expect(page.locator('.repayment-summary')).toContainText('Дата первого платежа пока не передана банком.');
    expect(await page.evaluate(async () => (await fetch('/api/documents/contract/file')).text())).toContain('%PDF');
    await page.getByRole('button', { name: '4. Документы и подпись', exact: true }).click();
    await expect(page.getByRole('link', { name: 'Договор.pdf' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Подписать', exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: '5. Результат', exact: true }).click();
    await expect(page.locator('.outcome-content')).toHaveCSS('opacity', '1');
    await page.setViewportSize({ width: 375, height: 812 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
    await page.screenshot({ path: `test-results/terminal-${status}-375.png`, fullPage: true });
    expect(writes).toEqual([]);
  });
}

test('expired: старое предложение недоступно, новая анкета переносит параметры без согласий и документов', async ({
  page,
}) => {
  const writes = await cabinet(page, 'expired');
  await expect(page.getByRole('heading', { name: 'Срок предложения истёк', exact: true })).toBeVisible();
  await expect(page.getByText('Предложение больше не действует', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Принять условия', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '3. Предложение банка', exact: true }).click();
  await expect(page.getByText('Сохранённые условия. Предложение больше не действует.', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Принять условия', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '5. Результат', exact: true }).click();
  await page.setViewportSize({ width: 375, height: 812 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: 'test-results/terminal-expired-375.png', fullPage: true });
  await page.getByRole('link', { name: 'Подать заново', exact: true }).click();
  await expect(page.getByLabel('ИНН компании', { exact: true })).toHaveValue(company.inn);
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue(/^6\s000\s000$/);
  await expect(page.getByLabel('Срок, месяцев', { exact: true })).toHaveValue('36');
  await expect(page.getByLabel('Стоимость оборудования, ₽', { exact: true })).toHaveValue(/^6\s700\s000$/);
  await expect(page.getByLabel('Среднемесячная выручка, ₽', { exact: true })).toHaveValue(/^2\s500\s000$/);
  await expect(page.getByLabel('Действую по доверенности', { exact: true })).toBeChecked();
  await expect(
    page.getByLabel('Согласен на обработку персональных данных для рассмотрения заявки', { exact: true }),
  ).not.toBeChecked();
  await expect(
    page.getByLabel('Согласен на запрос кредитной истории в бюро кредитных историй', { exact: true }),
  ).not.toBeChecked();
  await expect(page.getByRole('button', { name: 'Сохранить черновик', exact: true })).toBeDisabled();
  await page.getByText('Согласен на обработку персональных данных для рассмотрения заявки', { exact: true }).click();
  await page.getByText('Согласен на запрос кредитной истории в бюро кредитных историй', { exact: true }).click();
  await page.getByRole('button', { name: 'Сохранить черновик', exact: true }).click();
  await expect(page).toHaveURL(/new-draft$/);
  expect(writes).toHaveLength(1);
  expect(writes[0]).toMatchObject({
    amount_kop: 600000000,
    term_months: 36,
    product: 'equipment',
    bki_consent: true,
    pd_consent: true,
  });
  expect(writes[0]).not.toHaveProperty('documents');
  expect(writes[0]).not.toHaveProperty('offer');
});

test('отмена: сведения сохранены, фиктивных условий нет', async ({ page }) => {
  const writes = await cabinet(page, 'cancelled_by_client', { offer: undefined, documents: [] });
  await expect(page.getByRole('heading', { name: 'Заявка отменена', exact: true })).toBeVisible();
  await expect(page.locator('.terminal-result')).toContainText(company.short_name);
  await expect(page.locator('.terminal-result')).toContainText('6 000 000,00 ₽');
  await expect(page.getByRole('button', { name: 'Отменить заявку', exact: true })).toHaveCount(0);
  expect(writes).toEqual([]);
});

test('истечение без предложения: новый черновик возможен, вымышленных условий нет', async ({ page }) => {
  const writes = await cabinet(page, 'expired', { offer: undefined, documents: [] });
  await expect(page.getByRole('heading', { name: 'Срок ответа истёк', exact: true })).toBeVisible();
  await expect(page.locator('.expired-offer')).toHaveCount(0);
  await expect(page.getByRole('link', { name: 'Подать заново', exact: true })).toBeVisible();
  await page.getByRole('button', { name: '3. Предложение банка', exact: true }).click();
  await expect(page.getByText('По этой заявке предложение банка не сформировано.', { exact: true })).toBeVisible();
  expect(writes).toEqual([]);
});

test('овердрафт не обещает фиксированного платежа', async ({ page }) => {
  await cabinet(page, 'issued', { product: 'overdraft', offer: undefined, documents: [] });
  await expect(page.getByRole('heading', { name: 'Погашение овердрафта' })).toBeVisible();
  await expect(page.locator('.repayment-summary')).toContainText('Фиксированного ежемесячного платежа нет.');
  await expect(page.getByText('Прикреплённых документов нет.', { exact: true })).toBeVisible();
});
