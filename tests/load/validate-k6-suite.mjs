import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(fileURLToPath(new URL('../..', import.meta.url)));
const loadDir = join(root, 'tests/load');
const skillDir = join(root, '.codex/skills/live-auction-v2-stress-attacker');

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

const scripts = [
  'preflight.js',
  'final-second-bid-burst.js',
  'watcher-fanout.js',
  'reconnect-storm.js',
  'slow-consumer.js',
  'outbox-burst.js',
  'bid-abuse.js',
  'multi-room-isolation.js',
  'p3-bid-pressure.js',
  'p3-ws-fanout-pressure.js',
  'p3-slow-consumer-pressure.js',
  'p3-ws-connection-storm.js',
  'p3-healthy-vs-slow-consumer.js',
];

for (const script of scripts) {
  const text = readFileSync(join(loadDir, script), 'utf8');
  if (script !== 'preflight.js') {
    assert(text.includes('summaryTrendStats'), `${script} must include p99/p999 summary stats`);
  }
  assert(text.includes('thresholds'), `${script} must define explicit thresholds`);
  assert(!text.includes('k6/experimental/websockets'), `${script} uses deprecated websocket module`);
  assert(!text.includes('https://'), `${script} must not import remote code`);
}

const wsText = scripts.map((script) => readFileSync(join(loadDir, script), 'utf8')).join('\n');
const libText = readFileSync(join(loadDir, 'lib/live-auction.js'), 'utf8');
assert(libText.includes("import { WebSocket } from 'k6/websockets';"), 'suite must use k6/websockets');
assert(!libText.includes('https://'), 'load library must not import remote code');
assert(wsText.includes('issueTicket'), 'WS workloads must use browser-compatible tickets');

const coverage = {
  'final-second bid burst': 'final-second-bid-burst.js',
  'watcher fanout': 'watcher-fanout.js',
  'reconnect storm': 'reconnect-storm.js',
  'slow consumer': 'slow-consumer.js',
  'outbox burst': 'outbox-burst.js',
  'bid abuse': 'bid-abuse.js',
  'multi-room isolation': 'multi-room-isolation.js',
};
for (const [name, script] of Object.entries(coverage)) {
  assert(scripts.includes(script), `missing ${name} workload`);
}

const runnerText = readFileSync(join(loadDir, 'run-p2-linux-baseline.mjs'), 'utf8');
assert(runnerText.includes("process.platform !== 'linux'"), 'P2-07 final runner must refuse non-Linux final baselines');
assert(runnerText.includes('ulimit -n >= 65535'), 'P2-07 final runner must enforce high file descriptor limit');
assert(runnerText.includes('docs/perf/raw/p2-07'), 'P2-07 runner must store raw outputs under docs/perf/raw/p2-07');
assert(runnerText.includes('multi-room-isolation'), 'P2-07 runner must include multi-room isolation workload');
assert(runnerText.includes('bid-abuse'), 'P2-07 runner must include bid abuse workload');

const p3RunnerText = readFileSync(join(loadDir, 'run-p3-local-stress.mjs'), 'utf8');
assert(p3RunnerText.includes('docs/perf/raw/p3-local-stress'), 'P3 runner must store raw outputs under docs/perf/raw/p3-local-stress');
assert(p3RunnerText.includes('RAW_ROOT'), 'P3 runner must allow explicit raw output roots');
assert(p3RunnerText.includes('P3_ARTIFACT_MODE'), 'P3 runner must support minimal/full artifact retention modes');
assert(p3RunnerText.includes('analysis-compact.json'), 'P3 runner must write a compact analysis report');
assert(p3RunnerText.includes("join(backendDir, 'tmp')"), 'P3 runner must keep generated backend binaries outside docs/perf/raw');
assert(p3RunnerText.includes('MANAGE_SERVER'), 'P3 runner must be able to manage a local backend binary');
assert(p3RunnerText.includes('WORKLOADS'), 'P3 runner must support focused workload subsets');
assert(p3RunnerText.includes("const profile = process.env.P3_PROFILE || 'downstream-pressure'"), 'P3 runner must default to downstream-pressure');
assert(p3RunnerText.includes("ADMISSION_ENABLED: 'false'"), 'P3 downstream-pressure must disable admission with the explicit global switch');
assert(p3RunnerText.includes('assertAdmissionDisabled'), 'P3 runner must verify admission stays disabled before/after each workload');
assert(p3RunnerText.includes('auction_admission_enabled'), 'P3 runner must verify backend admission config metrics');
assert(p3RunnerText.includes('environmentSignals'), 'P3 runner must record environment/load-generator attribution signals');
assert(p3RunnerText.includes('dropped_iterations'), 'P3 runner must record k6 dropped iterations as possible environment/load-generator evidence');
assert(p3RunnerText.includes('socket_port_exhaustion'), 'P3 runner must classify Windows socket/port exhaustion signatures');
assert(p3RunnerText.includes('p3-ws-fanout-pressure.js'), 'P3 runner must include fanout pressure workload');
assert(p3RunnerText.includes('p3-slow-consumer-pressure.js'), 'P3 runner must include slow-consumer pressure workload');
assert(p3RunnerText.includes('p3-ws-connection-storm.js'), 'P3 runner must include connection storm workload');
assert(p3RunnerText.includes('p3-healthy-vs-slow-consumer.js'), 'P3 runner must include healthy-vs-slow workload');
for (const script of scripts.filter((script) => !script.startsWith('p3-'))) {
  assert(p3RunnerText.includes(script), `P3 runner must include ${script}`);
}

const p3OwnerKillText = readFileSync(join(loadDir, 'run-p3-relay-owner-kill.mjs'), 'utf8');
assert(p3OwnerKillText.includes('OUTBOX_WORKER_ID'), 'P3 owner-kill runner must use distinct relay worker IDs');
assert(p3OwnerKillText.includes('p3-bid-pressure.js'), 'P3 owner-kill runner must drive real bid pressure');
assert(p3OwnerKillText.includes('ownerA.kill()'), 'P3 owner-kill runner must kill the first owner');
assert(p3OwnerKillText.includes("ADMISSION_ENABLED: 'false'"), 'P3 owner-kill runner must disable admission during performance exploration');
assert(p3OwnerKillText.includes("join(backendDir, 'tmp')"), 'P3 owner-kill runner must keep generated binaries outside docs/perf/raw');

const p3FanoutText = readFileSync(join(loadDir, 'p3-ws-fanout-pressure.js'), 'utf8');
assert(p3FanoutText.includes('constant-arrival-rate'), 'P3 fanout pressure must use arrival-rate triggers');
assert(p3FanoutText.includes('auction_k6_ws_pressure_opened_total'), 'P3 fanout pressure must record opened sockets');
const p3SlowText = readFileSync(join(loadDir, 'p3-slow-consumer-pressure.js'), 'utf8');
assert(p3SlowText.includes('BLOCK_MS'), 'P3 slow-consumer pressure must support explicit client blocking');
assert(p3SlowText.includes('auction_k6_slow_consumer_opened_total'), 'P3 slow-consumer pressure must record opened sockets');
const p3StormText = readFileSync(join(loadDir, 'p3-ws-connection-storm.js'), 'utf8');
assert(p3StormText.includes('auction_k6_ws_storm_retry_later_total'), 'P3 connection storm must record controlled retry-later');
assert(p3StormText.includes('429'), 'P3 connection storm must treat admission 429 as controlled pressure');
const p3HealthyVsSlowText = readFileSync(join(loadDir, 'p3-healthy-vs-slow-consumer.js'), 'utf8');
assert(p3HealthyVsSlowText.includes('auction_k6_hvs_healthy_messages_total'), 'P3 healthy-vs-slow must record healthy messages');
assert(p3HealthyVsSlowText.includes('BLOCK_MS'), 'P3 healthy-vs-slow must support explicit slow-client blocking');

const p3AnalyzeText = readFileSync(join(loadDir, 'analyze-p3-artifacts.mjs'), 'utf8');
assert(p3AnalyzeText.includes('analysis-compact.json'), 'P3 artifact analyzer must read compact reports first');
assert(p3AnalyzeText.includes('p3-artifact-index.json'), 'P3 artifact analyzer must produce an aggregate compact index');
assert(p3AnalyzeText.includes('ENV_LIMIT'), 'P3 artifact analyzer must preserve environment-limit verdict hints');
assert(p3AnalyzeText.includes('next_artifacts'), 'P3 artifact analyzer must recommend the smallest next raw artifacts to inspect');
assert(p3AnalyzeText.includes('needs_full_drilldown'), 'P3 artifact analyzer must say when full artifacts may be needed');

const stressSkillText = readFileSync(join(skillDir, 'SKILL.md'), 'utf8');
assert(stressSkillText.includes('Read compact evidence first'), 'stress attacker skill must require compact-first context loading');
assert(stressSkillText.includes('Never open every file in `docs/perf/raw/**`'), 'stress attacker skill must forbid bulk raw artifact reading');
assert(stressSkillText.includes('Default to `P3_ARTIFACT_MODE=minimal`'), 'stress attacker skill must default to minimal artifact mode');

console.log('k6 suite config ok');
