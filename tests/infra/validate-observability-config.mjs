import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = join(fileURLToPath(new URL('../..', import.meta.url)));

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

const dashboard = JSON.parse(
  readFileSync(join(root, 'infra/grafana/dashboards/live-auction-overview.json'), 'utf8'),
);

assert(dashboard.title === 'Live Auction Overview', 'dashboard title mismatch');
assert(Array.isArray(dashboard.panels) && dashboard.panels.length >= 6, 'dashboard panels missing');

const expressions = dashboard.panels
  .flatMap((panel) => panel.targets ?? [])
  .map((target) => target.expr ?? '')
  .join('\n');

for (const metric of [
  'http_request_total',
  'auction_bid_request_total',
  'auction_bid_latency_seconds_bucket',
  'auction_outbox_lag_seconds_bucket',
  'auction_ws_connections',
  'auction_anomaly_total',
]) {
  assert(expressions.includes(metric), `dashboard missing metric ${metric}`);
}

const prometheusConfig = readFileSync(join(root, 'infra/prometheus/prometheus.yml'), 'utf8');
for (const expected of [
  'job_name: live-auction-backend',
  'metrics_path: /metrics',
  'host.docker.internal:8080',
]) {
  assert(prometheusConfig.includes(expected), `prometheus config missing ${expected}`);
}

const datasource = readFileSync(
  join(root, 'infra/grafana/provisioning/datasources/prometheus.yml'),
  'utf8',
);
assert(datasource.includes('url: http://prometheus:9090'), 'grafana datasource does not target prometheus service');

console.log('observability config ok');
