import { expect, test } from '@playwright/test';

test.beforeEach(async ({ page }) => {
  await page.route('/api/monitor/auctions', async (route) => route.fulfill({
    json: { items: [{ auction_id: 'auc_live', room_id: 'room_1', status: 'ACTIVE', current_price_cents: 45000, seq: 42 }] }
  }));
  await page.route('/api/monitor/anomalies', async (route) => route.fulfill({
    json: { items: [{ id: 1, severity: 'HIGH', type: 'CLOCK_STEP_BACKWARD', message: 'scheduler detected clock step backward' }] }
  }));
  await page.route('/api/monitor/outbox', async (route) => route.fulfill({
    json: { items: [{ outbox_id: 7, aggregate_id: 'auc_live', status: 'PENDING', attempts: 0, lag_ms: 1200 }] }
  }));
  await page.route('/api/monitor/scheduler', async (route) => route.fulfill({
    json: { items: [{ job_id: 'job_1', job_type: 'END_AUCTION', target_id: 'auc_live', status: 'PENDING' }] }
  }));
});

test('PC console renders control tables and real diagnostic API rows', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByText('青瓷手作茶盏')).toBeVisible();
  await expect(page.getByRole('cell', { name: 'ACTIVE' }).first()).toBeVisible();
  await expect(page.getByTestId('diagnostics')).toBeVisible();
  await expect(page.getByText('auc_live')).toBeVisible();

  await page.getByRole('tab', { name: 'Anomalies' }).click();
  await expect(page.getByText('CLOCK_STEP_BACKWARD')).toBeVisible();

  await page.getByRole('tab', { name: 'Outbox' }).click();
  await expect(page.getByText('outbox_id')).toBeVisible();

  await page.getByRole('tab', { name: 'Scheduler' }).click();
  await expect(page.getByText('END_AUCTION')).toBeVisible();
});

test('PC rule form blocks unreachable cap and offers legal caps', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('cap-price-cents').fill('55555');
  await expect(page.getByText('封顶价必须落在起拍价 + N * 加价幅度')).toBeVisible();
  await expect(page.getByRole('button', { name: '保存规则' })).toBeDisabled();
  await expect(page.getByTestId('cap-suggestions').getByRole('button', { name: '¥550.00' })).toBeVisible();
  await expect(page.getByTestId('cap-suggestions').getByRole('button', { name: '¥600.00' })).toBeVisible();

  await page.getByTestId('cap-suggestions').getByRole('button', { name: '¥600.00' }).click();
  await expect(page.getByText('封顶价可达，预计 10 口到顶')).toBeVisible();
  await expect(page.getByRole('button', { name: '保存规则' })).toBeEnabled();
});
