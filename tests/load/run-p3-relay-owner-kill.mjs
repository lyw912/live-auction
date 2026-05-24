import { createWriteStream, mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../..', import.meta.url));
const stamp = new Date().toISOString().replace(/\D/g, '').slice(0, 12);
const rawRoot = join(root, `docs/perf/raw/p3-relay-owner-kill-${stamp}`);
const backendDir = join(root, 'backend');
const binDir = join(backendDir, 'tmp');
const serverBin = process.platform === 'win32' ? join(binDir, 'p3-server.exe') : join(binDir, 'p3-server');
const relayBin = process.platform === 'win32' ? join(binDir, 'p3-outboxrelay.exe') : join(binDir, 'p3-outboxrelay');
const dsn = 'postgres://live_auction:live_auction@localhost:5432/live_auction?sslmode=disable';
const targetOwner = 'p3-relay-a';
const takeoverOwner = 'p3-relay-b';
const baseEnv = {
  ...process.env,
  ALLOW_MOCK_AUTH: 'true',
  ADMISSION_ENABLED: 'false',
  BID_USER_LIMIT_PER_SECOND: '0',
  BID_IP_LIMIT_PER_SECOND: '0',
  BID_AUCTION_LIMIT_PER_SECOND: '0',
  BID_AUCTION_MAX_IN_FLIGHT: '0',
  WS_TICKET_MAX_IN_FLIGHT: '0',
  WS_CONNECT_MAX_IN_FLIGHT: '0',
};

function run(command, args, options = {}) {
  return spawnSync(command, args, {
    cwd: options.cwd || root,
    env: { ...process.env, ...(options.env || {}) },
    encoding: 'utf8',
    shell: false,
    timeout: options.timeout || 120000,
  });
}

function save(name, result) {
  writeFileSync(join(rawRoot, name), `${result.stdout || ''}${result.stderr || ''}`);
}

function psqlResult(name, sql) {
  const result = run('docker', ['exec', 'live-auction-postgres', 'psql', '-U', 'live_auction', '-d', 'live_auction', '-c', sql]);
  save(name, result);
  if (result.status !== 0) throw new Error(`${name} failed`);
  return result.stdout || '';
}

function psql(name, sql) {
  psqlResult(name, sql);
}

function scalarSQL(sql) {
  const result = run('docker', [
    'exec',
    'live-auction-postgres',
    'psql',
    '-U',
    'live_auction',
    '-d',
    'live_auction',
    '-t',
    '-A',
    '-c',
    sql,
  ]);
  if (result.status !== 0) {
    throw new Error(`scalar SQL failed: ${result.stdout || ''}${result.stderr || ''}`);
  }
  return (result.stdout || '').trim();
}

function activeOwner() {
  return scalarSQL("SELECT COALESCE((SELECT owner_id FROM outbox_relay_shard_leases WHERE lease_until > now() ORDER BY shard_id LIMIT 1), '')");
}

function auctionOutboxPending() {
  const value = scalarSQL("SELECT count(*) FROM outbox_events e JOIN outbox_delivery d ON d.outbox_id=e.id WHERE e.auction_id='auc_live' AND d.status <> 'PUBLISHED'");
  return Number.parseInt(value || '0', 10);
}

function startServer(port, workerID) {
  const out = join(rawRoot, `${workerID}.log`);
  const err = join(rawRoot, `${workerID}.err.log`);
  const child = spawn(serverBin, [], {
    cwd: backendDir,
    env: {
      ...baseEnv,
      HTTP_ADDR: `127.0.0.1:${port}`,
      OUTBOX_WORKER_ID: workerID,
      SCHEDULER_WORKER_ID: `${workerID}-scheduler`,
      DISABLE_EMBEDDED_OUTBOX_RELAY: 'true',
    },
    shell: false,
    detached: false,
    windowsHide: true,
  });
  const outFile = awaitableWrite(out);
  const errFile = awaitableWrite(err);
  child.stdout.pipe(outFile);
  child.stderr.pipe(errFile);
  return child;
}

function startRelay(workerID) {
  const child = spawn(relayBin, [], {
    cwd: backendDir,
    env: {
      ...baseEnv,
      OUTBOX_WORKER_ID: workerID,
    },
    shell: false,
    detached: false,
    windowsHide: true,
  });
  child.stdout.pipe(awaitableWrite(join(rawRoot, `${workerID}.log`)));
  child.stderr.pipe(awaitableWrite(join(rawRoot, `${workerID}.err.log`)));
  return child;
}

function awaitableWrite(path) {
  return createWriteStream(path, { flags: 'a' });
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitReady(port) {
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    const result = run('curl.exe', ['-fsS', `http://127.0.0.1:${port}/readyz`], { timeout: 5000 });
    if (result.status === 0) return;
    await sleep(500);
  }
  throw new Error(`server ${port} did not become ready`);
}

async function waitForOwner(expectedOwner, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  let lastOwner = '';
  while (Date.now() < deadline) {
    lastOwner = activeOwner();
    if (lastOwner === expectedOwner) return;
    await sleep(500);
  }
  throw new Error(`expected live owner ${expectedOwner}, got ${lastOwner || '<none>'}`);
}

async function waitForOutboxDrain(timeoutMs) {
  const lines = ['elapsed_ms,pending'];
  const deadline = Date.now() + timeoutMs;
  const startedAt = Date.now();
  let pending = auctionOutboxPending();
  lines.push(`${Date.now() - startedAt},${pending}`);
  while (pending > 0 && Date.now() < deadline) {
    await sleep(5000);
    pending = auctionOutboxPending();
    lines.push(`${Date.now() - startedAt},${pending}`);
  }
  writeFileSync(join(rawRoot, 'outbox-drain-poll.csv'), `${lines.join('\n')}\n`);
  return pending;
}

async function main() {
  mkdirSync(rawRoot, { recursive: true });
  mkdirSync(binDir, { recursive: true });
  writeFileSync(join(rawRoot, 'runner-error.txt'), '');
  writeFileSync(join(rawRoot, 'raw-root.txt'), `${rawRoot}\n`);
  save('docker-up.txt', run('docker', ['compose', '-f', 'infra/docker-compose.yml', 'up', '-d', 'postgres', 'redis', 'minio']));
  save('goose-up.txt', run('goose', ['-dir', 'backend/migrations', 'postgres', dsn, 'up']));
  const build = run('go', ['build', '-o', serverBin, './cmd/server'], { cwd: backendDir, timeout: 180000 });
  save('go-build-server.txt', build);
  if (build.status !== 0) throw new Error('go-build-server.txt failed');
  const buildRelay = run('go', ['build', '-o', relayBin, './cmd/outboxrelay'], { cwd: backendDir, timeout: 180000 });
  save('go-build-relay.txt', buildRelay);
  if (buildRelay.status !== 0) throw new Error('go-build-relay.txt failed');
  psql('preclean-outbox.txt', "UPDATE outbox_delivery SET status='PUBLISHED', published_at=COALESCE(published_at, now()), locked_by=NULL, locked_until=NULL WHERE status NOT IN ('PUBLISHED','DEAD'); DELETE FROM outbox_relay_shard_leases;");
  save('seed.txt', run('go', ['run', './cmd/p1loadseed'], { cwd: backendDir }));

  const httpServer = startServer(18080, 'p3-http');
  const ownerA = startRelay(targetOwner);
  let ownerB;
  let k6;
  try {
    await waitReady(18080);
    psql('leases-before.txt', 'SELECT shard_id, owner_id, lease_until > now() AS live FROM outbox_relay_shard_leases ORDER BY shard_id;');

    k6 = spawn('k6', ['run', '--summary-export', join(rawRoot, 'bid-pressure-owner-kill.json'), 'tests/load/p3-bid-pressure.js'], {
      cwd: root,
      env: {
        ...process.env,
        BASE_URL: 'http://127.0.0.1:18080',
        RATE: process.env.RATE || '120',
        DURATION: '30s',
        PRE_ALLOCATED_VUS: '160',
        MAX_VUS: '400',
      },
      shell: false,
      windowsHide: true,
    });
    k6.stdout.pipe(awaitableWrite(join(rawRoot, 'bid-pressure-owner-kill.log')));
    k6.stderr.pipe(awaitableWrite(join(rawRoot, 'bid-pressure-owner-kill.err.log')));
    await sleep(12000);
    psql('leases-before-kill.txt', 'SELECT shard_id, owner_id, lease_until > now() AS live FROM outbox_relay_shard_leases ORDER BY shard_id;');
    await waitForOwner(targetOwner, 5000);
    ownerB = startRelay(takeoverOwner);
    await sleep(2000);
    writeFileSync(join(rawRoot, 'killed-owner.txt'), `${targetOwner}\n`);
    ownerA.kill();
    await sleep(10000);
    psql('leases-after-kill.txt', 'SELECT shard_id, owner_id, lease_until > now() AS live FROM outbox_relay_shard_leases ORDER BY shard_id;');
    const ownerAfterKill = activeOwner();
    if (ownerAfterKill !== takeoverOwner) {
      throw new Error(`expected takeover owner ${takeoverOwner}, got ${ownerAfterKill || '<none>'}`);
    }
    const k6Status = await new Promise((resolve) => k6.on('exit', (code) => resolve(code)));
    writeFileSync(join(rawRoot, 'k6-exit.txt'), String(k6Status));
    const pendingAfterDrain = await waitForOutboxDrain(90000);
    psql('outbox-after-drain.txt', "SELECT d.status, count(*) FROM outbox_events e JOIN outbox_delivery d ON d.outbox_id=e.id WHERE e.auction_id='auc_live' GROUP BY d.status ORDER BY d.status;");
    psql('leases-final.txt', 'SELECT shard_id, owner_id, lease_until > now() AS live FROM outbox_relay_shard_leases ORDER BY shard_id;');
    if (pendingAfterDrain !== 0) {
      throw new Error(`outbox did not drain after failover, pending=${pendingAfterDrain}`);
    }
  } finally {
    if (k6 && k6.exitCode === null) k6.kill();
    httpServer.kill();
    ownerA.kill();
    if (ownerB) ownerB.kill();
  }
}

main().catch((err) => {
  writeFileSync(join(rawRoot, 'runner-error.txt'), `${err.stack || err.message}\n`);
  process.exit(1);
});
