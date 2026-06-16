package screens

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/santifer/career-ops/dashboard/internal/data"
	"github.com/santifer/career-ops/dashboard/internal/model"
	"github.com/santifer/career-ops/dashboard/internal/theme"
)

// PipelineClosedMsg is emitted when the pipeline screen is dismissed.
type PipelineClosedMsg struct{}

// PipelineOpenReportMsg is emitted when a report should be opened in FileViewer.
type PipelineOpenReportMsg struct {
	Path   string
	Title  string
	JobURL string
}

// PipelineOpenURLMsg is emitted when a job URL should be opened in browser.
type PipelineOpenURLMsg struct {
	URL string
}

// PipelineLoadReportMsg requests lazy loading of a report summary.
type PipelineLoadReportMsg struct {
	CareerOpsPath string
	ReportPath    string
}

// PipelineUpdateStatusMsg requests a status update for an application.
type PipelineUpdateStatusMsg struct {
	CareerOpsPath string
	App           model.CareerApplication
	NewStatus     string
}

// PipelineRefreshMsg requests a full tracker reload from disk.
type PipelineRefreshMsg struct{}

// PipelineCheckLivenessMsg requests liveness checks for the given URLs.
type PipelineCheckLivenessMsg struct {
	URLs   []string
	Hybrid bool // escalate uncertain results to Playwright
}

// LivenessResultMsg carries the outcome of a single liveness check.
type LivenessResultMsg struct {
	URL    string
	Result data.LivenessResult
}

// PipelineUpdateURLMsg requests writing a new job URL into the report file.
type PipelineUpdateURLMsg struct {
	CareerOpsPath string
	App           model.CareerApplication
	NewURL        string
	OldURL        string
}

// PipelineUpdateNotesMsg requests a notes update in applications.md.
type PipelineUpdateNotesMsg struct {
	CareerOpsPath string
	App           model.CareerApplication
	NewNotes      string
}

// PipelineExecShellMsg requests running a shell command (TUI suspends meanwhile).
type PipelineExecShellMsg struct {
	Command string
}

// PipelineAddEntryMsg requests a quick-add of a new tracker entry (URL only).
type PipelineAddEntryMsg struct {
	CareerOpsPath string
	URL           string
	Company       string // optional — derived from URL host when empty
}

// PipelineOpenProgressMsg is emitted when the progress screen should open.
type PipelineOpenProgressMsg struct{}

type reportSummary struct {
	archetype   string
	tldr        string
	remote      string
	comp        string
	domain      string
	seniority   string
	lastUpdated string // ISO date (YYYY-MM-DD) from **Last Updated:** or file mtime
}

// Sort modes
const (
	sortScore   = "score"
	sortDate    = "date"
	sortCompany = "company"
	sortStatus  = "status"
)

// Filter modes
const (
	filterAll       = "all"
	filterEvaluated = "evaluated"
	filterApplied   = "applied"
	filterInterview = "interview"
	filterSkip      = "skip"
	filterRejected  = "rejected"
	filterDiscarded = "discarded"
	filterTop       = "top"
)

type pipelineTab struct {
	filter string
	label  string
}

var pipelineTabs = []pipelineTab{
	{filterAll, "ALL"},
	{filterEvaluated, "EVALUATED"},
	{filterApplied, "APPLIED"},
	{filterInterview, "INTERVIEW"},
	{filterTop, "TOP ≥4"},
	{filterSkip, "SKIP"},
	{filterRejected, "REJECTED"},
	{filterDiscarded, "DISCARDED"},
}

var sortCycle = []string{sortScore, sortDate, sortCompany, sortStatus}

var statusOptions = []string{"Evaluated", "Applied", "Responded", "Interview", "Offer", "Rejected", "Discarded", "SKIP"}

// statusGroupOrder defines display order for grouped view.
var statusGroupOrder = []string{"interview", "offer", "responded", "applied", "evaluated", "skip", "rejected", "discarded"}

// PipelineModel implements the career pipeline dashboard screen.
type PipelineModel struct {
	apps          []model.CareerApplication
	filtered      []model.CareerApplication
	metrics       model.PipelineMetrics
	cursor        int
	scrollOffset  int
	sortMode      string
	activeTab     int
	viewMode      string // "grouped" or "flat"
	width, height int
	theme         theme.Theme
	careerOpsPath string
	reportCache   map[string]reportSummary
	// Status picker sub-state
	statusPicker bool
	statusCursor int
	// Search sub-state — narrows the active tab by substring on company/role/notes.
	searchInput bool   // true while the user is typing the query
	searchQuery string // committed (or in-progress) lowercased query
	// Input-bar sub-state for command (:), shell (!), and URL edit (u) modes.
	inputMode  string // "" | "command" | "shell" | "url"
	inputText  string
	statusMsg  string // one-line feedback shown in the help area; cleared on next keypress
	liveness   map[string]data.LivenessResult
	checkingNo map[string]bool // URLs currently being checked
	// Help overlay sub-state
	showHelp bool
	// Compare mode sub-state
	compareMode  bool
	compareSelected map[int]bool // indices of selected items for comparison
	// Expandable rows sub-state
	expandedRows map[int]bool // indices of expanded rows
	// Inline editing sub-state
	editingRow   int    // index of row being edited (-1 if none)
	editingField string // "notes", "status", etc.
	editingText  string // current editing text
	// Multi-select sub-state
	multiSelectMode bool
	selectedRows    map[int]bool // indices of selected rows
	// Drag and drop sub-state
	dragMode    bool
	dragIndex   int // index of row being dragged
	dropIndex   int // index of drop target
	// Compact mode - skip header/tabs/metrics when in sidebar layout
	compact bool
}

// NewPipelineModel creates a new pipeline screen.
func NewPipelineModel(t theme.Theme, apps []model.CareerApplication, metrics model.PipelineMetrics, careerOpsPath string, width, height int) PipelineModel {
	m := PipelineModel{
		apps:          apps,
		metrics:       metrics,
		sortMode:      sortScore,
		activeTab:     0,
		viewMode:      "grouped",
		width:         width,
		height:        height,
		theme:         t,
		careerOpsPath: careerOpsPath,
		reportCache:   make(map[string]reportSummary),
		liveness:      make(map[string]data.LivenessResult),
		checkingNo:    make(map[string]bool),
		expandedRows:  make(map[int]bool),
		editingRow:    -1,
		selectedRows:  make(map[int]bool),
	}
	m.applyFilterAndSort()
	return m
}

// Init implements tea.Model.
func (m PipelineModel) Init() tea.Cmd {
	return nil
}

// Resize updates dimensions.
func (m *PipelineModel) Resize(width, height int) {
	m.width = width
	m.height = height
}

// Width returns the current width.
func (m PipelineModel) Width() int { return m.width }

// Height returns the current height.
func (m PipelineModel) Height() int { return m.height }

// SetCompact enables compact mode - skips header/tabs/metrics when in sidebar layout.
func (m *PipelineModel) SetCompact(compact bool) {
	m.compact = compact
}

// CopyReportCache copies the report cache from another pipeline model.
func (m *PipelineModel) CopyReportCache(other *PipelineModel) {
	for k, v := range other.reportCache {
		m.reportCache[k] = v
	}
}

// EnrichReport caches report summary data for preview.
func (m *PipelineModel) EnrichReport(reportPath, archetype, tldr, remote, comp, domain, seniority, lastUpdated string) {
	m.reportCache[reportPath] = reportSummary{
		archetype:   archetype,
		tldr:        tldr,
		remote:      remote,
		comp:        comp,
		domain:      domain,
		seniority:   seniority,
		lastUpdated: lastUpdated,
	}
}

// InvalidateReportCache drops the cached report summary for one report so the
// next render re-reads it from disk. Called after status/notes/URL updates so
// the dashboard's "Last Upd" column reflects the new `**Last Updated:**` line
// instead of the stale cached value.
func (m *PipelineModel) InvalidateReportCache(reportPath string) {
	if reportPath == "" {
		return
	}
	delete(m.reportCache, reportPath)
}

// WithReloadedData rebuilds the pipeline with fresh tracker data while preserving
// the current UI state so manual refresh feels seamless.
func (m PipelineModel) WithReloadedData(apps []model.CareerApplication, metrics model.PipelineMetrics) PipelineModel {
	selectedReportPath := ""
	selectedCompany := ""
	selectedRole := ""
	if app, ok := m.CurrentApp(); ok {
		selectedReportPath = app.ReportPath
		selectedCompany = app.Company
		selectedRole = app.Role
	}

	reloaded := NewPipelineModel(m.theme, apps, metrics, m.careerOpsPath, m.width, m.height)
	reloaded.sortMode = m.sortMode
	reloaded.activeTab = m.activeTab
	reloaded.viewMode = m.viewMode
	// Preserve search state across refresh — otherwise pressing `r` silently drops a
	// committed query and the user loses their place mid-investigation.
	reloaded.searchQuery = m.searchQuery
	reloaded.searchInput = m.searchInput
	reloaded.applyFilterAndSort()
	reloaded.CopyReportCache(&m)
	// Preserve liveness state across refresh — checks are expensive.
	for k, v := range m.liveness {
		reloaded.liveness[k] = v
	}
	for k, v := range m.checkingNo {
		reloaded.checkingNo[k] = v
	}

	for i, app := range reloaded.filtered {
		if selectedReportPath != "" && app.ReportPath == selectedReportPath {
			reloaded.cursor = i
			reloaded.adjustScroll()
			return reloaded
		}
		if selectedReportPath == "" && app.Company == selectedCompany && app.Role == selectedRole {
			reloaded.cursor = i
			reloaded.adjustScroll()
			return reloaded
		}
	}

	if len(reloaded.filtered) == 0 {
		reloaded.cursor = 0
		reloaded.scrollOffset = 0
		return reloaded
	}

	if m.cursor >= len(reloaded.filtered) {
		reloaded.cursor = len(reloaded.filtered) - 1
	} else if m.cursor > 0 {
		reloaded.cursor = m.cursor
	}
	reloaded.adjustScroll()
	return reloaded
}

// CurrentApp returns the currently selected application, if any.
func (m PipelineModel) CurrentApp() (model.CareerApplication, bool) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return model.CareerApplication{}, false
	}
	return m.filtered[m.cursor], true
}

// SetLiveness stores a liveness result for a URL (called from main on LivenessResultMsg).
func (m *PipelineModel) SetLiveness(url string, r data.LivenessResult) {
	m.liveness[url] = r
	delete(m.checkingNo, url)
}

// SeedLivenessCache pre-populates liveness state from the on-disk cache.
func (m *PipelineModel) SeedLivenessCache(cache map[string]data.LivenessResult) {
	for k, v := range cache {
		m.liveness[k] = v
	}
}

// LivenessSnapshot returns a copy of the current liveness map (for cache persistence).
func (m PipelineModel) LivenessSnapshot() map[string]data.LivenessResult {
	out := make(map[string]data.LivenessResult, len(m.liveness))
	for k, v := range m.liveness {
		out[k] = v
	}
	return out
}

// MarkChecking flags URLs as in-flight so the UI shows a spinner cell.
func (m *PipelineModel) MarkChecking(urls []string) {
	for _, u := range urls {
		m.checkingNo[u] = true
	}
}

// StaleURLs returns job URLs that need a (re)check: present, non-terminal
// status, and no fresh cached result.
func (m PipelineModel) StaleURLs() []string {
	seen := make(map[string]bool)
	var out []string
	for _, app := range m.apps {
		if app.JobURL == "" || seen[app.JobURL] {
			continue
		}
		norm := data.NormalizeStatus(app.Status)
		if norm == "rejected" || norm == "discarded" || norm == "skip" {
			continue
		}
		if r, ok := m.liveness[app.JobURL]; ok && r.IsFresh() {
			continue
		}
		if m.checkingNo[app.JobURL] {
			continue
		}
		seen[app.JobURL] = true
		out = append(out, app.JobURL)
	}
	return out
}

// SetStatusMsg sets the transient one-line feedback message.
func (m *PipelineModel) SetStatusMsg(s string) { m.statusMsg = s }

// Update handles input for the pipeline screen.
func (m PipelineModel) Update(msg tea.Msg) (PipelineModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.statusPicker {
			return m.handleStatusPicker(msg)
		}
		if m.searchInput {
			return m.handleSearchInput(msg)
		}
		if m.inputMode != "" {
			return m.handleInputBar(msg)
		}
		return m.handleKey(msg)
	case tea.MouseMsg:
		// Mouse scroll support for pipeline
		switch msg.Type {
		case tea.MouseWheelUp:
			if len(m.filtered) > 0 {
				m.cursor--
				if m.cursor < 0 {
					m.cursor = 0
				}
				m.adjustScroll()
			}
		case tea.MouseWheelDown:
			if len(m.filtered) > 0 {
				m.cursor++
				if m.cursor >= len(m.filtered) {
					m.cursor = len(m.filtered) - 1
				}
				m.adjustScroll()
			}
		}
		return m, m.loadCurrentReport()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}
	return m, nil
}

func (m PipelineModel) handleKey(msg tea.KeyMsg) (PipelineModel, tea.Cmd) {
	m.statusMsg = ""

	// Help overlay takes priority
	if m.showHelp {
		if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
			m.showHelp = false
		}
		return m, nil
	}

	switch msg.String() {
	case "?":
		m.showHelp = true
		return m, nil

	case ":":
		m.inputMode = "command"
		m.inputText = ""
		return m, nil

	case "!":
		m.inputMode = "shell"
		m.inputText = ""
		return m, nil

	case "n":
		m.inputMode = "new"
		m.inputText = ""
		return m, nil

	case "u":
		if _, ok := m.CurrentApp(); ok {
			// Start empty so a new link can be pasted directly; the old URL is
			// shown as a hint and restorable with Ctrl+R.
			m.inputMode = "url"
			m.inputText = ""
		}
		return m, nil

	case "L":
		if app, ok := m.CurrentApp(); ok && app.JobURL != "" {
			url := app.JobURL
			m.checkingNo[url] = true
			return m, func() tea.Msg {
				return PipelineCheckLivenessMsg{URLs: []string{url}, Hybrid: true}
			}
		}
		m.statusMsg = "no job URL on this row"
		return m, nil

	case "ctrl+l":
		return m.checkAllStale()

	case "esc":
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.applyFilterAndSort()
			m.cursor = 0
			m.scrollOffset = 0
			return m, m.loadCurrentReport()
		}
		if m.compareMode {
			m.compareMode = false
			m.compareSelected = make(map[int]bool)
			m.statusMsg = ""
			return m, nil
		}
		return m, nil

	case "q", "ctrl+c":
		return m, func() tea.Msg { return PipelineClosedMsg{} }

	case "/":
		m.searchInput = true
		return m, nil

	case "x":
		// Toggle compare mode
		if m.compareMode {
			m.compareMode = false
			m.compareSelected = make(map[int]bool)
			m.statusMsg = ""
		} else {
			m.compareMode = true
			m.compareSelected = make(map[int]bool)
			m.statusMsg = "compare mode: press x to select, enter to compare"
		}
		return m, nil

	case "space":
		if m.compareMode && len(m.filtered) > 0 {
			if m.compareSelected[m.cursor] {
				delete(m.compareSelected, m.cursor)
			} else {
				m.compareSelected[m.cursor] = true
			}
			m.statusMsg = fmt.Sprintf("selected: %d items", len(m.compareSelected))
			return m, nil
		}

	case "down", "j":
		if len(m.filtered) > 0 {
			m.cursor++
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
			m.adjustScroll()
			return m, m.loadCurrentReport()
		}

	case "up", "k":
		if len(m.filtered) > 0 {
			m.cursor--
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()
			return m, m.loadCurrentReport()
		}

	case "s":
		for i, s := range sortCycle {
			if s == m.sortMode {
				m.sortMode = sortCycle[(i+1)%len(sortCycle)]
				break
			}
		}
		m.applyFilterAndSort()
		m.cursor = 0
		m.scrollOffset = 0

	case "f", "right", "l", "tab":
		m.activeTab++
		if m.activeTab >= len(pipelineTabs) {
			m.activeTab = 0
		}
		m.applyFilterAndSort()
		m.cursor = 0
		m.scrollOffset = 0

	case "left", "h", "shift+tab":
		m.activeTab--
		if m.activeTab < 0 {
			m.activeTab = len(pipelineTabs) - 1
		}
		m.applyFilterAndSort()
		m.cursor = 0
		m.scrollOffset = 0

	case "v":
		if m.viewMode == "grouped" {
			m.viewMode = "flat"
		} else {
			m.viewMode = "grouped"
		}

	case "w":
		// wide mode: keep all useful columns visible longer; used when terminal is wide
		// intentionally left as a marker for future horizontal/wide rendering

	case "enter":
		if app, ok := m.CurrentApp(); ok && app.ReportPath != "" {
			fullPath := filepath.Join(m.careerOpsPath, app.ReportPath)
			title := fmt.Sprintf("%s — %s", app.Company, app.Role)
			jobURL := app.JobURL
			return m, func() tea.Msg {
				return PipelineOpenReportMsg{Path: fullPath, Title: title, JobURL: jobURL}
			}
		}

	case "o":
		if app, ok := m.CurrentApp(); ok && app.JobURL != "" {
			return m, func() tea.Msg {
				return PipelineOpenURLMsg{URL: app.JobURL}
			}
		}

	case "p":
		return m, func() tea.Msg { return PipelineOpenProgressMsg{} }

	case "r":
		return m, func() tea.Msg { return PipelineRefreshMsg{} }

	case "c":
		if len(m.filtered) > 0 {
			m.statusPicker = true
			m.statusCursor = 0
		}

	case "g":
		if len(m.filtered) > 0 {
			m.cursor = 0
			m.scrollOffset = 0
			return m, m.loadCurrentReport()
		}

	case "G", "end":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
			m.adjustScroll()
			return m, m.loadCurrentReport()
		}

	case "home":
		if len(m.filtered) > 0 {
			m.cursor = 0
			m.scrollOffset = 0
			return m, m.loadCurrentReport()
		}

	case "pgdown", "ctrl+d", "ctrl+f", " ":
		if len(m.filtered) > 0 {
			halfPage := m.height / 2
			if halfPage < 1 {
				halfPage = 1
			}
			m.cursor += halfPage
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
			m.adjustScroll()
			return m, m.loadCurrentReport()
		}

	case "pgup", "ctrl+u", "ctrl+b":
		if len(m.filtered) > 0 {
			halfPage := m.height / 2
			if halfPage < 1 {
				halfPage = 1
			}
			m.cursor -= halfPage
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.adjustScroll()
			return m, m.loadCurrentReport()
		}

	case "1", "2", "3", "4", "5", "6", "7", "8":
		idx := int(msg.Runes[0] - '1')
		if idx >= 0 && idx < len(pipelineTabs) {
			m.activeTab = idx
			m.applyFilterAndSort()
			m.cursor = 0
			m.scrollOffset = 0
		}

	case "e":
		// Toggle expand/collapse row
		if len(m.filtered) > 0 {
			if m.expandedRows[m.cursor] {
				delete(m.expandedRows, m.cursor)
			} else {
				m.expandedRows[m.cursor] = true
			}
		}

	case "i":
		// Start inline editing for notes
		if len(m.filtered) > 0 {
			m.editingRow = m.cursor
			m.editingField = "notes"
			m.inputMode = "inline_edit"
			m.inputText = ""
		}

	case "m":
		// Toggle multi-select mode
		m.multiSelectMode = !m.multiSelectMode
		if !m.multiSelectMode {
			m.selectedRows = make(map[int]bool)
			m.statusMsg = ""
		} else {
			m.statusMsg = "multi-select mode: space to select, enter to batch action"
		}

	case "ctrl+j":
		// Start drag mode
		if len(m.filtered) > 0 && !m.dragMode {
			m.dragMode = true
			m.dragIndex = m.cursor
			m.statusMsg = "drag mode: use arrow keys to move, enter to drop, esc to cancel"
		}
	}

	return m, nil
}

// handleSearchInput consumes keys while the search input bar is open.
// Esc cancels (closes input AND clears query). Enter commits (closes input,
// keeps query, refreshes filtered list). Backspace + printable chars edit
// the query and live-update the filter so the user sees results as they type.
//
// Report previews are NOT lazy-loaded on every keystroke — that would trigger
// a synchronous os.ReadFile per rune/backspace/ctrl+u and stutter live
// typing. Instead the load fires once when the user commits (Enter) or
// cancels (Esc); subsequent cursor movement in handleKey loads as before.
func (m PipelineModel) handleSearchInput(msg tea.KeyMsg) (PipelineModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.searchInput = false
		m.searchQuery = ""
		m.applyFilterAndSort()
		m.cursor = 0
		m.scrollOffset = 0
		return m, m.loadCurrentReport()

	case "enter":
		m.searchInput = false
		// Query already applied during typing; load the preview for the
		// committed first match (skipped during typing for perf).
		return m, m.loadCurrentReport()

	case "backspace":
		if len(m.searchQuery) > 0 {
			// Drop the last UTF-8 rune so multi-byte characters delete cleanly.
			runes := []rune(m.searchQuery)
			m.searchQuery = string(runes[:len(runes)-1])
			m.applyFilterAndSort()
			m.cursor = 0
			m.scrollOffset = 0
		}
		return m, nil

	case "ctrl+u":
		// vim-flavored: clear the in-progress query without leaving search mode.
		m.searchQuery = ""
		m.applyFilterAndSort()
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil
	}

	// Append printable runes (ignore other special keys like arrows / ctrl-combos).
	if r := msg.Runes; len(r) > 0 {
		m.searchQuery += strings.ToLower(string(r))
		m.applyFilterAndSort()
		m.cursor = 0
		m.scrollOffset = 0
		return m, nil
	}
	return m, nil
}

func (m PipelineModel) handleStatusPicker(msg tea.KeyMsg) (PipelineModel, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.statusPicker = false
		return m, nil

	case "down", "j":
		m.statusCursor++
		if m.statusCursor >= len(statusOptions) {
			m.statusCursor = len(statusOptions) - 1
		}

	case "up", "k":
		m.statusCursor--
		if m.statusCursor < 0 {
			m.statusCursor = 0
		}

	case "enter":
		m.statusPicker = false
		if app, ok := m.CurrentApp(); ok {
			newStatus := statusOptions[m.statusCursor]
			return m, func() tea.Msg {
				return PipelineUpdateStatusMsg{
					CareerOpsPath: m.careerOpsPath,
					App:           app,
					NewStatus:     newStatus,
				}
			}
		}
	}
	return m, nil
}

// checkAllStale queues liveness checks for every stale URL.
func (m PipelineModel) checkAllStale() (PipelineModel, tea.Cmd) {
	urls := m.StaleURLs()
	if len(urls) == 0 {
		m.statusMsg = "all liveness results are fresh"
		return m, nil
	}
	m.MarkChecking(urls)
	m.statusMsg = fmt.Sprintf("checking %d URL(s)…", len(urls))
	return m, func() tea.Msg {
		return PipelineCheckLivenessMsg{URLs: urls, Hybrid: false}
	}
}

// handleInputBar consumes keys while the command/shell/url input bar is open.
func (m PipelineModel) handleInputBar(msg tea.KeyMsg) (PipelineModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = ""
		m.inputText = ""
		return m, nil

	case "enter":
		mode, text := m.inputMode, strings.TrimSpace(m.inputText)
		m.inputMode = ""
		m.inputText = ""
		switch mode {
		case "command":
			return m.executeCommand(text)
		case "shell":
			if text == "" {
				return m, nil
			}
			return m, func() tea.Msg { return PipelineExecShellMsg{Command: text} }
		case "url":
			if text == "" {
				return m, nil
			}
			return m.submitURL(text)
		case "new":
			if text == "" {
				return m, nil
			}
			return m.submitNewEntry(text)
		}
		return m, nil

	case "backspace":
		if len(m.inputText) > 0 {
			runes := []rune(m.inputText)
			m.inputText = string(runes[:len(runes)-1])
		}
		return m, nil

	case "ctrl+u":
		m.inputText = ""
		return m, nil

	case "ctrl+r":
		// In url mode, recall the current URL for small edits.
		if m.inputMode == "url" {
			if app, ok := m.CurrentApp(); ok {
				m.inputText = app.JobURL
			}
		}
		return m, nil
	}

	if r := msg.Runes; len(r) > 0 {
		m.inputText += string(r)
	}
	return m, nil
}

// submitURL validates and dispatches a job URL update for the selected row.
func (m PipelineModel) submitURL(newURL string) (PipelineModel, tea.Cmd) {
	app, ok := m.CurrentApp()
	if !ok {
		m.statusMsg = "no row selected"
		return m, nil
	}
	if !strings.HasPrefix(newURL, "http://") && !strings.HasPrefix(newURL, "https://") {
		m.statusMsg = "URL must start with http:// or https://"
		return m, nil
	}
	old := app.JobURL
	path := m.careerOpsPath
	return m, func() tea.Msg {
		return PipelineUpdateURLMsg{CareerOpsPath: path, App: app, NewURL: newURL, OldURL: old}
	}
}

// submitNewEntry validates and dispatches a quick-add (`n`) entry.
// Input: "<url>" or "<url> <company name>".
func (m PipelineModel) submitNewEntry(input string) (PipelineModel, tea.Cmd) {
	urlPart, company, _ := strings.Cut(input, " ")
	company = strings.TrimSpace(company)
	if !strings.HasPrefix(urlPart, "http://") && !strings.HasPrefix(urlPart, "https://") {
		m.statusMsg = "URL must start with http:// or https://"
		return m, nil
	}
	path := m.careerOpsPath
	return m, func() tea.Msg {
		return PipelineAddEntryMsg{CareerOpsPath: path, URL: urlPart, Company: company}
	}
}

// executeCommand runs a `:` command bar command.
func (m PipelineModel) executeCommand(input string) (PipelineModel, tea.Cmd) {
	if input == "" {
		return m, nil
	}
	cmd, arg, _ := strings.Cut(input, " ")
	arg = strings.TrimSpace(arg)

	switch strings.ToLower(cmd) {
	case "help", "h":
		m.statusMsg = ":new <url>  :url <link>  :note <text>  :status <state>  :check  :checkall  :open  (n=new, L=check, !=shell, u=url)"
		return m, nil

	case "new", "add":
		if arg == "" {
			m.statusMsg = "usage: :new <url> [company]"
			return m, nil
		}
		return m.submitNewEntry(arg)

	case "url":
		if arg == "" {
			m.statusMsg = "usage: :url <link>"
			return m, nil
		}
		return m.submitURL(arg)

	case "note":
		app, ok := m.CurrentApp()
		if !ok {
			m.statusMsg = "no row selected"
			return m, nil
		}
		if arg == "" {
			m.statusMsg = "usage: :note <text>"
			return m, nil
		}
		path := m.careerOpsPath
		return m, func() tea.Msg {
			return PipelineUpdateNotesMsg{CareerOpsPath: path, App: app, NewNotes: arg}
		}

	case "status":
		app, ok := m.CurrentApp()
		if !ok {
			m.statusMsg = "no row selected"
			return m, nil
		}
		var match string
		for _, opt := range statusOptions {
			if strings.EqualFold(opt, arg) {
				match = opt
				break
			}
		}
		if match == "" {
			m.statusMsg = "unknown status — use: " + strings.Join(statusOptions, ", ")
			return m, nil
		}
		path := m.careerOpsPath
		return m, func() tea.Msg {
			return PipelineUpdateStatusMsg{CareerOpsPath: path, App: app, NewStatus: match}
		}

	case "check":
		if app, ok := m.CurrentApp(); ok && app.JobURL != "" {
			url := app.JobURL
			m.checkingNo[url] = true
			return m, func() tea.Msg {
				return PipelineCheckLivenessMsg{URLs: []string{url}, Hybrid: true}
			}
		}
		m.statusMsg = "no job URL on this row"
		return m, nil

	case "checkall":
		return m.checkAllStale()

	case "open", "o":
		if app, ok := m.CurrentApp(); ok && app.JobURL != "" {
			url := app.JobURL
			return m, func() tea.Msg { return PipelineOpenURLMsg{URL: url} }
		}
		m.statusMsg = "no job URL on this row"
		return m, nil
	}

	m.statusMsg = "unknown command: " + cmd + " — try :help"
	return m, nil
}

func (m PipelineModel) loadCurrentReport() tea.Cmd {
	app, ok := m.CurrentApp()
	if !ok || app.ReportPath == "" {
		return nil
	}
	if _, cached := m.reportCache[app.ReportPath]; cached {
		return nil
	}
	path := m.careerOpsPath
	report := app.ReportPath
	return func() tea.Msg {
		return PipelineLoadReportMsg{CareerOpsPath: path, ReportPath: report}
	}
}

// matchesSearch reports whether app contains the query as a case-insensitive
// substring of its company, role, or notes. Empty query matches everything.
// Lowercases both sides so callers don't have to remember the contract.
func matchesSearch(app model.CareerApplication, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(app.Company), q) {
		return true
	}
	if strings.Contains(strings.ToLower(app.Role), q) {
		return true
	}
	if strings.Contains(strings.ToLower(app.Notes), q) {
		return true
	}
	return false
}

// applyFilterAndSort rebuilds the filtered list from apps.
func (m *PipelineModel) applyFilterAndSort() {
	var filtered []model.CareerApplication

	currentFilter := pipelineTabs[m.activeTab].filter
	for _, app := range m.apps {
		if !matchesSearch(app, m.searchQuery) {
			continue
		}
		norm := data.NormalizeStatus(app.Status)
		switch currentFilter {
		case filterAll:
			filtered = append(filtered, app)
		case filterTop:
			if app.Score >= 4.0 && norm != "skip" {
				filtered = append(filtered, app)
			}
		default:
			if norm == currentFilter {
				filtered = append(filtered, app)
			}
		}
	}

	// Sort
	switch m.sortMode {
	case sortScore:
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].Score > filtered[j].Score
		})
	case sortDate:
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].Date > filtered[j].Date
		})
	case sortCompany:
		sort.SliceStable(filtered, func(i, j int) bool {
			return strings.ToLower(filtered[i].Company) < strings.ToLower(filtered[j].Company)
		})
	case sortStatus:
		sort.SliceStable(filtered, func(i, j int) bool {
			return data.StatusPriority(filtered[i].Status) < data.StatusPriority(filtered[j].Status)
		})
	}

	// In grouped mode, always sort by status priority first, then by selected sort within groups
	if m.viewMode == "grouped" {
		sort.SliceStable(filtered, func(i, j int) bool {
			pi := data.StatusPriority(filtered[i].Status)
			pj := data.StatusPriority(filtered[j].Status)
			if pi != pj {
				return pi < pj
			}
			// Within same group, use selected sort
			switch m.sortMode {
			case sortScore:
				return filtered[i].Score > filtered[j].Score
			case sortDate:
				return filtered[i].Date > filtered[j].Date
			case sortCompany:
				return strings.ToLower(filtered[i].Company) < strings.ToLower(filtered[j].Company)
			default:
				return filtered[i].Score > filtered[j].Score
			}
		})
	}

	m.filtered = filtered
}

// chromeRowsFixed returns the number of fixed chrome rows above/below the body
// (header + tabs(2) + metrics + sortbar + help + 1 search bar when active).
// Shared by View() and adjustScroll() so the search-row addition stays in sync.
func (m PipelineModel) chromeRowsFixed() int {
	rows := 8 // header + tabs(2) + metrics + sortbar + help(2) + preview baseline
	if m.searchInput || m.searchQuery != "" {
		rows++
	}
	if m.inputMode != "" {
		rows++
	}
	return rows
}

// previewBudgetApprox is the approximate row count reserved for the preview block
// when computing scroll positioning. View() measures the actual rendered preview
// height; adjustScroll uses this constant to avoid re-rendering on every keystroke.
const previewBudgetApprox = 5

// adjustScroll updates scrollOffset so the cursor stays visible.
func (m *PipelineModel) adjustScroll() {
	availHeight := m.height - m.chromeRowsFixed() - previewBudgetApprox
	if availHeight < 5 {
		availHeight = 5
	}
	line := m.cursorLineEstimate()
	margin := 3

	if line >= m.scrollOffset+availHeight-margin {
		m.scrollOffset = line - availHeight + margin + 1
	}
	if line < m.scrollOffset+margin {
		m.scrollOffset = line - margin
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
}

func (m PipelineModel) cursorLineEstimate() int {
	if m.viewMode != "grouped" {
		return m.cursor
	}
	// Account for group headers
	line := 0
	prevStatus := ""
	for i, app := range m.filtered {
		norm := data.NormalizeStatus(app.Status)
		if norm != prevStatus {
			line++ // group header
			prevStatus = norm
		}
		if i == m.cursor {
			return line
		}
		line++
	}
	return line
}

// colW defines a column: header text, visible width, minimum terminal width to show it.
type colW struct {
	hdr string
	w   int
	min int
}

// columnDefs returns the column layout definitions in order.
// Core 8 columns fit most terminals; extras only on very wide screens.
func columnDefs() []colW {
	return []colW{
		// Always visible
		{"#", 4, 0}, {"Sc", 5, 0}, {"Company", 16, 0},
		// Core columns
		{"Live", 4, 50}, {"Status", 9, 55}, {"Date", 8, 68}, {"Gaji", 14, 82}, {"Vrdct", 6, 98},
		// Medium (wide terminals)
		{"Loc", 9, 115}, {"Level", 8, 130}, {"Last Upd", 10, 145},
		// Extra (very wide terminals only)
		{"Domain", 9, 165}, {"Exp", 3, 185},
	}
}

// visibleCols returns the subset of columns visible at the given width.
func visibleCols(width int) []colW {
	var out []colW
	for _, c := range columnDefs() {
		if width >= c.min {
			out = append(out, c)
		}
	}
	return out
}

// calcRoleW computes Role column width. Role is always column index 3 (after #, Sc, Company).
func (m PipelineModel) calcRoleW() int {
	cols := visibleCols(m.width)
	fixed := 4 + 5 + 16 // #, Sc, Company always visible
	ncols := len(cols)
	for _, c := range cols {
		fixed += c.w
	}
	sepTotal := ncols * 3
	roleW := m.width - fixed - sepTotal - 2
	if roleW < 14 {
		roleW = 14
	}
	if roleW > 32 {
		roleW = 32
	}
	return roleW
}

// buildRow builds a row of cells separated by " │ ".
func buildRow(cells []string) string {
	return strings.Join(cells, " │ ")
}

// -- View --

// View renders the pipeline screen.
func (m PipelineModel) View() string {
	// In compact mode (sidebar layout), skip header/tabs/metrics
	if m.compact {
		return m.viewCompact()
	}
	
	header := m.renderHeader()
	tabs := m.renderTabs()
	metricsBar := m.renderMetrics()
	sortBar := m.renderSortBar()
	searchBar := m.renderSearchBar()
	body := m.renderBody()
	preview := m.renderPreview()
	help := m.renderHelp()

	// Apply scroll to body
	bodyLines := strings.Split(body, "\n")
	if m.scrollOffset > 0 && m.scrollOffset < len(bodyLines) {
		bodyLines = bodyLines[m.scrollOffset:]
	}

	// Calculate available height for body
	previewLines := strings.Count(preview, "\n") + 1
	availHeight := m.height - m.chromeRowsFixed() - previewLines
	if availHeight < 3 {
		availHeight = 3
	}
	if len(bodyLines) > availHeight {
		bodyLines = bodyLines[:availHeight]
	}
	body = strings.Join(bodyLines, "\n")

	// Status picker overlay
	if m.statusPicker {
		body = m.overlayStatusPicker(body)
	}

	sections := []string{header, tabs, metricsBar, sortBar}
	if searchBar != "" {
		sections = append(sections, searchBar)
	}
	if inputBar := m.renderInputBar(); inputBar != "" {
		sections = append(sections, inputBar)
	}
	sections = append(sections, body, preview, help)

	result := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Help overlay
	if m.showHelp {
		result = m.overlayHelp(result)
	}

	return result
}

// viewCompact renders the pipeline in compact mode (no header/tabs/metrics).
func (m PipelineModel) viewCompact() string {
	sortBar := m.renderSortBar()
	searchBar := m.renderSearchBar()
	body := m.renderBody()
	preview := m.renderPreview()
	help := m.renderHelp()

	// Apply scroll to body
	bodyLines := strings.Split(body, "\n")
	if m.scrollOffset > 0 && m.scrollOffset < len(bodyLines) {
		bodyLines = bodyLines[m.scrollOffset:]
	}

	// Calculate available height for body
	previewLines := strings.Count(preview, "\n") + 1
	// In compact mode, use fewer chrome rows
	chromeRows := 2 // sortBar + help
	if searchBar != "" {
		chromeRows++
	}
	if inputBar := m.renderInputBar(); inputBar != "" {
		chromeRows++
	}
	availHeight := m.height - chromeRows - previewLines
	if availHeight < 3 {
		availHeight = 3
	}
	if len(bodyLines) > availHeight {
		bodyLines = bodyLines[:availHeight]
	}
	body = strings.Join(bodyLines, "\n")

	// Status picker overlay
	if m.statusPicker {
		body = m.overlayStatusPicker(body)
	}

	sections := []string{sortBar}
	if searchBar != "" {
		sections = append(sections, searchBar)
	}
	if inputBar := m.renderInputBar(); inputBar != "" {
		sections = append(sections, inputBar)
	}
	sections = append(sections, body, preview, help)

	result := lipgloss.JoinVertical(lipgloss.Left, sections...)

	// Help overlay
	if m.showHelp {
		result = m.overlayHelp(result)
	}

	return result
}

// renderInputBar renders the `:` / `!` / `u` input line when active.
func (m PipelineModel) renderInputBar() string {
	if m.inputMode == "" {
		return ""
	}
	style := lipgloss.NewStyle().
		Foreground(m.theme.Text).
		Width(m.width).
		Padding(0, 2)

	var prompt, hint string
	switch m.inputMode {
	case "command":
		prompt = lipgloss.NewStyle().Bold(true).Foreground(m.theme.Mauve).Render(":")
		hint = "   :help for commands"
	case "shell":
		prompt = lipgloss.NewStyle().Bold(true).Foreground(m.theme.Peach).Render("!")
		hint = "   runs in career-ops dir, Enter to execute"
	case "url":
		prompt = lipgloss.NewStyle().Bold(true).Foreground(m.theme.Sky).Render("url:")
		hint = "   paste link baru → Enter simpan · Ctrl+R isi URL lama · Esc batal"
	case "new":
		prompt = lipgloss.NewStyle().Bold(true).Foreground(m.theme.Green).Render("new:")
		hint = "   paste URL lowongan (opsional: + spasi + nama company) → Enter"
	}

	cursor := lipgloss.NewStyle().Foreground(m.theme.Blue).Render("█")
	hintStyle := lipgloss.NewStyle().Foreground(m.theme.Subtext)
	return style.Render(prompt + " " + m.inputText + cursor + hintStyle.Render(hint))
}

// renderSearchBar returns an empty string when there is no active or in-progress
// search; otherwise it renders a vim-style status line showing the query and the
// match count. While in input mode, a trailing cursor is appended.
func (m PipelineModel) renderSearchBar() string {
	if !m.searchInput && m.searchQuery == "" {
		return ""
	}

	style := lipgloss.NewStyle().
		Foreground(m.theme.Text).
		Width(m.width).
		Padding(0, 2)

	prompt := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Blue).Render("/")
	queryStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	hintStyle := lipgloss.NewStyle().Foreground(m.theme.Subtext)

	display := queryStyle.Render(m.searchQuery)
	if m.searchInput {
		display += lipgloss.NewStyle().Foreground(m.theme.Blue).Render("█")
	}

	tabFiltered := m.countForFilter(pipelineTabs[m.activeTab].filter)
	matchInfo := hintStyle.Render(fmt.Sprintf("  %d/%d matching", len(m.filtered), tabFiltered))

	hint := ""
	if m.searchInput {
		hint = hintStyle.Render("   Enter: keep   Esc: cancel   Ctrl+U: clear")
	} else {
		hint = hintStyle.Render("   Esc: clear   /: edit")
	}

	return style.Render(prompt + " " + display + matchInfo + hint)
}

func (m PipelineModel) renderHeader() string {
	bg := lipgloss.NewStyle().
		Background(m.theme.Surface).
		Width(m.width).
		Padding(0, 2)

	// Enhanced title with better styling
	title := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Blue).Render("  CAREER PIPELINE")

	// Enhanced stats with better formatting
	right := lipgloss.NewStyle().Foreground(m.theme.Subtext)
	avg := fmt.Sprintf("%.1f", m.metrics.AvgScore)
	top := fmt.Sprintf("%.1f", m.metrics.TopScore)
	
	// Color code the stats
	avgStyle := lipgloss.NewStyle().Foreground(m.theme.Yellow).Bold(true)
	topStyle := lipgloss.NewStyle().Foreground(m.theme.Green).Bold(true)
	totalStyle := lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true)
	
	stats := right.Render("  ") +
		totalStyle.Render(fmt.Sprintf("%d", m.metrics.Total)) + right.Render(" lamaran") +
		right.Render(" · ") +
		avgStyle.Render(avg) + right.Render("/5 avg") +
		right.Render(" · ") +
		topStyle.Render(top) + right.Render(" top")

	gap := m.width - lipgloss.Width(title) - lipgloss.Width(stats) - 4
	if gap < 1 {
		gap = 1
	}

	return bg.Render(title + strings.Repeat(" ", gap) + stats)
}

func (m PipelineModel) renderTabs() string {
	var tabs []string

	for i, tab := range pipelineTabs {
		count := m.countForFilter(tab.filter)
		statusColor := m.statusColorMap()[tab.filter]

		// Enhanced pill-style tabs with colored dots and better styling
		dot := lipgloss.NewStyle().Foreground(statusColor).Render("●")
		label := fmt.Sprintf(" %s %s %d ", dot, tab.label, count)

		if i == m.activeTab {
			// Active tab with highlight background
			style := lipgloss.NewStyle().
				Bold(true).
				Foreground(m.theme.Base).
				Background(m.theme.Blue).
				Padding(0, 1).
				MarginRight(1)
			tabs = append(tabs, style.Render(label))
		} else {
			// Inactive tab with subtle styling
			style := lipgloss.NewStyle().
				Foreground(m.theme.Subtext).
				Padding(0, 0)
			tabs = append(tabs, style.Render(label))
		}
	}

	row := strings.Join(tabs, "")

	// Separator line below tabs with gradient effect
	sep := lipgloss.NewStyle().Foreground(m.theme.Overlay).Render(strings.Repeat("─", m.width))

	padStyle := lipgloss.NewStyle().Padding(0, 1)
	return padStyle.Render(row) + "\n" + padStyle.Render(sep)
}

func (m PipelineModel) countForFilter(filter string) int {
	count := 0
	for _, app := range m.apps {
		norm := data.NormalizeStatus(app.Status)
		switch filter {
		case filterAll:
			count++
		case filterTop:
			if app.Score >= 4.0 && norm != "skip" {
				count++
			}
		default:
			if norm == filter {
				count++
			}
		}
	}
	return count
}

func (m PipelineModel) renderMetrics() string {
	style := lipgloss.NewStyle().
		Background(m.theme.Surface).
		Width(m.width).
		Padding(0, 2)

	var parts []string
	statusColors := m.statusColorMap()

	// Add total count first
	totalStyle := lipgloss.NewStyle().Foreground(m.theme.Text).Bold(true)
	parts = append(parts, totalStyle.Render(fmt.Sprintf("TOTAL: %d", m.metrics.Total)))

	// Add separator
	sepStyle := lipgloss.NewStyle().Foreground(m.theme.Overlay)
	parts = append(parts, sepStyle.Render("│"))

	// Add status counts with enhanced styling
	for _, status := range statusGroupOrder {
		count, ok := m.metrics.ByStatus[status]
		if !ok || count == 0 {
			continue
		}
		color := statusColors[status]
		s := lipgloss.NewStyle().Foreground(color).Bold(true)
		parts = append(parts, s.Render(fmt.Sprintf("%d", count))+
			lipgloss.NewStyle().Foreground(m.theme.Subtext).Render(statusLabel(status)))
	}

	// Add separator
	parts = append(parts, sepStyle.Render("│"))

	// Add score summary
	scoreStyle := lipgloss.NewStyle().Foreground(m.theme.Yellow).Bold(true)
	parts = append(parts, scoreStyle.Render(fmt.Sprintf("AVG: %.1f", m.metrics.AvgScore)))
	parts = append(parts, scoreStyle.Render(fmt.Sprintf("TOP: %.1f", m.metrics.TopScore)))

	return style.Render(strings.Join(parts, "  "))
}

func (m PipelineModel) renderSortBar() string {
	style := lipgloss.NewStyle().
		Foreground(m.theme.Subtext).
		Width(m.width).
		Padding(0, 2)

	sortIcon := map[string]string{
		sortScore: "★", sortDate: "📅", sortCompany: "🏢", sortStatus: "📊",
	}[m.sortMode]
	if sortIcon == "" {
		sortIcon = "📋"
	}

	viewIcon := "≡"
	if m.viewMode == "grouped" {
		viewIcon = "▦"
	}

	searchHint := ""
	if m.searchQuery != "" {
		searchHint = fmt.Sprintf("  /%s", m.searchQuery)
	}

	// Sort direction indicator
	sortDir := "▼"
	if m.sortMode == sortDate || m.sortMode == sortCompany || m.sortMode == sortStatus {
		sortDir = "▲"
	}

	// Compare mode indicator
	compareHint := ""
	if m.compareMode {
		compareHint = lipgloss.NewStyle().Foreground(m.theme.Peach).Render(
			fmt.Sprintf("  [CMP:%d]", len(m.compareSelected)))
	}

	return style.Render(fmt.Sprintf("%s %s %s · %d shown%s%s",
		sortIcon, m.sortMode+" "+sortDir, viewIcon, len(m.filtered), searchHint, compareHint))
}

func (m PipelineModel) renderBody() string {
	if len(m.filtered) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(m.theme.Subtext).
			Padding(1, 2)
		return emptyStyle.Render("Tidak ada lamaran yang cocok dengan filter ini")
	}

	var lines []string
	prevStatus := ""
	padStyle := lipgloss.NewStyle().Padding(0, 2)

	// Column header (in flat mode or as first line in grouped)
	if m.viewMode == "flat" {
		lines = append(lines, padStyle.Render(m.renderColumnHeader()))
		// Separator line under header
		sepStyle := lipgloss.NewStyle().Foreground(m.theme.Overlay)
		lines = append(lines, padStyle.Render(sepStyle.Render(strings.Repeat("─", m.width-4))))
	}

	for i, app := range m.filtered {
		norm := data.NormalizeStatus(app.Status)

		// Group header in grouped mode
		if m.viewMode == "grouped" && norm != prevStatus {
			count := m.countByNormStatus(norm)
			color := m.statusColorMap()[norm]
			grpStyle := lipgloss.NewStyle().Bold(true).Foreground(color)
			dimStyle := lipgloss.NewStyle().Foreground(m.theme.Overlay)

			// Status icon with emoji
			statusIcon := map[string]string{
				"interview": "🎯", "offer": "🎉", "responded": "📬",
				"applied": "📤", "evaluated": "📋", "skip": "⏭",
				"rejected": "❌", "discarded": "🗑",
			}[norm]
			if statusIcon == "" {
				statusIcon = "●"
			}

			// Progress bar for group (shows count relative to total)
			totalApps := len(m.filtered)
			progressW := 15
			filled := 0
			if totalApps > 0 {
				filled = count * progressW / totalApps
			}
			progressBar := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled))
			progressBar += dimStyle.Render(strings.Repeat("░", progressW-filled))

			label := fmt.Sprintf(" %s %s (%d) ", statusIcon, statusLabel(norm), count)
			remaining := m.width - 4 - lipgloss.Width(label) - progressW - 2
			if remaining < 0 {
				remaining = 0
			}
			separator := dimStyle.Render(strings.Repeat("─", remaining))
			lines = append(lines, padStyle.Render(
				grpStyle.Render(label)+separator+progressBar,
			))
			// Column header for each group
			lines = append(lines, padStyle.Render(m.renderColumnHeader()))
			lines = append(lines, padStyle.Render(dimStyle.Render(strings.Repeat("─", m.width-4))))
			prevStatus = norm
		}

		selected := i == m.cursor
		compareSelected := m.compareSelected[i]
		isExpanded := m.expandedRows[i]
		isMultiSelected := m.selectedRows[i]
		isDragging := m.dragMode && i == m.dragIndex
		line := m.renderAppLine(app, selected, compareSelected, isExpanded, isMultiSelected, isDragging)
		lines = append(lines, line)

		// Render expanded details if row is expanded
		if isExpanded {
			expandedLines := m.renderExpandedDetails(app)
			lines = append(lines, expandedLines...)
		}
	}

	return strings.Join(lines, "\n")
}

func (m PipelineModel) renderAppLine(app model.CareerApplication, selected bool, compareSelected bool, isExpanded bool, isMultiSelected bool, isDragging bool) string {
	padStyle := lipgloss.NewStyle().Padding(0, 2)
	norm := data.NormalizeStatus(app.Status)
	isDead := norm == "rejected" || norm == "discarded" || norm == "skip"
	roleW := m.calcRoleW()

	dim := func(s lipgloss.Style) lipgloss.Style {
		if isDead {
			return s.Faint(true)
		}
		return s
	}

	// ── Parse all data sources ──────────────────────────────────
	nRemote, nLocation, nComp := parseNotes(app.Notes)
	remoteText, compText, domainText, seniorityText, lastUpdText := "", "", "", "", ""
	if s, ok := m.reportCache[app.ReportPath]; ok {
		remoteText = s.remote
		compText = s.comp
		domainText = s.domain
		seniorityText = s.seniority
		lastUpdText = s.lastUpdated
	}
	if remoteText == "" {
		remoteText = nRemote
	}
	// Comp priority: report's **Comp** > deriveNoteFields PayRange (regex-matched)
	// > parseNotes 3rd-comma fallback. The PayRange branch is what makes IDR/Rp
	// amounts in the notes column render correctly when the report has no
	// **Comp** header line.
	if compText == "" {
		if app.PayRange != "" {
			compText = app.PayRange
			if app.PaySource == "POSTED" {
				compText = compText + " (POSTED)"
			} else if app.PaySource == "est" {
				compText = compText + " (est)"
			}
		} else {
			compText = nComp
		}
	}
	lastUpdRender, lastUpdStyle := renderLastUpd(lastUpdText)

	// Extract location from remoteText if it contains a comma (e.g. "On-site, Singapore")
	rLoc := nLocation
	if rLoc == "" && strings.Contains(remoteText, ",") {
		parts := strings.SplitN(remoteText, ",", 2)
		remoteText = strings.TrimSpace(parts[0])
		rLoc = strings.TrimSpace(parts[1])
	}

	// ── Styles ──────────────────────────────────────────────────
	blue := dim(lipgloss.NewStyle().Foreground(m.theme.Blue).Bold(true))
	green := dim(lipgloss.NewStyle().Foreground(m.theme.Green))
	yellow := dim(lipgloss.NewStyle().Foreground(m.theme.Yellow))
	mauve := dim(lipgloss.NewStyle().Foreground(m.theme.Mauve))
	sub := dim(lipgloss.NewStyle().Foreground(m.theme.Subtext))
	txt := dim(lipgloss.NewStyle().Foreground(m.theme.Text))
	st := dim(lipgloss.NewStyle().Foreground(m.statusColorMap()[norm]))

	// ── Build columns — SAME ORDER as renderColumnHeader ────────
	// 1. # (always) - with compare and expand indicator
	numText := "#"
	if app.Number > 0 {
		numText = fmt.Sprintf("#%d", app.Number)
	}
	if isDead {
		numText = "x" + numText
	}
	
	// Expand/collapse indicator (plain text, no styling)
	expandIndicator := " "
	if isExpanded {
		expandIndicator = "▾"
	} else {
		expandIndicator = "▸"
	}
	
	numStyle := blue
	if compareSelected {
		numStyle = dim(lipgloss.NewStyle().Foreground(m.theme.Peach).Bold(true))
		numText = "▸" + numText[1:]
	}
	
	// Prepend expand indicator to numText
	numText = expandIndicator + numText

	// 2-4. Score, Company, Role (always) - with enhanced score visualization
	scoreText := fmt.Sprintf("%.1f", app.Score)
	scoreStyle := dim(m.scoreStyle(app.Score))

	// Add mini score bar for high scores
	if app.Score >= 4.0 {
		scoreText = fmt.Sprintf("%.1f█", app.Score)
	} else if app.Score >= 3.0 {
		scoreText = fmt.Sprintf("%.1f▌", app.Score)
	} else if app.Score > 0 {
		scoreText = fmt.Sprintf("%.1f.", app.Score)
	}

	cells := []string{
		colCell(numStyle, numText, 4),
		colCell(scoreStyle, scoreText, 5),
		colCell(txt, app.Company, 16),
		colCell(sub, app.Role, roleW),
	}

	// ── Responsive cells — SAME ORDER/SIZE/MIN as columnDefs ────
	type rc struct {
		text   string
		style  lipgloss.Style
		w, min int
	}
	liveText, liveStyle := m.livenessCell(app, dim)
	responsive := []rc{
		{liveText, liveStyle, 4, 50},
		{statusLabel(norm), st, 9, 55},
		{shortDate(app.Date), sub, 8, 68},
		{compText, yellow, 14, 82},
		{verdictIcon(app.Score), green, 6, 98},
		{rLoc, sub, 9, 115},
		{seniorityText, mauve, 8, 130},
		{lastUpdRender, lastUpdStyle, 10, 145},
		{domainText, mauve, 9, 165},
		{m.expLabel(app), blue, 3, 185},
	}
	for _, r := range responsive {
		if m.width >= r.min {
			cells = append(cells, colCell(r.style, r.text, r.w))
		}
	}

	line := buildRow(cells)
	if selected {
		// Enhanced selected row with better visual feedback
		selStyle := lipgloss.NewStyle().
			Background(m.theme.Overlay).
			Foreground(m.theme.Text).
			Width(m.width - 4)
		return padStyle.Render(selStyle.Render(" " + line + " "))
	}
	if compareSelected {
		// Compare selected row with different highlight
		compareStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("52")).
			Foreground(m.theme.Peach).
			Width(m.width - 4)
		return padStyle.Render(compareStyle.Render(" " + line + " "))
	}
	return padStyle.Render(line)
}

// renderExpandedDetails renders the expanded details for a row.
func (m PipelineModel) renderExpandedDetails(app model.CareerApplication) []string {
	padStyle := lipgloss.NewStyle().Padding(0, 2)
	dimStyle := lipgloss.NewStyle().Foreground(m.theme.Subtext)
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.Sky).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(m.theme.Text)
	detailWidth := m.width - 8

	var lines []string

	// Separator line
	lines = append(lines, padStyle.Render(dimStyle.Render(strings.Repeat("─", detailWidth))))

	// Parse notes for additional info
	nRemote, nLocation, nComp := parseNotes(app.Notes)
	remoteText, locText, compText := nRemote, nLocation, ""
	if s, ok := m.reportCache[app.ReportPath]; ok {
		if s.remote != "" {
			remoteText = s.remote
		}
		if s.comp != "" {
			compText = s.comp
		}
	}
	if compText == "" {
		if app.PayRange != "" {
			compText = app.PayRange
			if app.PaySource == "POSTED" {
				compText = compText + " (POSTED)"
			} else if app.PaySource == "est" {
				compText = compText + " (est)"
			}
		} else {
			compText = nComp
		}
	}
	if locText == "" && strings.Contains(remoteText, ",") {
		parts := strings.SplitN(remoteText, ",", 2)
		remoteText = strings.TrimSpace(parts[0])
		locText = strings.TrimSpace(parts[1])
	}

	// Details grid
	detailLines := []string{}

	// Row 1: Location | Work Mode | Compensation
	row1 := []string{}
	if locText != "" {
		row1 = append(row1, labelStyle.Render("Loc: ")+valueStyle.Render(locText))
	}
	if remoteText != "" {
		row1 = append(row1, labelStyle.Render("Mode: ")+valueStyle.Render(remoteText))
	}
	if compText != "" {
		row1 = append(row1, labelStyle.Render("Comp: ")+valueStyle.Render(compText))
	}
	if len(row1) > 0 {
		detailLines = append(detailLines, strings.Join(row1, "  │  "))
	}

	// Row 2: Last Upd | Domain | Level
	row2 := []string{}
	if s, ok := m.reportCache[app.ReportPath]; ok {
		if s.lastUpdated != "" {
			row2 = append(row2, labelStyle.Render("Last Upd: ")+valueStyle.Render(formatTimeAgo(s.lastUpdated)))
		}
		if s.domain != "" {
			row2 = append(row2, labelStyle.Render("Domain: ")+valueStyle.Render(s.domain))
		}
		if s.seniority != "" {
			row2 = append(row2, labelStyle.Render("Level: ")+valueStyle.Render(s.seniority))
		}
	}
	if len(row2) > 0 {
		detailLines = append(detailLines, strings.Join(row2, "  │  "))
	}

	// Row 3: Notes/TL;DR
	noteText := ""
	if s, ok := m.reportCache[app.ReportPath]; ok && s.tldr != "" {
		noteText = s.tldr
	} else if app.Notes != "" {
		noteText = app.Notes
	}
	if noteText != "" {
		detailLines = append(detailLines, labelStyle.Render("Note: ")+dimStyle.Render(truncateRunes(noteText, detailWidth-8)))
	}

	// Row 4: Report link and URL
	if app.ReportPath != "" || app.JobURL != "" {
		linkParts := []string{}
		if app.ReportPath != "" {
			linkParts = append(linkParts, lipgloss.NewStyle().Foreground(m.theme.Blue).Render(app.ReportPath))
		}
		if app.JobURL != "" {
			linkParts = append(linkParts, lipgloss.NewStyle().Foreground(m.theme.Sky).Render(truncateRunes(app.JobURL, detailWidth/2)))
		}
		detailLines = append(detailLines, strings.Join(linkParts, "  │  "))
	}

	// Add detail lines with padding
	for _, dl := range detailLines {
		lines = append(lines, padStyle.Render("  "+dl))
	}

	// Closing separator
	lines = append(lines, padStyle.Render(dimStyle.Render(strings.Repeat("─", detailWidth))))

	return lines
}

// livenessCell returns the Live column text and its style.
// All symbols are single-cell wide so columns stay aligned — emoji like ✅
// render double-width in most terminals and break the grid.
func (m PipelineModel) livenessCell(app model.CareerApplication, dim func(lipgloss.Style) lipgloss.Style) (string, lipgloss.Style) {
	sub := dim(lipgloss.NewStyle().Foreground(m.theme.Subtext))
	if app.JobURL == "" {
		return "-", sub
	}
	if m.checkingNo[app.JobURL] {
		return "…", dim(lipgloss.NewStyle().Foreground(m.theme.Blue))
	}
	r, ok := m.liveness[app.JobURL]
	if !ok {
		return "·", sub
	}
	switch r.State {
	case data.LiveActive:
		return "●", dim(lipgloss.NewStyle().Foreground(m.theme.Green).Bold(true))
	case data.LiveExpired:
		return "✗", dim(lipgloss.NewStyle().Foreground(m.theme.Red).Bold(true))
	case data.LiveUncertain:
		return "?", dim(lipgloss.NewStyle().Foreground(m.theme.Yellow).Bold(true))
	case data.LiveError:
		return "!", dim(lipgloss.NewStyle().Foreground(m.theme.Peach))
	}
	return "·", sub
}

func shortDate(d string) string {
	if len(d) == 10 && d[4] == '-' {
		return d[2:]
	}
	if d == "" {
		return "-"
	}
	return d
}

// formatTimeAgo renders an ISO date (YYYY-MM-DD) as a compact relative
// time string: "0h ago" / "3h ago" (same day), "3d ago" (this week),
// "2w ago" (this month), "5mo ago" (this year), "1y ago" (older).
// Non-date inputs (e.g. "—" or empty) pass through untouched so callers
// can use a single helper for both real and missing values.
func formatTimeAgo(iso string) string {
	if iso == "" || iso == "—" {
		return "—"
	}
	// Accept both date-only ("2026-06-16") and RFC3339 ("2026-06-16T14:32:00+07:00").
	// Date-only is treated as midnight LOCAL; RFC3339 is a real timestamp.
	var t time.Time
	if tt, err := time.Parse(time.RFC3339, iso); err == nil {
		t = tt
	} else if tt, err := time.ParseInLocation("2006-01-02", iso, time.Local); err == nil {
		t = tt
	} else {
		return iso
	}
	now := time.Now()
	if t.After(now) {
		return "0h ago"
	}
	d := now.Sub(t)
	h := int(d.Hours())
	if h < 1 {
		mins := int(d.Minutes())
		if mins < 1 {
			return "now"
		}
		return fmt.Sprintf("%dm ago", mins)
	}
	if h < 24 && t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return fmt.Sprintf("%dh ago", h)
	}
	days := int(d.Hours() / 24)
	switch {
	case days < 7:
		return fmt.Sprintf("%dd ago", days)
	case days < 30:
		return fmt.Sprintf("%dw ago", days/7)
	case days < 365:
		return fmt.Sprintf("%dmo ago", days/30)
	default:
		return fmt.Sprintf("%dy ago", days/365)
	}
}

// renderLastUpd returns the display text and a lipgloss style for the
// "Last Upd" column. Color encodes recency: green ≤7d (recent), yellow
// ≤30d (stale), dim >30d (ancient), and dim for missing values.
func renderLastUpd(iso string) (string, lipgloss.Style) {
	plain := lipgloss.NewStyle()
	if iso == "" {
		return "—", plain.Faint(true)
	}
	var t time.Time
	if tt, err := time.Parse(time.RFC3339, iso); err == nil {
		t = tt
	} else if tt, err := time.ParseInLocation("2006-01-02", iso, time.Local); err == nil {
		t = tt
	} else {
		return iso, plain
	}
	days := int(time.Since(t).Hours() / 24)
	if days < 0 {
		days = 0
	}
	text := formatTimeAgo(iso)
	switch {
	case days <= 7:
		return text, lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	case days <= 30:
		return text, lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	default:
		return text, plain.Faint(true)
	}
}
func rptLabel(n string) string {
	if n != "" {
		return "R" + n
	}
	return "-"
}
func verdictIcon(score float64) string {
	if score >= 4.0 {
		return "APPLY"
	}
	if score >= 3.0 {
		return "WAIT"
	}
	if score > 0 {
		return "SKIP"
	}
	return "-"
}

// expLabel extracts experience level from seniority/role text.
// Returns: "0" (entry), "1" (1yr), "2" (2yr), "3" (3yr), ">3" (senior+)
func (m PipelineModel) expLabel(app model.CareerApplication) string {
	// Check seniority from report cache first
	if s, ok := m.reportCache[app.ReportPath]; ok && s.seniority != "" {
		seniority := strings.ToLower(s.seniority)
		if strings.Contains(seniority, "entry") || strings.Contains(seniority, "fresh") || strings.Contains(seniority, "graduate") || strings.Contains(seniority, "0-2") || strings.Contains(seniority, "0–2") {
			return "0"
		}
		if strings.Contains(seniority, "1-3") || strings.Contains(seniority, "1–3") || strings.Contains(seniority, "junior") {
			return "1"
		}
		if strings.Contains(seniority, "2-4") || strings.Contains(seniority, "2–4") || strings.Contains(seniority, "mid") {
			return "2"
		}
		if strings.Contains(seniority, "3-5") || strings.Contains(seniority, "3–5") || strings.Contains(seniority, "senior") {
			return "3"
		}
		if strings.Contains(seniority, "5+") || strings.Contains(seniority, "lead") || strings.Contains(seniority, "principal") {
			return ">3"
		}
	}
	// Fallback to role text
	text := strings.ToLower(app.Role)
	if strings.Contains(text, "intern") || strings.Contains(text, "trainee") || strings.Contains(text, "graduate") || strings.Contains(text, "entry") || strings.Contains(text, "fresh") {
		return "0"
	}
	if strings.Contains(text, "junior") || strings.Contains(text, "1-2") || strings.Contains(text, "1–2") {
		return "1"
	}
	if strings.Contains(text, "associate") || strings.Contains(text, "analyst") || strings.Contains(text, "2-3") || strings.Contains(text, "2–3") || strings.Contains(text, "0-2") || strings.Contains(text, "0–2") {
		return "2"
	}
	if strings.Contains(text, "senior") || strings.Contains(text, "lead") || strings.Contains(text, "principal") || strings.Contains(text, "3-5") || strings.Contains(text, "3–5") || strings.Contains(text, "5+") {
		return ">3"
	}
	if strings.Contains(text, "management") || strings.Contains(text, "program") || strings.Contains(text, "future leader") {
		return "0"
	}
	return "2" // Default to mid-level
}

// renderColumnHeader returns the column label row with │ separators and sort indicator.
func (m PipelineModel) renderColumnHeader() string {
	hdr := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Blue)
	roleW := m.calcRoleW()
	cols := visibleCols(m.width)

	// Map sort mode to column names
	sortColMap := map[string]string{
		sortScore: "Sc", sortDate: "Date", sortCompany: "Company", sortStatus: "Status",
	}
	sortedCol := sortColMap[m.sortMode]

	// Build header cells with sort indicator and enhanced styling
	cells := []string{
		colCell(hdr, "#", 4), colCell(hdr, "Sc", 5), colCell(hdr, "Company", 16),
		colCell(hdr, "Role", roleW),
	}
	for _, c := range cols {
		if c.hdr == "#" || c.hdr == "Sc" || c.hdr == "Company" {
			continue
		}
		h := c.hdr
		if h == sortedCol {
			// Highlight sorted column header
			h = lipgloss.NewStyle().Bold(true).Foreground(m.theme.Yellow).Render(h + " ▼")
		}
		cells = append(cells, colCell(hdr, h, c.w))
	}
	return buildRow(cells)
}

func (m PipelineModel) renderPreview() string {
	app, ok := m.CurrentApp()
	if !ok {
		return ""
	}

	pad := lipgloss.NewStyle().Padding(0, 2)
	dim := lipgloss.NewStyle().Foreground(m.theme.Subtext)
	bold := lipgloss.NewStyle().Bold(true)
	green := lipgloss.NewStyle().Foreground(m.theme.Green).Bold(true)
	yellow := lipgloss.NewStyle().Foreground(m.theme.Yellow)
	mauve := lipgloss.NewStyle().Foreground(m.theme.Mauve)
	red := lipgloss.NewStyle().Foreground(m.theme.Red)

	// Separator with gradient effect
	sep := dim.Render(strings.Repeat("─", m.width-4))

	// Line 1: "Company — Role" with enhanced styling
	title := bold.Foreground(m.theme.Text).Render(truncateRunes(
		fmt.Sprintf("  %s — %s", app.Company, app.Role), m.width-6))

	// Line 2: Enhanced score bar + verdict + days
	scoreLine := ""
	if app.Score > 0 {
		// Enhanced score bar with gradient colors
		filled := int(app.Score * 2)
		if filled > 10 {
			filled = 10
		}
		
		// Color gradient for score bar
		bar := ""
		for i := 0; i < filled; i++ {
			if i < 4 {
				bar += lipgloss.NewStyle().Foreground(m.theme.Red).Render("█")
			} else if i < 7 {
				bar += lipgloss.NewStyle().Foreground(m.theme.Yellow).Render("█")
			} else {
				bar += lipgloss.NewStyle().Foreground(m.theme.Green).Render("█")
			}
		}
		bar += dim.Render(strings.Repeat("░", 10-filled))
		
		// Score with color coding
		scoreStyle := m.scoreStyle(app.Score)
		bar += scoreStyle.Render(fmt.Sprintf(" %.1f/5", app.Score))

		// Verdict with enhanced styling
		verdict := ""
		if app.Score >= 4.0 {
			verdict = green.Render("  ✓ LAYAK APPLY")
		} else if app.Score >= 3.0 {
			verdict = yellow.Render("  ? PERTIMBANGKAN")
		} else if app.Score > 0 {
			verdict = red.Render("  ✗ SKIP")
		}
		
		// Days since application
		days := ""
		if d := daysSince(app.Date); d != "" {
			days = dim.Render("  (" + d + ")")
		}
		scoreLine = bar + verdict + days
	}

	// Line 3: Enhanced status info with icons
	nRemote, nLocation, nComp := parseNotes(app.Notes)
	remoteText, locText, compText := nRemote, nLocation, ""
	if s, ok := m.reportCache[app.ReportPath]; ok {
		if s.remote != "" {
			remoteText = s.remote
		}
		if s.comp != "" {
			compText = s.comp
		}
	}
	if compText == "" {
		if app.PayRange != "" {
			compText = app.PayRange
			if app.PaySource == "POSTED" {
				compText = compText + " (POSTED)"
			} else if app.PaySource == "est" {
				compText = compText + " (est)"
			}
		} else {
			compText = nComp
		}
	}
	if locText == "" && strings.Contains(remoteText, ",") {
		parts2 := strings.SplitN(remoteText, ",", 2)
		remoteText = strings.TrimSpace(parts2[0])
		locText = strings.TrimSpace(parts2[1])
	}

	// Status with color coding
	statusColor := m.statusColorMap()[data.NormalizeStatus(app.Status)]
	statusStyle := lipgloss.NewStyle().Foreground(statusColor).Bold(true)
	
	// Info parts with enhanced icons
	infoParts := []string{}
	infoParts = append(infoParts, statusStyle.Render("● ")+bold.Render(app.Status))
	if locText != "" {
		infoParts = append(infoParts, dim.Render(" ")+lipgloss.NewStyle().Foreground(m.theme.Peach).Render(" ")+locText)
	}
	if remoteText != "" {
		infoParts = append(infoParts, dim.Render(" ")+lipgloss.NewStyle().Foreground(m.theme.Sky).Render(" ")+remoteText)
	}
	if compText != "" {
		infoParts = append(infoParts, yellow.Render(" ")+compText)
	}
	infoLine := strings.Join(infoParts, "  ")

	// Line 4: Enhanced last-updated + domain + level with better formatting
	extraParts := []string{}
	if s, ok := m.reportCache[app.ReportPath]; ok {
		if s.lastUpdated != "" {
			extraParts = append(extraParts, dim.Render("Last Upd: ")+mauve.Render(formatTimeAgo(s.lastUpdated)))
		}
		if s.domain != "" {
			extraParts = append(extraParts, dim.Render("Domain: ")+mauve.Render(s.domain))
		}
		if s.seniority != "" {
			extraParts = append(extraParts, dim.Render("Level: ")+mauve.Render(s.seniority))
		}
	}
	extraLine := ""
	if len(extraParts) > 0 {
		extraLine = strings.Join(extraParts, "  │  ")
	}

	// Line 5: TL;DR or Notes with better styling
	noteLine := ""
	if s, ok := m.reportCache[app.ReportPath]; ok && s.tldr != "" {
		noteLine = dim.Render(" " + truncateRunes(s.tldr, m.width-6))
	} else if app.Notes != "" {
		noteLine = dim.Render(" " + truncateRunes(app.Notes, m.width-6))
	}

	// Line 6: Enhanced report link with better formatting
	rptLine := ""
	if app.ReportPath != "" {
		rptLine = lipgloss.NewStyle().Foreground(m.theme.Blue).Render(
			fmt.Sprintf("  %s", app.ReportPath))
	}
	if app.JobURL != "" {
		rptLine += dim.Render("  │  ") + lipgloss.NewStyle().Foreground(m.theme.Sky).Render(
			truncateRunes(app.JobURL, m.width-50))
		if r, ok := m.liveness[app.JobURL]; ok {
			var liveStyle lipgloss.Style
			var liveText string
			switch r.State {
			case data.LiveActive:
				liveStyle, liveText = green, "✅ LIVE"
			case data.LiveExpired:
				liveStyle, liveText = red, "❌ EXPIRED"
			case data.LiveUncertain:
				liveStyle, liveText = yellow, "⚠ UNCERTAIN"
			default:
				liveStyle, liveText = dim, "✗ ERROR"
			}
			rptLine += dim.Render("  │  ") + liveStyle.Render(liveText)
		}
	}

	// Assemble with enhanced styling
	var lines []string
	lines = append(lines, pad.Render(sep))
	lines = append(lines, pad.Render(title))
	if scoreLine != "" {
		lines = append(lines, pad.Render(scoreLine))
	}
	lines = append(lines, pad.Render(infoLine))
	if extraLine != "" {
		lines = append(lines, pad.Render(extraLine))
	}
	if noteLine != "" {
		lines = append(lines, pad.Render(noteLine))
	}
	if rptLine != "" {
		lines = append(lines, pad.Render(rptLine))
	}
	lines = append(lines, pad.Render(sep))
	return strings.Join(lines, "\n")
}

func (m PipelineModel) renderHelp() string {
	key := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Blue)
	dim := lipgloss.NewStyle().Foreground(m.theme.Subtext)
	accent := lipgloss.NewStyle().Foreground(m.theme.Yellow)
	green := lipgloss.NewStyle().Foreground(m.theme.Green)

	if m.statusPicker {
		return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(
			key.Render("↑↓") + dim.Render(" pilih  ") +
				key.Render("⏎") + dim.Render(" ok  ") +
				key.Render("Esc") + dim.Render(" batal"))
	}

	if m.searchInput {
		return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(
			key.Render("…") + dim.Render(" filter  ") +
				key.Render("⏎") + dim.Render(" simpan  ") +
				key.Render("Ctrl+U") + dim.Render(" clear  ") +
				key.Render("Esc") + dim.Render(" batal"))
	}

	// Compare mode indicator
	if m.compareMode {
		compareStyle := lipgloss.NewStyle().
			Foreground(m.theme.Peach).
			Bold(true)
		return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(
			compareStyle.Render(" COMPARE MODE ") +
				dim.Render(fmt.Sprintf("  %d selected  ", len(m.compareSelected))) +
				key.Render("Space") + dim.Render(" select  ") +
				key.Render("Esc") + dim.Render(" exit  ") +
				key.Render("Enter") + dim.Render(" compare"))
	}

	// Legend line with styled keys
	expLegend := dim.Render("Exp ") +
		green.Render("0") + dim.Render("=entry ") +
		accent.Render("1") + dim.Render("=junior ") +
		dim.Bold(true).Render("2") + dim.Render("=mid ") +
		accent.Render("3") + dim.Render("=senior ") +
		green.Render(">3") + dim.Render("=lead")

	if m.inputMode != "" {
		return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(
			key.Render("⏎") + dim.Render(" jalankan  ") +
				key.Render("Ctrl+U") + dim.Render(" clear  ") +
				key.Render("Esc") + dim.Render(" batal"))
	}

	// Navigation line with styled keys
	nav := key.Render("↑↓") + dim.Render(" nav ") +
		key.Render("e") + dim.Render(" expand ") +
		key.Render("i") + dim.Render(" edit ") +
		key.Render("m") + dim.Render(" multi ") +
		key.Render("/") + dim.Render(" cari ") +
		key.Render("s") + dim.Render(" urut ") +
		key.Render("⏎") + dim.Render(" buka ") +
		key.Render("c") + dim.Render(" status ") +
		key.Render("n") + dim.Render(" baru ") +
		key.Render("x") + dim.Render(" cmp ") +
		key.Render("L") + dim.Render(" live ") +
		key.Render("?") + dim.Render(" help ") +
		key.Render(":") + dim.Render(" cmd ") +
		key.Render("!") + dim.Render(" shell ") +
		key.Render("u") + dim.Render(" url ") +
		key.Render("r") + dim.Render(" ↻ ") +
		key.Render("q") + dim.Render(" ×")

	if m.statusMsg != "" {
		msgStyle := lipgloss.NewStyle().Foreground(m.theme.Yellow)
		line2 := msgStyle.Render(truncateRunes(m.statusMsg, m.width-4))
		return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(nav + "\n" + line2)
	}

	brand := accent.Render("career-ops")

	// Build lines without background
	gap := m.width - lipgloss.Width(nav) - lipgloss.Width(brand) - 4
	if gap < 1 {
		gap = 1
	}
	line1 := nav + strings.Repeat(" ", gap) + brand

	gap2 := m.width - lipgloss.Width(expLegend) - 4
	if gap2 < 1 {
		gap2 = 1
	}
	line2 := expLegend + strings.Repeat(" ", gap2)

	return lipgloss.NewStyle().Width(m.width).Padding(0, 1).Render(line1 + "\n" + line2)
}

func (m PipelineModel) overlayStatusPicker(body string) string {
	// Render status picker inline at bottom of body
	bodyLines := strings.Split(body, "\n")

	pickerWidth := 30
	padStyle := lipgloss.NewStyle().Padding(0, 2)
	borderStyle := lipgloss.NewStyle().
		Foreground(m.theme.Blue).
		Bold(true)

	var picker []string
	picker = append(picker, padStyle.Render(borderStyle.Render("Change status:")))

	for i, opt := range statusOptions {
		style := lipgloss.NewStyle().Foreground(m.theme.Text).Width(pickerWidth)
		if i == m.statusCursor {
			style = style.Background(m.theme.Overlay).Bold(true)
		}
		prefix := "  "
		if i == m.statusCursor {
			prefix = "> "
		}
		picker = append(picker, padStyle.Render(style.Render(prefix+opt)))
	}

	// Append picker to body
	bodyLines = append(bodyLines, picker...)
	return strings.Join(bodyLines, "\n")
}

// -- Helpers --

func (m PipelineModel) scoreStyle(score float64) lipgloss.Style {
	switch {
	case score >= 4.5:
		return lipgloss.NewStyle().Foreground(m.theme.Green).Bold(true)
	case score >= 4.0:
		return lipgloss.NewStyle().Foreground(m.theme.Green)
	case score >= 3.5:
		return lipgloss.NewStyle().Foreground(m.theme.Yellow)
	case score > 0:
		return lipgloss.NewStyle().Foreground(m.theme.Red)
	default:
		return lipgloss.NewStyle().Foreground(m.theme.Subtext)
	}
}

func (m PipelineModel) statusColorMap() map[string]lipgloss.Color {
	return map[string]lipgloss.Color{
		"interview": m.theme.Green,
		"offer":     m.theme.Green,
		"responded": m.theme.Blue,
		"applied":   m.theme.Sky,
		"evaluated": m.theme.Text,
		"skip":      m.theme.Yellow,
		"rejected":  m.theme.Peach,
		"discarded": m.theme.Subtext,
	}
}

func (m PipelineModel) countByNormStatus(status string) int {
	count := 0
	for _, app := range m.filtered {
		if data.NormalizeStatus(app.Status) == status {
			count++
		}
	}
	return count
}

// truncateRunes truncates a string to at most maxRunes runes, appending "..." if truncated.
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-3]) + "..."
}

func statusLabel(norm string) string {
	switch norm {
	case "interview":
		return "Interview"
	case "offer":
		return "Offer"
	case "responded":
		return "Responded"
	case "applied":
		return "Applied"
	case "evaluated":
		return "Evaluated"
	case "skip":
		return "Skip"
	case "rejected":
		return "Rejected"
	case "discarded":
		return "Discarded"
	default:
		return norm
	}
}

// colPad pads visible text to exactly width characters, truncating with "…" if needed.
// This is the foundation for pixel-perfect column alignment.
func colPad(text string, width int) string {
	runes := []rune(text)
	if len(runes) > width {
		if width <= 1 {
			return string(runes[:width])
		}
		return string(runes[:width-1]) + "…"
	}
	if len(runes) < width {
		return text + strings.Repeat(" ", width-len(runes))
	}
	return text
}

// colCell pads text to width THEN applies ANSI styling.
// Because padding happens on visible chars BEFORE color codes are added,
// every column renders exactly `width` visible characters regardless of color.
func colCell(style lipgloss.Style, text string, width int) string {
	return style.Render(colPad(text, width))
}

// parseNotes extracts remote/location/comp from tracker Notes.
// Format 1: "{type}, {location}, {comp}. {extra}" — e.g. "On-site, Indonesia, Rp10M"
// Format 2: "{location}. {description}" — e.g. "Jakarta. Digital bank. Check exp req."
func parseNotes(notes string) (remote, location, comp string) {
	if notes == "" || strings.HasPrefix(notes, "http://") || strings.HasPrefix(notes, "https://") {
		// Quick-add entries store only the job URL in Notes — nothing to parse.
		return
	}
	parts := strings.Split(notes, ",")
	if len(parts) >= 3 {
		// Format 1: comma-separated
		remote = strings.TrimSpace(parts[0])
		location = strings.TrimSpace(parts[1])
		c := strings.TrimSpace(parts[2])
		// Cut at sentence boundary, but NOT at decimal points inside currency
		dotIdx := strings.Index(c, ".")
		if dotIdx > 0 {
			if dotIdx+1 < len(c) && c[dotIdx+1] >= '0' && c[dotIdx+1] <= '9' {
				nextDot := strings.Index(c[dotIdx+1:], ".")
				if nextDot >= 0 {
					c = c[:dotIdx+1+nextDot]
				}
			} else {
				c = c[:dotIdx]
			}
		}
		comp = c
	} else if len(parts) == 1 {
		// Format 2: period-separated — e.g. "Jakarta. Digital bank. ..."
		segments := strings.SplitN(strings.TrimSpace(parts[0]), ".", 2)
		first := strings.TrimSpace(segments[0])
		// Check if first segment looks like a location (not starting with common non-location words)
		if first != "" {
			lower := strings.ToLower(first)
			nonLoc := []string{"on-site", "hybrid", "remote", "internship", "contract", "skip", "rejected"}
			isLocation := true
			for _, nl := range nonLoc {
				if strings.HasPrefix(lower, nl) {
					isLocation = false
					remote = first
					break
				}
			}
			if isLocation {
				location = first
			}
		}
	}
	return
}

// daysSince returns days between a date string (YYYY-MM-DD or YY-MM-DD) and today.
func daysSince(dateStr string) string {
	if dateStr == "" {
		return ""
	}
	// Normalize: "26-06-09" → "2026-06-09"
	d := dateStr
	if len(d) == 8 && d[2] == '-' {
		d = "20" + d
	}
	t, err := time.ParseInLocation("2006-01-02", d, time.Local)
	if err != nil {
		return ""
	}
	days := int(time.Since(t).Hours() / 24)
	if days < 0 {
		return "future"
	}
	if days == 0 {
		return "today"
	}
	if days == 1 {
		return "1d"
	}
	return fmt.Sprintf("%dd", days)
}

// miniBar returns a tiny 4-char score visualization: ████ or ███░ etc.
func miniBar(score float64) string {
	if score <= 0 {
		return "    "
	}
	filled := int(score * 0.8) // 0-4 scale
	if filled > 4 {
		filled = 4
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", 4-filled)
}

// overlayHelp renders a help overlay on top of the current view.
func (m PipelineModel) overlayHelp(content string) string {
	helpWidth := 60
	if helpWidth > m.width-4 {
		helpWidth = m.width - 4
	}
	helpHeight := 28
	if helpHeight > m.height-4 {
		helpHeight = m.height - 4
	}

	// Help content
	helpLines := []string{
		"╔══════════════════════════════════════════════════════════╗",
		"║                    KEYBOARD SHORTCUTS                   ║",
		"╠══════════════════════════════════════════════════════════╣",
		"║  NAVIGATION                                             ║",
		"║    ↑/↓ or j/k    Move up/down                          ║",
		"║    ←/→ or h/l    Switch tabs                           ║",
		"║    g/G            Jump to top/bottom                    ║",
		"║    PgUp/PgDn      Page up/down                         ║",
		"║    1-8            Jump to tab by number                 ║",
		"╠══════════════════════════════════════════════════════════╣",
		"║  ROW INTERACTIONS                                       ║",
		"║    e              Expand/collapse row details           ║",
		"║    i              Inline edit notes                    ║",
		"║    Enter          Open report viewer                   ║",
		"║    o              Open job URL in browser              ║",
		"╠══════════════════════════════════════════════════════════╣",
		"║  ACTIONS                                                ║",
		"║    c              Change status                        ║",
		"║    n              Add new entry (URL)                  ║",
		"║    u              Edit job URL                         ║",
		"║    L              Check liveness (single)              ║",
		"║    Ctrl+L         Check all stale URLs                 ║",
		"║    r              Refresh data                         ║",
		"║    p              Open progress analytics              ║",
		"╠══════════════════════════════════════════════════════════╣",
		"║  SELECT & ORGANIZE                                      ║",
		"║    m              Toggle multi-select mode             ║",
		"║    Space          Select/deselect item (multi)         ║",
		"║    x              Toggle compare mode                  ║",
		"║    Ctrl+J         Start drag mode                      ║",
		"╠══════════════════════════════════════════════════════════╣",
		"║  SEARCH & FILTER                                        ║",
		"║    /              Start search                         ║",
		"║    Esc            Clear search / close overlay         ║",
		"║    s              Cycle sort mode                      ║",
		"║    v              Toggle grouped/flat view             ║",
		"║    Tab            Toggle sidebar focus                 ║",
		"║    Ctrl+B         Toggle sidebar                       ║",
		"╠══════════════════════════════════════════════════════════╣",
		"║  COMMANDS                                               ║",
		"║    :              Command bar (:help for list)         ║",
		"║    !              Shell command                        ║",
		"║    ?              Toggle this help                     ║",
		"║    q / Ctrl+C     Quit                                 ║",
		"╚══════════════════════════════════════════════════════════╝",
	}

	// Style the help box
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Blue).
		Width(helpWidth).
		Align(lipgloss.Center)

	contentStyle := lipgloss.NewStyle().
		Foreground(m.theme.Text).
		Width(helpWidth)

	// Build help content
	var styledLines []string
	styledLines = append(styledLines, titleStyle.Render("KEYBOARD SHORTCUTS"))
	styledLines = append(styledLines, "")

	for _, line := range helpLines[2:] {
		styledLines = append(styledLines, contentStyle.Render(line))
	}

	helpContent := strings.Join(styledLines, "\n")

	// Create overlay box
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Blue).
		Padding(1, 2).
		Width(helpWidth).
		Height(helpHeight)

	box := boxStyle.Render(helpContent)

	// Center the overlay
	overlayWidth := lipgloss.Width(box)
	overlayHeight := lipgloss.Height(box)

	startX := (m.width - overlayWidth) / 2
	startY := (m.height - overlayHeight) / 2

	// Create semi-transparent background
	bgLines := strings.Split(content, "\n")
	for i := range bgLines {
		if i >= startY && i < startY+overlayHeight {
			overlayLine := i - startY
			overlayLineContent := strings.Split(box, "\n")[overlayLine]
			if startX > 0 && startX+overlayWidth <= m.width {
				// Replace the center portion with overlay content
				prefix := ""
				if startX > 0 {
					prefix = bgLines[i][:startX]
				}
				suffix := ""
				if startX+overlayWidth < len(bgLines[i]) {
					suffix = bgLines[i][startX+overlayWidth:]
				}
				bgLines[i] = prefix + overlayLineContent + suffix
			}
		}
	}

	return strings.Join(bgLines, "\n")
}
