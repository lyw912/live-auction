import { spawn } from 'node:child_process';
import http from 'node:http';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const node = process.execPath;
const viteCli = join(root, 'node_modules', 'vite', 'bin', 'vite.js');
const playwrightCli = join(root, 'node_modules', '@playwright', 'test', 'cli.js');
const servers = [];

function startVite(appPath, port) {
  const child = spawn(node, [viteCli, appPath, '--host', '127.0.0.1', '--port', String(port)], {
    cwd: root,
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true
  });
  child.stdout.on('data', (chunk) => process.stdout.write(`[vite:${port}] ${chunk}`));
  child.stderr.on('data', (chunk) => process.stderr.write(`[vite:${port}] ${chunk}`));
  servers.push(child);
}

function waitFor(url, deadlineMs = 20_000) {
  const deadline = Date.now() + deadlineMs;
  return new Promise((resolve, reject) => {
    const tick = () => {
      const req = http.get(url, (res) => {
        res.resume();
        resolve();
      });
      req.on('error', () => {
        if (Date.now() > deadline) {
          reject(new Error(`Timed out waiting for ${url}`));
          return;
        }
        setTimeout(tick, 250);
      });
      req.setTimeout(1000, () => req.destroy());
    };
    tick();
  });
}

function runPlaywright() {
  return new Promise((resolve) => {
    const child = spawn(node, [playwrightCli, 'test', '--no-deps', '--workers=1', '--reporter=line'], {
      cwd: root,
      stdio: 'inherit',
      windowsHide: true
    });
    child.on('exit', (code) => resolve(code ?? 1));
  });
}

function stopServers() {
  for (const server of servers) {
    if (!server.killed) {
      server.kill();
    }
  }
}

try {
  startVite('frontend/mobile-h5', 5173);
  startVite('frontend/pc-console', 5174);
  await Promise.all([
    waitFor('http://127.0.0.1:5173'),
    waitFor('http://127.0.0.1:5174')
  ]);
  const code = await runPlaywright();
  stopServers();
  process.exit(code);
} catch (error) {
  stopServers();
  console.error(error);
  process.exit(1);
}
