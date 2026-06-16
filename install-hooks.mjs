#!/usr/bin/env node
/**
 * install-hooks.mjs — Install career-ops git hooks from `hooks/` to `.git/hooks/`.
 *
 * Copies all files from `hooks/` (without the `.sample` suffix) to `.git/hooks/`,
 * sets them executable, and skips files that already exist (so user customizations
 * are not clobbered — pass --force to override).
 *
 * Usage:
 *   node install-hooks.mjs           # safe install, skip existing
 *   node install-hooks.mjs --force   # overwrite existing
 */
import { readFileSync, writeFileSync, existsSync, readdirSync, statSync, chmodSync } from 'fs';
import { join, dirname, basename } from 'path';
import { fileURLToPath } from 'url';

const ROOT = dirname(fileURLToPath(import.meta.url));
const HOOKS_SRC = join(ROOT, 'hooks');
const HOOKS_DST = join(ROOT, '.git', 'hooks');
const FORCE = process.argv.includes('--force');

if (!existsSync(HOOKS_SRC)) {
  console.error(`❌ Hooks source dir not found: ${HOOKS_SRC}`);
  process.exit(1);
}
if (!existsSync(HOOKS_DST)) {
  console.error(`❌ Git hooks dir not found: ${HOOKS_DST} (not a git repo?)`);
  process.exit(1);
}

const files = readdirSync(HOOKS_SRC).filter(f => !f.endsWith('.sample') && !f.startsWith('.'));
let installed = 0, skipped = 0;
for (const f of files) {
  const src = join(HOOKS_SRC, f);
  const dst = join(HOOKS_DST, f);
  if (statSync(src).isDirectory()) continue;
  if (existsSync(dst) && !FORCE) {
    console.log(`⏭️  ${f} already exists — skipped (use --force to overwrite)`);
    skipped++;
    continue;
  }
  const content = readFileSync(src, 'utf-8');
  writeFileSync(dst, content, { mode: 0o755 });
  try { chmodSync(dst, 0o755); } catch {}
  console.log(`✅ Installed ${f}`);
  installed++;
}

console.log(`\n${installed} installed, ${skipped} skipped.`);
console.log('Re-run with --force to overwrite existing hooks.');
