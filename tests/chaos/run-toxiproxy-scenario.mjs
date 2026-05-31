import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { addToxic, createProxy, getProxy, listProxies, resetToxiproxy } from './toxiproxy-client.mjs';

const root = join(fileURLToPath(new URL('../..', import.meta.url)));
const config = JSON.parse(readFileSync(join(root, 'tests/chaos/toxiproxy-scenarios.json'), 'utf8'));
const scenarioName = process.argv[2];
const runMode = process.argv.includes('--run');
const baseURL = process.env.BASE_URL || 'http://127.0.0.1:18080';
const evidenceRoot = process.env.EVIDENCE_ROOT || join(root, 'docs/perf/pts/evidence/incoming');

if (!scenarioName) {
  console.error(`Usage: node tests/chaos/run-toxiproxy-scenario.mjs <${config.scenarios.map((s) => s.name).join('|')}|--clear|--status> [--run]`);
  process.exit(2);
}

async function recreateBaseProxies() {
  await resetToxiproxy();
  for (const proxy of config.proxies) {
    await createProxy(proxy);
  }
}

function summarizeProxies(proxies) {
  return Object.fromEntries(Object.entries(proxies ?? {}).map(([name, proxy]) => [
    name,
    {
      listen: proxy.listen,
      upstream: proxy.upstream,
      enabled: proxy.enabled,
      toxics: (proxy.toxics ?? []).map((toxic) => ({
        name: toxic.name,
        type: toxic.type,
        stream: toxic.stream,
        toxicity: toxic.toxicity,
        attributes: toxic.attributes,
      })),
    },
  ]));
}

async function fetchJSON(path, options = {}) {
  const response = await fetch(`${baseURL}${path}`, {
    ...options,
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/json',
      'X-Mock-Role': options.role || 'user',
      'X-Mock-User-Id': options.userID || 'chaos_user_1',
      ...(options.headers ?? {}),
    },
  });
  let body = null;
  try {
    body = await response.json();
  } catch {
    body = await response.text().catch(() => '');
  }
  return { status: response.status, body };
}

async function runProbe(scenario) {
  const label = `chaos-${scenario.name}-${new Date().toISOString().replace(/[:.]/g, '')}`;
  const outDir = join(evidenceRoot, label);
  mkdirSync(outDir, { recursive: true });

  const before = {
    readyz: await fetchJSON('/readyz'),
    snapshot: await fetchJSON('/api/auctions/auc_live'),
  };

  await recreateBaseProxies();
  for (const toxic of scenario.toxics) {
    await addToxic(scenario.proxy, toxic);
  }
  const active = summarizeProxies(await listProxies());

  const samples = [];
  const started = Date.now();
  for (let i = 0; i < Number(process.env.CHAOS_PROBE_ITERATIONS || 12); i += 1) {
    const clientBidID = `${label}-${i}`;
    const snapshot = await fetchJSON('/api/auctions/auc_live', { userID: `chaos_snap_${i}` });
    const current = Number(snapshot.body?.current_price_cents || 10000);
    const increment = Number(snapshot.body?.increment_cents || 5000);
    const bid = await fetchJSON('/api/auctions/auc_live/bids', {
      method: 'POST',
      userID: `chaos_bidder_${i}`,
      headers: { 'Idempotency-Key': clientBidID },
      body: JSON.stringify({
        client_bid_id: clientBidID,
        amount_cents: current + increment * ((i % 3) + 1),
        client_seen_seq: 0,
      }),
    });
    samples.push({ iteration: i, snapshot, bid });
    await new Promise((resolve) => setTimeout(resolve, Number(process.env.CHAOS_PROBE_SLEEP_MS || 250)));
  }

  await recreateBaseProxies();
  const after = {
    readyz: await fetchJSON('/readyz'),
    snapshot: await fetchJSON('/api/auctions/auc_live'),
    elapsed_ms: Date.now() - started,
  };

  const bidResults = samples.map((sample) => ({
    status: sample.bid.status,
    result: sample.bid.body?.result || sample.bid.body?.code || '',
    decision_status: sample.bid.body?.decision_status || '',
    durability_status: sample.bid.body?.durability_status || '',
    engine_seq: sample.bid.body?.engine_seq || 0,
  }));
  const dangerousSuccess = bidResults.filter((row) =>
    row.status === 200 &&
    row.decision_status === 'DECIDED' &&
    String(row.durability_status || '').length === 0
  ).length;
  const vagueFailures = bidResults.filter((row) => row.status >= 500 || row.result === '').length;
  const gates = [
    ['P0', 'toxics_active', active[scenario.proxy]?.toxics?.length > 0, 'scenario toxic must be active before probes'],
    ['P0', 'service_recovers_after_clear', after.readyz.status === 200 && after.snapshot.status === 200, 'service must answer readyz/snapshot after clearing toxics'],
    ['P0', 'no_decided_without_durability', dangerousSuccess === 0, 'DECIDED responses must include durability status'],
    ['P1', 'bounded_user_visible_failures', vagueFailures <= Number(process.env.CHAOS_MAX_VAGUE_FAILURES || 3), 'chaos should surface bounded explicit states rather than vague server errors'],
  ];

  const report = {
    label,
    scenario: scenario.name,
    expected: scenario.expected,
    base_url: baseURL,
    before,
    active_toxiproxy: active,
    bid_results: bidResults,
    after,
    gates: gates.map(([severity, name, passed, detail]) => ({
      severity,
      name,
      result: passed ? 'PASS' : 'FAIL',
      detail,
    })),
  };

  writeFileSync(join(outDir, 'chaos-report.json'), JSON.stringify(report, null, 2));
  writeFileSync(join(outDir, 'chaos-gates.tsv'), gates.map(([severity, name, passed, detail]) =>
    `${severity}\t${name}\t${passed ? 'PASS' : 'FAIL'}\t${detail}`
  ).join('\n') + '\n');

  console.log(JSON.stringify({
    label,
    evidence: outDir,
    gates: report.gates,
  }, null, 2));

  if (report.gates.some((gate) => gate.severity === 'P0' && gate.result !== 'PASS')) {
    process.exitCode = 1;
  }
}

if (scenarioName === '--clear') {
  await recreateBaseProxies();
  console.log(JSON.stringify({
    action: 'clear',
    proxies: summarizeProxies(await listProxies()),
  }, null, 2));
} else if (scenarioName === '--status') {
  console.log(JSON.stringify({
    action: 'status',
    proxies: summarizeProxies(await listProxies()),
  }, null, 2));
} else {
  const scenario = config.scenarios.find((candidate) => candidate.name === scenarioName);
  if (!scenario) {
    console.error(`Unknown scenario ${scenarioName}`);
    process.exitCode = 2;
  } else if (runMode) {
    await runProbe(scenario);
  } else {
    await recreateBaseProxies();
    for (const toxic of scenario.toxics) {
      await addToxic(scenario.proxy, toxic);
    }
    const proxyState = await getProxy(scenario.proxy);
    console.log(JSON.stringify({
      scenario: scenario.name,
      proxy: scenario.proxy,
      toxics: scenario.toxics.map((toxic) => toxic.name),
      active_toxics: (proxyState?.toxics ?? []).map((toxic) => ({
        name: toxic.name,
        type: toxic.type,
        stream: toxic.stream,
        toxicity: toxic.toxicity,
        attributes: toxic.attributes,
      })),
      expected: scenario.expected,
    }, null, 2));
  }
}
