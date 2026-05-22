import { spawn } from 'node:child_process';
import http from 'node:http';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const backendDir = join(root, 'backend');
const node = process.execPath;
const viteCli = join(root, 'node_modules', 'vite', 'bin', 'vite.js');
const playwrightCli = join(root, 'node_modules', '@playwright', 'test', 'cli.js');
const backendURL = process.env.LIVE_AUCTION_API_TARGET || 'http://127.0.0.1:18080';
const h5URL = process.env.LIVE_AUCTION_H5_URL || 'http://127.0.0.1:5175';
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

function stopChildren() {
  for (const child of children.splice(0)) {
    if (!child.killed) child.kill();
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
        LIVE_AUCTION_H5_URL: h5URL
      },
      stdio: 'inherit',
      windowsHide: true
    });
    child.on('exit', (code) => resolve(code ?? 1));
  });
}

try {
  const backendEnv = {
    HTTP_ADDR: '127.0.0.1:18080'
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
    '5175'
  ], {
    env: {
      LIVE_AUCTION_API_TARGET: backendURL
    },
    label: 'vite:5175'
  });
  await waitFor(h5URL);

  const code = await runPlaywright();
  stopChildren();
  process.exit(code);
} catch (error) {
  stopChildren();
  console.error(error);
  process.exit(1);
}
