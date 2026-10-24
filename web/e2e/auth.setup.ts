import { test as setup } from '@playwright/test';
import { login, state } from './cabinet';

for (const user of ['coffee', 'logistics', 'manager']) {
  setup(`вход ${user}`, async ({ page }) => {
    await login(page, user);
    await page.context().storageState({ path: state(user) });
  });
}
