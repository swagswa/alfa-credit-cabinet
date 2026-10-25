import { expect, type Browser, type Page } from '@playwright/test';

// вход один раз на прогон: на /login лимит попыток с одного ip, повторы тестов в него упираются
export const state = (user: string) => `e2e/.auth/${user}.json`;

export async function login(page: Page, user: string) {
  await page.goto('/login');
  await page.getByLabel('Логин').fill(user);
  await page.getByLabel('Пароль').fill('demo12345');
  await page.getByRole('button', { name: 'Войти' }).click();
  await expect(page.getByRole('button', { name: 'Выйти' })).toBeVisible();
}

// селекты core-components не нативные: открыть поле и кликнуть вариант.
// карточка перерисовывается по sse после загрузки документа и закрывает открытый список, тогда открываем заново
async function choose(page: Page, label: string, option: string) {
  await expect(async () => {
    await page.getByLabel(label).click();
    await page.getByRole('option', { name: option, exact: true }).click({ timeout: 2_000 });
  }).toPass();
}

// png 1x1: бэк смотрит только тип и размер, содержимое скана ему не важно
const scan = {
  name: 'scan.png',
  mimeType: 'image/png',
  buffer: Buffer.from(
    'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==',
    'base64',
  ),
};

type Draft = {
  inn: string;
  goal: string;
  product: string;
  amount: string;
  term: string;
  revenue: string;
  equipmentCost?: string;
  documents: string[];
};

// анкета от ИНН до отправки, на выходе открыта карточка поданной заявки
export async function apply(page: Page, draft: Draft) {
  await page.goto('/client/new');
  await page.getByLabel('ИНН компании').fill(draft.inn);
  await page.getByRole('button', { name: 'Найти компанию' }).click();
  // у unavailable реестр лежит, тогда карточка из данных кабинета
  await expect(page.locator('.company-result')).toBeVisible();
  await choose(page, 'На что нужны деньги', draft.goal);
  await page.getByLabel('Сумма, ₽').fill(draft.amount);
  await page.getByLabel('Срок, месяцев').fill(draft.term);
  await page.getByLabel('Среднемесячная выручка, ₽').fill(draft.revenue);
  await page.getByRole('button', { name: 'Подобрать продукт' }).click();
  await expect(page.getByRole('radio', { name: `Рекомендация: ${draft.product}`, exact: true })).toBeVisible();
  await choose(page, 'Выбранный продукт', draft.product);
  if (draft.equipmentCost) await page.getByLabel('Стоимость оборудования, ₽').fill(draft.equipmentCost);
  await page.getByText('Согласен на обработку персональных данных').click();
  await page.getByText('Согласен на запрос кредитной истории').click();
  await page.getByRole('button', { name: 'Сохранить черновик' }).click();
  // список документов продукта кабинет до подачи не показывает, без них процесс уйдёт в need_info
  await upload(page, draft.documents);
  await page.getByRole('button', { name: 'Отправить в банк' }).click();
  await expect(page.getByText('Заявка отправлена в банк.')).toBeVisible();
}

export async function upload(page: Page, kinds: string[]) {
  for (const kind of kinds) {
    const task = page.getByRole('group', { name: kind, exact: true });
    const additional = page.getByRole('button', { name: 'Загрузить дополнительный документ', exact: true });
    // count не ждёт: после сохранения черновика карточка документов дорисовывается не сразу
    await expect(task.or(additional).first()).toBeVisible();
    if (await task.count()) {
      await task.locator('input[type=file]').setInputFiles(scan);
      await task.getByRole('button', { name: `Загрузить: ${kind}`, exact: true }).click();
    } else {
      if ((await additional.getAttribute('aria-expanded')) === 'false') await additional.click();
      await choose(page, 'Вид документа', kind);
      await page.locator('#document-upload input[type=file]').setInputFiles(scan);
      await page.getByRole('button', { name: 'Загрузить', exact: true }).click();
    }
    await expect(page.getByRole('listitem').filter({ hasText: `scan.png: ${kind}` })).toBeVisible();
  }
}

// код подписи приходит в колокольчик, смс в прототипе нет
export async function acceptAndSign(page: Page) {
  await page.getByRole('button', { name: 'Принять условия' }).click();
  await page.getByRole('button', { name: 'Получить код' }).click();
  // в колокольчике лежат коды прошлых прогонов, берём код своей заявки
  const number = (await page.getByRole('heading', { name: /^Заявка № \d+$/ }).textContent())!.replace(/\D/g, '');
  const sent = await page
    .getByRole('dialog', { name: 'Уведомления' })
    .getByText(new RegExp(`^Код \\d{6} .*заявке №${number}\\.`))
    .first()
    .textContent();
  await page.getByLabel('Код 1 из 6', { exact: true }).pressSequentially(sent!.match(/\d{6}/)![0]);
  await page.getByRole('button', { name: 'Подписать' }).click();
  await expect(page.getByRole('heading', { name: 'Всё готово. Деньги на счёте!' })).toBeVisible();
}

// та же заявка глазами сотрудника, в отдельном контексте: cookie сессии один на браузер
export async function asBank(browser: Browser, user: string, client: Page) {
  const page = await (await browser.newContext({ storageState: state(user) })).newPage();
  await page.goto(client.url().replace('/client/', '/bank/'));
  return page;
}
