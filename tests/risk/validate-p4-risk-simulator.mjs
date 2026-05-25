import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const runnerPath = join(root, 'tests/risk/run-p4-risk-simulator.mjs');
const text = readFileSync(runnerPath, 'utf8');

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

for (const scenario of [
  'bid-idempotency-replay-and-conflict',
  'host-only-flight-recorder-acl',
  'cap-sold-payment-double-click',
]) {
  assert(text.includes(scenario), `missing P4 risk scenario ${scenario}`);
}

assert(text.includes("go', ['run', './cmd/p1loadseed']"), 'risk simulator must reset real seed data');
assert(text.includes("go', ['run', './cmd/invariantcheck'"), 'risk simulator must run invariant verifier');
assert(text.includes("go', ['build', '-o', serverBin, './cmd/server']"), 'managed mode must build a concrete server binary');
assert(text.includes("join(backendDir, 'tmp')"), 'managed mode must keep generated server binary outside docs/perf/raw');
assert(text.includes('/api/auctions/auc_live/bids'), 'risk simulator must exercise real bid API');
assert(text.includes('/api/monitor/auctions/auc_live/flight-recorder'), 'risk simulator must exercise flight recorder ACL');
assert(text.includes('/api/users/me/orders'), 'risk simulator must verify user-visible order state');
assert(text.includes('/pay-mock'), 'risk simulator must exercise payment endpoint');
assert(text.includes('risk-summary.json'), 'risk simulator must write machine-readable compact report');
assert(text.includes('risk-summary.md'), 'risk simulator must write human-readable compact report');
assert(text.includes('cleanupLocalPorts();'), 'risk simulator must clean managed server port on Windows');
assert(text.includes("child.kill('SIGKILL')"), 'risk simulator must force-stop a stuck managed server');
assert(!text.includes('page.route'), 'risk simulator must not use route mocks');
assert(!text.includes('https://'), 'risk simulator must not import remote code');

console.log('p4 risk simulator config ok');
