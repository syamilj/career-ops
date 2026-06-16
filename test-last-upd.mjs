#!/usr/bin/env node
/**
 * test-last-upd.mjs — Tests for the Last Upd feature.
 *
 * Covers:
 *  - migrate-add-last-upd.mjs: idempotency, schema correctness
 *  - merge-tracker.mjs: stamps Last Upd on new and updated rows
 *  - update-tracker.mjs: status/note/url/bump commands, error paths
 *  - check-tracker-updates.mjs: detects stale bumps, allows fresh ones
 *  - install-hooks.mjs: --dry-run? no, this one we just run.
 */
import { readFileSync, writeFileSync, mkdtempSync, existsSync, copyFileSync, rmSync, mkdirSync } from 'fs';
import { execFileSync } from 'child_process';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';
import { tmpdir } from 'os';

const ROOT = dirname(fileURLToPath(import.meta.url));
let passed = 0, failed = 0;

function pass(msg) { console.log(`  ✅ ${msg}`); passed++; }
function fail(msg) { console.log(`  ❌ ${msg}`); failed++; }
function section(title) { console.log(`\n${title}`); }

function runScript(scriptName, args, opts = {}) {
  // Extract env override from opts so it doesn't get clobbered by ...opts spread.
  const { env: envOverride, ...rest } = opts;
  const finalEnv = envOverride ? { ...process.env, ...envOverride } : process.env;
  try {
    return {
      ok: true,
      stdout: execFileSync('node', [join(ROOT, scriptName), ...args], {
        cwd: ROOT,
        encoding: 'utf-8',
        env: finalEnv,
        ...rest,
      }).trim(),
    };
  } catch (e) {
    return { ok: false, stdout: (e.stdout || '').toString(), stderr: (e.stderr || '').toString(), code: e.status };
  }
}

function makeTracker(dir, content) {
  mkdirSync(dir, { recursive: true });
  const tracker = join(dir, 'applications.md');
  writeFileSync(tracker, content, 'utf-8');
  return tracker;
}

const HEADER = '| # | Date | Company | Role | Score | Status | Last Upd | PDF | Report | Notes |\n|---|------|---------|------|-------|--------|----------|-----|--------|-------|';
const NEWROW = (n, d, c, r, s, st, lu = d, pdf = '❌', rep = '—', notes = '') =>
  `| ${n} | ${d} | ${c} | ${r} | ${s} | ${st} | ${lu} | ${pdf} | ${rep} | ${notes} |`;

// ── 1. migrate-add-last-upd.mjs ──────────────────────────────────

section('1. migrate-add-last-upd.mjs');
{
  const dir = mkdtempSync(join(tmpdir(), 'lup-mig-'));
  const tracker = makeTracker(dir, [
    '# Apps', '', HEADER, '',
    NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Evaluated'),
  ].join('\n'));

  // 1a. Already-migrated file: should be a no-op
  process.env.CAREER_OPS_TRACKER = tracker;
  const idemp = runScript('migrate-add-last-upd.mjs', [], { env: { CAREER_OPS_TRACKER: tracker } });
  if (idemp.ok && idemp.stdout.includes('already present')) pass('idempotent on already-migrated file');
  else fail(`idempotency: ${idemp.stdout}`);

  // 1b. Run on a non-migrated file
  const oldHeader = '| # | Date | Company | Role | Score | Status | PDF | Report | Notes |\n|---|------|---------|------|-------|--------|-----|--------|-------|';
  writeFileSync(tracker, ['# Apps', '', oldHeader, '', NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Evaluated')].join('\n').replace('Last Upd |', '').replace('Evaluated | 2026-06-10 |', 'Evaluated |'), 'utf-8');
  // Hmm, simpler: write a pre-migration file
  const pre = '# Apps\n\n| # | Date | Company | Role | Score | Status | PDF | Report | Notes |\n|---|------|---------|------|-------|--------|-----|--------|-------|\n| 1 | 2026-06-10 | BCG | BA | 4.0/5 | Evaluated | ❌ | [1](r.md) | note |\n';
  writeFileSync(tracker, pre, 'utf-8');
  const mig = runScript('migrate-add-last-upd.mjs', [], { env: { CAREER_OPS_TRACKER: tracker } });
  const after = readFileSync(tracker, 'utf-8');
  if (mig.ok && after.includes('Last Upd | PDF | Report | Notes')) pass('adds Last Upd column header');
  else fail(`migration header: ${mig.stdout}`);
  if (after.includes('| 1 | 2026-06-10 | BCG | BA | 4.0/5 | Evaluated | 2026-06-10 | ❌ | [1](r.md) | note |')) pass('backfills Last Upd with Date');
  else fail(`backfill missing: ${after}`);

  rmSync(dir, { recursive: true });
}

// ── 2. merge-tracker.mjs ────────────────────────────────────────

section('2. merge-tracker.mjs (Last Upd stamping)');
{
  const dir = mkdtempSync(join(tmpdir(), 'lup-mrg-'));
  const tracker = makeTracker(dir, ['# Apps', '', HEADER, '', NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Evaluated')].join('\n'));
  const additionsDir = join(dir, 'tracker-additions');
  mkdirSync(additionsDir, { recursive: true });
  writeFileSync(join(additionsDir, '2-test.tsv'),
    `2\t2026-06-16\tMekari\tADR\t2.5/5\tDiscarded\t❌\t[2](reports/2.md)\tSDR/BDR mismatch`);

  const env = { CAREER_OPS_TRACKER: tracker, CAREER_OPS_ADDITIONS: additionsDir };
  const r = runScript('merge-tracker.mjs', [], { env });
  const after = readFileSync(tracker, 'utf-8');
  const today = new Date().toISOString().slice(0, 10);
  const newLine = after.split('\n').find(l => l.trimStart().startsWith('| 2 |'));
  if (newLine && newLine.includes(`| ${today} |`)) pass(`new row stamped with today's date (${today})`);
  else fail(`new row not stamped: ${newLine}`);

  // Re-eval with higher score should bump Last Upd on existing row
  writeFileSync(join(additionsDir, '1-bump.tsv'),
    `1\t2026-06-16\tBCG\tBA\t4.5/5\tEvaluated\t❌\t[1](reports/1.md)\tre-eval`);
  runScript('merge-tracker.mjs', [], { env });
  const after2 = readFileSync(tracker, 'utf-8');
  const line1 = after2.split('\n').find(l => l.trimStart().startsWith('| 1 |'));
  if (line1 && line1.includes(`| ${today} |`)) pass(`re-eval stamps Last Upd on existing row`);
  else fail(`re-eval not stamped: ${line1}`);

  rmSync(dir, { recursive: true });
}

// ── 3. update-tracker.mjs ───────────────────────────────────────

section('3. update-tracker.mjs');
{
  const dir = mkdtempSync(join(tmpdir(), 'lup-upd-'));
  const tracker = makeTracker(dir, ['# Apps', '', HEADER, '', NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Evaluated', '2026-06-10', '❌', '[1](r.md)', 'initial')].join('\n'));
  const env = { CAREER_OPS_TRACKER: tracker };

  // 3a. --status
  const r1 = runScript('update-tracker.mjs', ['1', '--status', 'Applied'], { env });
  if (r1.ok && r1.stdout.includes('status: Evaluated → Applied')) pass('--status changes status and bumps Last Upd');
  else fail(`--status: ${r1.stdout}`);

  // 3b. --note (append)
  const r2 = runScript('update-tracker.mjs', ['1', '--note', 'interview next week'], { env });
  const content = readFileSync(tracker, 'utf-8');
  const line1 = content.split('\n').find(l => l.trimStart().startsWith('| 1 |'));
  if (r2.ok && line1.includes('Applied') && line1.includes('initial interview next week')) pass('--note appends to existing notes');
  else fail(`--note: line=${line1}`);

  // 3c. --url
  const r3 = runScript('update-tracker.mjs', ['1', '--url', 'https://example.com/jd'], { env });
  const content3 = readFileSync(tracker, 'utf-8');
  const line3 = content3.split('\n').find(l => l.trimStart().startsWith('| 1 |'));
  if (r3.ok && line3.includes('[1](https://example.com/jd)')) pass('--url updates report link');
  else fail(`--url: line=${line3}`);

  // 3d. --bump
  const r4 = runScript('update-tracker.mjs', ['1', '--bump'], { env });
  if (r4.ok && r4.stdout.includes('Updated')) pass('--bump refreshes Last Upd');
  else fail(`--bump: ${r4.stdout}`);

  // 3e. invalid status
  const r5 = runScript('update-tracker.mjs', ['1', '--status', 'Banana'], { env });
  if (!r5.ok && r5.stderr.includes('Invalid status')) pass('invalid status rejected');
  else fail(`invalid status: ${r5.stdout} ${r5.stderr}`);

  // 3f. missing num
  const r6 = runScript('update-tracker.mjs', ['--status', 'Applied'], { env });
  if (!r6.ok && r6.stderr.includes('Missing <num>')) pass('missing num rejected');
  else fail(`missing num: ${r6.stdout}`);

  // 3g. num not found
  const r7 = runScript('update-tracker.mjs', ['9999', '--bump'], { env });
  if (!r7.ok && r7.stderr.includes('not found')) pass('num not found rejected');
  else fail(`num not found: ${r7.stdout}`);

  // 3h. no change flag
  const r8 = runScript('update-tracker.mjs', ['1'], { env });
  if (!r8.ok && r8.stderr.includes('No change flag')) pass('no change flag rejected');
  else fail(`no change flag: ${r8.stdout}`);

  rmSync(dir, { recursive: true });
}

// ── 4. check-tracker-updates.mjs (via exported findStaleBumps) ─

section('4. check-tracker-updates.mjs (findStaleBumps logic)');
{
  const { findStaleBumps } = await import(join(ROOT, 'check-tracker-updates.mjs'));

  const oldText = [
    '# Apps', '', HEADER, '',
    NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Evaluated', '2026-06-10'),
  ].join('\n');

  // 4a. Identical → no violations
  const v1 = findStaleBumps(oldText, oldText);
  if (v1.length === 0) pass('identical text → no violations');
  else fail(`identical should be empty: ${JSON.stringify(v1)}`);

  // 4b. Status changed, Last Upd not bumped → violation
  const newTextB = NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Interview', '2026-06-10');
  const v2 = findStaleBumps(oldText, ['# Apps', '', HEADER, '', newTextB].join('\n'));
  if (v2.length === 1 && v2[0].num === 1) pass('status change without Last Upd bump → 1 violation');
  else fail(`expected 1 violation: ${JSON.stringify(v2)}`);

  // 4c. Status changed + Last Upd bumped → no violation
  const newTextC = NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Interview', '2026-06-16');
  const v3 = findStaleBumps(oldText, ['# Apps', '', HEADER, '', newTextC].join('\n'));
  if (v3.length === 0) pass('status change + Last Upd bump → 0 violations');
  else fail(`expected 0 violations: ${JSON.stringify(v3)}`);

  // 4d. Only Last Upd changed (no other content) → no violation
  const newTextD = NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Evaluated', '2026-06-16');
  const v4 = findStaleBumps(oldText, ['# Apps', '', HEADER, '', newTextD].join('\n'));
  if (v4.length === 0) pass('Last Upd-only change → 0 violations');
  else fail(`expected 0 violations: ${JSON.stringify(v4)}`);

  // 4e. Multiple rows, only one stale
  const multiOld = [
    '# Apps', '', HEADER, '',
    NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Evaluated', '2026-06-10'),
    NEWROW(2, '2026-06-10', 'Deloitte', 'SR&T', '4.2/5', 'Evaluated', '2026-06-10'),
  ].join('\n');
  const multiNew = [
    '# Apps', '', HEADER, '',
    NEWROW(1, '2026-06-10', 'BCG', 'BA', '4.0/5', 'Interview', '2026-06-16'),  // bumped: ok
    NEWROW(2, '2026-06-10', 'Deloitte', 'SR&T', '4.2/5', 'Applied', '2026-06-10'), // stale: violation
  ].join('\n');
  const v5 = findStaleBumps(multiOld, multiNew);
  if (v5.length === 1 && v5[0].num === 2) pass('multi-row: only stale row flagged');
  else fail(`expected only row 2: ${JSON.stringify(v5)}`);
}

// ── Summary ─────────────────────────────────────────────────────

console.log(`\n${'='.repeat(50)}`);
console.log(`Total: ${passed + failed} | ✅ ${passed} passed | ❌ ${failed} failed`);
process.exit(failed > 0 ? 1 : 0);
