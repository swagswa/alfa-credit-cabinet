import { expect, test } from '@playwright/test';
import { acceptAndSign, apply, state, upload } from './cabinet';

const coffee = {
  inn: '0000100012',
  goal: 'Закупка товара и сырья',
  product: 'Оборотный кредит',
  amount: '2500000',
  term: '24',
  revenue: '1200000',
};

test.describe('кофейня', () => {
  test.use({ storageState: state('coffee') });

  test('одобрение на запрошенных условиях и выдача, банк видит статусы без перезагрузки', async ({ page, browser }) => {
    const bank = await (await browser.newContext({ storageState: state('manager') })).newPage();
    await bank.goto('/bank');
    await apply(page, { ...coffee, documents: ['Паспорт'] });
    await expect(page.getByRole('heading', { name: 'Условия финансирования' })).toBeVisible();
    await expect(page.getByText(/19% годовых\. Платёж: 126.021/)).toBeVisible();

    // очередь банка открыта до подачи и не перезагружается, статусы приходят по sse
    const number = (await page.getByRole('heading', { name: /^Заявка № \d+$/ }).textContent())!.replace(/\D/g, '');
    const row = bank.getByRole('listitem').filter({ hasText: `№ ${number}: ООО «Тёплое зерно»` });
    await expect(row).toContainText('Одобрено');
    await acceptAndSign(page);
    await expect(row).toContainText('Деньги выданы');
  });

  test('без паспорта: банк просит документ, клиент догружает и получает одобрение', async ({ page }) => {
    await apply(page, { ...coffee, documents: [] });
    await expect(page.getByText('Не хватает документа: паспорт руководителя')).toBeVisible();
    await expect(page.getByRole('group', { name: 'Паспорт', exact: true })).toBeVisible();
    await upload(page, ['Паспорт']);
    await page.getByRole('button', { name: 'Отправить сведения банку' }).click();
    await expect(page.getByRole('heading', { name: 'Условия финансирования' })).toBeVisible();
  });
});

test.describe('логистика', () => {
  test.use({ storageState: state('logistics') });

  test('встречное предложение, принятие и выдача', async ({ page }) => {
    await apply(page, {
      inn: '000020001010',
      goal: 'Оборудование и транспорт',
      product: 'Кредит на оборудование',
      amount: '6000000',
      term: '36',
      revenue: '2000000',
      equipmentCost: '6700000',
      documents: ['Паспорт', 'Финансовая отчётность', 'Договор поставки'],
    });
    await expect(page.getByRole('heading', { name: 'Предлагаем другие условия' })).toBeVisible();
    await expect(page.getByRole('row', { name: 'Сумма 6 000 000,00 ₽ 3 900 000,00 ₽' })).toBeVisible();
    await expect(page.getByRole('row', { name: 'Ставка в год 17% 20,5%' })).toBeVisible();
    await acceptAndSign(page);
  });
});
