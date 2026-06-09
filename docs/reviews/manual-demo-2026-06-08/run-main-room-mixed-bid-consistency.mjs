import { chromium } from '@playwright/test';
import fs from 'node:fs/promises';
import path from 'node:path';

const apiBase = 'http://127.0.0.1:18080';
const h5Base = 'http://127.0.0.1:5276';
const pcBase = 'http://127.0.0.1:5277';
const auctionID = 'auc_live';
const roomID = 'room_main';
const outDir = 'docs/reviews/manual-demo-2026-06-08/evidence/main-room-mixed-bid-consistency';

async function api(pathname, options = {}, cookie = '') {
  const headers = { ...(options.headers || {}) };
  if (cookie) headers.Cookie = cookie;
  const response = await fetch(`${apiBase}${pathname}`, { ...options, headers });
  const text = await response.text();
  let body = null;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    body = text;
  }
  return { response, body, text };
}

async function login(account) {
  const { response, body, text } = await api('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ account })
  });
  if (!response.ok) throw new Error(`login ${account} failed: ${text}`);
  const setCookie = response.headers.get('set-cookie');
  if (!setCookie) throw new Error(`login ${account} missing cookie`);
  return { cookie: setCookie.split(';')[0], user: body.user };
}

async function getTruth(userCookie) {
  const [auction, leaderboard, bids, orders] = await Promise.all([
    api(`/api/auctions/${auctionID}`, {}, userCookie),
    api(`/api/auctions/${auctionID}/leaderboard?limit=10`, {}, userCookie),
    api(`/api/users/me/bids?auction_id=${auctionID}&limit=20`, {}, userCookie),
    api(`/api/users/me/orders?auction_id=${auctionID}&limit=20`, {}, userCookie)
  ]);
  for (const result of [auction, leaderboard, bids, orders]) {
    if (!result.response.ok) throw new Error(`truth API failed: ${result.response.status} ${result.text}`);
  }
  return { auction: auction.body, leaderboard: leaderboard.body, bids: bids.body, orders: orders.body };
}

async function waitForTruth(userCookie, expected) {
  let last = null;
  for (let i = 0; i < 100; i += 1) {
    last = await getTruth(userCookie);
    const top = last.leaderboard.entries?.[0];
    if (
      last.auction.current_price_cents === expected.price &&
      last.auction.current_winner_id === expected.winner &&
      last.leaderboard.current_price_cents === expected.price &&
      last.leaderboard.current_winner_id === expected.winner &&
      top?.amount_cents === expected.price &&
      top?.user_id === expected.winner &&
      last.leaderboard.seq >= (expected.minSeq ?? 0)
    ) {
      return last;
    }
    await new Promise((resolve) => setTimeout(resolve, 120));
  }
  throw new Error(`truth did not converge to ${JSON.stringify(expected)}: ${JSON.stringify(last)}`);
}

async function resetFromPC(page) {
  await page.getByRole('button', { name: '重置演示环境' }).click();
  await page.getByRole('button', { name: '确认重置' }).click();
  await page.getByTestId('pc-workbench-status').getByText('演示环境已重置').waitFor({ timeout: 30000 });
  await page.getByTestId('auction-control-summary').getByText('¥350.00').first().waitFor({ timeout: 12000 });
}

async function collectActionResponses(page, action, matchURL) {
  const responses = [];
  const onResponse = async (response) => {
    if (!matchURL(response.url())) return;
    let body = null;
    let text = '';
    try {
      text = await response.text();
      body = text ? JSON.parse(text) : null;
    } catch {
      body = text;
    }
    responses.push({
      url: response.url(),
      status: response.status(),
      ok: response.ok(),
      body
    });
  };
  page.on('response', onResponse);
  try {
    const result = await action();
    await new Promise((resolve) => setTimeout(resolve, 250));
    return { result, responses };
  } catch (error) {
    await new Promise((resolve) => setTimeout(resolve, 250));
    error.actionResponses = responses;
    throw error;
  } finally {
    page.off('response', onResponse);
  }
}

async function assertH5(page, expected) {
  await page.getByLabel('auction-state').getByRole('heading', { name: expected.priceText }).waitFor({ timeout: 10000 });
  await page.getByTestId('floating-auction-price').waitFor({ timeout: 10000 });
  const floating = await page.getByTestId('floating-auction-price').textContent();
  const race = await page.getByTestId('race-board').textContent();
  const rank = await page.getByTestId('rank-strip').textContent();
  const cta = await page.getByTestId('bid-cta').textContent();
  const disabled = await page.getByTestId('bid-cta').isDisabled();
  if (!floating?.includes(expected.priceText)) {
    throw new Error(`floating product card does not show ${expected.priceText}: ${floating}`);
  }
  if (expected.racePrice !== false && !race?.includes(expected.priceText)) {
    throw new Error(`race board does not show ${expected.priceText}: ${race}`);
  }
  if (expected.rankPattern && !expected.rankPattern.test(`${rank}${race}`)) {
    throw new Error(`rank/race does not match ${expected.rankPattern}: rank=${rank} race=${race}`);
  }
  if (expected.ctaPattern && !expected.ctaPattern.test(cta || '')) {
    throw new Error(`cta does not match ${expected.ctaPattern}: ${cta}`);
  }
  if (expected.ctaDisabled != null && disabled !== expected.ctaDisabled) {
    throw new Error(`cta disabled=${disabled}, want ${expected.ctaDisabled}`);
  }
  return { floating, race, rank, cta, disabled };
}

async function assertPC(page, expected) {
  try {
    await page.getByTestId('auction-control-summary').getByText(expected.priceText).first().waitFor({ timeout: 12000 });
  } catch (error) {
    error.summary = await page.getByTestId('auction-control-summary').textContent().catch(() => '');
    error.queue = await page.getByTestId('auction-queue').textContent().catch(() => '');
    throw error;
  }
  const summary = await page.getByTestId('auction-control-summary').textContent();
  const queue = await page.getByTestId('auction-queue').textContent();
  if (!summary?.includes(expected.priceText) && !queue?.includes(expected.priceText)) {
    throw new Error(`PC does not show ${expected.priceText}: summary=${summary} queue=${queue}`);
  }
  return { summary, queue };
}

async function clickH5ManualBid(page, priceText, options = {}) {
  return collectActionResponses(
    page,
    async () => {
      const cta = page.getByTestId('bid-cta');
      const before = {
        text: await cta.textContent(),
        disabled: await cta.isDisabled()
      };
      await cta.click();
      if (!options.skipPriceWait) {
        try {
          await page.getByLabel('auction-state').getByRole('heading', { name: priceText }).waitFor({ timeout: 10000 });
        } catch (error) {
          error.ctaBefore = before;
          error.ctaAfter = {
            text: await cta.textContent().catch(() => ''),
            disabled: await cta.isDisabled().catch(() => null)
          };
          error.auctionText = await page.getByLabel('auction-state').textContent().catch(() => '');
          error.raceText = await page.getByTestId('race-board').textContent().catch(() => '');
          throw error;
        }
      }
      return {
        before,
        after: {
          text: await cta.textContent().catch(() => ''),
          disabled: await cta.isDisabled().catch(() => null)
        }
      };
    },
    (url) => url.includes(`/api/auctions/${auctionID}/bids`)
  );
}

async function assertH5Sold(page, expected) {
  await page.getByText(expected.priceText).first().waitFor({ timeout: 12000 });
  const body = await page.locator('body').textContent();
  if (!body?.includes(expected.priceText)) {
    throw new Error(`H5 sold view does not show ${expected.priceText}: ${body}`);
  }
  if (expected.pattern && !expected.pattern.test(body)) {
    throw new Error(`H5 sold view does not match ${expected.pattern}: ${body}`);
  }
  return { body };
}

async function setH5MaxBid(page, targetText) {
  return collectActionResponses(
    page,
    async () => {
      await page.getByLabel('bid-dock-shortcuts').getByRole('button', { name: '自动加价' }).click();
      const sheet = page.getByTestId('bottom-sheet');
      await sheet.getByTestId('max-bid-sheet').waitFor({ timeout: 8000 });
      for (let i = 0; i < 8; i += 1) {
        const text = await sheet.getByLabel('max-bid-amount').textContent();
        if (text?.includes(targetText)) break;
        await sheet.getByLabel('increase-max-bid').click();
      }
      const finalText = await sheet.getByLabel('max-bid-amount').textContent();
      if (!finalText?.includes(targetText)) {
        throw new Error(`max bid amount did not reach ${targetText}: ${finalText}`);
      }
      await sheet.getByRole('button', { name: /设置自动加价|更新自动加价/ }).click();
      await sheet.getByText(/自动加价已生效|仅自己可见/).waitFor({ timeout: 10000 });
      await page.getByTestId('bottom-sheet-backdrop').click({ position: { x: 8, y: 8 } });
      return { finalText };
    },
    (url) => url.includes(`/api/auctions/${auctionID}/max-bid-intent`)
  );
}

async function pcSetRivalMaxBid(page) {
  const seen = [];
  page.on('response', async (response) => {
    if (!response.url().includes(`/api/demo/auctions/${auctionID}/competing-bid`)) return;
    try {
      seen.push({ status: response.status(), body: await response.json() });
    } catch {
      seen.push({ status: response.status(), body: await response.text() });
    }
  });
  await page.getByTestId('demo-driver').getByRole('button', { name: '打开场景演示' }).click();
  await page.getByLabel('rival-max-bid-yuan').fill('550');
  await page.getByRole('button', { name: '设置对手自动加价' }).click();
  for (let i = 0; i < 50; i += 1) {
    const match = seen.find((item) => item.body?.intent?.user_id === 'user_2' && item.body?.intent?.status === 'ACTIVE');
    if (match) {
      return match.body;
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`rival max bid intent response not observed: ${JSON.stringify(seen)}`);
}

async function pcDemoBid(page, label, expectedPriceText) {
  return collectActionResponses(
    page,
    async () => {
      await page.getByTestId('demo-driver').getByRole('button', { name: '打开场景演示' }).click();
      await page.getByRole('button', { name: label }).click();
      await page.keyboard.press('Escape');
      await page.getByTestId('auction-control-summary').getByText(expectedPriceText).first().waitFor({ timeout: 12000 });
      return { label };
    },
    (url) => url.includes(`/api/demo/auctions/${auctionID}/competing-bid`)
  );
}

async function main() {
  await fs.mkdir(outDir, { recursive: true });
  const user = await login('user');

  const browser = await chromium.launch({ headless: true });
  const h5Context = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true });
  const pcContext = await browser.newContext({ viewport: { width: 1440, height: 920 } });
  const h5 = await h5Context.newPage();
  const pc = await pcContext.newPage();
  const evidence = [];
  const writeEvidence = async (extra = {}) => {
    await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({ evidence, ...extra }, null, 2));
  };

  await pc.goto(`${pcBase}/`, { waitUntil: 'networkidle' });
  await resetFromPC(pc);
  await h5.goto(`${h5Base}/rooms/${roomID}`, { waitUntil: 'networkidle' });
  await h5.getByTestId('floating-product-card').click();
  await assertH5(h5, { priceText: '¥350.00', racePrice: false, rankPattern: /等你第一手|下一口 ¥400\.00/, ctaPattern: /400\.00/, ctaDisabled: false });
  await assertPC(pc, { priceText: '¥350.00' });
  await h5.screenshot({ path: path.join(outDir, '01-initial-h5.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '01-initial-pc.png'), fullPage: true });

  const h5Manual400 = await clickH5ManualBid(h5, '¥400.00');
  const h5After400 = await waitForTruth(user.cookie, { price: 40000, winner: 'user_1', minSeq: 1 });
  evidence.push({ step: 'h5_manual_400', action: h5Manual400, truth: h5After400, h5: await assertH5(h5, { priceText: '¥400.00', rankPattern: /#1|领先|第 1 名/, ctaPattern: /等待|已领先/, ctaDisabled: true }) });
  await writeEvidence();
  await h5.screenshot({ path: path.join(outDir, '02-h5-400-leading.png'), fullPage: true });

  const pcManualOutbid = await pcDemoBid(pc, '对手压过买家', '¥450.00');
  const afterPcOutbid = await waitForTruth(user.cookie, { price: 45000, winner: 'user_2', minSeq: 2 });
  evidence.push({
    step: 'pc_rival_manual_450',
    action: pcManualOutbid,
    truth: afterPcOutbid,
    h5: await assertH5(h5, { priceText: '¥450.00', rankPattern: /#2|差|第 2 名|被超越/, ctaPattern: /500\.00/, ctaDisabled: false }),
    pc: await assertPC(pc, { priceText: '¥450.00' })
  });
  await writeEvidence();
  await h5.screenshot({ path: path.join(outDir, '04-pc-outbid-h5.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '04-pc-outbid-pc.png'), fullPage: true });

  const rivalMaxBid = await pcSetRivalMaxBid(pc);
  evidence.push({ step: 'pc_set_rival_max_bid_550', rivalMaxBid, truth: await getTruth(user.cookie), pc: await assertPC(pc, { priceText: '¥450.00' }) });
  await writeEvidence();
  await pc.screenshot({ path: path.join(outDir, '05-pc-rival-max-bid.png'), fullPage: true });

  let h5TriggerProxy;
  try {
    h5TriggerProxy = await clickH5ManualBid(h5, '¥550.00');
  } catch (error) {
    const failedTruth = await getTruth(user.cookie).catch((truthError) => ({ error: String(truthError) }));
    evidence.push({
      step: 'h5_500_click_failed_before_proxy_defense',
      error: {
        name: error.name,
        message: error.message,
        actionResponses: error.actionResponses ?? [],
        ctaBefore: error.ctaBefore,
        ctaAfter: error.ctaAfter,
        auctionText: error.auctionText,
        raceText: error.raceText
      },
      truth: failedTruth
    });
    await h5.screenshot({ path: path.join(outDir, '05-failed-h5-click.png'), fullPage: true });
    await pc.screenshot({ path: path.join(outDir, '05-failed-pc-after-h5-click.png'), fullPage: true });
    await writeEvidence({ failed: true });
    throw error;
  }
  const afterRivalAutoDefense = await waitForTruth(user.cookie, { price: 55000, winner: 'user_2', minSeq: 4 });
  const h5AfterProxy = await assertH5(h5, { priceText: '¥550.00', rankPattern: /#2|差|第 2 名|被超越/, ctaPattern: /600\.00|封顶/, ctaDisabled: false });
  let pcAfterProxy;
  try {
    pcAfterProxy = await assertPC(pc, { priceText: '¥550.00' });
  } catch (error) {
    evidence.push({
      step: 'h5_500_triggers_rival_auto_defense_550_pc_failed',
      action: h5TriggerProxy,
      truth: afterRivalAutoDefense,
      h5: h5AfterProxy,
      pcError: {
        name: error.name,
        message: error.message,
        summary: error.summary,
        queue: error.queue
      }
    });
    await h5.screenshot({ path: path.join(outDir, '05-rival-auto-defense-h5.png'), fullPage: true });
    await pc.screenshot({ path: path.join(outDir, '05-failed-pc-after-proxy.png'), fullPage: true });
    await writeEvidence({ failed: true });
    throw error;
  }
  evidence.push({
    step: 'h5_500_triggers_rival_auto_defense_550',
    action: h5TriggerProxy,
    truth: afterRivalAutoDefense,
    h5: h5AfterProxy,
    pc: pcAfterProxy
  });
  await writeEvidence();
  await h5.screenshot({ path: path.join(outDir, '05-rival-auto-defense-h5.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '05-rival-auto-defense-pc.png'), fullPage: true });

  const h5ManualCap = await clickH5ManualBid(h5, '¥600.00', { skipPriceWait: true });
  const afterH5Cap = await waitForTruth(user.cookie, { price: 60000, winner: 'user_1', minSeq: 5 });
  evidence.push({
    step: 'h5_manual_cap_600',
    action: h5ManualCap,
    truth: afterH5Cap,
    h5: await assertH5Sold(h5, { priceText: '¥600.00', pattern: /成交|中拍|订单|支付|已领先/ }),
    pc: await assertPC(pc, { priceText: '¥600.00' })
  });
  await writeEvidence();
  await h5.screenshot({ path: path.join(outDir, '06-h5-cap-h5.png'), fullPage: true });
  await pc.screenshot({ path: path.join(outDir, '06-h5-cap-pc.png'), fullPage: true });

  await fs.writeFile(path.join(outDir, 'result.json'), JSON.stringify({ evidence }, null, 2));

  const finalTruth = evidence.at(-1).truth;
  const accepted = finalTruth.bids.items?.filter((bid) => bid.result === 'ENGINE_ACCEPTED' || bid.result === 'ACCEPTED' || bid.result === 'ACCEPTED_SOLD') ?? [];
  if (finalTruth.auction.current_price_cents !== finalTruth.leaderboard.current_price_cents) {
    throw new Error('auction and leaderboard current price diverged');
  }
  if (finalTruth.auction.current_winner_id !== finalTruth.leaderboard.current_winner_id) {
    throw new Error('auction and leaderboard winner diverged');
  }
  if ((finalTruth.leaderboard.entries?.[0]?.amount_cents ?? 0) !== finalTruth.auction.current_price_cents) {
    throw new Error('leaderboard top amount does not match auction price');
  }
  if (accepted.length === 0) {
    throw new Error('user bid history has no accepted facts');
  }
  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
