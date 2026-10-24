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
  await expect(page.getByText(`Рекомендация: ${draft.product}`)).toBeVisible();
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
    // банк запросил один документ: селекта нет, вид уже выбран
    if (!(await page.locator('#document-upload h3').filter({ hasText: kind }).count()))
      await choose(page, 'Вид документа', kind);
    await page.locator('#document-upload input[type=file]').setInputFiles(scan);
    await page.getByRole('button', { name: 'Загрузить', exact: true }).click();
    await expect(page.getByRole('listitem').filter({ hasText: `scan.png: ${kind}` })).toBeVisible();
  }
}

// код подписи кабинет показывает сам, смс в прототипе нет
export async function acceptAndSign(page: Page) {
  await page.getByRole('button', { name: 'Принять условия' }).click();
  await page.getByRole('button', { name: 'Получить код' }).click();
  const hint = await page.getByText(/^Код: \d{6}\./).textContent();
  await page.getByLabel('Код подписания').fill(hint!.match(/\d{6}/)![0]);
  await page.getByRole('button', { name: 'Подписать' }).click();
  await expect(page.getByRole('heading', { name: 'Всё готово. Деньги на счёте!' })).toBeVisible();
}

// та же заявка глазами сотрудника, в отдельном контексте: cookie сессии один на браузер
export async function asBank(browser: Browser, user: string, client: Page) {
  const page = await (await browser.newContext({ storageState: state(user) })).newPage();
  await page.goto(client.url().replace('/client/', '/bank/'));
  return page;
}
