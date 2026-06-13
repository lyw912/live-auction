import fs from 'node:fs';
import path from 'node:path';

const docsRoot = path.resolve('docs');
const generatedStart = '<!-- excalidraw-generated:start -->';
const generatedEnd = '<!-- excalidraw-generated:end -->';

function walkMarkdown(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'assets') continue;
      walkMarkdown(full, out);
    } else if (entry.isFile() && entry.name.endsWith('.md')) {
      out.push(full);
    }
  }
  return out;
}

function svgWidth(svgPath) {
  const svg = fs.readFileSync(svgPath, 'utf8');
  const width = svg.match(/\bwidth="([^"]+)"/)?.[1];
  const viewBox = svg.match(/\bviewBox="([^"]+)"/)?.[1];
  if (width && /^\d+(?:\.\d+)?$/.test(width)) return Math.ceil(Number(width));
  if (viewBox) {
    const parts = viewBox.trim().split(/\s+/).map(Number);
    if (parts.length === 4 && Number.isFinite(parts[2])) return Math.ceil(parts[2]);
  }
  return 1200;
}

function htmlEscape(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

let updated = 0;
for (const mdFile of walkMarkdown(docsRoot)) {
  let text = fs.readFileSync(mdFile, 'utf8');
  text = text.replace(new RegExp(`${generatedStart}[\\s\\S]*?${generatedEnd}`, 'g'), (block) => {
    const exc = block.match(/href="([^"]+\.excalidraw)"/)?.[1];
    const svg = block.match(/src="([^"]+\.svg)"/)?.[1];
    const alt = block.match(/alt="([^"]*)"/)?.[1] ?? 'Excalidraw diagram';
    if (!exc || !svg) return block;
    const svgPath = path.resolve(path.dirname(mdFile), svg);
    const width = svgWidth(svgPath);
    updated += 1;
    return `${generatedStart}\n<div class="excalidraw-diagram" style="overflow-x: auto; width: 100%; margin: 12px 0 18px 0; padding: 12px 12px 14px 12px; border: 1px solid #e2e8f0; border-radius: 8px; background: #fffdf7;">\n  <a href="${htmlEscape(exc)}">打开可编辑 Excalidraw 源文件</a>\n  <br />\n  <img src="${htmlEscape(svg)}" alt="${htmlEscape(alt)}" loading="lazy" width="${width}" style="display: block; width: ${width}px; max-width: none !important; height: auto;" />\n</div>\n${generatedEnd}`;
  });
  fs.writeFileSync(mdFile, text, 'utf8');
}

console.log(`Updated ${updated} Excalidraw image widths.`);
