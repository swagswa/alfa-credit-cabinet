import { expect, test } from '@playwright/test';
import type { components } from '../src/api/schema.gen';

type App = components['schemas']['ApplicationView'];
const start = new Date('2026-10-25T12:00:00Z');
const due = '2026-10-25T12:02:41Z';

for (const role of ['manager', 'client'] as const) {
  test(`${role}: сроки этапов, истечение, обновление карточки и мобильная ширина`, async ({ page }) => {
    page.on('pageerror', (error) => {
      throw error;
    });
    let card = {
      id: 'deadline-test',
      number: 1,
      version: 1,
      status: 'manual_review',
      company: { id: 'company', inn: '0000300011', short_name: 'Тестовая компания' },
      amount_kop: 250000000,
      term_months: 24,
      product: 'working_capital',
      purpose: 'purchase',
      representative_role: 'director',
      bki_consent: true,
      pd_consent: true,
      allowed_next: [],
      reasons: [],
      documents: [],
      due_at: due,
      created_at: start.toISOString(),
      updated_at: start.toISOString(),
    } as App;
    await page.clock.install({ time: start });
    await page.clock.pauseAt(start);
    await page.route('**/api/**', async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (!path.startsWith('/api/')) return route.continue();
      if (path === '/api/notifications') return route.fulfill({ json: [] });
      if (path.endsWith('/signing-code'))
        return route.fulfill({ json: { code: '123456', attempts_left: 3, expires_at: '2026-10-25T12:03:00Z' } });
      if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
      const data =
        path === '/api/me'
          ? {
              user: { id: 'bank', login: 'manager', full_name: 'Менеджер', role },
              companies: [],
            }
          : path.endsWith('/history')
            ? []
            : path === '/api/products'
              ? []
              : card;
      await route.fulfill({ json: data });
    });
    await page.goto(`${role === 'client' ? '/client' : '/bank'}/applications/deadline-test`);
    const deadline = page.getByRole('complementary', { name: 'Срок рассмотрения', exact: true });
    await expect(deadline).toContainText('Осталось 2:41');
    if (role === 'manager') await expect(page.getByText('Предварительная оценка.', { exact: true })).toBeVisible();
    await page.clock.runFor(2000);
    await expect(deadline).toContainText('Осталось 2:39');
    await page.clock.fastForward(160000);
    await expect(deadline).toContainText('Банк продолжает работу с заявкой.');
    await expect(page.getByRole('heading', { name: 'Заявка № 1', exact: true })).toBeVisible();

    async function update(patch: Partial<App>) {
      card = { ...card, ...patch, version: card.version + 1 };
      await page.evaluate(() => window.dispatchEvent(new Event('application-refresh')));
    }
    const later = '2026-10-25T13:00:00Z';
    await update({
      status: 'need_info',
      due_at: later,
      info_request: { due_at: later, documents: [], reasons: [] } as App['info_request'],
    });
    await expect(page.getByRole('complementary', { name: 'Ответить банку', exact: true })).toContainText('Осталось');
    await expect(page.getByText('Предварительная оценка.', { exact: true })).toHaveCount(0);
    await update({
      status: 'counter_offer',
      offer: {
        product: 'working_capital',
        amount_kop: 200000000,
        term_months: 24,
        rate_bp: 1900,
        monthly_payment_kop: 10000000,
        guarantor_required: false,
        pledge_required: false,
        expires_at: '2026-10-25T12:30:00Z',
      },
    });
    await expect(page.getByRole('complementary', { name: 'Принять предложение', exact: true })).toContainText(
      'Осталось 27:18',
    );
    await update({ status: 'signing', allowed_next: role === 'client' ? ['issued'] : [] });
    await expect(page.getByRole('complementary', { name: 'Подписать документы', exact: true })).toBeVisible();
    if (role === 'client') {
      await page.getByRole('button', { name: 'Получить код', exact: true }).click();
      await expect(page.getByRole('complementary', { name: 'Срок действия кода', exact: true })).toContainText(
        'Осталось 0:18',
      );
    }
    await page.setViewportSize({ width: 375, height: 812 });
    await page.clock.runFor(300);
    await expect(page.getByRole('complementary', { name: 'Подписать документы', exact: true })).toBeVisible();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
    await page.screenshot({ path: `test-results/deadline-${role}-375.png`, fullPage: true });
    await update({ status: 'issued' });
    await expect(page.locator('.application-deadline')).toHaveCount(0);
    await update({ status: 'checking', due_at: 'invalid' });
    await expect(page.locator('.application-deadline')).toHaveCount(0);
  });
}
