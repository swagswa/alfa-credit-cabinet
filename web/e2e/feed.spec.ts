import { expect, test } from '@playwright/test';
import { state } from './cabinet';

const catering = 'Господдержка для общепита';
const transport = 'Кредит на грузовики и прицепы';

for (const [user, own, other] of [
  ['coffee', catering, transport],
  ['logistics', transport, catering],
]) {
  test.describe(() => {
    test.use({ storageState: state(user) });
    test(`лента ${user}: подсказки под свою отрасль`, async ({ page }) => {
      await page.goto('/client');
      const tips = page.getByRole('region', { name: 'Подсказки' });
      await expect(tips.getByRole('heading', { name: own })).toBeVisible();
      await expect(tips.getByRole('heading', { name: other })).toHaveCount(0);
    });
  });
}

test.describe(() => {
  test.use({ storageState: state('coffee') });
  test('панель уведомлений закрывается кликом мимо и по Escape', async ({ page }) => {
    await page.goto('/client');
    const bell = page.getByRole('button', { name: /^Уведомления/ });
    const panel = page.getByRole('dialog', { name: 'Уведомления' });
    await bell.click();
    await expect(panel).toBeVisible();
    await panel.getByText('Уведомления', { exact: true }).click();
    await expect(panel).toBeVisible();
    await page.getByRole('heading', { name: 'Обзор', level: 1 }).click();
    await expect(panel).toBeHidden();
    await bell.click();
    await expect(panel).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(panel).toBeHidden();
  });
});
