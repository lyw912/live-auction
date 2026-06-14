import { spawn } from 'node:child_process';
import http from 'node:http';
import net from 'node:net';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const node = process.execPath;
const viteCli = join(root, 'node_modules', 'vite', 'bin', 'vite.js');
const playwrightCli = join(root, 'node_modules', '@playwright', 'test', 'cli.js');
const servers = [];

function startVite(appPath, port) {
  const child = spawn(node, [viteCli, appPath, '--host', '127.0.0.1', '--port', String(port), '--strictPort'], {
    cwd: root,
    stdio: ['ignore', 'pipe', 'pipe'],
    windowsHide: true
  });
  child.stdout.on('data', (chunk) => process.stdout.write(`[vite:${port}] ${chunk}`));
  child.stderr.on('data', (chunk) => process.stderr.write(`[vite:${port}] ${chunk}`));
  servers.push(child);
}

function canListen(port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once('error', () => resolve(false));
    server.once('listening', () => {
      server.close(() => resolve(true));
    });
    server.listen(port, '127.0.0.1');
  });
}

async function findAvailablePort(startPort) {
  for (let port = startPort; port < startPort + 80; port += 1) {
    if (await canListen(port)) return port;
  }
  throw new Error(`No available localhost port found from ${startPort}`);
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

function runPlaywright(h5Port, pcPort) {
  return new Promise((resolve) => {
    const child = spawn(node, [playwrightCli, 'test', '--no-deps', '--workers=1', '--reporter=line'], {
      cwd: root,
      env: {
        ...process.env,
        H5_PORT: String(h5Port),
        PC_PORT: String(pcPort)
      },
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
  const h5Port = Number(process.env.H5_PORT || 5276);
  const pcStartPort = Number(process.env.PC_PORT || 5277);
  const selectedH5Port = await findAvailablePort(h5Port);
  const selectedPCPort = await findAvailablePort(Math.max(pcStartPort, selectedH5Port + 1));
  startVite('frontend/mobile-h5', selectedH5Port);
  startVite('frontend/pc-console', selectedPCPort);
  await Promise.all([
    waitFor(`http://127.0.0.1:${selectedH5Port}`),
    waitFor(`http://127.0.0.1:${selectedPCPort}`)
  ]);
  const code = await runPlaywright(selectedH5Port, selectedPCPort);
  stopServers();
  process.exit(code);
} catch (error) {
  stopServers();
  console.error(error);
  process.exit(1);
}
