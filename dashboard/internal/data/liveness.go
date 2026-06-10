package data

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Liveness states.
const (
	LiveActive    = "active"
	LiveExpired   = "expired"
	LiveUncertain = "uncertain"
	LiveError     = "error"
)

// LivenessResult is the outcome of checking a job posting URL.
type LivenessResult struct {
	State     string // active | expired | uncertain | error
	Reason    string
	CheckedAt time.Time
}

// LivenessCacheTTL is how long a cached result stays fresh.
const LivenessCacheTTL = 24 * time.Hour

// Patterns ported from liveness-core.mjs — keep in sync.
var (
	hardExpiredPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)job (is )?no longer available`),
		regexp.MustCompile(`(?i)job.*no longer open`),
		regexp.MustCompile(`(?i)position has been filled`),
		regexp.MustCompile(`(?i)this job has expired`),
		regexp.MustCompile(`(?i)job posting has expired`),
		regexp.MustCompile(`(?i)no longer accepting applications`),
		regexp.MustCompile(`(?i)this (position|role|job) (is )?no longer`),
		regexp.MustCompile(`(?i)this job (listing )?is closed`),
		regexp.MustCompile(`(?i)job (listing )?not found`),
		regexp.MustCompile(`(?i)the page you are looking for doesn.t exist`),
		regexp.MustCompile(`(?i)applications?\s+(?:(?:have|are|is)\s+)?closed`),
		regexp.MustCompile(`(?i)closed on \d{1,2}\s+(?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)`),
		regexp.MustCompile(`(?i)closed on (?:jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)\w*\s+\d{1,2}`),
		regexp.MustCompile(`(?i)diese stelle (ist )?(nicht mehr|bereits) besetzt`),
		regexp.MustCompile(`(?i)offre (expirée|n'est plus disponible)`),
		// Indonesian portals
		regexp.MustCompile(`(?i)lowongan (ini )?(sudah|telah) (ditutup|tidak tersedia|berakhir|kedaluwarsa)`),
		regexp.MustCompile(`(?i)lowongan kerja ini sudah tidak aktif`),
	}

	listingPagePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\d+\s+jobs?\s+found`),
		regexp.MustCompile(`(?i)search for jobs page is loaded`),
	}

	expiredURLPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)[?&]error=true`),
	}

	applyPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bapply\b`),
		regexp.MustCompile(`(?i)\bsolicitar\b`),
		regexp.MustCompile(`(?i)\bbewerben\b`),
		regexp.MustCompile(`(?i)\bpostuler\b`),
		regexp.MustCompile(`(?i)\blamar\b`),
		regexp.MustCompile(`(?i)submit application`),
		regexp.MustCompile(`(?i)easy apply`),
		regexp.MustCompile(`(?i)start application`),
		regexp.MustCompile(`(?i)ich bewerbe mich`),
	}

	privateHostPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^localhost$`),
		regexp.MustCompile(`^127\.`),
		regexp.MustCompile(`^10\.`),
		regexp.MustCompile(`^192\.168\.`),
		regexp.MustCompile(`^172\.(1[6-9]|2\d|3[01])\.`),
		regexp.MustCompile(`^169\.254\.`),
		regexp.MustCompile(`^::1$`),
		regexp.MustCompile(`(?i)^fc[0-9a-f]{2}:`),
		regexp.MustCompile(`(?i)^fe80:`),
	}

	reHTMLTag    = regexp.MustCompile(`(?s)<(script|style|noscript)[^>]*>.*?</(script|style|noscript)>`)
	reAnyTag     = regexp.MustCompile(`<[^>]+>`)
	reWhitespace = regexp.MustCompile(`\s+`)
)

const minContentChars = 300

func firstMatch(patterns []*regexp.Regexp, text string) *regexp.Regexp {
	for _, p := range patterns {
		if p.MatchString(text) {
			return p
		}
	}
	return nil
}

// rejectPrivateOrInvalid returns a non-empty reason if the URL must not be fetched.
func rejectPrivateOrInvalid(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "invalid URL"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "unsupported protocol " + parsed.Scheme
	}
	host := parsed.Hostname()
	for _, p := range privateHostPatterns {
		if p.MatchString(host) {
			return "blocked host " + host
		}
	}
	return ""
}

// classifyLiveness mirrors liveness-core.mjs classifyLiveness.
func classifyLiveness(status int, finalURL, bodyText string) LivenessResult {
	now := time.Now()
	if status == 404 || status == 410 {
		return LivenessResult{LiveExpired, fmt.Sprintf("HTTP %d", status), now}
	}
	if p := firstMatch(expiredURLPatterns, finalURL); p != nil {
		return LivenessResult{LiveExpired, "redirect to " + finalURL, now}
	}
	if p := firstMatch(hardExpiredPatterns, bodyText); p != nil {
		return LivenessResult{LiveExpired, "matched: " + p.String(), now}
	}
	if firstMatch(applyPatterns, bodyText) != nil {
		return LivenessResult{LiveActive, "apply control visible", now}
	}
	if p := firstMatch(listingPagePatterns, bodyText); p != nil {
		return LivenessResult{LiveExpired, "listing page: " + p.String(), now}
	}
	if len(strings.TrimSpace(bodyText)) < minContentChars {
		return LivenessResult{LiveExpired, "insufficient content — likely nav/footer only", now}
	}
	return LivenessResult{LiveUncertain, "content present but no apply control found", now}
}

// htmlToText strips tags so the expired/apply patterns match rendered text, not markup.
func htmlToText(html string) string {
	out := reHTMLTag.ReplaceAllString(html, " ")
	out = reAnyTag.ReplaceAllString(out, " ")
	out = strings.NewReplacer("&amp;", "&", "&nbsp;", " ", "&#39;", "'", "&quot;", `"`).Replace(out)
	return reWhitespace.ReplaceAllString(out, " ")
}

// CheckLivenessHTTP performs a fast HTTP-based liveness check (no browser).
// JS-heavy sites may come back uncertain — escalate with CheckLivenessPlaywright.
func CheckLivenessHTTP(ctx context.Context, rawURL string) LivenessResult {
	if reason := rejectPrivateOrInvalid(rawURL); reason != "" {
		return LivenessResult{LiveError, reason, time.Now()}
	}

	client := &http.Client{Timeout: 12 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return LivenessResult{LiveError, err.Error(), time.Now()}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "en,id;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return LivenessResult{LiveError, "fetch failed: " + err.Error(), time.Now()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	text := htmlToText(string(body))
	return classifyLiveness(resp.StatusCode, resp.Request.URL.String(), text)
}

// CheckLivenessPlaywright shells out to check-liveness.mjs (full browser render).
// Returns ok=false when the script or node is unavailable.
func CheckLivenessPlaywright(ctx context.Context, careerOpsPath, rawURL string) (LivenessResult, bool) {
	script := filepath.Join(careerOpsPath, "check-liveness.mjs")
	if _, err := os.Stat(script); err != nil {
		return LivenessResult{}, false
	}
	if _, err := exec.LookPath("node"); err != nil {
		return LivenessResult{}, false
	}

	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "node", script, rawURL)
	cmd.Dir = careerOpsPath
	out, err := cmd.CombinedOutput()
	text := string(out)

	// Parse the per-URL result line: "✅ active ..." / "❌ expired ..." / "⚠️ uncertain ..."
	for _, line := range strings.Split(text, "\n") {
		l := strings.TrimSpace(line)
		switch {
		case strings.Contains(l, "active") && strings.Contains(l, rawURL):
			return LivenessResult{LiveActive, "playwright: apply control visible", time.Now()}, true
		case strings.Contains(l, "expired") && strings.Contains(l, rawURL):
			return LivenessResult{LiveExpired, "playwright: expired", time.Now()}, true
		case strings.Contains(l, "uncertain") && strings.Contains(l, rawURL):
			return LivenessResult{LiveUncertain, "playwright: no apply control", time.Now()}, true
		}
	}
	if err != nil {
		return LivenessResult{LiveError, "playwright check failed", time.Now()}, true
	}
	return LivenessResult{LiveUncertain, "playwright: unparsed output", time.Now()}, true
}

// CheckLivenessHybrid runs the fast HTTP check and escalates uncertain results
// to Playwright when available.
func CheckLivenessHybrid(ctx context.Context, careerOpsPath, rawURL string) LivenessResult {
	res := CheckLivenessHTTP(ctx, rawURL)
	if res.State == LiveUncertain || res.State == LiveError {
		if pw, ok := CheckLivenessPlaywright(ctx, careerOpsPath, rawURL); ok {
			return pw
		}
	}
	return res
}

// --- Cache (data/liveness-cache.tsv) ---

func livenessCachePath(careerOpsPath string) string {
	return filepath.Join(careerOpsPath, "data", "liveness-cache.tsv")
}

// LoadLivenessCache reads the cache file. Missing file returns an empty map.
func LoadLivenessCache(careerOpsPath string) map[string]LivenessResult {
	out := make(map[string]LivenessResult)
	content, err := os.ReadFile(livenessCachePath(careerOpsPath))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 4 || fields[0] == "url" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, fields[3])
		if err != nil {
			continue
		}
		out[fields[0]] = LivenessResult{State: fields[1], Reason: fields[2], CheckedAt: ts}
	}
	return out
}

// SaveLivenessCache writes the whole cache back to disk.
func SaveLivenessCache(careerOpsPath string, cache map[string]LivenessResult) error {
	var b strings.Builder
	b.WriteString("url\tstate\treason\tchecked_at\n")
	for u, r := range cache {
		reason := strings.NewReplacer("\t", " ", "\n", " ").Replace(r.Reason)
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\n", u, r.State, reason, r.CheckedAt.Format(time.RFC3339))
	}
	path := livenessCachePath(careerOpsPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// IsFresh reports whether a cached result is still within TTL.
func (r LivenessResult) IsFresh() bool {
	return r.State != "" && time.Since(r.CheckedAt) < LivenessCacheTTL
}
