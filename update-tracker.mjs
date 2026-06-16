#!/usr/bin/env node
/**
 * update-tracker.mjs — Update an existing tracker row and auto-bump `Last Upd`.
 *
 * This is the supported way to change a job's status/URL/note/score from the
 * command line. Editing `data/applications.md` by hand still works, but a
 * pre-commit hook warns when Last Upd is left stale.
 *
 * Usage:
 *   node update-tracker.mjs <num> --status <new>          # Evaluated|Applied|Responded|Interview|Offer|Rejected|Discarded|SKIP
 *   node update-tracker.mjs <num> --score  <3.8/5>        # new score
 *   node update-tracker.mjs <num> --note  "text"          # append note (default) or replace with --note-replace
 *   node update-tracker.mjs <num> --note-replace "text"   # replace notes
 *   node update-tracker.mjs <num> --url   "https://..."   # updates the [N](reports/...) link in the Report column
 *   node update-tracker.mjs <num> --bump                  # only refresh Last Upd to today
 *   node update-tracker.mjs <num> --dry-run               # show what would change
 *
 * Multiple flags combine: --status Interview --note "Onsite 2026-06-20"
 *
 * Status values are validated against templates/states.yml (canonical names).
 * Last Upd is set to today's local date (YYYY-MM-DD) on every successful update.
 *
 * Exit codes:
 *   0  success / dry-run with no errors
 *   1  invalid input (bad num, invalid status, no flag given)
 *   2  num not found in tracker
 */
import { readFileSync, writeFileSync, existsSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const ROOT = dirname(fileURLToPath(import.meta.url));
const APPS_FILE = process.env.CAREER_OPS_TRACKER
  ? process.env.CAREER_OPS_TRACKER
  : existsSync(join(ROOT, 'data/applications.md'))
    ? join(ROOT, 'data/applications.md')
    : join(ROOT, 'applications.md');

const CANONICAL_STATES = ['Evaluated', 'Applied', 'Responded', 'Interview', 'Offer', 'Rejected', 'Discarded', 'SKIP'];

function todayISO() {
  const d = new Date();
  return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');
}

function parseArgs(argv) {
  const args = argv.slice(2);
  const opts = { dryRun: false, noteMode: 'append' };
  let num = null;
  for (let i = 0; i < args.length; i++) {
    const a = args[i];
    if (a === '--dry-run') { opts.dryRun = true; continue; }
    if (a === '--bump')    { opts.bump = true; continue; }
    if (a === '--status')  { opts.status = args[++i]; continue; }
    if (a === '--score')   { opts.score = args[++i]; continue; }
    if (a === '--note')    { opts.note = args[++i]; continue; }
    if (a === '--note-replace') { opts.note = args[++i]; opts.noteMode = 'replace'; continue; }
    if (a === '--url')     { opts.url = args[++i]; continue; }
    if (a === '--pdf')     { opts.pdf = args[++i]; continue; }
    if (a === '-h' || a === '--help') { opts.help = true; continue; }
    if (num === null && /^\d+$/.test(a)) { num = a; continue; }
    console.error(`❌ Unknown argument: ${a}`);
    process.exit(1);
  }
  return { num, ...opts };
}

function showHelp() {
  console.log(`update-tracker.mjs — update a tracker row and auto-bump Last Upd

Usage:
  node update-tracker.mjs <num> [flags]

Flags:
  --status <Evaluated|Applied|Responded|Interview|Offer|Rejected|Discarded|SKIP>
  --score  <X.X/5>
  --note   "text"          append to Notes (default)
  --note-replace "text"    replace Notes entirely
  --url    "https://..."   update the report link
  --pdf    <✅|❌|—>        update the PDF column
  --bump                   only refresh Last Upd to today
  --dry-run                show diff, do not write
  -h, --help               this help

Examples:
  node update-tracker.mjs 194 --status Applied
  node update-tracker.mjs 194 --status Interview --note "Onsite scheduled 2026-06-20"
  node update-tracker.mjs 191 --url "https://example.com/jd"
  node update-tracker.mjs 1 --bump
`);
}

function parseRow(line) {
  const parts = line.split('|').map(s => s.trim());
  if (parts.length < 10) return null;
  const num = parseInt(parts[1]);
  if (isNaN(num) || num === 0) return null;
  return {
    num,
    date: parts[2],
    company: parts[3],
    role: parts[4],
    score: parts[5],
    status: parts[6],
    lastUpd: parts[7] || parts[2],
    pdf: parts[8] || '—',
    report: parts[9] || '—',
    notes: parts[10] || '',
    raw: line,
  };
}

function rowToLine(row) {
  return `| ${row.num} | ${row.date} | ${row.company} | ${row.role} | ${row.score} | ${row.status} | ${row.lastUpd} | ${row.pdf} | ${row.report} | ${row.notes} |`;
}

function validateStatus(s) {
  const clean = s.replace(/\*\*/g, '').trim();
  const lower = clean.toLowerCase();
  for (const v of CANONICAL_STATES) if (v.toLowerCase() === lower) return v;
  return null;
}

function main() {
  const opts = parseArgs(process.argv);
  if (opts.help) { showHelp(); return; }
  if (!opts.num) { console.error('❌ Missing <num> argument'); showHelp(); process.exit(1); }

  const noChange = !opts.status && !opts.score && !opts.note && !opts.url && !opts.pdf && !opts.bump;
  if (noChange) { console.error('❌ No change flag given. Use --status, --score, --note, --url, --pdf, or --bump'); process.exit(1); }

  if (!existsSync(APPS_FILE)) { console.error(`❌ Tracker not found: ${APPS_FILE}`); process.exit(2); }

  if (opts.status) {
    const valid = validateStatus(opts.status);
    if (!valid) {
      console.error(`❌ Invalid status "${opts.status}". Canonical: ${CANONICAL_STATES.join(', ')}`);
      process.exit(1);
    }
    opts.status = valid;
  }

  const content = readFileSync(APPS_FILE, 'utf-8');
  const lines = content.split('\n');
  const target = parseInt(opts.num);

  let rowIdx = -1;
  for (let i = 0; i < lines.length; i++) {
    const r = parseRow(lines[i]);
    if (r && r.num === target) { rowIdx = i; break; }
  }
  if (rowIdx === -1) {
    console.error(`❌ #${target} not found in tracker (${APPS_FILE})`);
    process.exit(2);
  }

  const row = parseRow(lines[rowIdx]);
  const today = todayISO();
  const changes = [];
  const oldLastUpd = row.lastUpd;

  if (opts.status && opts.status !== row.status) {
    changes.push(`status: ${row.status} → ${opts.status}`);
    row.status = opts.status;
  }
  if (opts.score && opts.score !== row.score) {
    changes.push(`score: ${row.score} → ${opts.score}`);
    row.score = opts.score;
  }
  if (opts.note !== undefined) {
    if (opts.noteMode === 'replace') {
      if (opts.note !== row.notes) { changes.push(`notes: replaced (${row.notes.length} → ${opts.note.length} chars)`); row.notes = opts.note; }
    } else {
      const before = row.notes;
      row.notes = (before ? before + ' ' : '') + opts.note;
      if (row.notes !== before) changes.push(`notes: appended (+${opts.note.length} chars)`);
    }
  }
  if (opts.url) {
    // Update the [N](path) link in the Report column to point at the new URL.
    // If there is no [N] link, prepend a fresh one.
    const m = row.report.match(/\[(\d+)\]\(([^)]+)\)/);
    const label = m ? m[1] : String(row.num);
    const newReport = `[${label}](${opts.url})`;
    if (newReport !== row.report) {
      changes.push(`report: ${row.report} → ${newReport}`);
      row.report = newReport;
    }
  }
  if (opts.pdf && opts.pdf !== row.pdf) {
    changes.push(`pdf: ${row.pdf} → ${opts.pdf}`);
    row.pdf = opts.pdf;
  }

  if (changes.length === 0 && !opts.bump) {
    console.log(`ℹ️  #${target} ${row.company} — ${row.role}: no actual changes (values already match). Last Upd not bumped.`);
    return;
  }

  // Always bump Last Upd when at least one real change is made.
  if (changes.length > 0) row.lastUpd = today;
  else if (opts.bump) row.lastUpd = today;

  const newLine = rowToLine(row);
  const oldLine = lines[rowIdx];
  lines[rowIdx] = newLine;

  console.log(`\n📝 #${target} ${row.company} — ${row.role}`);
  for (const c of changes) console.log(`   • ${c}`);
  if (row.lastUpd !== oldLastUpd) console.log(`   • lastUpd: ${oldLastUpd} → ${row.lastUpd}`);

  if (opts.dryRun) {
    console.log('\n(dry-run — no changes written)');
    console.log(`\nOLD: ${oldLine}`);
    console.log(`NEW: ${newLine}`);
    return;
  }

  writeFileSync(APPS_FILE, lines.join('\n'), 'utf-8');
  console.log(`\n✅ Updated ${APPS_FILE}`);
}

main();
