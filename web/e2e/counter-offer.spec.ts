import { expect, test, type Page } from '@playwright/test';
import type { components } from '../src/api/schema.gen';

type App = components['schemas']['ApplicationView'];
const requested = {
  product: 'equipment',
  primary: true,
  amount_kop: 600000000,
  term_months: 36,
  rate_bp: 1700,
  monthly_payment_kop: 21391600,
};
const offer = {
  product: 'equipment',
  amount_kop: 390000000,
  term_months: 36,
  rate_bp: 1700,
  monthly_payment_kop: 11972000,
  guarantor_required: true,
  pledge_required: true,
  expires_at: '2099-10-25T12:00:00Z',
};
const original = {
  id: 'counter',
  number: 2,
  version: 5,
  status: 'counter_offer',
  company: { id: 'company', inn: '000020001010', short_name: 'ИП Лебедев' },
  product: 'equipment',
  purpose: 'equipment',
  amount_kop: 600000000,
  term_months: 36,
  equipment_cost_kop: 670000000,
  bki_consent: true,
  pd_consent: true,
  representative_role: 'owner_ip',
  documents: [],
  allowed_next: ['accepted'],
  reasons: [
    { code: 'R_AMOUNT_MULT', kind: 'minus', text: 'Сумма снижена с учётом выручки.' },
    { code: 'R_TAX_CLEAR', kind: 'plus', text: 'Налоговых долгов нет.' },
  ],
  offer,
  created_at: '2026-10-25T12:00:00Z',
  updated_at: '2026-10-25T12:02:00Z',
} as App;

async function open(
  page: Page,
  options: { bank?: boolean; baseline?: 'error' | 'mismatch'; card?: Partial<App>; queueFailure?: boolean } = {},
) {
  let card = { ...original, ...options.card };
  const writes: { path: string; body: unknown }[] = [];
  let cardReads = 0;
  await page.clock.setFixedTime(new Date('2026-10-25T12:03:00Z'));
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.route('**/api/**', async (route) => {
    const req = route.request();
    const path = new URL(req.url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (path === '/api/notifications') return route.fulfill({ json: [] });
    if (req.method() === 'POST') {
      writes.push({ path, body: req.postDataJSON() });
      card = { ...card, status: 'accepted', allowed_next: [], version: 6 };
      return route.fulfill({ json: card });
    }
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    if (path === '/api/products/recommendation') {
      if (options.baseline === 'error') return route.fulfill({ status: 503, json: { detail: 'Недоступно' } });
      return route.fulfill({
        json: { items: [options.baseline === 'mismatch' ? { ...requested, amount_kop: 100000000 } : requested] },
      });
    }
    if (path === '/api/applications/counter') {
      cardReads++;
      if (options.queueFailure) return route.fulfill({ status: 503, json: { detail: 'Недоступно' } });
    }
    const { offer: _, ...summary } = card;
    void _;
    const json =
      path === '/api/me'
        ? {
            user: { id: 'user', role: options.bank ? 'manager' : 'client', login: 'test', full_name: 'Тест' },
            companies: [],
          }
        : path === '/api/products' || path.endsWith('/history')
          ? []
          : path === '/api/applications'
            ? { items: [summary], total: 1 }
            : card;
    return route.fulfill({ json });
  });
  await page.goto(options.bank ? '/bank/applications' : '/client/applications/counter');
  return {
    writes,
    reads: () => cardReads,
    update: (patch: Partial<App>) => {
      card = { ...card, ...patch };
    },
  };
}

test('встречное сразу сравнивает все параметры, показывает собственный взнос и отделяет плюсы', async ({ page }) => {
  const { writes } = await open(page);
  const comparison = page.getByRole('table', { name: 'Запрос и предложение банка', exact: true });
  await expect(comparison).toBeVisible();
  await expect(comparison.getByRole('row').filter({ hasText: 'Сумма' })).toContainText('6 000 000,00 ₽');
  await expect(comparison.getByRole('row').filter({ hasText: 'Сумма' })).toContainText('3 900 000,00 ₽');
  await expect(comparison.getByRole('row').filter({ hasText: 'Платёж в месяц' })).toContainText('213 916,00 ₽');
  await expect(comparison.getByRole('row').filter({ hasText: 'Платёж в месяц' })).toContainText('119 720,00 ₽');
  await expect(page.locator('.counter-own-funds')).toContainText('2 800 000,00 ₽');
  await expect(page.locator('.counter-reasons')).not.toContainText('Налоговых долгов нет.');
  await expect(page.locator('.counter-positives')).toContainText('Налоговых долгов нет.');
  await page.setViewportSize({ width: 375, height: 812 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.locator('.counter-comparison').screenshot({ path: 'test-results/counter-comparison-375.png' });
  await page.getByRole('button', { name: 'Принять условия', exact: true }).click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0]).toEqual({ path: '/api/applications/counter/accept', body: { version: 5 } });
});

for (const baseline of ['error', 'mismatch'] as const) {
  test(`ориентир ${baseline}: условия банка доступны, выдуманного платежа нет`, async ({ page }) => {
    await open(page, { baseline });
    const comparison = page.getByRole('table', { name: 'Запрос и предложение банка', exact: true });
    await expect(comparison.getByText('Ориентир недоступен', { exact: true })).toHaveCount(2);
    await expect(comparison).toContainText('119 720,00 ₽');
    await expect(comparison).not.toContainText('213 916,00 ₽');
    await expect(page.getByRole('button', { name: 'Принять условия', exact: true })).toBeEnabled();
  });
}

test('покрытая покупка не даёт отрицательный взнос', async ({ page }) => {
  await open(page, { card: { equipment_cost_kop: 300000000 } });
  await expect(
    page.getByRole('heading', { name: 'Стоимость покупки покрыта предложенной суммой', exact: true }),
  ).toBeVisible();
  await expect(page.locator('.counter-own-funds')).not.toContainText('-900');
});

test('очередь показывает предложение, сохраняет исходный запрос и использует кэш', async ({ page }) => {
  const { reads, writes, update } = await open(page, { bank: true });
  const row = page.locator('.application-card');
  await expect(row.locator('.application-amount')).toHaveText('3 900 000,00 ₽');
  await expect(row).toContainText('Запрошено: 6 000 000,00 ₽');
  expect(reads()).toBe(1);
  update({
    updated_at: '2026-10-25T12:05:00Z',
    offer: { ...offer, product: 'equipment', amount_kop: 400000000, term_months: 24 },
  });
  await page.evaluate(() => window.dispatchEvent(new Event('application-refresh')));
  await expect(row.locator('.application-amount')).toHaveText('4 000 000,00 ₽');
  await expect(row).toContainText('24 мес.');
  expect(reads()).toBe(2);
  const refreshed = page.waitForResponse((response) => new URL(response.url()).pathname === '/api/applications');
  await page.evaluate(() => window.dispatchEvent(new Event('application-refresh')));
  await refreshed;
  await expect(row.locator('.application-amount')).toHaveText('4 000 000,00 ₽');
  expect(reads()).toBe(2);
  await page.setViewportSize({ width: 375, height: 812 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  expect(writes).toEqual([]);
});

test('очередь при ошибке карточки не называет запрошенную сумму предложением', async ({ page }) => {
  await open(page, { bank: true, queueFailure: true });
  const row = page.locator('.application-card');
  await expect(row).toContainText('Не удалось загрузить условия. Откройте карточку заявки.');
  await expect(row.locator('.application-amount')).toHaveCount(0);
  await expect(row.getByRole('link')).toHaveAttribute('href', '/bank/applications/counter');
});
