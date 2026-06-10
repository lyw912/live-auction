import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { basename, join } from 'node:path';
import { spawnSync } from 'node:child_process';
import os from 'node:os';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const rawRoot = join(root, 'artifacts/perf/raw/p2-07');
const reportPath = join(root, 'artifacts/perf/p2-07-linux-baseline-round-1.md');
const finalMode = process.argv.includes('--final');
const smokeMode = process.argv.includes('--smoke') || !finalMode;
const runs = finalMode ? 3 : Number(process.env.RUNS || 1);
const duration = process.env.DURATION || (smokeMode ? '5s' : '60s');
const vus = process.env.VUS || (smokeMode ? '2' : '32');

const workloads = [
  { name: 'final-second-bid-burst', script: 'tests/load/final-second-bid-burst.js', env: { VUS: vus, DURATION: duration } },
  { name: 'watcher-fanout', script: 'tests/load/watcher-fanout.js', env: { VUS: vus, DURATION: duration } },
  { name: 'reconnect-storm', script: 'tests/load/reconnect-storm.js', env: { VUS: vus, DURATION: duration } },
  { name: 'slow-consumer', script: 'tests/load/slow-consumer.js', env: { VUS: vus, DURATION: duration } },
  { name: 'outbox-burst', script: 'tests/load/outbox-burst.js', env: { VUS: vus, DURATION: duration } },
  { name: 'bid-abuse', script: 'tests/load/bid-abuse.js', env: { VUS: vus, DURATION: duration } },
  {
    name: 'multi-room-isolation',
    script: 'tests/load/multi-room-isolation.js',
    env: { DURATION: duration, HOT_BID_VUS: vus, COLD_WS_VUS: process.env.COLD_WS_VUS || (smokeMode ? '1' : '16') },
  },
];

function run(command, args, options = {}) {
  return spawnSync(command, args, {
    cwd: root,
    env: { ...process.env, ...(options.env || {}) },
    encoding: 'utf8',
    shell: process.platform === 'win32',
  });
}

function commandOutput(command, args) {
  const result = run(command, args);
  return (result.stdout || result.stderr || '').trim();
}

function requireFinalEnvironment() {
  if (process.platform !== 'linux') {
    throw new Error('P2-07 final baseline requires Linux native. Use --smoke for local script validation; do not claim performance numbers.');
  }
  const ulimit = commandOutput('sh', ['-lc', 'ulimit -n']);
  const fdLimit = Number(ulimit);
  if (!Number.isFinite(fdLimit) || fdLimit < 65535) {
    throw new Error(`P2-07 final baseline requires ulimit -n >= 65535, got ${ulimit || 'unknown'}`);
  }
}

function collectEnvironment() {
  return {
    platform: process.platform,
    arch: process.arch,
    cpu: os.cpus()[0]?.model || 'unknown',
    cpu_count: os.cpus().length,
    ram_bytes: os.totalmem(),
    os_release: os.release(),
    kernel: commandOutput('uname', ['-a']),
    go: commandOutput('go', ['version']),
    k6: commandOutput('k6', ['version']),
    postgres: commandOutput('psql', ['--version']),
    redis: commandOutput('redis-server', ['--version']),
    ulimit_n: process.platform === 'linux' ? commandOutput('sh', ['-lc', 'ulimit -n']) : 'not-linux',
    somaxconn: process.platform === 'linux' && existsSync('/proc/sys/net/core/somaxconn') ? readFileSync('/proc/sys/net/core/somaxconn', 'utf8').trim() : 'not-linux',
    ephemeral_port_range: process.platform === 'linux' && existsSync('/proc/sys/net/ipv4/ip_local_port_range') ? readFileSync('/proc/sys/net/ipv4/ip_local_port_range', 'utf8').trim() : 'not-linux',
    git_sha: commandOutput('git', ['rev-parse', 'HEAD']),
    mode: finalMode ? 'final-linux-3-run' : 'local-smoke',
    duration,
    vus,
  };
}

function assertPrerequisites() {
  if (finalMode) requireFinalEnvironment();
  const k6Version = commandOutput('k6', ['version']);
  if (!k6Version) {
    throw new Error('k6 is required to run the P2-07 baseline harness.');
  }
}

function writeReport(env, results) {
  const rows = results.map((item) => `| ${item.workload} | ${item.run} | ${item.status} | ${item.raw.replaceAll('\\', '/')} |`).join('\n');
  const report = `# P2-07 Linux Baseline Round 1

Date: ${new Date().toISOString()}

Commit: ${env.git_sha}

Status: ${finalMode ? 'FINAL_RUN_CAPTURED_REVIEW_REQUIRED' : 'SMOKE_ONLY_NOT_A_CAPACITY_BASELINE'}

## Environment

\`\`\`json
${JSON.stringify(env, null, 2)}
\`\`\`

## Workloads

| Workload | Run | Status | Raw Output |
|---|---:|---|---|
${rows}

## Verdict

Measured claim allowed: ${finalMode ? 'only after manual review of all raw outputs, system metrics, and bottlenecks' : 'no'}

Known limits:

- Smoke mode exists only to validate the harness and scripts.
- Windows/WSL/local smoke output must not be used for QPS, p99, fanout, or online-user claims.
- Final mode requires Linux native, documented DB/Redis/backend/k6 boundaries, and 3 raw runs per workload.

Next action:

- Run \`node tests/load/run-p2-linux-baseline.mjs --final\` on the Linux baseline host after starting PostgreSQL, Redis, backend, and seeding with \`go run ./cmd/p1loadseed\`.
`;
  writeFileSync(reportPath, report);
}

assertPrerequisites();
mkdirSync(rawRoot, { recursive: true });
const env = collectEnvironment();
writeFileSync(join(rawRoot, 'environment.json'), JSON.stringify(env, null, 2));

const results = [];
for (const workload of workloads) {
  for (let runNumber = 1; runNumber <= runs; runNumber += 1) {
    const rawPath = join(rawRoot, `${workload.name}-run-${runNumber}.json`);
    const args = ['run', '--summary-export', rawPath, workload.script];
    const result = run('k6', args, { env: workload.env });
    const status = result.status === 0 ? 'PASS' : 'FAIL';
    results.push({ workload: workload.name, run: runNumber, status, raw: rawPath });
    writeFileSync(join(rawRoot, `${workload.name}-run-${runNumber}.log`), `${result.stdout || ''}${result.stderr || ''}`);
    if (result.status !== 0) {
      writeReport(env, results);
      throw new Error(`${workload.name} run ${runNumber} failed; see ${join(rawRoot, `${workload.name}-run-${runNumber}.log`)}`);
    }
  }
}

writeReport(env, results);
console.log(`${finalMode ? 'final' : 'smoke'} P2-07 baseline harness complete: ${basename(reportPath)}`);
