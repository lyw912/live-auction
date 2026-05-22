import { expect, test } from '@playwright/test';

test('H5 disables bid CTA for unsafe states and keeps text inside controls', async ({ page }) => {
  await page.goto('/');
  const unsafeStates = ['领先中', '提交中', '恢复中', '已断开', '已成交', '流拍', '已取消'];
  for (const state of unsafeStates) {
    await page.getByRole('button', { name: state }).click();
    await expect(page.getByTestId('bid-cta')).toBeDisabled();
  }

  await page.getByRole('button', { name: '竞价中' }).click();
  await expect(page.getByTestId('bid-cta')).toBeEnabled();

  await page.getByRole('button', { name: '提交中' }).click();
  await expect(page.getByText('等待服务端确认')).toBeVisible();
  await expect(page.getByText('状态来自服务端事件')).toBeVisible();

  const buttons = await page.locator('button').all();
  for (const button of buttons) {
    const box = await button.boundingBox();
    if (!box) continue;
    expect(box.height).toBeGreaterThan(20);
    expect(box.width).toBeGreaterThan(24);
  }
});

test('H5 recovering and disconnected states show stale marker', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: '恢复中' }).click();
  await expect(page.getByText('状态可能已过期')).toBeVisible();
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
  await page.getByRole('button', { name: '已断开' }).click();
  await expect(page.getByLabel('auction-state').locator('.leader-row strong')).toHaveText('重连中');
  await expect(page.getByTestId('bid-cta')).toBeDisabled();
});
