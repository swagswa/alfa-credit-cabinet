import { expect, test, type Page } from '@playwright/test';
test.use({ reducedMotion: 'reduce' });

const base = {
  id: 'example',
  number: 1,
  version: 1,
  status: 'signing',
  company: { id: 'company', inn: '0000100012', short_name: 'Тестовая компания' },
  amount_kop: 250000000,
  term_months: 24,
  product: 'working_capital',
  purpose: 'purchase',
  representative_role: 'director',
  bki_consent: true,
  pd_consent: true,
  allowed_next: ['issued'],
  reasons: [],
  documents: [] as Record<string, unknown>[],
  created_at: '2026-10-25T12:00:00Z',
  updated_at: '2026-10-25T12:00:00Z',
};
async function cabinet(page: Page, changes = {}, bank = false) {
  const state = {
    card: { ...base, ...changes },
    codes: 0,
    signs: [] as Record<string, unknown>[],
    reject: false,
    confirmed: [] as Record<string, unknown>[],
  };
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (path === '/api/notifications') return route.fulfill({ json: [] });
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    if (path.endsWith('/signing-code')) {
      state.codes++;
      return route.fulfill({
        json: {
          code: '123456',
          attempts_left: 3,
          expires_at: new Date(Date.now() + 120000 + state.codes * 1000).toISOString(),
        },
      });
    }
    if (path.endsWith('/sign')) {
      state.signs.push(route.request().postDataJSON());
      if (state.reject) return route.fulfill({ status: 422, json: {} });
      state.card = { ...state.card, status: 'issued', version: 2, allowed_next: [] };
      return route.fulfill({ json: state.card });
    }
    if (path.endsWith('/confirm')) {
      state.confirmed.push(route.request().postDataJSON());
      return route.fulfill({ status: 422, json: {} });
    }
    const json =
      path === '/api/me'
        ? { user: { id: 'user', login: 'test', full_name: 'Тест', role: bank ? 'manager' : 'client' }, companies: [] }
        : path === '/api/products' || path.endsWith('/history')
          ? []
          : path.startsWith('/api/documents/')
            ? { id: 'financials', kind: 'financials', status: 'skipped', file_name: 'Отчётность.pdf' }
            : state.card;
    return route.fulfill({ json });
  });
  const response = await page.goto(`${bank ? '/bank' : '/client'}/applications/example`);
  expect(response?.headers()['permissions-policy']).toContain('otp-credentials=()');
  return state;
}

test('код: ввод, исправление, ошибка, новый код и явное подписание', async ({ page }) => {
  const state = await cabinet(page);
  await page.getByRole('button', { name: 'Получить код', exact: true }).click();
  const first = page.getByLabel('Код 1 из 6', { exact: true });
  await first.pressSequentially('1234');
  await expect(page.getByLabel('Код 5 из 6', { exact: true })).toBeFocused();
  await expect(page.getByRole('button', { name: 'Подписать', exact: true })).toBeDisabled();
  await page.getByLabel('Код 5 из 6', { exact: true }).pressSequentially('56');
  await expect(page.getByRole('button', { name: 'Подписать', exact: true })).toBeEnabled();
  expect(state.signs).toEqual([]);
  state.reject = true;
  await page.getByRole('button', { name: 'Подписать', exact: true }).click();
  await expect(first).toHaveValue('');
  await expect(page.getByRole('button', { name: 'Подписать', exact: true })).toBeDisabled();
  await page.getByRole('button', { name: 'Получить новый код', exact: true }).click();
  await expect(first).toHaveValue('');
  await first.fill('123456');
  state.reject = false;
  await page.getByRole('button', { name: 'Подписать', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Всё готово. Деньги на счёте!' })).toBeVisible();
  expect(state.signs).toEqual([{ code: '123456' }, { code: '123456' }]);
});

test('раскрытие клавиатурой, скрытые поля без фокуса, таблица сравнения на 375 px', async ({ page }) => {
  await cabinet(page, {
    status: 'approved',
    allowed_next: ['accepted'],
    offer: {
      product: 'working_capital',
      amount_kop: 250000000,
      term_months: 24,
      rate_bp: 1900,
      monthly_payment_kop: 12602000,
      expires_at: '2099-10-25T12:00:00Z',
      guarantor_required: false,
      pledge_required: false,
    },
  });
  await page.setViewportSize({ width: 375, height: 900 });
  const trigger = page.getByRole('button', { name: 'Сравнить с моей заявкой', exact: true });
  await expect(trigger).toHaveAttribute('aria-expanded', 'false');
  await expect(page.getByRole('table')).toHaveCount(0);
  await trigger.focus();
  await trigger.press('Space');
  await expect(trigger).toHaveAttribute('aria-expanded', 'true');
  await expect(page.getByRole('columnheader', { name: 'Запрошено', exact: true })).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: 'Сумма' })).toContainText('2 500 000,00 ₽');
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: 'test-results/core-offer-375.png', fullPage: true });
  await trigger.press('Enter');
  await expect(trigger).toHaveAttribute('aria-expanded', 'false');
  await expect(page.getByRole('table')).toHaveCount(0);
});

test('отчётность сохраняет отрицательную прибыль и копейки через AmountInput', async ({ page }) => {
  const state = await cabinet(page, {
    status: 'need_info',
    allowed_next: ['checking'],
    documents: [{ id: 'financials', kind: 'financials', status: 'skipped', file_name: 'Отчётность.pdf' }],
    info_request: { documents: ['financials'], reasons: [] },
  });
  await page.getByLabel('Отчётный год', { exact: true }).fill(String(new Date().getFullYear() - 1));
  await page.getByLabel('Выручка, ₽ (строка 2110)', { exact: true }).fill('1200000,25');
  await page.getByLabel('Чистая прибыль, ₽ (строка 2400)', { exact: true }).fill('-30000,50');
  await page.getByLabel('Долгосрочные обязательства, ₽ (строка 1400)', { exact: true }).fill('10000');
  await page.getByLabel('Краткосрочные обязательства, ₽ (строка 1500)', { exact: true }).fill('20000');
  await page.getByLabel('Итог баланса, ₽ (строка 1600)', { exact: true }).fill('50000');
  await page.getByRole('button', { name: 'Подтвердить отчётность', exact: true }).click();
  await expect.poll(() => state.confirmed.length).toBe(1);
  expect(state.confirmed[0]).toMatchObject({ financials: { revenue_kop: 120000025, net_profit_kop: -3000050 } });
});

test('банковская таблица скоринга сохраняет заголовки и значения', async ({ page }) => {
  await cabinet(
    page,
    {
      status: 'manual_review',
      allowed_next: [],
      scoring: {
        score: 70,
        grade: 'B',
        pd: 0.02,
        version: 'test',
        factors: [{ code: 'age', name: 'Возраст бизнеса', value: 36, points: 8, max_points: 10 }],
      },
    },
    true,
  );
  await expect(page.getByRole('columnheader', { name: 'Фактор', exact: true })).toBeVisible();
  await expect(page.getByRole('row').filter({ hasText: 'Возраст бизнеса' })).toContainText('8 / 10');
  await page.setViewportSize({ width: 375, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});

test('банковское сравнение документа сохраняет отдельно модель и подтверждённые значения', async ({ page }) => {
  await cabinet(
    page,
    {
      status: 'manual_review',
      allowed_next: [],
      documents: [{ id: 'financials', kind: 'financials', status: 'confirmed', file_name: 'Отчётность.pdf' }],
    },
    true,
  );
  await page.route('**/api/documents/financials', (route) =>
    route.fulfill({
      json: {
        id: 'financials',
        kind: 'financials',
        status: 'confirmed',
        file_name: 'Отчётность.pdf',
        edited_fields: ['revenue_kop'],
        extracted: { financials: { revenue_kop: 120000000 } },
        confirmed: { financials: { revenue_kop: 130000000 } },
      },
    }),
  );
  await page.reload();
  const table = page.getByRole('table', { name: 'Данные модели и клиента', exact: true });
  await expect(table.getByRole('columnheader', { name: 'Модель', exact: true })).toBeVisible();
  await expect(table.getByRole('row').filter({ hasText: 'Выручка (изменено)' })).toContainText('1 200 000,00 ₽');
  await expect(table.getByRole('row').filter({ hasText: 'Выручка (изменено)' })).toContainText('1 300 000,00 ₽');
  await page.setViewportSize({ width: 375, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});
