package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/santifer/career-ops/dashboard/internal/data"
	"github.com/santifer/career-ops/dashboard/internal/model"
	"github.com/santifer/career-ops/dashboard/internal/theme"
	"github.com/santifer/career-ops/dashboard/internal/ui/screens"
)

type viewState int

const (
	viewPipeline viewState = iota
	viewReport
	viewProgress
)

// shellDoneMsg signals that a `!` shell command finished and the TUI resumed.
type shellDoneMsg struct{ err error }

type appModel struct {
	layout          screens.LayoutModel
	viewer          screens.ViewerModel
	progress        screens.ProgressModel
	state           viewState
	careerOpsPath   string
	theme           theme.Theme
	progressMetrics model.ProgressMetrics
	// livenessSem bounds concurrent HTTP liveness checks to 3 workers.
	livenessSem chan struct{}
	cacheMu     *sync.Mutex
}

// checkLivenessCmd returns a tea.Cmd that checks one URL (bounded by the semaphore)
// and reports back via LivenessResultMsg.
func (m appModel) checkLivenessCmd(url string, hybrid bool) tea.Cmd {
	sem := m.livenessSem
	path := m.careerOpsPath
	return func() tea.Msg {
		sem <- struct{}{}
		defer func() { <-sem }()
		var res data.LivenessResult
		if hybrid {
			res = data.CheckLivenessHybrid(context.Background(), path, url)
		} else {
			res = data.CheckLivenessHTTP(context.Background(), url)
		}
		return screens.LivenessResultMsg{URL: url, Result: res}
	}
}

// persistLivenessCache writes the current liveness snapshot to disk.
func (m appModel) persistLivenessCache() {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	_ = data.SaveLivenessCache(m.careerOpsPath, m.layout.Pipeline().LivenessSnapshot())
}

func (m *appModel) reloadPipelineData() {
	apps := data.ParseApplications(m.careerOpsPath)
	metrics := data.ComputeMetrics(apps)
	m.progressMetrics = data.ComputeProgressMetrics(apps)
	reloaded := m.layout.Pipeline().WithReloadedData(apps, metrics)
	m.layout.PipelineCopy(reloaded)
}

func (m appModel) Init() tea.Cmd {
	// Kick off background liveness checks for stale URLs (HTTP-only, 3 workers).
	urls := m.layout.Pipeline().StaleURLs()
	if len(urls) == 0 {
		return nil
	}
	m.layout.Pipeline().MarkChecking(urls)
	cmds := make([]tea.Cmd, 0, len(urls))
	for _, u := range urls {
		cmds = append(cmds, m.checkLivenessCmd(u, false))
	}
	return tea.Batch(cmds...)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.layout.Resize(msg.Width, msg.Height)
		if m.state == viewReport {
			m.viewer.Resize(msg.Width, msg.Height)
		}
		if m.state == viewProgress {
			m.progress.Resize(msg.Width, msg.Height)
		}
		lm, cmd := m.layout.Update(msg)
		m.layout = lm
		return m, cmd

	case screens.PipelineClosedMsg:
		return m, tea.Quit

	case screens.PipelineLoadReportMsg:
		archetype, tldr, remote, comp, domain, seniority := data.LoadReportSummary(msg.CareerOpsPath, msg.ReportPath)
		m.layout.Pipeline().EnrichReport(msg.ReportPath, archetype, tldr, remote, comp, domain, seniority)
		return m, nil

	case screens.PipelineUpdateStatusMsg:
		err := data.UpdateApplicationStatus(msg.CareerOpsPath, msg.App, msg.NewStatus)
		if err != nil {
			// Log the error but still reload data to keep UI consistent
			fmt.Fprintf(os.Stderr, "WARN: status update failed: %v\n", err)
		}
		m.reloadPipelineData()
		return m, nil

	case screens.PipelineRefreshMsg:
		m.reloadPipelineData()
		return m, nil

	case screens.PipelineCheckLivenessMsg:
		cmds := make([]tea.Cmd, 0, len(msg.URLs))
		for _, u := range msg.URLs {
			cmds = append(cmds, m.checkLivenessCmd(u, msg.Hybrid))
		}
		return m, tea.Batch(cmds...)

	case screens.LivenessResultMsg:
		m.layout.Pipeline().SetLiveness(msg.URL, msg.Result)
		m.persistLivenessCache()
		return m, nil

	case screens.PipelineUpdateURLMsg:
		if err := data.UpdateJobURL(msg.CareerOpsPath, msg.App, msg.NewURL); err != nil {
			m.layout.SetStatusMsg("URL update failed: " + err.Error())
			return m, nil
		}
		m.reloadPipelineData()
		m.layout.SetStatusMsg("URL saved to " + msg.App.ReportPath)
		// Old URL's liveness no longer applies; check the new one.
		return m, m.checkLivenessCmd(msg.NewURL, false)

	case screens.PipelineUpdateNotesMsg:
		if err := data.UpdateApplicationNotes(msg.CareerOpsPath, msg.App, msg.NewNotes); err != nil {
			m.layout.SetStatusMsg("notes update failed: " + err.Error())
			return m, nil
		}
		m.reloadPipelineData()
		m.layout.SetStatusMsg("notes updated")
		return m, nil

	case screens.PipelineAddEntryMsg:
		num, err := data.AddApplication(msg.CareerOpsPath, msg.URL, msg.Company, "")
		if err != nil {
			m.layout.SetStatusMsg("add failed: " + err.Error())
			return m, nil
		}
		m.reloadPipelineData()
		m.layout.SetStatusMsg(fmt.Sprintf("entry #%d added — checking liveness…", num))
		return m, m.checkLivenessCmd(msg.URL, false)

	case screens.PipelineExecShellMsg:
		c := exec.Command("zsh", "-ic", msg.Command)
		c.Dir = m.careerOpsPath
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return shellDoneMsg{err: err}
		})

	case shellDoneMsg:
		if msg.err != nil {
			m.layout.SetStatusMsg("shell exited with error: " + msg.err.Error())
		} else {
			m.layout.SetStatusMsg("shell command finished — data reloaded")
		}
		m.reloadPipelineData()
		return m, nil

	case screens.PipelineOpenReportMsg:
		m.viewer = screens.NewViewerModel(
			m.theme,
			msg.Path, msg.Title,
			m.layout.Pipeline().Width(), m.layout.Pipeline().Height(),
		)
		m.state = viewReport
		return m, nil

	case screens.ViewerClosedMsg:
		m.state = viewPipeline
		return m, nil

	case screens.PipelineOpenProgressMsg:
		m.progress = screens.NewProgressModel(
			theme.NewTheme("catppuccin-mocha"),
			m.progressMetrics,
			m.layout.Pipeline().Width(), m.layout.Pipeline().Height(),
		)
		m.state = viewProgress
		return m, nil

	case screens.ProgressClosedMsg:
		m.state = viewPipeline
		return m, nil

	case screens.PipelineOpenURLMsg:
		url := msg.URL
		return m, func() tea.Msg {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", url)
			case "linux":
				cmd = exec.Command("xdg-open", url)
			case "windows":
				cmd = exec.Command("cmd", "/c", "start", "", url)
			default:
				cmd = exec.Command("xdg-open", url)
			}
			_ = cmd.Run()
			return nil
		}

	default:
		if m.state == viewReport {
			vm, cmd := m.viewer.Update(msg)
			m.viewer = vm
			return m, cmd
		}
		if m.state == viewProgress {
			pg, cmd := m.progress.Update(msg)
			m.progress = pg
			return m, cmd
		}
		lm, cmd := m.layout.Update(msg)
		m.layout = lm
		return m, cmd
	}
}

func (m appModel) View() string {
	switch m.state {
	case viewReport:
		return m.viewer.View()
	case viewProgress:
		return m.progress.View()
	default:
		return m.layout.View()
	}
}

func main() {
	pathFlag := flag.String("path", ".", "Path to career-ops directory")
	flag.Parse()

	careerOpsPath := *pathFlag

	// Load applications
	apps := data.ParseApplications(careerOpsPath)
	if apps == nil {
		fmt.Fprintf(os.Stderr, "Error: could not find applications.md in %s or %s/data/\n", careerOpsPath, careerOpsPath)
		os.Exit(1)
	}

	// Compute metrics
	metrics := data.ComputeMetrics(apps)
	progressMetrics := data.ComputeProgressMetrics(apps)

	// Create theme
	t := theme.NewTheme("auto")

	// Create layout (sidebar + pipeline)
	layout := screens.NewLayoutModel(t, apps, metrics, careerOpsPath, 120, 40)

	// Batch-load all report summaries
	for _, app := range apps {
		if app.ReportPath == "" {
			continue
		}
		archetype, tldr, remote, comp, domain, seniority := data.LoadReportSummary(careerOpsPath, app.ReportPath)
		if archetype != "" || tldr != "" || remote != "" || comp != "" || domain != "" || seniority != "" {
			layout.Pipeline().EnrichReport(app.ReportPath, archetype, tldr, remote, comp, domain, seniority)
		}
	}

	// Seed liveness state from the on-disk cache (24h TTL).
	layout.Pipeline().SeedLivenessCache(data.LoadLivenessCache(careerOpsPath))

	m := appModel{
		layout:          layout,
		careerOpsPath:   careerOpsPath,
		theme:           t,
		progressMetrics: progressMetrics,
		livenessSem:     make(chan struct{}, 3),
		cacheMu:         &sync.Mutex{},
	}

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
