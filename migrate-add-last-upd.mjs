#!/usr/bin/env node
/**
 * migrate-add-last-upd.mjs — One-time migration: add `Last Upd` column to applications.md
 *
 * Inserts `| Last Upd |` after the Status column and backfills existing rows with
 * the Date value as the initial Last Upd (lower bound — Date is the most recent
 * programmatic write we can prove; manual edits between then and now are unknown).
 *
 * Run: node migrate-add-last-upd.mjs [--dry-run]
 */
import { readFileSync, writeFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const ROOT = dirname(fileURLToPath(import.meta.url));
const APPS_FILE = process.env.CAREER_OPS_TRACKER
  ? process.env.CAREER_OPS_TRACKER
  : join(ROOT, 'data/applications.md');
const DRY_RUN = process.argv.includes('--dry-run');

const STATUS_COL = 6; // 1-indexed: # | Date | Company | Role | Score | Status | PDF | Report | Notes
// After: # | Date | Company | Role | Score | Status | Last Upd | PDF | Report | Notes

const content = readFileSync(APPS_FILE, 'utf-8');
const lines = content.split('\n');
// Idempotency check: if the header already has the Last Upd column, do nothing.
if (content.includes('Last Upd | PDF | Report | Notes')) {
  console.log('ℹ️  Last Upd column already present — nothing to migrate.');
  process.exit(0);
}
let headerTouched = 0, sepTouched = 0, dataRowsTouched = 0;
const newLines = lines.map((line) => {
  if (line.startsWith('| # | Date | Company | Role | Score | Status |') && !line.includes('Last Upd')) {
    headerTouched++;
    return '| # | Date | Company | Role | Score | Status | Last Upd | PDF | Report | Notes |';
  }
  if (line.startsWith('|---|') && line.includes('------')) {
    sepTouched++;
    return '|---|------|---------|------|-------|--------|----------|-----|--------|-------|';
  }
  if (line.trimStart().startsWith('|')) {
    // Data row: split, validate count, insert Date into Last Upd position
    const parts = line.split('|').map(s => s.trim());
    // parts[0] is empty (before first |), parts[1]=#, ..., parts[9]=notes
    if (parts.length < 10) return line; // malformed, leave
    const date = parts[2];
    const newParts = [
      parts[0], // empty
      parts[1], // #
      parts[2], // Date
      parts[3], // Company
      parts[4], // Role
      parts[5], // Score
      parts[6], // Status
      date,     // Last Upd (new) = Date as initial value
      parts[7], // PDF
      parts[8], // Report
      parts[9], // Notes
    ];
    dataRowsTouched++;
    return line.startsWith(' ') ? ' | ' + newParts.slice(1).join(' | ') + ' |' : '| ' + newParts.slice(1).join(' | ') + ' |';
  }
  return line;
});

const out = newLines.join('\n');
if (DRY_RUN) {
  console.log(`(dry-run) Would update: ${headerTouched} header, ${sepTouched} separator, ${dataRowsTouched} data rows`);
  console.log('--- First 3 data rows after migration ---');
  for (const l of newLines) {
    if (l.startsWith('|') && !l.includes('---') && !l.includes('Date |')) {
      console.log(l);
      if (dataRowsTouched > 0 && --dataRowsTouched === 0) break;
    }
  }
} else {
  writeFileSync(APPS_FILE, out, 'utf-8');
  console.log(`✅ Migrated: ${headerTouched} header, ${sepTouched} separator, ${dataRowsTouched} data rows`);
  console.log(`   File: ${APPS_FILE}`);
}
