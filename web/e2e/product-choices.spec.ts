import { expect, test, type Page } from '@playwright/test';

const company = { id: 'company', inn: '0000100012', short_name: 'Тестовая компания', kind: 'OOO', simulated: true };
const items = [
  {
    product: 'equipment',
    primary: true,
    amount_kop: 250000000,
    term_months: 24,
    rate_bp: 1700,
    monthly_payment_kop: 12300000,
  },
  {
    product: 'working_capital',
    primary: false,
    amount_kop: 240000000,
    term_months: 12,
    rate_bp: 1900,
    monthly_payment_kop: 24000000,
  },
];

async function form(page: Page, recommendations = items, ai = false) {
  const writes: Record<string, unknown>[] = [];
  const queries: URL[] = [];
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) return route.continue();
    if (url.pathname === '/api/notifications') return route.fulfill({ json: [] });
    if (url.pathname === '/api/health') return route.fulfill({ json: { status: 'ok', checks: {}, ai } });
    if (url.pathname === '/api/products/need') {
      writes.push(route.request().postDataJSON());
      return route.fulfill({ json: { goal: 'equipment', amount_kop: 200000000, term_months: 24 } });
    }
    if (route.request().method() === 'POST') {
      writes.push(route.request().postDataJSON());
      return route.fulfill({ status: 422, json: { detail: 'Тестовая ошибка сохранения' } });
    }
    const json =
      url.pathname === '/api/me'
        ? {
            user: { id: 'client', role: 'client', login: 'coffee', full_name: 'Анна' },
            companies: [{ company, representative_role: 'director' }],
          }
        : url.pathname === '/api/products'
          ? ['working_capital', 'equipment', 'overdraft'].map((code) => ({
              code,
              name:
                code === 'equipment'
                  ? 'Кредит на оборудование'
                  : code === 'overdraft'
                    ? 'Овердрафт'
                    : 'Оборотный кредит',
              amount_min_kop: 30000000,
              amount_max_kop: 1000000000,
              term_min_months: 3,
              term_max_months: 36,
              rate_min_bp: 1700,
              rate_max_bp: 2500,
              min_business_age_months: 12,
              requirements: [],
              documents: [],
              repayment: 'annuity',
            }))
          : { items: recommendations };
    if (url.pathname.endsWith('/recommendation')) queries.push(url);
    await route.fulfill({ json });
  });
  await page.goto('/client/new');
  await expect(page.getByLabel('ИНН компании', { exact: true })).toHaveValue(company.inn);
  return { writes, queries };
}

test('карточки выбираются мышью и клавиатурой, новый расчёт скрывает старый, выбор уходит в API', async ({ page }) => {
  const { writes, queries } = await form(page);
  const amount = page.getByLabel('Сумма, ₽', { exact: true });
  await amount.fill('2500000');
  await expect(amount).toHaveValue(/^2\s500\s000$/);
  await expect(page.getByText('От 300 000 ₽ до 10 000 000 ₽', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Подобрать продукт', exact: true }).click();
  const primary = page.getByRole('radio', { name: 'Рекомендация: Кредит на оборудование', exact: true });
  const alternative = page.getByRole('radio', { name: 'Альтернатива: Оборотный кредит', exact: true });
  await expect(primary).toBeChecked();
  await expect(page.locator('.product-choice-payment').first()).toHaveText('123 000 ₽');
  await expect(page.getByLabel('Стоимость оборудования, ₽', { exact: true })).toBeVisible();
  await alternative.check();
  await expect(alternative).toBeChecked();
  await expect(page.getByLabel('Срок, месяцев', { exact: true })).toHaveValue('12');
  await expect(amount).toHaveValue(/^2\s400\s000$/);
  await expect(page.getByLabel('Стоимость оборудования, ₽', { exact: true })).toHaveCount(0);
  await alternative.press('ArrowLeft');
  await expect(primary).toBeChecked();
  await expect(amount).toHaveValue(/^2\s500\s000$/);
  await page.getByText('Согласен на обработку персональных данных для рассмотрения заявки', { exact: true }).click();
  await expect(page.getByRole('radio')).toHaveCount(2);
  await page.getByLabel('Стоимость оборудования, ₽', { exact: true }).fill('3000000,25');
  await expect(page.getByRole('radio')).toHaveCount(2);
  await page.setViewportSize({ width: 375, height: 812 });
  await expect(primary).toBeVisible();
  await amount.scrollIntoViewIfNeeded();
  expect(
    await amount.evaluate(
      (el) => getComputedStyle(el).getPropertyValue('-webkit-text-fill-color') === getComputedStyle(el).color,
    ),
  ).toBeTruthy();
  await amount.screenshot({ path: 'test-results/product-amount.png' });
  await page.locator('.product-choices').screenshot({ path: 'test-results/product-choices-375.png' });
  await page.getByRole('button', { name: 'Сохранить черновик', exact: true }).click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0]).toMatchObject({
    product: 'equipment',
    amount_kop: 250000000,
    term_months: 24,
    equipment_cost_kop: 300000025,
  });
  expect(queries[0].searchParams.get('amount_kop')).toBe('250000000');
  await amount.fill('2500000,75');
  await expect(amount).toHaveValue(/^2\s500\s000,75$/);
  await expect(page.getByRole('radio')).toHaveCount(0);
  await page.getByRole('button', { name: 'Подобрать продукт', exact: true }).click();
  await expect(primary).toBeChecked();
  expect(queries.at(-1)?.searchParams.get('amount_kop')).toBe('250000075');
  await amount.fill('0');
  await expect(page.getByText('Сумма вне диапазона выбранного продукта', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Подобрать продукт', exact: true })).toBeDisabled();
  await amount.fill('');
  await expect(amount).toHaveValue('');
});

test('проценты по овердрафту не названы фиксированным платежом', async ({ page }) => {
  await form(page, [items[0], { ...items[1], product: 'overdraft' }]);
  await page.getByLabel('Сумма, ₽', { exact: true }).fill('2500000');
  await page.getByRole('button', { name: 'Подобрать продукт', exact: true }).click();
  await expect(page.getByText('проценты за месяц при полном использовании лимита', { exact: true })).toBeVisible();
  await page.getByRole('radio', { name: 'Альтернатива: Овердрафт', exact: true }).check();
  await expect(page.getByRole('radio', { name: 'Альтернатива: Овердрафт', exact: true })).toBeChecked();
});

test('пустой подбор не ломает форму', async ({ page }) => {
  await form(page, []);
  await page.getByLabel('Сумма, ₽', { exact: true }).fill('2500000');
  await page.getByRole('button', { name: 'Подобрать продукт', exact: true }).click();
  await expect(
    page.getByText('Для этих параметров нет подходящих продуктов. Измените сумму или срок.', { exact: true }),
  ).toBeVisible();
  await expect(page.getByRole('radio')).toHaveCount(0);
});

test('без ИИ поля «своими словами» нет, с ИИ оно заполняет цель, сумму и срок', async ({ page }) => {
  await form(page);
  await expect(page.getByLabel('Опишите своими словами', { exact: true })).toHaveCount(0);

  const { writes } = await form(page, items, true);
  await page.getByLabel('Опишите своими словами', { exact: true }).fill('Нужно 2 млн на кофемашину, отдам за пару лет');
  await page.getByRole('button', { name: 'Заполнить', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Заполнили цель, сумму и срок' })).toBeVisible();
  expect(writes[0]).toEqual({ text: 'Нужно 2 млн на кофемашину, отдам за пару лет' });
  await expect(page.getByLabel('Сумма, ₽', { exact: true })).toHaveValue(/^2\s000\s000$/);
  await expect(page.getByLabel('Срок, месяцев', { exact: true })).toHaveValue('24');
  await expect(page.getByText('Оборудование и транспорт', { exact: true }).first()).toBeVisible();
  await page.setViewportSize({ width: 375, height: 812 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
});
