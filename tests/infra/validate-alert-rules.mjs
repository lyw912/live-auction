import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(fileURLToPath(new URL('../..', import.meta.url)));
const alertPath = join(root, 'infra/prometheus/rules/live-auction-alerts.yml');
const prometheusPath = join(root, 'infra/prometheus/prometheus.yml');
const composePath = join(root, 'infra/docker-compose.yml');
const runbookPath = join(root, 'docs/design/05-alert-runbooks.md');

const implementedMetrics = [
  'auction_anomaly_total',
  'auction_outbox_dead_total',
  'auction_outbox_lag_seconds_bucket',
  'auction_scheduler_drift_seconds_bucket',
  'auction_ws_recover_total',
  'auction_snapshot_source_total',
  'auction_ws_slow_consumer_disconnect_total'
];

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function parseAlertBlocks(raw) {
  const blocks = [];
  const lines = raw.split(/\r?\n/);
  let current = null;
  for (const line of lines) {
    const alertMatch = line.match(/^\s+- alert: ([A-Za-z0-9_]+)\s*$/);
    if (alertMatch) {
      if (current) {
        blocks.push(current);
      }
      current = { name: alertMatch[1], lines: [line] };
      continue;
    }
    if (current) {
      current.lines.push(line);
    }
  }
  if (current) {
    blocks.push(current);
  }
  return blocks;
}

function extractExpr(block) {
  const line = block.lines.find((entry) => entry.trim().startsWith('expr: '));
  assert(line, `${block.name} missing expr`);
  return line.trim().slice('expr: '.length);
}

function runbookAnchor(name) {
  return name.toLowerCase();
}

const rawAlerts = readFileSync(alertPath, 'utf8');
const prometheusConfig = readFileSync(prometheusPath, 'utf8');
const compose = readFileSync(composePath, 'utf8');
const runbook = readFileSync(runbookPath, 'utf8').toLowerCase();

assert(prometheusConfig.includes('rule_files:'), 'prometheus.yml missing rule_files');
assert(prometheusConfig.includes('/etc/prometheus/rules/*.yml'), 'prometheus.yml missing rules glob');
assert(compose.includes('./prometheus/rules:/etc/prometheus/rules:ro'), 'docker compose does not mount prometheus rules');

const blocks = parseAlertBlocks(rawAlerts);
assert(blocks.length >= 6, 'expected at least 6 alert rules');

for (const block of blocks) {
  const text = block.lines.join('\n');
  const expr = extractExpr(block);
  assert(text.includes('severity:'), `${block.name} missing severity label`);
  assert(text.includes('component:'), `${block.name} missing component label`);
  assert(text.includes('runbook:'), `${block.name} missing runbook annotation`);
  assert(text.includes(`docs/design/05-alert-runbooks.md#${runbookAnchor(block.name)}`), `${block.name} runbook link mismatch`);
  assert(runbook.includes(`## ${block.name}`.toLowerCase()), `${block.name} missing runbook section`);

  const metricRefs = expr.match(/[a-zA-Z_:][a-zA-Z0-9_:]*/g) ?? [];
  const projectMetrics = metricRefs.filter((name) => name.startsWith('auction_'));
  assert(projectMetrics.length > 0, `${block.name} expression does not reference auction metrics`);
  for (const metric of projectMetrics) {
    assert(implementedMetrics.includes(metric), `${block.name} references unimplemented metric ${metric}`);
  }
}

console.log(`alert rules ok (${blocks.length} rules)`);
