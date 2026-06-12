# Mode: latex — LaTeX/Overleaf CV Export

Export a tailored, ATS-optimized CV as a `.tex` file and compile it to PDF via `tectonic` or `pdflatex`.

## Pipeline

1. Read `cv.md` as source of truth
2. Read `config/profile.yml` for candidate identity and contact info
3. Ask the user for the JD if not already in context (text or URL)
4. Extract 15-20 keywords from the JD
5. Detect JD language → CV language (EN default)
6. Detect role archetype → adapt framing
7. Rewrite Professional Summary injecting JD keywords (same rules as `pdf` mode — NEVER invent skills)
8. Select top 3-4 most relevant projects for the offer
9. Reorder experience bullets by JD relevance
10. Inject keywords naturally into existing achievements
11. Generate the `.tex` file using `templates/cv-template.tex`
12. Write to `output/cv-{candidate}-{company}-{YYYY-MM-DD}.tex`
13. Run: `node generate-latex.mjs output/cv-{candidate}-{company}-{YYYY-MM-DD}.tex output/cv-{candidate}-{company}-{YYYY-MM-DD}.pdf`
14. **Check page count (MANDATORY):** the JSON report includes `pdf.pageCount`. If > 1, condense content (fewer bullets per older role, trim least-relevant items) and recompile until exactly 1 page. NEVER shrink fonts or margins to force a fit.
15. Report: .tex path, .pdf path, file sizes, page count, section count, keyword coverage %

## One-Page Rule (CRITICAL)

The compiled PDF MUST be exactly 1 page, matching the design in `templates/cv-example.jpg`:
- Max 3 bullets for the most recent/relevant roles, 2 for older ones
- Merge short related bullets; cut items with the least JD relevance
- Fit by trimming content, not by shrinking text or margins

**Requires:** `tectonic` (preferred — `brew install tectonic`, auto-downloads packages) or `pdflatex` (MiKTeX / TeX Live) on PATH.

## Template Placeholders

The template at `templates/cv-template.tex` uses `{{PLACEHOLDER}}` syntax:

| Placeholder | Source |
|-------------|--------|
| `{{NAME}}` | `profile.yml → candidate.full_name` |
| `{{LOCATION}}` | City, Country from profile.yml (e.g. `Jakarta, Indonesia`) |
| `{{EMAIL_URL}}` | Raw email for `mailto:` URL — must not be LaTeX-escaped (from profile.yml) |
| `{{EMAIL_DISPLAY}}` | Escaped email for display text — LaTeX-special chars like `_` must be escaped, e.g. `first\_name@example.com` |
| `{{PHONE_URL}}` | Phone for `tel:` URL — digits and leading `+` only, NO spaces/dashes (e.g. `+6285161112223`) |
| `{{PHONE_DISPLAY}}` | Phone display text (may contain spaces) |
| `{{LINKEDIN_URL}}` | Full URL with scheme for `\href{}`: e.g. `https://linkedin.com/in/username`. If `profile.yml` stores a bare host+path (no scheme), prepend `https://` before substitution. |
| `{{LINKEDIN_DISPLAY}}` | Display text only (no scheme): `linkedin.com/in/username` |
| `{{GITHUB_URL}}` | Full URL with scheme for `\href{}`: e.g. `https://github.com/username`. If `profile.yml` stores a bare host+path, prepend `https://`. |
| `{{GITHUB_DISPLAY}}` | Display text only (no scheme): `github.com/username` |
| `{{SUMMARY}}` | Personalized Professional Summary with JD keywords (escaped, italic in template) |
| `{{EDUCATION}}` | LaTeX `\resumeSubheading` blocks from cv.md Education section |
| `{{EXPERIENCE}}` | LaTeX `\resumeSubheading` + `\resumeItem` blocks — reordered bullets |
| `{{AWARDS}}` | `\item` lines from cv.md Honors & Awards (inside Additional Information) |
| `{{CERTIFICATIONS}}` | `\item` lines: languages + certifications (inside Additional Information) |
| `{{SKILLS}}` | `\item` lines from cv.md Technical Skills (inside Additional Information) |

**Section order in template (matches `cv-example.jpg`):** Header → Professional Summary → Education → Professional Experience → Additional Information (Awards, Languages & Certifications, Technical Skills).

## LaTeX Content Generation Rules

### Education

Each entry becomes:

```latex
    \resumeSubheading
    {Institution}{City, State}
    {Degree}{Date Range}
```

If coursework exists, add:

```latex
        \resumeItemListStart
            \resumeItem{\textbf{Coursework:} Course1, Course2, ...}
        \resumeItemListEnd
```

### Experience

Each role becomes:

```latex
    \resumeSubheading
      {Company}{Date Range}
      {Role Title}{Location}
      \resumeItemListStart
        \resumeItem{Bullet text with JD keywords injected}
        ...
      \resumeItemListEnd
```

### Additional Information (Awards / Certifications / Skills)

Each line becomes a plain `\item`:

```latex
    \item 1st Place — National Business Plan Competition: BUMDes Incubation Model (270+ teams, Nov 2021).
```

For skills, group inline on one or two `\item` lines to save vertical space:

```latex
    \item \textbf{Excel:} Advanced (Pivot Tables, XLOOKUP, INDEX/MATCH, macros). \textbf{Statistical:} EViews, STATA, SPSS. \textbf{Programming:} Python (Pandas, NumPy), SQL.
```

## LaTeX Escaping (CRITICAL)

All text content MUST be escaped for LaTeX before insertion:

| Character | Escape |
|-----------|--------|
| `&` | `\&` |
| `%` | `\%` |
| `$` | `\$` |
| `#` | `\#` |
| `_` | `\_` |
| `{` | `\{` |
| `}` | `\}` |
| `~` | `\textasciitilde{}` |
| `^` | `\textasciicircum{}` |
| `\` | `\textbackslash{}` |
| `±` | `$\pm$` |
| `→` | `$\rightarrow$` |

**Exception:** Do NOT escape LaTeX commands themselves (`\resumeItem`, `\textbf`, etc.) — only user-supplied text content.

**Exception for URLs:** Do NOT escape text inside `\href{URL}{...}` first arguments. The URL must remain raw (or RFC 3986 percent-encoded). Only escape the *display text* (second argument). For example:
```latex
\href{https://example.com/path_with_underscores}{Example\_Display}
```

## ATS Rules (same as pdf mode)

- Single-column layout (enforced by template)
- Standard section headers: Professional Summary, Education, Professional Experience, Additional Information
- UTF-8, machine-readable via `\pdfgentounicode=1`
- Keywords distributed: first bullet of each role, skills section
- No images, no graphics, no color in body text

## Keyword Injection Strategy

Same ethical rules as `modes/pdf.md`:
- NEVER add skills the candidate doesn't have
- Only reformulate existing experience using JD vocabulary
- Examples:
  - JD says "RAG pipelines" → reword "LLM workflows with retrieval" to "RAG pipeline design"
  - JD says "MLOps" → reword "observability, evals" to "MLOps and observability"

## Overleaf Compatibility

The generated `.tex` file uses only standard CTAN packages (no custom or bundled dependencies):

- `latexsym`, `fullpage`, `titlesec`, `marvosym`, `color`, `verbatim`, `enumitem`
- `hyperref`, `fancyhdr`, `babel`, `tabularx`, `fontawesome5`, `multicol`, `glyphtounicode`

Upload the `.tex` file directly to Overleaf — compiles with no extra configuration.
