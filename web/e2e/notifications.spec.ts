import { expect, test, type Page } from '@playwright/test';
import { createRequire } from 'node:module';
import type { AxeResults } from 'axe-core';

const require = createRequire(import.meta.url);

async function cabinet(page: Page, empty = false) {
  let notifications = empty
    ? []
    : [
        {
          id: 1,
          application_id: 'example',
          kind: 'signing_code',
          title: 'Код подписания',
          text: 'Код 194111 для подписания договора по заявке №1. Никому не сообщайте его',
          created_at: '2026-10-26T12:00:00Z',
          read_at: undefined as string | undefined,
        },
        ...Array.from({ length: 8 }, (_, index) => ({
          id: index + 2,
          application_id: 'example',
          kind: 'approved',
          title: `Решение по заявке №${index + 2}`,
          text: 'Банк одобрил заявку. Проверьте условия финансирования.',
          created_at: '2026-10-26T11:30:00Z',
          read_at: index ? '2026-10-26T11:40:00Z' : undefined,
        })),
      ];
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    if (path === '/api/notifications') return route.fulfill({ json: notifications });
    if (path.endsWith('/read') || path.endsWith('/read-all')) {
      const id = Number(path.split('/').at(-2));
      notifications = notifications.map((item) =>
        item.id === id || path.endsWith('/read-all') ? { ...item, read_at: new Date().toISOString() } : item,
      );
      return route.fulfill({ status: 204 });
    }
    if (path === '/api/me')
      return route.fulfill({
        json: { user: { id: 'user', role: 'client', login: 'coffee', full_name: 'Анна' }, companies: [] },
      });
    if (path === '/api/products' || path.endsWith('/history')) return route.fulfill({ json: [] });
    return route.fulfill({
      json: {
        id: 'example',
        number: 1,
        version: 1,
        status: 'signing',
        company: { id: 'company', inn: '0000100012', short_name: 'Тёплое зерно' },
        product: 'working_capital',
        purpose: 'purchase',
        amount_kop: 250000000,
        term_months: 24,
        representative_role: 'director',
        pd_consent: true,
        bki_consent: true,
        allowed_next: ['issued'],
        reasons: [],
        documents: [],
        created_at: '2026-10-26T12:00:00Z',
        updated_at: '2026-10-26T12:00:00Z',
      },
    });
  });
  await page.goto('/client/applications/example');
}

for (const width of [1440, 375]) {
  test(`уведомления ${width}: код, копирование, непрочитанные, закрытие и доступность`, async ({ page, context }) => {
    await page.setViewportSize({ width, height: 900 });
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await cabinet(page);
    const bell = page.getByRole('button', { name: /^Уведомления/ });
    await expect(bell).toHaveAttribute('aria-label', 'Уведомления, непрочитанных: 2');
    await bell.click();
    const panel = page.getByRole('dialog', { name: 'Уведомления', exact: true });
    await expect(panel).toBeVisible();
    const code = panel.getByLabel('Код подписания', { exact: true });
    await expect(code).toHaveText('194111');
    await expect(panel.getByText('Код подписания', { exact: true })).toBeVisible();
    await page.screenshot({
      path: `test-results/notifications-${width}-new.png`,
      fullPage: true,
      animations: 'disabled',
    });
    expect(await code.evaluate((node) => parseFloat(getComputedStyle(node).fontSize))).toBeGreaterThanOrEqual(28);
    await panel.getByRole('button', { name: 'Копировать', exact: true }).click();
    await expect(panel.getByText('Код скопирован', { exact: true })).toHaveText('Код скопирован');
    expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('194111');
    await expect(bell).toHaveAttribute('aria-label', 'Уведомления, непрочитанных: 1');
    await panel.getByRole('button', { name: 'Прочитать все', exact: true }).click();
    await expect(bell).toHaveAttribute('aria-label', 'Уведомления');
    await expect(panel.getByText('Всё прочитано', { exact: true })).toBeVisible();
    expect(await panel.locator('.notification-items').evaluate((node) => node.scrollTop)).toBe(0);
    await page.addScriptTag({ path: require.resolve('axe-core/axe.min.js') });
    const violations = await page.evaluate(async () =>
      (await (window as unknown as { axe: { run: () => Promise<AxeResults> } }).axe.run()).violations.filter(
        (v) => v.impact === 'critical' || v.impact === 'serious',
      ),
    );
    expect(violations).toEqual([]);
    const bounds = (await panel.locator('.notification-list').count())
      ? await panel.locator('.notification-list').boundingBox()
      : await panel.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(width);
    if (width === 375) expect(bounds!.y + bounds!.height).toBeGreaterThanOrEqual(898);
    await panel.locator('.notification-items').evaluate((node) => {
      node.scrollTop = 0;
    });
    await page.screenshot({ path: `test-results/notifications-${width}.png`, fullPage: true, animations: 'disabled' });
    await page.keyboard.press('Escape');
    await expect(panel).toBeHidden();
    await expect(bell).toBeFocused();
    await bell.click();
    await panel.getByRole('button', { name: 'Закрыть уведомления' }).click();
    await expect(panel).toBeHidden();
    if (width === 1440) {
      await bell.click();
      await page.getByRole('heading', { name: 'Мои заявки', exact: true }).click();
      await expect(panel).toBeHidden();
    }
  });
}

test('пустая панель и открытие из формы подписания', async ({ page }) => {
  await cabinet(page, true);
  await page.evaluate(() => window.dispatchEvent(new Event('open-notifications')));
  const panel = page.getByRole('dialog', { name: 'Уведомления', exact: true });
  await expect(panel.getByText('Пока нет уведомлений')).toBeVisible();
  await expect(panel.getByRole('button', { name: 'Прочитать все' })).toHaveCount(0);
});
