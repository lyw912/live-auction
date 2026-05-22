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

test('PC rule form blocks invalid extension, fat-finger, and deposit combinations', async ({ page }) => {
  await page.goto('/');

  await page.getByLabel('extend-window-seconds').fill('31');
  await expect(page.getByRole('alert')).toHaveText('延时窗口和每次延时必须在 10 秒到 30 秒之间');
  await expect(page.getByRole('button', { name: '保存规则' })).toBeDisabled();
  await page.getByLabel('extend-window-seconds').fill('10');

  await page.getByLabel('increment-cents').fill('100000');
  await expect(page.getByRole('alert')).toHaveText('高额确认阈值必须大于加价幅度');
  await expect(page.getByRole('button', { name: '保存规则' })).toBeDisabled();
  await page.getByLabel('increment-cents').fill('5000');

  await page.getByLabel('deposit-floor-cents').fill('60000');
  await expect(page.getByRole('alert')).toHaveText('保证金下限不能高于上限');
  await expect(page.getByRole('button', { name: '保存规则' })).toBeDisabled();
  await page.getByLabel('deposit-floor-cents').fill('5000');

  await expect(page.getByRole('button', { name: '保存规则' })).toBeEnabled();
});

test('PC rule save sends backend contract and surfaces backend suggestions', async ({ page }) => {
  let saveCount = 0;
  await page.route('/api/auctions/auc_next/rules', async (route, request) => {
    saveCount += 1;
    const body = request.postDataJSON() as Record<string, unknown>;
    if (saveCount === 1) {
      expect(body.start_price_cents).toBe(10000);
      expect(body.increment_cents).toBe(5000);
      expect(body.cap_price_cents).toBe(60000);
      expect(body.duration_seconds).toBe(600);
      expect(body.extend_window_seconds).toBe(10);
      expect(body.extend_by_seconds).toBe(10);
      expect(body.max_extend_count).toBe(3);
      expect(body.fat_finger_threshold_cents).toBe(100000);
      expect(body.deposit_bps).toBe(1000);
      expect(body.deposit_floor_cents).toBe(5000);
      expect(body.deposit_cap_cents).toBe(50000);
      await route.fulfill({
        json: {
          auction_id: 'auc_next',
          rule_version: 2
        }
      });
      return;
    }
    await route.fulfill({
      status: 400,
      json: {
        code: 'INVALID_AUCTION_RULE_CAP_UNREACHABLE',
        message: 'cap price is unreachable',
        details: {
          suggested_caps: [65000, 70000]
        }
      }
    });
  });

  await page.goto('/');
  await page.getByRole('button', { name: '保存规则' }).click();
  await expect(page.getByText('规则已保存')).toBeVisible();

  await page.getByLabel('cap-price-cents').fill('65000');
  await expect(page.getByRole('button', { name: '保存规则' })).toBeEnabled();
  await page.getByRole('button', { name: '保存规则' }).click();
  await expect(page.getByRole('alert')).toHaveText('后端拒绝：封顶价不可达');
  await expect(page.getByTestId('cap-suggestions').getByRole('button', { name: '¥650.00' })).toBeVisible();
  await expect(page.getByTestId('cap-suggestions').getByRole('button', { name: '¥700.00' })).toBeVisible();
});
