package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBumpTrackerLastUpdUpdatesOnlyMatchingRow verifies the helper changes
// only the Last Upd cell of the row matching the report number, leaving other
// rows untouched.
func TestBumpTrackerLastUpdUpdatesOnlyMatchingRow(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tracker := `# Applications Tracker

| # | Date | Company | Role | Score | Status | Last Upd | PDF | Report | Notes |
|---|------|---------|------|-------|--------|----------|-----|--------|-------|
| 1 | 2026-06-01 | Acme | BA | 4.0/5 | Evaluated | 2026-06-01 | ❌ | [1](reports/1.md) | note a |
| 2 | 2026-06-02 | Beta | SA | 3.5/5 | Applied | 2026-06-02 | ❌ | [2](reports/2.md) | note b |
`
	if err := os.WriteFile(filepath.Join(dataDir, "applications.md"), []byte(tracker), 0o644); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	if err := bumpTrackerLastUpd(dir, "1", today); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dataDir, "applications.md"))
	gs := string(got)
	if !strings.Contains(gs, "| 1 | 2026-06-01 | Acme | BA | 4.0/5 | Evaluated | "+today+" |") {
		t.Errorf("row 1 not bumped:\n%s", gs)
	}
	if !strings.Contains(gs, "| 2 | 2026-06-02 | Beta | SA | 3.5/5 | Applied | 2026-06-02 |") {
		t.Errorf("row 2 was changed (should be untouched):\n%s", gs)
	}
}

// TestBumpReportLastUpdatedRewritesHeaderLine verifies the helper replaces the
// existing `**Last Updated:**` line in the report file with today.
func TestBumpReportLastUpdatedRewritesHeaderLine(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(reportsDir, "x.md")
	content := "# Eval\n\n**Date:** 2026-06-01\n**Last Updated:** 2026-06-01\n**URL:** https://example.com\n**Score:** 4.0/5\n"
	if err := os.WriteFile(report, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	if err := bumpReportLastUpdated(dir, "reports/x.md", today); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(report)
	gs := string(got)
	if !strings.Contains(gs, "**Last Updated:** "+today) {
		t.Errorf("Last Updated not bumped:\n%s", gs)
	}
	if strings.Contains(gs, "**Last Updated:** 2026-06-01") {
		t.Errorf("old date still present:\n%s", gs)
	}
}

// TestBumpReportLastUpdatedInsertsWhenMissing verifies the helper inserts a
// new `**Last Updated:**` line after `**Score:**` when the report has none.
func TestBumpReportLastUpdatedInsertsWhenMissing(t *testing.T) {
	dir := t.TempDir()
	reportsDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := filepath.Join(reportsDir, "y.md")
	content := "# Eval\n\n**Date:** 2026-06-01\n**Score:** 3.0/5\n"
	if err := os.WriteFile(report, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format("2006-01-02")
	if err := bumpReportLastUpdated(dir, "reports/y.md", today); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(report)
	gs := string(got)
	if !strings.Contains(gs, "**Last Updated:** "+today) {
		t.Errorf("Last Updated not inserted:\n%s", gs)
	}
	// Should appear after the Score line.
	scoreIdx := strings.Index(gs, "**Score:**")
	updIdx := strings.Index(gs, "**Last Updated:**")
	if scoreIdx == -1 || updIdx == -1 || updIdx < scoreIdx {
		t.Errorf("Last Updated not after Score (scoreIdx=%d, updIdx=%d):\n%s", scoreIdx, updIdx, gs)
	}
}
