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
