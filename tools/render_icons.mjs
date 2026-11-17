// Renders the SVG icons to PNGs at every .ico size (needs sharp).
import fs from 'node:fs';

// `npm i sharp` (falls back to a preinstalled copy)
const sharp = (await import('sharp').catch(() => import('/opt/npm-tools/node_modules/sharp/dist/index.mjs'))).default;
const out = process.argv[2];
fs.mkdirSync(out, { recursive: true });
for (const variant of ['', '-paused']) {
  for (const size of [16, 20, 24, 32, 40, 48, 64, 128, 256]) {
    const src = size <= 32 ? `res/icon-small${variant}.svg` : `res/icon${variant}.svg`;
    const svg = fs.readFileSync(src);
    await sharp(svg, { density: Math.ceil(72 * size / 32 * 4) }).resize(size, size).png().toFile(`${out}/icon${variant}-${size}.png`);
  }
}
console.log('ok');
