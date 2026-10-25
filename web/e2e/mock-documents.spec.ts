import { expect, test } from '@playwright/test';

// Run against dev:mock: this exercises its real catalog rather than a replacement fixture.
test('каталог мока передаёт правила документов; старый ответ не роняет черновик', async ({ page }) => {
  page.on('pageerror', (error) => {
    throw error;
  });
  await page.request.post('/api/login', { data: { login: 'coffee', password: 'demo12345' } });
  const response = await page.request.get('/api/products');
  expect(response.ok()).toBeTruthy();
  const products = await response.json();
  for (const product of products) expect(Array.isArray(product.documents)).toBeTruthy();
  const working = products.find((product: { code: string }) => product.code === 'working_capital');
  expect(working.documents).toContainEqual({ kind: 'passport' });
  expect(working.documents).toContainEqual({ kind: 'statement', when: 'no_turnover' });
  expect(working.documents).toContainEqual({ kind: 'financials', when: 'amount_over', amount_over_kop: 300000000 });
  const draft = await page.request.post('/api/applications', {
    headers: { 'Idempotency-Key': crypto.randomUUID() },
    data: {
      inn: '0000100012',
      product: 'working_capital',
      purpose: 'purchase',
      amount_kop: 250000000,
      term_months: 24,
      representative_role: 'proxy',
      bki_consent: false,
      pd_consent: true,
    },
  });
  expect(draft.ok()).toBeTruthy();
  const card = await draft.json();
  await page.goto(`/client/applications/${card.id}`);
  await expect(page.getByRole('heading', { name: 'Документы для заявки', exact: true })).toBeVisible();
  await expect(page.locator('.document-checklist')).toContainText('Доверенность');
  await expect(page.locator('.document-checklist')).toContainText('Паспорт');
  await page.route('**/api/products', (route) =>
    route.fulfill({
      json: products.map((product: Record<string, unknown>) =>
        Object.fromEntries(Object.entries(product).filter(([key]) => key !== 'documents')),
      ),
    }),
  );
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Документы для заявки', exact: true })).toBeVisible();
  await expect(page.locator('.document-checklist')).toContainText('Доверенность');
});
