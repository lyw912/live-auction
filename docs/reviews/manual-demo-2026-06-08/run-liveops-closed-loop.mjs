import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/liveops-closed-loop';
const pcURL = process.env.LIVE_AUCTION_PC_URL || 'http://127.0.0.1:5277';
const h5URL = process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5276/rooms/room_main';

async function shot(page, name, fullPage = true) {
  await page.screenshot({ path: `${outDir}/${name}.png`, fullPage });
}

async function ensureSession(page, account) {
  await page.evaluate(async (nextAccount) => {
    await fetch('/api/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ account: nextAccount })
    });
  }, account);
}

async function clickFirstVisible(locator, timeout = 12_000) {
  await locator.first().waitFor({ state: 'visible', timeout });
  await locator.first().click();
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const browser = await chromium.launch({ headless: true });
  const pcContext = await browser.newContext({ viewport: { width: 1440, height: 950 } });
  const h5Context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true });
  const pc = await pcContext.newPage();
  const h5 = await h5Context.newPage();
  const network = [];
  for (const page of [pc, h5]) {
    page.on('response', (response) => {
      const url = response.url();
      if (url.includes('/api/')) network.push(`${response.status()} ${response.request().method()} ${url}`);
    });
  }

  await pc.goto(pcURL, { waitUntil: 'networkidle' });
  await ensureSession(pc, 'host');
  await pc.reload({ waitUntil: 'networkidle' });
  await pc.getByRole('heading', { name: '开播中控' }).waitFor({ timeout: 15_000 });

  await clickFirstVisible(pc.getByRole('button', { name: '重置演示环境' }));
  await clickFirstVisible(pc.getByRole('button', { name: '确认重置' }));
  await pc.getByText('演示环境已重置').first().waitFor({ timeout: 25_000 });
  await shot(pc, '01-pc-reset-clean');

  await clickFirstVisible(pc.getByTestId('ops-data-entry').getByRole('button', { name: '查看' }));
  const panel = pc.getByTestId('pc-liveops-host-panel');
  await panel.waitFor({ timeout: 15_000 });
  await shot(pc, '02-pc-liveops-default');

  const title = `证书讲解福利 ${Date.now().toString().slice(-4)}`;
  await panel.locator('label').filter({ hasText: '活动名称' }).getByRole('textbox').fill(title);
  await panel.locator('label').filter({ hasText: '权益名称' }).getByRole('textbox').fill('主播优先答疑');
  await panel.locator('label').filter({ hasText: '完成任务数' }).getByRole('spinbutton').fill('2');
  await panel.locator('label').filter({ hasText: '买家端说明' }).getByRole('textbox').fill('完成任意两个互动任务后，可领取本场直播展示权益：主播优先回答你的拍品问题。该权益不抵扣订单金额，也不影响成交排序。');
  await Promise.all([
    pc.waitForResponse((response) => response.url().includes('/api/host/rooms/room_main/liveops') && response.request().method() === 'PATCH' && response.status() < 300),
    clickFirstVisible(panel.getByRole('button', { name: '保存权益活动' }))
  ]);
  await pc.getByText('互动权益已保存').first().waitFor({ timeout: 10_000 });
  await shot(pc, '03-pc-liveops-saved');

  await h5.goto(h5URL, { waitUntil: 'networkidle' });
  await ensureSession(h5, 'user');
  await h5.reload({ waitUntil: 'networkidle' });
  await clickFirstVisible(h5.getByRole('button', { name: '直播互动' }));
  await h5.getByTestId('live-ops-panel').waitFor({ timeout: 15_000 });
  await h5.getByText(title).waitFor({ timeout: 15_000 });
  await shot(h5, '04-h5-liveops-config-visible');

  await h5.getByRole('button', { name: '看拍品' }).click();
  await h5.getByRole('tab', { name: '互动' }).click();
  await h5.getByRole('button', { name: '看榜单' }).click();
  await h5.getByRole('tab', { name: '互动' }).click();
  await h5.getByText(/2\/4 已完成|2\/4/).first().waitFor({ timeout: 10_000 });
  await shot(h5, '05-h5-tasks-completed');

  await Promise.all([
    h5.waitForResponse((response) => response.url().includes('/api/rooms/room_main/liveops/lucky-draw/enter') && response.status() < 300),
    clickFirstVisible(h5.getByRole('button', { name: '领取资格' }))
  ]);
  await h5.getByText('可查看奖励').waitFor({ timeout: 10_000 });
  await shot(h5, '06-h5-entry-claimed');

  await Promise.all([
    h5.waitForResponse((response) => response.url().includes('/api/rooms/room_main/liveops/lucky-draw/open') && response.status() < 300),
    clickFirstVisible(h5.getByRole('button', { name: '查看奖励' }))
  ]);
  await h5.getByLabel('lucky-draw-reward').getByText('主播优先答疑').waitFor({ timeout: 10_000 });
  await shot(h5, '07-h5-reward-opened');

  await Promise.all([
    h5.waitForResponse((response) => response.url().includes('/api/rooms/room_main/liveops/team') && response.status() < 300),
    clickFirstVisible(h5.getByRole('button', { name: /看工艺/ }))
  ]);
  await shot(h5, '08-h5-preference-selected');

  await pc.bringToFront();
  await ensureSession(pc, 'host');
  await pc.reload({ waitUntil: 'networkidle' });
  await clickFirstVisible(pc.getByTestId('ops-data-entry').getByRole('button', { name: '查看' }));
  await pc.getByTestId('pc-liveops-host-panel').getByText('主播优先答疑').first().waitFor({ timeout: 15_000 });
  await pc.getByText('看工艺').first().waitFor({ timeout: 15_000 });
  await shot(pc, '09-pc-liveops-summary-after-h5');

  const hostSummary = await pc.evaluate(async () => {
    const response = await fetch('/api/host/rooms/room_main/liveops');
    return response.json();
  });
  await fs.writeFile(path.join(outDir, 'host-summary.json'), JSON.stringify(hostSummary, null, 2));
  await fs.writeFile(path.join(outDir, 'network.json'), JSON.stringify(network, null, 2));

  await browser.close();
  console.log(JSON.stringify({
    outDir,
    title,
    participantCount: hostSummary.participant_count,
    openedCount: hostSummary.opened_count,
    qualifiedCount: hostSummary.qualified_count,
    preferences: hostSummary.preference_summary
  }, null, 2));
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
