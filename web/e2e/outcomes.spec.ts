import { expect, test, type Page } from '@playwright/test';
import { acceptAndSign, apply, asBank, state, upload } from './cabinet';
import statement from '../../testdata/documents/0000300011_statement_a.truth.json' with { type: 'json' };
import financials from '../../testdata/documents/0000300011_financials_a.truth.json' with { type: 'json' };

const purchase = { goal: 'Закупка товара и сырья', product: 'Оборотный кредит' };
const date = (iso: string) => iso.split('-').reverse().join('.');

// без профиля ai документы skipped, итоги вводим руками из эталона генератора
async function confirmDocuments(page: Page) {
  await page.getByLabel('Начало периода').fill(date(statement.period_from));
  await page.getByLabel('Конец периода').fill(date(statement.period_to));
  await page.getByLabel('Остаток на начало, ₽').fill(String(statement.opening_balance));
  await page.getByLabel('Поступления за период, ₽').fill(String(statement.turnover_in));
  await page.getByLabel('Списания за период, ₽').fill(String(statement.turnover_out));
  await page.getByLabel('Остаток на конец, ₽').fill(String(statement.closing_balance));
  for (const { month, amount } of statement.monthly_in)
    await page.getByLabel(`Поступления за ${month}, ₽`).fill(String(amount));
  await page.getByRole('button', { name: 'Подтвердить выписку' }).click();

  await page.getByLabel('Отчётный год').fill(String(financials.year));
  for (const [line, value] of Object.entries({
    2110: financials.revenue,
    2400: financials.net_profit,
    1400: financials.liabilities_long,
    1500: financials.liabilities_short,
    1600: financials.balance_total,
  })) {
    await page.getByLabel(`(строка ${line})`).fill(String(value));
  }
  await page.getByRole('button', { name: 'Подтвердить отчётность' }).click();
  // статус есть и в карточке запрошенного документа, считаем только список файлов
  await expect(page.getByRole('listitem').filter({ hasText: 'Подтверждён' })).toHaveCount(2);
}

test.describe('it', () => {
  test.use({ storageState: state('it') });

  test('дозапрос выписки и отчётности, андеррайтер даёт встречное, клиент принимает', async ({ page, browser }) => {
    await apply(page, {
      ...purchase,
      inn: '0000300011',
      amount: '8000000',
      term: '12',
      revenue: '3500000',
      documents: ['Паспорт'],
    });
    await expect(
      page.getByText(
        'Не хватает документа: выписка по счёту из другого банка, бухгалтерская отчётность или декларация',
      ),
    ).toBeVisible();
    await upload(page, ['Выписка по счёту', 'Финансовая отчётность']);
    await confirmDocuments(page);
    await page.getByRole('button', { name: 'Отправить сведения банку' }).click();
    await expect(page.getByText('Сотрудник банка рассматривает заявку.')).toBeVisible();

    const bank = await asBank(browser, 'underwriter', page);
    await bank.getByLabel('Сумма решения, ₽').fill('7000000');
    await bank.getByLabel('Комментарий к решению').fill('8 млн много для оборота, даём 7 под поручительство');
    await bank.getByRole('button', { name: 'Сохранить решение' }).click();

    await expect(page.getByRole('heading', { name: 'Предлагаем другие условия' })).toBeVisible();
    await expect(page.getByLabel('7 000 000,00 ₽')).toBeVisible();
    await acceptAndSign(page);
  });
});

test.describe('стройка', () => {
  test.use({ storageState: state('build') });

  test('отказ с понятным объяснением, без кодов', async ({ page }) => {
    await apply(page, {
      ...purchase,
      inn: '0000400016',
      amount: '5000000',
      term: '12',
      revenue: '4000000',
      documents: ['Паспорт', 'Финансовая отчётность'],
    });
    await expect(page.getByRole('heading', { name: 'Банк не одобрил заявку' })).toBeVisible();
    await expect(page.getByText('высокая долговая нагрузка и нестабильная выручка')).toBeVisible();
    await expect(page.getByText('R_GRADE_E')).toHaveCount(0);
  });
});

test.describe('реестр не отвечает', () => {
  test.use({ storageState: state('unavailable') });

  test('после ретраев ручное рассмотрение, менеджер видит причину и может повторить проверки', async ({
    page,
    browser,
    request,
  }) => {
    const egrul = (mode: string) =>
      request.put(`${process.env.E2E_MOCKS_URL ?? 'http://localhost:8081'}/mocks/admin/faults`, {
        data: { source: 'egrul', mode, inn: '0000500010' },
      });
    // пока реестр лежит, кабинет не находит компанию и черновик не создать: на поиск сбой снимаем и сразу возвращаем
    await egrul('ok');
    await page.route('**/api/companies/*', async (route) => {
      const response = await route.fetch();
      await egrul('http_503');
      await route.fulfill({ response });
    });
    await apply(page, {
      ...purchase,
      inn: '0000500010',
      amount: '1500000',
      term: '12',
      revenue: '1000000',
      documents: ['Паспорт'],
    });
    // пять попыток с backoff 1-2-4-8 с, плюс остальной процесс
    await expect(page.getByText('Сотрудник банка рассматривает заявку.')).toBeVisible({ timeout: 60_000 });
    await expect(page.getByText('SF_SOURCE_UNAVAILABLE')).toHaveCount(0);

    const bank = await asBank(browser, 'manager', page);
    await expect(bank.getByText('Причины: SF_SOURCE_UNAVAILABLE')).toBeVisible();
    await expect(bank.getByText('Результат: Источник недоступен')).toBeVisible();
    await expect(bank.getByRole('button', { name: 'Повторить проверки' })).toBeEnabled();
  });
});
