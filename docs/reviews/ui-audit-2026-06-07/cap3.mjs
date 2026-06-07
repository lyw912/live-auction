// UI audit pass 3 — capture clean/working states after fresh seed. Review artifact.
import { chromium, devices } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const OUT = path.resolve('docs/reviews/ui-audit-2026-06-07/shots');
fs.mkdirSync(OUT, { recursive: true });
const H5 = 'http://127.0.0.1:5276/';

const struct = (page) => page.evaluate(() => {
  const txt = (el) => (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 80);
  const grab = (sel) => Array.from(document.querySelectorAll(sel)).map(txt).filter(Boolean).slice(0, 40);
  return { headings: grab('h1,h2,h3'), buttons: grab('button,[role=button]'), body: (document.body.innerText||'').replace(/\s+/g,' ').trim().slice(0,1100) };
});
async function tryClick(page, rx, note) {
  try { await page.getByRole('button', { name: rx }).first().click({ timeout: 2500 }); return `btn:${note}`; }
  catch { try { await page.getByText(rx).first().click({ timeout: 2000 }); return `txt:${note}`; } catch { return `MISS:${note}`; } }
}

const r = {};
const b = await chromium.launch({ headless: true });
const ctx = await b.newContext({ ...devices['iPhone 13'], locale: 'zh-CN' });
const p = await ctx.newPage();
await p.goto(H5, { waitUntil: 'domcontentloaded' }).catch(()=>{});
await p.waitForTimeout(6000);
await p.screenshot({ path: path.join(OUT, 'h5-06-live-clean.png') });
r.live = await struct(p);

r.openSheet = await tryClick(p, /看拍品信息|拍品信息|参与竞拍|去出价/, 'sheet');
await p.waitForTimeout(2000);
await p.screenshot({ path: path.join(OUT, 'h5-07-sheet-clean.png') });
r.sheet = await struct(p);

r.bid = await tryClick(p, /^立即出价$|立即出价|确认出价|出价 ¥|我要出价/, 'bid');
await p.waitForTimeout(2500);
await p.screenshot({ path: path.join(OUT, 'h5-08-after-bid.png') });
r.afterBid = await struct(p);

fs.writeFileSync(path.join(OUT, 'structure-pass3.json'), JSON.stringify(r, null, 2));
console.log(JSON.stringify(r, null, 2).slice(0, 4500));
await b.close();
