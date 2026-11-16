import { execFileSync } from 'node:child_process';
import { copyFileSync, cpSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const web = resolve(root, 'backend/web');
const vendor = resolve(web, 'vendor');
mkdirSync(vendor, { recursive: true });

const npmExec = process.platform === 'win32' ? 'npx.cmd' : 'npx';
execFileSync(npmExec, [
  '@tailwindcss/cli',
  '-i', 'backend/web/tailwind.input.css',
  '-o', 'backend/web/tailwind.css',
  '--minify',
], { cwd: root, stdio: 'inherit' });

for (const [from, to] of [
  ['node_modules/@xterm/xterm/lib/xterm.js', 'backend/web/vendor/xterm.js'],
  ['node_modules/@xterm/xterm/css/xterm.css', 'backend/web/vendor/xterm.css'],
  ['node_modules/@xterm/addon-fit/lib/addon-fit.js', 'backend/web/vendor/addon-fit.js'],
]) {
  copyFileSync(resolve(root, from), resolve(root, to));
}
cpSync(resolve(root, 'node_modules/mathjax-full/es5'), resolve(root, 'backend/web/vendor/mathjax'), { recursive: true });
console.log('Built local Tailwind CSS, xterm.js, and MathJax assets.');
