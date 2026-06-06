import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';

const sampleRate = 44100;
const outDir = 'frontend/mobile-h5/public/audio/auction';

function envelope(t, duration, attack = 0.01, release = 0.08) {
  if (t < attack) return t / attack;
  if (t > duration - release) return Math.max(0, (duration - t) / release);
  return 1;
}

function sine(freq, t) {
  return Math.sin(2 * Math.PI * freq * t);
}

function noise(seed) {
  const x = Math.sin(seed * 12.9898) * 43758.5453;
  return (x - Math.floor(x)) * 2 - 1;
}

function writeWav(samples) {
  const dataSize = samples.length * 2;
  const buffer = Buffer.alloc(44 + dataSize);
  buffer.write('RIFF', 0);
  buffer.writeUInt32LE(36 + dataSize, 4);
  buffer.write('WAVE', 8);
  buffer.write('fmt ', 12);
  buffer.writeUInt32LE(16, 16);
  buffer.writeUInt16LE(1, 20);
  buffer.writeUInt16LE(1, 22);
  buffer.writeUInt32LE(sampleRate, 24);
  buffer.writeUInt32LE(sampleRate * 2, 28);
  buffer.writeUInt16LE(2, 32);
  buffer.writeUInt16LE(16, 34);
  buffer.write('data', 36);
  buffer.writeUInt32LE(dataSize, 40);
  samples.forEach((sample, index) => {
    const value = Math.max(-1, Math.min(1, sample));
    buffer.writeInt16LE(Math.round(value * 32767), 44 + index * 2);
  });
  return buffer;
}

function render(duration, fn) {
  const total = Math.floor(duration * sampleRate);
  const samples = new Float32Array(total);
  for (let i = 0; i < total; i += 1) {
    const t = i / sampleRate;
    samples[i] = fn(t, i) * envelope(t, duration);
  }
  return writeWav(samples);
}

const assets = {
  'heartbeat-bed.wav': render(2.4, (t) => {
    const beat = (t % 0.72);
    const thump = Math.exp(-beat * 18) * sine(72, t);
    const second = Math.exp(-Math.max(0, beat - 0.18) * 24) * sine(96, t) * 0.55;
    return (thump + second) * 0.22;
  }),
  'whoosh-rank.wav': render(0.52, (t, i) => {
    const sweep = sine(260 + t * 1040, t);
    return (noise(i + 17) * 0.28 + sweep * 0.22) * Math.pow(1 - t / 0.52, 0.7);
  }),
  'coin-leading.wav': render(0.62, (t) => (
    sine(1120, t) * Math.exp(-t * 5) +
    sine(1680, t) * Math.exp(-t * 7) * 0.55 +
    sine(2240, t) * Math.exp(-t * 9) * 0.34
  ) * 0.28),
  'hammer-hit.wav': render(0.72, (t, i) => {
    const hit = Math.exp(-t * 12) * (sine(118, t) + sine(236, t) * 0.55);
    const wood = noise(i + 91) * Math.exp(-t * 24) * 0.26;
    return (hit + wood) * 0.44;
  }),
  'system-chime.wav': render(0.7, (t) => (
    sine(660, t) * Math.exp(-t * 4) +
    sine(880, Math.max(0, t - 0.08)) * Math.exp(-Math.max(0, t - 0.08) * 5) * 0.64
  ) * 0.24)
};

await mkdir(outDir, { recursive: true });
await Promise.all(Object.entries(assets).map(([name, data]) => writeFile(join(outDir, name), data)));
console.log(`generated ${Object.keys(assets).length} H5 sound assets in ${outDir}`);
