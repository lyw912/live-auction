// UI audit capture - pass 2 (interactions). Review artifact, not product code.
import { chromium, devices } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const OUT = path.resolve('docs/reviews/ui-audit-2026-06-07/shots');
fs.mkdirSync(OUT, { recursive: true });
const H5 = 'http://127.0.0.1:5276/';
const PC = 'http://127.0.0.1:5277/';

const struct = (page) => page.evaluate(() => {
  const txt = (el) => (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 70);
  const grab = (sel) => Array.from(document.querySelectorAll(sel)).map(txt).filter(Boolean).slice(0, 80);
  return { headings: grab('h1,h2,h3,h4'), buttons: grab('button,[role=button]'), bodyText: (document.body.innerText||'').replace(/\s+/g,' ').trim().slice(0,1400) };
});
const report = {};
async function tryClick(page, rx, note) {
  try { await page.getByText(rx).first().click({ timeout: 2500 }); return `clicked:${note}`; }
  catch { try { await page.getByRole('button', { name: rx }).first().click({ timeout: 2000 }); return `clicked-role:${note}`; } catch (e) { return `MISS:${note}`; } }
}

async function run() {
  const browser = await chromium.launch({ headless: true });

  // ===== H5 deep =====
  const h5ctx = await browser.newContext({ ...devices['iPhone 13'], locale: 'zh-CN' });
  const h5 = await h5ctx.newPage();
  await h5.goto(H5, { waitUntil: 'domcontentloaded' }).catch(()=>{});
  await h5.waitForTimeout(4500);
  report.h5_open_sheet = await tryClick(h5, /看拍品信息|拍品信息|看详情/, 'open-sheet');
  await h5.waitForTimeout(1500);
  await h5.screenshot({ path: path.join(OUT, 'h5-02-sheet.png') });
  report.h5_sheet = await struct(h5);
  // try rules tab inside sheet
  report.h5_rules = await tryClick(h5, /拍品与规则|竞拍规则|规则/, 'rules');
  await h5.waitForTimeout(1000);
  await h5.screenshot({ path: path.join(OUT, 'h5-03-rules.png') });
  // try a bid action
  report.h5_bid = await tryClick(h5, /立即出价|我要出价|出价|加价/, 'bid');
  await h5.waitForTimeout(1200);
  await h5.screenshot({ path: path.join(OUT, 'h5-04-bidpanel.png') });
  report.h5_bid_struct = await struct(h5);
  // leaderboard
  report.h5_board = await tryClick(h5, /出价榜|排行|榜单/, 'board');
  await h5.waitForTimeout(1000);
  await h5.screenshot({ path: path.join(OUT, 'h5-05-board.png') });
  await h5ctx.close();

  // ===== PC tabs =====
  const pcctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN' });
  const pc = await pcctx.newPage();
  await pc.goto(PC, { waitUntil: 'domcontentloaded' }).catch(()=>{});
  await pc.waitForTimeout(3500);
  const tabs = [['拍品','products'],['竞拍','auctions'],['风险处理','risk'],['诊断','diag']];
  report.pc_tabs = {};
  for (const [label, slug] of tabs) {
    const r = await tryClick(pc, new RegExp(`^${label}$`), `tab-${slug}`);
    await pc.waitForTimeout(1600);
    await pc.screenshot({ path: path.join(OUT, `pc-tab-${slug}.png`), fullPage: false });
    await pc.screenshot({ path: path.join(OUT, `pc-tab-${slug}-full.png`), fullPage: true });
    report.pc_tabs[slug] = { click: r, ...(await struct(pc)) };
  }
  // try open publish/add product form on products tab
  await tryClick(pc, /^拍品$/, 'back-products');
  await pc.waitForTimeout(1200);
  report.pc_addproduct = await tryClick(pc, /添加商品|发布拍品|新建拍品|新增拍品|上架/, 'add-product');
  await pc.waitForTimeout(1400);
  await pc.screenshot({ path: path.join(OUT, 'pc-publish-form.png'), fullPage: true });
  report.pc_publish = await struct(pc);
  await pcctx.close();

  fs.writeFileSync(path.join(OUT, 'structure-pass2.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report, null, 2).slice(0, 7000));
  await browser.close();
}
run().catch((e) => { console.error(e); process.exit(1); });
