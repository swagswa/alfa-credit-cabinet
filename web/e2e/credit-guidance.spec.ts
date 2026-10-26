import { expect, test, type Page } from '@playwright/test';

const company = { id: 'company', inn: '0000100012', short_name: 'ООО «Тёплое зерно»', kind: 'UL', simulated: true };
const plan = {
  available: true,
  score: 25,
  grade: 'E',
  version: '2026-10-26.1',
  items: [
    {
      input: 'tax_arrears_rub',
      title: 'Погасить налоговую задолженность',
      detail: 'После обновления данных ФНС.',
      target: 0,
      delta: 8,
      score: 33,
      grade: 'E',
    },
    {
      input: 'current_dsr',
      title: 'Снизить платежи по текущим кредитам',
      detail: 'До 9% выручки.',
      target: 0.09,
      delta: 14,
      score: 39,
      grade: 'D',
    },
  ],
};
const estimate = {
  version: '2026-10-26.1',
  monthly_revenue_kop: 120000000,
  sources: ['fns', 'internal'],
  items: [{ product: 'working_capital', amount_kop: 240000000 }],
};

async function cabinet(page: Page, options: { failure?: boolean; unavailable?: boolean; empty?: boolean } = {}) {
  const writes: string[] = [];
  let estimates = 0;
  let recovered = false;
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.route('**/api/**', async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (req.method() !== 'GET') writes.push(path);
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    if (path.endsWith('/preliminary')) {
      estimates++;
      if (options.failure && !recovered) return route.fulfill({ status: 503, json: { detail: 'Источник недоступен' } });
      return route.fulfill({ json: { ...estimate, items: options.empty ? [] : estimate.items } });
    }
    if (path.endsWith('/improvements'))
      return route.fulfill({ json: options.unavailable ? { ...plan, available: false, items: [] } : plan });
    if (path === '/api/me')
      return route.fulfill({
        json: {
          user: { id: 'client', login: 'coffee', full_name: 'Анна', role: 'client' },
          companies: [{ company, representative_role: 'director' }],
        },
      });
    if (path === '/api/notifications' || path.endsWith('/history')) return route.fulfill({ json: [] });
    if (path === '/api/products')
      return route.fulfill({
        json: [
          {
            code: 'working_capital',
            name: 'Оборотный кредит',
            amount_min_kop: 30000000,
            amount_max_kop: 1000000000,
            term_min_months: 3,
            term_max_months: 36,
            rate_min_bp: 1900,
            rate_max_bp: 2500,
            requirements: [],
            documents: [],
            repayment: 'annuity',
            min_business_age_months: 12,
          },
        ],
      });
    if (path === `/api/companies/${company.inn}`)
      return route.fulfill({
        json: { ...company, address: 'Москва', okved_main: { code: '56', name: 'Подача напитков' } },
      });
    return route.fulfill({
      json: {
        id: 'rejected',
        number: 4,
        version: 1,
        status: 'rejected',
        company,
        product: 'working_capital',
        purpose: 'purchase',
        amount_kop: 500000000,
        term_months: 24,
        representative_role: 'director',
        bki_consent: true,
        pd_consent: true,
        allowed_next: [],
        reasons: [{ code: 'R_GRADE_E', text: 'Уровень риска выше допустимого.' }],
        documents: [],
        created_at: '2026-10-26T12:00:00Z',
        updated_at: '2026-10-26T12:00:00Z',
      },
    });
  });
  return {
    writes,
    estimateCount: () => estimates,
    recover: () => {
      recovered = true;
    },
  };
}

test('отказ: независимые эффекты, класс и пояснение без изменения решения, 375 px', async ({ page }) => {
  const { writes } = await cabinet(page);
  await page.setViewportSize({ width: 375, height: 812 });
  await page.goto('/client/applications/rejected');
  const section = page.getByRole('region', { name: 'Сценарии улучшения' });
  await expect(section).toContainText('+8 баллов');
  await expect(section).toContainText('Класс: E → D');
  await expect(section).toContainText('Эффекты не складываются');
  await expect(section).toContainText('не обещание одобрения');
  await section.scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: 'test-results/guidance-rejected-mobile.png', fullPage: true });
  expect(writes).toEqual([]);
});

test('отказ без снимка не показывает придуманные баллы', async ({ page }) => {
  await cabinet(page, { unavailable: true });
  await page.goto('/client/applications/rejected');
  const section = page.getByRole('region', { name: 'Сценарии улучшения' });
  await expect(section).toContainText('Числовой прогноз недоступен');
  await expect(section.getByText('+8 баллов')).toHaveCount(0);
});

test('ориентир после поиска, сбой и повтор, изменение ИНН скрывает старую сумму', async ({ page }) => {
  const { writes, estimateCount, recover } = await cabinet(page, { failure: true });
  await page.goto('/client/new');
  await expect(page.getByLabel('ИНН компании', { exact: true })).toHaveValue(company.inn);
  expect(estimateCount()).toBe(0);
  await page.getByRole('button', { name: 'Найти компанию', exact: true }).click();
  const section = page.getByRole('region', { name: 'Предварительный ориентир' });
  await expect(section).toContainText('Можно продолжить заполнение');
  recover();
  await section.getByRole('button', { name: 'Повторить оценку' }).click();
  await expect(section).toContainText('до 2 400 000,00 ₽');
  await expect(section).toContainText('Кредитную историю не запрашиваем');
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue('');
  await page.setViewportSize({ width: 375, height: 812 });
  await section.scrollIntoViewIfNeeded();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
  await page.screenshot({ path: 'test-results/guidance-preliminary-mobile.png', fullPage: true });
  await page.getByLabel('ИНН компании', { exact: true }).fill('0000400016');
  await expect(section).toHaveCount(0);
  expect(writes).toEqual([]);
});

test('пустой ориентир не объявляет отказ', async ({ page }) => {
  await cabinet(page, { empty: true });
  await page.goto('/client/new');
  await page.getByRole('button', { name: 'Найти компанию', exact: true }).click();
  await expect(page.getByRole('region', { name: 'Предварительный ориентир' })).toContainText('Это не отказ');
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toBeEnabled();
});
