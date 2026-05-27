import { spawn } from 'node:child_process';
import fs from 'node:fs/promises';
import http from 'node:http';
import net from 'node:net';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const backendDir = join(root, 'backend');
const node = process.execPath;
const viteCli = join(root, 'node_modules', 'vite', 'bin', 'vite.js');
const playwrightCli = join(root, 'node_modules', '@playwright', 'test', 'cli.js');
const backendURL = process.env.LIVE_AUCTION_API_TARGET || 'http://127.0.0.1:18080';
const h5URL = process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5276';
const pcURL = process.env.LIVE_AUCTION_PC_URL || 'http://127.0.0.1:5277';
const evidencePath = join(root, 'docs', 'perf', 'raw', 'p10-no-mock-live-smoke.json');
const children = [];

function spawnLogged(command, args, options = {}) {
  const child = spawn(command, args, {
    cwd: options.cwd || root,
    env: {
      ...process.env,
      ...options.env
    },
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true
  });
  const label = options.label || command;
  child.stdout.on('data', (chunk) => process.stdout.write(`[${label}] ${chunk}`));
  child.stderr.on('data', (chunk) => process.stderr.write(`[${label}] ${chunk}`));
  children.push(child);
  return child;
}

function run(command, args, options = {}) {
  return new Promise((resolve, reject) => {
    const child = spawnLogged(command, args, options);
    child.on('exit', (code) => {
      const index = children.indexOf(child);
      if (index >= 0) children.splice(index, 1);
      if (code === 0) {
        resolve();
      } else {
        reject(new Error(`${options.label || command} exited with ${code}`));
      }
    });
  });
}

function waitFor(url, deadlineMs = 30_000) {
  const deadline = Date.now() + deadlineMs;
  return new Promise((resolve, reject) => {
    const tick = () => {
      const req = http.get(url, (res) => {
        res.resume();
        if (res.statusCode && res.statusCode < 500) {
          resolve();
          return;
        }
        retry();
      });
      req.on('error', retry);
      req.setTimeout(1000, () => req.destroy());
    };
    const retry = () => {
      if (Date.now() > deadline) {
        reject(new Error(`Timed out waiting for ${url}`));
        return;
      }
      setTimeout(tick, 250);
    };
    tick();
  });
}

function assertTCPPortFree(url) {
  const parsed = new URL(url);
  const host = parsed.hostname;
  const port = Number(parsed.port || (parsed.protocol === 'https:' ? 443 : 80));
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.once('error', (error) => {
      reject(new Error(`${host}:${port} is already in use before live smoke startup: ${error.message}`));
    });
    server.once('listening', () => {
      server.close(() => resolve());
    });
    server.listen(port, host);
  });
}

function stopChildren() {
  for (const child of children.splice(0)) {
    if (child.killed) continue;
    if (process.platform === 'win32') {
      spawn('taskkill', ['/PID', String(child.pid), '/T', '/F'], {
        stdio: 'ignore',
        windowsHide: true
      });
    } else {
      child.kill();
    }
  }
}

async function runPlaywright() {
  return new Promise((resolve) => {
    const child = spawn(node, [
      playwrightCli,
      'test',
      '--config',
      'tests/e2e/playwright-live.config.ts',
      '--reporter=line'
    ], {
      cwd: root,
      env: {
        ...process.env,
        LIVE_AUCTION_H5_URL: h5URL,
        LIVE_AUCTION_PC_URL: pcURL
      },
      stdio: 'inherit',
      windowsHide: true
    });
    child.on('exit', (code) => resolve(code ?? 1));
  });
}

async function assertEvidenceWritten() {
  const raw = await fs.readFile(evidencePath, 'utf8');
  const evidence = JSON.parse(raw);
  if (evidence.result !== 'PASS' || evidence.no_browser_route_mocks !== true || !evidence.smoke_auction_id || !evidence.smoke_item_id) {
    throw new Error(`P10 smoke evidence is incomplete at ${evidencePath}`);
  }
}

try {
  await assertTCPPortFree(backendURL);
  await assertTCPPortFree(h5URL);
  await assertTCPPortFree(pcURL);

  const backendEnv = {
    HTTP_ADDR: '127.0.0.1:18080',
    APP_ENV: 'test',
    ALLOW_MOCK_AUTH: 'true'
  };
  await run('go', ['run', './cmd/p0smokeseed'], {
    cwd: backendDir,
    env: backendEnv,
    label: 'seed'
  });

  spawnLogged('go', ['run', './cmd/server'], {
    cwd: backendDir,
    env: backendEnv,
    label: 'backend'
  });
  await waitFor(`${backendURL}/readyz`);

  spawnLogged(node, [
    viteCli,
    'frontend/mobile-h5',
    '--host',
    '127.0.0.1',
    '--port',
    new URL(h5URL).port
  ], {
    env: {
      LIVE_AUCTION_API_TARGET: backendURL
    },
    label: `vite:${new URL(h5URL).port}`
  });
  await waitFor(h5URL);

  spawnLogged(node, [
    viteCli,
    'frontend/pc-console',
    '--host',
    '127.0.0.1',
    '--port',
    new URL(pcURL).port
  ], {
    env: {
      LIVE_AUCTION_API_TARGET: backendURL
    },
    label: `vite:${new URL(pcURL).port}`
  });
  await waitFor(pcURL);

  const code = await runPlaywright();
  if (code === 0) {
    await assertEvidenceWritten();
  }
  stopChildren();
  await new Promise((resolve) => setTimeout(resolve, 1000));
  process.exit(code);
} catch (error) {
  stopChildren();
  console.error(error);
  process.exit(1);
}
