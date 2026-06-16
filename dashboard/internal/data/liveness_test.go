package data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santifer/career-ops/dashboard/internal/model"
)

func mkApp(reportPath string) model.CareerApplication {
	return model.CareerApplication{Company: "Acme", ReportPath: reportPath, ReportNumber: "001"}
}

func TestClassifyLiveness(t *testing.T) {
	longBody := strings.Repeat("lorem ipsum dolor sit amet ", 20)

	cases := []struct {
		name     string
		status   int
		finalURL string
		body     string
		want     string
	}{
		{"http 404", 404, "https://x.com/job", longBody, LiveExpired},
		{"http 410", 410, "https://x.com/job", longBody, LiveExpired},
		{"error redirect", 200, "https://x.com/jobs?error=true", longBody, LiveExpired},
		{"position filled", 200, "https://x.com/job", "Sorry, this position has been filled. " + longBody, LiveExpired},
		{"no longer accepting", 200, "https://x.com/job", "We are no longer accepting applications. " + longBody, LiveExpired},
		{"indonesian closed", 200, "https://x.com/job", "Lowongan ini sudah ditutup. " + longBody, LiveExpired},
		{"apply visible", 200, "https://x.com/job", longBody + " Apply now for this role", LiveActive},
		{"lamar visible", 200, "https://x.com/job", longBody + " Lamar sekarang", LiveActive},
		{"listing page", 200, "https://x.com/jobs", "123 jobs found " + longBody, LiveExpired},
		{"thin content", 200, "https://x.com/job", "nav footer", LiveExpired},
		{"content no apply", 200, "https://x.com/job", longBody, LiveUncertain},
		// expired pattern must win over apply text (project rule)
		{"expired beats apply", 200, "https://x.com/job", "This job has expired. Apply to other roles. " + longBody, LiveExpired},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyLiveness(c.status, c.finalURL, c.body)
			if got.State != c.want {
				t.Errorf("classifyLiveness(%q) = %s (%s), want %s", c.name, got.State, got.Reason, c.want)
			}
		})
	}
}

func TestRejectPrivateOrInvalid(t *testing.T) {
	for _, bad := range []string{
		"http://localhost/x", "http://127.0.0.1/x", "http://10.0.0.5/x",
		"http://192.168.1.1/x", "http://172.16.0.1/x", "ftp://example.com/x", "::not a url::",
	} {
		if rejectPrivateOrInvalid(bad) == "" {
			t.Errorf("expected rejection for %q", bad)
		}
	}
	for _, good := range []string{"https://careers.example.com/job/1", "http://example.com"} {
		if r := rejectPrivateOrInvalid(good); r != "" {
			t.Errorf("unexpected rejection for %q: %s", good, r)
		}
	}
}

func TestHTMLToText(t *testing.T) {
	html := `<html><head><style>.x{color:red}</style><script>var a=1;</script></head>
	<body><h1>Data Analyst</h1><button>Apply now</button></body></html>`
	text := htmlToText(html)
	if strings.Contains(text, "color:red") || strings.Contains(text, "var a=1") {
		t.Errorf("script/style not stripped: %q", text)
	}
	if !strings.Contains(text, "Data Analyst") || !strings.Contains(text, "Apply now") {
		t.Errorf("visible text lost: %q", text)
	}
}

func TestLivenessCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cache := map[string]LivenessResult{
		"https://x.com/job/1": {State: LiveActive, Reason: "apply control visible", CheckedAt: time.Now().Truncate(time.Second)},
		"https://x.com/job/2": {State: LiveExpired, Reason: "HTTP 404", CheckedAt: time.Now().Add(-48 * time.Hour).Truncate(time.Second)},
	}
	if err := SaveLivenessCache(dir, cache); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded := LoadLivenessCache(dir)
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	if !loaded["https://x.com/job/1"].IsFresh() {
		t.Error("fresh entry reported stale")
	}
	if loaded["https://x.com/job/2"].IsFresh() {
		t.Error("48h-old entry reported fresh")
	}
	if loaded["https://x.com/job/2"].Reason != "HTTP 404" {
		t.Errorf("reason mismatch: %q", loaded["https://x.com/job/2"].Reason)
	}
}

func TestLoadLivenessCacheMissingFile(t *testing.T) {
	if got := LoadLivenessCache(t.TempDir()); len(got) != 0 {
		t.Errorf("expected empty cache, got %d entries", len(got))
	}
}

func TestCompanyFromURL(t *testing.T) {
	cases := map[string]string{
		"https://careers.goto.com/job/123":             "Goto",
		"https://jobs.lever.co/stripe/abc":             "Stripe",
		"https://boards.greenhouse.io/airbnb/jobs/1":   "Airbnb",
		"https://www.shopee.co.id/careers/x":           "Shopee",
		"https://jobs.ashbyhq.com/plug-and-play/x":     "Plug And Play",
		"https://careers.ey.com/ey/job/Jakarta-X/1234": "Ey",
	}
	for in, want := range cases {
		if got := companyFromURL(in); got != want {
			t.Errorf("companyFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddApplication(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	tracker := filepath.Join(dir, "data", "applications.md")
	os.WriteFile(tracker, []byte("# Applications Tracker\n\n| # | Date | Company | Role | Score | Status | Last Upd | PDF | Report | Notes |\n|---|------|---------|------|-------|--------|----------|-----|--------|-------|\n| 41 | 2026-06-01 | Acme | Analyst | 4.0/5 | Applied | 2026-06-01 | ✅ | [041](reports/041-acme-2026-06-01.md) | On-site, Jakarta, Rp10M |\n"), 0o644)

	num, err := AddApplication(dir, "https://careers.goto.com/job/99", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if num != 42 {
		t.Errorf("expected #42, got #%d", num)
	}

	apps := ParseApplications(dir)
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(apps))
	}
	added := apps[1]
	if added.Company != "Goto" {
		t.Errorf("company = %q, want Goto", added.Company)
	}
	if added.JobURL != "https://careers.goto.com/job/99" {
		t.Errorf("JobURL not enriched from notes: %q", added.JobURL)
	}
	if NormalizeStatus(added.Status) != "evaluated" {
		t.Errorf("status = %q", added.Status)
	}

	// Duplicate URL rejected.
	if _, err := AddApplication(dir, "https://careers.goto.com/job/99", "", ""); err == nil {
		t.Error("expected duplicate URL error")
	}
	// Invalid URL rejected.
	if _, err := AddApplication(dir, "notaurl", "", ""); err == nil {
		t.Error("expected invalid URL error")
	}
}

func TestUpdateJobURL(t *testing.T) {
	dir := t.TempDir()
	repDir := filepath.Join(dir, "reports")
	if err := os.MkdirAll(repDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("replaces existing URL line", func(t *testing.T) {
		p := filepath.Join(repDir, "001-acme-2026-06-01.md")
		os.WriteFile(p, []byte("# Report\n**Score:** 4.2/5\n**URL:** https://old.example.com/x\n**PDF:** ✅\n"), 0o644)
		app := mkApp("reports/001-acme-2026-06-01.md")
		if err := UpdateJobURL(dir, app, "https://new.example.com/y"); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(p)
		if !strings.Contains(string(got), "**URL:** https://new.example.com/y") {
			t.Errorf("URL not replaced:\n%s", got)
		}
		if strings.Contains(string(got), "old.example.com") {
			t.Errorf("old URL still present:\n%s", got)
		}
	})

	t.Run("inserts after Score when URL absent", func(t *testing.T) {
		p := filepath.Join(repDir, "002-beta-2026-06-01.md")
		os.WriteFile(p, []byte("# Report\n**Score:** 3.8/5\n**PDF:** ❌\n"), 0o644)
		app := mkApp("reports/002-beta-2026-06-01.md")
		if err := UpdateJobURL(dir, app, "https://jobs.example.com/z"); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(p)
		idxScore := strings.Index(string(got), "**Score:**")
		idxURL := strings.Index(string(got), "**URL:** https://jobs.example.com/z")
		if idxURL == -1 || idxURL < idxScore {
			t.Errorf("URL not inserted after Score:\n%s", got)
		}
	})

	t.Run("rejects non-http URL", func(t *testing.T) {
		app := mkApp("reports/001-acme-2026-06-01.md")
		if err := UpdateJobURL(dir, app, "javascript:alert(1)"); err == nil {
			t.Error("expected error for non-http URL")
		}
	})

	t.Run("errors without report", func(t *testing.T) {
		app := mkApp("")
		if err := UpdateJobURL(dir, app, "https://x.com"); err == nil {
			t.Error("expected error when ReportPath is empty")
		}
	})
}
