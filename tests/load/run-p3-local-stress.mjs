import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import os from 'node:os';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const rawRoot = join(root, 'docs/perf/raw/p3-00');
const duration = process.env.DURATION || '5s';
const vus = process.env.VUS || '2';
const workloadTimeoutMs = Number(process.env.WORKLOAD_TIMEOUT_MS || 60000);

const workloads = [
  { name: 'preflight', script: 'tests/load/preflight.js', env: { VUS: '1', DURATION: '1s' } },
  { name: 'final-second-bid-burst', script: 'tests/load/final-second-bid-burst.js', env: { VUS: vus, DURATION: duration } },
  { name: 'outbox-burst', script: 'tests/load/outbox-burst.js', env: { VUS: vus, DURATION: duration } },
  { name: 'watcher-fanout', script: 'tests/load/watcher-fanout.js', env: { VUS: vus, DURATION: duration, SESSION_SECONDS: '1.2', SESSION_MS: '1000' } },
  { name: 'slow-consumer', script: 'tests/load/slow-consumer.js', env: { VUS: vus, DURATION: duration, SESSION_SECONDS: '1.2', SESSION_MS: '1000', BLOCK_MS: '5' } },
  { name: 'reconnect-storm', script: 'tests/load/reconnect-storm.js', env: { VUS: vus, DURATION: duration, SESSION_SECONDS: '1.2' } },
  {
    name: 'multi-room-isolation',
    script: 'tests/load/multi-room-isolation.js',
    env: { DURATION: duration, HOT_BID_VUS: vus, COLD_WS_VUS: process.env.COLD_WS_VUS || '1', COLD_SESSION_SECONDS: '1.2' },
  },
  { name: 'bid-abuse', script: 'tests/load/bid-abuse.js', env: { VUS: vus, DURATION: duration } },
];

function run(command, args, options = {}) {
  return spawnSync(command, args, {
    cwd: root,
    env: { ...process.env, ...(options.env || {}) },
    encoding: 'utf8',
    shell: process.platform === 'win32',
    timeout: options.timeout || workloadTimeoutMs,
  });
}

function output(command, args) {
  const result = run(command, args);
  return (result.stdout || result.stderr || '').trim();
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
  duration,
  vus,
}, null, 2));

const results = [];
for (const workload of workloads) {
  const rawPath = join(rawRoot, `${workload.name}.json`);
  const logPath = join(rawRoot, `${workload.name}.log`);
  const result = run('k6', ['run', '--summary-export', rawPath, workload.script], { env: workload.env, timeout: workloadTimeoutMs });
  const status = result.status === 0 ? 'PASS' : 'FAIL';
  const timedOut = result.error?.code === 'ETIMEDOUT';
  results.push({ workload: workload.name, status, timedOut, raw: rawPath, log: logPath });
  writeFileSync(logPath, `${result.stdout || ''}${result.stderr || ''}`);
  if (status !== 'PASS') {
    writeFileSync(join(rawRoot, 'summary.json'), JSON.stringify(results, null, 2));
    throw new Error(`${workload.name} failed; see ${logPath}`);
  }
}

writeFileSync(join(rawRoot, 'summary.json'), JSON.stringify(results, null, 2));
console.log('p3 local stress smoke complete');
