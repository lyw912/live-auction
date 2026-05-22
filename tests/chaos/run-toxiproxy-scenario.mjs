import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { addToxic, createProxy, getProxy, listProxies, resetToxiproxy } from './toxiproxy-client.mjs';

const root = join(fileURLToPath(new URL('../..', import.meta.url)));
const config = JSON.parse(readFileSync(join(root, 'tests/chaos/toxiproxy-scenarios.json'), 'utf8'));
const scenarioName = process.argv[2];

if (!scenarioName) {
  console.error(`Usage: node tests/chaos/run-toxiproxy-scenario.mjs <${config.scenarios.map((s) => s.name).join('|')}|--clear|--status>`);
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
