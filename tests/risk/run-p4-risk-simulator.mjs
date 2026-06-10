import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import os from 'node:os';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const backendDir = join(root, 'backend');
const serverBinDir = join(backendDir, 'tmp');
const serverBin = process.platform === 'win32' ? join(serverBinDir, 'p4-risk-server.exe') : join(serverBinDir, 'p4-risk-server');
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 12);
const rawRoot = join(root, process.env.RAW_ROOT || `artifacts/perf/raw/p4-risk-simulator-${stamp}`);
const baseURL = process.env.BASE_URL || 'http://127.0.0.1:18080';
const manageServer = process.env.MANAGE_SERVER === '1';
const scenarioFilter = (process.env.SCENARIOS || '')
  .split(',')
  .map((name) => name.trim())
  .filter(Boolean);

const scenarios = [
  {
    name: 'bid-idempotency-replay-and-conflict',
    expected: 'same bid key replays the original response, different body with same key is rejected',
    run: runBidIdempotencyScenario,
  },
  {
    name: 'host-only-flight-recorder-acl',
    expected: 'user is forbidden and host can read the auction flight recorder',
    run: runFlightRecorderACLScenario,
  },
  {
    name: 'cap-sold-payment-double-click',
    expected: 'cap bid creates one SOLD order and repeated payment with same key is idempotent',
    run: runCapSoldPaymentScenario,
  },
];

const selectedScenarios = scenarioFilter.length > 0
  ? scenarios.filter((scenario) => scenarioFilter.includes(scenario.name))
  : scenarios;
if (selectedScenarios.length !== (scenarioFilter.length || scenarios.length)) {
  const known = new Set(scenarios.map((scenario) => scenario.name));
  const unknown = scenarioFilter.filter((name) => !known.has(name));
  throw new Error(`unknown scenario(s): ${unknown.join(', ')}`);
}

function run(command, args, options = {}) {
  return spawnSync(command, args, {
    cwd: options.cwd || root,
    env: { ...process.env, ...(options.env || {}) },
    encoding: 'utf8',
    shell: false,
    timeout: options.timeout || 60000,
  });
}

function save(name, result) {
  writeFileSync(join(rawRoot, name), `${result.stdout || ''}${result.stderr || ''}`);
}

function output(command, args, options = {}) {
  const result = run(command, args, options);
  return (result.stdout || result.stderr || '').trim();
}

function headers(role, userID) {
  return {
    'Content-Type': 'application/json',
    'X-Mock-Role': role,
    'X-Mock-User-Id': userID,
  };
}

async function request(method, path, { role = 'user', userID = 'user_1', body, idempotencyKey } = {}) {
  const h = headers(role, userID);
  if (idempotencyKey) h['Idempotency-Key'] = idempotencyKey;
  const res = await fetch(`${baseURL}${path}`, {
    method,
    headers: h,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  let json = {};
  if (text) {
    try {
      json = JSON.parse(text);
    } catch {
      json = { raw: text };
    }
  }
  return {
    status: res.status,
    headers: Object.fromEntries(res.headers.entries()),
    body: json,
  };
}

function assert(condition, message, details = {}) {
  if (!condition) {
    const error = new Error(message);
    error.details = details;
    throw error;
  }
}

async function waitReady() {
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    try {
      const res = await fetch(`${baseURL}/readyz`);
      if (res.ok) return;
    } catch {
      // retry until deadline
    }
    await sleep(500);
  }
  throw new Error(`backend did not become ready at ${baseURL}`);
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function seedLoadData(name) {
  const result = run('go', ['run', './cmd/p1loadseed'], { cwd: backendDir, timeout: 120000 });
  save(name, result);
  if (result.status !== 0) throw new Error(`${name} failed`);
}

async function startManagedServer() {
  mkdirSync(serverBinDir, { recursive: true });
  const build = run('go', ['build', '-o', serverBin, './cmd/server'], { cwd: backendDir, timeout: 120000 });
  save('server-build.log', build);
  if (build.status !== 0) throw new Error('build managed server failed');
  const child = spawn(serverBin, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      ALLOW_MOCK_AUTH: 'true',
      HTTP_ADDR: '127.0.0.1:18080',
      ADMISSION_ENABLED: process.env.ADMISSION_ENABLED || 'true',
    },
    shell: false,
    windowsHide: true,
  });
  let stdout = '';
  let stderr = '';
  child.stdout.on('data', (chunk) => {
    stdout += chunk.toString();
    writeFileSync(join(rawRoot, 'server.log'), stdout, { flag: 'w' });
  });
  child.stderr.on('data', (chunk) => {
    stderr += chunk.toString();
    writeFileSync(join(rawRoot, 'server.err.log'), stderr, { flag: 'w' });
  });
  child.on('exit', (code, signal) => {
    writeFileSync(join(rawRoot, 'server-exit.txt'), `code=${code ?? ''}\nsignal=${signal ?? ''}\n`);
  });
  await waitReady();
  return child;
}

async function stopServer(child) {
  if (child && child.exitCode === null) {
    child.kill();
    cleanupLocalPorts();
    await Promise.race([
      new Promise((resolve) => child.once('exit', resolve)),
      sleep(5000),
    ]);
    if (child.exitCode === null) {
      child.kill('SIGKILL');
      await Promise.race([
        new Promise((resolve) => child.once('exit', resolve)),
        sleep(2000),
      ]);
    }
  }
}

function cleanupLocalPorts() {
  if (process.platform !== 'win32') return;
  const script = [
    '$ports = @(18080)',
    'foreach ($port in $ports) {',
    '  Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue |',
    '    Select-Object -ExpandProperty OwningProcess -Unique |',
    '    ForEach-Object { if ($_ -and $_ -ne 0 -and $_ -ne $PID) { Stop-Process -Id $_ -Force -ErrorAction SilentlyContinue } }',
    '}',
  ].join('; ');
  save('port-cleanup.txt', run('powershell.exe', ['-NoProfile', '-Command', script], { timeout: 30000 }));
}

async function runBidIdempotencyScenario() {
  const snapshot = await request('GET', '/api/auctions/auc_live', { userID: 'user_1' });
  assert(snapshot.status === 200, 'snapshot should succeed', snapshot);
  const amount = Number(snapshot.body.current_price_cents) + Number(snapshot.body.increment_cents);
  const clientBidID = 'p4-risk-bid-idempotency-key';
  const body = { client_bid_id: clientBidID, amount_cents: amount, client_seen_seq: 0 };

  const first = await request('POST', '/api/auctions/auc_live/bids', {
    userID: 'user_1',
    idempotencyKey: clientBidID,
    body,
  });
  assert(first.status === 200, 'first bid should get business response', first);
  assert(['ACCEPTED', 'ACCEPTED_EXTENDED', 'ACCEPTED_SOLD', 'REJECTED'].includes(first.body.result), 'first bid should produce a bid result', first);

  const replay = await request('POST', '/api/auctions/auc_live/bids', {
    userID: 'user_1',
    idempotencyKey: clientBidID,
    body,
  });
  assert(replay.status === 200, 'replayed bid should return 200', replay);
  assert(replay.body.result === first.body.result, 'replayed bid should return original result', { first, replay });
  assert(replay.body.seq === first.body.seq, 'replayed bid should return original seq', { first, replay });

  const conflict = await request('POST', '/api/auctions/auc_live/bids', {
    userID: 'user_1',
    idempotencyKey: clientBidID,
    body: { ...body, amount_cents: amount + Number(snapshot.body.increment_cents) },
  });
  assert(conflict.status === 409, 'same idempotency key with different body should conflict', conflict);
  assert(conflict.body.code === 'IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST', 'conflict should expose idempotency code', conflict);

  return {
    checks: [
      'first bid reached real bid endpoint',
      'same key and same body replayed original result',
      'same key and different body rejected with IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST',
    ],
    user_visible_state: {
      first_result: first.body.result,
      first_seq: first.body.seq,
      replay_seq: replay.body.seq,
      conflict_code: conflict.body.code,
    },
  };
}

async function runFlightRecorderACLScenario() {
  const userAttempt = await request('GET', '/api/monitor/auctions/auc_live/flight-recorder', { role: 'user', userID: 'user_1' });
  assert(userAttempt.status === 403, 'non-host should be forbidden from flight recorder', userAttempt);

  const hostAttempt = await request('GET', '/api/monitor/auctions/auc_live/flight-recorder?limit=20&timeline_limit=80', { role: 'host', userID: 'host_1' });
  assert(hostAttempt.status === 200, 'host should read flight recorder', hostAttempt);
  assert(hostAttempt.body.summary?.auction_id === 'auc_live', 'flight recorder should return requested auction', hostAttempt);
  assert(Array.isArray(hostAttempt.body.timeline) && hostAttempt.body.timeline.length > 0, 'flight recorder should include timeline', hostAttempt);

  return {
    checks: [
      'user role is blocked from host-only monitor endpoint',
      'host role receives DB-backed flight recorder timeline',
    ],
    user_visible_state: {
      user_status: userAttempt.status,
      host_status: hostAttempt.status,
      timeline_rows: hostAttempt.body.timeline.length,
    },
  };
}

async function runCapSoldPaymentScenario() {
  const snapshot = await request('GET', '/api/auctions/auc_live', { userID: 'user_2' });
  assert(snapshot.status === 200, 'snapshot should succeed before cap bid', snapshot);
  const cap = Number(snapshot.body.cap_price_cents);
  assert(Number.isFinite(cap) && cap > 0, 'seed auction should have a cap', snapshot);
  const key = 'p4-risk-cap-sold-bid';
  const bid = await request('POST', '/api/auctions/auc_live/bids', {
    userID: 'user_2',
    idempotencyKey: key,
    body: { client_bid_id: key, amount_cents: cap, client_seen_seq: Number(snapshot.body.seq || 0) },
  });
  assert(bid.status === 200, 'cap bid should get business response', bid);
  assert(bid.body.result === 'ACCEPTED_SOLD', 'cap bid should sell auction', bid);

  const orders = await request('GET', '/api/users/me/orders', { userID: 'user_2' });
  assert(orders.status === 200, 'winner should list orders', orders);
  const order = (orders.body.items || []).find((item) => item.auction_id === 'auc_live');
  assert(order, 'winner should see generated order', orders);
  assert(order.order_status === 'ORDER_PENDING', 'generated order should start pending', order);

  const payKey = 'p4-risk-payment-double-click';
  const firstPay = await request('POST', `/api/orders/${order.order_id}/pay-mock`, {
    userID: 'user_2',
    idempotencyKey: payKey,
    body: { confirm: true },
  });
  assert(firstPay.status === 200, 'first payment should succeed', firstPay);
  assert(firstPay.body.order_status === 'PAID', 'first payment should mark order paid', firstPay);

  const secondPay = await request('POST', `/api/orders/${order.order_id}/pay-mock`, {
    userID: 'user_2',
    idempotencyKey: payKey,
    body: { confirm: true },
  });
  assert(secondPay.status === 200, 'same-key payment replay should succeed', secondPay);
  assert(secondPay.body.order_status === 'PAID', 'same-key payment replay should remain paid', secondPay);
  assert(secondPay.body.provider_payment_id === firstPay.body.provider_payment_id, 'same-key payment replay should reuse provider payment id', { firstPay, secondPay });

  return {
    checks: [
      'cap bid sold auction through normal bid API',
      'winner sees exactly generated pending order',
      'same-key payment double click is idempotent',
    ],
    user_visible_state: {
      bid_result: bid.body.result,
      order_id: order.order_id,
      first_payment_status: firstPay.body.order_status,
      second_payment_status: secondPay.body.order_status,
    },
  };
}

function runInvariantCheck(scenarioName) {
  const jsonPath = join(rawRoot, `${scenarioName}-invariants.json`);
  const markdownPath = join(rawRoot, `${scenarioName}-invariants.md`);
  const jsonResult = run('go', ['run', './cmd/invariantcheck', '-auction', 'auc_live', '-format', 'json', '-out', jsonPath, '-max-details', '20'], {
    cwd: backendDir,
    timeout: 60000,
  });
  if (jsonResult.status !== 0) {
    save(`${scenarioName}-invariants.err.log`, jsonResult);
    return { status: 'FAIL', json: existsSync(jsonPath) ? jsonPath : undefined, markdown: undefined, error: (jsonResult.stderr || jsonResult.stdout || '').trim() };
  }
  const mdResult = run('go', ['run', './cmd/invariantcheck', '-auction', 'auc_live', '-format', 'markdown', '-out', markdownPath, '-max-details', '20'], {
    cwd: backendDir,
    timeout: 60000,
  });
  if (mdResult.status !== 0) {
    save(`${scenarioName}-invariants-md.err.log`, mdResult);
    return { status: 'FAIL', json: jsonPath, markdown: markdownPath, error: (mdResult.stderr || mdResult.stdout || '').trim() };
  }
  const report = JSON.parse(readFileSync(jsonPath, 'utf8'));
  return { status: report.status || 'UNKNOWN', summary: report.summary, json: jsonPath, markdown: markdownPath, error: '' };
}

function writeReports(results) {
  const report = {
    generated_at: new Date().toISOString(),
    raw_root: rawRoot,
    base_url: baseURL,
    manage_server: manageServer,
    environment: {
      platform: process.platform,
      arch: process.arch,
      cpu: os.cpus()[0]?.model || 'unknown',
      cpu_count: os.cpus().length,
      ram_bytes: os.totalmem(),
      os_release: os.release(),
      go: output('go', ['version']),
      git_sha: output('git', ['rev-parse', 'HEAD']),
    },
    scenarios: results,
  };
  writeFileSync(join(rawRoot, 'risk-summary.json'), JSON.stringify(report, null, 2));
  const markdown = [
    '# P4 Risk Simulator Compact Report',
    '',
    `- generated_at: ${report.generated_at}`,
    `- raw_root: ${rawRoot}`,
    `- base_url: ${baseURL}`,
    `- manage_server: ${manageServer}`,
    '',
    '| Scenario | Status | Invariants | Expected Outcome | User Visible State | Error |',
    '|---|---:|---:|---|---|---|',
    ...results.map((result) => [
      result.name,
      result.status,
      result.invariants?.status || '',
      result.expected,
      JSON.stringify(result.user_visible_state || {}),
      result.error || '',
    ].map((cell) => String(cell).replace(/\|/g, '/')).join(' | ')).map((row) => `| ${row} |`),
    '',
    'Each scenario resets seed data, exercises real backend APIs, then runs the scoped invariant verifier for `auc_live`.',
    '',
  ].join('\n');
  writeFileSync(join(rawRoot, 'risk-summary.md'), markdown);
}

mkdirSync(rawRoot, { recursive: true });

let server;
const results = [];
try {
  if (manageServer) {
    cleanupLocalPorts();
    server = await startManagedServer();
  } else {
    await waitReady();
  }

  for (const scenario of selectedScenarios) {
    seedLoadData(`${scenario.name}-seed.txt`);
    const startedAt = new Date().toISOString();
    let status = 'PASS';
    let error = '';
    let details = {};
    try {
      details = await scenario.run();
    } catch (err) {
      status = 'FAIL';
      error = err.message;
      details = { ...(details || {}), failure_details: err.details || {} };
    }
    const invariants = runInvariantCheck(scenario.name);
    if (invariants.status === 'FAIL') {
      status = 'FAIL';
      error = error ? `${error}; invariant verifier failed` : 'invariant verifier failed';
    }
    results.push({
      name: scenario.name,
      expected: scenario.expected,
      status,
      started_at: startedAt,
      completed_at: new Date().toISOString(),
      checks: details.checks || [],
      user_visible_state: details.user_visible_state || {},
      invariants,
      error,
    });
    writeReports(results);
    if (status !== 'PASS') {
      throw new Error(`${scenario.name} failed${error ? `: ${error}` : ''}`);
    }
  }
  writeReports(results);
  console.log(`p4 risk simulator complete: ${rawRoot}`);
} finally {
  await stopServer(server);
  if (manageServer) cleanupLocalPorts();
}
