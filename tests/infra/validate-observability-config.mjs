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
const pts1bDashboard = JSON.parse(
  readFileSync(join(root, 'infra/grafana/dashboards/live-auction-pts1b-bottlenecks.json'), 'utf8'),
);

assert(dashboard.title === 'Live Auction Overview', 'dashboard title mismatch');
assert(Array.isArray(dashboard.panels) && dashboard.panels.length >= 6, 'dashboard panels missing');
assert(pts1bDashboard.title === 'Live Auction PTS1-B Bottlenecks', 'pts1b dashboard title mismatch');
assert(
  Array.isArray(pts1bDashboard.panels) && pts1bDashboard.panels.length >= 9,
  'pts1b dashboard panels missing',
);

const expressions = [...dashboard.panels, ...pts1bDashboard.panels]
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
  'auction_bid_gateway_stage_seconds_bucket',
  'auction_bid_http_stage_seconds_bucket',
  'auction_bid_kafka_append_seconds_bucket',
  'auction_bid_redis_ledger_seconds_bucket',
  'auction_bid_redis_pending_decisions',
  'auction_bid_engine_pause_total',
  'db_pool_conns',
]) {
  assert(expressions.includes(metric), `dashboard missing metric ${metric}`);
}

const prometheusConfig = readFileSync(join(root, 'infra/prometheus/prometheus.yml'), 'utf8');
for (const expected of [
  'job_name: live-auction-backend',
  'metrics_path: /metrics',
  'host.docker.internal:18080',
  'job_name: live-auction-otel-collector',
  'otel-collector:8889',
]) {
  assert(prometheusConfig.includes(expected), `prometheus config missing ${expected}`);
}

const datasource = readFileSync(
  join(root, 'infra/grafana/provisioning/datasources/prometheus.yml'),
  'utf8',
);
assert(datasource.includes('url: http://prometheus:9090'), 'grafana datasource does not target prometheus service');
assert(datasource.includes('url: http://tempo:3200'), 'grafana datasource does not target tempo service');
assert(datasource.includes('url: http://pyroscope:4040'), 'grafana datasource does not target pyroscope service');

const composeConfig = readFileSync(join(root, 'infra/docker-compose.yml'), 'utf8');
for (const expected of ['tempo:', 'otel-collector:', 'pyroscope:', '4317:4317', '4040:4040']) {
  assert(composeConfig.includes(expected), `docker compose missing ${expected}`);
}

const otelConfig = readFileSync(join(root, 'infra/otel/collector.yml'), 'utf8');
for (const expected of ['receivers:', 'otlp:', 'exporters:', 'otlp/tempo:', 'endpoint: tempo:4317']) {
  assert(otelConfig.includes(expected), `otel collector config missing ${expected}`);
}

const tempoConfig = readFileSync(join(root, 'infra/tempo/tempo.yml'), 'utf8');
for (const expected of ['http_listen_port: 3200', 'otlp:', 'block_retention: 24h']) {
  assert(tempoConfig.includes(expected), `tempo config missing ${expected}`);
}

console.log('observability config ok');
