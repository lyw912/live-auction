import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';

const root = path.resolve(process.cwd());
const host = process.env.DOCS_PREVIEW_HOST || '127.0.0.1';
const port = Number(process.env.DOCS_PREVIEW_PORT || 18181);

const contentTypes = new Map([
  ['.css', 'text/css; charset=utf-8'],
  ['.excalidraw', 'application/json; charset=utf-8'],
  ['.html', 'text/html; charset=utf-8'],
  ['.js', 'text/javascript; charset=utf-8'],
  ['.json', 'application/json; charset=utf-8'],
  ['.md', 'text/markdown; charset=utf-8'],
  ['.png', 'image/png'],
  ['.svg', 'image/svg+xml; charset=utf-8'],
]);

function resolveRequest(urlPath) {
  const clean = decodeURIComponent(urlPath.split('?')[0]).replace(/^\/+/, '') || 'docs/README.md';
  const full = path.resolve(root, clean);
  if (full !== root && !full.startsWith(`${root}${path.sep}`)) {
    return null;
  }
  return full;
}

const server = http.createServer((req, res) => {
  const file = resolveRequest(req.url ?? '/');
  if (!file) {
    res.writeHead(403);
    res.end('Forbidden');
    return;
  }

  fs.readFile(file, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end('Not found');
      return;
    }

    res.writeHead(200, {
      'content-type': contentTypes.get(path.extname(file).toLowerCase()) ?? 'application/octet-stream',
      'cache-control': 'no-store',
    });
    res.end(data);
  });
});

server.listen(port, host, () => {
  console.log(`Docs preview server listening at http://${host}:${port}/`);
  console.log(`Serving ${root}`);
});
