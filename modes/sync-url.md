# Mode: sync-url — Single-Job URL Hygiene

## Purpose

When the user has updated a job's link/info externally, re-verify that **one**
job's URL via LinkedIn MCP and patch its report header and tracker row.

Trigger phrases (route here from the agent's startup detection, alongside
`auto-pipeline`):

- `update reports` / `update laporan` / `update report`
- `sinkronkan url` / `sync url` / `sync-url` / `sync the url`
- `perbaiki link` / `fix link` / `refresh link` / `refresh report`
- Explicit: `/career-ops sync-url <entry|company|role>`

**Scope: exactly one job per invocation.** For multi-job sweeps, use the
manual workflow in `modes/oferta.md` (run each one explicitly) or batch mode.

## Inputs

- `data/applications.md` — application tracker (entry number → file path)
- `reports/` — evaluation reports (target file headers)
- LinkedIn MCP — `linkedin_search_jobs`, `linkedin_get_job_details`,
  `linkedin_get_company_profile` (for the verification step)

## Step 1 — Identify the target job

Accept the job hint in any of these forms. Most-specific wins.

| Hint form | Example | Resolution |
|-----------|---------|------------|
| Entry number | `#59` or `entry 59` or `nomor 59` | look up `data/applications.md` row with `| 59 |` |
| Company slug / role text | `fore strategy associate`, `paragon fpa` | filter `data/applications.md` on Company + Role columns (case-insensitive substring) |
| Explicit `sync-url` with no hint | `/career-ops sync-url` | most recent Evaluated entry (by `Date` desc) whose report file has an alias URL, root-careers URL, or no `**URL:**` line |
| Trigger phrase only | `update laporan` | same as "explicit with no hint" |

**Filename vs. entry-number divergence** (carry-over from prior session): the
tracker `#` and the report filename `{###}` can differ. Example: tracker #108
A&M → file `081-alvarez-marsal-restructuring-sg-2026-06-09.md`. Always
`ls reports/ | grep -i <company-slug>` and confirm by filename, not by entry
number alone.

**If still ambiguous** (2+ candidates within the hint), show a numbered list of
the top 3 candidates with `# | Company | Role | Score | URL state` and ask the
user to pick. Do NOT auto-pick.

## Step 2 — Read the current state

1. Read the report file (top 20 lines) and capture:
   - `**URL:**` value, or note absence
   - `**Score:**` / `**Archetype:**` (read-only, for confirmation)
   - `**Status:**` (read-only — see `D-status-preserve` rule below)
2. Read the matching `data/applications.md` row to find the Notes column
   position and confirm the entry still exists.

## Step 3 — Verify via LinkedIn MCP

Carry over the prior session's refined rules verbatim:

**`D-eval`:** `linkedin_search_jobs(keywords=<short company>, location=<city>)`
→ candidate `job_id`s → `linkedin_get_job_details(job_id)` returns a non-empty
`job_posting` payload. Only proceed to write when liveness is confirmed.

**`D-promote`:** prefer simple company-only keyword searches over long compound
queries (compound queries trigger LinkedIn's promoted-ads fallback and hide
real matches). If a company-only search is empty, fall back to
`linkedin_get_company_profile(company_name=<slug>, sections="jobs")`. If both
fail, the role isn't on LinkedIn (small PE/VC firms, internal ATS) — record a
closure note instead of a URL.

Pick the best match when multiple results come back: closest title → closest
location → freshest posted-date.

## Step 4 — Patch the report

Edit `reports/{###}-{slug}-{YYYY-MM-DD}.md` in place.

- If `**URL:**` exists: replace its value (and inline re-verification comment,
  if any) with:
  `https://www.linkedin.com/jobs/view/<job_id>/  <!-- re-verified {YYYY-MM-DD} via LinkedIn MCP; live: {verified title}, {company}, {location} -->`
- If `**URL:**` is missing: insert the line between `**Score:**` and `**PDF:**`
  (per the standard header order in AGENTS.md).
- If a closure note applies: append a new line directly under the existing
  `**URL:**` (or where it would have been) reading:
  `**URL:** {existing or careers-page URL}  <!-- re-checked {YYYY-MM-DD}: not findable on LinkedIn; manual check at {careers URL} -->`

**`D-status-preserve`:** NEVER change `**Status:**`, `**Score:**`, or
`**Archetype:**`. URL hygiene is informational only. Status changes are
explicit, separate operations (`modes/tracker.md` handles those).

## Step 5 — Patch the tracker

Edit the matching row in `data/applications.md`. Replace any prior URL-hygiene
note with:

```
URL re-verified {YYYY-MM-DD}: https://www.linkedin.com/jobs/view/<job_id>/
```

Or for closure:

```
URL not findable via LinkedIn MCP {YYYY-MM-DD} — manual check at <careers URL>
```

Do not touch the Status column, Score column, or any other column.

## Step 6 — Confirm

Show the user a one-screen summary:

```
✓ Synced #{entry} {Company} — {Role}
  Old: {old URL or "(none)"}
  New: {new URL or "closure note: <careers URL>"}
  Files: reports/{###}-{slug}-{date}.md, data/applications.md (Notes column)
```

## Edge cases

- `**URL:**` line in a different format (markdown link, not bold, or backticks)
  → fall back to a regex on `https?://` in the first 20 lines.
- Report has 2+ `**URL:**` lines (corrupt edit) → surface to the user with the
  exact line numbers; do NOT auto-fix.
- Entry number not found in tracker → list the 5 closest entries by date and
  ask the user to pick.
- Report file missing entirely → stop with a clear "report file not found for
  entry #{N}" message; do NOT create a new report (that's `oferta` mode's job).
- LinkedIn MCP returns 403 / auth-expired → stop with "LinkedIn MCP session
  expired — re-auth via `claude mcp list`" and do not write anything.
- New URL reveals location deviation (e.g. SG role is actually Jakarta-only) →
  flag it to the user explicitly in the Step 6 summary, do not silently
  substitute.

## Rules

- **Single job only.** This mode never processes more than one entry. If the
  user wants a sweep, point them at the manual workflow in `modes/oferta.md`.
- **No analysis rewrite.** This mode touches only the report header
  `**URL:**` line. Sections A–F and Block G are owned by `oferta.md` / a full
  re-evaluation. If the JD content has materially changed, recommend running
  `/career-ops oferta {new URL}` instead.
- **`D-status-preserve` is load-bearing.** Status changes are a separate,
  explicit operation. Do not roll a "URL not findable" outcome into a status
  change.
- **Idempotent.** Re-running on the same job should be a no-op if nothing
  changed (still print the Step 6 summary so the user sees confirmation).
