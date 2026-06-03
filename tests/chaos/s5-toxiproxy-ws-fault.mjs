import { addToxic, createProxy, resetToxiproxy } from './toxiproxy-client.mjs';

const action = process.argv[2] || '';
const proxy = {
  name: 'ws-backend',
  listen: process.env.TOXIPROXY_WS_LISTEN || '0.0.0.0:18081',
  upstream: process.env.TOXIPROXY_WS_UPSTREAM || 'host.docker.internal:18080',
};

async function main() {
  if (action === 'init' || action === 'clear') {
    await resetToxiproxy();
    await createProxy(proxy);
    console.log(JSON.stringify({ action, proxy }));
    return;
  }
  if (action === 'inject') {
    await resetToxiproxy();
    await createProxy(proxy);
    await addToxic(proxy.name, {
      name: 'ws_reset_peer',
      type: 'reset_peer',
      stream: process.env.TOXIPROXY_WS_STREAM || 'downstream',
      toxicity: Number(process.env.TOXIPROXY_WS_TOXICITY || 0.3),
      attributes: {
        timeout: Number(process.env.TOXIPROXY_WS_RESET_TIMEOUT_MS || 0),
      },
    });
    console.log(JSON.stringify({ action, proxy: proxy.name }));
    return;
  }
  console.error('Usage: node tests/chaos/s5-toxiproxy-ws-fault.mjs <init|inject|clear>');
  process.exit(2);
}

await main();
