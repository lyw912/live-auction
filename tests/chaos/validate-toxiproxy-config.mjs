import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(fileURLToPath(new URL('../..', import.meta.url)));

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

const compose = readFileSync(join(root, 'infra/docker-compose.toxiproxy.yml'), 'utf8');
assert(/^  toxiproxy:/m.test(compose), 'toxiproxy override compose missing toxiproxy service');
assert(compose.includes('8474:8474'), 'compose missing toxiproxy API port');
assert(compose.includes('15432:15432'), 'compose missing postgres proxy port');
assert(compose.includes('16379:16379'), 'compose missing redis proxy port');

const scenarios = JSON.parse(readFileSync(join(root, 'tests/chaos/toxiproxy-scenarios.json'), 'utf8'));
const required = ['redis_latency_reconnect', 'redis_timeout_reconnect', 'postgres_bid_latency'];
for (const name of required) {
  const scenario = scenarios.scenarios.find((candidate) => candidate.name === name);
  assert(scenario, `missing scenario ${name}`);
  assert(Array.isArray(scenario.toxics) && scenario.toxics.length > 0, `${name} has no toxics`);
  assert(scenario.expected && scenario.expected.length > 20, `${name} missing expected behavior`);
}

const client = readFileSync(join(root, 'tests/chaos/toxiproxy-client.mjs'), 'utf8');
assert(client.includes('/proxies'), 'toxiproxy client does not use proxy API');
assert(client.includes('listProxies'), 'toxiproxy client cannot inspect configured proxies');

const runner = readFileSync(join(root, 'tests/chaos/run-toxiproxy-scenario.mjs'), 'utf8');
assert(runner.includes('--clear'), 'toxiproxy runner missing clear mode');
assert(runner.includes('active_toxics'), 'toxiproxy runner does not print active toxics');

console.log('toxiproxy config ok');
