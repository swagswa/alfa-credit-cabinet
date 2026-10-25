import { expect, test, type Page } from '@playwright/test';
import { parseCheckEvent, updateCheckProgress } from '../src/api/checkProgress';
import { createRequire } from 'node:module';
import type { AxeResults } from 'axe-core';

const require = createRequire(import.meta.url);
const original = {
  id: 'example',
  number: 1,
  version: 2,
  status: 'checking',
  company: { id: 'company', inn: '0000100012', short_name: 'Тестовая компания' },
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
  checks: [] as Record<string, unknown>[],
  created_at: '2026-10-25T12:00:00Z',
  updated_at: '2026-10-25T12:00:00Z',
};
declare global {
  interface Window {
    checkStreams: Array<EventTarget & { closed: boolean; onopen: (() => void) | null; onerror: (() => void) | null }>;
  }
}
async function cabinet(page: Page, bank = false, initial = original) {
  const state = { card: { ...initial }, reads: 0 };
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.addInitScript(() => {
    window.checkStreams = [];
    class Stream extends EventTarget {
      closed = false;
      onopen: (() => void) | null = null;
      onerror: (() => void) | null = null;
      constructor() {
        super();
        window.checkStreams.push(this);
        queueMicrotask(() => {
          if (!this.closed) this.onopen?.();
        });
      }
      close() {
        this.closed = true;
      }
    }
    window.EventSource = Stream as unknown as typeof EventSource;
  });
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (path === '/api/notifications') return route.fulfill({ json: [] });
    if (path.startsWith('/api/applications/')) state.reads++;
    const json =
      path === '/api/me'
        ? {
            user: { id: 'user', role: bank ? 'manager' : 'client', login: 'test', full_name: 'Тест' },
            companies: [],
          }
        : path === '/api/products' || path.endsWith('/history')
          ? []
          : path === '/api/applications'
            ? { items: [state.card], total: 1 }
            : bank
              ? state.card
              : { ...state.card, checks: undefined };
    return route.fulfill({ json });
  });
  await page.goto(`${bank ? '/bank' : '/client'}/applications/example`);
  await expect(page.getByRole('region', { name: 'Ход проверок', exact: true })).toBeVisible();
  return state;
}
async function emit(page: Page, data: unknown, type = 'check') {
  await page.evaluate(
    ({ data, type }) => {
      for (const stream of window.checkStreams.filter((s) => !s.closed)) {
        stream.dispatchEvent(new MessageEvent(type, { data: typeof data === 'string' ? data : JSON.stringify(data) }));
      }
    },
    { data, type },
  );
}
const event = (changes: Record<string, unknown> = {}) => ({
  application_id: 'example',
  source: 'egrul',
  version: 2,
  attempt: 1,
  attempts: 5,
  ...changes,
});

test('парсер и слияние защищают от неверных, старых и повторных событий', () => {
  for (const data of [
    '{',
    'null',
    JSON.stringify(event({ source: 'unknown' })),
    JSON.stringify(event({ attempt: 6 })),
    JSON.stringify(event({ version: -1 })),
  ])
    expect(parseCheckEvent(data)).toBeUndefined();
  const third = parseCheckEvent(JSON.stringify(event({ attempt: 3 })))!;
  const state = updateCheckProgress({ applicationId: '', version: 0, items: {} }, third);
  expect(updateCheckProgress(state, { ...third, attempt: 1 })).toEqual(state);
  const done = updateCheckProgress(state, { ...third, check_status: 'unavailable' });
  expect(updateCheckProgress(done, third)).toEqual(done);
  expect(updateCheckProgress(done, { ...third, version: 1 })).toEqual(done);
});

for (const bank of [false, true]) {
  test(`${bank ? 'банк' : 'клиент'}: повторные попытки и результат появляются без перезагрузки`, async ({ page }) => {
    const state = await cabinet(page, bank);
    const map = page.getByRole('region', { name: 'Ход проверок', exact: true });
    const first = map.getByRole('listitem').first();
    await expect(map.getByRole('listitem')).toHaveCount(4);
    await emit(page, event({ attempt: 3, error: 'egrul: upstream 503' }));
    await expect(first).toContainText('Попытка 3 из 5');
    await expect(first).toContainText('Повторяем запрос');
    await expect(map.getByText('egrul: upstream 503')).toHaveCount(bank ? 1 : 0);
    await emit(page, event({ attempt: 1 }));
    await expect(first).toContainText('Попытка 3 из 5');
    await emit(page, event({ application_id: 'another', attempt: 5 }));
    await emit(page, '{');
    await emit(page, event({ attempt: 5, check_status: 'unavailable' }));
    await expect(first).toContainText('Источник не ответил');
    await expect(first).toContainText('Последняя попытка 5 из 5');
    state.card = { ...state.card, version: 3, status: 'manual_review' };
    await emit(page, { application_id: 'example', version: 3, status: 'manual_review' }, 'status');
    await expect(page.getByText('Рассматривает сотрудник', { exact: true })).toBeVisible();
    if (!bank) await expect(map).toContainText('Банк продолжит рассмотрение');
    await expect.poll(() => state.reads).toBeGreaterThan(1);
    expect(await page.evaluate(() => window.checkStreams.filter((s) => !s.closed).length)).toBe(bank ? 1 : 2);
    await page.setViewportSize({ width: 375, height: 812 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
    await map.screenshot({ path: `test-results/check-progress-${bank ? 'bank' : 'client'}-375.png` });
    await expect(map.getByRole('link')).toHaveCount(bank ? 2 : 0);
    if (bank) {
      const workflow = new URL((await map.getByRole('link', { name: 'Процесс в Temporal ↗' }).getAttribute('href'))!);
      expect(workflow.port).toBe('8233');
      expect(workflow.pathname).toBe('/namespaces/default/workflows/example');
      const trace = new URL((await map.getByRole('link', { name: 'Трейс в Jaeger ↗' }).getAttribute('href'))!);
      expect(trace.port).toBe('16686');
      expect(JSON.parse(trace.searchParams.get('tags')!)).toEqual({ temporalWorkflowID: 'example' });
    }
  });
}

test('четыре ответа завершают проверки, но не означают одобрение', async ({ page }) => {
  await cabinet(page, true);
  const map = page.getByRole('region', { name: 'Ход проверок', exact: true });
  for (const source of ['egrul', 'fns', 'bki', 'internal']) await emit(page, event({ source, check_status: 'ok' }));
  await expect(map).toContainText('Завершено 4 из 4');
  await expect(map.getByText('Сведения получены', { exact: true })).toHaveCount(4);
  await expect(page.getByText('Одобрено', { exact: true })).toHaveCount(0);
});

test('переподключение перечитывает результаты и убирает эфемерные попытки', async ({ page }) => {
  const state = await cabinet(page, true);
  const map = page.getByRole('region', { name: 'Ход проверок', exact: true });
  await emit(page, event({ attempt: 3 }));
  await page.evaluate(() => window.checkStreams.filter((s) => !s.closed).forEach((s) => s.onerror?.()));
  await expect(map).toContainText('Связь прервалась');
  await expect(map.getByRole('listitem').first()).toContainText('Ожидаем соединения');
  state.card = { ...state.card, checks: [{ source: 'egrul', status: 'ok', checked_at: '2026-10-25T12:01:00Z' }] };
  await page.evaluate(() => window.checkStreams.filter((s) => !s.closed).forEach((s) => s.onopen?.()));
  await expect(map).not.toContainText('Связь прервалась');
  await expect(map.getByRole('listitem').first()).toContainText('Сведения получены');
  await expect(map).not.toContainText('Попытка 3 из 5');
  await page.reload();
  await expect(map.getByRole('listitem').first()).toContainText('Сведения получены');
});

test('новый цикл не показывает старые ответы; переходы закрывают поток', async ({ page }) => {
  const state = await cabinet(page, true);
  const map = page.getByRole('region', { name: 'Ход проверок', exact: true });
  await emit(page, event({ check_status: 'ok' }));
  await expect(map.getByRole('listitem').first()).toContainText('Сведения получены');
  state.card = {
    ...state.card,
    version: 4,
    updated_at: '2026-10-25T12:05:00Z',
    checks: [{ source: 'egrul', status: 'ok', checked_at: '2026-10-25T12:01:00Z' }],
  };
  await emit(page, { application_id: 'example', status: 'checking', version: 4 }, 'status');
  await expect(map.getByRole('listitem').first()).toContainText('Ожидаем ответа');
  await emit(page, event({ version: 4, attempt: 1 }));
  await expect(map.getByRole('listitem').first()).toContainText('Попытка 1 из 5');
  await page.getByRole('link', { name: 'Очередь заявок', exact: true }).click();
  state.card = { ...state.card, status: 'manual_review', version: 5 };
  await emit(page, { application_id: 'example', status: 'manual_review', version: 5 }, 'status');
  await expect(page.getByText('Рассматривает сотрудник', { exact: true })).toBeVisible();
  expect(await page.evaluate(() => window.checkStreams.filter((s) => !s.closed).length)).toBe(1);
});

test('карта доступна на мобильном и уважает reduced motion', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'reduce' });
  await cabinet(page, true);
  await emit(page, event({ attempt: 2, error: 'x'.repeat(1000) }));
  const map = page.getByRole('region', { name: 'Ход проверок', exact: true });
  await expect(map.locator('.check-retrying .check-indicator > span')).toHaveCSS('animation-name', 'none');
  for (const width of [375, 768, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  }
  await page.addScriptTag({ path: require.resolve('axe-core/axe.min.js') });
  const result = await page.evaluate(async () => {
    const axe = (window as unknown as { axe: { run: (node: Element) => Promise<AxeResults> } }).axe;
    return axe.run(document.querySelector('.check-progress')!);
  });
  expect(result.violations.filter((v) => ['serious', 'critical'].includes(v.impact ?? ''))).toEqual([]);
});
