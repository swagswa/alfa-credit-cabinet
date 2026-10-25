import { expect, test, type Page } from '@playwright/test';
import { createRequire } from 'node:module';
import type { AxeResults } from 'axe-core';

const require = createRequire(import.meta.url);
test.use({ reducedMotion: 'reduce' });

const card = {
  id: 'example',
  number: 1,
  version: 1,
  status: 'approved',
  company: { id: 'company', inn: '0000100012', short_name: 'Тестовая компания', kind: 'OOO', simulated: true },
  product: 'working_capital',
  purpose: 'purchase',
  amount_kop: 250000000,
  term_months: 24,
  representative_role: 'director',
  bki_consent: true,
  pd_consent: true,
  allowed_next: ['accepted'],
  reasons: [],
  documents: [],
  created_at: '2026-10-25T12:00:00Z',
  updated_at: '2026-10-25T12:00:00Z',
  offer: {
    product: 'working_capital',
    amount_kop: 250000000,
    term_months: 24,
    rate_bp: 1900,
    monthly_payment_kop: 12602000,
    guarantor_required: false,
    pledge_required: false,
    expires_at: '2099-10-25T12:00:00Z',
  },
};

async function cabinet(page: Page, path: string) {
  const bank = path.startsWith('/bank');
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    if (!url.pathname.startsWith('/api/')) return route.continue();
    if (url.pathname === '/api/notifications') return route.fulfill({ json: [] });
    if (url.pathname === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    const json =
      url.pathname === '/api/me'
        ? {
            user: { id: 'user', role: bank ? 'manager' : 'client', login: 'test', full_name: 'Тест' },
            companies: bank ? [] : [{ company: card.company, representative_role: 'director' }],
          }
        : url.pathname === '/api/stats'
          ? {
              days: Number(url.searchParams.get('days')),
              submitted: 1234567,
              decided: 50,
              auto_decided: 40,
              approved: 30,
              decision_time: { median_sec: 5400, p90_sec: 10800 },
              issued_kop: 999999999900,
              by_status: [
                { status: 'need_info', count: 30, overdue: 5 },
                { status: 'manual_review', count: 5, overdue: 1 },
              ],
            }
          : url.pathname === '/api/applications'
            ? { items: [card], total: 1 }
            : url.pathname === '/api/products'
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
                    min_business_age_months: 12,
                    requirements: [],
                    documents: [],
                    repayment: 'annuity',
                  },
                ]
              : url.pathname === '/api/feed'
                ? [
                    {
                      id: 'event',
                      kind: 'event',
                      title: 'Банк одобрил заявку',
                      text: 'Проверьте условия финансирования',
                      application_id: card.id,
                      created_at: card.updated_at,
                    },
                  ]
                : url.pathname.endsWith('/history')
                  ? []
                  : card;
    return route.fulfill({ json });
  });
  await page.goto(path);
  await expect(page.locator('h1')).toBeVisible();
}

async function audit(page: Page) {
  await page.addScriptTag({ path: require.resolve('axe-core/axe.min.js') });
  const result = await page.evaluate(async () =>
    (window as unknown as { axe: { run: () => Promise<AxeResults> } }).axe.run(),
  );
  const violations = result.violations.filter((v) => v.impact === 'critical' || v.impact === 'serious');
  // Три известные ошибки установленного Select, остальные правила проверяем полностью.
  const known = (id: string, html: string) =>
    (id === 'aria-input-field-name' && html.includes('class="select__component_')) ||
    (id === 'aria-valid-attr-value' && html.includes('select__field_')) ||
    (id === 'svg-img-alt' && html.includes('select__arrow_'));
  await test.info().attach('axe-known-select-issues', {
    body: JSON.stringify(violations.map((v) => ({ id: v.id, nodes: v.nodes.filter((n) => known(v.id, n.html)) }))),
    contentType: 'application/json',
  });
  expect(
    violations
      .map((v) => ({
        id: v.id,
        nodes: v.nodes
          .filter((n) => !known(v.id, n.html))
          .map((n) => ({ target: n.target, html: n.html, summary: n.failureSummary })),
      }))
      .filter((v) => v.nodes.length),
  ).toEqual([]);
}

test('сводка: заголовок, большие числа, четыре ширины и выбор периода', async ({ page }) => {
  await cabinet(page, '/bank/stats');
  await expect(page.locator('.stats-metrics')).toContainText('9 999 999 999 ₽');
  for (const width of [375, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
    const title = page.getByRole('heading', { name: 'Сводка', exact: true });
    expect(await title.evaluate((el) => el.getBoundingClientRect().height)).toBeLessThan(60);
    expect(
      await page
        .locator('.stats-metrics strong')
        .evaluateAll((elements) => elements.every((el) => el.scrollWidth <= el.clientWidth)),
    ).toBeTruthy();
    await page.screenshot({ path: `test-results/stats-${width}.png`, fullPage: true });
    await audit(page);
  }
  await page.getByRole('button', { name: '7 дн.', exact: true }).click();
  await expect(page.getByRole('button', { name: '7 дн.', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('button', { name: '30 дн.', exact: true })).toHaveAttribute('aria-pressed', 'false');
  await audit(page);
});

for (const path of [
  '/client',
  '/client/applications',
  '/client/new',
  '/client/applications/example',
  '/bank/applications',
  '/bank/applications/example',
]) {
  test(`доступность ${path}`, async ({ page }) => {
    await cabinet(page, path);
    await page.setViewportSize({ width: 375, height: 1000 });
    if (path === '/client/applications') {
      const value = page.locator('.motion-value');
      await expect(value).toHaveText('2 500 000,00 ₽');
      await expect(value).not.toHaveAttribute('aria-label');
      expect(
        await value.evaluate((el) => el.childNodes.length === 1 && el.firstChild?.nodeType === Node.TEXT_NODE),
      ).toBeTruthy();
    }
    if (path === '/client/new') {
      const consent = page.getByRole('checkbox', {
        name: 'Согласен на обработку персональных данных для рассмотрения заявки',
        exact: true,
      });
      await consent.check({ timeout: 3000 });
      await expect(consent).toBeChecked();
      await consent.press('Space');
      await expect(consent).not.toBeChecked();
    }
    await audit(page);
  });
}

test('уведомление имеет регион, статус и доступную кнопку закрытия', async ({ page }) => {
  await cabinet(page, '/client');
  const refresh = page.getByRole('button', { name: 'Обновить события', exact: true });
  await refresh.focus();
  await page.evaluate(() =>
    window.dispatchEvent(
      new CustomEvent('cabinet-notice', { detail: { id: 'test', message: 'Изменения сохранены.', tone: 'success' } }),
    ),
  );
  const region = page.getByRole('region', { name: 'Уведомления', exact: true });
  await expect(region.getByRole('status')).toHaveText('Изменения сохранены.');
  await expect(refresh).toBeFocused();
  await audit(page);
  await region.getByRole('button', { name: 'Закрыть уведомление', exact: true }).click();
  await expect(region.getByRole('status')).toHaveCount(0);
});
