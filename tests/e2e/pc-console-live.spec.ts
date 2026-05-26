import { expect, test } from '@playwright/test';

test('PC console covers live backend host workflow and diagnostics', async ({ page }) => {
  await page.goto('/');
  await expect(page.locator('.arco-table').first().getByText('P0 Live Smoke Item')).toBeVisible();
  await expect(page.getByTestId('diagnostics')).toBeVisible();
  await expect(page.getByTestId('auction-control-summary')).toBeVisible();
  await expect(page.getByTestId('recent-events')).toBeVisible();
  await page.getByLabel('room-selector').selectOption('room_side');
  await expect(page.locator('.arco-table').first().getByText('Side Room Smoke Item')).toBeVisible();

  const suffix = Date.now();
  await page.getByLabel('item-title').fill(`Live PC Item ${suffix}`);
  await page.getByLabel('item-image-url').fill(`http://cdn.local/live-pc-${suffix}.jpg`);
  await page.getByLabel('item-description').fill('live backend pc smoke item');
  await page.getByLabel('start-price-cents').fill('10000');
  await page.getByLabel('increment-cents').fill('5000');
  await page.getByLabel('cap-price-cents').fill('60000');
  await page.getByRole('button', { name: '创建拍品和竞拍' }).click();

  await expect(page.getByText(`Live PC Item ${suffix}`)).toBeVisible();
  await page.getByText(`Live PC Item ${suffix}`).click();
  await expect(page.getByRole('button', { name: '保存规则' })).toBeEnabled();

  await page.getByLabel('cap-price-cents').fill('65000');
  await page.getByRole('button', { name: '保存规则' }).click();
  await expect(page.getByText('规则已保存')).toBeVisible();

  await page.getByRole('button', { name: '排期' }).click();
  await expect(page.getByRole('button', { name: '开拍' })).toBeEnabled();
  await page.getByRole('button', { name: '开拍' }).click();
  await expect(page.getByText('ACTIVE').first()).toBeVisible();

  await page.getByRole('tab', { name: 'Auctions' }).click();
  await expect(page.getByLabel('Auctions').getByText('ACTIVE').first()).toBeVisible();
  await page.getByRole('tab', { name: 'Outbox' }).click();
  await expect(page.getByLabel('Outbox').getByText(/outbox:\d+/).first()).toBeVisible();

  await page.getByLabel('cancel-reason').fill('live pc smoke cleanup');
  await page.getByRole('button', { name: '取消' }).click();
  await page.getByRole('dialog', { name: '确认取消竞拍' }).getByRole('button', { name: '确定' }).click();
  await expect(page.getByText('CANCELLED').first()).toBeVisible();
});
