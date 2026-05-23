import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(fileURLToPath(new URL('../..', import.meta.url)));
const loadDir = join(root, 'tests/load');

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

const scripts = [
  'final-second-bid-burst.js',
  'watcher-fanout.js',
  'reconnect-storm.js',
  'slow-consumer.js',
  'outbox-burst.js',
  'bid-abuse.js',
];

for (const script of scripts) {
  const text = readFileSync(join(loadDir, script), 'utf8');
  assert(text.includes('summaryTrendStats'), `${script} must include p99/p999 summary stats`);
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
};
for (const [name, script] of Object.entries(coverage)) {
  assert(scripts.includes(script), `missing ${name} workload`);
}

console.log('k6 suite config ok');
