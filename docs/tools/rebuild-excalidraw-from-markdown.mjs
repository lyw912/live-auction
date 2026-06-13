import fs from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const docsRoot = path.resolve(__dirname, '..');
const assetsDir = path.join(docsRoot, 'assets', 'excalidraw');
const generatedStart = '<!-- excalidraw-generated:start -->';
const generatedEnd = '<!-- excalidraw-generated:end -->';
const shouldExportSvg = process.argv.includes('--export-svg') || process.env.EXCALIDRAW_EXPORT_SVG === '1';

const palette = {
  ink: '#1e1e1e',
  muted: '#5f6368',
  faint: '#f8f9fa',
  white: '#fffdf7',
  slateFill: '#f8f9fa',
  slateStroke: '#5f6368',
  blueFill: '#d0ebff',
  blueStroke: '#1971c2',
  cyanFill: '#c5f6fa',
  cyanStroke: '#0b7285',
  greenFill: '#d3f9d8',
  greenStroke: '#2b8a3e',
  amberFill: '#fff3bf',
  amberStroke: '#e67700',
  roseFill: '#ffe3e3',
  roseStroke: '#c92a2a',
  violetFill: '#e5dbff',
  violetStroke: '#6741d9',
  indigoFill: '#dbe4ff',
  indigoStroke: '#364fc7',
};

const accents = [
  [palette.blueFill, palette.blueStroke],
  [palette.greenFill, palette.greenStroke],
  [palette.amberFill, palette.amberStroke],
  [palette.violetFill, palette.violetStroke],
  [palette.cyanFill, palette.cyanStroke],
  [palette.roseFill, palette.roseStroke],
  [palette.indigoFill, palette.indigoStroke],
];

let idCounter = 0;

function eid(prefix) {
  idCounter += 1;
  return `${prefix}_${idCounter.toString(36)}`;
}

function nonce() {
  idCounter += 1;
  return 100000000 + idCounter * 7919;
}

function resetIds() {
  idCounter = 0;
}

function htmlEscape(value) {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');
}

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

function markdownPath(fromFile, toFile) {
  const rel = path.relative(path.dirname(fromFile), toFile).replace(/\\/g, '/');
  return rel.startsWith('.') ? rel : `./${rel}`;
}

function visualWidth(char) {
  return /[\u2e80-\u9fff\uff00-\uffef]/.test(char) ? 2 : 1;
}

function wrapLabel(value, maxWidth = 14, maxLines = 2) {
  const raw = String(value ?? '').replace(/\\n/g, '\n').trim();
  if (!raw) return '';
  const manual = raw.split('\n');
  const lines = [];

  for (const part of manual) {
    const tokens = part.includes(' ') ? part.split(/\s+/).filter(Boolean) : Array.from(part);
    let line = '';
    let width = 0;
    for (const token of tokens) {
      const tokenWidth = Array.from(token).reduce((sum, char) => sum + visualWidth(char), 0);
      const sep = part.includes(' ') && line ? ' ' : '';
      const sepWidth = sep ? 1 : 0;
      if (line && width + sepWidth + tokenWidth > maxWidth) {
        lines.push(line);
        line = token;
        width = tokenWidth;
      } else {
        line += `${sep}${token}`;
        width += sepWidth + tokenWidth;
      }
    }
    if (line) lines.push(line);
  }

  if (lines.length > 1 && /^[。,.，;；:：.!?！？]+$/.test(lines.at(-1))) {
    lines[lines.length - 2] += lines.at(-1);
    lines.pop();
  }

  if (lines.length <= maxLines) return lines.join('\n');
  const kept = lines.slice(0, maxLines);
  const last = Array.from(kept[kept.length - 1]);
  kept[kept.length - 1] = `${last.slice(0, Math.max(1, last.length - 1)).join('')}…`;
  return kept.join('\n');
}

function elementBase(type, x, y, width, height, style = {}) {
  return {
    id: eid(type),
    type,
    x,
    y,
    width,
    height,
    angle: 0,
    strokeColor: style.stroke ?? palette.ink,
    backgroundColor: style.fill ?? 'transparent',
    fillStyle: style.fillStyle ?? 'hachure',
    strokeWidth: style.strokeWidth ?? 2,
    strokeStyle: style.strokeStyle ?? 'solid',
    roughness: style.roughness ?? 2,
    opacity: style.opacity ?? 100,
    groupIds: style.groupIds ?? [],
    frameId: null,
    roundness: type === 'rectangle' || type === 'diamond' ? { type: 3 } : null,
    seed: nonce(),
    version: 1,
    versionNonce: nonce(),
    isDeleted: false,
    boundElements: null,
    updated: 1,
    link: null,
    locked: false,
  };
}

function textElement(label, x, y, width, fontSize = 22, color = palette.ink, align = 'center', style = {}) {
  const value = String(label ?? '');
  const lineCount = Math.max(1, value.split('\n').length);
  return {
    ...elementBase('text', x, y, width, lineCount * fontSize * 1.25, { stroke: color, fill: 'transparent', groupIds: style.groupIds }),
    strokeColor: color,
    backgroundColor: 'transparent',
    fontSize,
    fontFamily: style.fontFamily ?? 1,
    text: value,
    textAlign: align,
    verticalAlign: 'middle',
    containerId: null,
    originalText: value,
    autoResize: false,
    lineHeight: 1.25,
  };
}

function titleBlock(elements, title, subtitle, width) {
  elements.push(textElement(title, 64, 42, width - 128, 34, palette.ink, 'left'));
  if (subtitle) elements.push(textElement(subtitle, 66, 88, width - 132, 18, palette.muted, 'left'));
}

function rect(label, x, y, width, height, style = {}) {
  const groupIds = [eid('group')];
  const fill = style.fill ?? palette.white;
  const stroke = style.stroke ?? palette.slateStroke;
  const box = elementBase('rectangle', x, y, width, height, {
    fill,
    stroke,
    fillStyle: style.fillStyle,
    strokeWidth: style.strokeWidth ?? 2,
    strokeStyle: style.strokeStyle,
    roughness: style.roughness,
    groupIds,
  });
  const fontSize = style.fontSize ?? 20;
  const labelText = wrapLabel(label, style.wrap ?? 13, style.maxLines ?? 2);
  const labelLines = labelText.split('\n').length;
  const labelHeight = labelLines * fontSize * 1.25;
  const labelEl = textElement(
    labelText,
    x + (style.padX ?? 14),
    y + height / 2 - labelHeight / 2,
    width - (style.padX ?? 14) * 2,
    fontSize,
    style.color ?? palette.ink,
    style.align ?? 'center',
    { groupIds },
  );
  return [box, labelEl];
}

function ellipse(label, x, y, width, height, style = {}) {
  const groupIds = [eid('group')];
  const shape = elementBase('ellipse', x, y, width, height, {
    fill: style.fill ?? palette.white,
    stroke: style.stroke ?? palette.slateStroke,
    fillStyle: style.fillStyle,
    strokeWidth: style.strokeWidth ?? 2,
    roughness: style.roughness,
    groupIds,
  });
  const fontSize = style.fontSize ?? 19;
  const labelText = wrapLabel(label, style.wrap ?? 10, style.maxLines ?? 2);
  const labelHeight = labelText.split('\n').length * fontSize * 1.25;
  const labelEl = textElement(labelText, x + 12, y + height / 2 - labelHeight / 2, width - 24, fontSize, style.color ?? palette.ink, 'center', { groupIds });
  return [shape, labelEl];
}

function diamond(label, x, y, width, height, style = {}) {
  const groupIds = [eid('group')];
  const shape = elementBase('diamond', x, y, width, height, {
    fill: style.fill ?? palette.amberFill,
    stroke: style.stroke ?? palette.amberStroke,
    fillStyle: style.fillStyle,
    strokeWidth: style.strokeWidth ?? 2,
    roughness: style.roughness,
    groupIds,
  });
  const fontSize = style.fontSize ?? 18;
  const labelText = wrapLabel(label, style.wrap ?? 9, style.maxLines ?? 3);
  const labelHeight = labelText.split('\n').length * fontSize * 1.25;
  const labelEl = textElement(labelText, x + 22, y + height / 2 - labelHeight / 2, width - 44, fontSize, style.color ?? palette.ink, 'center', { groupIds });
  return [shape, labelEl];
}

function arrow(x1, y1, x2, y2, label = '', style = {}) {
  const arr = {
    ...elementBase('arrow', x1, y1, x2 - x1, y2 - y1, {
      stroke: style.stroke ?? palette.slateStroke,
      strokeWidth: style.strokeWidth ?? 2.5,
      strokeStyle: style.strokeStyle,
    }),
    points: [[0, 0], [x2 - x1, y2 - y1]],
    lastCommittedPoint: null,
    startBinding: null,
    endBinding: null,
    startArrowhead: Object.hasOwn(style, 'startArrowhead') ? style.startArrowhead : null,
    endArrowhead: Object.hasOwn(style, 'endArrowhead') ? style.endArrowhead : 'arrow',
  };
  const out = [arr];
  if (label) {
    out.push(textElement(wrapLabel(label, 12, 1), (x1 + x2) / 2 - 58, (y1 + y2) / 2 - 24, 116, 15, style.labelColor ?? palette.muted));
  }
  return out;
}

function routedArrow(points, label = '', style = {}) {
  const [x0, y0] = points[0];
  const relPoints = points.map(([x, y]) => [x - x0, y - y0]);
  const last = points.at(-1);
  const arr = {
    ...elementBase('arrow', x0, y0, last[0] - x0, last[1] - y0, {
      stroke: style.stroke ?? palette.slateStroke,
      strokeWidth: style.strokeWidth ?? 2.5,
      strokeStyle: style.strokeStyle,
    }),
    points: relPoints,
    lastCommittedPoint: null,
    startBinding: null,
    endBinding: null,
    startArrowhead: Object.hasOwn(style, 'startArrowhead') ? style.startArrowhead : null,
    endArrowhead: Object.hasOwn(style, 'endArrowhead') ? style.endArrowhead : 'arrow',
  };
  const out = [arr];
  if (label) {
    const segment = points.length > 2 ? [points[0], points[1]] : [points[0], points.at(-1)];
    out.push(textElement(
      wrapLabel(label, 10, 1),
      (segment[0][0] + segment[1][0]) / 2 - 48,
      (segment[0][1] + segment[1][1]) / 2 - 26,
      96,
      15,
      style.labelColor ?? palette.muted,
    ));
  }
  return out;
}

function frame(label, x, y, width, height, style = {}) {
  const stroke = style.stroke ?? palette.slateStroke;
  const box = elementBase('rectangle', x, y, width, height, {
    fill: style.fill ?? 'transparent',
    stroke,
    fillStyle: style.fillStyle ?? 'hachure',
    strokeWidth: style.strokeWidth ?? 2,
    strokeStyle: style.strokeStyle ?? 'dashed',
    roughness: style.roughness ?? 1.4,
  });
  return [
    box,
    textElement(label, x + 18, y + 14, width - 36, style.fontSize ?? 20, stroke, 'left'),
  ];
}

function note(label, x, y, width, height, style = {}) {
  return rect(label, x, y, width, height, {
    fill: style.fill ?? palette.faint,
    stroke: style.stroke ?? '#cbd5e1',
    fontSize: style.fontSize ?? 17,
    wrap: style.wrap ?? 16,
    maxLines: style.maxLines ?? 3,
    align: style.align ?? 'left',
    padX: 18,
  });
}

function badge(label, x, y, style = {}) {
  return ellipse(label, x, y, style.width ?? 58, style.height ?? 42, {
    fill: style.fill ?? palette.slateFill,
    stroke: style.stroke ?? palette.slateStroke,
    fontSize: style.fontSize ?? 17,
    wrap: 5,
    maxLines: 1,
  });
}

function scene(elements, width, height, meta = {}) {
  return {
    type: 'excalidraw',
    version: 2,
    source: 'https://excalidraw.com',
    elements,
    appState: {
      gridSize: null,
      viewBackgroundColor: palette.white,
      currentItemFontFamily: 1,
      exportBackground: true,
      exportWithDarkMode: false,
      exportEmbedScene: true,
    },
    files: {},
    meta: { generatedBy: 'docs/tools/rebuild-excalidraw-from-markdown.mjs', canvas: { width, height }, ...meta },
  };
}

function buildArchitecture(title, subtitle, zones, options = {}) {
  const width = 1340;
  const height = options.height ?? 780;
  const elements = [];
  titleBlock(elements, title, subtitle, width);
  const top = 148;
  const zoneWidth = Math.floor((width - 132 - (zones.length - 1) * 18) / zones.length);
  const gap = 18;
  zones.forEach((zone, index) => {
    const [fill, stroke] = accents[index % accents.length];
    const x = 48 + index * (zoneWidth + gap);
    elements.push(...frame(zone.label, x, top, zoneWidth, 462, {
      stroke,
      fill: '#ffffff',
      fillStyle: 'solid',
      roughness: 1.7,
    }));
    zone.nodes.slice(0, 4).forEach((node, nodeIndex) => {
      const y = top + 82 + nodeIndex * 84;
      elements.push(...rect(node, x + 24, y, zoneWidth - 48, 58, {
        fill,
        stroke,
        fontSize: 18,
        wrap: 12,
        maxLines: 2,
        roughness: 1.9,
      }));
    });
    if (index < zones.length - 1) {
      const y = top + 250 + (index % 2) * 54;
      elements.push(...arrow(x + zoneWidth + 2, y, x + zoneWidth + gap - 4, y, zone.edge ?? '', { strokeWidth: 3 }));
    }
  });
  if (options.footer) {
    elements.push(...note(options.footer, 92, 656, width - 184, 58, { wrap: 52, maxLines: 2, fontSize: 18, fill: '#fff7ed', stroke: '#fb923c' }));
  }
  return { scene: scene(elements, width, height, { layout: 'architecture-zones' }), width, height };
}

function buildDocMap() {
  const width = 1320;
  const height = 860;
  const elements = [];
  titleBlock(elements, '文档库导航地图', '父子层级、最小闭环、证据和答辩材料可以互相跳转', width);
  elements.push(...rect('README\n入口', 70, 360, 150, 88, { fill: palette.blueFill, stroke: palette.blueStroke, fontSize: 24, wrap: 8 }));
  const groups = [
    ['理解项目', ['00 项目总览', '01 架构', '02 领域模型'], palette.greenFill, palette.greenStroke],
    ['下钻闭环', ['03 后端 L4', '04 实时 L4', '05 前端 L4'], palette.amberFill, palette.amberStroke],
    ['证明能力', ['06 可观测', '07 证据', '08 风险测试'], palette.violetFill, palette.violetStroke],
    ['答辩收口', ['09 评委追问', '10 附录', 'submission 覆盖'], palette.cyanFill, palette.cyanStroke],
  ];
  groups.forEach(([label, items, fill, stroke], groupIndex) => {
    const x = 320 + (groupIndex % 2) * 445;
    const y = 160 + Math.floor(groupIndex / 2) * 310;
    elements.push(...frame(label, x, y, 380, 250, { stroke, fill: '#ffffff' }));
    items.forEach((item, index) => {
      elements.push(...rect(item, x + 44, y + 72 + index * 56, 292, 42, { fill, stroke, fontSize: 18, wrap: 13, maxLines: 1 }));
    });
    elements.push(...arrow(220, 404, x, y + 126, '进入', { strokeWidth: 2.5 }));
  });
  elements.push(...note('读法：先看 L0/L1 建立全局，再进 L4 文档讲一个输入到结果的最小闭环；最后用证据和答辩文档防追问。', 96, 740, 1128, 62, { wrap: 48, maxLines: 2, fill: '#ecfeff', stroke: palette.cyanStroke }));
  return { scene: scene(elements, width, height, { layout: 'doc-map' }), width, height };
}

function buildIndex(title, subtitle, children, options = {}) {
  const width = 1160;
  const height = 700;
  const elements = [];
  titleBlock(elements, title, subtitle ?? '父文档进入若干 L4 最小闭环，按追问继续下钻', width);
  elements.push(...rect(options.parent ?? '父文档', 72, 292, 190, 94, { fill: palette.blueFill, stroke: palette.blueStroke, fontSize: 24, wrap: 7 }));
  elements.push(...frame('子文档 / 最小闭环', 360, 150, 720, 430, { stroke: palette.slateStroke, fill: '#ffffff' }));
  children.slice(0, 8).forEach((child, index) => {
    const col = index % 2;
    const row = Math.floor(index / 2);
    const [fill, stroke] = accents[index % accents.length];
    const x = 410 + col * 320;
    const y = 218 + row * 76;
    elements.push(...rect(child, x, y, 240, 54, { fill, stroke, fontSize: 18, wrap: 12, maxLines: 2 }));
    elements.push(...arrow(262, 338, x, y + 27, '', { strokeWidth: 2 }));
  });
  elements.push(...note('答辩技巧：每个子文档都按“输入 -> 判断 -> 状态 -> 异常 -> 证据”讲，不要只背模块名。', 106, 610, 930, 52, { wrap: 38, maxLines: 2, fill: '#fff7ed', stroke: '#fb923c' }));
  return { scene: scene(elements, width, height, { layout: 'index-map' }), width, height };
}

function buildSwimlane(title, subtitle, lanes, steps, options = {}) {
  const width = 1480;
  const height = options.height ?? 820;
  const elements = [];
  titleBlock(elements, title, subtitle, width);
  const laneLabelW = 188;
  const x0 = 250;
  const cardW = steps.length >= 7 ? 150 : 168;
  const usableW = width - x0 - 118;
  const gap = steps.length > 1
    ? Math.min(58, Math.max(30, (usableW - cardW * steps.length) / (steps.length - 1)))
    : 0;
  const y0 = 168;
  const laneH = 158;
  lanes.forEach((lane, index) => {
    const [fill, stroke] = accents[index % accents.length];
    const y = y0 + index * laneH;
    elements.push(...frame('', 48, y - 24, width - 96, laneH - 16, {
      stroke,
      fill: '#ffffff',
      fillStyle: 'solid',
      strokeStyle: 'dashed',
      roughness: 1.4,
    }));
    elements.push(...rect(lane, 74, y + 14, laneLabelW - 38, 74, {
      fill,
      stroke,
      fontSize: 19,
      wrap: 8,
      maxLines: 3,
      fillStyle: 'solid',
      roughness: 1.8,
    }));
  });
  const coords = [];
  steps.forEach((step, index) => {
    const laneIndex = Math.min(lanes.length - 1, step.lane ?? 0);
    const x = x0 + index * (cardW + gap);
    const y = y0 + laneIndex * laneH + 28;
    const [fill, stroke] = accents[laneIndex % accents.length];
    const shape = step.decision
      ? diamond(step.label, x + 14, y - 12, cardW - 10, 88, { fill, stroke, fontSize: 16, wrap: 10, maxLines: 3, roughness: 2.1 })
      : rect(step.label, x, y, cardW, 76, { fill, stroke, fontSize: 17, wrap: 11, maxLines: 3, roughness: 1.9 });
    elements.push(...shape);
    elements.push(...badge(String(index + 1), x + 10, y - 34, { fill: palette.white, stroke, width: 34, height: 28, fontSize: 13 }));
    coords.push({ x, y, cx: x + cardW / 2, cy: y + 38 });
  });
  for (let i = 0; i < coords.length - 1; i += 1) {
    const from = coords[i];
    const to = coords[i + 1];
    const startX = from.x + cardW + 8;
    const endX = to.x - 8;
    const sameLane = Math.abs(from.cy - to.cy) < 4;
    if (sameLane) {
      elements.push(...arrow(startX, from.cy, endX, to.cy, steps[i].edge ?? '', { strokeWidth: 2.6 }));
    } else {
      const routeX = Math.min(endX - 18, Math.max(startX + 28, (startX + endX) / 2));
      const exitY = from.cy;
      const enterY = to.cy;
      elements.push(...routedArrow([
        [startX, exitY],
        [routeX, exitY],
        [routeX, enterY],
        [endX, enterY],
      ], steps[i].edge ?? '', { strokeWidth: 2.3, strokeStyle: 'dashed' }));
    }
  }
  if (options.callout) {
    elements.push(...note(options.callout, width - 456, height - 118, 378, 74, { wrap: 19, maxLines: 3, fill: '#fff7ed', stroke: '#fb923c' }));
  }
  return { scene: scene(elements, width, height, { layout: 'swimlane' }), width, height };
}

function buildPipeline(title, subtitle, nodes, options = {}) {
  const width = 1240;
  const height = options.height ?? 700;
  const elements = [];
  titleBlock(elements, title, subtitle, width);
  const y = 288;
  const startX = 72;
  const cardW = 150;
  const gap = 48;
  nodes.forEach((node, index) => {
    const [fill, stroke] = accents[index % accents.length];
    const x = startX + index * (cardW + gap);
    const shape = node.decision
      ? diamond(node.label, x + 8, y - 22, cardW - 16, 96, { fill, stroke, fontSize: 17, wrap: 8, maxLines: 3 })
      : rect(node.label, x, y, cardW, 72, { fill, stroke, fontSize: 18, wrap: 9, maxLines: 2 });
    elements.push(...shape);
    if (index < nodes.length - 1) elements.push(...arrow(x + cardW + 4, y + 36, x + cardW + gap - 6, y + 36, node.edge ?? '', { strokeWidth: 2.7 }));
    if (node.note) elements.push(...note(node.note, x - 6, y + 112, cardW + 12, 68, { fontSize: 15, wrap: 9, maxLines: 3, fill: palette.faint }));
  });
  if (options.bottom) {
    elements.push(...note(options.bottom, 90, 586, 1020, 58, { wrap: 44, maxLines: 2, fill: '#ecfeff', stroke: palette.cyanStroke }));
  }
  return { scene: scene(elements, width, height, { layout: 'pipeline' }), width, height };
}

function buildLoopCards(title, subtitle, cards, options = {}) {
  const width = 1180;
  const height = options.height ?? 760;
  const elements = [];
  titleBlock(elements, title, subtitle, width);
  cards.slice(0, 6).forEach((card, index) => {
    const col = index % 3;
    const row = Math.floor(index / 3);
    const [fill, stroke] = accents[index % accents.length];
    const x = 86 + col * 350;
    const y = 174 + row * 214;
    elements.push(...frame(card.title, x, y, 286, 168, { stroke, fill: '#ffffff' }));
    elements.push(...rect(card.main, x + 24, y + 58, 238, 48, { fill, stroke, fontSize: 19, wrap: 12, maxLines: 1 }));
    elements.push(textElement(wrapLabel(card.detail, 18, 3), x + 30, y + 122, 226, 16, palette.muted, 'left'));
  });
  if (options.footer) elements.push(...note(options.footer, 102, 656, 940, 54, { wrap: 42, maxLines: 2, fill: '#fff7ed', stroke: '#fb923c' }));
  return { scene: scene(elements, width, height, { layout: 'loop-cards' }), width, height };
}

function buildMatrix(title, subtitle, columns, rows, options = {}) {
  const width = 1360;
  const height = options.height ?? Math.max(660, 230 + rows.length * 78);
  const elements = [];
  titleBlock(elements, title, subtitle, width);
  const x0 = 70;
  const y0 = 164;
  const usable = width - x0 * 2;
  const colWidths = columns.map((_, index) => {
    if (columns.length === 4) return index === 0 ? 210 : Math.floor((usable - 210) / 3);
    return index === 0 ? 220 : Math.floor((usable - 220) / Math.max(1, columns.length - 1));
  });
  let x = x0;
  columns.forEach((column, index) => {
    elements.push(...rect(column, x, y0, colWidths[index], 54, {
      fill: palette.slateFill,
      stroke: palette.slateStroke,
      fontSize: 18,
      wrap: index === 0 ? 11 : 18,
      maxLines: 1,
      fillStyle: 'solid',
      roughness: 1.2,
    }));
    x += colWidths[index];
  });
  rows.forEach((row, rowIndex) => {
    let cx = x0;
    const y = y0 + 54 + rowIndex * 76;
    row.forEach((cell, colIndex) => {
      const [fill, stroke] = colIndex === 0 ? accents[rowIndex % accents.length] : [palette.white, '#cbd5e1'];
      elements.push(...rect(cell, cx, y, colWidths[colIndex], 76, {
        fill,
        stroke,
        fontSize: colIndex === 0 ? 17 : 16,
        wrap: colIndex === 0 ? 10 : 22,
        maxLines: 2,
        fillStyle: colIndex === 0 ? 'hachure' : 'solid',
        roughness: colIndex === 0 ? 1.8 : 0.9,
      }));
      cx += colWidths[colIndex];
    });
  });
  if (options.footer) elements.push(...note(options.footer, 92, height - 86, width - 184, 54, { wrap: 58, maxLines: 2, fill: '#ecfeff', stroke: palette.cyanStroke }));
  return { scene: scene(elements, width, height, { layout: 'matrix' }), width, height };
}

function buildStateMachine(title, subtitle, states, edges, options = {}) {
  const width = 1260;
  const height = 760;
  const elements = [];
  titleBlock(elements, title, subtitle, width);
  const center = { x: 580, y: 390 };
  const radiusX = 410;
  const radiusY = 210;
  const positions = states.map((state, index) => {
    const angle = (-90 + index * (360 / states.length)) * Math.PI / 180;
    return {
      state,
      x: center.x + Math.cos(angle) * radiusX - 80,
      y: center.y + Math.sin(angle) * radiusY - 34,
    };
  });
  positions.forEach(({ state, x, y }, index) => {
    const [fill, stroke] = accents[index % accents.length];
    elements.push(...rect(state, x, y, 166, 70, { fill, stroke, fontSize: 18, wrap: 8, maxLines: 2 }));
  });
  edges.forEach(([fromIndex, toIndex, label]) => {
    const from = positions[fromIndex];
    const to = positions[toIndex];
    elements.push(...arrow(from.x + 83, from.y + 35, to.x + 83, to.y + 35, label, { strokeWidth: 2.2 }));
  });
  if (options.center) {
    elements.push(...note(options.center, 885, 604, 300, 82, { wrap: 16, maxLines: 3, fill: '#fff7ed', stroke: '#fb923c' }));
  }
  return { scene: scene(elements, width, height, { layout: 'state-machine' }), width, height };
}

function buildDefenseMap() {
  const width = 1320;
  const height = 820;
  const elements = [];
  titleBlock(elements, '答辩攻防地图', '把追问拆成角色视角：架构、测试、运维、前端、产品', width);
  elements.push(...ellipse('核心口径\n失败关闭交易链路', 484, 136, 352, 120, {
    fill: palette.amberFill,
    stroke: palette.amberStroke,
    fontSize: 24,
    wrap: 14,
    maxLines: 3,
    roughness: 2.2,
  }));
  const cards = [
    ['架构师', '为什么不是 PG 行锁？', 'Redis Lua 单写者 + PG 真相'],
    ['测试工程师', '弱网重试会不会乱？', '同 key 回放 + seq 恢复'],
    ['SRE', 'Kafka 慢怎么证明？', 'Redis durable + lag'],
    ['前端', '断线后如何恢复？', 'last_seq + 快照'],
    ['产品', 'AI 能否改结果？', 'AI 不碰钱/胜者/订单'],
    ['安全', '恶意输入怎么办？', 'ACL/admission/规则拒绝'],
  ];
  const points = [
    [84, 320], [514, 320], [944, 320],
    [84, 560], [514, 560], [944, 560],
  ];
  cards.forEach(([role, question, answer], index) => {
    const [fill, stroke] = accents[index % accents.length];
    const [x, y] = points[index];
    const anchorY = index < 3 ? 258 : 540;
    const anchorX = index < 3 ? x + 168 : x + 168;
    elements.push(...arrow(anchorX, anchorY, x + 168, y - 8, '', {
      stroke,
      strokeWidth: 1.7,
      strokeStyle: 'dashed',
    }));
    elements.push(...frame(role, x, y, 336, 150, { stroke, fill: '#ffffff', fillStyle: 'solid', roughness: 1.5 }));
    elements.push(textElement(wrapLabel(question, 20, 2), x + 26, y + 58, 284, 18, palette.ink, 'left'));
    elements.push(textElement(wrapLabel(answer, 20, 2), x + 26, y + 104, 284, 16, stroke, 'left'));
  });
  elements.push(...note('读图方式：先给核心口径，再按评委角色落到代码边界和证据。', 386, 726, 548, 54, {
    wrap: 28,
    maxLines: 2,
    fill: '#ecfeff',
    stroke: palette.cyanStroke,
  }));
  return { scene: scene(elements, width, height, { layout: 'defense-map' }), width, height };
}

function commonArchitecture(title = '全局架构：失败关闭交易链路') {
  return buildArchitecture(
    title,
    '只画关键边界：入口、网关、热决策、WAL/真相、实时与证据',
    [
      { label: '体验入口', edge: 'HTTP/WS', nodes: ['H5 竞拍', 'PC 控制台', 'AI 运营'] },
      { label: '网关边界', edge: '准入', nodes: ['Auth', 'ACL', 'Admission', 'Idempotency'] },
      { label: '热决策', edge: '决策日志', nodes: ['Redis Lua', 'engine_seq', 'Idem 结果', 'Redis Stream'] },
      { label: 'WAL / 真相', edge: '投递', nodes: ['Kafka ledger', 'PG settlement', 'Order', 'Outbox'] },
      { label: '恢复 / 证据', nodes: ['WebSocket seq', 'Snapshot', 'Reconciler', 'S1-S5'] },
    ],
    { footer: '核心口径：热决策、可重放、真相库、可靠投递、证据收敛。' },
  );
}

function bidSwimlane() {
  return buildSwimlane(
    '单次出价闭环：点击到最终决策',
    '按真实代码路径讲：H5 -> Gateway -> Redis Lua -> Kafka/PG -> Outbox/WS',
    ['客户端 / 网关', 'Redis 热决策', '异步持久 / 实时'],
    [
      { lane: 0, label: '生成\nbid id', edge: 'HTTP' },
      { lane: 0, label: 'Auth\nACL\nAdmission', edge: '调用' },
      { lane: 1, label: 'Lua 原子\n规则决策', edge: '写入' },
      { lane: 1, label: 'idem + stream\nengine_seq', edge: '等待' },
      { lane: 2, label: 'Kafka ACK\n40ms', edge: '响应', decision: true },
      { lane: 0, label: 'HTTP 决策\nACKED / DURABLE', edge: '后台' },
      { lane: 2, label: 'PG 结算\nOutbox/WS' },
    ],
    { callout: '拒绝也是决策：有序号、有日志、可证明。' },
  );
}

function redisLuaDataFlow() {
  return buildArchitecture(
    'Redis Lua 决策数据流',
    'Lua 内一次完成幂等、规则、序号和决策日志，避免多组件竞态',
    [
      { label: '输入', edge: 'hash', nodes: ['auction', 'user', 'amount', 'bid id'] },
      { label: '幂等', edge: '校验', nodes: ['idem key', 'request hash', 'result json'] },
      { label: '规则', edge: '决策', nodes: ['ACTIVE', 'min_next', 'cap', 'fat finger'] },
      { label: '状态', edge: '落日志', nodes: ['highest_bid', 'engine_seq', 'end_at', 'sold'] },
      { label: '输出', nodes: ['decision', 'reason', 'durability', 'snapshot'] },
    ],
    { footer: '取舍：用 Lua 复杂度换热路径原子性。' },
  );
}

function durabilityBranch() {
  return buildPipeline(
    '响应 durability 分支',
    'HTTP 响应区分 Kafka 同步确认和 Redis 已持久、后台收敛',
    [
      { label: 'Lua 成功', note: '状态、idem、stream 已写' },
      { label: '唤醒 relay', note: 'pending auction discovery' },
      { label: '等 Kafka ACK', decision: true, note: 'waitKafkaAck 40ms' },
      { label: 'KAFKA_ACKED', note: '同步确认 WAL' },
      { label: 'DURABLE', note: 'Kafka 慢或熔断，后台继续' },
      { label: 'PG + Outbox', note: '结算和广播收敛' },
    ],
    { bottom: 'DURABLE 不等于 Kafka 已成功；后续看 lag、settlement、outbox。' },
  );
}

function settlementPipeline() {
  return buildPipeline(
    'Kafka 结算闭环',
    'Kafka 负责有序重放，PG 唯一约束和 CAS 吸收重复消费',
    [
      { label: 'Redis Stream' },
      { label: 'Kafka ledger', note: 'auction 分区内有序' },
      { label: 'Settlement worker', note: 'at-least-once 消费' },
      { label: 'PG bids', note: '唯一键防重' },
      { label: 'Order / outbox', note: '终态才建单' },
      { label: 'WS 发布', note: '客户端最终一致' },
    ],
    { bottom: '不吹 Kafka EOS；靠幂等消费、PG 约束和 outbox。' },
  );
}

function recoveryFlow(title = 'Redis 状态缺失恢复闭环') {
  return buildSwimlane(
    title,
    '状态缺失时先失败关闭，再从 PG/ledger/checkpoint 重建，恢复后才接新出价',
    ['故障入口', '重建链路', '恢复证明'],
    [
      { lane: 0, label: 'Redis 缺失', edge: '返回' },
      { lane: 0, label: 'RECONCILING\n暂停出价', edge: '读取' },
      { lane: 1, label: 'PG/ledger\n读取真相', edge: '重建' },
      { lane: 1, label: 'checkpoint\n恢复状态', edge: '校验' },
      { lane: 2, label: 'seq 连续\n规则一致', edge: '恢复' },
      { lane: 2, label: '重新 ACTIVE\n继续服务' },
    ],
    { callout: '恢复期失败关闭：宁可拒绝，不给假成功。' },
  );
}

function aiGuardrail() {
  return buildArchitecture(
    'AI 运营闭环与护栏',
    'AI 只提升运营效率，不参与价格、胜者和订单终态',
    [
      { label: '运营输入', edge: '上下文', nodes: ['商品信息', '直播状态', '历史数据'] },
      { label: 'AI 能做', edge: '建议', nodes: ['选品建议', '解说文案', 'Q&A', '复盘'] },
      { label: '硬护栏', edge: '隔离', nodes: ['不改价格', '不定胜者', '不建订单', '可审计'] },
      { label: '人工确认', edge: '发布', nodes: ['主播确认', '运营审批', '审计日志'] },
      { label: '业务输出', nodes: ['更好讲解', '风险提示', '效率提升'] },
    ],
    { footer: 'AI 只做运营建议；交易结果仍由 Redis/PG 决定。' },
  );
}

function l4Loop(topic) {
  return buildSwimlane(
    topic.title,
    topic.subtitle,
    ['输入', '权威判断', '状态 / 证据'],
    topic.steps,
    { callout: topic.callout },
  );
}

const l4Topics = [
  {
    match: 'idempotency-key',
    title: '出价幂等键最小闭环',
    subtitle: '同一个 bid id 只能代表同一次请求；重试回放，改参拒绝',
    steps: [
      { lane: 0, label: 'H5 生成\nbid id', edge: '等值' },
      { lane: 0, label: 'Header\nidem key', edge: 'hash' },
      { lane: 1, label: 'Lua request\nhash', edge: '判断', decision: true },
      { lane: 1, label: '同参回放\nresult json', edge: '或' },
      { lane: 1, label: '改参拒绝\nconflict', edge: '兜底' },
      { lane: 2, label: 'PG 唯一键\n吸收重放' },
    ],
    callout: '三层幂等：HTTP、Redis、PG。',
  },
  {
    match: 'lua-price-rule',
    title: 'Lua 价格规则最小闭环',
    subtitle: '最小价、加价网格、封顶成交都在 Redis 原子判断内完成',
    steps: [
      { lane: 0, label: 'amount 输入', edge: '读取' },
      { lane: 1, label: '状态 ACTIVE?', decision: true, edge: '计算' },
      { lane: 1, label: 'min_next\nincrement', edge: '判断' },
      { lane: 1, label: 'cap / hard cap', decision: true, edge: '写入' },
      { lane: 2, label: 'ACCEPT / SOLD\n或拒绝原因', edge: '审计' },
      { lane: 2, label: 'decision log\nengine_seq' },
    ],
    callout: '价格判断必须在热路径原子完成。',
  },
  {
    match: 'fat-finger',
    title: '高额误触确认最小闭环',
    subtitle: '高风险出价先要求确认，不写决策流、不推进 engine_seq',
    steps: [
      { lane: 0, label: '用户出价\n明显偏高', edge: '检测' },
      { lane: 1, label: 'fat finger\n阈值判断', decision: true, edge: '要求' },
      { lane: 0, label: '返回确认\nrequired', edge: '二次提交' },
      { lane: 1, label: '带确认标记\n再判断', edge: '写入' },
      { lane: 2, label: '接受 / 拒绝\n可审计', edge: '同步' },
      { lane: 2, label: 'UI 明确\n非假失败' },
    ],
    callout: '确认前不消耗 engine_seq。',
  },
  {
    match: 'cap-sold-order',
    title: '一口价成交与订单闭环',
    subtitle: '等于 cap 立即 SOLD，订单由 PG 结算阶段幂等创建',
    steps: [
      { lane: 0, label: 'amount == cap', edge: '决策' },
      { lane: 1, label: 'Lua 标记\nSOLD', edge: '日志' },
      { lane: 1, label: 'Kafka ledger', edge: '消费' },
      { lane: 2, label: 'PG settlement\nCAS', edge: '建单' },
      { lane: 2, label: 'order unique', edge: '广播' },
      { lane: 2, label: 'Outbox/WS\n终态同步' },
    ],
    callout: 'Redis 做决策，PG 建订单。',
  },
  {
    match: 'kafka-ack-durability',
    title: 'Kafka ACK 持久性最小闭环',
    subtitle: '同步 ACK 和后台收敛分开表达，避免把慢 Kafka 伪装成失败或成功',
    steps: [
      { lane: 0, label: 'Lua 已写\nRedis durable', edge: '等待' },
      { lane: 1, label: 'waitKafkaAck\n40ms', decision: true, edge: '分支' },
      { lane: 2, label: 'KAFKA_ACKED', edge: '或' },
      { lane: 2, label: 'DURABLE', edge: '后台' },
      { lane: 1, label: 'relay 重试\nappend Kafka', edge: '消费' },
      { lane: 2, label: 'PG/outbox\n最终收敛' },
    ],
    callout: '异步收敛看 lag、settlement、outbox。',
  },
  {
    match: 'redis-state-missing',
    title: 'Redis 状态缺失恢复最小闭环',
    subtitle: '缺状态先进入 RECONCILING，再从持久真相重建',
    steps: [
      { lane: 0, label: 'state key\nmissing', edge: '失败关闭' },
      { lane: 0, label: '返回\nRECONCILING', edge: '读取' },
      { lane: 1, label: 'PG / ledger\n真相源', edge: '重建' },
      { lane: 1, label: '写回 Redis\ncheckpoint', edge: '校验' },
      { lane: 2, label: 'seq/价格\n一致', edge: '恢复' },
      { lane: 2, label: '继续接单' },
    ],
    callout: '不猜状态，只从真相源恢复。',
  },
  {
    match: 'checkpoint-rebuild',
    title: 'Checkpoint 重建闭环',
    subtitle: '用 checkpoint 降低重放成本，再用 ledger/PG 校验正确性',
    steps: [
      { lane: 0, label: '发现漂移', edge: '定位' },
      { lane: 1, label: '读取\ncheckpoint', edge: '补齐' },
      { lane: 1, label: '重放增量\nledger', edge: '写回' },
      { lane: 2, label: 'Redis 状态\n更新', edge: '比对' },
      { lane: 2, label: 'PG/seq\n校验', edge: '开放' },
      { lane: 2, label: '恢复 ACTIVE' },
    ],
    callout: 'checkpoint 加速，PG/ledger 校验。',
  },
  {
    match: 'kafka-redelivery',
    title: 'Kafka 重投幂等闭环',
    subtitle: 'at-least-once 消费下，重复消息由 PG 幂等吸收',
    steps: [
      { lane: 0, label: 'Kafka 重投', edge: '消费' },
      { lane: 1, label: 'worker 处理', edge: '写入' },
      { lane: 1, label: 'PG 唯一键\n/ CAS', decision: true, edge: '吸收' },
      { lane: 2, label: '已处理\n直接跳过', edge: '或' },
      { lane: 2, label: '首次处理\n写 outbox', edge: '确认' },
      { lane: 2, label: 'commit offset' },
    ],
    callout: '重投是正常路径，幂等吸收。',
  },
  {
    match: 'order-creation-exactly-once',
    title: '订单创建 Exactly-once 语义闭环',
    subtitle: '不是 Kafka EOS，而是业务 exactly-once：唯一键 + 状态 CAS',
    steps: [
      { lane: 0, label: 'SOLD 决策', edge: '消费' },
      { lane: 1, label: '结算事务', edge: 'CAS' },
      { lane: 1, label: 'auction\n未结算?', decision: true, edge: '建单' },
      { lane: 2, label: 'orders unique', edge: 'outbox' },
      { lane: 2, label: '重复消息\n无副作用', edge: '证据' },
      { lane: 2, label: '订单终态\n可审计' },
    ],
    callout: '业务 exactly-once，不吹 Kafka EOS。',
  },
  {
    match: 'outbox-publish-retry',
    title: 'Outbox 发布重试闭环',
    subtitle: '业务事务先写 outbox，发布失败可重试且不丢事件',
    steps: [
      { lane: 0, label: 'PG 事务\n写 outbox', edge: '扫描' },
      { lane: 1, label: 'relay 取待发', edge: '发布' },
      { lane: 1, label: 'WS / broker\n成功?', decision: true, edge: '更新' },
      { lane: 2, label: '标记 sent', edge: '或' },
      { lane: 2, label: '保留 pending\n下次重试', edge: '监控' },
      { lane: 2, label: 'lag 告警' },
    ],
    callout: 'Outbox 补齐提交与发布缺口。',
  },
  {
    match: 'ticket-scope-consume',
    title: 'WebSocket ticket 作用域闭环',
    subtitle: 'ticket 绑定用户/房间/拍品，消费后失效，降低泄漏风险',
    steps: [
      { lane: 0, label: 'H5 获取\nticket', edge: '连接' },
      { lane: 0, label: 'WS 参数\nroom/auction', edge: '校验' },
      { lane: 1, label: 'ticket scope\n匹配?', decision: true, edge: '消费' },
      { lane: 1, label: '一次性\nconsume', edge: '订阅' },
      { lane: 2, label: '绑定 user\nauction', edge: '断开' },
      { lane: 2, label: '重连换票' },
    ],
    callout: 'ticket 短时、带作用域、一次性。',
  },
  {
    match: 'last-seq-recovery',
    title: 'last_seq 恢复闭环',
    subtitle: '断线重连从 last_seq 后补历史，补不了就拉快照',
    steps: [
      { lane: 0, label: '保存\nlast_seq', edge: '重连' },
      { lane: 1, label: '服务端查\nhistory', edge: '判断' },
      { lane: 1, label: '连续可补?', decision: true, edge: '推送' },
      { lane: 2, label: '补事件', edge: '或' },
      { lane: 2, label: '要求快照', edge: '更新' },
      { lane: 0, label: 'UI 恢复\n最新状态' },
    ],
    callout: '关键不是不断线，而是可恢复。',
  },
  {
    match: 'slow-consumer',
    title: '慢消费者断开闭环',
    subtitle: '有界队列保护服务端，慢客户端断开后通过 last_seq 恢复',
    steps: [
      { lane: 0, label: '客户端处理慢', edge: '堆积' },
      { lane: 1, label: 'send queue\n满?', decision: true, edge: '断开' },
      { lane: 1, label: '关闭连接\n释放资源', edge: '重连' },
      { lane: 2, label: '客户端换票', edge: 'last_seq' },
      { lane: 2, label: '补历史\n或快照', edge: '恢复' },
      { lane: 0, label: '继续观看' },
    ],
    callout: '背压保护服务，重连后恢复。',
  },
  {
    match: 'bid-timeout-uncertain',
    title: 'H5 超时不确定重试闭环',
    subtitle: '前端超时不等于服务端失败，用同 key 重试或查快照消除不确定',
    steps: [
      { lane: 0, label: '点击出价', edge: '请求' },
      { lane: 0, label: '请求超时\n8s', decision: true, edge: '超时' },
      { lane: 1, label: '同 key 重试', edge: '回放' },
      { lane: 1, label: '服务端 idem\n结果回放', edge: '同步' },
      { lane: 2, label: '拉快照\n校准 UI', edge: '提示' },
      { lane: 0, label: '显示最终\n决策' },
    ],
    callout: '弱网重试：同 key。',
  },
  {
    match: 'countdown-server-time',
    title: '服务端时间锚点倒计时闭环',
    subtitle: '倒计时基于服务端时间偏移，减少客户端本地时钟漂移',
    steps: [
      { lane: 0, label: '获取快照', edge: '包含' },
      { lane: 1, label: 'server_now\nend_at', edge: '计算' },
      { lane: 0, label: '本地 offset', edge: '渲染' },
      { lane: 0, label: '倒计时 UI', edge: '事件' },
      { lane: 2, label: 'WS end_at\n更新', edge: '校准' },
      { lane: 0, label: '延时后\n继续倒计时' },
    ],
    callout: '终态由服务端事件决定。',
  },
  {
    match: 'seq-gap-snapshot',
    title: 'seq gap 快照恢复闭环',
    subtitle: '前端发现事件序号断档，立即拉快照重建 UI 状态',
    steps: [
      { lane: 0, label: '收到 WS 事件', edge: '比较' },
      { lane: 0, label: 'seq 不连续?', decision: true, edge: '暂停' },
      { lane: 1, label: '请求快照', edge: '返回' },
      { lane: 1, label: '覆盖本地状态', edge: '设置' },
      { lane: 2, label: 'last_seq\n重置', edge: '继续' },
      { lane: 0, label: 'UI 恢复' },
    ],
    callout: '发现 gap 就拉快照。',
  },
];

function findL4Topic(slug) {
  return l4Topics.find((topic) => slug.includes(topic.match));
}

function diagramFor(slug, heading, mdRel, localIndex) {
  if (slug === 'readme-01') return buildDocMap();
  if (slug.includes('00-project-00-overview') || slug.includes('01-architecture-00-system-architecture')) return commonArchitecture(slug.includes('overview') ? '全局架构：失败关闭交易链路' : '系统架构：热决策与持久真相分层');
  if (slug.includes('01-architecture-01-data-consistency')) {
    return buildArchitecture(
      '数据一致性边界',
      '把“快、准、可重放、可审计”分别放在合适组件里',
      [
        { label: 'Redis', edge: 'append', nodes: ['热状态', 'Lua 原子', 'idem', 'stream'] },
        { label: 'Kafka', edge: 'consume', nodes: ['WAL', '重放', '分区有序'] },
        { label: 'PostgreSQL', edge: 'outbox', nodes: ['结算真相', '唯一约束', '订单'] },
      { label: 'WebSocket', edge: 'recover', nodes: ['实时投递', 'last_seq', '快照'] },
        { label: '校验器', nodes: ['S1-S5', '漂移检测', '证据'] },
      ],
      { footer: '一致性靠边界责任：幂等、序号、恢复、证据。' },
    );
  }
  if (slug.includes('00-project-01-product-scope')) {
    return buildSwimlane(
      '产品业务闭环',
      '真实直播竞拍从主播排品到买家出价、成交、复盘',
      ['主播 / 运营', '买家 H5', '交易系统'],
      [
        { lane: 0, label: '创建商品\n配置规则', edge: '排期' },
        { lane: 0, label: '启动拍品', edge: '进入' },
        { lane: 1, label: '进房看价', edge: '点击' },
        { lane: 1, label: '出价/确认', edge: '决策' },
        { lane: 2, label: '封顶成交\n或继续竞价', decision: true, edge: '同步' },
        { lane: 2, label: '订单/广播\n复盘证据' },
      ],
      { callout: '当前闭环到竞拍成交；支付履约是扩展方向。' },
    );
  }
  if (slug.includes('template-coverage')) {
    return buildMatrix(
      localIndex === 1 ? '模板要求覆盖图' : '答辩材料覆盖图',
      '把模板中的建设性要求映射到真实文档和代码证据',
      ['要求', '文档落点', '证据方式', '答辩用途'],
      [
        ['分层文档', 'README -> L1 -> L4', '链接门禁', '按层讲解'],
        ['最小闭环', 'backend/realtime/frontend 子目录', '代码+测试锚点', '回答细节追问'],
        ['工业对标', '技术选型文档', '官方参考', '解释取舍'],
        ['困难解决', '工程难点文档', '故障/测试记录', '体现工程能力'],
        ['图表可视化', 'Excalidraw + 表格', '61 张可编辑图', '辅助演示'],
      ],
      { footer: '模板是检查清单；最终以真实代码为准。' },
    );
  }
  if (slug.includes('visualization-map')) {
    return buildMatrix(
      localIndex === 1 ? '可视化资产地图' : '图表使用规则',
      '图用于讲结构，表用于讲对比，代码锚点用于证明真实实现',
      ['类型', '使用位置', '解决问题', '质量标准'],
      [
        ['架构图', '00/01 架构', '边界和主链路', '少箭头、强分区'],
        ['泳道图', 'L4 闭环', '输入到结果', '每步短标签'],
        ['矩阵表', '证据/风险/对标', '横向比较', '结论可扫描'],
        ['状态机', '领域模型', '终态边界', '状态不混淆'],
        ['答辩图', '09 defense', '角色追问', '问题到证据'],
      ],
      { footer: 'Markdown 显示 SVG；源文件保留 .excalidraw。' },
    );
  }
  if (slug.includes('technology-selection')) {
    return buildMatrix(
      localIndex === 1 ? '技术选型对标' : '关键取舍矩阵',
      '选型围绕热点争用、可恢复、可证明，而不是堆组件',
      ['问题', '常见做法', '本项目选择', '理由'],
      [
        ['热点出价', 'DB 行锁 / 队列', 'Redis Lua', '单写者原子决策'],
        ['事件日志', 'MQ / binlog', 'Kafka WAL', '有序重放和削峰'],
        ['订单真相', '缓存即真相', 'PostgreSQL', '事务与审计'],
        ['实时同步', '轮询 / WS', 'WS + seq', '弱网可恢复'],
        ['异步投递', '直接发消息', 'Outbox', '避免提交后丢消息'],
      ],
      { footer: '不是组件崇拜，而是职责分层。' },
    );
  }
  if (slug.includes('domain-model')) {
    if (localIndex === 1) {
      return buildStateMachine(
        '拍品状态机',
        '所有交易判断都围绕拍品状态、时间和终态边界展开',
        ['DRAFT', 'SCHEDULED', 'ACTIVE', 'SOLD', 'ENDED', 'CANCELED'],
        [[0, 1, '排期'], [1, 2, '启动'], [2, 3, 'cap'], [2, 4, '超时'], [0, 5, '取消'], [1, 5, '取消']],
        { center: 'ACTIVE 才能接受出价；终态只能由服务端决策推进。' },
      );
    }
    return buildMatrix(
      '领域规则矩阵',
      '领域模型不是表结构列表，而是交易规则和异常边界',
      ['规则', '判断点', '输出', '证据'],
      [
        ['加价网格', 'amount vs min_next', '接受/拒绝', 'Lua tests'],
        ['封顶成交', 'amount == cap', 'SOLD', 'settlement tests'],
        ['误触确认', '高额阈值', 'confirm required', 'H5/Lua'],
        ['反狙击延时', '接近结束', 'end_at 延长', 'engine tests'],
        ['权限 ACL', '成员关系', 'forbidden', 'gateway tests'],
      ],
    );
  }
  if (slug.includes('03-backend-01-bid-decision-closed-loop-01')) return bidSwimlane();
  if (slug.includes('03-backend-01-bid-decision-closed-loop-02')) return redisLuaDataFlow();
  if (slug.includes('03-backend-01-bid-decision-closed-loop-03')) return durabilityBranch();
  if (slug.includes('03-backend-02-kafka-settlement')) return settlementPipeline();
  if (slug.includes('03-backend-03-redis-loss') || slug.includes('03-backend-recovery-')) return recoveryFlow(slug.includes('checkpoint') ? 'Checkpoint 重建闭环' : 'Redis 恢复闭环');
  if (slug.includes('03-backend-04-ai-ops')) return aiGuardrail();
  if (slug.includes('03-backend-05-engineering')) {
    return buildLoopCards(
      `工程难点 ${localIndex}`,
      '每个困难都按“风险 -> 解法 -> 证据”讲，避免只说概念',
      [
        { title: '热点争用', main: 'Redis Lua', detail: '避免 PG 行锁排队；代价是 Lua 复杂度和测试压力。' },
        { title: '重复请求', main: '三层幂等', detail: 'HTTP key、Redis hash、PG unique/CAS。' },
        { title: '异步不确定', main: 'durability 分级', detail: 'ACKED 与 DURABLE 清晰区分。' },
        { title: '弱网恢复', main: 'seq + 快照', detail: 'WS gap 不静默吞掉，前端拉快照修正。' },
        { title: 'AI 风险', main: '硬隔离', detail: 'AI 只做运营内容，不触碰交易终态。' },
        { title: '证据闭环', main: 'S1-S5', detail: '用脚本和监控证明最终一致，而不是口头保证。' },
      ],
      { footer: '回答边界问题：先异常输入，再防线，再证据。' },
    );
  }
  if (slug.includes('03-backend-auction-bid-00-index')) {
    return buildIndex('出价热路径 L4 索引', '围绕一个买家点击出价后的所有关键追问', ['幂等键', '价格规则', '误触确认', '封顶成交', 'Kafka ACK'], { parent: '出价闭环' });
  }
  if (slug.includes('03-backend-settlement-00-index')) {
    return buildIndex('结算 L4 索引', '围绕 Kafka 重放、PG 事务、订单和 outbox 的追问', ['Kafka 重投', '订单 exactly-once', 'Outbox 重试'], { parent: '结算闭环' });
  }
  if (slug.includes('04-realtime-websocket-00-index')) {
    return buildIndex('WebSocket L4 索引', '围绕连接凭证、断线恢复和慢消费者背压', ['ticket 作用域', 'last_seq 恢复', '慢消费者断开'], { parent: '实时恢复' });
  }
  if (slug.includes('05-frontend-mobile-h5-00-index')) {
    return buildIndex('H5 L4 索引', '围绕弱网、倒计时和事件 gap 的前端追问', ['超时不确定重试', '服务端时间锚点', 'seq gap 快照恢复'], { parent: 'H5 闭环' });
  }
  const topic = findL4Topic(slug);
  if (topic) return l4Loop(topic);
  if (slug.includes('04-realtime-01-websocket-recovery')) {
    return buildSwimlane(
      'WebSocket 恢复闭环',
      '实时投递不保证永不断线，但必须可发现 gap、可补历史、可拉快照',
      ['连接', '事件流', '恢复'],
      [
        { lane: 0, label: '获取 ticket', edge: '连接' },
        { lane: 0, label: '订阅 auction', edge: '推送' },
        { lane: 1, label: '按 seq\n接收事件', edge: '发现' },
        { lane: 1, label: 'seq gap?', decision: true, edge: '恢复' },
        { lane: 2, label: 'history\n或快照', edge: '更新' },
        { lane: 2, label: 'UI 对齐\nlast_seq' },
      ],
      { callout: '网络抖动靠 last_seq + 快照。' },
    );
  }
  if (slug.includes('05-frontend-01-mobile-h5')) {
    return buildSwimlane(
      'H5 竞拍体验闭环',
      '前端把弱网、幂等、倒计时和实时事件统一成可解释状态',
      ['用户操作', '网络 / 状态', 'UI 呈现'],
      [
        { lane: 0, label: '进房拿\n快照', edge: '连接' },
        { lane: 1, label: 'WS ticket', edge: '同步' },
        { lane: 2, label: '展示最高价\n倒计时', edge: '点击' },
        { lane: 0, label: '出价\nbid id', edge: '请求' },
        { lane: 1, label: '超时/重试\n同 key', edge: '结果' },
        { lane: 2, label: '决策 toast\n状态更新' },
      ],
      { callout: '前端展示权威状态，不判断胜者。' },
    );
  }
  if (slug.includes('05-frontend-02-pc-console')) {
    return buildSwimlane(
      localIndex === 1 ? 'PC 主播控制台闭环' : 'PC 运营监控闭环',
      'PC 端负责创建、排期、启动、观察和运营辅助',
      ['配置', '控制', '观察'],
      [
        { lane: 0, label: '创建商品\n规则配置', edge: '冻结' },
        { lane: 0, label: '排期/启动', edge: '控制' },
        { lane: 1, label: 'ACTIVE\n房间状态', edge: '订阅' },
        { lane: 2, label: '实时出价\n事件列表', edge: 'AI' },
        { lane: 2, label: '解说/哨兵\n建议', edge: '复盘' },
        { lane: 2, label: '证据和\n告警入口' },
      ],
      { callout: 'PC 是控制面，不绕过交易引擎。' },
    );
  }
  if (slug.includes('06-observability')) {
    return buildPipeline(
      localIndex === 1 ? '可观测数据流' : '告警到处置闭环',
      '指标、日志、链路和告警要能回到具体交易边界',
      [
        { label: 'Go metrics' },
        { label: 'Prometheus' },
        { label: 'Grafana' },
        { label: 'Alertmanager' },
        { label: 'Runbook' },
        { label: '复盘证据' },
      ],
      { bottom: '关键指标围绕延迟、Kafka lag、outbox lag、WS 连接、恢复中状态和错误原因。' },
    );
  }
  if (slug.includes('07-performance-and-evidence')) {
    return buildMatrix(
      localIndex === 1 ? '证据等级地图' : 'S1-S5 门禁链路',
      '不要把演示截图当性能证明；证据需要脚本、环境和边界',
      ['证据', '证明什么', '不能证明什么', '答辩说法'],
      [
        ['单测/集成', '规则正确性', '线上容量', '强正确性证据'],
        ['PTS 脚本', '高并发一致性', '任意机器可复现数值', '带环境解释'],
        ['Playwright', '端到端流程', '后端极限吞吐', '体验证据'],
        ['Grafana', '运行状态', '根因自动成立', '辅助定位'],
        ['Review 文档', '第三方审查', '当前代码全部一致', '需按 HEAD 修正'],
      ],
      { footer: '性能数字必须绑定环境、命令、时间、证据。' },
    );
  }
  if (slug.includes('08-tests-and-risk')) {
    return buildMatrix(
      localIndex === 1 ? '风险与滥用矩阵' : '测试攻击面',
      '按真实业务事故设计测试：重复点击、弱网、重放、慢消费者、组件故障',
      ['风险', '攻击/事故', '系统防线', '验证方式'],
      [
        ['重复出价', '双击/重试', '幂等三层', 'unit + PTS'],
        ['恶意金额', '低价/越 cap', 'Lua 规则', 'engine tests'],
        ['断线漏事件', 'WS gap', 'last_seq/快照', 'load/e2e'],
        ['Kafka 慢', 'ACK 超时', 'durability 分级', 'lag 检查'],
        ['慢消费者', '队列堆积', '断开+恢复', 'load test'],
      ],
      { footer: '回答测试追问：攻击输入 -> 防线 -> 证据。' },
    );
  }
  if (slug.includes('09-judge-defense')) return buildDefenseMap();

  if (mdRel.includes('00-index')) {
    return buildIndex(heading || 'L4 文档索引', '父子层级和跳转关系', ['最小闭环 1', '最小闭环 2', '最小闭环 3'], { parent: '父文档' });
  }

  return buildPipeline(
    heading || '最小闭环',
    '通用闭环：输入、判断、状态、异常和证据',
    [
      { label: '输入' },
      { label: '校验' },
      { label: '权威决策' },
      { label: '状态写入' },
      { label: '异常兜底' },
      { label: '证据验证' },
    ],
    { bottom: '如果评委追问，按代码入口、状态变化和测试证据三步展开。' },
  );
}

function exportSvgWithCli(excalidrawPath, svgPath) {
  const args = [
    'convert',
    excalidrawPath,
    '--format',
    'svg',
    '-o',
    svgPath,
    '--padding',
    '20',
    '--embed-scene',
  ];
  const result = process.platform === 'win32'
    ? spawnSync('cmd.exe', ['/d', '/s', '/c', 'excalidraw-cli', ...args], {
      cwd: path.resolve(docsRoot, '..'),
      encoding: 'utf8',
      shell: false,
    })
    : spawnSync('excalidraw-cli', args, {
    cwd: path.resolve(docsRoot, '..'),
    encoding: 'utf8',
    shell: false,
  });
  if (result.status !== 0) {
    throw new Error(`excalidraw-cli convert failed for ${excalidrawPath}\n${result.stdout}\n${result.stderr}`);
  }
}

fs.mkdirSync(assetsDir, { recursive: true });
let count = 0;
for (const mdFile of walkMarkdown(docsRoot)) {
  let markdown = fs.readFileSync(mdFile, 'utf8');
  let localIndex = 0;
  const mdRel = path.relative(docsRoot, mdFile).replace(/\\/g, '/');
  markdown = markdown.replace(new RegExp(`${generatedStart}[\\s\\S]*?${generatedEnd}`, 'g'), (block, offset) => {
    localIndex += 1;
    const before = markdown.slice(0, offset);
    const headings = [...before.matchAll(/^##\s+(.+)$/gm)];
    const heading = headings.length ? headings.at(-1)[1].trim() : 'Excalidraw 图';
    const excRelOld = block.match(/href="([^"]+\.excalidraw)"/)?.[1];
    const svgRelOld = block.match(/src="([^"]+\.svg)"/)?.[1];
    if (!excRelOld || !svgRelOld) return block;

    const excPath = path.resolve(path.dirname(mdFile), excRelOld);
    const svgPath = path.resolve(path.dirname(mdFile), svgRelOld);
    const slug = path.basename(excPath, '.excalidraw');
    resetIds();
    const data = diagramFor(slug, heading, mdRel, localIndex);

    fs.mkdirSync(path.dirname(excPath), { recursive: true });
    fs.writeFileSync(excPath, `${JSON.stringify(data.scene, null, 2)}\n`, 'utf8');
    if (shouldExportSvg) {
      exportSvgWithCli(excPath, svgPath);
    }
    count += 1;

    const excRel = markdownPath(mdFile, excPath);
    const svgRel = markdownPath(mdFile, svgPath);
    return `${generatedStart}\n<div class="excalidraw-diagram" style="overflow-x: auto; width: 100%; margin: 12px 0 18px 0; padding: 12px 12px 14px 12px; border: 1px solid #e2e8f0; border-radius: 8px; background: #ffffff;">\n  <a href="${htmlEscape(excRel)}">打开可编辑 Excalidraw 源文件</a>\n  <br />\n  <img src="${htmlEscape(svgRel)}" alt="${htmlEscape(heading)}" loading="lazy" width="${data.width}" style="display: block; width: ${data.width}px; max-width: none !important; height: auto;" />\n</div>\n${generatedEnd}`;
  });
  fs.writeFileSync(mdFile, markdown, 'utf8');
}

console.log(`Rebuilt ${count} Excalidraw diagrams from markdown references${shouldExportSvg ? ' and exported SVG previews with excalidraw-cli' : ''}.`);
