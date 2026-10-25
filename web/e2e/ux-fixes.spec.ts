import { expect, test, type Page } from '@playwright/test';

test.use({ reducedMotion: 'reduce' });
const company = { id: 'company', inn: '0000100012', short_name: 'Тестовая компания', kind: 'OOO', simulated: true };
const base = {
  id: 'example',
  number: 1,
  version: 1,
  status: 'draft',
  company,
  product: 'working_capital',
  purpose: 'purchase',
  amount_kop: 250000000,
  term_months: 24,
  monthly_revenue_kop: 300000000,
  representative_role: 'director',
  bki_consent: true,
  pd_consent: true,
  documents: [],
  reasons: [],
  allowed_next: ['submitted'],
  created_at: '2026-10-25T12:00:00Z',
  updated_at: '2026-10-25T12:00:00Z',
};
async function fixture(page: Page, changes: Record<string, unknown> = {}) {
  const state = {
    card: { ...base, ...changes },
    authenticated: true,
    user: 'client',
    role: 'client',
    conflict: false,
    writes: [] as unknown[],
    document: {} as Record<string, unknown>,
  };
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.route('**/api/**', async (route) => {
    const request = route.request(),
      path = new URL(request.url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (path === '/api/notifications') return route.fulfill({ json: [] });
    const session = {
      user: { id: state.user, login: 'coffee', full_name: 'Анна', role: state.role },
      companies: [{ company, representative_role: 'director' }],
    };
    if (path === '/api/login') {
      state.authenticated = true;
      return route.fulfill({ json: session });
    }
    if (!state.authenticated) return route.fulfill({ status: 401, json: { detail: 'Session expired' } });
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    if (request.method() !== 'GET') {
      state.writes.push(request.postDataJSON());
      if (state.conflict) return route.fulfill({ status: 409, json: { detail: 'Version conflict' } });
      return route.fulfill({ json: state.card });
    }
    const json =
      path === '/api/me'
        ? session
        : path === '/api/products'
          ? [
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
            ]
          : path === '/api/applications'
            ? { items: [], total: 0 }
            : path.endsWith('/history') || path === '/api/feed'
              ? []
              : path.startsWith('/api/documents/')
                ? state.document
                : state.card;
    return route.fulfill({ json });
  });
  return state;
}

test('анкета переживает reload и переходы, согласия не переносятся другому входу', async ({ page }) => {
  const state = await fixture(page);
  await page.goto('/client/new');
  await page.getByLabel('Сумма, ₽', { exact: true }).fill('3700000,25');
  await page.getByLabel('Срок, месяцев', { exact: true }).fill('18');
  await page.getByLabel('Среднемесячная выручка, ₽', { exact: true }).fill('900000');
  await page.getByText('Согласен на обработку персональных данных для рассмотрения заявки', { exact: true }).click();
  await page.reload();
  await expect(
    page.getByRole('checkbox', {
      name: 'Согласен на обработку персональных данных для рассмотрения заявки',
      exact: true,
    }),
  ).not.toBeChecked();
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue(/^3\s700\s000,25$/);
  await expect(page.getByLabel('Срок, месяцев', { exact: true })).toHaveValue('18');
  await page.getByRole('link', { name: 'Мои заявки', exact: true }).click();
  await page.getByRole('link', { name: 'Новая заявка', exact: true }).click();
  await expect(page.getByLabel('Срок, месяцев', { exact: true })).toHaveValue('18');
  expect(state.writes).toEqual([]);
  state.user = 'other-client';
  await page.reload();
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue('');
});

test('истечение сессии объясняется, вход возвращает к заявке и фильтру', async ({ page }) => {
  const state = await fixture(page);
  await page.goto('/client/applications/example?group=attention');
  await expect(page.getByRole('heading', { name: 'Заявка № 1', exact: true })).toBeVisible();
  state.authenticated = false;
  await page.evaluate(() => window.dispatchEvent(new Event('application-refresh')));
  await expect(page.getByText('Сессия завершена. Войдите снова, чтобы продолжить с открытой страницы.')).toBeVisible();
  await page.getByRole('button', { name: /ООО «Тёплое зерно»/ }).click();
  await expect(page).toHaveURL(/\/client\/applications\/example\?group=attention$/);
});

for (const returnTo of ['https://evil.example', '//evil.example', '/bank/stats']) {
  test(`возврат после входа безопасен: ${returnTo}`, async ({ page }) => {
    const state = await fixture(page);
    state.authenticated = false;
    await page.goto('/login?' + new URLSearchParams({ returnTo }));
    await page.getByRole('button', { name: /ООО «Тёплое зерно»/ }).click();
    await expect(page).toHaveURL(/\/client$/);
  });
}

test('409 сохраняет ввод; только явное обновление заменяет его свежими данными', async ({ page }) => {
  const state = await fixture(page);
  await page.goto('/client/applications/example');
  await page.getByText('Изменить данные заявки', { exact: true }).click();
  await expect(page.getByLabel('Стоимость оборудования, ₽', { exact: true })).toHaveCount(0);
  await page.getByLabel('Сумма, ₽', { exact: true }).fill('3300000');
  state.conflict = true;
  await page.getByRole('button', { name: 'Сохранить изменения', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Обновить данные', exact: true })).toBeVisible();
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue(/^3\s300\s000$/);
  await expect(page.getByRole('button', { name: 'Сохранить изменения', exact: true })).toBeDisabled();
  state.card = { ...state.card, version: 2, amount_kop: 410000000 };
  await page.getByRole('button', { name: 'Обновить данные', exact: true }).click();
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue(/^4\s100\s000$/);
  state.conflict = false;
  await page.getByRole('button', { name: 'Сохранить изменения', exact: true }).click();
  await expect.poll(() => state.writes.length).toBe(2);
  expect(state.writes[1]).toMatchObject({ version: 2, amount_kop: 410000000 });
});

test('паспорт запрошен одним блоком, без выручки и финансовых инструкций', async ({ page }) => {
  await fixture(page, {
    status: 'need_info',
    allowed_next: ['checking'],
    representative_role: 'proxy',
    info_request: { documents: ['passport'], reasons: [], comment: 'Нужен действующий паспорт' },
  });
  await page.goto('/client/applications/example');
  await expect(page.getByRole('heading', { name: 'Нужно от вас', exact: true })).toHaveCount(1);
  await expect(page.getByLabel('Уточнённая выручка за месяц, ₽')).toHaveCount(0);
  await expect(page.getByText('Доверенность', { exact: true })).toHaveCount(0);
  await expect(page.getByText(/Проверьте и подтвердите распознанные/)).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Отправить сведения банку' })).toBeDisabled();
  await page.setViewportSize({ width: 375, height: 812 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: 'test-results/ux-passport-375.png', fullPage: true });
});

test('распознанная отчётность сразу в полях с именем файла и не затирает правки', async ({ page }) => {
  const state = await fixture(page, {
    status: 'need_info',
    allowed_next: ['checking'],
    documents: [{ id: 'doc', kind: 'financials', status: 'recognized', file_name: 'Отчётность.pdf' }],
    info_request: { documents: ['financials'], reasons: [{ code: 'SF_REVENUE_MISMATCH', text: 'Уточните выручку' }] },
  });
  state.document = {
    id: 'doc',
    kind: 'financials',
    status: 'recognized',
    file_name: 'Отчётность.pdf',
    extracted: {
      financials: {
        inn: company.inn,
        year: 2025,
        revenue_kop: 120000000,
        net_profit_kop: 10000000,
        liabilities_long_kop: 0,
        liabilities_short_kop: 5000000,
        balance_total_kop: 20000000,
      },
    },
  };
  await page.goto('/client/applications/example');
  await expect(page.getByRole('heading', { name: 'Отчётность.pdf', exact: true })).toBeVisible();
  const revenue = page.getByLabel('Выручка, ₽ (строка 2110)', { exact: true });
  await expect(revenue).toHaveValue(/^1\s200\s000$/);
  await expect(page.getByLabel('Уточнённая выручка за месяц, ₽')).toBeVisible();
  await revenue.fill('1300000');
  await page.evaluate(() => window.dispatchEvent(new Event('application-refresh')));
  await expect(revenue).toHaveValue(/^1\s300\s000$/);
  await expect(page.getByRole('button', { name: 'Заполнить из документа' })).toHaveCount(0);
});

test('сохранённый черновик очищает локальную анкету, новая заявка начинается заново', async ({ page }) => {
  await fixture(page);
  await page.goto('/client/new');
  await page.getByLabel('Сумма, ₽', { exact: true }).fill('3700000');
  await page.getByText('Согласен на обработку персональных данных для рассмотрения заявки', { exact: true }).click();
  await page.getByRole('button', { name: 'Сохранить черновик', exact: true }).click();
  await expect(page).toHaveURL(/\/client\/applications\/example$/);
  expect(await page.evaluate(() => sessionStorage.getItem('credit:new:v1:client'))).toBeNull();
  await page.getByRole('link', { name: 'Новая заявка', exact: true }).click();
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue('');
});

test('недоступное хранилище не блокирует заполнение анкеты', async ({ page }) => {
  await page.addInitScript(() => {
    Storage.prototype.getItem = () => {
      throw new Error('Storage disabled');
    };
    Storage.prototype.setItem = () => {
      throw new Error('Storage disabled');
    };
  });
  await fixture(page);
  await page.goto('/client/new');
  await page.getByLabel('Сумма, ₽', { exact: true }).fill('1800000');
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue(/^1\s800\s000$/);
});

test('ручной ввод во время распознавания сохраняется после ответа модели', async ({ page }) => {
  const state = await fixture(page, {
    status: 'need_info',
    allowed_next: ['checking'],
    documents: [{ id: 'doc', kind: 'financials', status: 'recognizing', file_name: 'Отчёт.pdf' }],
    info_request: { documents: ['financials'], reasons: [] },
  });
  state.document = { id: 'doc', kind: 'financials', status: 'recognizing', file_name: 'Отчёт.pdf' };
  await page.goto('/client/applications/example');
  await expect(page.getByText('Распознаём документ - обычно это занимает около минуты.')).toBeVisible();
  await page.getByRole('button', { name: 'Заполнить вручную', exact: true }).click();
  const revenue = page.getByLabel('Выручка, ₽ (строка 2110)', { exact: true });
  await revenue.fill('1300000');
  state.document = {
    ...state.document,
    status: 'recognized',
    extracted: {
      financials: {
        inn: company.inn,
        year: 2025,
        revenue_kop: 120000000,
        net_profit_kop: 10000000,
        liabilities_long_kop: 0,
        liabilities_short_kop: 5000000,
        balance_total_kop: 20000000,
      },
    },
  };
  await expect(page.getByText('Распознаём документ - обычно это занимает около минуты.')).toHaveCount(0);
  await expect(revenue).toHaveValue(/^1\s300\s000$/);
});
