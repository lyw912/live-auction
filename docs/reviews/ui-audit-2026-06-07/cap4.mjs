// UI audit pass 4 — drive full atmosphere loop (bid -> outbid -> sold). Review artifact.
import { chromium, devices } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const OUT = path.resolve('docs/reviews/ui-audit-2026-06-07/shots');
const H5 = 'http://127.0.0.1:5276/';
const PC = 'http://127.0.0.1:5277/';
const struct = (page) => page.evaluate(() => {
  const txt = (el) => (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 80);
  return { buttons: Array.from(document.querySelectorAll('button,[role=button]')).map(txt).filter(Boolean).slice(0,18), body: (document.body.innerText||'').replace(/\s+/g,' ').trim().slice(0,900) };
});
async function click(page, rx, opt={}) {
  try { await page.getByRole('button', { name: rx })[opt.last?'last':'first']().click({ timeout: 3000 }); return 'ok'; }
  catch { try { await page.getByText(rx).first().click({ timeout: 2000 }); return 'txt'; } catch { return 'MISS'; } }
}
const r = {};
const b = await chromium.launch({ headless: true });
const h5 = await (await b.newContext({ ...devices['iPhone 13'], locale:'zh-CN' })).newPage();
const pc = await (await b.newContext({ viewport:{width:1440,height:900}, locale:'zh-CN' })).newPage();
await h5.goto(H5, { waitUntil:'domcontentloaded' }).catch(()=>{});
await pc.goto(PC, { waitUntil:'domcontentloaded' }).catch(()=>{});
await h5.waitForTimeout(6000); await pc.waitForTimeout(4000);

// 1) H5 user places a bid: open sheet then tap inner CTA
await click(h5, /出价 ¥|看拍品信息|参与竞拍/);
await h5.waitForTimeout(1500);
r.bid1 = await click(h5, /出价 ¥|确认出价/, {last:true});
await h5.waitForTimeout(2800);
await h5.screenshot({ path: path.join(OUT, 'h5-09-leading.png') });
r.leading = await struct(h5);

// 2) PC host triggers "second buyer outbid"
r.pcOutbid = await click(pc, /第二买家超越/);
await h5.waitForTimeout(3500);
await h5.screenshot({ path: path.join(OUT, 'h5-10-outbid.png') });
r.outbid = await struct(h5);

// 3) PC host triggers cap/sold
r.pcSold = await click(pc, /封顶成交/);
await h5.waitForTimeout(4000);
await h5.screenshot({ path: path.join(OUT, 'h5-11-sold.png') });
r.sold = await struct(h5);
await pc.screenshot({ path: path.join(OUT, 'pc-after-drive.png') });

fs.writeFileSync(path.join(OUT,'structure-pass4.json'), JSON.stringify(r,null,2));
console.log(JSON.stringify(r,null,2).slice(0,4000));
await b.close();
