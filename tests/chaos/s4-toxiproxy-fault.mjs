import { addToxic, createProxy, resetToxiproxy } from './toxiproxy-client.mjs';

const action = process.argv[2] || '';
const proxy = {
  name: 'redis',
  listen: '0.0.0.0:16379',
  upstream: process.env.TOXIPROXY_REDIS_UPSTREAM || 'redis:6379',
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
    if (process.env.TOXIPROXY_LATENCY_MS) {
      const latencyMs = Number(process.env.TOXIPROXY_LATENCY_MS);
      const jitterMs = Number(process.env.TOXIPROXY_JITTER_MS || 50);
      for (const stream of ['upstream', 'downstream']) {
        await addToxic('redis', {
          name: `redis_partial_latency_${stream}`,
          type: 'latency',
          stream,
          toxicity: Number(process.env.TOXIPROXY_LATENCY_TOXICITY || 1),
          attributes: {
            latency: latencyMs,
            jitter: jitterMs,
          },
        });
      }
    }
    for (const stream of ['upstream', 'downstream']) {
      await addToxic('redis', {
        name: `redis_partial_reset_${stream}`,
        type: 'reset_peer',
        stream,
        toxicity: Number(process.env.TOXIPROXY_RESET_TOXICITY || 1),
        attributes: {
          timeout: Number(process.env.TOXIPROXY_RESET_TIMEOUT_MS || 0),
        },
      });
    }
    await addToxic('redis', {
      name: 'redis_partial_timeout_downstream',
      type: 'timeout',
      stream: process.env.TOXIPROXY_TIMEOUT_STREAM || 'downstream',
      toxicity: Number(process.env.TOXIPROXY_TIMEOUT_TOXICITY || 1),
      attributes: {
        timeout: Number(process.env.TOXIPROXY_TIMEOUT_MS || 250),
      },
    });
    console.log(JSON.stringify({ action, proxy: proxy.name }));
    return;
  }
  console.error('Usage: node tests/chaos/s4-toxiproxy-fault.mjs <init|inject|clear>');
  process.exit(2);
}

await main();
