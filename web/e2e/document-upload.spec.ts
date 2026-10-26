import { expect, test } from '@playwright/test';

test('паспорт и доверенность загружаются из своих карточек с правильным типом', async ({ page }) => {
  page.on('pageerror', (error) => {
    throw error;
  });
  const documents: Record<string, string>[] = [];
  const uploads: string[] = [];
  const card = {
    id: 'upload',
    number: 1,
    version: 1,
    status: 'need_info',
    product: 'working_capital',
    company: { id: 'company', inn: '0000100012', short_name: 'Компания' },
    representative_role: 'proxy',
    amount_kop: 250000000,
    term_months: 24,
    allowed_next: ['checking'],
    reasons: [],
    documents,
    created_at: '2026-10-25T12:00:00Z',
    updated_at: '2026-10-25T12:00:00Z',
    info_request: { documents: ['passport', 'power_of_attorney'], reasons: [] },
  };
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    if (route.request().method() === 'POST' && path.endsWith('/documents')) {
      const body = route.request().postData()!;
      const kind = /name="kind"\r\n\r\n([^\r]+)/.exec(body)![1];
      const filename = /filename="([^"]+)"/.exec(body)![1];
      uploads.push(kind);
      const document = { id: String(uploads.length), kind, file_name: filename, status: 'uploaded' };
      documents.push(document);
      return route.fulfill({ json: document });
    }
    const json =
      path === '/api/me'
        ? { user: { id: 'user', role: 'client', login: 'test', full_name: 'Тест' }, companies: [] }
        : path === '/api/notifications' || path === '/api/products' || path.endsWith('/history')
          ? []
          : card;
    return route.fulfill({ json });
  });
  await page.goto('/client/applications/upload');
  const passport = page.getByRole('group', { name: 'Паспорт', exact: true });
  const power = page.getByRole('group', { name: 'Доверенность', exact: true });
  await passport
    .locator('input[type=file]')
    .setInputFiles({ name: 'passport.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4 passport') });
  await power
    .locator('input[type=file]')
    .setInputFiles({ name: 'power.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4 power') });
  await passport.getByRole('button', { name: 'Загрузить: Паспорт', exact: true }).click();
  await expect(passport.getByRole('status')).toHaveText('Успешно прикреплён');
  await expect(power).toContainText('power.pdf');
  await power.getByRole('button', { name: 'Загрузить: Доверенность', exact: true }).click();
  await expect(power.getByRole('status')).toHaveText('Успешно прикреплён');
  expect(uploads).toEqual(['passport', 'power_of_attorney']);
  await expect(page.getByRole('button', { name: 'Отправить сведения банку', exact: true })).toBeEnabled();
  await expect(page.locator('#document-upload')).toHaveCount(0);
  await page.setViewportSize({ width: 375, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: 'test-results/document-upload-375.png', fullPage: true });
});

test('скан не сошёлся: прочитанное в форме, ошибки у своих полей по-русски', async ({ page }) => {
  page.on('pageerror', (error) => {
    throw error;
  });
  const statement = {
    inn: '0000100012',
    period_from: '2026-07-01',
    period_to: '2026-09-30',
    opening_balance_kop: 10000,
    turnover_in_kop: 95000,
    turnover_out_kop: 40000,
    closing_balance_kop: 60000,
    monthly_in: [
      { month: '2026-07', amount_kop: 20000 },
      { month: '2026-08', amount_kop: 30000 },
      { month: '2026-09', amount_kop: 40000 },
    ],
  };
  const document = {
    id: 'scan',
    kind: 'statement',
    file_name: 'scan.png',
    status: 'failed',
    error: 'цифры из документа не сходятся, проверьте подсвеченные поля',
    extracted: { statement },
    validation_errors: [
      { field: 'closing_balance_kop', message: 'остаток на начало плюс приход минус расход не равен остатку на конец' },
      { field: 'monthly_in', message: 'сумма по месяцам не равна поступлениям за период' },
    ],
  };
  const card = {
    id: 'app',
    number: 1,
    version: 1,
    status: 'draft',
    product: 'working_capital',
    company: { id: 'company', inn: '0000100012', short_name: 'Компания' },
    representative_role: 'director',
    amount_kop: 250000000,
    term_months: 24,
    allowed_next: ['submitted'],
    reasons: [],
    documents: [document],
    created_at: '2026-10-25T12:00:00Z',
    updated_at: '2026-10-25T12:00:00Z',
  };
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (!path.startsWith('/api/')) return route.continue();
    if (path === '/api/events') return route.fulfill({ contentType: 'text/event-stream', body: ': open\n\n' });
    const json =
      path === '/api/me'
        ? { user: { id: 'user', role: 'client', login: 'test', full_name: 'Тест' }, companies: [] }
        : path === '/api/documents/scan'
          ? document
          : path === '/api/notifications' || path === '/api/products' || path.endsWith('/history')
            ? []
            : card;
    return route.fulfill({ json });
  });
  await page.goto('/client/applications/app');
  await expect(page.getByLabel('Поступления за период, ₽')).toHaveValue('950');
  await expect(page.getByText('остаток на начало плюс приход минус расход не равен остатку на конец')).toBeVisible();
  await expect(
    page.getByText('Поступления по месяцам: сумма по месяцам не равна поступлениям за период'),
  ).toBeVisible();
  await expect(page.getByText('closing_balance_kop')).toHaveCount(0);
  await expect(page.getByText('неверные данные')).toHaveCount(0);
  await page.screenshot({ path: 'test-results/document-failed.png', fullPage: true });
});
