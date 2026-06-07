// UI audit capture script (review artifact, not product code).
// Usage: node docs/reviews/ui-audit-2026-06-07/cap.mjs <pass>
import { chromium, devices } from '@playwright/test';
import fs from 'node:fs';
import path from 'node:path';

const OUT = path.resolve('docs/reviews/ui-audit-2026-06-07/shots');
fs.mkdirSync(OUT, { recursive: true });

const H5 = 'http://127.0.0.1:5276/';
const PC = 'http://127.0.0.1:5277/';

function attachLogs(page, tag, sink) {
  page.on('console', (m) => { if (['error', 'warning'].includes(m.type())) sink.push(`[${tag}][${m.type()}] ${m.text()}`.slice(0, 300)); });
  page.on('pageerror', (e) => sink.push(`[${tag}][pageerror] ${String(e).slice(0, 300)}`));
}

async function dumpStructure(page) {
  return await page.evaluate(() => {
    const txt = (el) => (el.innerText || el.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 60);
    const grab = (sel) => Array.from(document.querySelectorAll(sel)).map(txt).filter(Boolean).slice(0, 60);
    return {
      title: document.title,
      url: location.href,
      headings: grab('h1,h2,h3,h4'),
      buttons: grab('button,[role=button]'),
      tabs: grab('[role=tab],.arco-tabs-header-title'),
      links: grab('a'),
      bodyText: (document.body.innerText || '').replace(/\s+/g, ' ').trim().slice(0, 1500),
    };
  });
}

async function shoot(page, name) {
  await page.screenshot({ path: path.join(OUT, `${name}.png`) });
}

async function run() {
  const browser = await chromium.launch({ headless: true });
  const logs = [];
  const report = {};

  // ---- H5 (mobile) ----
  const h5ctx = await browser.newContext({ ...devices['iPhone 13'], locale: 'zh-CN' });
  const h5 = await h5ctx.newPage();
  attachLogs(h5, 'H5', logs);
  await h5.goto(H5, { waitUntil: 'networkidle' }).catch((e) => logs.push(`[H5][goto] ${e}`));
  await h5.waitForTimeout(4000);
  await shoot(h5, 'h5-01-initial');
  report.h5 = await dumpStructure(h5);

  // ---- PC (desktop) ----
  const pcctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN' });
  const pc = await pcctx.newPage();
  attachLogs(pc, 'PC', logs);
  await pc.goto(PC, { waitUntil: 'networkidle' }).catch((e) => logs.push(`[PC][goto] ${e}`));
  await pc.waitForTimeout(3500);
  await shoot(pc, 'pc-01-landing');
  report.pc = await dumpStructure(pc);

  report.logs = logs;
  fs.writeFileSync(path.join(OUT, 'structure.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report, null, 2).slice(0, 6000));
  await browser.close();
}
run().catch((e) => { console.error(e); process.exit(1); });
