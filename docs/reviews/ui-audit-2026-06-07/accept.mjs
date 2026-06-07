// ============================================================================
// UI 验收门禁 (acceptance gate) — 让"做完了"可被机器判定，而非人眼看截图。
// 用法：先 seed 干净数据，再运行：
//   env $(grep -vE '^\s*#|^\s*$' .env | xargs -d '\n') go -C backend run ./cmd/p0smokeseed
//   node docs/reviews/ui-audit-2026-06-07/accept.mjs
// 退出码 0 = 全部门禁通过；非 0 = 有 FAIL。SKIP 表示该状态未能驱动到（需人工确认）。
// 这是零依赖的快速门禁；生产级建议迁到 @playwright/test + @axe-core/playwright +
// toHaveScreenshot（见《实现工艺手册》§验收）。改 UI 时本文件即"红→绿"的待办清单。
// ============================================================================
import { chromium, devices } from '@playwright/test';

const H5 = 'http://127.0.0.1:5276/';
const PC = 'http://127.0.0.1:5277/';

// ---- 禁止出现（在对应状态的可见文本中）。命中即 FAIL。 ----
// [名称, 正则, 适用状态[]]
const FORBIDDEN = [
  ['英文占位名 Smoke/Engine Item', /smoke item|engine item|p0 live backend/i, ['h5-live','h5-sheet','pc-chrome','pc-rules']],
  ['原始 UUID', /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i, ['h5-live','pc-chrome','pc-orders','pc-rules']],
  ['engine_ 前缀房间ID', /engine_[0-9a-f]{6}/i, ['pc-chrome','pc-orders']],
  ['ord_ 前缀订单ID', /\bord_[0-9a-f]{6}/i, ['pc-orders']],
  ['黑话 本地到零', /本地到零/, ['h5-live','h5-sheet']],
  ['黑话 权威价格同步中', /权威价格同步中/, ['h5-sheet','h5-leading']],
  ['黑话 已同步最新竞拍状态', /已同步最新竞拍状态/, ['h5-sheet','h5-leading']],
  ['原始指标 待同步N·最久Nms', /待同步\s*\d+\s*·\s*最久\s*\d+\s*ms/, ['pc-chrome']],
  ['原始指标 最久 大数ms', /最久\s*\d{3,}\s*ms/, ['pc-chrome']],
  ['哨兵值 seconds_since_last_bid', /seconds_since_last_bid/, ['pc-chrome','pc-control']],
  ['英文健康标签', /Redis pending|Settlement (pending|failed)|Paused engines|Lag max/, ['pc-risk']],
  ['合规 出现"拍卖"字样', /拍卖/, ['h5-live','h5-sheet','pc-chrome','pc-control']],
  ['货币单位用"分"(应为¥元)', /\d\s*分(?!钟)/, ['pc-rules']],
  ['领先态错误文案 暂不提交出价', /暂不提交出价/, ['h5-leading']],
];

// ---- 必须出现（或可交互）。缺失即 FAIL。null 正则=特殊布尔检查。 ----
const REQUIRED = [
  ['H5 出价主按钮可见且可点(enabled)', null, ['h5-live']],
  ['H5 当前价/起拍价文案', /当前最高价|起拍价|当前价/, ['h5-live']],
  ['H5 倒计时 剩余 mm:ss', /剩余\s*\d/, ['h5-live']],
  ['H5 领先态文案 你已领先', /你已领先|已领先|当前领先/, ['h5-leading']],
  ['PC 聚合健康灯(引擎·正常/降级/异常)', /引擎[\s\S]{0,8}(正常|降级|异常|健康)/, ['pc-chrome']],
  ['PC 人读单号(非UUID)', /单号|JP\d|AU\d|No\.?\s?\d/, ['pc-orders']],
  ['PC 规则金额用 ¥ 元', /¥\s?\d/, ['pc-rules']],
];

const results = [];
function record(kind, name, target, status, detail) { results.push({ kind, name, target, status, detail: detail || '' }); }
function snip(text, re) { const m = text.match(re); if (!m) return ''; const i = Math.max(0, m.index - 12); return '…' + text.slice(i, m.index + m[0].length + 12).replace(/\s+/g, ' ') + '…'; }

async function grab(page) { return (await page.locator('body').innerText().catch(() => '')).replace(/ /g, ' '); }
async function clickAny(page, re, last = false) {
  try { await page.getByRole('button', { name: re })[last ? 'last' : 'first']().click({ timeout: 2500 }); return true; }
  catch { try { await page.getByText(re).first().click({ timeout: 1800 }); return true; } catch { return false; } }
}

const texts = {};        // target -> visible text
let bidEnabled = null;   // special check

const browser = await chromium.launch({ headless: true });
try {
  // ===== H5 =====
  const h5 = await (await browser.newContext({ ...devices['iPhone 13'], locale: 'zh-CN' })).newPage();
  await h5.goto(H5, { waitUntil: 'domcontentloaded' }).catch(() => {});
  await h5.waitForTimeout(6000);
  texts['h5-live'] = await grab(h5);
  // 出价主按钮是否可见可点（用 hasText 过滤，规避 ¥ 前的不换行空格 nbsp 导致的漏匹配）
  const bidLike = h5.getByRole('button').filter({ hasText: /出一手|出价|立即出价/ });
  const nBtn = await bidLike.count().catch(() => 0);
  bidEnabled = false;
  for (let i = 0; i < nBtn; i++) {
    const b = bidLike.nth(i);
    if ((await b.isVisible().catch(() => false)) && (await b.isEnabled().catch(() => false))) { bidEnabled = true; break; }
  }
  // 打开出价/拍品半屏
  const openBid = h5.locator('[data-testid="floating-product-card"]');
  const openedBidSheet = (await openBid.isVisible().catch(() => false))
    ? await openBid.click({ timeout: 2500 }).then(() => true).catch(() => false)
    : await clickAny(h5, /出一手|出价|看拍品信息|参与竞拍/);
  if (openedBidSheet) {
    await h5.waitForTimeout(1600);
    texts['h5-sheet'] = await grab(h5);
    // 提交一手以进入"领先"态：点击半屏内最后一个可点的出价按钮
    const directSubmit = h5.locator('[data-testid="bid-cta"]');
    if ((await directSubmit.isVisible().catch(() => false)) && (await directSubmit.isEnabled().catch(() => false))) {
      await directSubmit.click().catch(() => {});
    } else {
    const submit = h5.getByRole('button').filter({ hasText: /出一手|出价|确认出价/ });
    const nSub = await submit.count().catch(() => 0);
    for (let i = nSub - 1; i >= 0; i--) {
      const b = submit.nth(i);
      if ((await b.isVisible().catch(() => false)) && (await b.isEnabled().catch(() => false))) { await b.click().catch(() => {}); break; }
    }
    }
    await h5.waitForTimeout(3000);
    texts['h5-leading'] = await grab(h5);
  }

  // ===== PC =====
  const pc = await (await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN' })).newPage();
  await pc.goto(PC, { waitUntil: 'domcontentloaded' }).catch(() => {});
  await pc.waitForTimeout(3500);
  texts['pc-chrome'] = texts['pc-control'] = await grab(pc);
  if (await clickAny(pc, /^订单记录$/)) { await pc.waitForTimeout(1500); texts['pc-orders'] = await grab(pc); }
  if (await clickAny(pc, /^拍品与规则$/)) { await pc.waitForTimeout(1500); texts['pc-rules'] = await grab(pc); }
  if (await clickAny(pc, /^运行监控$/)) { await pc.waitForTimeout(1500); texts['pc-risk'] = await grab(pc); }
} finally {
  await browser.close();
}

// ===== 评估 =====
for (const [name, re, tgts] of FORBIDDEN) for (const t of tgts) {
  if (texts[t] == null) { record('禁止', name, t, 'SKIP', '未捕获该状态'); continue; }
  const hit = re.test(texts[t]);
  record('禁止', name, t, hit ? 'FAIL' : 'PASS', hit ? snip(texts[t], re) : '');
}
for (const [name, re, tgts] of REQUIRED) for (const t of tgts) {
  if (re === null) { // 特殊：H5 出价按钮
    if (bidEnabled == null) record('必需', name, t, 'SKIP', '');
    else record('必需', name, t, bidEnabled ? 'PASS' : 'FAIL', bidEnabled ? '' : '按钮不可见或被禁用');
    continue;
  }
  if (texts[t] == null) { record('必需', name, t, 'SKIP', '未捕获该状态'); continue; }
  const ok = re.test(texts[t]);
  record('必需', name, t, ok ? 'PASS' : 'FAIL', ok ? '' : '未找到');
}

// ===== 报告 =====
const pad = (s, n) => String(s).padEnd(n);
let fail = 0, pass = 0, skip = 0;
console.log('\n================ UI 验收门禁结果 ================');
console.log(pad('状态', 6) + pad('类别', 5) + pad('门禁', 34) + pad('适用', 12) + '说明');
for (const r of results) {
  if (r.status === 'FAIL') fail++; else if (r.status === 'PASS') pass++; else skip++;
  const mark = r.status === 'PASS' ? '✅PASS' : r.status === 'FAIL' ? '❌FAIL' : '⚠️SKIP';
  console.log(pad(mark, 7) + pad(r.kind, 5) + pad(r.name, 34) + pad(r.target, 12) + r.detail);
}
console.log('------------------------------------------------');
console.log(`合计：✅ ${pass}  ❌ ${fail}  ⚠️ ${skip}（SKIP 表示该状态未驱动到，需人工确认或先 seed）`);
console.log('================================================\n');
process.exit(fail > 0 ? 1 : 0);
