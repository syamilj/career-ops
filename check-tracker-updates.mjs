#!/usr/bin/env node
/**
 * check-tracker-updates.mjs — Detect tracker rows that were edited without
 * bumping `Last Upd`. Used by the pre-commit hook in `hooks/pre-commit`.
 *
 * Exits 0 if all changes are clean (Last Upd was bumped on every modified row,
 * or no rows were modified). Exits 1 if any row was edited without a bump — the
 * hook then prints a remediation hint: `node update-tracker.mjs <num> --bump`.
 *
 * Usage:
 *   node check-tracker-updates.mjs [--staged|--working]   (default --staged)
 *
 * --staged   compare HEAD:applications.md to the staged version (default for pre-commit)
 * --working  compare HEAD:applications.md to the working tree (default for pre-push)
 */
import { readFileSync, existsSync } from 'fs';
import { execFileSync } from 'child_process';
import { join, dirname } from 'path';
import { fileURLToPath, pathToFileURL } from 'url';

const ROOT = dirname(fileURLToPath(import.meta.url));
const TRACKER_REL = existsSync(join(ROOT, 'data/applications.md'))
  ? 'data/applications.md'
  : 'applications.md';
const TRACKER_ABS = join(ROOT, TRACKER_REL);

const STAGED = !process.argv.includes('--working');

function parseRows(text) {
  const out = new Map();
  for (const line of text.split('\n')) {
    if (!line.trimStart().startsWith('|')) continue;
    if (line.includes('---')) continue;
    if (line.includes('Date |')) continue;
    const p = line.split('|').map(s => s.trim());
    if (p.length < 10) continue;
    const num = parseInt(p[1]);
    if (isNaN(num) || num === 0) continue;
    out.set(num, {
      num,
      date: p[2],
      company: p[3],
      role: p[4],
      score: p[5],
      status: p[6],
      lastUpd: p[7] || p[2],
      pdf: p[8] || '—',
      report: p[9] || '—',
      notes: p[10] || '',
    });
  }
  return out;
}

function contentSignature(r) {
  return [r.date, r.company, r.role, r.score, r.status, r.pdf, r.report, r.notes].join('\u0001');
}

/**
 * Compare two tracker texts and return rows that were edited without bumping Last Upd.
 * Exported so tests can call it directly without going through git.
 *
 * @param {string} oldText
 * @param {string} newText
 * @returns {Array<{num:number, company:string, role:string, lastUpd:string, line:string, lineIdx:number}>}
 */
export function findStaleBumps(oldText, newText) {
  const oldRows = parseRows(oldText);
  const newRows = parseRows(newText);
  const oldLines = oldText.split('\n');
  const newLines = newText.split('\n');
  const violations = [];
  for (let i = 0; i < newLines.length; i++) {
    const line = newLines[i];
    if (!line.trimStart().startsWith('|')) continue;
    const p = line.split('|').map(s => s.trim());
    if (p.length < 10) continue;
    const num = parseInt(p[1]);
    if (isNaN(num) || num === 0) continue;
    const oldRow = oldRows.get(num);
    if (!oldRow) continue;
    const newRow = newRows.get(num);
    const contentChanged = contentSignature(newRow) !== contentSignature(oldRow);
    const lastUpdBumped  = newRow.lastUpd !== oldRow.lastUpd;
    if (contentChanged && !lastUpdBumped) {
      violations.push({ num, company: newRow.company, role: newRow.role, lastUpd: newRow.lastUpd, line, lineIdx: i });
    }
  }
  return violations;
}

/**
 * Return a new tracker text with `Last Upd` set to today for every stale row.
 * Pure function — does not touch the filesystem.
 *
 * @param {string} text
 * @param {string} [today] ISO date; defaults to today
 * @returns {{ text: string, bumped: Array<{num:number, company:string, role:string, from:string, to:string}> }}
 */
export function autoBumpStaleRows(text, today = new Date().toISOString().slice(0, 10)) {
  // Build a "before" by zeroing out the Last Upd column of every row (so
  // findStaleBumps compares the user's actual edits vs. the pre-edit state
  // reconstructed from the *current* file with all Last Upds cleared).
  // This works because we only care about content columns — Last Upd is the
  // *consequence* of an edit, not the cause.
  const lines = text.split('\n');
  const cleared = lines.map((line) => {
    if (!line.trimStart().startsWith('|')) return line;
    const p = line.split('|').map(s => s.trim());
    if (p.length < 10) return line;
    if (p[1] === '' || isNaN(parseInt(p[1]))) return line;
    // Replace Last Upd (fields[7]) with Date (fields[2]) — pre-bump baseline.
    // This is safe: any row whose content has changed will now show as stale,
    // and any row whose content is unchanged will not (signature matches).
    const newP = [...p];
    newP[7] = p[2];
    // Reassemble. Preserve leading space pattern of original line.
    const leading = line.startsWith(' ') ? ' | ' : '| ';
    const trailing = line.trimEnd().endsWith('|') ? ' |' : '';
    return leading + newP.slice(1, -1).join(' | ') + trailing;
  }).join('\n');

  const stale = findStaleBumps(cleared, text);
  if (stale.length === 0) return { text, bumped: [] };

  const bumped = [];
  const newLines = text.split('\n');
  const staleByIdx = new Map(stale.map(s => [s.lineIdx, s]));
  for (const [lineIdx, st] of staleByIdx) {
    const line = newLines[lineIdx];
    const p = line.split('|').map(s => s.trim());
    const newP = [...p];
    newP[7] = today;
    const leading = line.startsWith(' ') ? ' | ' : '| ';
    const trailing = line.trimEnd().endsWith('|') ? ' |' : '';
    newLines[lineIdx] = leading + newP.slice(1, -1).join(' | ') + trailing;
    bumped.push({ num: st.num, company: st.company, role: st.role, from: st.lastUpd, to: today });
  }
  return { text: newLines.join('\n'), bumped };
}

// CLI entrypoint: only run when invoked directly (not when imported as a module).
const isMain = (() => {
  try {
    return import.meta.url === pathToFileURL(process.argv[1]).href;
  } catch { return false; }
})();

if (isMain) {
  const AUTO_FIX = process.argv.includes('--auto-fix');

  let oldText, newText;
  try {
    if (STAGED) {
      oldText = execFileSync('git', ['show', `HEAD:${TRACKER_REL}`], { cwd: ROOT, encoding: 'utf-8' });
      newText = execFileSync('git', ['show', `:${TRACKER_REL}`], { cwd: ROOT, encoding: 'utf-8' });
    } else {
      oldText = execFileSync('git', ['show', `HEAD:${TRACKER_REL}`], { cwd: ROOT, encoding: 'utf-8' });
      newText = readFileSync(TRACKER_ABS, 'utf-8');
    }
  } catch (err) {
    console.log('ℹ️  No HEAD version of tracker found — skipping Last Upd check (first commit or untracked).');
    process.exit(0);
  }

  const violations = findStaleBumps(oldText, newText);
  if (violations.length === 0) {
    process.exit(0);
  }

  if (!AUTO_FIX) {
    console.error('❌ Tracker rows changed without bumping `Last Upd`:');
    for (const v of violations) {
      console.error(`   • #${v.num} ${v.company} — ${v.role}  (Last Upd still: ${v.lastUpd})`);
    }
    console.error('\nFix with one of:');
    console.error('  node update-tracker.mjs <num> --bump     # refresh Last Upd to today');
    console.error('  node update-tracker.mjs <num> --status <new>  # also update the status (auto-bumps)');
    console.error('  node update-tracker.mjs <num> --note "..."    # also append a note (auto-bumps)');
    console.error('\nOr re-run this script with --auto-fix to bump them automatically.');
    process.exit(1);
  }

  // --auto-fix: rewrite the tracker with Last Upd = today for every stale row,
  // then re-stage so the commit picks up the fix. The user never sees an error.
  const targetText = STAGED ? newText : readFileSync(TRACKER_ABS, 'utf-8');
  const { text, bumped } = autoBumpStaleRows(targetText);

  if (bumped.length === 0) {
    // No real edits to bump (e.g. only formatting changes). Exit clean.
    process.exit(0);
  }

  writeFileSync(TRACKER_ABS, text, 'utf-8');
  // Re-stage so the commit includes the bump.
  try {
    execFileSync('git', ['add', TRACKER_REL], { cwd: ROOT });
  } catch (err) {
    console.error(`⚠️  Auto-bumped ${TRACKER_REL} on disk but failed to re-stage: ${err.message}`);
    console.error('   You may need to `git add` the file manually before committing.');
  }

  console.log(`🔧 Auto-bumped Last Upd on ${bumped.length} row(s):`);
  for (const b of bumped) {
    console.log(`   • #${b.num} ${b.company} — ${b.role}  (${b.from} → ${b.to})`);
  }
  process.exit(0);
}
