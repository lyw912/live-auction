import { createWriteStream, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import os from 'node:os';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 12);
const rawRoot = join(root, process.env.RAW_ROOT || `artifacts/perf/raw/p3-local-stress-${stamp}`);
const backendDir = join(root, 'backend');
const serverBinDir = join(backendDir, 'tmp');
const serverBin = process.platform === 'win32' ? join(serverBinDir, 'p3-local-server.exe') : join(serverBinDir, 'p3-local-server');
const invariantBin = process.platform === 'win32' ? join(serverBinDir, 'invariantcheck.exe') : join(serverBinDir, 'invariantcheck');
const duration = process.env.DURATION || '5s';
const durationSeconds = (() => {
  const match = /^(\d+(?:\.\d+)?)(ms|s|m)?$/.exec(duration);
  if (!match) return 5;
  const value = Number(match[1]);
  const unit = match[2] || 's';
  if (unit === 'ms') return value / 1000;
  if (unit === 'm') return value * 60;
  return value;
})();
const shortSessionSeconds = String(Math.max(1, Math.min(5, Math.floor(durationSeconds * 0.6))));
const shortTriggerStartDelay = `${Math.max(1, Math.min(5, Math.floor(durationSeconds * 0.2)))}s`;
const vus = process.env.VUS || '2';
const workloadTimeoutMs = Number(process.env.WORKLOAD_TIMEOUT_MS || 60000);
const artifactMode = process.env.P3_ARTIFACT_MODE || 'minimal';
const keepFullArtifacts = artifactMode === 'full';
const profile = process.env.P3_PROFILE || 'downstream-pressure';
const manageServer = process.env.MANAGE_SERVER === '1';
const isolateWorkloads = process.env.ISOLATE_WORKLOADS !== '0';
const baseURL = process.env.BASE_URL || 'http://127.0.0.1:18080';
const resetBetweenWorkloads = process.env.RESET_BETWEEN_WORKLOADS !== '0';
const startDelaySeconds = Number(process.env.START_DELAY_SECONDS || 0);
const metricsSampleSeconds = Number(process.env.METRICS_SAMPLE_SECONDS || 5);
const postWorkloadObserveSeconds = Number(process.env.POST_WORKLOAD_OBSERVE_SECONDS || 0);
const invariantCheckEnabled = process.env.P3_INVARIANT_CHECK !== '0';
const invariantAuctionID = process.env.INVARIANT_AUCTION_ID || process.env.AUCTION_ID || 'auc_live';
const invariantRoomID = process.env.INVARIANT_ROOM_ID || '';
const admissionEnabled = process.env.ADMISSION_ENABLED ?? (profile === 'admission-on' ? 'true' : 'false');
const workloadFilter = (process.env.WORKLOADS || '')
  .split(',')
  .map((name) => name.trim())
  .filter(Boolean);

const pressureEnv = profile === 'downstream-pressure'
  ? {
      ADMISSION_ENABLED: 'false',
      BID_USER_LIMIT_PER_SECOND: process.env.BID_USER_LIMIT_PER_SECOND || '0',
      BID_IP_LIMIT_PER_SECOND: process.env.BID_IP_LIMIT_PER_SECOND || '0',
      BID_AUCTION_LIMIT_PER_SECOND: process.env.BID_AUCTION_LIMIT_PER_SECOND || '0',
      BID_AUCTION_MAX_IN_FLIGHT: process.env.BID_AUCTION_MAX_IN_FLIGHT || '0',
      WS_TICKET_MAX_IN_FLIGHT: process.env.WS_TICKET_MAX_IN_FLIGHT || '0',
      WS_CONNECT_MAX_IN_FLIGHT: process.env.WS_CONNECT_MAX_IN_FLIGHT || '0',
      WS_RETRY_AFTER: process.env.WS_RETRY_AFTER || '1s',
    }
  : { ADMISSION_ENABLED: admissionEnabled };

const workloads = [
  { name: 'preflight', script: 'tests/load/preflight.js', env: { VUS: '1', DURATION: '1s' } },
  { name: 'final-second-bid-burst', script: 'tests/load/final-second-bid-burst.js', env: { VUS: vus, DURATION: duration, GRACEFUL_STOP: '5s' } },
  { name: 'outbox-burst', script: 'tests/load/outbox-burst.js', env: { VUS: vus, DURATION: duration, GRACEFUL_STOP: '5s' } },
  {
    name: 'p3-bid-pressure',
    script: 'tests/load/p3-bid-pressure.js',
    env: {
      RATE: process.env.RATE || '100',
      DURATION: duration,
      PRE_ALLOCATED_VUS: process.env.PRE_ALLOCATED_VUS || '80',
      MAX_VUS: process.env.MAX_VUS || '256',
    },
  },
  { name: 'watcher-fanout', script: 'tests/load/watcher-fanout.js', env: { VUS: vus, DURATION: duration, SESSION_SECONDS: '1.2', SESSION_MS: '1000' } },
  { name: 'slow-consumer', script: 'tests/load/slow-consumer.js', env: { VUS: vus, DURATION: duration, SESSION_SECONDS: '1.2', SESSION_MS: '1000', BLOCK_MS: '5' } },
  { name: 'reconnect-storm', script: 'tests/load/reconnect-storm.js', env: { VUS: vus, DURATION: duration, SESSION_SECONDS: '1.2' } },
  {
    name: 'multi-room-isolation',
    script: 'tests/load/multi-room-isolation.js',
    env: {
      DURATION: duration,
      HOT_BID_VUS: vus,
      COLD_WS_VUS: process.env.COLD_WS_VUS || '1',
      COLD_SESSION_SECONDS: process.env.COLD_SESSION_SECONDS || shortSessionSeconds,
      HOT_SLEEP_SECONDS: process.env.HOT_SLEEP_SECONDS || '0.05',
    },
  },
  { name: 'bid-abuse', script: 'tests/load/bid-abuse.js', env: { VUS: vus, DURATION: duration, GRACEFUL_STOP: '5s' } },
  {
    name: 'p3-admission-calibration',
    script: 'tests/load/p3-admission-calibration.js',
    env: {
      RATE: process.env.RATE || '120',
      DURATION: duration,
      PRE_ALLOCATED_VUS: process.env.PRE_ALLOCATED_VUS || '120',
      MAX_VUS: process.env.MAX_VUS || '400',
      USERS: process.env.USERS || '512',
    },
  },
  {
    name: 'p3-ws-fanout-pressure',
    script: 'tests/load/p3-ws-fanout-pressure.js',
    env: {
      WATCHERS: process.env.WATCHERS || vus,
      DURATION: duration,
      SESSION_SECONDS: process.env.SESSION_SECONDS || '5',
      TRIGGER_RATE: process.env.TRIGGER_RATE || '5',
      TRIGGER_PRE_ALLOCATED_VUS: process.env.TRIGGER_PRE_ALLOCATED_VUS || '10',
      TRIGGER_MAX_VUS: process.env.TRIGGER_MAX_VUS || '50',
      CONNECT_STAGGER_MS: process.env.CONNECT_STAGGER_MS || '0',
      TRIGGER_START_DELAY: process.env.TRIGGER_START_DELAY || '0s',
    },
  },
  {
    name: 'p3-slow-consumer-pressure',
    script: 'tests/load/p3-slow-consumer-pressure.js',
    env: {
      CONSUMERS: process.env.CONSUMERS || vus,
      DURATION: duration,
      SESSION_SECONDS: process.env.SESSION_SECONDS || '5',
      BLOCK_MS: process.env.BLOCK_MS || '150',
      TRIGGER_RATE: process.env.TRIGGER_RATE || '5',
      TRIGGER_PRE_ALLOCATED_VUS: process.env.TRIGGER_PRE_ALLOCATED_VUS || '10',
      TRIGGER_MAX_VUS: process.env.TRIGGER_MAX_VUS || '50',
      CONNECT_STAGGER_MS: process.env.CONNECT_STAGGER_MS || '0',
      TRIGGER_START_DELAY: process.env.TRIGGER_START_DELAY || '0s',
    },
  },
  {
    name: 'p3-ws-connection-storm',
    script: 'tests/load/p3-ws-connection-storm.js',
    env: {
      CONNECTIONS: process.env.CONNECTIONS || process.env.WATCHERS || vus,
      DURATION: duration,
      SESSION_SECONDS: process.env.SESSION_SECONDS || '8',
      CONNECT_STAGGER_MS: process.env.CONNECT_STAGGER_MS || '0',
      TICKET_RETRIES: process.env.TICKET_RETRIES || '3',
      RETRY_SLEEP_MS: process.env.RETRY_SLEEP_MS || '250',
    },
  },
  {
    name: 'p3-healthy-vs-slow-consumer',
    script: 'tests/load/p3-healthy-vs-slow-consumer.js',
    env: {
      HEALTHY_WATCHERS: process.env.HEALTHY_WATCHERS || vus,
      SLOW_CONSUMERS: process.env.SLOW_CONSUMERS || vus,
      DURATION: duration,
      SESSION_SECONDS: process.env.SESSION_SECONDS || shortSessionSeconds,
      BLOCK_MS: process.env.BLOCK_MS || '150',
      TRIGGER_RATE: process.env.TRIGGER_RATE || '20',
      TRIGGER_PRE_ALLOCATED_VUS: process.env.TRIGGER_PRE_ALLOCATED_VUS || '40',
      TRIGGER_MAX_VUS: process.env.TRIGGER_MAX_VUS || '120',
      TRIGGER_START_DELAY: process.env.TRIGGER_START_DELAY || shortTriggerStartDelay,
      CONNECT_STAGGER_MS: process.env.CONNECT_STAGGER_MS || '10',
    },
  },
];

const selectedWorkloads = workloadFilter.length > 0
  ? workloads.filter((workload) => workloadFilter.includes(workload.name))
  : workloads;
if (workloadFilter.length > 0 && selectedWorkloads.length !== workloadFilter.length) {
  const known = new Set(workloads.map((workload) => workload.name));
  const unknown = workloadFilter.filter((name) => !known.has(name));
  throw new Error(`unknown workload(s): ${unknown.join(', ')}`);
}

function run(command, args, options = {}) {
  return spawnSync(command, args, {
    cwd: options.cwd || root,
    env: { ...process.env, ...(options.env || {}) },
    encoding: 'utf8',
    shell: false,
    timeout: options.timeout || workloadTimeoutMs,
  });
}

function runInvariantCheck(workloadName) {
  if (!invariantCheckEnabled) {
    return { status: 'SKIP', json: undefined, markdown: undefined, error: '' };
  }
  const jsonPath = join(rawRoot, `${workloadName}-invariants.json`);
  const markdownPath = join(rawRoot, `${workloadName}-invariants.md`);
  const command = existsSync(invariantBin) ? invariantBin : 'go';
  const args = existsSync(invariantBin)
    ? ['-format', 'json', '-out', jsonPath, '-max-details', '20']
    : ['run', './cmd/invariantcheck', '-format', 'json', '-out', jsonPath, '-max-details', '20'];
  if (invariantAuctionID) args.push('-auction', invariantAuctionID);
  if (invariantRoomID) args.push('-room', invariantRoomID);
  const result = run(command, args, { cwd: backendDir, timeout: 60000 });
  if (result.status === 0) {
    const mdArgs = existsSync(invariantBin)
      ? ['-format', 'markdown', '-out', markdownPath, '-max-details', '20']
      : ['run', './cmd/invariantcheck', '-format', 'markdown', '-out', markdownPath, '-max-details', '20'];
    if (invariantAuctionID) mdArgs.push('-auction', invariantAuctionID);
    if (invariantRoomID) mdArgs.push('-room', invariantRoomID);
    const mdResult = run(command, mdArgs, { cwd: backendDir, timeout: 60000 });
    if (mdResult.status !== 0) {
      return { status: 'FAIL', json: jsonPath, markdown: markdownPath, error: (mdResult.stderr || mdResult.stdout || '').trim() };
    }
    try {
      const report = JSON.parse(readFileSync(jsonPath, 'utf8'));
      return {
        status: report.status || 'UNKNOWN',
        json: jsonPath,
        markdown: markdownPath,
        summary: report.summary,
        error: '',
      };
    } catch (err) {
      return { status: 'FAIL', json: jsonPath, markdown: markdownPath, error: err.message };
    }
  }
  writeFileSync(join(rawRoot, `${workloadName}-invariants.err.log`), `${result.stdout || ''}${result.stderr || ''}`);
  return {
    status: 'FAIL',
    json: existsSync(jsonPath) ? jsonPath : undefined,
    markdown: existsSync(markdownPath) ? markdownPath : undefined,
    error: (result.stderr || result.stdout || '').trim(),
  };
}

function runK6WithSampling(workload, rawPath, logPath) {
  return new Promise((resolve) => {
    const child = spawn('k6', ['run', '--summary-export', rawPath, workload.script], {
      cwd: root,
      env: { ...process.env, BASE_URL: baseURL, ...workload.env },
      shell: false,
      windowsHide: true,
    });
    let stdout = '';
    let stderr = '';
    let settled = false;
    let timedOut = false;
    const samplesPath = join(rawRoot, `${workload.name}-metrics-samples.prom`);
    const activitySamplesPath = join(rawRoot, `${workload.name}-db-activity-samples.txt`);
    const locksSamplesPath = join(rawRoot, `${workload.name}-db-locks-samples.txt`);
    const outboxSamplesPath = join(rawRoot, `${workload.name}-outbox-samples.txt`);
    if (keepFullArtifacts) writeFileSync(samplesPath, '');
    if (keepFullArtifacts) writeFileSync(activitySamplesPath, '');
    if (keepFullArtifacts) writeFileSync(locksSamplesPath, '');
    if (keepFullArtifacts) writeFileSync(outboxSamplesPath, '');

    function appendSample(label) {
      if (!keepFullArtifacts) return;
      const result = run('curl.exe', ['-fsS', `${baseURL}/metrics`], { timeout: 5000 });
      const now = new Date().toISOString();
      const body = `${result.stdout || result.stderr || ''}`.trim();
      writeFileSync(samplesPath, `\n# sample ${label} ${now} status=${result.status ?? ''}\n${body}\n`, { flag: 'a' });
      writeFileSync(activitySamplesPath, `\n# sample ${label} ${now}\n${sqlOutput("SELECT pid, state, wait_event_type, wait_event, now()-query_start AS age, left(query, 180) AS query FROM pg_stat_activity WHERE datname='live_auction' AND (state <> 'idle' OR wait_event_type IS NOT NULL) ORDER BY query_start NULLS LAST LIMIT 30;")}\n`, { flag: 'a' });
      writeFileSync(locksSamplesPath, `\n# sample ${label} ${now}\n${sqlOutput("SELECT locktype, mode, granted, count(*) FROM pg_locks GROUP BY locktype, mode, granted ORDER BY locktype, mode, granted;")}\n`, { flag: 'a' });
      writeFileSync(outboxSamplesPath, `\n# sample ${label} ${now}\n${sqlOutput("SELECT status, count(*) FROM outbox_delivery GROUP BY status ORDER BY status; SELECT event_type, count(*) FROM auction_events GROUP BY event_type ORDER BY event_type;")}\n`, { flag: 'a' });
    }

    child.stdout.on('data', (chunk) => { stdout += chunk.toString(); });
    child.stderr.on('data', (chunk) => { stderr += chunk.toString(); });
    const sampleTimer = metricsSampleSeconds > 0
      ? setInterval(() => appendSample('during'), metricsSampleSeconds * 1000)
      : undefined;
    const timeoutTimer = setTimeout(() => {
      timedOut = true;
      child.kill();
    }, workloadTimeoutMs);

    child.on('error', (error) => {
      if (settled) return;
      settled = true;
      if (sampleTimer) clearInterval(sampleTimer);
      clearTimeout(timeoutTimer);
      writeFileSync(logPath, `${stdout}${stderr}${error.message ? `\n${error.message}\n` : ''}`);
      resolve({ status: null, stdout, stderr, error });
    });
    child.on('exit', (code, signal) => {
      if (settled) return;
      settled = true;
      if (sampleTimer) clearInterval(sampleTimer);
      clearTimeout(timeoutTimer);
      (async () => {
        appendSample('final');
        await observePostWorkload(appendSample);
        writeFileSync(logPath, `${stdout}${stderr}`);
        resolve({
          status: code,
          signal,
          stdout,
          stderr,
          error: timedOut ? { code: 'ETIMEDOUT' } : undefined,
        });
      })();
    });
  });
}

function output(command, args) {
  const result = run(command, args);
  return (result.stdout || result.stderr || '').trim();
}

function save(name, result) {
  writeFileSync(join(rawRoot, name), `${result.stdout || ''}${result.stderr || ''}`);
}

function capture(name, command, args) {
  save(name, run(command, args, { timeout: 10000 }));
}

function captureSQL(name, sql) {
  capture(name, 'docker', ['exec', 'live-auction-postgres', 'psql', '-U', 'live_auction', '-d', 'live_auction', '-c', sql]);
}

function sqlOutput(sql) {
  return output('docker', ['exec', 'live-auction-postgres', 'psql', '-U', 'live_auction', '-d', 'live_auction', '-c', sql]);
}

function parseMetricValue(text, metric, labels = {}) {
  const labelEntries = Object.entries(labels);
  const lines = String(text || '').split(/\r?\n/);
  for (const line of lines) {
    if (!line.startsWith(metric)) continue;
    if (!labelEntries.every(([key, value]) => line.includes(`${key}="${value}"`))) continue;
    const parts = line.trim().split(/\s+/);
    const value = Number(parts[parts.length - 1]);
    return Number.isFinite(value) ? value : 0;
  }
  return 0;
}

function admissionSnapshot(metricsText) {
  return {
    bidAdmissionRejected: parseMetricValue(metricsText, 'auction_bid_request_total', { result: 'admission_rejected' }),
    wsTicketRejected: parseMetricValue(metricsText, 'auction_ws_admission_rejected_total', { stage: 'ticket' }),
    wsConnectRejected: parseMetricValue(metricsText, 'auction_ws_admission_rejected_total', { stage: 'connect' }),
    bidHTTP429: parseMetricValue(metricsText, 'http_request_total', { path: '/api/auctions/{id}/bids', status: '429' }),
    wsHTTP429: parseMetricValue(metricsText, 'http_request_total', { path: '/ws', status: '429' }),
    wsTicketHTTP429: parseMetricValue(metricsText, 'http_request_total', { path: '/api/auth/ws-ticket', status: '429' }),
  };
}

function admissionDelta(before, after) {
  const out = {};
  for (const key of Object.keys(after)) {
    out[key] = after[key] - (before[key] || 0);
  }
  return out;
}

function admissionProof(beforeMetrics, afterMetrics) {
  const beforeEnabled = parseMetricValue(beforeMetrics, 'auction_admission_enabled');
  const afterEnabled = parseMetricValue(afterMetrics, 'auction_admission_enabled');
  const delta = admissionDelta(admissionSnapshot(beforeMetrics), admissionSnapshot(afterMetrics));
  const positiveDeltas = Object.fromEntries(Object.entries(delta).filter(([, value]) => value > 0));
  return {
    expected_disabled: admissionEnabled === 'false',
    enabled_before: beforeEnabled,
    enabled_after: afterEnabled,
    reject_delta: delta,
    polluted: admissionEnabled === 'false' && (beforeEnabled !== 0 || afterEnabled !== 0 || Object.keys(positiveDeltas).length > 0),
    pollution: positiveDeltas,
  };
}

function readK6Summary(rawPath) {
  try {
    return JSON.parse(readFileSync(rawPath, 'utf8'));
  } catch {
    return undefined;
  }
}

function metricRate(summary, name) {
  const metric = summary?.metrics?.[name] || {};
  const values = metric.values || metric;
  const value = values.rate;
  if (!Number.isFinite(Number(value)) && Number.isFinite(Number(values.passes)) && Number.isFinite(Number(values.fails))) {
    const total = Number(values.passes) + Number(values.fails);
    return total > 0 ? Number(values.passes) / total : 0;
  }
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function metricValue(summary, name, key) {
  const metric = summary?.metrics?.[name] || {};
  const values = metric.values || metric;
  const value = values[key];
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function metricCount(summary, name) {
  const metric = summary?.metrics?.[name] || {};
  const values = metric.values || metric;
  const value = values.count ?? values.passes;
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function metricMax(summary, name) {
  const metric = summary?.metrics?.[name] || {};
  const values = metric.values || metric;
  const value = values.max ?? values.value;
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function environmentSignals(summary, logText) {
  const iterations = metricCount(summary, 'iterations');
  const droppedIterations = metricCount(summary, 'dropped_iterations');
  const totalOffered = iterations + droppedIterations;
  const droppedIterationRate = totalOffered > 0 ? droppedIterations / totalOffered : 0;
  const patterns = [
    ['windows_or_accept_refused', /connectex:.*actively refused|No connection could be made because the target machine actively refused/i],
    ['socket_port_exhaustion', /can't assign requested address|Only one usage of each socket address|address already in use/i],
    ['load_generator_timeout', /context deadline exceeded|i\/o timeout|request timeout/i],
    ['k6_vu_ceiling', /Insufficient VUs|reached the max VUs/i],
  ];
  const matched = patterns
    .filter(([, pattern]) => pattern.test(logText || ''))
    .map(([name]) => name);
  if (droppedIterations > 0) matched.push('dropped_iterations');
  return {
    iterations,
    dropped_iterations: droppedIterations,
    dropped_iteration_rate: Number(droppedIterationRate.toFixed(6)),
    vus_max_observed: metricMax(summary, 'vus_max'),
    signals: [...new Set(matched)],
    possible_env_limit: matched.length > 0,
  };
}

function compactK6Summary(summary) {
  if (!summary) return {};
  const metricNames = Object.keys(summary.metrics || {});
  const customCounters = {};
  for (const name of metricNames) {
    if (!name.startsWith('auction_k6_')) continue;
    const metric = summary.metrics[name] || {};
    const values = metric.values || metric;
    customCounters[name] = {
      count: metricCount(summary, name),
      rate: metricRate(summary, name),
      value: Number.isFinite(Number(values.value)) ? Number(values.value) : undefined,
      p95: Number.isFinite(Number(values['p(95)'])) ? Number(values['p(95)']) : undefined,
      p99: Number.isFinite(Number(values['p(99)'])) ? Number(values['p(99)']) : undefined,
      max: Number.isFinite(Number(values.max)) ? Number(values.max) : undefined,
    };
  }
  return {
    iterations: metricCount(summary, 'iterations'),
    dropped_iterations: metricCount(summary, 'dropped_iterations'),
    checks: {
      passes: metricCount(summary, 'checks'),
      rate: metricRate(summary, 'checks'),
    },
    http_reqs: metricCount(summary, 'http_reqs'),
    http_req_failed_rate: metricRate(summary, 'http_req_failed'),
    http_req_duration_ms: {
      p95: metricValue(summary, 'http_req_duration', 'p(95)'),
      p99: metricValue(summary, 'http_req_duration', 'p(99)'),
      p999: metricValue(summary, 'http_req_duration', 'p(99.9)'),
      max: metricMax(summary, 'http_req_duration'),
    },
    ws_sessions: metricCount(summary, 'ws_sessions'),
    ws_session_duration_ms: {
      p95: metricValue(summary, 'ws_session_duration', 'p(95)'),
      p99: metricValue(summary, 'ws_session_duration', 'p(99)'),
      max: metricMax(summary, 'ws_session_duration'),
    },
    customCounters,
  };
}

function compactLog(text) {
  return String(text || '')
    .split(/\r?\n/)
    .filter((line) => /error|fail|warn|timeout|refused|429|RATE_LIMITED|BID_AUCTION_TOO_HOT|dropped|Insufficient VUs|can't assign/i.test(line))
    .slice(-80);
}

function assertAdmissionDisabled(workloadName, beforeMetrics, afterMetrics) {
  if (admissionEnabled !== 'false') return;
  const proof = admissionProof(beforeMetrics, afterMetrics);
  if (proof.enabled_before !== 0 || proof.enabled_after !== 0) {
    throw new Error(`${workloadName} expected auction_admission_enabled 0 before/after, got ${proof.enabled_before}/${proof.enabled_after}`);
  }
  const blocked = Object.entries(proof.reject_delta).filter(([, value]) => value > 0);
  if (blocked.length > 0) {
    throw new Error(`${workloadName} observed admission while ADMISSION_ENABLED=false: ${blocked.map(([k, v]) => `${k}+${v}`).join(', ')}`);
  }
}

function stream(path) {
  writeFileSync(path, '');
  return createWriteStream(path, { flags: 'a' });
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function observePostWorkload(appendSample) {
  if (!keepFullArtifacts || postWorkloadObserveSeconds <= 0) return;
  const intervalSeconds = metricsSampleSeconds > 0 ? metricsSampleSeconds : 5;
  const deadline = Date.now() + postWorkloadObserveSeconds * 1000;
  let sample = 0;
  while (Date.now() < deadline) {
    await sleep(Math.min(intervalSeconds * 1000, Math.max(0, deadline - Date.now())));
    sample++;
    appendSample(`post-${sample}`);
  }
}

async function waitReady() {
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    const result = run('curl.exe', ['-fsS', `${baseURL}/readyz`], { timeout: 5000 });
    if (result.status === 0) return;
    await sleep(500);
  }
  throw new Error(`backend did not become ready at ${baseURL}`);
}

async function startManagedServer() {
  mkdirSync(serverBinDir, { recursive: true });
  save('docker-up.txt', run('docker', ['compose', '-f', 'infra/docker-compose.yml', 'up', '-d', 'postgres', 'redis', 'minio'], { timeout: 120000 }));
  save('goose-up.txt', run('goose', ['-dir', 'backend/migrations', 'postgres', 'postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable', 'up'], { timeout: 120000 }));
  const build = run('go', ['build', '-o', serverBin, './cmd/server'], { cwd: backendDir, timeout: 180000 });
  save('go-build-server.txt', build);
  if (build.status !== 0) throw new Error('go-build-server.txt failed');
  const invariantBuild = run('go', ['build', '-o', invariantBin, './cmd/invariantcheck'], { cwd: backendDir, timeout: 180000 });
  save('go-build-invariantcheck.txt', invariantBuild);
  if (invariantBuild.status !== 0) throw new Error('go-build-invariantcheck.txt failed');
  seedLoadData('seed-start.txt');
  const child = spawn(serverBin, [], {
    cwd: backendDir,
    env: {
      ...process.env,
      ALLOW_MOCK_AUTH: 'true',
      HTTP_ADDR: '127.0.0.1:18080',
      ...pressureEnv,
    },
    shell: false,
    windowsHide: true,
  });
  child.stdout.pipe(stream(join(rawRoot, 'server.log')));
  child.stderr.pipe(stream(join(rawRoot, 'server.err.log')));
  child.on('exit', (code, signal) => {
    writeFileSync(join(rawRoot, 'server-exit.txt'), `code=${code ?? ''}\nsignal=${signal ?? ''}\n`);
  });
  await waitReady();
  return child;
}

function seedLoadData(name) {
  const result = run('go', ['run', './cmd/p1loadseed'], { cwd: backendDir, timeout: 120000 });
  save(name, result);
  if (result.status !== 0) throw new Error(`${name} failed`);
}

async function stopServer(child) {
  if (child && child.exitCode === null) {
    child.kill();
    await new Promise((resolve) => child.once('exit', resolve));
  }
}

function cleanupLocalPorts() {
  if (process.platform !== 'win32') return;
  const script = [
    '$ports = @(8080,18080,5173,5174,5175,5176,5177)',
    'foreach ($port in $ports) {',
    '  Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue |',
    '    Select-Object -ExpandProperty OwningProcess -Unique |',
    '    ForEach-Object { if ($_ -and $_ -ne 0 -and $_ -ne $PID) { Stop-Process -Id $_ -Force -ErrorAction SilentlyContinue } }',
    '}',
  ].join('; ');
  save('port-cleanup.txt', run('powershell.exe', ['-NoProfile', '-Command', script], { timeout: 30000 }));
}

function validateK6Summary(workloadName, rawPath) {
  const summary = readK6Summary(rawPath);
  if (!summary) throw new Error('could not read k6 summary');
  const checks = summary.metrics?.checks;
  const checkCount = Number(checks?.passes || 0) + Number(checks?.fails || 0);
  if (checkCount <= 0) {
    throw new Error(`${workloadName} produced zero checks; workload did not exercise the expected path`);
  }
  return summary;
}

function writeCompactReport(finalResults) {
  const report = {
    generated_at: new Date().toISOString(),
    raw_root: rawRoot,
    artifact_mode: artifactMode,
    profile,
    admission_enabled: admissionEnabled,
    invariant_check_enabled: invariantCheckEnabled,
    invariant_scope: {
      auction_id: invariantAuctionID,
      room_id: invariantRoomID,
    },
    workloads: finalResults.map((result) => ({
      workload: result.workload,
      status: result.status,
      timedOut: result.timedOut,
      validationError: result.validationError,
      environment_signals: result.environment_signals,
      admission_proof: result.admission_proof,
      k6: result.k6,
      invariants: result.invariants,
      raw: result.raw,
      metrics_before: result.metrics_before,
      metrics_after: result.metrics_after,
      log_excerpt: result.log_excerpt,
    })),
  };
  writeFileSync(join(rawRoot, 'analysis-compact.json'), JSON.stringify(report, null, 2));
  const markdown = [
    '# P3 Local Stress Compact Report',
    '',
    `- generated_at: ${report.generated_at}`,
    `- artifact_mode: ${artifactMode}`,
    `- profile: ${profile}`,
    `- admission_enabled: ${admissionEnabled}`,
    `- invariant_check_enabled: ${invariantCheckEnabled}`,
    `- invariant_scope: auction=${invariantAuctionID || '-'} room=${invariantRoomID || '-'}`,
    `- raw_root: ${rawRoot}`,
    '',
    '| Workload | Status | Invariants | Admission enabled before/after | Admission reject delta | Iterations | Dropped | Env signals | p99 ms | Error |',
    '|---|---:|---:|---:|---:|---:|---:|---|---:|---|',
    ...report.workloads.map((workload) => [
      workload.workload,
      workload.status,
      workload.invariants?.status ?? '',
      `${workload.admission_proof?.enabled_before ?? ''}/${workload.admission_proof?.enabled_after ?? ''}`,
      workload.admission_proof ? JSON.stringify(workload.admission_proof.reject_delta) : '',
      workload.k6?.iterations ?? 0,
      workload.k6?.dropped_iterations ?? 0,
      workload.environment_signals?.signals?.join(', ') || '',
      workload.k6?.http_req_duration_ms?.p99 ?? '',
      workload.validationError || '',
    ].map((cell) => String(cell).replace(/\|/g, '/')).join(' | ')).map((row) => `| ${row} |`),
    '',
    'Read this file first for attribution. Open raw JSON, Prometheus snapshots, DB snapshots, or logs only when this compact report points to a specific workload and bottleneck candidate.',
    '',
  ].join('\n');
  writeFileSync(join(rawRoot, 'analysis-compact.md'), markdown);
}

function pruneSuccessfulArtifacts() {
  if (keepFullArtifacts) return;
  const suffixes = [
    '.log',
    '-metrics-samples.prom',
    '-readyz.txt',
    '-metrics.prom',
    '-db-activity.txt',
    '-db-locks.txt',
  ];
  for (const result of results) {
    if (result.status !== 'PASS') continue;
    for (const suffix of suffixes) {
      const path = join(rawRoot, `${result.workload}${suffix}`);
      if (existsSync(path)) rmSync(path, { force: true });
    }
  }
}

mkdirSync(rawRoot, { recursive: true });
writeFileSync(join(rawRoot, 'environment.json'), JSON.stringify({
  platform: process.platform,
  arch: process.arch,
  cpu: os.cpus()[0]?.model || 'unknown',
  cpu_count: os.cpus().length,
  ram_bytes: os.totalmem(),
  os_release: os.release(),
  go: output('go', ['version']),
  k6: output('k6', ['version']),
  git_sha: output('git', ['rev-parse', 'HEAD']),
  mode: 'p3-local-smoke',
  profile,
  admission_enabled: admissionEnabled,
  effective_admission_env: {
    ADMISSION_ENABLED: pressureEnv.ADMISSION_ENABLED ?? process.env.ADMISSION_ENABLED ?? '',
    BID_USER_LIMIT_PER_SECOND: pressureEnv.BID_USER_LIMIT_PER_SECOND ?? process.env.BID_USER_LIMIT_PER_SECOND ?? '',
    BID_IP_LIMIT_PER_SECOND: pressureEnv.BID_IP_LIMIT_PER_SECOND ?? process.env.BID_IP_LIMIT_PER_SECOND ?? '',
    BID_AUCTION_LIMIT_PER_SECOND: pressureEnv.BID_AUCTION_LIMIT_PER_SECOND ?? process.env.BID_AUCTION_LIMIT_PER_SECOND ?? '',
    BID_AUCTION_MAX_IN_FLIGHT: pressureEnv.BID_AUCTION_MAX_IN_FLIGHT ?? process.env.BID_AUCTION_MAX_IN_FLIGHT ?? '',
    WS_TICKET_MAX_IN_FLIGHT: pressureEnv.WS_TICKET_MAX_IN_FLIGHT ?? process.env.WS_TICKET_MAX_IN_FLIGHT ?? '',
    WS_CONNECT_MAX_IN_FLIGHT: pressureEnv.WS_CONNECT_MAX_IN_FLIGHT ?? process.env.WS_CONNECT_MAX_IN_FLIGHT ?? '',
  },
  manage_server: manageServer,
  isolate_workloads: isolateWorkloads,
  reset_between_workloads: resetBetweenWorkloads,
  start_delay_seconds: startDelaySeconds,
  metrics_sample_seconds: metricsSampleSeconds,
  post_workload_observe_seconds: postWorkloadObserveSeconds,
  invariant_check_enabled: invariantCheckEnabled,
  invariant_scope: {
    auction_id: invariantAuctionID,
    room_id: invariantRoomID,
  },
  artifact_mode: artifactMode,
  duration,
  vus,
}, null, 2));

let server;
const results = [];
try {
  if (manageServer && !isolateWorkloads) {
    server = await startManagedServer();
  }
  for (const workload of selectedWorkloads) {
    if (manageServer && isolateWorkloads) {
      server = await startManagedServer();
    } else if (manageServer && resetBetweenWorkloads && workload.name !== 'preflight') {
      seedLoadData(`${workload.name}-seed.txt`);
    }
    const rawPath = join(rawRoot, `${workload.name}.json`);
    const logPath = join(rawRoot, `${workload.name}.log`);
    const beforeMetrics = run('curl.exe', ['-fsS', `${baseURL}/metrics`], { timeout: 5000 });
    save(`${workload.name}-metrics-before.prom`, beforeMetrics);
    if (startDelaySeconds > 0) {
      await sleep(startDelaySeconds * 1000);
      await waitReady();
    }
    const result = await runK6WithSampling(workload, rawPath, logPath);
    const afterMetrics = run('curl.exe', ['-fsS', `${baseURL}/metrics`], { timeout: 5000 });
    save(`${workload.name}-metrics-after.prom`, afterMetrics);
    let status = result.status === 0 ? 'PASS' : 'FAIL';
    let validationError = '';
    let k6Summary = readK6Summary(rawPath);
    const admission_proof = admissionProof(beforeMetrics.stdout, afterMetrics.stdout);
    const combinedLog = `${result.stdout || ''}${result.stderr || ''}`;
    if (status === 'PASS') {
      try {
        k6Summary = validateK6Summary(workload.name, rawPath);
      } catch (err) {
        status = 'FAIL';
        validationError = err.message;
      }
    }
    try {
      assertAdmissionDisabled(workload.name, beforeMetrics.stdout, afterMetrics.stdout);
    } catch (err) {
      status = 'FAIL';
      validationError = validationError ? `${validationError}; ${err.message}` : err.message;
    }
    const timedOut = result.error?.code === 'ETIMEDOUT';
    const invariantResult = runInvariantCheck(workload.name);
    if (invariantResult.status === 'FAIL') {
      status = 'FAIL';
      validationError = validationError ? `${validationError}; invariant verifier failed` : 'invariant verifier failed';
    }
    results.push({
      workload: workload.name,
      status,
      timedOut,
      validationError,
      profile,
      admission_enabled: admissionEnabled,
      environment_signals: environmentSignals(k6Summary, combinedLog),
      admission_proof,
      k6: compactK6Summary(k6Summary),
      invariants: invariantResult,
      metrics_before: join(rawRoot, `${workload.name}-metrics-before.prom`),
      metrics_after: join(rawRoot, `${workload.name}-metrics-after.prom`),
      log_excerpt: compactLog(combinedLog),
      raw: rawPath,
      log: keepFullArtifacts || status !== 'PASS' ? logPath : undefined,
    });
    if (status !== 'PASS') {
      capture(`${workload.name}-readyz.txt`, 'curl.exe', ['-fsS', `${baseURL}/readyz`]);
      capture(`${workload.name}-metrics.prom`, 'curl.exe', ['-fsS', `${baseURL}/metrics`]);
      captureSQL(`${workload.name}-db-activity.txt`, "SELECT pid, state, wait_event_type, wait_event, now()-query_start AS age, left(query, 160) AS query FROM pg_stat_activity WHERE datname='live_auction' ORDER BY query_start NULLS LAST LIMIT 20;");
      captureSQL(`${workload.name}-db-locks.txt`, "SELECT locktype, mode, granted, count(*) FROM pg_locks GROUP BY locktype, mode, granted ORDER BY locktype, mode, granted;");
      writeFileSync(join(rawRoot, 'summary.json'), JSON.stringify(results, null, 2));
      writeCompactReport(results);
      throw new Error(`${workload.name} failed${validationError ? `: ${validationError}` : ''}; see ${logPath}`);
    }
    if (manageServer && isolateWorkloads) {
      await stopServer(server);
      server = undefined;
    }
  }
  pruneSuccessfulArtifacts();
  writeFileSync(join(rawRoot, 'summary.json'), JSON.stringify(results, null, 2));
  writeCompactReport(results);
  console.log('p3 local stress smoke complete');
} finally {
  await stopServer(server);
  cleanupLocalPorts();
}
