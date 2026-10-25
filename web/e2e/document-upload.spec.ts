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
  await expect(passport.getByText('Загружен', { exact: true })).toBeVisible();
  await expect(power).toContainText('power.pdf');
  await power.getByRole('button', { name: 'Загрузить: Доверенность', exact: true }).click();
  await expect(power.getByText('Загружен', { exact: true })).toBeVisible();
  expect(uploads).toEqual(['passport', 'power_of_attorney']);
  await expect(page.getByRole('button', { name: 'Отправить сведения банку', exact: true })).toBeEnabled();
  await expect(page.locator('#document-upload')).toHaveCount(0);
  await page.setViewportSize({ width: 375, height: 900 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBeTruthy();
  await page.screenshot({ path: 'test-results/document-upload-375.png', fullPage: true });
});
