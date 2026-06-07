import { expect, test } from '@playwright/test';

test('PC console covers live backend host workflow and diagnostics', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('auction-queue').getByText('天然翡翠A货平安扣吊坠')).toBeVisible();
  await expect(page.getByTestId('auction-control-summary')).toBeVisible();
  await expect(page.getByTestId('recent-events')).toBeVisible();
  await page.getByRole('button', { name: '诊断', exact: true }).click();
  await expect(page.getByTestId('diagnostics')).toBeVisible();
  await page.getByRole('button', { name: '竞拍', exact: true }).click();
  await page.getByLabel('room-selector').selectOption('room_side');
  await expect(page.getByTestId('auction-queue').getByText('和田玉福牌吊坠')).toBeVisible();
  const activeCarryOver = page.getByTestId('auction-queue').getByRole('button', { name: /冰种翡翠戒面 .*当前直播主拍品/ });
  if (await activeCarryOver.isVisible().catch(() => false)) {
    await activeCarryOver.click();
    await expect(page.getByTestId('auction-control-summary').getByRole('button', { name: '取消' })).toBeEnabled();
    await page.getByLabel('cancel-reason').fill('live pc smoke pre-cleanup');
    await page.getByTestId('auction-control-summary').getByRole('button', { name: '取消' }).click();
    await page.getByRole('dialog', { name: '确认取消竞拍' }).getByRole('button', { name: '确定' }).click();
    await expect(page.getByText('已取消').first()).toBeVisible();
  }

  const suffix = Date.now();
  await page.getByRole('button', { name: '拍品', exact: true }).click();
  await page.getByLabel('item-title').fill(`冰种翡翠戒面 ${suffix}`);
  await page.getByLabel('item-image-url').fill(`http://cdn.local/live-pc-${suffix}.jpg`);
  await page.getByLabel('item-description').fill('附证书，可核验，支持 7 天无理由。');
  await page.getByLabel('start-price-cents').fill('100');
  await page.getByLabel('increment-cents').fill('50');
  await page.getByLabel('cap-price-cents').fill('600');
  await page.getByRole('button', { name: '创建拍品和竞拍' }).click();

  await expect(page.getByText(`冰种翡翠戒面 ${suffix}`)).toBeVisible();
  await page.getByText(`冰种翡翠戒面 ${suffix}`).click();
  await expect(page.getByRole('button', { name: '保存规则' })).toBeEnabled();

  await page.getByLabel('cap-price-cents').fill('650');
  await page.getByRole('button', { name: '保存规则' }).click();
  await expect(page.getByText('规则已保存')).toBeVisible();

  await page.getByRole('button', { name: '竞拍', exact: true }).click();
  await page.getByTestId('auction-queue').getByText(`冰种翡翠戒面 ${suffix}`).click();
  await expect(page.getByTestId('auction-control-summary').getByRole('heading', { name: `冰种翡翠戒面 ${suffix}` })).toBeVisible();
  await expect(page.getByTestId('auction-control-summary')).toBeVisible();
  await page.getByRole('button', { name: '排期', exact: true }).click();
  await expect(page.getByTestId('auction-control-summary').getByRole('button', { name: '开拍' })).toBeEnabled();
  await page.getByTestId('auction-control-summary').getByRole('button', { name: '开拍' }).click();
  await expect(page.getByText('开拍中').first()).toBeVisible();

  await page.getByRole('button', { name: '诊断', exact: true }).click();
  await page.getByRole('tab', { name: '竞拍状态' }).click();
  await expect(page.getByLabel('竞拍状态').getByRole('row', { name: new RegExp(`ACTIVE room_side 冰种翡翠戒面 ${suffix}`) })).toBeVisible();
  await page.getByRole('tab', { name: '推送队列' }).click();
  await expect(page.getByLabel('推送队列').getByText(/outbox:\d+/).first()).toBeVisible();

  await page.getByRole('button', { name: '竞拍', exact: true }).click();
  await page.getByLabel('cancel-reason').fill('live pc smoke cleanup');
  await page.getByTestId('auction-control-summary').getByRole('button', { name: '取消' }).click();
  await page.getByRole('dialog', { name: '确认取消竞拍' }).getByRole('button', { name: '确定' }).click();
  await expect(page.getByText('已取消').first()).toBeVisible();
});
